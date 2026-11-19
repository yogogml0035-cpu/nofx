// Package gatemcp 封装 Gate MCP 免认证只读端点（/mcp/info、/mcp/news）的取数能力。
// 边界（父任务 D5）：仅免认证只读；不接 /mcp/exchange、/mcp/dex。
package gatemcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// 默认端点基址（GATEMCP_ENDPOINT 可覆盖，断链验收与容器环境用）
const DefaultEndpoint = "https://api.gatemcp.ai"

// 可公开的错误类别（API 502 响应体只含类别摘要；完整错误仅进服务日志）
const (
	ErrUnreachable = "Gate MCP upstream unreachable"
	ErrUpstream    = "Gate MCP upstream error"
	ErrTool        = "Gate MCP tool returned error"
)

// UpstreamError 携带可公开类别的上游错误；handler 据此返回 502
type UpstreamError struct {
	Category string
	Err      error
}

func (e *UpstreamError) Error() string { return e.Category + ": " + e.Err.Error() }
func (e *UpstreamError) Unwrap() error { return e.Err }

func unreachable(err error) error { return &UpstreamError{Category: ErrUnreachable, Err: err} }

func upstreamErr(err error) error { return &UpstreamError{Category: ErrUpstream, Err: err} }

func toolErr(err error) error { return &UpstreamError{Category: ErrTool, Err: err} }

// classify 把上游错误归入可公开类别
func classify(err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) ||
		errors.Is(err, syscall.ECONNREFUSED) {
		return unreachable(err)
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Timeout() {
		return unreachable(err)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return unreachable(err)
	}
	return upstreamErr(err)
}

const (
	infoPath = "/mcp/info"
	newsPath = "/mcp/news"

	kindInfo = "info"
	kindNews = "news"

	connectTimeout = 15 * time.Second
)

// Client 封装 /mcp/info 与 /mcp/news 两个 MCP server 连接（懒初始化）。
// 会话协议/网络错误即 Close 丢弃，下次取数时重建；取数统一走 TTL 缓存（fetchers.go）。
type Client struct {
	endpoint   string
	logger     *slog.Logger
	httpClient *http.Client

	mu          sync.Mutex
	infoSession *mcp.ClientSession
	newsSession *mcp.ClientSession

	overview  *cacheEntry[MarketOverview]
	news      *cacheEntry[[]NewsItem]
	anncs     *cacheEntry[[]NewsItem]
	sentiment *sentimentCache
	fgHistory *fearGreedHistory
}

var (
	sharedOnce sync.Once
	shared     *Client
)

// Shared 进程内单例；端点取 GATEMCP_ENDPOINT 或默认值
func Shared() *Client {
	sharedOnce.Do(func() {
		shared = New(endpointFromEnv())
	})
	return shared
}

func endpointFromEnv() string {
	if ep := strings.TrimSpace(os.Getenv("GATEMCP_ENDPOINT")); ep != "" {
		return ep
	}
	return DefaultEndpoint
}

// New 创建 Client（endpoint 为基址、不含 /mcp/xxx 路径；测试注入 httptest 地址）
func New(endpoint string) *Client {
	c := &Client{
		endpoint:   strings.TrimRight(endpoint, "/"),
		logger:     slog.Default(),
		httpClient: &http.Client{Timeout: 60 * time.Second},
		fgHistory:  newFearGreedHistory(),
	}
	c.overview = &cacheEntry[MarketOverview]{
		name:  "overview",
		ttl:   overviewTTL,
		fetch: c.fetchOverview,
	}
	c.news = &cacheEntry[[]NewsItem]{
		name:  "news",
		ttl:   newsTTL,
		fetch: c.fetchNews,
	}
	c.anncs = &cacheEntry[[]NewsItem]{
		name:  "announcements",
		ttl:   newsTTL,
		fetch: c.fetchAnnouncements,
	}
	c.sentiment = newSentimentCache(c.newSentimentEntry)
	return c
}

// Logger 覆盖 SDK 日志器（测试静音或运行时定制用）
func (c *Client) Logger(l *slog.Logger) *Client {
	c.logger = l
	return c
}

// Close 关闭两个会话（进程优雅退出用；幂等）
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.infoSession != nil {
		_ = c.infoSession.Close()
		c.infoSession = nil
	}
	if c.newsSession != nil {
		_ = c.newsSession.Close()
		c.newsSession = nil
	}
}

// session 取懒初始化的会话（ClientSession 并发安全，调用不持锁）
func (c *Client) session(kind string) (*mcp.ClientSession, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if kind == kindInfo {
		if c.infoSession != nil {
			return c.infoSession, nil
		}
		s, err := c.connect(c.endpoint + infoPath)
		if err != nil {
			return nil, err
		}
		c.infoSession = s
		return s, nil
	}
	if c.newsSession != nil {
		return c.newsSession, nil
	}
	s, err := c.connect(c.endpoint + newsPath)
	if err != nil {
		return nil, err
	}
	c.newsSession = s
	return s, nil
}

// connect 建立会话；SDK 会话生命周期独立于本 ctx（仅约束 initialize 握手）
func (c *Client) connect(endpoint string) (*mcp.ClientSession, error) {
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	transport := &mcp.StreamableClientTransport{Endpoint: endpoint, HTTPClient: c.httpClient}
	client := mcp.NewClient(&mcp.Implementation{Name: "nofx", Version: "0.1.0"}, &mcp.ClientOptions{Logger: c.logger})
	s, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, classify(err)
	}
	return s, nil
}

// invalidate 仅当仍是当前会话时丢弃（防止误关并发方刚重建的新会话）
func (c *Client) invalidate(kind string, s *mcp.ClientSession) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if kind == kindInfo {
		if c.infoSession == s && s != nil {
			c.infoSession = nil
			_ = s.Close()
		}
		return
	}
	if c.newsSession == s && s != nil {
		c.newsSession = nil
		_ = s.Close()
	}
}

// callTool 调用指定 server 上的工具，返回 structuredContent。
// 协议/网络错误丢弃会话并归类返回；isError 是工具级错误（会话仍健康，仅归类）。
func (c *Client) callTool(ctx context.Context, kind, tool string, args map[string]any) (map[string]any, error) {
	s, err := c.session(kind)
	if err != nil {
		return nil, err
	}
	res, err := s.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		c.invalidate(kind, s)
		return nil, classify(err)
	}
	if res == nil {
		return nil, upstreamErr(errors.New("empty tool result"))
	}
	if res.IsError {
		return nil, toolErr(errors.New(firstText(res.Content)))
	}

	if sc, ok := res.StructuredContent.(map[string]any); ok {
		return sc, nil
	}
	// structuredContent 缺失时回退 content[0].text（探针实证两者同形）
	if txt := firstText(res.Content); txt != "" {
		var m map[string]any
		if err := json.Unmarshal([]byte(txt), &m); err != nil {
			return nil, upstreamErr(fmt.Errorf("decode tool content: %w", err))
		}
		return m, nil
	}
	return nil, upstreamErr(errors.New("tool result has no structuredContent"))
}

func firstText(contents []mcp.Content) string {
	for _, ct := range contents {
		if tc, ok := ct.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}
