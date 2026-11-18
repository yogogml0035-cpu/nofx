package api

import "testing"

// D7：/api/symbols 与 /api/klines 无参时默认数据源均为 Gate
func TestDefaultMarketDataSourceIsGate(t *testing.T) {
	if defaultExchange != "gate" {
		t.Fatalf("default exchange should be gate, got %q", defaultExchange)
	}
}
