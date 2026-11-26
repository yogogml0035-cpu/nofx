package agent

import (
	"strings"
	"testing"

	"nofx/market"
	"nofx/store"
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

// 最小开仓金额与服务端同口径：低于 10 USDT 或高于 1000 USDT 直接拒绝，而不是保存后被静默夹回
func TestApplyStrategyConfigPatchMinPositionSizeBounds(t *testing.T) {
	cfg := store.GetDefaultStrategyConfig("zh")
	for _, tc := range []struct {
		value string
		want  string
	}{
		{"5", "不能低于 10"},
		{"0", "不能低于 10"},
		{"-3", "不能低于 10"},
		{"5000", "不能高于 1000"},
	} {
		err := applyStrategyConfigPatch(&cfg, "min_position_size", tc.value)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("min_position_size %q: error = %v, want it to contain %q", tc.value, err, tc.want)
		}
	}
	if err := applyStrategyConfigPatch(&cfg, "min_position_size", "20"); err != nil {
		t.Fatalf("min_position_size 20 must be accepted: %v", err)
	}
	if cfg.RiskControl.MinPositionSize != 20 {
		t.Fatalf("min_position_size = %v, want 20", cfg.RiskControl.MinPositionSize)
	}
}

// 被拒绝的值要带回给用户：追问前说明原因
func TestPrefixStrategyPatchError(t *testing.T) {
	if got := prefixStrategyPatchError("zh", "", "reply"); got != "reply" {
		t.Fatalf("no error must keep the reply, got %q", got)
	}
	got := prefixStrategyPatchError("zh", `主周期 "3m" 不在支持范围`, "reply")
	if !strings.Contains(got, "上一个值没有被接受") || !strings.Contains(got, "3m") || !strings.HasSuffix(got, "reply") {
		t.Fatalf("zh reply = %q", got)
	}
	got = prefixStrategyPatchError("en", "invalid", "reply")
	if !strings.Contains(got, "was not accepted") || !strings.HasSuffix(got, "reply") {
		t.Fatalf("en reply = %q", got)
	}
}
