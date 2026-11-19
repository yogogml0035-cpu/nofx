package gatemcp

import (
	"context"
	"regexp"
	"strings"
	"time"
)

// 工具调用参数为 research/ 探针定稿：
// - news 仅传 coin 不传 query（query 任何取值实测全空）
// - 一次拉满 50（探针实测可用），按请求切片，缓存不按 limit 分键
const (
	overviewTTL = 5 * time.Minute
	newsTTL     = 15 * time.Minute

	newsTool          = "news_feed_search_news"
	announcementsTool = "news_feed_get_exchange_announcements"
	sentimentTool     = "news_feed_get_social_sentiment"
	overviewTool      = "info_marketsnapshot_get_market_overview"

	fetchNewsLimit = 50
	fetchAnnLimit  = 50

	defaultNewsCoin = "BTC"
)

var coinPattern = regexp.MustCompile(`^[A-Za-z0-9]{1,16}$`)

// ValidCoin 校验 coin 参数（handler 层用；非法值应在入口 400）
func ValidCoin(coin string) bool {
	return coinPattern.MatchString(coin)
}

// NormalizeCoin 规范化 coin 参数（大写、去空白）
func NormalizeCoin(coin string) string {
	return strings.ToUpper(strings.TrimSpace(coin))
}

func (c *Client) fetchOverview(ctx context.Context) (MarketOverview, error) {
	m, err := c.callTool(ctx, kindInfo, overviewTool, map[string]any{})
	if err != nil {
		return MarketOverview{}, err
	}
	ov, err := trimOverview(m)
	if err != nil {
		return MarketOverview{}, upstreamErr(err)
	}
	// 恐惧贪婪 24h 变化的样本点：每次真实刷新追加（缓存命中不追加）
	c.fgHistory.add(time.Now(), ov.FearGreedIndex)
	return ov, nil
}

func (c *Client) fetchNews(ctx context.Context) ([]NewsItem, error) {
	m, err := c.callTool(ctx, kindNews, newsTool, map[string]any{
		"coin":  defaultNewsCoin,
		"limit": fetchNewsLimit,
	})
	if err != nil {
		return nil, err
	}
	items, err := trimNewsItems(m)
	if err != nil {
		return nil, upstreamErr(err)
	}
	return items, nil
}

func (c *Client) fetchAnnouncements(ctx context.Context) ([]NewsItem, error) {
	m, err := c.callTool(ctx, kindNews, announcementsTool, map[string]any{
		"limit": fetchAnnLimit,
	})
	if err != nil {
		return nil, err
	}
	items, err := trimAnnouncements(m)
	if err != nil {
		return nil, upstreamErr(err)
	}
	return items, nil
}

func (c *Client) newSentimentEntry(coin string) *cacheEntry[Sentiment] {
	return &cacheEntry[Sentiment]{
		name: "sentiment:" + coin,
		ttl:  newsTTL,
		fetch: func(ctx context.Context) (Sentiment, error) {
			m, err := c.callTool(ctx, kindNews, sentimentTool, map[string]any{"coin": coin})
			if err != nil {
				return Sentiment{}, err
			}
			s, err := trimSentiment(m)
			if err != nil {
				return Sentiment{}, upstreamErr(err)
			}
			return s, nil
		},
	}
}

// MarketOverview 市场总览（TTL 5min；含恐惧贪婪字段）
func (c *Client) MarketOverview(ctx context.Context) (*MarketOverview, error) {
	v, _, err := c.overview.get(ctx)
	if err != nil {
		return nil, err
	}
	ov := v
	return &ov, nil
}

// FearGreed 恐惧贪婪（从 overview 缓存切片，零额外上游调用）；
// change_24h 由进程内 25h 历史环派生，冷启动不足 24h 为 null。
func (c *Client) FearGreed(ctx context.Context) (*FearGreed, error) {
	v, _, err := c.overview.get(ctx)
	if err != nil {
		return nil, err
	}
	fg := FearGreed{
		Value:     v.FearGreedIndex,
		Label:     v.FearGreedLabel,
		UpdatedAt: v.UpdatedAt,
	}
	if ch, ok := c.fgHistory.change24h(time.Now(), v.FearGreedIndex); ok {
		fg.Change24h = &ch
	}
	if t, ok := c.fgHistory.first(); ok {
		fg.FirstSeenAt = &t
	}
	return &fg, nil
}

// News BTC 新闻 top limit（上游一次拉满 50，此处切片；limit ≤ 0 返回空）
func (c *Client) News(ctx context.Context, limit int) ([]NewsItem, error) {
	v, _, err := c.news.get(ctx)
	if err != nil {
		return nil, err
	}
	if limit < 0 {
		limit = 0
	}
	if limit > len(v) {
		limit = len(v)
	}
	return v[:limit], nil
}

// Announcements 市场级公告/标题 top limit
func (c *Client) Announcements(ctx context.Context, limit int) ([]NewsItem, error) {
	v, _, err := c.anncs.get(ctx)
	if err != nil {
		return nil, err
	}
	if limit < 0 {
		limit = 0
	}
	if limit > len(v) {
		limit = len(v)
	}
	return v[:limit], nil
}

// SocialSentiment 社交情绪（coin 必传，已规范化；按 coin 分键缓存）
func (c *Client) SocialSentiment(ctx context.Context, coin string) (*Sentiment, error) {
	v, _, err := c.sentiment.entry(coin).get(ctx)
	if err != nil {
		return nil, err
	}
	s := v
	return &s, nil
}
