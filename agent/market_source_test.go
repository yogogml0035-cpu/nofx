package agent

import (
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"nofx/market"
)

func TestValidKlineIntervalD25(t *testing.T) {
	for _, interval := range []string{"10s", "1m", "5m", "15m", "30m", "1h", "4h", "8h", "1d", "7d", "1w", "30d"} {
		if !validKlineInterval(interval) {
			t.Errorf("interval %q must be accepted", interval)
		}
	}
	for _, interval := range []string{"1s", "5s", "30s", "3m", "10m", "2h", "6h", "12h", "3d", "1mo"} {
		if validKlineInterval(interval) {
			t.Errorf("interval %q must be rejected", interval)
		}
	}
}

func TestAssistantCryptoSymbol(t *testing.T) {
	cases := map[string]string{
		"BTC": "BTCUSDT", "btc": "BTCUSDT", "BTCUSDT": "BTCUSDT",
		"BTC_USDT": "BTCUSDT", "BTC-USDT": "BTCUSDT", "BTC/USDT": "BTCUSDT",
		"BTC-USDTSWAP": "BTCUSDT", "DOGE": "DOGEUSDT",
	}
	for in, want := range cases {
		if got := assistantCryptoSymbol(in); got != want {
			t.Errorf("assistantCryptoSymbol(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestToolGetKlineUsesGateAndRetainsPeriods(t *testing.T) {
	prev := assistantGetKlines
	t.Cleanup(func() { assistantGetKlines = prev })

	var gotInterval string
	var gotLimit int
	assistantGetKlines = func(symbol, interval string, limit int) ([]market.Kline, error) {
		gotInterval, gotLimit = interval, limit
		return []market.Kline{{OpenTime: 1700000000000, Open: 1, High: 2, Low: 0.5, Close: 1.5, Volume: 100, CloseTime: 1700000300000}}, nil
	}
	a := New(nil, nil, DefaultConfig(), nil)

	raw := a.toolGetKline(`{"symbol":"DOGE","interval":"10s","limit":5}`)
	if !strings.Contains(raw, `"volume_unit":"USDT"`) || !strings.Contains(raw, `"interval":"10s"`) {
		t.Fatalf("unexpected kline output: %s", raw)
	}
	if gotInterval != "10s" || gotLimit != 5 {
		t.Fatalf("kline request = %s/%d", gotInterval, gotLimit)
	}

	if raw := a.toolGetKline(`{"symbol":"DOGE","interval":"3m"}`); !strings.Contains(raw, "invalid interval") {
		t.Fatalf("3m must be rejected: %s", raw)
	}

	// limit 上界 300
	_ = a.toolGetKline(`{"symbol":"DOGE","interval":"30d","limit":9999}`)
	if gotInterval != "30d" || gotLimit != 300 {
		t.Fatalf("30d request = %s/%d, want 30d/300", gotInterval, gotLimit)
	}
}

func TestHandleTickersUsesGate(t *testing.T) {
	prev := assistantListTickers
	t.Cleanup(func() { assistantListTickers = prev })
	assistantListTickers = func() ([]market.GateTicker, error) {
		return []market.GateTicker{
			{Contract: "BTC_USDT", Symbol: "BTCUSDT", Last: 97000, ChangePercent24h: 1.5, High24h: 98000, Low24h: 95000, QuoteVolume24h: 100},
			{Contract: "ETH_USDT", Symbol: "ETHUSDT", Last: 3000, ChangePercent24h: -2, High24h: 3100, Low24h: 2900, QuoteVolume24h: 50},
		}, nil
	}

	w := NewWebHandler(nil, slog.Default())
	req := httptest.NewRequest("GET", "/api/agent/tickers?symbols=ETHUSDT,BTCUSDT", nil)
	rec := httptest.NewRecorder()
	w.HandleTickers(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var out []map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != 2 || out[0]["symbol"] != "ETHUSDT" || out[1]["symbol"] != "BTCUSDT" {
		t.Fatalf("order/fields = %+v", out)
	}
	if out[1]["lastPrice"] != "97000" || out[1]["volume"] != "100" {
		t.Fatalf("BTC ticker mapping = %+v", out[1])
	}
}
