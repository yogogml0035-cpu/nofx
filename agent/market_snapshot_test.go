package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"nofx/market"
)

// 助手快照工具的单元测试：取数经 Gate 汇聚层的测试缝注入，组装走纯函数。
func TestToolGetMarketSnapshotReturnsRealtimeAnalysisContext(t *testing.T) {
	prevTicker, prevMetrics, prevKlines := assistantGetTicker, assistantGetMetrics, assistantGetKlines
	t.Cleanup(func() {
		assistantGetTicker, assistantGetMetrics, assistantGetKlines = prevTicker, prevMetrics, prevKlines
	})
	assistantGetTicker = func(symbol string) (market.GateTicker, error) {
		if symbol != "BTCUSDT" {
			t.Errorf("ticker symbol = %q, want BTCUSDT", symbol)
		}
		return market.GateTicker{
			Contract: "BTC_USDT", Symbol: "BTCUSDT", Last: 65000,
			ChangePercent24h: 1.88, Change24h: 1200, High24h: 66000, Low24h: 63800,
			QuoteVolume24h: 800000000,
		}, nil
	}
	assistantGetMetrics = func(symbol string) (market.GateContractMetrics, error) {
		return market.GateContractMetrics{
			MarkPrice: 65010, IndexPrice: 64990, FundingRate: 0.0001,
			NextFundingTime: 1710000000000, OpenInterestUSD: 45678.9,
		}, nil
	}
	assistantGetKlines = func(symbol, interval string, limit int) ([]market.Kline, error) {
		if symbol != "BTCUSDT" || interval != "15m" || limit != 2 {
			t.Errorf("klines request = %s %s %d", symbol, interval, limit)
		}
		return []market.Kline{
			{OpenTime: 1710000000000, Open: 64000, High: 65100, Low: 63900, Close: 64500, Volume: 100, CloseTime: 1710000899999},
			{OpenTime: 1710000900000, Open: 64500, High: 65500, Low: 64400, Close: 65000, Volume: 120, CloseTime: 1710001799999},
		}, nil
	}

	a := New(nil, nil, DefaultConfig(), nil)
	raw := a.toolGetMarketSnapshot(`{"symbol":"BTC","interval":"15m","limit":2}`)

	var resp struct {
		Symbol    string  `json:"symbol"`
		Price     float64 `json:"price"`
		Ticker24h struct {
			PriceChangePercent float64 `json:"price_change_percent"`
			HighPrice          float64 `json:"high_price"`
			LowPrice           float64 `json:"low_price"`
			VolumeUnit         string  `json:"volume_unit"`
		} `json:"ticker_24h"`
		PerpMetrics struct {
			FundingRate      float64 `json:"funding_rate"`
			OpenInterest     float64 `json:"open_interest"`
			OpenInterestUnit string  `json:"open_interest_unit"`
			NextFundingTime  int64   `json:"next_funding_time"`
		} `json:"perp_metrics"`
		KlineSnapshot struct {
			Interval            string           `json:"interval"`
			Limit               int              `json:"limit"`
			PeriodChangePercent float64          `json:"period_change_percent"`
			VolumeUnit          string           `json:"volume_unit"`
			RecentKlines        []map[string]any `json:"recent_klines"`
		} `json:"kline_snapshot"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("failed to parse tool response: %v\nraw=%s", err, raw)
	}
	if resp.Error != "" {
		t.Fatalf("unexpected tool error: %s", resp.Error)
	}
	if resp.Symbol != "BTCUSDT" || resp.Price != 65000 {
		t.Fatalf("unexpected symbol/price: %+v", resp)
	}
	if resp.Ticker24h.PriceChangePercent != 1.88 || resp.Ticker24h.HighPrice != 66000 || resp.Ticker24h.LowPrice != 63800 {
		t.Fatalf("unexpected ticker_24h: %+v", resp.Ticker24h)
	}
	if resp.Ticker24h.VolumeUnit != "USDT" {
		t.Fatalf("volume unit = %q", resp.Ticker24h.VolumeUnit)
	}
	if resp.PerpMetrics.FundingRate != 0.0001 || resp.PerpMetrics.OpenInterest != 45678.9 {
		t.Fatalf("unexpected perp_metrics: %+v", resp.PerpMetrics)
	}
	if resp.PerpMetrics.OpenInterestUnit != "USD" || resp.PerpMetrics.NextFundingTime != 1710000000000 {
		t.Fatalf("unexpected perp_metrics units/time: %+v", resp.PerpMetrics)
	}
	if resp.KlineSnapshot.Interval != "15m" || resp.KlineSnapshot.Limit != 2 || len(resp.KlineSnapshot.RecentKlines) != 2 {
		t.Fatalf("unexpected kline_snapshot: %+v", resp.KlineSnapshot)
	}
	if resp.KlineSnapshot.VolumeUnit != "USDT" || resp.KlineSnapshot.PeriodChangePercent <= 0 {
		t.Fatalf("unexpected kline units/change: %+v", resp.KlineSnapshot)
	}
}

func TestToolGetMarketSnapshotRejectsStockSymbols(t *testing.T) {
	a := New(nil, nil, DefaultConfig(), nil)
	raw := a.toolGetMarketSnapshot(`{"symbol":"AAPL"}`)
	if !strings.Contains(raw, "currently supports crypto symbols only") {
		t.Fatalf("expected stock rejection, got: %s", raw)
	}
}
