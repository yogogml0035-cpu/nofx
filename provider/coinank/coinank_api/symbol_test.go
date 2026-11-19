package coinank_api

import (
	"testing"

	"nofx/provider/coinank/coinank_enum"
)

// 各交易所合约形态归一：幂等、三种输入形态全覆盖、未知后缀 fast-fail 原样返回
func TestNormalizePairSymbol(t *testing.T) {
	cases := []struct {
		name     string
		symbol   string
		exchange coinank_enum.Exchange
		want     string
	}{
		{"gate 下划线输入保持", "BTC_USDT", coinank_enum.Gate, "BTC_USDT"},
		{"gate 紧凑输入转下划线", "BTCUSDT", coinank_enum.Gate, "BTC_USDT"},
		{"gate 小写输入转大写下划线", "btc_usdt", coinank_enum.Gate, "BTC_USDT"},
		{"gate OKX 形态输入转下划线", "BTC-USDT", coinank_enum.Gate, "BTC_USDT"},
		{"gate USDC 报价", "ETHUSDC", coinank_enum.Gate, "ETH_USDC"},
		{"gate 未知后缀原样返回", "SP500", coinank_enum.Gate, "SP500"},
		{"okx 紧凑输入转 SWAP", "BTCUSDT", coinank_enum.Okex, "BTC-USDT-SWAP"},
		{"okx 下划线输入转 SWAP", "BTC_USDT", coinank_enum.Okex, "BTC-USDT-SWAP"},
		{"okx SWAP 形态幂等", "BTC-USDT-SWAP", coinank_enum.Okex, "BTC-USDT-SWAP"},
		{"binance 下划线输入转紧凑", "BTC_USDT", coinank_enum.Binance, "BTCUSDT"},
		{"binance 紧凑输入保持", "BTCUSDT", coinank_enum.Binance, "BTCUSDT"},
		{"binance SWAP 输入去后缀", "BTC-USDT-SWAP", coinank_enum.Binance, "BTCUSDT"},
		{"bybit 下划线输入转紧凑", "ETH_USDT", coinank_enum.Bybit, "ETHUSDT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizePairSymbol(tc.symbol, tc.exchange)
			if got != tc.want {
				t.Fatalf("normalizePairSymbol(%q, %s) = %q, want %q", tc.symbol, tc.exchange, got, tc.want)
			}
		})
	}
}
