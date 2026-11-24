package gate

import (
	"testing"

	"github.com/gate/gateapi-go/v7"
)

// 验证 Gate 合约+ticker 到 SymbolInfo 的映射与 in_delisting 过滤
func TestSymbolInfoFromContract(t *testing.T) {
	contract := gateapi.Contract{
		Name:         "BTC_USDT",
		LeverageMax:  "100",
		OrderSizeMin: "1",
		InDelisting:  false,
	}
	ticker := gateapi.FuturesTicker{
		Contract:         "BTC_USDT",
		Last:             "65000.5",
		MarkPrice:        "65001.2",
		Volume24hSettle:  "12345678.9",
		ChangePercentage: "-7.45",
	}

	info, ok := SymbolInfoFromContract(contract, ticker, true)
	if !ok {
		t.Fatalf("normal contract should be included")
	}
	if info.Symbol != "BTC_USDT" || info.Display != "BTC_USDT" {
		t.Errorf("symbol/display = %s/%s, want BTC_USDT", info.Symbol, info.Display)
	}
	if info.Name != "BTC" {
		t.Errorf("name = %s, want BTC", info.Name)
	}
	if info.Category != "crypto" || info.Exchange != "gate" {
		t.Errorf("category/exchange = %s/%s, want crypto/gate", info.Category, info.Exchange)
	}
	if info.MarkPrice != 65000.5 {
		t.Errorf("mark_price = %f, want last price 65000.5", info.MarkPrice)
	}
	if info.Volume24h != 12345678.9 {
		t.Errorf("volume_24h = %f, want 12345678.9", info.Volume24h)
	}
	if info.Change24hPct != -7.45 {
		t.Errorf("change_24h_pct = %f, want -7.45", info.Change24hPct)
	}
	if info.MaxLeverage != 100 {
		t.Errorf("maxLeverage = %d, want 100", info.MaxLeverage)
	}
	if info.SzDecimals != 1 {
		t.Errorf("sz_decimals = %d, want order_size_min 1", info.SzDecimals)
	}
}

func TestSymbolInfoFromContractInDelisting(t *testing.T) {
	contract := gateapi.Contract{
		Name:        "DEAD_USDT",
		InDelisting: true,
	}
	if _, ok := SymbolInfoFromContract(contract, gateapi.FuturesTicker{}, true); ok {
		t.Fatalf("in_delisting contract must be filtered out")
	}
}

func TestSymbolInfoFromContractMarkPriceFallback(t *testing.T) {
	contract := gateapi.Contract{Name: "ETH_USDT", LeverageMax: "50", OrderSizeMin: "1"}
	ticker := gateapi.FuturesTicker{MarkPrice: "3000.25"}

	info, ok := SymbolInfoFromContract(contract, ticker, true)
	if !ok {
		t.Fatalf("contract should be included")
	}
	if info.MarkPrice != 3000.25 {
		t.Errorf("mark_price = %f, want mark fallback 3000.25", info.MarkPrice)
	}
}

func TestBaseSymbol(t *testing.T) {
	cases := map[string]string{
		"BTC_USDT":      "BTC",
		"1000PEPE_USDT": "1000PEPE",
		"NOSEP":         "NOSEP",
	}
	for contract, want := range cases {
		if got := baseSymbol(contract); got != want {
			t.Errorf("baseSymbol(%q) = %q, want %q", contract, got, want)
		}
	}
}
