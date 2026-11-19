package kernel

import (
	"strings"
	"testing"

	"nofx/store"
)

func zhConfig() store.StrategyConfig {
	cfg := store.GetDefaultStrategyConfig("zh")
	cfg.MarketContext = store.MarketContextConfig{
		Overview:      true,
		FearGreed:     true,
		NewsSentiment: true,
	}
	return cfg
}

func enConfig() store.StrategyConfig {
	cfg := store.GetDefaultStrategyConfig("en")
	cfg.MarketContext = store.MarketContextConfig{
		Overview:      true,
		FearGreed:     true,
		NewsSentiment: true,
	}
	return cfg
}

func fullSnapshot() *MarketEnvSnapshot {
	change := -5.0
	return &MarketEnvSnapshot{
		TotalMarketCap:     1_280_000_000_000,
		MarketCapChange24h: 1.3,
		TotalVolume24h:     220_000_000_000,
		BTCPrice:           63747,
		BTCDominance:       58.9,
		ETHDominance:       10.1,
		AltcoinSeasonIndex: 57,
		FearGreedValue:     12,
		FearGreedLabel:     "Extreme Fear",
		FearGreedChange24h: &change,
		NewsTitles:         []string{"BTC ETF inflow hits record", "Exchange reserves dropping"},
		OverallSentiment:   -0.29,
		SentimentLabel:     "negative",
	}
}

// 未勾选 / nil 快照：不注入任何内容
func TestWriteMarketEnvDisabledOrNil(t *testing.T) {
	cfg := zhConfig()
	cfg.MarketContext = store.MarketContextConfig{}
	var sb strings.Builder
	WriteMarketEnv(&sb, fullSnapshot(), cfg, LangChinese)
	if sb.Len() != 0 {
		t.Fatalf("disabled config must not inject, got: %s", sb.String())
	}

	var sb2 strings.Builder
	WriteMarketEnv(&sb2, nil, zhConfig(), LangChinese)
	if sb2.Len() != 0 {
		t.Fatalf("nil snapshot must not inject, got: %s", sb2.String())
	}

	grid := zhConfig()
	grid.StrategyType = "grid_trading"
	var sb3 strings.Builder
	WriteMarketEnv(&sb3, fullSnapshot(), grid, LangChinese)
	if sb3.Len() != 0 {
		t.Fatalf("grid strategy must not inject, got: %s", sb3.String())
	}
}

// 中文注入：三个子段齐全、数值与快照一致
func TestWriteMarketEnvZH(t *testing.T) {
	var sb strings.Builder
	WriteMarketEnv(&sb, fullSnapshot(), zhConfig(), LangChinese)
	out := sb.String()

	for _, want := range []string{
		"## 市场环境上下文",
		"总市值 $1280000000000",
		"BTC $63747",
		"恐惧贪婪: 12 Extreme Fear (-5 24h)",
		"情绪分 -0.29 negative",
		"BTC ETF inflow hits record",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("zh output missing %q, got: %s", want, out)
		}
	}
}

// 英文注入
func TestWriteMarketEnvEN(t *testing.T) {
	var sb strings.Builder
	WriteMarketEnv(&sb, fullSnapshot(), enConfig(), LangEnglish)
	out := sb.String()

	for _, want := range []string{
		"## Market Environment Context",
		"Market Overview: cap $1280000000000",
		"Fear & Greed: 12 Extreme Fear (-5 24h)",
		"sentiment -0.29 negative",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("en output missing %q, got: %s", want, out)
		}
	}
}

// 冷启动：FearGreedChange24h 为 nil 时不输出 24h 变化（仅检查恐惧贪婪行）
func TestWriteMarketEnvColdStartOmitsChange(t *testing.T) {
	snap := fullSnapshot()
	snap.FearGreedChange24h = nil
	var sb strings.Builder
	WriteMarketEnv(&sb, snap, enConfig(), LangEnglish)
	out := sb.String()
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "Fear & Greed") && strings.Contains(line, "24h") {
			t.Fatalf("cold start must omit fear&greed 24h change, got line: %s", line)
		}
	}
}

// Unavailable：勾选但取数失败的源输出占位行
func TestWriteMarketEnvUnavailable(t *testing.T) {
	snap := fullSnapshot()
	snap.Unavailable = map[string]bool{"overview": true, "fear_greed": true, "news_sentiment": true}
	var sb strings.Builder
	WriteMarketEnv(&sb, snap, zhConfig(), LangChinese)
	out := sb.String()
	if strings.Count(out, "[数据暂不可用]") != 3 {
		t.Fatalf("expected 3 unavailable markers, got: %s", out)
	}
}

// 超限截断：先丢新闻条目，行尾标注 [truncated]
func TestWriteMarketEnvTruncatesNews(t *testing.T) {
	snap := fullSnapshot()
	snap.NewsTitles = make([]string, 0, 100)
	for i := 0; i < 100; i++ {
		snap.NewsTitles = append(snap.NewsTitles, "Very long news headline repeated to blow past the token limit quickly")
	}
	var sb strings.Builder
	WriteMarketEnv(&sb, snap, enConfig(), LangEnglish)
	out := sb.String()

	if !strings.Contains(out, "[truncated]") {
		t.Fatalf("expected [truncated] marker, got: %s", out)
	}
	// 截断后应在软上限附近（600 token × 4 chars ≈ 2400 chars，留余量）
	if len([]rune(out)) > 600*4+200 {
		t.Fatalf("output exceeds soft limit: %d runes", len([]rune(out)))
	}
}

// 决策 prompt 集成：勾选策略 BuildUserPrompt 含注入段；未勾选不含
func TestBuildUserPromptIncludesMarketEnvWhenEnabled(t *testing.T) {
	cfg := enConfig()
	engine := NewStrategyEngine(&cfg)
	snap := fullSnapshot()
	snap.NewsTitles = []string{"BTC ETF inflow hits record"}
	ctx := &Context{
		MarketEnv:      snap,
		CandidateCoins: []CandidateCoin{{Symbol: "BTCUSDT", Sources: []string{"static"}}},
	}

	out := engine.BuildUserPrompt(ctx)
	if !strings.Contains(out, "## Market Environment Context") {
		t.Fatalf("prompt must contain market env section")
	}
	if !strings.Contains(out, "BTC ETF inflow hits record") {
		t.Fatalf("prompt must contain market env news title")
	}

	// 未勾选：同快照也不注入
	cfg.MarketContext = store.MarketContextConfig{}
	engine2 := NewStrategyEngine(&cfg)
	out2 := engine2.BuildUserPrompt(ctx)
	if strings.Contains(out2, "## Market Environment Context") {
		t.Fatalf("prompt must NOT contain market env section when disabled")
	}
}
