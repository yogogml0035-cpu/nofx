package backtest

import (
	"database/sql"
	"encoding/json"
	"nofx/store"
	"testing"
)

func TestToStrategyConfig_PreservesKlineCount(t *testing.T) {
	// Create a strategy config with custom kline count
	strategyConfig := &store.StrategyConfig{
		CoinSource: store.CoinSourceConfig{
			SourceType:  "static",
			StaticCoins: []string{"BTCUSDT", "ETHUSDT"},
		},
		Indicators: store.IndicatorConfig{
			Klines: store.KlineConfig{
				PrimaryTimeframe:     "5m",
				PrimaryCount:         50, // Custom kline count
				LongerTimeframe:      "1h",
				LongerCount:          20,
				EnableMultiTimeframe: true,
				SelectedTimeframes:   []string{"5m", "15m", "1h"},
			},
			EnableRawKlines: true,
			EnableEMA:       true,
			EMAPeriods:      []int{20, 50},
		},
		RiskControl: store.RiskControlConfig{
			MaxPositions:       3,
			BTCETHMaxLeverage:  5,
			AltcoinMaxLeverage: 3,
			MaxMarginUsage:     0.8,
			MinPositionSize:    10,
			MinRiskRewardRatio: 2.0,
			MinConfidence:      70,
		},
	}

	// Create backtest config with strategy
	cfg := BacktestConfig{
		RunID:             "test_run",
		UserID:            "test_user",
		StrategyID:        "test_strategy",
		Symbols:           []string{"BTCUSDT"},
		Timeframes:        []string{"5m"},
		DecisionTimeframe: "5m",
		StartTS:           1000000,
		EndTS:             2000000,
		InitialBalance:    1000,
		Leverage: LeverageConfig{
			BTCETHLeverage:  5,
			AltcoinLeverage: 3,
		},
	}

	// Set loaded strategy
	cfg.SetLoadedStrategy(strategyConfig)

	// Convert to strategy config
	result := cfg.ToStrategyConfig()

	// Verify kline count is preserved from strategy
	if result.Indicators.Klines.PrimaryCount != 50 {
		t.Errorf("Expected PrimaryCount=50, got %d", result.Indicators.Klines.PrimaryCount)
	}

	// Verify timeframes are preserved from strategy
	if len(result.Indicators.Klines.SelectedTimeframes) != 3 {
		t.Errorf("Expected 3 timeframes, got %d", len(result.Indicators.Klines.SelectedTimeframes))
	}

	// Verify primary timeframe is preserved
	if result.Indicators.Klines.PrimaryTimeframe != "5m" {
		t.Errorf("Expected PrimaryTimeframe=5m, got %s", result.Indicators.Klines.PrimaryTimeframe)
	}

	// Verify longer count is preserved
	if result.Indicators.Klines.LongerCount != 20 {
		t.Errorf("Expected LongerCount=20, got %d", result.Indicators.Klines.LongerCount)
	}

	// Verify leverage is overridden from backtest config
	if result.RiskControl.BTCETHMaxLeverage != 5 {
		t.Errorf("Expected BTCETHMaxLeverage=5, got %d", result.RiskControl.BTCETHMaxLeverage)
	}
}

func TestLoadStrategyForBacktest_LoadsAndAffectsValidate(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`
		CREATE TABLE strategies (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			name TEXT NOT NULL,
			is_default BOOLEAN NOT NULL DEFAULT 0,
			config TEXT NOT NULL
		)
	`); err != nil {
		t.Fatalf("create strategies table: %v", err)
	}

	strategyConfig := store.StrategyConfig{
		Language: "zh",
		Indicators: store.IndicatorConfig{
			Klines: store.KlineConfig{
				PrimaryTimeframe:     "30m",
				PrimaryCount:         100,
				EnableMultiTimeframe: true,
				SelectedTimeframes:   []string{"30m", "4h", "1d"},
			},
		},
	}
	cfgJSON, err := json.Marshal(strategyConfig)
	if err != nil {
		t.Fatalf("marshal strategy config: %v", err)
	}

	if _, err := db.Exec(`INSERT INTO strategies (id, user_id, name, is_default, config) VALUES (?, ?, ?, ?, ?)`,
		"strategy-1", "default", "4H策略1.2", false, string(cfgJSON),
	); err != nil {
		t.Fatalf("insert strategy: %v", err)
	}

	UseDatabaseWithType(db, false)
	defer UseDatabase(nil)

	cfg := BacktestConfig{
		RunID:             "load-strategy",
		UserID:            "default",
		StrategyID:        "strategy-1",
		Symbols:           []string{"BTCUSDT"},
		Timeframes:        []string{"4h"},
		DecisionTimeframe: "4h",
		StartTS:           1704067200,
		EndTS:             1704153600,
		InitialBalance:    1000,
	}

	if err := loadStrategyForBacktest(&cfg); err != nil {
		t.Fatalf("load strategy: %v", err)
	}
	if cfg.loadedStrategy == nil {
		t.Fatalf("loaded strategy is nil")
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}

	has30m := false
	for _, tf := range cfg.Timeframes {
		if tf == "30m" {
			has30m = true
			break
		}
	}
	if !has30m {
		t.Fatalf("expected cfg.Timeframes to include 30m, got %v", cfg.Timeframes)
	}
	if cfg.DecisionTimeframe != "30m" {
		t.Fatalf("expected cfg.DecisionTimeframe=30m, got %s", cfg.DecisionTimeframe)
	}
}

func TestToStrategyConfig_FallbackWithoutStrategy(t *testing.T) {
	// Create backtest config without strategy
	cfg := BacktestConfig{
		RunID:             "test_run",
		UserID:            "test_user",
		Symbols:           []string{"BTCUSDT", "ETHUSDT"},
		Timeframes:        []string{"5m", "15m"},
		DecisionTimeframe: "5m",
		StartTS:           1000000,
		EndTS:             2000000,
		InitialBalance:    1000,
		Leverage: LeverageConfig{
			BTCETHLeverage:  5,
			AltcoinLeverage: 3,
		},
	}

	// Convert to strategy config (should use fallback logic)
	result := cfg.ToStrategyConfig()

	// Verify default kline count is used
	if result.Indicators.Klines.PrimaryCount != 30 {
		t.Errorf("Expected default PrimaryCount=30, got %d", result.Indicators.Klines.PrimaryCount)
	}

	// Verify timeframes from backtest config
	if len(result.Indicators.Klines.SelectedTimeframes) != 2 {
		t.Errorf("Expected 2 timeframes, got %d", len(result.Indicators.Klines.SelectedTimeframes))
	}

	// Verify primary timeframe from backtest config
	if result.Indicators.Klines.PrimaryTimeframe != "5m" {
		t.Errorf("Expected PrimaryTimeframe=5m, got %s", result.Indicators.Klines.PrimaryTimeframe)
	}
}
