package api

import "testing"

func TestIsSupportedTraderSymbol(t *testing.T) {
	tests := []struct {
		name   string
		symbol string
		want   bool
	}{
		{name: "standard USDT perp", symbol: "BTCUSDT", want: true},
		{name: "USDT perp lowercase", symbol: "ethusdt", want: true},
		{name: "gate style USDT perp", symbol: "BTC_USDT", want: true},
		{name: "empty slot ignored", symbol: "  ", want: true},
		{name: "bare base symbol rejected", symbol: "SMSN", want: false},
		{name: "non-USDT quote rejected", symbol: "BTCUSD", want: false},
		{name: "USDC pair rejected", symbol: "SMSN-USDC", want: false},
		{name: "xyz prefix rejected", symbol: "xyz:SMSN", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSupportedTraderSymbol(tt.symbol); got != tt.want {
				t.Fatalf("isSupportedTraderSymbol(%q) = %v, want %v", tt.symbol, got, tt.want)
			}
		})
	}
}
