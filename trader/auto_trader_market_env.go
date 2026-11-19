package trader

import (
	"context"
	"sync"

	"nofx/kernel"
	"nofx/logger"
	"nofx/store"
	"nofx/provider/gatemcp"
)

// fetchMarketEnvSnapshot 按策略勾选拉取市场环境数据（走 C1 TTL 缓存，各源并行），
// 任一源失败仅标记 Unavailable，不阻断决策（可选上下文语义）。
// 全不勾选时调用方不会进入本函数（零 gatemcp 触达）。
// 并发纪律：各 goroutine 只写自己的局部 failed bool 与互不相交的结构体标量字段；
// Unavailable map 由主 goroutine 在 wg.Wait() 之后单写者合并（避免并发 map 写 fatal）。
func (at *AutoTrader) fetchMarketEnvSnapshot(ctx context.Context, mc store.MarketContextConfig) *kernel.MarketEnvSnapshot {
	snap := &kernel.MarketEnvSnapshot{Unavailable: map[string]bool{}}

	client := gatemcp.Shared()
	var wg sync.WaitGroup
	var overviewFailed, fearGreedFailed, newsFailed bool

	if mc.Overview {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ov, err := client.MarketOverview(ctx)
			if err != nil {
				logger.Warnf("🌐 [%s] market overview unavailable: %v", at.name, err)
				overviewFailed = true
				return
			}
			snap.TotalMarketCap = ov.TotalMarketCap
			snap.MarketCapChange24h = ov.MarketCapChange24h
			snap.TotalVolume24h = ov.TotalVolume24h
			snap.BTCPrice = ov.BTCPrice
			snap.BTCDominance = ov.BTCDominance
			snap.ETHDominance = ov.ETHDominance
			snap.AltcoinSeasonIndex = ov.AltcoinSeasonIndex
		}()
	}

	if mc.FearGreed {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fg, err := client.FearGreed(ctx)
			if err != nil {
				logger.Warnf("🌐 [%s] fear & greed unavailable: %v", at.name, err)
				fearGreedFailed = true
				return
			}
			snap.FearGreedValue = fg.Value
			snap.FearGreedLabel = fg.Label
			snap.FearGreedChange24h = fg.Change24h
		}()
	}

	if mc.NewsSentiment {
		wg.Add(1)
		go func() {
			defer wg.Done()
			failed := false

			news, err := client.News(ctx, 5)
			if err != nil {
				logger.Warnf("🌐 [%s] BTC news unavailable: %v", at.name, err)
				failed = true
			} else {
				for _, item := range news {
					snap.NewsTitles = append(snap.NewsTitles, item.Title)
				}
			}

			sentiment, err := client.SocialSentiment(ctx, "BTC")
			if err != nil {
				logger.Warnf("🌐 [%s] social sentiment unavailable: %v", at.name, err)
				failed = true
			} else {
				snap.OverallSentiment = sentiment.OverallSentiment
				snap.SentimentLabel = sentiment.SentimentLabel
			}

			newsFailed = failed
		}()
	}

	wg.Wait()

	// 主 goroutine 单写者合并失败标记（wg.Wait 提供 happens-before）
	if overviewFailed {
		snap.Unavailable["overview"] = true
	}
	if fearGreedFailed {
		snap.Unavailable["fear_greed"] = true
	}
	if newsFailed {
		snap.Unavailable["news_sentiment"] = true
	}

	return snap
}
