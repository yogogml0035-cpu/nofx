package trader

import (
	"strings"
	"testing"

	"nofx/kernel"
)

func universeTestContext() *kernel.Context {
	return &kernel.Context{
		CandidateCoins: []kernel.CandidateCoin{
			{Symbol: "BTCUSDT"},
			{Symbol: "ETHUSDT"},
		},
	}
}

func TestFilterDecisions_PortfolioNoOpsBypassUniverse(t *testing.T) {
	at := &AutoTrader{name: "universe-test"}
	ctx := universeTestContext()

	decisions := []kernel.Decision{
		{Symbol: "ALL", Action: "wait"},
		{Symbol: "", Action: "hold"},
	}

	got := at.filterDecisionsToStrategyUniverse(decisions, ctx)
	if len(got) != len(decisions) {
		t.Fatalf("wait/hold must never be dropped, got %d of %d", len(got), len(decisions))
	}
	for _, d := range got {
		if d.Symbol == "ALLUSDT" {
			t.Fatalf("ALL sentinel must not be normalized: %+v", d)
		}
	}
}

func TestFilterDecisions_OpenOutsideUniverseIsBlocked(t *testing.T) {
	at := &AutoTrader{name: "universe-test"}
	ctx := universeTestContext()

	got := at.filterDecisionsToStrategyUniverse([]kernel.Decision{
		{Symbol: "SOLUSDT", Action: "open_long"},
	}, ctx)
	if len(got) != 0 {
		t.Fatalf("out-of-universe open must be dropped, got %+v", got)
	}
}

func TestFilterDecisions_MissingSymbolIsDroppedForConcreteActions(t *testing.T) {
	at := &AutoTrader{name: "universe-test"}
	ctx := universeTestContext()
	// Even a pathological "USDT" candidate must not make an empty symbol pass:
	// the old check normalized "" to "USDT" and could match it.
	ctx.CandidateCoins = append(ctx.CandidateCoins, kernel.CandidateCoin{Symbol: "USDT"})

	for _, action := range []string{"open_long", "close_long", "update_stop_loss"} {
		var dropped []string
		got := at.filterDecisionsToStrategyUniverseWithLog([]kernel.Decision{
			{Symbol: "  ", Action: action},
		}, ctx, func(line string) { dropped = append(dropped, line) })
		if len(got) != 0 {
			t.Fatalf("%s with missing symbol must be dropped, got %+v", action, got)
		}
		if len(dropped) != 1 || !strings.Contains(dropped[0], "missing symbol") {
			t.Fatalf("%s must report the missing symbol, got %v", action, dropped)
		}
	}
}

func TestFilterDecisions_CanonicalizesSymbolVariants(t *testing.T) {
	at := &AutoTrader{name: "universe-test"}
	ctx := universeTestContext()
	ctx.Positions = []kernel.PositionInfo{{Symbol: "DOGEUSDT", Side: "LONG"}}

	tests := []struct {
		symbol string
		action string
		want   string
	}{
		{"BTC/USDT", "open_long", "BTCUSDT"},
		{"BTC-USDT", "open_long", "BTCUSDT"},
		{"BTC_USDT", "open_long", "BTCUSDT"},
		{"BTC-USDT-SWAP", "open_long", "BTCUSDT"},
		{"btc", "open_long", "BTCUSDT"},
		{"DOGE", "close_long", "DOGEUSDT"},
		{"DOGE/USDT", "close_long", "DOGEUSDT"},
	}

	for _, tt := range tests {
		got := at.filterDecisionsToStrategyUniverse([]kernel.Decision{
			{Symbol: tt.symbol, Action: tt.action},
		}, ctx)
		if len(got) != 1 {
			t.Fatalf("%s %s must survive, got %+v", tt.action, tt.symbol, got)
		}
		if got[0].Symbol != tt.want {
			t.Fatalf("%s %s must canonicalize to %q, got %q", tt.action, tt.symbol, tt.want, got[0].Symbol)
		}
	}
}

func TestFilterDecisions_DoesNotRewriteForeignQuote(t *testing.T) {
	at := &AutoTrader{name: "universe-test"}
	ctx := universeTestContext()

	for _, symbol := range []string{"BTC/USDC", "BTC-USDC", "BTCUSDC", "BTC/USD"} {
		got := at.filterDecisionsToStrategyUniverse([]kernel.Decision{
			{Symbol: symbol, Action: "open_long"},
		}, ctx)
		if len(got) != 0 {
			t.Fatalf("%s must not be rewritten to a USDT pair, got %+v", symbol, got)
		}
	}
}

func TestFilterDecisions_PositionSymbolIsAllowed(t *testing.T) {
	at := &AutoTrader{name: "universe-test"}
	ctx := &kernel.Context{
		CandidateCoins: []kernel.CandidateCoin{{Symbol: "BTCUSDT"}},
		Positions:      []kernel.PositionInfo{{Symbol: "DOGEUSDT", Side: "LONG"}},
	}

	got := at.filterDecisionsToStrategyUniverse([]kernel.Decision{
		{Symbol: "DOGE_USDT", Action: "close_long"},
	}, ctx)
	if len(got) != 1 {
		t.Fatalf("position symbol must stay manageable, got %+v", got)
	}
	if got[0].Symbol != "DOGEUSDT" {
		t.Fatalf("position variant must canonicalize to the position symbol, got %q", got[0].Symbol)
	}
}
