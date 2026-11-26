package trader

// 扫描调度（子任务 16）：按策略主周期的 K 线收线对齐，收线后固定延迟触发（D17）。
// 边界使用 UTC 墙钟：10s..1d 与 7d 走 Unix 纪元网格；1w 为每周一 00:00；30d 为每月 1 日 00:00。

import (
	"fmt"
	"strings"
	"time"

	"nofx/market"
	"nofx/store"
)

// scanCloseDelay 收线后的固定延迟：给 Gate 收线与本地时钟留出稳定窗口。
const scanCloseDelay = 5 * time.Second

// scanFallbackInterval 返回回退扫描间隔：ScanInterval 非正时用 1 分钟保底（防御旧配置）。
func (at *AutoTrader) scanFallbackInterval() time.Duration {
	if at.config.ScanInterval <= 0 {
		return time.Minute
	}
	return at.config.ScanInterval
}

// scanTimeframe 返回调度使用的周期，与引擎取数的主周期同口径（kernel/engine_analysis.go）：
// PrimaryTimeframe → SelectedTimeframes[0] → "5m"；都不在 D25 保留集合时返回空串（调用方回退到 ScanInterval）。
// 主周期为空的存量策略因此也按引擎实际使用的周期收线对齐，而不是回到固定间隔。
func scanTimeframe(config *store.StrategyConfig) string {
	if config == nil {
		return ""
	}
	klines := config.Indicators.Klines
	candidates := []string{klines.PrimaryTimeframe}
	if len(klines.SelectedTimeframes) > 0 {
		candidates = append(candidates, klines.SelectedTimeframes[0])
	} else {
		candidates = append(candidates, "5m")
	}
	for _, candidate := range candidates {
		tf := strings.ToLower(strings.TrimSpace(candidate))
		if tf == "" {
			continue
		}
		if _, err := market.GateIntervalDuration(tf); err != nil {
			continue
		}
		return tf
	}
	return ""
}

// nextScanTime 返回 now 之后（含当前边界尚未到点的触发时刻）最近的一次扫描时刻：
// 触发时刻 = 周期边界 + delay。1w 为每周一 00:00 UTC；30d 为每月 1 日 00:00 UTC；
// 其余周期（含 7d）按 Unix 纪元的周期网格。
func nextScanTime(now time.Time, timeframe string, delay time.Duration) (time.Time, error) {
	if delay < 0 {
		return time.Time{}, fmt.Errorf("scan delay must not be negative, got %v", delay)
	}
	now = now.UTC()

	switch timeframe {
	case "1w":
		// Monday=0 偏移：Go 的 Sunday=0、Monday=1
		offset := (int(now.Weekday()) + 6) % 7
		monday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -offset)
		trigger := monday.Add(delay)
		if trigger.After(now) {
			return trigger, nil
		}
		return monday.AddDate(0, 0, 7).Add(delay), nil
	case "30d":
		first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		trigger := first.Add(delay)
		if trigger.After(now) {
			return trigger, nil
		}
		return first.AddDate(0, 1, 0).Add(delay), nil
	default:
		duration, err := market.GateIntervalDuration(timeframe)
		if err != nil {
			return time.Time{}, err
		}
		step := int64(duration / time.Second)
		if step <= 0 {
			return time.Time{}, fmt.Errorf("timeframe %q has no whole-second duration", timeframe)
		}
		boundary := (now.Unix() / step) * step
		trigger := time.Unix(boundary, 0).UTC().Add(delay)
		if trigger.After(now) {
			return trigger, nil
		}
		return time.Unix(boundary+step, 0).UTC().Add(delay), nil
	}
}
