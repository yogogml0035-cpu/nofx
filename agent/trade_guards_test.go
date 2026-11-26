package agent

// 助手开仓的风险说明与 Gate 优先符号路由的接线测试（对抗审查视角 D）。

import (
	"errors"
	"strings"
	"testing"

	"nofx/market"
)

// 有加密 trader 时按 Gate 合约清单判定；清单不可用时回落启发式；只有股票 trader 时不查 Gate
func TestResolveStockSymbolUsesGateListWithCryptoTraders(t *testing.T) {
	previousExchanges, previousTickers := configuredTraderExchanges, assistantListTickers
	t.Cleanup(func() { configuredTraderExchanges, assistantListTickers = previousExchanges, previousTickers })

	listCalls := 0
	configuredTraderExchanges = func(*Agent) []string { return []string{"gate"} }
	assistantListTickers = func() ([]market.GateTicker, error) {
		listCalls++
		return []market.GateTicker{{Symbol: "HYPEUSDT"}, {Symbol: "BTCUSDT"}}, nil
	}
	a := &Agent{}
	if a.resolveStockSymbol("HYPE") {
		t.Fatal("HYPE is a listed Gate contract even though it is missing from the hardcoded list")
	}
	if !a.resolveStockSymbol("AAPL") {
		t.Fatal("AAPL is not a Gate contract and must stay a stock")
	}
	if listCalls != 2 {
		t.Fatalf("gate ticker list queried %d times, want 2", listCalls)
	}

	assistantListTickers = func() ([]market.GateTicker, error) { return nil, errors.New("gate down") }
	if !a.resolveStockSymbol("HYPE") {
		t.Fatal("when the Gate list is unavailable the heuristic must apply (HYPE looks like a stock)")
	}

	configuredTraderExchanges = func(*Agent) []string { return []string{"alpaca"} }
	listCalls = 0
	assistantListTickers = func() ([]market.GateTicker, error) { listCalls++; return nil, nil }
	if !a.resolveStockSymbol("HYPE") {
		t.Fatal("stock-only setups must not route through Gate")
	}
	if listCalls != 0 {
		t.Fatal("the Gate ticker list must not be queried when no crypto trader is configured")
	}
}

// 助手开仓确认必须带风险说明（不带止损止盈、不由 trader 管理）；平仓不带
func TestFormatTradeConfirmationRiskNotice(t *testing.T) {
	for _, tc := range []struct {
		lang     string
		openWord string
	}{
		{"zh", "不附带止损止盈"},
		{"en", "no stop loss or take profit"},
	} {
		open := &TradeAction{ID: "trade_1", Action: "open_long", Symbol: "BTCUSDT", Quantity: 0.01, EstimatedNotional: 500}
		msg := formatTradeConfirmation(open, tc.lang)
		if !strings.Contains(msg, tc.openWord) {
			t.Fatalf("%s open confirmation must carry the risk notice: %s", tc.lang, msg)
		}
		if !strings.Contains(msg, "确认 trade_1") && !strings.Contains(msg, "confirm trade_1") {
			t.Fatalf("%s confirmation must still carry the confirmation command: %s", tc.lang, msg)
		}
		closeTrade := &TradeAction{ID: "trade_2", Action: "close_long", Symbol: "BTCUSDT"}
		if msg := formatTradeConfirmation(closeTrade, tc.lang); strings.Contains(msg, tc.openWord) {
			t.Fatalf("%s close confirmation must not carry the open risk notice: %s", tc.lang, msg)
		}
	}
}
