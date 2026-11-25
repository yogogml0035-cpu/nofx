package market

// 引擎行情路径切 Gate 的单元测试（子任务 13）：GetWithTimeframes（含 10s/7d/30d）与
// GetBoxData 都只请求 Gate candlesticks，成交量是 sum 成交额。

import (
	"encoding/json"
	"fmt"
	"math"
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
	// 箱体固定请求 Gate 1h × LongBoxPeriod(500)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/futures/usdt/candlesticks" {
			http.NotFound(w, r)
			return
		}
		query := r.URL.Query()
		if got := query.Get("interval"); got != "1h" {
			t.Errorf("box interval = %q, want 1h", got)
		}
		if got := query.Get("limit"); got != "500" {
			t.Errorf("box limit = %q, want 500", got)
		}
		rows := make([]map[string]interface{}, 0, 30)
		for i := 0; i < 30; i++ {
			price := 100.0 + float64(i)*0.5
			rows = append(rows, map[string]interface{}{
				"t": 1700000000 + int64(i)*3600, "v": "10",
				"o": fmt.Sprintf("%.4f", price), "h": fmt.Sprintf("%.4f", price+1),
				"l": fmt.Sprintf("%.4f", price-1), "c": fmt.Sprintf("%.4f", price+0.25),
				"sum": "1000",
			})
		}
		json.NewEncoder(w).Encode(rows)
	}))
	overrideGateBaseURL(t, srv)

	box, err := GetBoxData("DOGEUSDT")
	if err != nil {
		t.Fatalf("GetBoxData: %v", err)
	}
	if box == nil || box.ShortUpper <= 0 || box.LongUpper <= 0 || box.CurrentPrice <= 0 {
		t.Errorf("box data = %+v", box)
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

func TestGetWithTimeframesFailureClosure(t *testing.T) {
	// 全周期失败 → 主周期为空，整次失败
	failAll := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	overrideGateBaseURL(t, failAll)
	if _, err := GetWithTimeframes("DOGEUSDT", []string{"5m", "1h"}, "5m", 5); err == nil {
		t.Fatal("all-period failure must return an error")
	}

	// 主周期失败、副周期成功 → 仍然失败（没有主周期指标）
	primaryFails := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/futures/usdt/candlesticks" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("interval") == "5m" {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		writeOneCandle(w)
	}))
	overrideGateBaseURL(t, primaryFails)
	if _, err := GetWithTimeframes("DOGEUSDT", []string{"5m", "1h"}, "5m", 5); err == nil {
		t.Fatal("primary timeframe failure must return an error")
	}

	// 副周期失败、主周期成功 → 返回主周期数据（既有语义：逐周期 log 跳过）
	secondaryFails := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/futures/usdt/candlesticks" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("interval") == "1h" {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		writeOneCandle(w)
	}))
	overrideGateBaseURL(t, secondaryFails)
	data, err := GetWithTimeframes("DOGEUSDT", []string{"5m", "1h"}, "5m", 5)
	if err != nil {
		t.Fatalf("secondary failure should be tolerated: %v", err)
	}
	if _, ok := data.TimeframeData["5m"]; !ok {
		t.Error("primary timeframe data missing")
	}
	if _, ok := data.TimeframeData["1h"]; ok {
		t.Error("failed secondary timeframe must be absent")
	}
}

func writeOneCandle(w http.ResponseWriter) {
	json.NewEncoder(w).Encode([]map[string]interface{}{{
		"t": 1700000000, "v": "1", "o": "100", "h": "101", "l": "99", "c": "100.5", "sum": "500",
	}})
}

func TestCalculatePriceChangeByBarsWindows(t *testing.T) {
	// 5m：1h 回看 12 根
	build := func(step time.Duration, n int) []Kline {
		klines := make([]Kline, n)
		for i := range klines {
			klines[i] = Kline{
				OpenTime: int64(1700000000000) + int64(i)*step.Milliseconds(),
				Close:    100 + float64(i),
			}
		}
		return klines
	}
	fiveMinute := build(5*time.Minute, 30)
	// 最后一根 close=129，idx = 29-12 = 17 → close=117
	if got := calculatePriceChangeByBars(fiveMinute, "5m", 60); math.Abs(got-(129.0-117.0)/117.0*100) > 1e-9 {
		t.Errorf("5m 1h change = %v", got)
	}

	// 10s 主周期：200 根只有约 33 分钟，回看被夹到最早一根（不是 1h）
	tenSecond := build(10*time.Second, 200)
	if got := calculatePriceChangeByBars(tenSecond, "10s", 60); got <= 0 {
		t.Errorf("10s 1h change = %v, want positive over the available window", got)
	}

	// 30d 主周期：barsBack 退化为 1 根
	monthly := build(30*24*time.Hour, 3)
	got := calculatePriceChangeByBars(monthly, "30d", 60)
	want := (102.0 - 101.0) / 101.0 * 100
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("30d change = %v, want %v (one bar back)", got, want)
	}

	// 非保留周期：无法换算，返回 0
	if got := calculatePriceChangeByBars(monthly, "3m", 60); got != 0 {
		t.Errorf("unsupported timeframe change = %v, want 0", got)
	}
}
