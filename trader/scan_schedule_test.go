package trader

// 扫描调度的纯函数测试（子任务 16）：UTC 墙钟边界、收线 + 固定延迟、1w 周一 / 30d 月初、
// 7d Unix 网格与回退语义。

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nofx/market"
	"nofx/store"
)

func utc(y int, m time.Month, d, h, min, sec int) time.Time {
	return time.Date(y, m, d, h, min, sec, 0, time.UTC)
}

func TestNextScanTimeEpochGrid(t *testing.T) {
	delay := 5 * time.Second
	cases := []struct {
		name string
		now  time.Time
		tf   string
		want time.Time
	}{
		{name: "1m 距边界 2s（当前边界触发未到）", now: utc(2026, 11, 25, 12, 0, 2), tf: "1m", want: utc(2026, 11, 25, 12, 0, 5)},
		{name: "1m 边界后 30s", now: utc(2026, 11, 25, 12, 0, 30), tf: "1m", want: utc(2026, 11, 25, 12, 1, 5)},
		{name: "1m 恰在触发时刻", now: utc(2026, 11, 25, 12, 0, 5), tf: "1m", want: utc(2026, 11, 25, 12, 1, 5)},
		{name: "5m", now: utc(2026, 11, 25, 12, 2, 0), tf: "5m", want: utc(2026, 11, 25, 12, 5, 5)},
		{name: "30m", now: utc(2026, 11, 25, 12, 10, 0), tf: "30m", want: utc(2026, 11, 25, 12, 30, 5)},
		{name: "15m 边界后 1s（触发时刻未到）", now: utc(2026, 11, 25, 12, 15, 1), tf: "15m", want: utc(2026, 11, 25, 12, 15, 5)},
		{name: "15m 触发时刻已过", now: utc(2026, 11, 25, 12, 15, 6), tf: "15m", want: utc(2026, 11, 25, 12, 30, 5)},
		{name: "4h", now: utc(2026, 11, 25, 13, 0, 0), tf: "4h", want: utc(2026, 11, 25, 16, 0, 5)},
		{name: "8h", now: utc(2026, 11, 25, 9, 0, 0), tf: "8h", want: utc(2026, 11, 25, 16, 0, 5)},
		{name: "1d UTC 午夜", now: utc(2026, 11, 25, 12, 0, 0), tf: "1d", want: utc(2026, 11, 26, 0, 0, 5)},
		{name: "1d 午夜后 2s", now: utc(2026, 11, 26, 0, 0, 2), tf: "1d", want: utc(2026, 11, 26, 0, 0, 5)},
		{name: "10s 边界后 3s（触发时刻未到）", now: utc(2026, 11, 25, 12, 0, 3), tf: "10s", want: utc(2026, 11, 25, 12, 0, 5)},
		{name: "10s 触发时刻已过", now: utc(2026, 11, 25, 12, 0, 7), tf: "10s", want: utc(2026, 11, 25, 12, 0, 15)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := nextScanTime(c.now, c.tf, delay)
			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}

func TestNextScanTimeWeekAndMonth(t *testing.T) {
	delay := 5 * time.Second

	// 1w：周三 → 下周一 00:00:05；周一 00:00:02 → 当日 00:00:05；周一 00:00:10 → 下周一
	wednesday := utc(2026, 11, 25, 12, 0, 0)
	require.Equal(t, time.Wednesday, wednesday.Weekday())
	got, err := nextScanTime(wednesday, "1w", delay)
	require.NoError(t, err)
	assert.Equal(t, time.Monday, got.Weekday())
	assert.Equal(t, utc(2026, 11, 30, 0, 0, 5), got)

	mondayEarly := utc(2026, 11, 30, 0, 0, 2)
	got, err = nextScanTime(mondayEarly, "1w", delay)
	require.NoError(t, err)
	assert.Equal(t, utc(2026, 11, 30, 0, 0, 5), got)

	mondayLate := utc(2026, 11, 30, 0, 0, 10)
	got, err = nextScanTime(mondayLate, "1w", delay)
	require.NoError(t, err)
	assert.Equal(t, utc(2026, 12, 7, 0, 0, 5), got)

	mondayExact := utc(2026, 11, 30, 0, 0, 5)
	got, err = nextScanTime(mondayExact, "1w", delay)
	require.NoError(t, err)
	assert.Equal(t, utc(2026, 12, 7, 0, 0, 5), got, "恰在周一触发时刻 → 下一周")

	// 30d：11 月 15 日 → 12 月 1 日 00:00:05；12 月 1 日 00:00:02 → 当日；12 月 1 日 00:00:10 → 1 月 1 日
	got, err = nextScanTime(utc(2026, 11, 15, 12, 0, 0), "30d", delay)
	require.NoError(t, err)
	assert.Equal(t, utc(2026, 12, 1, 0, 0, 5), got)

	got, err = nextScanTime(utc(2026, 12, 1, 0, 0, 2), "30d", delay)
	require.NoError(t, err)
	assert.Equal(t, utc(2026, 12, 1, 0, 0, 5), got)

	got, err = nextScanTime(utc(2026, 12, 1, 0, 0, 10), "30d", delay)
	require.NoError(t, err)
	assert.Equal(t, utc(2027, 1, 1, 0, 0, 5), got)

	got, err = nextScanTime(utc(2026, 12, 1, 0, 0, 5), "30d", delay)
	require.NoError(t, err)
	assert.Equal(t, utc(2027, 1, 1, 0, 0, 5), got, "恰在月初触发时刻 → 下一月")
}

func TestNextScanTimeSevenDayGrid(t *testing.T) {
	// 7d 与 Gate 一致：Unix 纪元的 7 天网格（起点为周四）
	now := utc(2026, 11, 25, 12, 0, 0)
	require.Equal(t, time.Wednesday, now.Weekday())
	got, err := nextScanTime(now, "7d", 5*time.Second)
	require.NoError(t, err)
	// 2026-11-26 是周四（Unix 纪元 7 天网格），触发为 00:00:05
	assert.Equal(t, utc(2026, 11, 26, 0, 0, 5), got)
	assert.Equal(t, time.Thursday, got.Weekday())
}

func TestNextScanTimeRejectsBadInput(t *testing.T) {
	if _, err := nextScanTime(utc(2026, 11, 25, 0, 0, 0), "3m", 5*time.Second); err == nil {
		t.Fatal("non-retained timeframe must be rejected")
	}
	if _, err := nextScanTime(utc(2026, 11, 25, 0, 0, 0), "1m", -time.Second); err == nil {
		t.Fatal("negative delay must be rejected")
	}
}

func TestScanCadence(t *testing.T) {
	cases := []struct {
		name       string
		configured time.Duration
		grid       bool
		want       time.Duration
	}{
		{name: "用户配置的间隔优先", configured: 5 * time.Minute, want: 5 * time.Minute},
		{name: "零值回退到下限", configured: 0, want: 3 * time.Minute},
		{name: "负值回退到下限", configured: -time.Minute, want: 3 * time.Minute},
		{name: "低于下限的存量值收敛到下限", configured: time.Minute, want: 3 * time.Minute},
		{name: "高于上限的存量值收敛到上限", configured: 90 * time.Minute, want: 60 * time.Minute},
		{name: "240 分钟收敛到上限", configured: 240 * time.Minute, want: 60 * time.Minute},
		// 网格循环含突破/回撤/日亏检查，旧调度固定 5 分钟，不能被更长的间隔拖慢
		{name: "网格策略封顶 5 分钟", configured: 60 * time.Minute, grid: true, want: 5 * time.Minute},
		{name: "网格策略的短间隔保留", configured: 3 * time.Minute, grid: true, want: 3 * time.Minute},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			at := &AutoTrader{config: AutoTraderConfig{ScanInterval: c.configured}}
			if c.grid {
				at.config.StrategyConfig = &store.StrategyConfig{
					StrategyType: "grid_trading",
					GridConfig:   &store.GridStrategyConfig{},
				}
			}
			assert.Equal(t, c.want, at.scanCadence())
		})
	}
}

func TestLatestDecisionAt(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "scan.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	at := &AutoTrader{id: "trader-1", store: st}

	// 没有记录：返回零值（启动时照常立即跑一轮）
	assert.True(t, at.latestDecisionAt().IsZero())

	recordedAt := time.Now().UTC().Add(-30 * time.Second).Truncate(time.Second)
	require.NoError(t, st.Decision().LogDecision(&store.DecisionRecord{
		TraderID:    at.id,
		CycleNumber: 1,
		Timestamp:   recordedAt,
	}))

	// 跨进程/重启也能读到上一轮决策，用于跳过 1 分钟内的重复立即决策
	assert.WithinDuration(t, recordedAt, at.latestDecisionAt(), time.Second)
}

func TestScanCadenceIgnoresPrimaryTimeframe(t *testing.T) {
	// 原始故障：短线策略的 primary=4h 只用于背景分析，不能把 5 分钟扫描压成 4 小时一轮。
	cfg := store.GetDefaultStrategyConfig("en")
	cfg.Indicators.Klines.PrimaryTimeframe = "4h"
	cfg.Indicators.Klines.SelectedTimeframes = []string{"5m", "4h", "15m", "1h"}
	at := &AutoTrader{config: AutoTraderConfig{
		ScanInterval:   5 * time.Minute,
		StrategyConfig: &cfg,
	}}
	assert.Equal(t, 5*time.Minute, at.scanCadence())
}

func TestScanTimeframeForInterval(t *testing.T) {
	cases := []struct {
		interval time.Duration
		want     string
	}{
		{interval: 5 * time.Minute, want: "5m"},
		{interval: 15 * time.Minute, want: "15m"},
		{interval: 30 * time.Minute, want: "30m"},
		{interval: time.Hour, want: "1h"},
		// 没有同长度的周期：按固定计时器运行，不能擅自换挡
		{interval: 3 * time.Minute, want: ""},
		{interval: 7 * time.Minute, want: ""},
		{interval: time.Minute, want: ""},
		// 超出 3–60 分钟的手动范围，由 scanCadence 收敛，不参与对齐
		{interval: 4 * time.Hour, want: ""},
		{interval: 8 * time.Hour, want: ""},
		{interval: 24 * time.Hour, want: ""},
		// 7d 与 1w 时长相同但边界不同，无法从间隔唯一确定
		{interval: 7 * 24 * time.Hour, want: ""},
		{interval: 0, want: ""},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, scanTimeframeForInterval(c.interval), "interval=%v", c.interval)
	}
}

// 本函数用周期长度作为 minGap 验证公式；生产第一次等待用 1 分钟（见 TestNextScanTriggerOnlyGuardsFirstWait）。
func TestNextAlignedScanRespectsMinimumGap(t *testing.T) {
	delay := 5 * time.Second

	// 首轮在 10:34:30 立即执行；最近的 5m 边界 10:35:05 只隔 35 秒，跳到下一个边界。
	last := utc(2026, 11, 25, 10, 34, 30)
	got, err := nextAlignedScan(utc(2026, 11, 25, 10, 34, 31), "5m", delay, 5*time.Minute, last)
	require.NoError(t, err)
	assert.Equal(t, utc(2026, 11, 25, 10, 40, 5), got)

	// 首轮在 10:30:00，最近的边界 10:35:05 间隔 5m5s，保留。
	last = utc(2026, 11, 25, 10, 30, 0)
	got, err = nextAlignedScan(utc(2026, 11, 25, 10, 30, 1), "5m", delay, 5*time.Minute, last)
	require.NoError(t, err)
	assert.Equal(t, utc(2026, 11, 25, 10, 35, 5), got)

	// 1h 间隔：10:30 首轮后最近边界 11:00:05 只隔 30 分钟，跳到 12:00:05。
	last = utc(2026, 11, 25, 10, 30, 0)
	got, err = nextAlignedScan(utc(2026, 11, 25, 10, 30, 1), "1h", delay, time.Hour, last)
	require.NoError(t, err)
	assert.Equal(t, utc(2026, 11, 25, 12, 0, 5), got)

	// 边界恰好等于间隔：保留（不误跳到再下一个）。
	last = utc(2026, 11, 25, 11, 0, 5)
	got, err = nextAlignedScan(utc(2026, 11, 25, 11, 0, 6), "1h", delay, time.Hour, last)
	require.NoError(t, err)
	assert.Equal(t, utc(2026, 11, 25, 12, 0, 5), got)

	// minGap 非正时不做防抖，直接返回最近边界。
	last = utc(2026, 11, 25, 10, 34, 30)
	got, err = nextAlignedScan(utc(2026, 11, 25, 10, 34, 31), "5m", delay, 0, last)
	require.NoError(t, err)
	assert.Equal(t, utc(2026, 11, 25, 10, 35, 5), got)

	// 墙钟回拨：lastStart 晚于 now 时不做防抖，直接返回最近边界；不能被回拨距离放大成多周期停摆。
	last = utc(2026, 11, 25, 12, 10, 0)
	got, err = nextAlignedScan(utc(2026, 11, 25, 12, 0, 0), "5m", delay, time.Minute, last)
	require.NoError(t, err)
	assert.Equal(t, utc(2026, 11, 25, 12, 0, 5), got)
}

func TestNextScanTriggerOnlyGuardsFirstWait(t *testing.T) {
	delay := 5 * time.Second

	// 启动立即跑过一轮后的第一次等待：循环顶端离边界太近时跳过一个。
	got, aligned, err := nextScanTrigger(
		utc(2026, 11, 25, 10, 34, 31), 5*time.Minute, delay, true,
	)
	require.NoError(t, err)
	assert.True(t, aligned)
	assert.Equal(t, utc(2026, 11, 25, 10, 40, 5), got)

	// 立即轮跑了 65 秒、在边界前 1 秒结束：必须跳过边界，不能刚返回就再调用模型。
	got, aligned, err = nextScanTrigger(
		utc(2026, 11, 25, 12, 5, 4), 5*time.Minute, delay, true,
	)
	require.NoError(t, err)
	assert.True(t, aligned)
	assert.Equal(t, utc(2026, 11, 25, 12, 10, 5), got)

	// 防抖门槛是 1 分钟而不是整个扫描间隔：隔 64 秒的边界要保留，不能拖到下下个边界。
	got, aligned, err = nextScanTrigger(
		utc(2026, 11, 25, 10, 34, 1), 5*time.Minute, delay, true,
	)
	require.NoError(t, err)
	assert.True(t, aligned)
	assert.Equal(t, utc(2026, 11, 25, 10, 35, 5), got)

	// 1 小时策略启动在 10:30：下一次是 11:00 的收线，不是 12:00。
	got, aligned, err = nextScanTrigger(
		utc(2026, 11, 25, 10, 30, 1), time.Hour, delay, true,
	)
	require.NoError(t, err)
	assert.True(t, aligned)
	assert.Equal(t, utc(2026, 11, 25, 11, 0, 5), got)

	// 第一次等待执行后：timer 实际触发比计划晚几毫秒，也必须只等下一个边界，不能翻倍。
	got, aligned, err = nextScanTrigger(
		utc(2026, 11, 25, 10, 40, 6), 5*time.Minute, delay, false,
	)
	require.NoError(t, err)
	assert.True(t, aligned)
	assert.Equal(t, utc(2026, 11, 25, 10, 45, 5), got)

	got, aligned, err = nextScanTrigger(
		utc(2026, 11, 25, 11, 0, 6), time.Hour, delay, false,
	)
	require.NoError(t, err)
	assert.True(t, aligned)
	assert.Equal(t, utc(2026, 11, 25, 12, 0, 5), got)

	// 不对齐的间隔交给固定计时器，调用方不应使用 trigger。
	trigger, aligned, err := nextScanTrigger(
		utc(2026, 11, 25, 10, 34, 31), 3*time.Minute, delay, true,
	)
	require.NoError(t, err)
	assert.False(t, aligned)
	assert.True(t, trigger.IsZero())

	// 错误要向上传，让循环回退到固定间隔。
	_, aligned, err = nextScanTrigger(
		utc(2026, 11, 25, 10, 34, 31), 5*time.Minute, -time.Second, false,
	)
	assert.True(t, aligned)
	assert.Error(t, err)
}

// 周期集合只有一份事实：调度、策略配置与 Gate 取数的周期表必须一致，不能各自漂移
func TestRetainedTimeframesMatchGateIntervals(t *testing.T) {
	require.Len(t, store.RetainedTimeframes, 12)
	for _, timeframe := range store.RetainedTimeframes {
		if _, err := market.GateIntervalDuration(timeframe); err != nil {
			t.Errorf("store.RetainedTimeframes contains %q, which GateIntervalDuration rejects", timeframe)
		}
		if got := store.NormalizeTimeframe(timeframe); got != timeframe {
			t.Errorf("NormalizeTimeframe(%q) = %q, want the same canonical value", timeframe, got)
		}
		if _, err := nextScanTime(utc(2026, 11, 25, 12, 0, 0), timeframe, 5*time.Second); err != nil {
			t.Errorf("nextScanTime rejects retained timeframe %q: %v", timeframe, err)
		}
	}
}
