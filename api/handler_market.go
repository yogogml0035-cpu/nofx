package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"nofx/logger"
	"nofx/provider/gate"
	"nofx/provider/gatemcp"
)

// Gate 市场数据公开端点（免鉴权只读，姿态与 /api/symbols 一致）。
// 上游（Gate MCP / Gate REST）失败一律 502 + 可公开错误类别摘要
// （不用 SafeInternalError 的 generic 隐藏语义——前端需要明确的错误态），
// 完整错误进服务日志。
//
// 测试注入：gateMCP 字段非 nil 时优先于进程单例（api 测试不触网）。

// handleMarketOverview GET /api/market/overview — 市场总览
func (s *Server) handleMarketOverview(c *gin.Context) {
	ov, err := s.marketClient().MarketOverview(c.Request.Context())
	if err != nil {
		upstreamError(c, "Get market overview", err)
		return
	}
	c.JSON(http.StatusOK, ov)
}

// handleMarketFearGreed GET /api/market/fear-greed — 恐惧贪婪（change_24h 后端派生，冷启动 null）
func (s *Server) handleMarketFearGreed(c *gin.Context) {
	fg, err := s.marketClient().FearGreed(c.Request.Context())
	if err != nil {
		upstreamError(c, "Get fear & greed", err)
		return
	}
	c.JSON(http.StatusOK, fg)
}

// handleMarketNews GET /api/market/news?limit= — BTC 新闻 top N（默认 20 上限 50）
func (s *Server) handleMarketNews(c *gin.Context) {
	limit := clampLimit(c.Query("limit"), 20)
	items, err := s.marketClient().News(c.Request.Context(), limit)
	if err != nil {
		upstreamError(c, "Get market news", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "count": len(items)})
}

// handleMarketAnnouncements GET /api/market/announcements?limit= — 市场级公告 top N（默认 10 上限 50）
func (s *Server) handleMarketAnnouncements(c *gin.Context) {
	limit := clampLimit(c.Query("limit"), 10)
	items, err := s.marketClient().Announcements(c.Request.Context(), limit)
	if err != nil {
		upstreamError(c, "Get market announcements", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "count": len(items)})
}

// handleMarketSentiment GET /api/market/sentiment?coin= — 社交情绪（coin 默认 BTC）
func (s *Server) handleMarketSentiment(c *gin.Context) {
	coin := gatemcp.NormalizeCoin(c.DefaultQuery("coin", "BTC"))
	if !gatemcp.ValidCoin(coin) {
		SafeBadRequest(c, "invalid coin parameter")
		return
	}
	sentiment, err := s.marketClient().SocialSentiment(c.Request.Context(), coin)
	if err != nil {
		upstreamError(c, "Get social sentiment", err)
		return
	}
	c.JSON(http.StatusOK, sentiment)
}

// handleMarketLiquidations GET /api/market/liquidations — 清算热点 top 20（30 分钟滚动窗口）
func (s *Server) handleMarketLiquidations(c *gin.Context) {
	now := time.Now()
	hotspots := gate.LiquidationHotspots(gate.LiquidationWindow, gate.LiquidationTopN)
	resp := gin.H{
		"window_seconds": int(gate.LiquidationWindow.Seconds()),
		"window_start":   now.Add(-gate.LiquidationWindow).Format(time.RFC3339),
		"hotspots":       hotspots,
		"count":          len(hotspots),
	}
	if last, ok := gate.LastLiquidationPull(); ok {
		resp["updated_at"] = last.Format(time.RFC3339)
	}
	c.JSON(http.StatusOK, resp)
}

// marketClient 测试可注入的取数客户端（nil 时用进程单例）
func (s *Server) marketClient() *gatemcp.Client {
	if s.gateMCP != nil {
		return s.gateMCP
	}
	return gatemcp.Shared()
}

// upstreamError 上游失败 → 502 + 可公开类别摘要；完整错误进服务日志
func upstreamError(c *gin.Context, op string, err error) {
	var ue *gatemcp.UpstreamError
	if errors.As(err, &ue) {
		logger.Errorf("[Market API] %s: %v", op, err)
		writeAPIError(c, http.StatusBadGateway, ue.Category, "", nil)
		return
	}
	SafeInternalError(c, op, err)
}

// clampLimit 解析 limit 参数：非法/缺省用 def，范围 [1, 50]
func clampLimit(raw string, def int) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	if n > 50 {
		return 50
	}
	return n
}
