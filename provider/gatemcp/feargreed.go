package gatemcp

import (
	"sort"
	"sync"
	"time"
)

// fearGreedHistory 恐惧贪婪指数进程内历史环：
// overview 每次真实刷新（非缓存命中）追加样本，保留 25h；
// change_24h = 当前值 − 与 now-24h 最近的历史样本；跨度不足 24h 时 ok=false（不编造）。
// 上游无变化字段（探针实证），进程重启后历史清零。
type fearGreedHistory struct {
	mu      sync.Mutex
	samples []fgSample
	maxAge  time.Duration
	maxN    int
}

type fgSample struct {
	at    time.Time
	value float64
}

func newFearGreedHistory() *fearGreedHistory {
	// overview TTL 5min → 25h 约 300 样本；maxN 兜底防异常高频刷新撑爆内存
	return &fearGreedHistory{maxAge: 25 * time.Hour, maxN: 400}
}

// add 追加一次真实刷新样本（at 为刷新完成时刻）
func (h *fearGreedHistory) add(at time.Time, value float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	// 同一时刻重复样本（缓存并发重建）不重复记
	if n := len(h.samples); n > 0 && h.samples[n-1].at.Equal(at) {
		return
	}
	h.samples = append(h.samples, fgSample{at: at, value: value})
	h.pruneLocked(at)
}

func (h *fearGreedHistory) pruneLocked(now time.Time) {
	cutoff := now.Add(-h.maxAge)
	i := sort.Search(len(h.samples), func(i int) bool { return h.samples[i].at.After(cutoff) })
	if i > 0 {
		h.samples = append(h.samples[:0], h.samples[i:]...)
	}
	if len(h.samples) > h.maxN {
		h.samples = h.samples[len(h.samples)-h.maxN:]
	}
}

// first 返回当前保留的最早样本时间（first_seen_at）
func (h *fearGreedHistory) first() (time.Time, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.samples) == 0 {
		return time.Time{}, false
	}
	return h.samples[0].at, true
}

// change24h 计算当前值相对 now-24h 最近样本的变化；历史跨度不足 24h 时 ok=false
func (h *fearGreedHistory) change24h(now time.Time, current float64) (float64, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pruneLocked(now)
	target := now.Add(-24 * time.Hour)
	// 样本按时间递增；找最后一个 at ≤ target 的样本（即距 target 最近的过去样本）
	idx := sort.Search(len(h.samples), func(i int) bool { return h.samples[i].at.After(target) })
	if idx == 0 {
		return 0, false
	}
	return current - h.samples[idx-1].value, true
}
