package store

import "testing"

// 验证 SourceType 枚举收窄后 default 重定向 static（旧 hyper_* 值一律归一为 static）
func TestNormalizeProductSchemaRedirectsLegacySourceTypesToStatic(t *testing.T) {
	for _, legacy := range []string{"hyper_rank", "hyper_all", "hyper_main", "hyperrank", "unknown_source"} {
		cfg := StrategyConfig{CoinSource: CoinSourceConfig{SourceType: legacy, StaticCoins: []string{"BTCUSDT"}}}
		cfg.NormalizeProductSchema()
		if cfg.CoinSource.SourceType != "static" {
			t.Errorf("source_type %q should be redirected to static, got %q", legacy, cfg.CoinSource.SourceType)
		}
		if cfg.CoinSource.UseAI500 || cfg.CoinSource.UseOITop || cfg.CoinSource.UseOILow {
			t.Errorf("source_type %q redirect must clear all programmatic source flags", legacy)
		}
	}
}

func TestNormalizeProductSchemaKeepsValidSourceTypes(t *testing.T) {
	for _, valid := range []string{"static", "ai500", "oi_top", "oi_low"} {
		cfg := StrategyConfig{CoinSource: CoinSourceConfig{SourceType: valid}}
		cfg.NormalizeProductSchema()
		if cfg.CoinSource.SourceType != valid {
			t.Errorf("valid source_type %q must be preserved, got %q", valid, cfg.CoinSource.SourceType)
		}
	}
}

func TestInferCoinSourceTypeDefaultsToStatic(t *testing.T) {
	if got := inferCoinSourceType(CoinSourceConfig{}); got != "static" {
		t.Errorf("empty coin source should infer static, got %q", got)
	}
	if got := inferCoinSourceType(CoinSourceConfig{StaticCoins: []string{"ETHUSDT"}}); got != "static" {
		t.Errorf("static coins should infer static, got %q", got)
	}
}

// 验证新建策略的默认币源为 static（手工清单选币）
func TestGetDefaultStrategyConfigUsesStaticSource(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		cfg := GetDefaultStrategyConfig(lang)
		if cfg.CoinSource.SourceType != "static" {
			t.Errorf("default source type for lang %s should be static, got %q", lang, cfg.CoinSource.SourceType)
		}
	}
}
