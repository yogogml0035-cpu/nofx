package gatemcp

import (
	"encoding/json"
	"strings"
	"testing"
)

// 裁剪器以 research/ 探针实证的真实返回做断言（字段名/形态）

func mustMap(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("sample JSON: %v", err)
	}
	return m
}

func TestTrimOverview(t *testing.T) {
	// 探针实证 structuredContent（2026-11-19）
	ov, err := trimOverview(mustMap(t, `{
		"total_market_cap":1283843560000,"market_cap_change_24h":0.3,
		"total_volume_24h":220887122000,"btc_dominance":58.93,"eth_dominance":10.13,
		"btc_price":63747.1,"fear_greed_index":12,"fear_greed_label":"Extreme Fear",
		"ahr999":0.2961,"altcoin_season_index":57,"active_coins_count":3153,
		"updated_at":"2026-11-19T07:00:00Z","duration_ms":659
	}`))
	if err != nil {
		t.Fatalf("trimOverview: %v", err)
	}
	if ov.FearGreedIndex != 12 || ov.FearGreedLabel != "Extreme Fear" {
		t.Fatalf("fear greed fields: %+v", ov)
	}
	if ov.BTCDominance != 58.93 || ov.TotalMarketCap != 1283843560000 {
		t.Fatalf("market fields: %+v", ov)
	}
	b, _ := json.Marshal(ov)
	if strings.Contains(string(b), "duration_ms") {
		t.Fatal("internal duration_ms must not be exposed")
	}
}

func TestTrimNewsItems(t *testing.T) {
	// coin-only search_news 探针实证条目形态（metadata 内嵌 title/url/platform/labels.sentiment）
	items, err := trimNewsItems(mustMap(t, `{
		"coin":"BTC","total":1,"count":1,"items":[{
			"app_id":"data_processer","create_time":1794540180,"distance":0,"id":"28606163",
			"metadata":{
				"article_id":"28606163","business":"news","create_time":"2026-11-13 11:23:00",
				"labels":{"categories":["市场"],"sentiment":"neu","tickers":["BTC"]},
				"lang":"zh","platform":"followin","platform_type":"website",
				"title":"过去1小时全网爆仓2.25亿美元，多单爆仓1.74亿美元",
				"total_score":80,"url":"https://followin.io/zh-Hans/feed/28606163"
			},
			"score":0.5,"text":"火星财经消息，11 月 13 日……"
		}]
	}`))
	if err != nil {
		t.Fatalf("trimNewsItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d", len(items))
	}
	it := items[0]
	if it.Title != "过去1小时全网爆仓2.25亿美元，多单爆仓1.74亿美元" ||
		it.URL != "https://followin.io/zh-Hans/feed/28606163" ||
		it.Source != "followin" || it.PublishedAt != 1794540180 || it.Sentiment != "neu" {
		t.Fatalf("trimmed item mismatch: %+v", it)
	}
}

func TestTrimAnnouncements(t *testing.T) {
	// 运行时探针实证：publish_time 为数字字符串形态
	items, err := trimAnnouncements(mustMap(t, `{
		"limit":1,"total":1,"count":1,"items":[{
			"classify_name":"","coin_name":"","content":"The CLARITY Act was meant to……",
			"create_time":"2026-11-19 23:31:14","id":"18981","lang":"en",
			"platform":"btse","platform_type":"exchange","publish_time":"1795131074",
			"title":"CLARITY Act Stalls: What Crypto Regulation Looks Like in 2027",
			"url":"https://www.btse.com/blog/clarity-act-stalled-crypto-regulation-2027/"
		}]
	}`))
	if err != nil {
		t.Fatalf("trimAnnouncements: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d", len(items))
	}
	it := items[0]
	if it.Title != "CLARITY Act Stalls: What Crypto Regulation Looks Like in 2027" ||
		it.Source != "btse" || it.PublishedAt != 1795131074 || it.Sentiment != "" {
		t.Fatalf("trimmed announcement mismatch: %+v", it)
	}
	// 长正文不外发
	b, _ := json.Marshal(items)
	if strings.Contains(string(b), "CLARITY Act was meant") {
		t.Fatal("announcement full content must not be exposed")
	}
}

func TestTrimSentiment(t *testing.T) {
	s, err := trimSentiment(mustMap(t, `{
		"coin":"BTC","time_range":"24h","overall_sentiment":-0.28899999,
		"sentiment_label":"negative","mention_count":22,
		"sentiment_distribution":{"positive_ratio":0.272727,"negative_ratio":0.727273,"neutral_ratio":0,"positive_count":6,"negative_count":16},
		"top_tweets":[{
			"author":"https://x.com/i/user/3222562058","comments_num":44,
			"content":"ETFs sold $306,000,000 in $BTC and 39,830,000 in $ETH yesterday.",
			"created_time":"2026-11-19 07:15:21","forwards_num":17,"likes_num":208,
			"sentiment":0,"tweet_id":"2123314166589190349",
			"url":"https://x.com/TedPillows/status/2123314166589190349"
		}]
	}`))
	if err != nil {
		t.Fatalf("trimSentiment: %v", err)
	}
	if s.Coin != "BTC" || s.OverallSentiment != -0.28899999 || s.SentimentLabel != "negative" || s.MentionCount != 22 {
		t.Fatalf("sentiment scalars: %+v", s)
	}
	if s.PositiveRatio != 0.272727 || s.NegativeRatio != 0.727273 || s.NeutralRatio != 0 {
		t.Fatalf("ratios: %+v", s)
	}
	if len(s.TopTweets) != 1 || s.TopTweets[0].Likes != 208 ||
		s.TopTweets[0].URL != "https://x.com/TedPillows/status/2123314166589190349" {
		t.Fatalf("tweets: %+v", s.TopTweets)
	}
}

// 裁剪视图 JSON 字段名（API 契约）
func TestViewJSONFieldNames(t *testing.T) {
	b, _ := json.Marshal(FearGreed{})
	for _, key := range []string{"value", "label", "change_24h", "first_seen_at", "updated_at"} {
		if !strings.Contains(string(b), `"`+key+`"`) {
			t.Fatalf("FearGreed JSON missing %q: %s", key, b)
		}
	}
}

// flexInt64：数字/数字字符串均可；异常形态按 0（不 fail 整批）
func TestFlexInt64(t *testing.T) {
	cases := map[string]flexInt64{
		`123`:         123,
		`"456"`:       456,
		`""`:          0,
		`"not-a-num"`: 0,
		`null`:        0,
	}
	for in, want := range cases {
		var got flexInt64
		if err := json.Unmarshal([]byte(in), &got); err != nil {
			t.Fatalf("flexInt64(%s): %v", in, err)
		}
		if got != want {
			t.Fatalf("flexInt64(%s) = %d, want %d", in, got, want)
		}
	}
}
