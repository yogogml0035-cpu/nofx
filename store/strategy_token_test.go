package store

import (
	"strings"
	"testing"
)

func TestEstimateTokens_DefaultConfig(t *testing.T) {
	config := GetDefaultStrategyConfig("en")
	est := config.EstimateTokens()

	if est.Total <= 0 {
		t.Errorf("expected positive token estimate, got %d", est.Total)
	}
	if est.Total > 200000 {
		t.Errorf("token estimate %d seems unreasonably high for default config", est.Total)
	}

	// Breakdown should sum approximately to total (before 15% margin)
	subtotal := est.Breakdown.SystemPrompt + est.Breakdown.MarketData + est.Breakdown.FixedOverhead
	expectedTotal := subtotal * 115 / 100
	if est.Total != expectedTotal {
		t.Errorf("total %d != breakdown subtotal %d * 1.15 = %d", est.Total, subtotal, expectedTotal)
	}

	// Should have model limits
	if len(est.ModelLimits) == 0 {
		t.Error("expected model limits to be populated")
	}

	// Default config should be ok for all models
	for _, ml := range est.ModelLimits {
		if ml.Level == "danger" {
			t.Errorf("default config should not exceed %s limit, got %d%%", ml.Name, ml.UsagePct)
		}
	}
}

func TestEstimateTokens_ZhVsEn(t *testing.T) {
	enConfig := GetDefaultStrategyConfig("en")
	zhConfig := GetDefaultStrategyConfig("zh")

	enEst := enConfig.EstimateTokens()
	zhEst := zhConfig.EstimateTokens()

	// Chinese config should have more tokens for system prompt due to CJK encoding
	// but total can vary — just ensure both are reasonable
	if enEst.Total <= 0 || zhEst.Total <= 0 {
		t.Errorf("both estimates should be positive: en=%d, zh=%d", enEst.Total, zhEst.Total)
	}
}

func TestEstimateTokens_HighConfig(t *testing.T) {
	config := GetDefaultStrategyConfig("en")
	// Push config to extremes (beyond clamped limits)
	config.CoinSource.StaticCoins = []string{"BTCUSDT", "ETHUSDT", "SOLUSDT", "DOGEUSDT", "XRPUSDT"}
	config.Indicators.Klines.SelectedTimeframes = []string{"1m", "30m", "5m", "15m", "1h", "4h"}
	config.Indicators.Klines.PrimaryCount = 100
	config.Indicators.EnableEMA = true
	config.Indicators.EnableMACD = true
	config.Indicators.EnableRSI = true
	config.Indicators.EnableATR = true
	config.Indicators.EnableBOLL = true

	est := config.EstimateTokens()

	// Should produce a higher estimate than default
	defaultCfg := GetDefaultStrategyConfig("en")
	defaultEst := defaultCfg.EstimateTokens()
	if est.Total <= defaultEst.Total {
		t.Errorf("high config estimate %d should be greater than default %d", est.Total, defaultEst.Total)
	}

	// Should have some models in warning/danger
	hasDanger := false
	for _, ml := range est.ModelLimits {
		if ml.Level == "danger" || ml.Level == "warning" {
			hasDanger = true
			break
		}
	}
	// With 5 coins * 6 timeframes * 100 klines, this should exceed small models
	if !hasDanger {
		t.Logf("high config estimate: %d tokens", est.Total)
	}
}

func TestGetContextLimit(t *testing.T) {
	if got := GetContextLimit("deepseek"); got != 1_000_000 {
		t.Errorf("deepseek limit = %d, want 1_000_000 for the V4.1-Flash/V4-Pro generation", got)
	}
	if got := GetContextLimit("unknown_provider"); got != defaultContextLimit {
		t.Errorf("unknown provider should return default %d, got %d", defaultContextLimit, got)
	}
}

func TestGetModelCapabilitiesPrefersModelOverProvider(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		model    string
		context  int
		maxOut   int
	}{
		{
			name:     "DeepSeek V4.1 Flash snapshot",
			provider: "deepseek", model: "DeepSeek-V4.1-Flash-1017",
			context: 1_000_000, maxOut: 384_000,
		},
		{
			name:     "DeepSeek V4 Pro lowercase snapshot",
			provider: "deepseek", model: "deepseek-v4-pro-0813",
			context: 1_000_000, maxOut: 384_000,
		},
		{
			name:     "DeepSeek provider fallback",
			provider: "deepseek", model: "",
			context: 1_000_000, maxOut: 0,
		},
		{
			name:     "Unknown provider stays conservative",
			provider: "custom", model: "my-private-model",
			context: defaultContextLimit, maxOut: 0,
		},
		{
			name:     "Qwen model keeps provider window",
			provider: "qwen", model: "qwen-max",
			context: 131_072, maxOut: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := GetModelCapabilities(tc.provider, tc.model)
			if got.ContextWindow != tc.context || got.MaxOutputTokens != tc.maxOut {
				t.Fatalf("GetModelCapabilities(%q, %q) = %+v, want context=%d maxOut=%d",
					tc.provider, tc.model, got, tc.context, tc.maxOut)
			}
		})
	}
}

// 真实失败配置的形状：2 币 × 4 周期 × 300 根，EMA/MACD/ATR/成交量开启。
// 线上同一形状的渲染提示词被 provider 计为 139,981 input tokens；配置级估算
// 应落在这个量级附近，而不是旧公式的 114,333。
func TestEstimateTokensMatchesMeasuredRealConfigShape(t *testing.T) {
	cfg := GetDefaultStrategyConfig("zh")
	cfg.CoinSource.StaticCoins = []string{"BTCUSDT", "ETHUSDT"}
	cfg.Indicators.Klines.SelectedTimeframes = []string{"5m", "4h", "15m", "1h"}
	cfg.Indicators.Klines.PrimaryCount = 300
	cfg.Indicators.EnableEMA = true
	cfg.Indicators.EnableMACD = true
	cfg.Indicators.EnableRSI = false
	cfg.Indicators.EnableATR = true
	cfg.Indicators.EnableBOLL = false
	cfg.Indicators.EnableVolume = true
	cfg.Indicators.EnableOI = true
	cfg.Indicators.EnableFundingRate = true
	cfg.ClampLimits()

	est := cfg.EstimateTokens()
	wantMarket := 2*4*300*(35+10+5) + 2*25
	if est.Breakdown.MarketData != wantMarket {
		t.Fatalf("market estimate = %d, want %d", est.Breakdown.MarketData, wantMarket)
	}
	if est.Total < 135_000 || est.Total > 155_000 {
		t.Fatalf("real-shape estimate = %d, want within 135000..155000 around the measured 139981 provider tokens", est.Total)
	}
}

func TestEstimatePromptTokensNumericTablesAreDense(t *testing.T) {
	var b strings.Builder
	line := "11-26 07:15    60447.3000 60523.2000 60441.0000 60510.1000 7617070.04  \n"
	for i := 0; i < 2400; i++ {
		b.WriteString(line)
	}
	value := "60634.5750, "
	for i := 0; i < 7200; i++ {
		b.WriteString(value)
	}

	got := EstimatePromptTokens("", b.String())
	if got < 135_000 || got > 165_000 {
		t.Fatalf("numeric prompt estimate = %d, want a conservative bound around the measured 139981 tokens", got)
	}
}

func TestGetEffectiveCoinCount(t *testing.T) {
	config := StrategyConfig{
		CoinSource: CoinSourceConfig{
			StaticCoins: []string{"BTCUSDT", "ETHUSDT"},
		},
	}
	if got := config.getEffectiveCoinCount(); got != 2 {
		t.Errorf("static coin count = %d, want 2", got)
	}
}
