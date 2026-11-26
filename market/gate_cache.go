package market

// Gate 公共行情的短 TTL 缓存与并发合并。
//
// 限频事实（2026-11-26 查证 Gate API v4 文档「限频规则」）：公共接口按「单个接口 200r/10s、IP」计，
// 合约 K 线与 tickers 小节没有单独标注；被限频时返回 429 与 JSON 的 label/message，并带
// X-Gate-RateLimit-Requests-Remain / X-Gate-RateLimit-Reset-Timestamp 响应头（文档未承诺 Retry-After）。
//
// 行情被图表、落地页、助手与全部 trader 共用同一个 IP 配额，收线时刻多个 trader 会同时请求
// 同一组合约/周期。这里保证同一个 key 在 TTL 内只打一次上游，并发请求共享同一个结果；
// 错误与 429 不缓存。缓存里的切片按只读对待（调用方不得原地修改）。

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// ErrGateRateLimited 上游返回 429（公共接口限频，按 IP 计）。调用方应稍后重试而不是当作数据缺失。
	ErrGateRateLimited = errors.New("gate rate limited")
	// ErrGateContractNotFound 合约在 Gate 不存在（GET positions/candlesticks 返回 400 CONTRACT_NOT_FOUND）。
	ErrGateContractNotFound = errors.New("gate contract not found")
)

// GateRateLimitError 携带限流响应头里的恢复时间；该头缺失时 ResetAt 为零值。
type GateRateLimitError struct {
	ResetAt time.Time
	Body    string
}

func (e *GateRateLimitError) Error() string {
	if e.ResetAt.IsZero() {
		return fmt.Sprintf("%v (status 429): %s", ErrGateRateLimited, e.Body)
	}
	return fmt.Sprintf("%v (status 429, resets at %s): %s", ErrGateRateLimited, e.ResetAt.UTC().Format(time.RFC3339), e.Body)
}

func (e *GateRateLimitError) Unwrap() error { return ErrGateRateLimited }

// RetryAfter 距离限流窗口恢复的等待时长，至少 1 秒。
func (e *GateRateLimitError) RetryAfter(now time.Time) time.Duration {
	if e.ResetAt.IsZero() {
		return time.Second
	}
	wait := e.ResetAt.Sub(now)
	if wait < time.Second {
		return time.Second
	}
	return wait
}

type gateCacheEntry struct {
	value   any
	expires time.Time
}

type gateCall struct {
	done  chan struct{}
	value any
	err   error
}

var (
	gateCacheMu    sync.Mutex
	gateCache      = map[string]gateCacheEntry{}
	gateInflight   = map[string]*gateCall{}
	gateCacheOn    atomic.Bool
	gateNow        = time.Now
	gateMaxEntries = 512
)

func init() {
	gateCacheOn.Store(true)
}

// SetGateCacheEnabled 供测试开关缓存；返回恢复函数。切换时清空缓存。
func SetGateCacheEnabled(enabled bool) func() {
	previous := gateCacheOn.Load()
	gateCacheOn.Store(enabled)
	FlushGateCache()
	return func() {
		gateCacheOn.Store(previous)
		FlushGateCache()
	}
}

// FlushGateCache 清空已缓存的行情（不清进行中的请求）。
func FlushGateCache() {
	gateCacheMu.Lock()
	gateCache = map[string]gateCacheEntry{}
	gateCacheMu.Unlock()
}

// gateCached 返回 key 的缓存值；未命中时执行 fetch 一次，并发的同 key 请求等待同一个结果。
// 错误不缓存，避免把一次上游故障或 429 固定成 TTL 内的失败。
func gateCached(key string, ttl time.Duration, fetch func() (any, error)) (any, error) {
	if !gateCacheOn.Load() {
		return fetch()
	}
	gateCacheMu.Lock()
	if entry, ok := gateCache[key]; ok && gateNow().Before(entry.expires) {
		gateCacheMu.Unlock()
		return entry.value, nil
	}
	if call, ok := gateInflight[key]; ok {
		gateCacheMu.Unlock()
		<-call.done
		return call.value, call.err
	}
	call := &gateCall{done: make(chan struct{})}
	gateInflight[key] = call
	gateCacheMu.Unlock()

	call.value, call.err = fetch()

	gateCacheMu.Lock()
	if call.err == nil {
		gateSweepLocked()
		gateCache[key] = gateCacheEntry{value: call.value, expires: gateNow().Add(ttl)}
	}
	delete(gateInflight, key)
	gateCacheMu.Unlock()
	close(call.done)
	return call.value, call.err
}

// gateSweepLocked 清掉过期项；仍然超限时丢弃最早的缓存（行情缓存只做降载，丢了会重新取）。
func gateSweepLocked() {
	now := gateNow()
	for key, entry := range gateCache {
		if now.After(entry.expires) || now.Equal(entry.expires) {
			delete(gateCache, key)
		}
	}
	for len(gateCache) >= gateMaxEntries {
		var oldestKey string
		var oldest time.Time
		for key, entry := range gateCache {
			if oldestKey == "" || entry.expires.Before(oldest) {
				oldestKey, oldest = key, entry.expires
			}
		}
		delete(gateCache, oldestKey)
	}
}

// gateKlineCacheTTL K 线缓存时长，按周期缩放到 2–10 秒。
// 收线后不会拿到收线前的快照：这一点由缓存 key 里的 K 线时间桶保证（见 gateKlineCacheKey），
// TTL 只决定同一根 K 线内未收线数据的最大滞后。
func gateKlineCacheTTL(interval string, limit int) time.Duration {
	_ = limit
	// 1w/30d 是自然周/月边界，不在 Unix 纪元网格上，key 不带时间桶：只用 2 秒合并并发请求
	if interval == "1w" || interval == "30d" {
		return 2 * time.Second
	}
	ttl := 10 * time.Second
	if duration, err := GateIntervalDuration(interval); err == nil {
		ttl = duration / 10
	}
	if ttl < 2*time.Second {
		ttl = 2 * time.Second
	}
	if ttl > 10*time.Second {
		ttl = 10 * time.Second
	}
	return ttl
}

// gateKlineCacheKey K 线缓存键。带当前 K 线的时间桶（now / 周期时长）：收线后的请求落入新的桶，
// 永远不会与收线前的快照共用缓存——否则主周期 ≥5m 时，助手快照、试运行或图表在收线前几秒
// 写入的同一组 (合约, 周期, limit) 会让引擎在收线时刻读到缺了刚收那根的旧数据。
// 1w/30d 的边界是自然周/月而不是纪元网格，不带时间桶（TTL 只有 2 秒）。
func gateKlineCacheKey(contract, interval string, limit int) string {
	key := fmt.Sprintf("klines|%s|%s|%d", contract, interval, limit)
	duration, err := GateIntervalDuration(interval)
	if err != nil || interval == "1w" || interval == "30d" {
		return key
	}
	return fmt.Sprintf("%s|%d", key, gateNow().Unix()/int64(duration/time.Second))
}

// classifyGateError 把上游的非 200 响应归一成可判定的错误：429 是限频，
// 400/404 且 label 为 CONTRACT_NOT_FOUND 是未知合约，其余保持带状态码的普通错误。
func classifyGateError(status int, header http.Header, body []byte) error {
	snippet := errorBodySnippet(body)
	if status == http.StatusTooManyRequests {
		limitErr := &GateRateLimitError{Body: snippet}
		if raw := strings.TrimSpace(header.Get("X-Gate-RateLimit-Reset-Timestamp")); raw != "" {
			if seconds, err := parseUnixSeconds(raw); err == nil {
				limitErr.ResetAt = seconds
			}
		}
		return limitErr
	}
	var labeled struct {
		Label string `json:"label"`
	}
	if status == http.StatusBadRequest || status == http.StatusNotFound {
		if json.Unmarshal(body, &labeled) == nil && labeled.Label == "CONTRACT_NOT_FOUND" {
			return fmt.Errorf("%w: %s", ErrGateContractNotFound, snippet)
		}
	}
	return fmt.Errorf("returned status %d: %s", status, snippet)
}

// parseUnixSeconds 解析限流响应头里的 Unix 秒时间戳。
func parseUnixSeconds(raw string) (time.Time, error) {
	seconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(seconds, 0), nil
}
