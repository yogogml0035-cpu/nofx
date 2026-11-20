package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"nofx/provider/gatemcp"
)

// fakeGateMCP 最小 MCP Streamable HTTP 假上游（只 fake 返回结构，不 fake 业务逻辑）
type fakeGateMCP struct {
	toolResults map[string]func(args map[string]any) (map[string]any, bool)
}

func (f *fakeGateMCP) handler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed) // standalone SSE GET，SDK 容忍
		return
	}
	var req struct {
		ID     json.Number `json:"id"`
		Method string      `json:"method"`
		Params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		} `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	switch req.Method {
	case "initialize":
		w.Header().Set("Mcp-Session-Id", "sess-test")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0", "id": req.ID,
			"result": map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "fake", "version": "0.0.1"},
			},
		})
	case "notifications/initialized":
		w.WriteHeader(http.StatusAccepted)
	case "tools/call":
		gen, ok := f.toolResults[req.Params.Name]
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		sc, isErr := gen(req.Params.Arguments)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0", "id": req.ID,
			"result": map[string]any{
				"content":           []map[string]any{{"type": "text", "text": "{}"}},
				"structuredContent": sc,
				"isError":           isErr,
			},
		})
	default:
		w.WriteHeader(http.StatusBadRequest)
	}
}

func newMarketTestServer(t *testing.T, url string) *Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	s := &Server{router: gin.New()}
	s.gateMCP = gatemcp.New(url)
	s.gateMCP.Logger(discardTestLogger())
	s.setupRoutes()
	t.Cleanup(func() { s.gateMCP.Close() })
	return s
}

func discardTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func getJSON(t *testing.T, s *Server, path string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s response: %v (%s)", path, err, w.Body.String())
	}
	return w.Code, body
}

// 六个 /api/market/* 路由全部注册为公开路由
func TestMarketRoutesRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &Server{router: gin.New()}
	s.setupRoutes()

	want := map[string]bool{
		"GET /api/market/overview":      false,
		"GET /api/market/fear-greed":    false,
		"GET /api/market/news":          false,
		"GET /api/market/announcements": false,
		"GET /api/market/sentiment":     false,
		"GET /api/market/liquidations":  false,
	}
	for _, r := range s.router.Routes() {
		key := r.Method + " " + r.Path
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for route, found := range want {
		if !found {
			t.Fatalf("route not registered: %s", route)
		}
	}
}

// 正常路径：假上游返回 → 200 且结构正确
func TestMarketHandlersUpstreamOK(t *testing.T) {
	f := &fakeGateMCP{toolResults: map[string]func(args map[string]any) (map[string]any, bool){
		"info_marketsnapshot_get_market_overview": func(args map[string]any) (map[string]any, bool) {
			return map[string]any{
				"total_market_cap": 1283843560000.0, "market_cap_change_24h": 0.3,
				"total_volume_24h": 220887122000.0, "btc_dominance": 58.93, "eth_dominance": 10.13,
				"btc_price": 63747.1, "fear_greed_index": 12.0, "fear_greed_label": "Extreme Fear",
				"ahr999": 0.2961, "altcoin_season_index": 57.0, "active_coins_count": 3153.0,
				"updated_at": "2026-11-19T07:00:00Z",
			}, false
		},
		"news_feed_search_news": func(args map[string]any) (map[string]any, bool) {
			return map[string]any{"items": []map[string]any{{
				"create_time": 1794540180.0,
				"metadata": map[string]any{
					"title": "n1", "url": "u1", "platform": "p1", "labels": map[string]any{"sentiment": "neu"},
				},
			}}}, false
		},
		"news_feed_get_exchange_announcements": func(args map[string]any) (map[string]any, bool) {
			return map[string]any{"items": []map[string]any{}}, false
		},
		"news_feed_get_social_sentiment": func(args map[string]any) (map[string]any, bool) {
			return map[string]any{
				"coin": "BTC", "overall_sentiment": -0.289, "sentiment_label": "negative",
				"mention_count": 22,
				"sentiment_distribution": map[string]any{
					"positive_ratio": 0.27, "negative_ratio": 0.73, "neutral_ratio": 0.0,
				},
				"top_tweets": []map[string]any{},
			}, false
		},
	}}
	srv := httptest.NewServer(http.HandlerFunc(f.handler))
	t.Cleanup(srv.Close)
	s := newMarketTestServer(t, srv.URL)

	code, body := getJSON(t, s, "/api/market/overview")
	if code != 200 || body["fear_greed_index"] != float64(12) {
		t.Fatalf("overview: code=%d body=%v", code, body)
	}

	code, body = getJSON(t, s, "/api/market/fear-greed")
	if code != 200 || body["value"] != float64(12) || body["label"] != "Extreme Fear" {
		t.Fatalf("fear-greed: code=%d body=%v", code, body)
	}
	if _, ok := body["change_24h"]; !ok {
		t.Fatal("change_24h key must exist (null on cold start)")
	}

	code, body = getJSON(t, s, "/api/market/news?limit=5")
	if code != 200 {
		t.Fatalf("news code=%d", code)
	}
	items := body["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("news items=%v", items)
	}

	code, _ = getJSON(t, s, "/api/market/announcements?limit=5")
	if code != 200 {
		t.Fatalf("announcements code=%d", code)
	}

	code, body = getJSON(t, s, "/api/market/sentiment?coin=btc")
	if code != 200 || body["sentiment_label"] != "negative" {
		t.Fatalf("sentiment: code=%d body=%v", code, body)
	}

	code, body = getJSON(t, s, "/api/market/liquidations")
	if code != 200 {
		t.Fatalf("liquidations code=%d", code)
	}
	for _, key := range []string{"window_seconds", "window_start", "hotspots", "count"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("liquidations missing %q: %v", key, body)
		}
	}
	if body["window_seconds"] != float64(1800) {
		t.Fatalf("window_seconds = %v", body["window_seconds"])
	}
}

// 上游不可达 → 502 + 明确错误类别（不用 generic 隐藏语义）
func TestMarketHandlersUpstreamDown(t *testing.T) {
	s := newMarketTestServer(t, "http://127.0.0.1:9")

	for _, path := range []string{
		"/api/market/overview",
		"/api/market/fear-greed",
		"/api/market/news",
		"/api/market/announcements",
		"/api/market/sentiment",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)
		if w.Code != http.StatusBadGateway {
			t.Fatalf("%s: want 502, got %d", path, w.Code)
		}
		var body APIErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: decode: %v", path, err)
		}
		if !strings.HasPrefix(body.Error, "Gate MCP upstream") {
			t.Fatalf("%s: error should carry public category, got %q", path, body.Error)
		}
	}
}

// 断链不影响 /api/symbols（Gate REST 直连路径）与清算端点（本地窗口）
func TestMarketLiquidationsLiveWithoutMCP(t *testing.T) {
	s := newMarketTestServer(t, "http://127.0.0.1:9")
	code, body := getJSON(t, s, "/api/market/liquidations")
	if code != 200 {
		t.Fatalf("liquidations must not depend on MCP, code=%d", code)
	}
	if body["hotspots"].([]any) == nil {
		t.Fatal("hotspots should be empty list, not null")
	}
}

// 非法 coin → 400
func TestMarketSentimentInvalidCoin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("upstream should not be called for invalid coin")
	}))
	t.Cleanup(srv.Close)
	s := newMarketTestServer(t, srv.URL)

	req := httptest.NewRequest(http.MethodGet, "/api/market/sentiment?coin=BAD_COIN!", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid coin: want 400, got %d", w.Code)
	}
}

// clampLimit 纯函数：缺省/非法回默认，范围 [1,50]
func TestClampLimit(t *testing.T) {
	cases := []struct {
		raw  string
		def  int
		want int
	}{
		{"", 20, 20},
		{"abc", 20, 20},
		{"0", 20, 20},
		{"-5", 20, 20},
		{"5", 20, 5},
		{"100", 20, 50},
		{"", 10, 10},
		{"100", 10, 50},
	}
	for _, c := range cases {
		if got := clampLimit(c.raw, c.def); got != c.want {
			t.Fatalf("clampLimit(%q,%d) = %d, want %d", c.raw, c.def, got, c.want)
		}
	}
}
