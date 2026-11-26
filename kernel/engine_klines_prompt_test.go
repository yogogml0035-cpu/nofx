package kernel

// 引擎行情提示词的周期与成交量口径（子任务 13）：D25 保留周期里的 10s/7d/30d 必须出现在
// 提示词中，成交量列与指标说明明确为 USDT 成交额（D6）。

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"nofx/market"
	"nofx/store"
)

func TestPromptRendersRetainedPeriodsAndUSDTVolume(t *testing.T) {
	cfg := store.GetDefaultStrategyConfig("en")
	engine := NewStrategyEngine(&cfg)

	bar := func(openTime int64, volume float64) market.KlineBar {
		return market.KlineBar{Time: openTime, Open: 0.05, High: 0.06, Low: 0.04, Close: 0.0543, Volume: volume}
	}
	data := &market.Data{
		Symbol:       "DOGEUSDT",
		CurrentPrice: 0.0543,
		TimeframeData: map[string]*market.TimeframeSeriesData{
			"10s": {Timeframe: "10s", Klines: []market.KlineBar{bar(1700000000000, 12345.678)}},
			"7d":  {Timeframe: "7d", Klines: []market.KlineBar{bar(1700000000000, 99999)}},
			"30d": {Timeframe: "30d", Klines: []market.KlineBar{bar(1700000000000, 88888)}},
			"5m":  {Timeframe: "5m", Klines: []market.KlineBar{bar(1700000000000, 77777)}},
			"3m":  {Timeframe: "3m", Klines: []market.KlineBar{bar(1700000000000, 66666)}}, // 非保留周期不渲染
		},
	}

	out := engine.formatMarketData(data)
	assert.Contains(t, out, "=== 10S Timeframe")
	assert.Contains(t, out, "=== 7D Timeframe")
	assert.Contains(t, out, "=== 30D Timeframe")
	assert.Contains(t, out, "=== 5M Timeframe")
	assert.NotContains(t, out, "=== 3M Timeframe", "非 D25 周期不进入提示词")
	assert.Contains(t, out, "Volume(USDT)")
	assert.Contains(t, out, "12345.68")
}

func TestSystemPromptDeclaresUSDTVolume(t *testing.T) {
	cfg := store.GetDefaultStrategyConfig("en")
	engine := NewStrategyEngine(&cfg)
	assert.Contains(t, engine.BuildSystemPrompt(100, "balanced"), "Volume data (USDT quote volume)")

	zhConfig := store.GetDefaultStrategyConfig("zh")
	zhEngine := NewStrategyEngine(&zhConfig)
	assert.Contains(t, zhEngine.BuildSystemPrompt(100, "balanced"), "成交量数据（USDT 成交额）")
}
