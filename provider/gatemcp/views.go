package gatemcp

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// 上游字段集以探针实证为准（任务目录 research/ 四份留档）：
// overview 无板块数据、无逐币 24h 涨跌；news coin-only 走 OpenSearch；
// announcements 为市场级新闻/公告混合流；sentiment coin 必传。
// 数值统一 float64 解码（上游指数类字段偶有小数形态）。

// MarketOverview 市场总览（含恐惧贪婪；duration_ms 等内部字段不外发）
type MarketOverview struct {
	TotalMarketCap     float64 `json:"total_market_cap"`
	MarketCapChange24h float64 `json:"market_cap_change_24h"`
	TotalVolume24h     float64 `json:"total_volume_24h"`
	BTCDominance       float64 `json:"btc_dominance"`
	ETHDominance       float64 `json:"eth_dominance"`
	BTCPrice           float64 `json:"btc_price"`
	FearGreedIndex     float64 `json:"fear_greed_index"`
	FearGreedLabel     string  `json:"fear_greed_label"`
	Ahr999             float64 `json:"ahr999"`
	AltcoinSeasonIndex float64 `json:"altcoin_season_index"`
	ActiveCoinsCount   float64 `json:"active_coins_count"`
	UpdatedAt          string  `json:"updated_at"`
}

// NewsItem 裁剪视图：标题/链接/来源/发布时间/情绪标签（公告流无情绪标签）
type NewsItem struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Source      string `json:"source"`
	PublishedAt int64  `json:"published_at"`
	Sentiment   string `json:"sentiment,omitempty"`
}

// FearGreed 恐惧贪婪视图；change_24h/first_seen_at 由进程内历史环派生（冷启动为 null）
type FearGreed struct {
	Value       float64    `json:"value"`
	Label       string     `json:"label"`
	Change24h   *float64   `json:"change_24h"`
	FirstSeenAt *time.Time `json:"first_seen_at"`
	UpdatedAt   string     `json:"updated_at"`
}

// Sentiment 社交情绪裁剪视图：整体分值 + 正负比例 + 高热度推文
type Sentiment struct {
	Coin             string  `json:"coin"`
	OverallSentiment float64 `json:"overall_sentiment"`
	SentimentLabel   string  `json:"sentiment_label"`
	MentionCount     int     `json:"mention_count"`
	PositiveRatio    float64 `json:"positive_ratio"`
	NegativeRatio    float64 `json:"negative_ratio"`
	NeutralRatio     float64 `json:"neutral_ratio"`
	TopTweets        []Tweet `json:"top_tweets"`
}

// Tweet 高热度推文裁剪视图
type Tweet struct {
	Author    string `json:"author"`
	Content   string `json:"content"`
	URL       string `json:"url"`
	CreatedAt string `json:"created_time"`
	Likes     int    `json:"likes_num"`
}

// decodeStruct structuredContent(map) → 目标结构（经 JSON 往返严格对齐字段名）
func decodeStruct(m map[string]any, target any) error {
	b, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("re-marshal structuredContent: %w", err)
	}
	if err := json.Unmarshal(b, target); err != nil {
		return fmt.Errorf("decode structuredContent: %w", err)
	}
	return nil
}

// ---- 上游原始结构 ----

// flexInt64 兼容上游时间戳的混合形态：JSON 数字或数字字符串（运行时探针实证
// announcements 的 publish_time 全量为数字字符串）。无法解析按 0（真实缺失，不造数）。
type flexInt64 int64

func (f *flexInt64) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		*f = 0 // 形态漂移容差：按缺失处理，不 fail 整批
		return nil
	}
	*f = flexInt64(v)
	return nil
}

type rawNewsEnvelope struct {
	Items []rawNewsItem `json:"items"`
}

type rawNewsItem struct {
	CreateTime flexInt64 `json:"create_time"`
	Metadata   struct {
		Title    string `json:"title"`
		URL      string `json:"url"`
		Platform string `json:"platform"`
		Labels   struct {
			Sentiment string `json:"sentiment"`
		} `json:"labels"`
	} `json:"metadata"`
}

type rawAnnouncementEnvelope struct {
	Items []rawAnnouncement `json:"items"`
}

type rawAnnouncement struct {
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Platform    string    `json:"platform"`
	PublishTime flexInt64 `json:"publish_time"`
}

type rawSentimentEnvelope struct {
	Coin                  string  `json:"coin"`
	OverallSentiment      float64 `json:"overall_sentiment"`
	SentimentLabel        string  `json:"sentiment_label"`
	MentionCount          int     `json:"mention_count"`
	SentimentDistribution struct {
		PositiveRatio float64 `json:"positive_ratio"`
		NegativeRatio float64 `json:"negative_ratio"`
		NeutralRatio  float64 `json:"neutral_ratio"`
	} `json:"sentiment_distribution"`
	TopTweets []rawTweet `json:"top_tweets"`
}

type rawTweet struct {
	Author      string `json:"author"`
	Content     string `json:"content"`
	URL         string `json:"url"`
	CreatedTime string `json:"created_time"`
	LikesNum    int    `json:"likes_num"`
}

// ---- 纯函数裁剪器 ----

func trimOverview(m map[string]any) (MarketOverview, error) {
	var ov MarketOverview
	if err := decodeStruct(m, &ov); err != nil {
		return MarketOverview{}, err
	}
	return ov, nil
}

// trimNewsItems 保持上游顺序裁剪（上游已按相关性/时间排序）
func trimNewsItems(m map[string]any) ([]NewsItem, error) {
	var raw rawNewsEnvelope
	if err := decodeStruct(m, &raw); err != nil {
		return nil, err
	}
	items := make([]NewsItem, 0, len(raw.Items))
	for _, it := range raw.Items {
		items = append(items, NewsItem{
			Title:       it.Metadata.Title,
			URL:         it.Metadata.URL,
			Source:      it.Metadata.Platform,
			PublishedAt: int64(it.CreateTime),
			Sentiment:   it.Metadata.Labels.Sentiment,
		})
	}
	return items, nil
}

func trimAnnouncements(m map[string]any) ([]NewsItem, error) {
	var raw rawAnnouncementEnvelope
	if err := decodeStruct(m, &raw); err != nil {
		return nil, err
	}
	items := make([]NewsItem, 0, len(raw.Items))
	for _, it := range raw.Items {
		items = append(items, NewsItem{
			Title:       it.Title,
			URL:         it.URL,
			Source:      it.Platform,
			PublishedAt: int64(it.PublishTime),
		})
	}
	return items, nil
}

func trimSentiment(m map[string]any) (Sentiment, error) {
	var raw rawSentimentEnvelope
	if err := decodeStruct(m, &raw); err != nil {
		return Sentiment{}, err
	}
	tweets := make([]Tweet, 0, len(raw.TopTweets))
	for _, t := range raw.TopTweets {
		tweets = append(tweets, Tweet{
			Author:    t.Author,
			Content:   t.Content,
			URL:       t.URL,
			CreatedAt: t.CreatedTime,
			Likes:     t.LikesNum,
		})
	}
	return Sentiment{
		Coin:             raw.Coin,
		OverallSentiment: raw.OverallSentiment,
		SentimentLabel:   raw.SentimentLabel,
		MentionCount:     raw.MentionCount,
		PositiveRatio:    raw.SentimentDistribution.PositiveRatio,
		NegativeRatio:    raw.SentimentDistribution.NegativeRatio,
		NeutralRatio:     raw.SentimentDistribution.NeutralRatio,
		TopTweets:        tweets,
	}, nil
}
