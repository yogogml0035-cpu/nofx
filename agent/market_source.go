package agent

// 助手行情汇聚层（子任务 15）：助手侧所有行情取数只走 Gate（D1/D6）。
// 包级变量是测试缝：agent 包测试必须保持非并行（t.Parallel 会造成竞态）。

import (
	"encoding/json"
	"fmt"
	"strings"

	"nofx/market"
)

var (
	assistantGetTicker   = market.GetGateTicker
	assistantListTickers = market.ListGateTickers
	assistantGetKlines   = market.GetGateKlines
	assistantGetMetrics  = market.GetGateContractMetrics
)

// assistantCryptoSymbol 把模型/用户给的币种写法归一为内部符号：BTC → BTCUSDT，
// BTC_USDT / BTC-USDT / BTC/USDT → BTCUSDT。
func assistantCryptoSymbol(raw string) string {
	s := strings.ToUpper(strings.TrimSpace(raw))
	s = strings.NewReplacer("_", "", "-", "", "/", "").Replace(s)
	s = strings.TrimSuffix(s, "SWAP")
	if s == "" {
		return ""
	}
	if !strings.HasSuffix(s, "USDT") {
		s += "USDT"
	}
	return s
}

// buildMarketSnapshot 纯函数：把 Gate ticker、合约指标与 K 线组装成助手的
// get_market_snapshot 输出。成交量统一为 USDT 成交额（D6），持仓量为 USD；
// Gate 不提供成交笔数，因此不输出 trade_count。
func buildMarketSnapshot(symbol, interval string, ticker market.GateTicker, metrics market.GateContractMetrics, klines []market.Kline) string {
	highestHigh, lowestLow := 0.0, 0.0
	firstClose, lastClose := 0.0, 0.0
	totalVolume := 0.0
	candles := make([]map[string]any, 0, len(klines))
	for i, k := range klines {
		if i == 0 {
			firstClose, highestHigh, lowestLow = k.Close, k.High, k.Low
		}
		if k.High > highestHigh {
			highestHigh = k.High
		}
		if lowestLow == 0 || (k.Low > 0 && k.Low < lowestLow) {
			lowestLow = k.Low
		}
		lastClose = k.Close
		totalVolume += k.Volume
		candles = append(candles, map[string]any{
			"open_time":  k.OpenTime,
			"open":       k.Open,
			"high":       k.High,
			"low":        k.Low,
			"close":      k.Close,
			"volume":     k.Volume, // USDT 成交额
			"close_time": k.CloseTime,
		})
	}
	periodChangePercent := 0.0
	if firstClose > 0 && lastClose > 0 {
		periodChangePercent = (lastClose - firstClose) / firstClose * 100
	}
	averageVolume := 0.0
	if len(klines) > 0 {
		averageVolume = totalVolume / float64(len(klines))
	}

	out, _ := json.Marshal(map[string]any{
		"symbol": symbol,
		"price":  ticker.Last,
		"ticker_24h": map[string]any{
			"price_change":         ticker.Change24h,
			"price_change_percent": ticker.ChangePercent24h,
			"high_price":           ticker.High24h,
			"low_price":            ticker.Low24h,
			"volume":               ticker.QuoteVolume24h,
			"quote_volume":         ticker.QuoteVolume24h,
			"volume_unit":          "USDT",
		},
		"perp_metrics": map[string]any{
			"mark_price":         metrics.MarkPrice,
			"index_price":        metrics.IndexPrice,
			"funding_rate":       metrics.FundingRate,
			"next_funding_time":  metrics.NextFundingTime, // 毫秒
			"open_interest":      metrics.OpenInterestUSD,
			"open_interest_unit": "USD",
		},
		"kline_snapshot": map[string]any{
			"interval":              interval,
			"limit":                 len(candles),
			"period_change_percent": periodChangePercent,
			"highest_high":          highestHigh,
			"lowest_low":            lowestLow,
			"average_volume":        averageVolume,
			"volume_unit":           "USDT",
			"recent_klines":         candles,
		},
	})
	return string(out)
}

// assistantMarketSummary 无 AI 兜底时的简洁行情摘要（数据来自 Gate 多周期取数）。
func assistantMarketSummary(data *market.Data) string {
	if data == nil {
		return ""
	}
	funding := "n/a"
	if data.FundingRate != nil {
		funding = fmt.Sprintf("%.4f%%", *data.FundingRate*100)
	}
	return fmt.Sprintf(
		"Price: $%.4f | 1h: %+.2f%% | 4h: %+.2f%% | RSI7: %.1f | EMA20: %.4f | MACD: %.6f | Funding: %s",
		data.CurrentPrice, data.PriceChange1h, data.PriceChange4h, data.CurrentRSI7,
		data.CurrentEMA20, data.CurrentMACD, funding)
}
