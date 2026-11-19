package kernel

import (
	"fmt"
	"strings"

	"nofx/store"
)

// MarketEnvSnapshot 市场环境上下文快照：决策循环按策略勾选拉取并组装，
// 注入 User Prompt 的「市场环境上下文」节。数据缺失/取数失败以 Unavailable 标记，
// 注入端输出占位行而非阻断决策（可选上下文语义）。
type MarketEnvSnapshot struct {
	// 市场总览
	TotalMarketCap     float64
	MarketCapChange24h float64
	TotalVolume24h     float64
	BTCPrice           float64
	BTCDominance       float64
	ETHDominance       float64
	AltcoinSeasonIndex float64

	// 恐惧贪婪
	FearGreedValue     float64
	FearGreedLabel     string
	FearGreedChange24h *float64 // 冷启动不足 24h 为 nil，注入端省略变化

	// 新闻（BTC 标题）与社交情绪
	NewsTitles       []string
	OverallSentiment float64
	SentimentLabel   string

	// 各数据源不可用标记（key: overview / fear_greed / news_sentiment）
	Unavailable map[string]bool
}

// marketEnvSectionTokenLimit 注入节软上限（token），与 EstimateTokens 口径一致：
// 中文按 len/2、英文按 len/4 换算——此处统一以英文字符 len/4 保守估算后控制
const marketEnvSectionTokenLimit = 600

// WriteMarketEnv 把市场环境上下文节写入 sb（锚点：Current Positions 之后、Candidate Coins 之前）。
// 按策略勾选输出对应子段；勾选但取数失败的源输出 Unavailable 占位；中英文按 language。
func WriteMarketEnv(sb *strings.Builder, snap *MarketEnvSnapshot, cfg store.StrategyConfig, lang Language) {
	if snap == nil || !cfg.MarketContext.MarketContextEnabled() || cfg.StrategyType == "grid_trading" {
		return
	}

	zh := lang == LangChinese
	label := func(en, zhText string) string {
		if zh {
			return zhText
		}
		return en
	}

	var section strings.Builder
	if cfg.MarketContext.Overview {
		if snap.Unavailable["overview"] {
			section.WriteString(label(
				"- Market Overview: [unavailable]\n",
				"- 市场总览: [数据暂不可用]\n",
			))
		} else {
			section.WriteString(fmt.Sprintf(
				label(
					"- Market Overview: cap $%.0f (%+.1f%% 24h) | vol $%.0f | BTC $%.0f (dom %.1f%%) | ETH dom %.1f%% | altcoin season %.0f\n",
					"- 市场总览: 总市值 $%.0f（24h %+.1f%%）| 成交 $%.0f | BTC $%.0f（占比 %.1f%%）| ETH 占比 %.1f%% | 山寨季指数 %.0f\n",
				),
				snap.TotalMarketCap, snap.MarketCapChange24h, snap.TotalVolume24h,
				snap.BTCPrice, snap.BTCDominance, snap.ETHDominance, snap.AltcoinSeasonIndex,
			))
		}
	}

	if cfg.MarketContext.FearGreed {
		if snap.Unavailable["fear_greed"] {
			section.WriteString(label(
				"- Fear & Greed: [unavailable]\n",
				"- 恐惧贪婪: [数据暂不可用]\n",
			))
		} else {
			changePart := ""
			if snap.FearGreedChange24h != nil {
				changePart = fmt.Sprintf(" (%+.0f 24h)", *snap.FearGreedChange24h)
			}
			section.WriteString(fmt.Sprintf(
				label(
					"- Fear & Greed: %.0f %s%s\n",
					"- 恐惧贪婪: %.0f %s%s\n",
				),
				snap.FearGreedValue, snap.FearGreedLabel, changePart,
			))
		}
	}

	if cfg.MarketContext.NewsSentiment {
		if snap.Unavailable["news_sentiment"] {
			section.WriteString(label(
				"- News & Sentiment (BTC): [unavailable]\n",
				"- 新闻与情绪 (BTC): [数据暂不可用]\n",
			))
		} else {
			section.WriteString(label(
				fmt.Sprintf("- News & Sentiment (BTC): sentiment %.2f %s\n", snap.OverallSentiment, snap.SentimentLabel),
				fmt.Sprintf("- 新闻与情绪 (BTC): 情绪分 %.2f %s\n", snap.OverallSentiment, snap.SentimentLabel),
			))
			for _, title := range snap.NewsTitles {
				section.WriteString(label(
					fmt.Sprintf("  * %s\n", title),
					fmt.Sprintf("  * %s\n", title),
				))
			}
		}
	}

	if section.Len() == 0 {
		return
	}

	text := section.String()

	// 软上限截断：先减新闻条目再减总览字段（英文 len/4 保守估算，中文按 len/2 更精确）
	limitChars := marketEnvSectionTokenLimit * 4
	if zh {
		limitChars = marketEnvSectionTokenLimit * 2
	}
	if len([]rune(text)) > limitChars {
		text = truncateMarketEnv(text, limitChars, zh)
	}

	sb.WriteString(label(
		"## Market Environment Context\n\n",
		"## 市场环境上下文\n\n",
	))
	sb.WriteString(text)
	sb.WriteString("\n")
}

// truncateMarketEnv 超限截断：先丢新闻条目，仍超则按字符截断并在行尾标注 [truncated]
func truncateMarketEnv(text string, limitChars int, zh bool) string {
	lines := strings.Split(text, "\n")
	// 从后往前丢新闻行（"  * " 开头）
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.HasPrefix(lines[i], "  * ") {
			lines = append(lines[:i], lines[i+1:]...)
			break
		}
	}
	text = strings.Join(lines, "\n")
	if len([]rune(text)) <= limitChars {
		return text
	}

	runes := []rune(text)
	out := string(runes[:limitChars])
	if zh {
		return out + "…[truncated]"
	}
	return out + "...[truncated]"
}
