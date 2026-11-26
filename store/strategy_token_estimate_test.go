package store

import "testing"

func TestStrategyClampWarningsCoversPrimaryCount(t *testing.T) {
	before := GetDefaultStrategyConfig("zh")
	before.Indicators.Klines.PrimaryCount = 5000
	after := before
	after.ClampLimits()

	warnings := StrategyClampWarnings(before, after, "zh")

	found := false
	for _, warning := range warnings {
		if warning == "K 线数量 已从 5000 调整为 2000" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected primary_count clamp warning, got %v", warnings)
	}
}

func TestEstimateTokensCountsPrimaryOutsideSelection(t *testing.T) {
	cfg := GetDefaultStrategyConfig("zh")
	cfg.CoinSource.StaticCoins = []string{"BTCUSDT"}
	cfg.Indicators.Klines.SelectedTimeframes = []string{"5m", "15m"}
	cfg.Indicators.Klines.PrimaryTimeframe = "1h" // 1h is fetched and rendered in addition to the selection

	if got := cfg.getEffectiveTimeframeCount(); got != 3 {
		t.Fatalf("getEffectiveTimeframeCount = %d, want 3", got)
	}
	outsideSelection := cfg.EstimateTokens().Total

	cfg.Indicators.Klines.SelectedTimeframes = []string{"5m", "15m", "1h"}
	if got := cfg.getEffectiveTimeframeCount(); got != 3 {
		t.Fatalf("getEffectiveTimeframeCount = %d, want 3", got)
	}
	if included := cfg.EstimateTokens().Total; included != outsideSelection {
		t.Fatalf("estimate with primary outside selection = %d, want %d (primary must count as an extra timeframe)", outsideSelection, included)
	}
}

func TestEstimateTokensSkipsExcludedCoins(t *testing.T) {
	cfg := GetDefaultStrategyConfig("zh")
	cfg.CoinSource.StaticCoins = []string{"BTCUSDT", "ETHUSDT", "SOLUSDT"}
	cfg.CoinSource.ExcludedCoins = []string{"SOLUSDT"}

	if got := cfg.getEffectiveCoinCount(); got != 2 {
		t.Fatalf("getEffectiveCoinCount = %d, want 2 after excluding SOLUSDT", got)
	}

	twoCoins := GetDefaultStrategyConfig("zh")
	twoCoins.CoinSource.StaticCoins = []string{"BTCUSDT", "ETHUSDT"}
	if got, want := cfg.EstimateTokens().Total, twoCoins.EstimateTokens().Total; got != want {
		t.Fatalf("estimate with an excluded coin = %d, want %d", got, want)
	}
}

func TestExceedsAllModelLimits(t *testing.T) {
	cfg := GetDefaultStrategyConfig("zh")
	cfg.CoinSource.StaticCoins = []string{
		"BTCUSDT", "ETHUSDT", "SOLUSDT", "DOGEUSDT", "XRPUSDT",
		"BNBUSDT", "ADAUSDT", "LTCUSDT", "TRXUSDT", "DOTUSDT",
	}
	cfg.Indicators.Klines.SelectedTimeframes = []string{"5m", "15m", "1h", "4h"}
	cfg.Indicators.Klines.PrimaryCount = 2000

	estimate, exceedsAll := cfg.ExceedsAllModelLimits()
	if !exceedsAll {
		t.Fatalf("expected 10 coins x 4 timeframes x 2000 bars to exceed every model, estimate=%d", estimate.Total)
	}
	if len(estimate.ExceededModelNames()) == 0 {
		t.Fatal("expected exceeded model names")
	}

	cfg.Indicators.Klines.PrimaryCount = MinKlineCount
	if _, exceedsAll := cfg.ExceedsAllModelLimits(); exceedsAll {
		t.Fatal("default-sized config must not exceed every model")
	}
}
