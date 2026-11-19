package gatemcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// fakeMCPServer 只 fake 上游返回结构（MCP Streamable HTTP 协议壳），不 fake 业务逻辑。
// GET 回 405（SDK 规范容忍）；tools/call 结果由 handler 决定。
type fakeMCPServer struct {
	toolResults map[string]func(args map[string]any) (map[string]any, bool) // 工具名 → structuredContent/isError
	initCount   atomic.Int32
	callCount   atomic.Int32
	failCalls   atomic.Int32 // >0 时前 N 次 tools/call 返回 HTTP 500（触发会话重建路径）
}

func (f *fakeMCPServer) handler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		// standalone SSE GET：规范允许 405，SDK 容忍
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
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
		f.initCount.Add(1)
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
		if n := f.failCalls.Load(); n > 0 && f.callCount.Add(1) <= n {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
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

func newFakeClient(t *testing.T, f *fakeMCPServer) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(f.handler))
	t.Cleanup(srv.Close)
	c := New(srv.URL)
	// 测试静音 SDK 日志
	c.logger = discardLogger()
	return c
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestClientFetchers(t *testing.T) {
	f := &fakeMCPServer{toolResults: map[string]func(args map[string]any) (map[string]any, bool){
		overviewTool: func(args map[string]any) (map[string]any, bool) {
			return map[string]any{
				"total_market_cap": 1283843560000.0, "market_cap_change_24h": 0.3,
				"total_volume_24h": 220887122000.0, "btc_dominance": 58.93, "eth_dominance": 10.13,
				"btc_price": 63747.1, "fear_greed_index": 12.0, "fear_greed_label": "Extreme Fear",
				"ahr999": 0.2961, "altcoin_season_index": 57.0, "active_coins_count": 3153.0,
				"updated_at": "2026-11-19T07:00:00Z",
			}, false
		},
		newsTool: func(args map[string]any) (map[string]any, bool) {
			// 定稿参数：仅 coin 不传 query
			if _, has := args["query"]; has {
				return nil, true
			}
			if args["coin"] != "BTC" {
				return nil, true
			}
			return map[string]any{"items": []map[string]any{{
				"create_time": 1794540180.0,
				"metadata": map[string]any{
					"title": "过去1小时全网爆仓2.25亿美元", "url": "https://followin.io/zh-Hans/feed/28606163",
					"platform": "followin", "labels": map[string]any{"sentiment": "neu"},
				},
			}}}, false
		},
		announcementsTool: func(args map[string]any) (map[string]any, bool) {
			return map[string]any{"items": []map[string]any{{
				"title": "CLARITY Act Stalls", "url": "https://www.btse.com/blog/x",
				"platform": "btse", "publish_time": 1795131074.0,
			}}}, false
		},
		sentimentTool: func(args map[string]any) (map[string]any, bool) {
			if args["coin"] == "" {
				return nil, true
			}
			return map[string]any{
				"coin": args["coin"], "overall_sentiment": -0.289, "sentiment_label": "negative",
				"mention_count": 22,
				"sentiment_distribution": map[string]any{
					"positive_ratio": 0.272727, "negative_ratio": 0.727273, "neutral_ratio": 0.0,
				},
				"top_tweets": []map[string]any{{
					"author": "https://x.com/i/user/3222562058", "content": "ETFs sold $306,000,000 in $BTC",
					"url": "https://x.com/TedPillows/status/1", "created_time": "2026-11-19 07:15:21", "likes_num": 208.0,
				}},
			}, false
		},
	}}
	c := newFakeClient(t, f)
	ctx := context.Background()

	ov, err := c.MarketOverview(ctx)
	if err != nil {
		t.Fatalf("MarketOverview: %v", err)
	}
	if ov.FearGreedIndex != 12 || ov.BTCPrice != 63747.1 {
		t.Fatalf("overview mismatch: %+v", ov)
	}

	items, err := c.News(ctx, 20)
	if err != nil || len(items) != 1 || items[0].Source != "followin" {
		t.Fatalf("News: items=%+v err=%v", items, err)
	}

	ann, err := c.Announcements(ctx, 10)
	if err != nil || len(ann) != 1 || ann[0].PublishedAt != 1795131074 {
		t.Fatalf("Announcements: items=%+v err=%v", ann, err)
	}

	sent, err := c.SocialSentiment(ctx, "BTC")
	if err != nil || sent.SentimentLabel != "negative" || len(sent.TopTweets) != 1 || sent.TopTweets[0].Likes != 208 {
		t.Fatalf("SocialSentiment: %+v err=%v", sent, err)
	}

	// FearGreed 从 overview 缓存切片 + 历史环派生（冷启动 change_24h 为 null）
	fg, err := c.FearGreed(ctx)
	if err != nil {
		t.Fatalf("FearGreed: %v", err)
	}
	if fg.Value != 12 || fg.Label != "Extreme Fear" {
		t.Fatalf("feargreed scalars: %+v", fg)
	}
	if fg.Change24h != nil {
		t.Fatalf("cold start change_24h must be null: %v", *fg.Change24h)
	}
	if fg.FirstSeenAt == nil {
		t.Fatal("first_seen_at should exist after first fetch")
	}

	// 恐惧贪婪样本已随真实刷新入环
	if got := f.initCount.Load(); got < 1 {
		t.Fatalf("initialize count = %d", got)
	}
}

// 上游不可达 → ErrUnreachable 类别（fast-fail，不造数据）
func TestClientUnreachable(t *testing.T) {
	c := New("http://127.0.0.1:9")
	c.logger = discardLogger()
	ctx := context.Background()

	for name, fn := range map[string]func() error{
		"overview":  func() error { _, err := c.MarketOverview(ctx); return err },
		"feargreed": func() error { _, err := c.FearGreed(ctx); return err },
		"news":      func() error { _, err := c.News(ctx, 5); return err },
		"anncs":     func() error { _, err := c.Announcements(ctx, 5); return err },
		"sentiment": func() error { _, err := c.SocialSentiment(ctx, "BTC"); return err },
	} {
		err := fn()
		var ue *UpstreamError
		if !errors.As(err, &ue) {
			t.Fatalf("%s: want UpstreamError, got %v", name, err)
		}
		if ue.Category != ErrUnreachable {
			t.Fatalf("%s: category = %q, want %q", name, ue.Category, ErrUnreachable)
		}
	}
}

// 工具级错误（isError=true）→ ErrTool 类别
func TestClientToolError(t *testing.T) {
	f := &fakeMCPServer{toolResults: map[string]func(args map[string]any) (map[string]any, bool){
		overviewTool: func(args map[string]any) (map[string]any, bool) {
			return map[string]any{"message": "upstream snapshot unavailable"}, true
		},
	}}
	c := newFakeClient(t, f)
	_, err := c.MarketOverview(context.Background())
	var ue *UpstreamError
	if !errors.As(err, &ue) || ue.Category != ErrTool {
		t.Fatalf("want ErrTool category, got %v", err)
	}
}

// 协议失败（HTTP 500）→ 会话丢弃重建；第二次取数重新 initialize
func TestClientSessionRebuild(t *testing.T) {
	f := &fakeMCPServer{toolResults: map[string]func(args map[string]any) (map[string]any, bool){
		overviewTool: func(args map[string]any) (map[string]any, bool) {
			return map[string]any{"fear_greed_index": 12.0, "fear_greed_label": "Fear"}, false
		},
	}}
	f.failCalls.Store(1)
	c := newFakeClient(t, f)
	ctx := context.Background()

	if _, err := c.MarketOverview(ctx); err == nil {
		t.Fatal("first call should fail (HTTP 500)")
	}
	if _, err := c.MarketOverview(ctx); err != nil {
		t.Fatalf("second call should rebuild and succeed: %v", err)
	}
	if got := f.initCount.Load(); got != 2 {
		t.Fatalf("initialize count = %d, want 2 (rebuild)", got)
	}
}

// News 切片：请求超过缓存条数时返回实际条数；0/negative 安全
func TestClientNewsSlice(t *testing.T) {
	f := &fakeMCPServer{toolResults: map[string]func(args map[string]any) (map[string]any, bool){
		newsTool: func(args map[string]any) (map[string]any, bool) {
			items := []map[string]any{}
			for i := 0; i < 3; i++ {
				items = append(items, map[string]any{
					"create_time": float64(1794540180 + i),
					"metadata": map[string]any{
						"title": "n", "url": "u", "platform": "p", "labels": map[string]any{"sentiment": "neu"},
					},
				})
			}
			return map[string]any{"items": items}, false
		},
	}}
	c := newFakeClient(t, f)
	ctx := context.Background()
	if got, err := c.News(ctx, 50); err != nil || len(got) != 3 {
		t.Fatalf("News(50) = %d items err=%v", len(got), err)
	}
	if got, err := c.News(ctx, 2); err != nil || len(got) != 2 {
		t.Fatalf("News(2) = %d items err=%v", len(got), err)
	}
	if got, err := c.News(ctx, 0); err != nil || len(got) != 0 {
		t.Fatalf("News(0) = %d items err=%v", len(got), err)
	}
}
