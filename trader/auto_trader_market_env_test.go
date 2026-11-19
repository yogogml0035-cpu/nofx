package trader

import (
	"context"
	"strings"
	"testing"

	"nofx/kernel"
	"nofx/store"
)

// 断链场景：gatemcp 指向不可达地址时，快照带 Unavailable 标记且决策不中断
// （prompt 注入端会输出占位行；本测试验证组装层不 panic、三源均有标记）
func TestBuildMarketEnvSnapshotUnavailable(t *testing.T) {
	// Shared() 单例在首次调用时读环境变量——必须在任何取数前设置
	t.Setenv("GATEMCP_ENDPOINT", "http://127.0.0.1:9")
	at := &AutoTrader{name: "probe-trader"}
	cfg := store.MarketContextConfig{Overview: true, FearGreed: true, NewsSentiment: true}

	// 127.0.0.1:9 不可达（discard 端口）
	ctx, cancel := context.WithTimeout(context.Background(), 10_000_000_000) // 10s
	defer cancel()

	snap := at.fetchMarketEnvSnapshot(ctx, cfg)
	if snap == nil {
		t.Fatal("snapshot must never be nil")
	}
	if !snap.Unavailable["overview"] || !snap.Unavailable["fear_greed"] || !snap.Unavailable["news_sentiment"] {
		t.Fatalf("all three sources must be marked unavailable on dead endpoint, got: %+v", snap.Unavailable)
	}
}

// 注入端对 Unavailable 快照输出占位行、决策链不中断（与 kernel 注入语义闭环）
func TestUnavailableSnapshotRendersPlaceholder(t *testing.T) {
	t.Setenv("GATEMCP_ENDPOINT", "http://127.0.0.1:9")
	at := &AutoTrader{name: "probe-trader"}
	cfg := store.MarketContextConfig{Overview: true, FearGreed: true, NewsSentiment: true}
	ctx, cancel := context.WithTimeout(context.Background(), 10_000_000_000)
	defer cancel()

	snap := at.fetchMarketEnvSnapshot(ctx, cfg)

	engine := kernel.NewStrategyEngine(&store.StrategyConfig{
		StrategyType:  "ai_trading",
		Language:      "zh",
		MarketContext: cfg,
	})
	decisionCtx := &kernel.Context{MarketEnv: snap}
	out := engine.BuildUserPrompt(decisionCtx)

	if !strings.Contains(out, "[数据暂不可用]") {
		t.Fatalf("prompt must contain unavailable placeholder, got: %s", out)
	}
	if !strings.Contains(out, "## 市场环境上下文") {
		t.Fatalf("prompt must still contain market env heading, got: %s", out)
	}
}
