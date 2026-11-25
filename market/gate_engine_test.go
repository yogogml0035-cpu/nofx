package market

// 引擎行情路径切 Gate 的单元测试（子任务 13）：GetWithTimeframes（含 10s/7d/30d）、
// GetBoxData、旧内嵌 GetWithExchange 都只请求 Gate candlesticks，成交量是 sum 成交额。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// gateCandleServer 按 interval 返回一组递增的 Gate K 线；记录收到的 interval 与 limit。
func gateCandleServer(t *testing.T, bars int, basePrice float64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/futures/usdt/candlesticks" {
			// OI/资金费率容忍失败，其余未知路径直接 404
			http.NotFound(w, r)
			return
		}
		query := r.URL.Query()
		interval := query.Get("interval")
		duration, err := GateIntervalDuration(interval)
		if err != nil {
			t.Errorf("unexpected interval %q", interval)
			http.Error(w, "bad interval", http.StatusBadRequest)
			return
		}
		start := int64(1700000000)
		step := int64(duration / time.Second)
		rows := make([]map[string]interface{}, 0, bars)
		for i := 0; i < bars; i++ {
			price := basePrice + float64(i)*0.5
			rows = append(rows, map[string]interface{}{
				"t":   start + int64(i)*step,
				"v":   "10",
				"o":   fmt.Sprintf("%.4f", price),
				"h":   fmt.Sprintf("%.4f", price+1),
				"l":   fmt.Sprintf("%.4f", price-1),
				"c":   fmt.Sprintf("%.4f", price+0.25),
				"sum": fmt.Sprintf("%.2f", 1000+float64(i)),
			})
		}
		json.NewEncoder(w).Encode(rows)
	}))
}

func TestGetWithTimeframesUsesGate(t *testing.T) {
	srv := gateCandleServer(t, 30, 100)
	overrideGateBaseURL(t, srv)

	timeframes := []string{"10s", "5m", "1d", "7d", "30d"}
	data, err := GetWithTimeframes("DOGEUSDT", timeframes, "10s", 5)
	if err != nil {
		t.Fatalf("GetWithTimeframes: %v", err)
	}
	if data.Symbol != "DOGEUSDT" {
		t.Errorf("symbol = %q", data.Symbol)
	}
	// 最新价来自 10s 主周期最后一根收盘
	if data.CurrentPrice <= 0 {
		t.Errorf("current price = %v", data.CurrentPrice)
	}
	for _, tf := range timeframes {
		series, ok := data.TimeframeData[tf]
		if !ok {
			t.Fatalf("missing timeframe %s in %v", tf, data.TimeframeData)
		}
		if len(series.Klines) != 5 {
			t.Errorf("%s klines = %d, want count 5", tf, len(series.Klines))
		}
		if len(series.Klines) > 0 && series.Klines[0].Volume <= 0 {
			t.Errorf("%s volume = %v, want quote volume from sum", tf, series.Klines[0].Volume)
		}
	}
	// 30 根里取最后 5 根：第一根是第 25 根，sum=1000+25
	if got := data.TimeframeData["10s"].Klines[0].Volume; got != 1025 {
		t.Errorf("10s first displayed volume = %v, want 1025 (sum)", got)
	}
}

func TestGetWithTimeframesRejectsUnsupportedPeriod(t *testing.T) {
	srv := gateCandleServer(t, 10, 100)
	overrideGateBaseURL(t, srv)

	_, err := GetWithTimeframes("DOGEUSDT", []string{"3m"}, "3m", 5)
	if err == nil {
		t.Fatal("3m must be rejected")
	}
	if !strings.Contains(err.Error(), "Primary timeframe 3m") {
		t.Errorf("error = %v", err)
	}
}

func TestGetBoxDataUsesGate(t *testing.T) {
	srv := gateCandleServer(t, 30, 100)
	overrideGateBaseURL(t, srv)

	box, err := GetBoxData("DOGEUSDT")
	if err != nil {
		t.Fatalf("GetBoxData: %v", err)
	}
	if box == nil || box.ShortUpper <= 0 || box.LongUpper <= 0 || box.CurrentPrice <= 0 {
		t.Errorf("box data = %+v", box)
	}
}

func TestGetWithExchangeUsesGateAndIgnoresExchange(t *testing.T) {
	srv := gateCandleServer(t, 30, 100)
	overrideGateBaseURL(t, srv)

	for _, exchange := range []string{"binance", "okx", "unknown"} {
		data, err := GetWithExchange("DOGEUSDT", exchange)
		if err != nil {
			t.Fatalf("GetWithExchange(%q): %v", exchange, err)
		}
		if data.IntradaySeries == nil || data.LongerTermContext == nil {
			t.Errorf("%q: legacy series missing: %+v", exchange, data)
		}
		if data.CurrentPrice <= 0 || data.CurrentEMA20 <= 0 {
			t.Errorf("%q: indicators missing: %+v", exchange, data)
		}
		if data.PriceChange1h == 0 {
			t.Errorf("%q: 1h price change = 0, want computed from 5m bars", exchange)
		}
		if len(data.IntradaySeries.Volume) == 0 || data.IntradaySeries.Volume[0] <= 0 {
			t.Errorf("%q: intraday volume missing (want quote volume)", exchange)
		}
	}
}

func TestGetBoxDataFailClosedWhenGateFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	overrideGateBaseURL(t, srv)
	if _, err := GetBoxData("DOGEUSDT"); err == nil {
		t.Fatal("GetBoxData must fail closed when Gate returns 500")
	}
}
