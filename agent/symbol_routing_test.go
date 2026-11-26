package agent

import (
	"testing"

	"nofx/market"
)

// 助手的价格与下单路由：Gate 优先。基础币种是已上架的 Gate USDT 永续时按加密货币处理，
// 即使它不在硬编码清单里（HYPE、ZEC 等此前被字母启发式误判为股票）。
func TestGateContractInList(t *testing.T) {
	tickers := []market.GateTicker{{Symbol: "BTCUSDT"}, {Symbol: "ZECUSDT"}, {Symbol: "HYPEUSDT"}}
	for _, tc := range []struct {
		base string
		want bool
	}{
		{"ZEC", true}, {"hype", true}, {"BTC_USDT", true}, {"BTC-USDT", true},
		{"AAPL", false}, {"", false},
	} {
		if got := gateContractInList(tc.base, tickers); got != tc.want {
			t.Fatalf("gateContractInList(%q) = %v, want %v", tc.base, got, tc.want)
		}
	}
	if gateContractInList("BTC", nil) {
		t.Fatal("empty ticker list must not match")
	}
}

// 没有 trader manager（股票专属环境或单测）时不查 Gate，沿用字母启发式。
func TestResolveStockSymbolWithoutManager(t *testing.T) {
	a := &Agent{}
	for _, tc := range []struct {
		symbol string
		want   bool
	}{
		{"BTC", false}, {"BTCUSDT", false}, {"BTC_USDT", false},
		{"AAPL", true}, {"ZEC", true},
	} {
		if got := a.resolveStockSymbol(tc.symbol); got != tc.want {
			t.Fatalf("resolveStockSymbol(%q) = %v, want %v", tc.symbol, got, tc.want)
		}
	}
}
