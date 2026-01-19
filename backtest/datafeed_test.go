package backtest

import (
	"testing"

	"nofx/store"
)

// TestDataFeedUsesStrategyKlineCount verifies that DataFeed uses the kline count from strategy config
func TestDataFeedUsesStrategyKlineCount(t *testing.T) {
	// Create a strategy config with specific kline count
	strategyConfig := &store.StrategyConfig{
		Indicators: store.IndicatorConfig{
			Klines: store.KlineConfig{
				PrimaryTimeframe:   "30m",
				PrimaryCount:       100, // Strategy specifies 100 klines
				SelectedTimeframes: []string{"30m", "4h", "1d"},
			},
			EMAPeriods: []int{13, 55},
			RSIPeriods: []int{14},
			ATRPeriods: []int{14},
		},
	}

	// Create backtest config
	cfg := BacktestConfig{
		RunID:             "test-kline-count",
		Symbols:           []string{"BTCUSDT"},
		Timeframes:        []string{"30m", "4h", "1d"},
		DecisionTimeframe: "30m",
		StartTS:           1704067200, // 2024-01-01
		EndTS:             1704153600, // 2024-01-02
		InitialBalance:    1000,
	}

	// Set loaded strategy
	cfg.SetLoadedStrategy(strategyConfig)

	// Convert to strategy config
	result := cfg.ToStrategyConfig()

	// Verify that kline count is preserved from strategy config
	if result.Indicators.Klines.PrimaryCount != 100 {
		t.Errorf("Expected PrimaryCount=100, got %d", result.Indicators.Klines.PrimaryCount)
	}

	// Verify that timeframes are preserved from strategy config
	if len(result.Indicators.Klines.SelectedTimeframes) != 3 {
		t.Errorf("Expected 3 timeframes, got %d", len(result.Indicators.Klines.SelectedTimeframes))
	}

	// Verify that indicator periods are preserved
	if len(result.Indicators.EMAPeriods) != 2 || result.Indicators.EMAPeriods[0] != 13 {
		t.Errorf("Expected EMAPeriods=[13, 55], got %v", result.Indicators.EMAPeriods)
	}
}
