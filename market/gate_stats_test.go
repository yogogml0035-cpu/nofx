package market

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestToGateContractSymbol(t *testing.T) {
	cases := map[string]string{
		"BTCUSDT":      "BTC_USDT",
		"ETHUSDT":      "ETH_USDT",
		"BTC_USDT":     "BTC_USDT",
		"btcusdt":      "BTC_USDT",
		"BTC-USDT":     "BTC_USDT",
		"HYPEUSDT":     "HYPE_USDT",
		"XAUUSDT":      "XAU_USDT",
		"BTC-USDTSWAP": "BTC_USDT",
	}
	for in, want := range cases {
		if got := toGateContractSymbol(in); got != want {
			t.Errorf("toGateContractSymbol(%q) = %q, want %q", in, got, want)
		}
	}
}

// serveGateStats 起一个本地 HTTP 服务返回固定的 contract_stats 响应，并校验请求参数形状。
func serveGateStats(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/futures/usdt/contract_stats" {
			t.Errorf("unexpected path %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		if q.Get("contract") != "BTC_USDT" || q.Get("interval") != "5m" || q.Get("limit") != "12" {
			t.Errorf("unexpected query %v", q)
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
}

func overrideGateBaseURL(t *testing.T, srv *httptest.Server) {
	t.Helper()
	orig := gateFuturesBaseURL
	gateFuturesBaseURL = srv.URL
	t.Cleanup(func() { gateFuturesBaseURL = orig })
}

// 响应乱序且 number/string 两种数值形态混排：最新期必须按时间戳选取，均值不受顺序影响。
func TestGetGateOpenInterestAndFunding(t *testing.T) {
	srv := serveGateStats(t, http.StatusOK, `[
		{"time":300,"open_interest":"1020","open_interest_usd":3600000000,"last_funding_rate":0.0003},
		{"time":100,"open_interest":"1000","open_interest_usd":"3000000000","last_funding_rate":"0.0001"},
		{"time":200,"open_interest":"1010","open_interest_usd":"3300000000","last_funding_rate":0.0002}
	]`)
	defer srv.Close()
	overrideGateBaseURL(t, srv)

	oi, fr, err := getGateOpenInterestAndFunding("BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if oi == nil || fr == nil {
		t.Fatalf("expected non-nil OI and funding rate")
	}
	if oi.Latest != 3_600_000_000 {
		t.Errorf("Latest = %v, want 3600000000 (the row with max time)", oi.Latest)
	}
	if oi.Average != 3_300_000_000 {
		t.Errorf("Average = %v, want 3300000000", oi.Average)
	}
	if *fr != 0.0003 {
		t.Errorf("funding rate = %v, want 0.0003 (the row with max time)", *fr)
	}
}

func TestGetGateOpenInterestAndFundingEmpty(t *testing.T) {
	srv := serveGateStats(t, http.StatusOK, `[]`)
	defer srv.Close()
	overrideGateBaseURL(t, srv)

	oi, fr, err := getGateOpenInterestAndFunding("BTCUSDT")
	if err == nil || oi != nil || fr != nil {
		t.Fatalf("expected error with nil results, got oi=%v fr=%v err=%v", oi, fr, err)
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error should mention empty, got: %v", err)
	}
}

func TestGetGateOpenInterestAndFundingLatestInvalid(t *testing.T) {
	// 最新一期缺少 open_interest_usd（Gate 以字段缺失表达无值）
	srv := serveGateStats(t, http.StatusOK, `[
		{"time":100,"open_interest":"1000","open_interest_usd":"3000000000","last_funding_rate":"0.0001"},
		{"time":200,"open_interest":"1010","last_funding_rate":"0.0002"}
	]`)
	defer srv.Close()
	overrideGateBaseURL(t, srv)

	oi, fr, err := getGateOpenInterestAndFunding("BTCUSDT")
	if err == nil || oi != nil || fr != nil {
		t.Fatalf("expected error for invalid latest row, got oi=%v fr=%v err=%v", oi, fr, err)
	}
	if !strings.Contains(err.Error(), "latest open_interest_usd invalid") {
		t.Errorf("error should mention invalid latest open_interest_usd, got: %v", err)
	}
}

// 限频等错误响应是 JSON 对象而非数组，必须以状态码明确报错，不能落到解码错误上。
func TestGetGateOpenInterestAndFundingNonOKStatus(t *testing.T) {
	srv := serveGateStats(t, http.StatusTooManyRequests,
		`{"label":"TOO_MANY_REQUESTS","message":"request too frequent"}`)
	defer srv.Close()
	overrideGateBaseURL(t, srv)

	oi, fr, err := getGateOpenInterestAndFunding("BTCUSDT")
	if err == nil || oi != nil || fr != nil {
		t.Fatalf("expected error for non-200 response, got oi=%v fr=%v err=%v", oi, fr, err)
	}
	for _, want := range []string{"status 429", "TOO_MANY_REQUESTS"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should contain %q, got: %v", want, err)
		}
	}
}

func TestErrorBodySnippet(t *testing.T) {
	if got := errorBodySnippet([]byte("  {\"label\":\"X\"} \n")); got != `{"label":"X"}` {
		t.Errorf("short body should be trimmed, got %q", got)
	}
	if got := errorBodySnippet([]byte(strings.Repeat("x", 500))); len(got) != 200 {
		t.Errorf("long body should be truncated to 200 bytes, got %d", len(got))
	}
}
