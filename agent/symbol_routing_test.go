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

// 最小开仓金额已删除：助手再传这个字段要明确报不支持，而不是静默忽略
func TestApplyStrategyConfigPatchRejectsRemovedMinPositionSize(t *testing.T) {
	cfg := store.GetDefaultStrategyConfig("zh")
	err := applyStrategyConfigPatch(&cfg, "min_position_size", "20")
	if err == nil || !strings.Contains(err.Error(), "unsupported strategy config field") {
		t.Fatalf("min_position_size must be rejected as an unsupported field, got: %v", err)
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
	// 字段不存在时不是「值不合法」：说明这个值没有被应用，而不是让用户换一个值
	got = prefixStrategyPatchError("zh", "unsupported strategy config field: min_position_size", "reply")
	if !strings.Contains(got, "策略配置里没有「min_position_size」这个字段") || strings.Contains(got, "请换一个值") || !strings.HasSuffix(got, "reply") {
		t.Fatalf("zh unsupported-field reply = %q", got)
	}
	got = prefixStrategyPatchError("en", "unsupported strategy config field: min_position_size", "reply")
	if !strings.Contains(got, `has no field "min_position_size"`) || strings.Contains(got, "another value") || !strings.HasSuffix(got, "reply") {
		t.Fatalf("en unsupported-field reply = %q", got)
	}
}
