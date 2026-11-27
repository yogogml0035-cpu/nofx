package trader

// 扫描调度：以用户配置的扫描间隔（ScanInterval）为决策节奏。
// 间隔正好等于某个可对齐的 K 线周期时，决策在该周期收线后固定延迟触发，避免把半根 K 线当完整数据；
// 其余间隔按固定计时器运行。
//
// 2026-11-27 修订（原 D17）：primary_timeframe 是分析口径，不是决策节奏。把主周期当决策周期会让
// “4h 背景 + 5m 触发”的短线策略被压成 4 小时一轮，用户配置的扫描间隔完全失效。扫描间隔是用户
// 唯一能表达“多久决策一次”的控件，必须由它决定节奏；主周期只由引擎取数使用。

import (
	"fmt"
	"time"

	"nofx/market"
)

// scanCloseDelay 收线后的固定延迟：给 Gate 收线与本地时钟留出稳定窗口。
const scanCloseDelay = 5 * time.Second

// scanFallbackInterval 是扫描间隔缺失时的保底值，与手动配置的下限一致。
const scanFallbackInterval = 3 * time.Minute

// 手动面板与助手允许的扫描间隔范围；调度器再收敛一次，防御存量配置（0/1/90/240 等）。
const (
	scanMinInterval = 3 * time.Minute
	scanMaxInterval = 60 * time.Minute
)

// gridScanIntervalCap 网格循环的上限。网格的 RunGridCycle 里带着突破、最大回撤和日亏检查；
// 旧调度没有 ai_config 周期时按 5 分钟运行，扫描间隔不能把这三项安全检查拖慢。
const gridScanIntervalCap = 5 * time.Minute

// scanStartupGuard 是启动/重启立即跑过一轮后，第一次对齐等待的最小间距。
// 只挡住“刚跑完就撞上收线边界、短时间内连续调用模型”的情况；它远小于扫描间隔，
// 不会把第一次常规扫描拖后一个完整周期（否则 1 小时间隔的 trader 启动后要等 1.5 小时）。
const scanStartupGuard = time.Minute

// scanAlignableTimeframes 按升序列出可把扫描间隔映射成 K 线周期的周期。
// 范围与手动配置一致（3–60 分钟）：只有 5/15/30/60 分钟能整周期对齐。7d 与 1w 时长相同
// 但收线边界不同（Unix 周四网格 vs 周一），无法从间隔唯一确定，也不在此列。
var scanAlignableTimeframes = []string{"5m", "15m", "30m", "1h"}

// scanCadence 返回扫描节奏：用户配置的 ScanInterval，收敛到手动配置的 3–60 分钟。
// 低于下限（含非正值）用下限保底（防御旧配置）；超出上限的存量值同样收敛，避免和界面/助手口径不一致。
// 网格策略再压到 5 分钟以内：网格循环包含突破/回撤/日亏检查，不能被更长的扫描间隔拖慢。
func (at *AutoTrader) scanCadence() time.Duration {
	interval := at.config.ScanInterval
	if interval < scanMinInterval {
		interval = scanFallbackInterval
	}
	if interval > scanMaxInterval {
		interval = scanMaxInterval
	}
	if at.IsGridStrategy() && interval > gridScanIntervalCap {
		interval = gridScanIntervalCap
	}
	return interval
}

// scanTimeframeForInterval 返回与扫描间隔等长的 K 线周期；没有完全相等的周期时返回空串。
// 只接受整周期匹配：不对齐的间隔（如 3、7 分钟）按固定计时器运行，不会擅自变快或变慢。
func scanTimeframeForInterval(interval time.Duration) string {
	for _, timeframe := range scanAlignableTimeframes {
		duration, err := market.GateIntervalDuration(timeframe)
		if err == nil && duration == interval {
			return timeframe
		}
	}
	return ""
}

// nextAlignedScan 返回 now 之后最近一次对齐触发时刻，且与参照点 reference 至少相隔 minGap。
// 生产只用于启动后的第一次对齐等待：参照点是循环顶端（立即轮刚结束），不是立即轮开始，
// 避免一轮跑了 60 秒后下一次只隔 1 秒。后续等待直接按边界推进，不参与防抖——否则 timer 实际
// 触发比计划晚几毫秒，每轮的间隔都会比一个周期少一点，防线会误跳一个边界，节奏直接翻倍。
// timeframe 必须是 scanTimeframeForInterval 返回的周期。
func nextAlignedScan(now time.Time, timeframe string, delay, minGap time.Duration, reference time.Time) (time.Time, error) {
	next, err := nextScanTime(now, timeframe, delay)
	if err != nil {
		return time.Time{}, err
	}
	if minGap <= 0 {
		return next, nil
	}
	period, err := market.GateIntervalDuration(timeframe)
	if err != nil {
		return time.Time{}, err
	}
	if period <= 0 {
		return time.Time{}, fmt.Errorf("timeframe %q has no positive duration", timeframe)
	}
	// 墙钟回拨时 reference 可能晚于 now：此时不要用防抖把扫描推到更远的边界。
	if reference.After(now) {
		return next, nil
	}
	// 防抖门槛（1 分钟）总是小于对齐周期（5 分钟起），跳一个边界就够。
	// 用“缺多少就补几个周期”的算法会把时钟回拨放大成多周期停摆，也会引入溢出面。
	if minGap > period {
		minGap = period
	}
	if next.Sub(reference) < minGap {
		next = next.Add(period)
	}
	return next, nil
}

// nextScanTrigger 计算下一次扫描。aligned=false 表示扫描间隔没有同长度的 K 线周期，
// 调用方应使用固定计时器。firstWait=true 表示启动立即跑过一轮后的第一次等待：以循环顶端
// （立即轮刚结束）为起点，下一次触发至少隔 scanStartupGuard，避免一轮跑了 60 秒后下一次只隔 1 秒。
func nextScanTrigger(now time.Time, interval, delay time.Duration, firstWait bool) (trigger time.Time, aligned bool, err error) {
	timeframe := scanTimeframeForInterval(interval)
	if timeframe == "" {
		return time.Time{}, false, nil
	}
	if firstWait {
		trigger, err = nextAlignedScan(now, timeframe, delay, scanStartupGuard, now)
	} else {
		trigger, err = nextScanTime(now, timeframe, delay)
	}
	return trigger, true, err
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
