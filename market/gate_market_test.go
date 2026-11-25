package market

// Gate 合约行情取数（子任务 12）的单元测试：周期映射、sum 成交量、时间与 OHLC 校验、
// ticker 解析与失败闭合。HTTP 服务按 Gate 线上格式返回（数值字段字符串/数字混排）。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestGateIntervalDuration(t *testing.T) {
	cases := map[string]time.Duration{
		"10s": 10 * time.Second,
		"1m":  time.Minute,
		"5m":  5 * time.Minute,
		"15m": 15 * time.Minute,
		"30m": 30 * time.Minute,
		"1h":  time.Hour,
		"4h":  4 * time.Hour,
		"8h":  8 * time.Hour,
		"1d":  24 * time.Hour,
		"7d":  7 * 24 * time.Hour,
		"1w":  7 * 24 * time.Hour,
		"30d": 30 * 24 * time.Hour,
	}
	for interval, want := range cases {
		got, err := GateIntervalDuration(interval)
		if err != nil {
			t.Fatalf("GateIntervalDuration(%q): %v", interval, err)
		}
		if got != want {
			t.Errorf("GateIntervalDuration(%q) = %v, want %v", interval, got, want)
		}
	}

	for _, interval := range []string{"3m", "10m", "2h", "6h", "12h", "3d", "1M", "", "60m"} {
		if _, err := GateIntervalDuration(interval); err == nil {
			t.Errorf("GateIntervalDuration(%q) must reject non-retained intervals", interval)
		}
	}
}

// serveGateCandles 起本地服务返回固定 candlesticks 响应，并校验请求形状。
func serveGateCandles(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/futures/usdt/candlesticks" {
			t.Errorf("unexpected path %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		if q.Get("contract") != "DOGE_USDT" || q.Get("interval") != "1m" || q.Get("limit") != "200" {
			t.Errorf("unexpected query %v", q)
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
}

const gateCandlesBody = `[
 {"t":1700000000,"v":"10","c":"100.5","h":"101","l":"99.5","o":"100","sum":"12000.5"},
 {"t":1700000060,"v":"12","c":"101","h":"102","l":"100","o":"100.5","sum":12500},
 {"t":1700000120,"v":"8","c":"100","h":"101","l":"99","o":"101","sum":"11000"}
]`

func TestGetGateKlinesMapsSumVolume(t *testing.T) {
	srv := serveGateCandles(t, http.StatusOK, gateCandlesBody)
	overrideGateBaseURL(t, srv)

	klines, err := GetGateKlines("DOGEUSDT", "1m", 200)
	if err != nil {
		t.Fatalf("GetGateKlines: %v", err)
	}
	if len(klines) != 3 {
		t.Fatalf("len(klines) = %d, want 3", len(klines))
	}
	first := klines[0]
	if first.OpenTime != 1700000000000 || first.CloseTime != 1700000000000+60000 {
		t.Errorf("first bar times = %d/%d", first.OpenTime, first.CloseTime)
	}
	if first.Open != 100 || first.High != 101 || first.Low != 99.5 || first.Close != 100.5 {
		t.Errorf("first bar OHLC = %+v", first)
	}
	if first.Volume != 12000.5 {
		t.Errorf("first bar volume = %v, want quote volume 12000.5 (sum)", first.Volume)
	}
	if klines[1].Volume != 12500 {
		t.Errorf("second bar volume = %v, want 12500 (numeric sum)", klines[1].Volume)
	}
	for i := 1; i < len(klines); i++ {
		if klines[i].OpenTime <= klines[i-1].OpenTime {
			t.Errorf("klines are not strictly increasing at %d", i)
		}
	}
}

func TestGetGateKlinesMonthlyCloseTime(t *testing.T) {
	// 30d 是自然月：一月根到二月根的开盘，最后一根用 30 天近似
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("interval"); got != "30d" {
			t.Errorf("interval = %q, want 30d", got)
		}
		fmt.Fprint(w, `[
		 {"t":1767225600,"v":"1","c":"100","h":"101","l":"99","o":"100","sum":"10"},
		 {"t":1769904000,"v":"1","c":"110","h":"111","l":"109","o":"110","sum":"11"}
		]`)
	}))
	overrideGateBaseURL(t, srv)

	klines, err := GetGateKlines("BTC_USDT", "30d", 2)
	if err != nil {
		t.Fatalf("GetGateKlines: %v", err)
	}
	if klines[0].CloseTime != klines[1].OpenTime {
		t.Errorf("first close = %d, want next open %d", klines[0].CloseTime, klines[1].OpenTime)
	}
	if got, want := klines[1].CloseTime-klines[1].OpenTime, int64(30*24*time.Hour/time.Millisecond); got != want {
		t.Errorf("last close duration = %d ms, want 30d approximation %d", got, want)
	}
}

func TestGetGateKlinesWeeklyCloseTime(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("interval"); got != "1w" {
			t.Errorf("interval = %q, want 1w", got)
		}
		fmt.Fprint(w, `[{"t":1700000000,"v":"1","c":"100","h":"101","l":"99","o":"100","sum":"10"}]`)
	}))
	overrideGateBaseURL(t, srv)

	klines, err := GetGateKlines("BTC_USDT", "1w", 1)
	if err != nil {
		t.Fatalf("GetGateKlines: %v", err)
	}
	if got, want := klines[0].CloseTime-klines[0].OpenTime, int64(7*24*time.Hour/time.Millisecond); got != want {
		t.Errorf("weekly bar duration = %d ms, want %d", got, want)
	}
}

func TestGetGateKlinesFailClosed(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{name: "HTTP 500", status: http.StatusInternalServerError, body: `{"label":"INTERNAL"}`, wantErr: "status 500"},
		{name: "空数组", status: http.StatusOK, body: `[]`, wantErr: "returned no data"},
		{name: "非法 JSON", status: http.StatusOK, body: `{`, wantErr: "decode failed"},
		{name: "缺 sum", status: http.StatusOK, body: `[{"t":1700000000,"v":"1","c":"100","h":"101","l":"99","o":"100"}]`, wantErr: "missing sum"},
		{name: "sum 为负", status: http.StatusOK, body: `[{"t":1700000000,"v":"1","c":"100","h":"101","l":"99","o":"100","sum":"-1"}]`, wantErr: "negative sum"},
		{name: "收盘价为零", status: http.StatusOK, body: `[{"t":1700000000,"v":"1","c":"0","h":"101","l":"99","o":"100","sum":"1"}]`, wantErr: "non-positive c"},
		{name: "OHLC 不一致（收盘高于最高）", status: http.StatusOK, body: `[{"t":1700000000,"v":"1","c":"105","h":"101","l":"99","o":"100","sum":"1"}]`, wantErr: "inconsistent ohlc"},
		{name: "OHLC 不一致（开盘低于最低）", status: http.StatusOK, body: `[{"t":1700000000,"v":"1","c":"100","h":"101","l":"99","o":"98","sum":"1"}]`, wantErr: "inconsistent ohlc"},
		{name: "OHLC 不一致（开盘高于最高）", status: http.StatusOK, body: `[{"t":1700000000,"v":"1","c":"100","h":"101","l":"99","o":"102","sum":"1"}]`, wantErr: "inconsistent ohlc"},
		{name: "OHLC 不一致（最低高于收盘）", status: http.StatusOK, body: `[{"t":1700000000,"v":"1","c":"98","h":"101","l":"99","o":"100","sum":"1"}]`, wantErr: "inconsistent ohlc"},
		{name: "t 为零", status: http.StatusOK, body: `[{"t":0,"v":"1","c":"100","h":"101","l":"99","o":"100","sum":"1"}]`, wantErr: "non-positive t"},
		{name: "t 为负", status: http.StatusOK, body: `[{"t":-1,"v":"1","c":"100","h":"101","l":"99","o":"100","sum":"1"}]`, wantErr: "non-positive t"},
		{name: "缺 t", status: http.StatusOK, body: `[{"v":"1","c":"100","h":"101","l":"99","o":"100","sum":"1"}]`, wantErr: "missing t"},
		{name: "t 非整数", status: http.StatusOK, body: `[{"t":1700000000.5,"v":"1","c":"100","h":"101","l":"99","o":"100","sum":"1"}]`, wantErr: "not an integer second"},
		{
			name:    "时间戳非递增",
			status:  http.StatusOK,
			body:    `[{"t":1700000060,"v":"1","c":"100","h":"101","l":"99","o":"100","sum":"1"},{"t":1700000060,"v":"1","c":"100","h":"101","l":"99","o":"100","sum":"1"}]`,
			wantErr: "is not after previous",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := serveGateCandles(t, c.status, c.body)
			overrideGateBaseURL(t, srv)

			_, err := GetGateKlines("DOGE_USDT", "1m", 200)
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, c.wantErr)
			}
		})
	}
}

func TestGetGateKlinesRejectsBadArgumentsWithoutRequest(t *testing.T) {
	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		fmt.Fprint(w, `[]`)
	}))
	overrideGateBaseURL(t, srv)

	for _, interval := range []string{"3m", "2h", "6h", "12h", "3d", ""} {
		if _, err := GetGateKlines("DOGE_USDT", interval, 200); err == nil {
			t.Errorf("interval %q must be rejected", interval)
		}
	}
	for _, limit := range []int{0, -1, 2001} {
		if _, err := GetGateKlines("DOGE_USDT", "1m", limit); err == nil {
			t.Errorf("limit %d must be rejected", limit)
		}
	}
	if _, err := GetGateKlines("", "1m", 200); err == nil {
		t.Error("empty symbol must be rejected")
	}
	if _, err := GetGateTicker("  "); err == nil {
		t.Error("blank ticker symbol must be rejected")
	}
	if got := atomic.LoadInt32(&requests); got != 0 {
		t.Errorf("invalid arguments made %d requests, want 0", got)
	}
}

// serveGateTickers 校验 ticker 请求形状：wantContract 非空时必须有该 contract 查询参数，
// 为空表示全量列表（不带 contract）。
func serveGateTickers(t *testing.T, wantContract, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/futures/usdt/tickers" {
			t.Errorf("unexpected path %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("contract"); got != wantContract {
			t.Errorf("ticker contract query = %q, want %q", got, wantContract)
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, body)
	}))
}

func TestGetGateTicker(t *testing.T) {
	srv := serveGateTickers(t, "BTC_USDT", `[
		{"contract":"BTC_USDT","last":"97000.5","change_percentage":"-1.25","volume_24h_quote":"1234567.89"}
	]`)
	overrideGateBaseURL(t, srv)

	ticker, err := GetGateTicker("BTCUSDT")
	if err != nil {
		t.Fatalf("GetGateTicker: %v", err)
	}
	if ticker.Contract != "BTC_USDT" || ticker.Symbol != "BTCUSDT" {
		t.Errorf("ticker symbols = %q/%q", ticker.Contract, ticker.Symbol)
	}
	if ticker.Last != 97000.5 || ticker.ChangePercent24h != -1.25 || ticker.QuoteVolume24h != 1234567.89 {
		t.Errorf("ticker values = %+v", ticker)
	}
}

func TestGetGateTickerFailClosed(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantErr string
	}{
		{name: "空数组", body: `[]`, wantErr: "no entry"},
		{name: "其它合约", body: `[{"contract":"ETH_USDT","last":"3000","change_percentage":"1","volume_24h_quote":"1"}]`, wantErr: "no entry"},
		{name: "缺 last", body: `[{"contract":"BTC_USDT","change_percentage":"1","volume_24h_quote":"1"}]`, wantErr: "missing last"},
		{name: "缺 24h 成交额", body: `[{"contract":"BTC_USDT","last":"97000","change_percentage":"1"}]`, wantErr: "missing volume_24h_quote"},
		// 非数字字符串不是合法 json.Number：整个响应解码失败（fail-closed，不返回半截数据）
		{name: "涨跌幅非法", body: `[{"contract":"BTC_USDT","last":"97000","change_percentage":"abc","volume_24h_quote":"1"}]`, wantErr: "decode failed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := serveGateTickers(t, "BTC_USDT", c.body)
			overrideGateBaseURL(t, srv)
			_, err := GetGateTicker("BTCUSDT")
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, c.wantErr)
			}
		})
	}
}

func TestListGateTickers(t *testing.T) {
	srv := serveGateTickers(t, "", `[
		{"contract":"BTC_USDT","last":"97000","change_percentage":"1.5","volume_24h_quote":"100"},
		{"contract":"DOGE_USDT","last":"0.0543","change_percentage":"-2","volume_24h_quote":"20"}
	]`)
	overrideGateBaseURL(t, srv)

	tickers, err := ListGateTickers()
	if err != nil {
		t.Fatalf("ListGateTickers: %v", err)
	}
	if len(tickers) != 2 {
		t.Fatalf("len(tickers) = %d, want 2", len(tickers))
	}
	if tickers[0].Symbol != "BTCUSDT" || tickers[1].Symbol != "DOGEUSDT" {
		t.Errorf("normalized symbols = %q/%q", tickers[0].Symbol, tickers[1].Symbol)
	}
	if tickers[1].Last != 0.0543 || tickers[1].ChangePercent24h != -2 {
		t.Errorf("second ticker = %+v", tickers[1])
	}
}

func TestListGateTickersFailClosed(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantErr string
	}{
		{name: "空数组", body: `[]`, wantErr: "no contracts"},
		{
			name:    "任一条目缺字段",
			body:    `[{"contract":"BTC_USDT","last":"97000","change_percentage":"1","volume_24h_quote":"100"},{"contract":"DOGE_USDT","last":"0.0543"}]`,
			wantErr: "DOGE_USDT",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := serveGateTickers(t, "", c.body)
			overrideGateBaseURL(t, srv)
			_, err := ListGateTickers()
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, c.wantErr)
			}
		})
	}
}

func TestGetGateTickerNumericFields(t *testing.T) {
	// Gate 的数值字段也可能是 JSON number 而不是字符串
	srv := serveGateTickers(t, "BTC_USDT", `[
		{"contract":"BTC_USDT","last":97000.5,"change_percentage":1.25,"volume_24h_quote":1234567.89}
	]`)
	overrideGateBaseURL(t, srv)

	ticker, err := GetGateTicker("BTCUSDT")
	if err != nil {
		t.Fatalf("GetGateTicker: %v", err)
	}
	if ticker.Last != 97000.5 || ticker.ChangePercent24h != 1.25 || ticker.QuoteVolume24h != 1234567.89 {
		t.Errorf("numeric ticker fields = %+v", ticker)
	}
}

func TestGetGateKlinesAcceptsMaxLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("limit"); got != "2000" {
			t.Errorf("limit = %q, want 2000", got)
		}
		fmt.Fprint(w, `[{"t":1700000000,"v":"1","c":"100","h":"101","l":"99","o":"100","sum":"1"}]`)
	}))
	overrideGateBaseURL(t, srv)

	if _, err := GetGateKlines("DOGE_USDT", "1m", 2000); err != nil {
		t.Fatalf("limit 2000 must be accepted: %v", err)
	}
}

func TestGetGateKlinesMonthlySmallLimitIsNotMonthStart(t *testing.T) {
	// 小 limit 的 30d 窗口不保证月初（实测 limit=3 为 9/10/11 月 2 日）：
	// 代码不依赖月初对齐，CloseTime 仍取下一根开盘。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[
		 {"t":1788220800,"v":"1","c":"100","h":"101","l":"99","o":"100","sum":"10"},
		 {"t":1790812800,"v":"1","c":"110","h":"111","l":"109","o":"110","sum":"11"}
		]`)
	}))
	overrideGateBaseURL(t, srv)

	klines, err := GetGateKlines("BTC_USDT", "30d", 3)
	if err != nil {
		t.Fatalf("GetGateKlines: %v", err)
	}
	if klines[0].CloseTime != klines[1].OpenTime {
		t.Errorf("first close = %d, want next open %d", klines[0].CloseTime, klines[1].OpenTime)
	}
}

func TestGateNumberRejectsNonFinite(t *testing.T) {
	for _, raw := range []json.Number{"NaN", "Inf", "+Inf", "-Inf", "abc"} {
		if _, err := gateNumber("x", raw); err == nil {
			t.Errorf("gateNumber(%q) must be rejected", raw)
		}
	}
	for _, raw := range []json.Number{"0", "-1.5", "123.45"} {
		if _, err := gateNumber("x", raw); err != nil {
			t.Errorf("gateNumber(%q): %v", raw, err)
		}
	}
}

func TestGetGateTickerExtendedFields(t *testing.T) {
	srv := serveGateTickers(t, "BTC_USDT", `[
		{"contract":"BTC_USDT","last":"97000","change_percentage":"2","change_price":"1900","high_24h":"98000","low_24h":"94000","volume_24h_quote":"123456"}
	]`)
	overrideGateBaseURL(t, srv)

	ticker, err := GetGateTicker("BTCUSDT")
	if err != nil {
		t.Fatalf("GetGateTicker: %v", err)
	}
	if ticker.Change24h != 1900 || ticker.High24h != 98000 || ticker.Low24h != 94000 {
		t.Errorf("extended ticker fields = %+v", ticker)
	}
}

func TestGetGateTickerOptional24hFields(t *testing.T) {
	// 新合约可能没有 24h 高低/涨跌金额：缺省为 0，不让整条 ticker 失败
	srv := serveGateTickers(t, "NEW_USDT", `[{"contract":"NEW_USDT","last":"1.5","change_percentage":"0","volume_24h_quote":"0"}]`)
	overrideGateBaseURL(t, srv)
	ticker, err := GetGateTicker("NEWUSDT")
	if err != nil {
		t.Fatalf("GetGateTicker: %v", err)
	}
	if ticker.Change24h != 0 || ticker.High24h != 0 || ticker.Low24h != 0 {
		t.Errorf("optional fields should default to 0: %+v", ticker)
	}
}

func serveGateContract(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/futures/usdt/contracts/BTC_USDT":
			fmt.Fprint(w, `{"name":"BTC_USDT","mark_price":"96950.5","index_price":"97000.2","funding_rate":"0.0001","funding_next_apply":1767225600}`)
		case r.URL.Path == "/futures/usdt/contract_stats":
			fmt.Fprint(w, `[{"time":100,"open_interest":"1000","open_interest_usd":"3000000000","last_funding_rate":"0.0001"}]`)
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
}

func TestGetGateContractMetrics(t *testing.T) {
	srv := serveGateContract(t)
	overrideGateBaseURL(t, srv)

	metrics, err := GetGateContractMetrics("BTCUSDT")
	if err != nil {
		t.Fatalf("GetGateContractMetrics: %v", err)
	}
	if metrics.MarkPrice != 96950.5 || metrics.IndexPrice != 97000.2 || metrics.FundingRate != 0.0001 {
		t.Errorf("metrics = %+v", metrics)
	}
	if metrics.NextFundingTime != 1767225600000 {
		t.Errorf("next funding ms = %d", metrics.NextFundingTime)
	}
	if metrics.OpenInterestUSD != 3000000000 {
		t.Errorf("open interest USD = %v", metrics.OpenInterestUSD)
	}
}

func TestGetGateContractMetricsFailClosed(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
		wantErr string
	}{
		{
			name: "合约接口 500",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "boom", http.StatusInternalServerError)
			},
			wantErr: "status 500",
		},
		{
			name: "标记价缺失",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/futures/usdt/contract_stats" {
					fmt.Fprint(w, `[{"time":100,"open_interest":"1","open_interest_usd":"2","last_funding_rate":"0"}]`)
					return
				}
				fmt.Fprint(w, `{"index_price":"97000","funding_rate":"0"}`)
			},
			wantErr: "missing mark_price",
		},
		{
			name: "OI 不可用",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/futures/usdt/contract_stats" {
					http.Error(w, "boom", http.StatusInternalServerError)
					return
				}
				fmt.Fprint(w, `{"mark_price":"1","index_price":"1","funding_rate":"0"}`)
			},
			wantErr: "open interest",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(c.handler)
			overrideGateBaseURL(t, srv)
			_, err := GetGateContractMetrics("BTCUSDT")
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, c.wantErr)
			}
		})
	}
}

func TestGetGateKlinesMonthlySmallLimitFetchesAllHistory(t *testing.T) {
	// 30d 小 limit：向 Gate 请求全历史上限，再截取最后 limit 根（避免非月初/占位根）
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if got := query.Get("interval"); got != "30d" {
			t.Errorf("interval = %q", got)
		}
		if got := query.Get("limit"); got != "2000" {
			t.Errorf("30d small limit must request 2000, got %q", got)
		}
		fmt.Fprint(w, `[
		 {"t":1767225600,"v":"1","c":"90","h":"91","l":"89","o":"90","sum":"10"},
		 {"t":1769904000,"v":"1","c":"91","h":"92","l":"90","o":"91","sum":"11"},
		 {"t":1772323200,"v":"1","c":"92","h":"93","l":"91","o":"92","sum":"12"},
		 {"t":1775001600,"v":"1","c":"93","h":"94","l":"92","o":"93","sum":"13"},
		 {"t":1777593600,"v":"1","c":"94","h":"95","l":"93","o":"94","sum":"14"}
		]`)
	}))
	overrideGateBaseURL(t, srv)

	klines, err := GetGateKlines("BTC_USDT", "30d", 3)
	if err != nil {
		t.Fatalf("GetGateKlines: %v", err)
	}
	if len(klines) != 3 {
		t.Fatalf("len = %d, want last 3 bars", len(klines))
	}
	// 截取后的第一根（原第 3 根）CloseTime 仍是全序列里下一根的开盘（1775001600）
	if klines[0].CloseTime != 1775001600000 {
		t.Errorf("sliced close time = %d, want 1775001600000", klines[0].CloseTime)
	}
	if klines[2].Close != 94 || klines[2].Volume != 14 {
		t.Errorf("last sliced bar = %+v", klines[2])
	}
}
