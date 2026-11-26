package trader

// 扫描调度的纯函数测试（子任务 16）：UTC 墙钟边界、收线 + 固定延迟、1w 周一 / 30d 月初、
// 7d Unix 网格与回退语义。

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func TestScanTimeframe(t *testing.T) {
	base := store.GetDefaultStrategyConfig("en")
	if got := scanTimeframe(nil); got != "" {
		t.Errorf("nil config = %q", got)
	}
	// 与引擎同口径：主周期为空时取 SelectedTimeframes[0]
	cfg := base
	cfg.Indicators.Klines.PrimaryTimeframe = ""
	cfg.Indicators.Klines.SelectedTimeframes = []string{"15m", "1h"}
	assert.Equal(t, "15m", scanTimeframe(&cfg), "主周期为空时取多周期第一个")

	// 主周期与多周期都为空：引擎回退到 5m
	cfg.Indicators.Klines.SelectedTimeframes = nil
	assert.Equal(t, "5m", scanTimeframe(&cfg), "两者都为空时与引擎一致回退到 5m")

	// 旧周期跳过，取下一个候选
	cfg.Indicators.Klines.PrimaryTimeframe = "3m"
	cfg.Indicators.Klines.SelectedTimeframes = []string{"4h", "1d"}
	assert.Equal(t, "4h", scanTimeframe(&cfg), "旧周期跳过后取多周期第一个")

	// 主周期与多周期都是旧周期：回退
	cfg.Indicators.Klines.SelectedTimeframes = []string{"2h"}
	assert.Equal(t, "", scanTimeframe(&cfg), "全部无效时必须回退")

	cfg.Indicators.Klines.PrimaryTimeframe = "30D"
	assert.Equal(t, "30d", scanTimeframe(&cfg))

	cfg.Indicators.Klines.PrimaryTimeframe = "1h"
	assert.Equal(t, "1h", scanTimeframe(&cfg))
}
