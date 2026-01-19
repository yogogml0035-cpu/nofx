package market

import (
	"context"
	"fmt"
	"nofx/logger"
	"nofx/provider/okx"
	"sort"
	"time"
)

// GetKlinesRange fetches K-line series within specified time range (closed interval), returns data sorted by time in ascending order.
// Now uses OKX API instead of Binance to avoid network issues
func GetKlinesRange(symbol string, timeframe string, start, end time.Time) ([]Kline, error) {
	symbol = Normalize(symbol)
	normTF, err := NormalizeTimeframe(timeframe)
	if err != nil {
		return nil, err
	}
	if !end.After(start) {
		return nil, fmt.Errorf("end time must be after start time")
	}

	logger.Infof("📊 Fetching klines from OKX: %s %s from %s to %s", symbol, normTF, start.Format("2006-01-02 15:04"), end.Format("2006-01-02 15:04"))

	// 创建 OKX 客户端（使用代理）
	okxClient := okx.NewOKXMarketClient("", "", "")

	// 计算需要的K线数量（基于实际的时间范围）
	duration := end.Sub(start)
	tfDuration, err := TFDuration(normTF)
	if err != nil {
		return nil, fmt.Errorf("invalid timeframe: %w", err)
	}

	// 计算需要多少根K线（向上取整，多获取一些以确保覆盖整个时间范围）
	estimatedBars := int(duration/tfDuration) + 1 // +1 for rounding up
	limit := estimatedBars + 50                   // 多获取50根作为buffer

	// OKX API 限制：单次最多可以获取很多数据（通过分批）
	// 但为了避免过大的请求，我们限制最大值
	const maxLimit = 2000
	if limit > maxLimit {
		limit = maxLimit
	}
	if limit < 100 {
		limit = 100
	}

	logger.Infof("📊 Requesting %d klines from OKX (time range: %.2f hours, estimated bars: %d)",
		limit, duration.Hours(), estimatedBars)

	// 调用 OKX API 获取K线数据
	ctx := context.Background()
	okxKlines, err := okxClient.GetKlines(ctx, symbol, normTF, limit)
	if err != nil {
		return nil, fmt.Errorf("OKX API error: %w", err)
	}

	if len(okxKlines) == 0 {
		return nil, fmt.Errorf("no klines returned from OKX")
	}

	logger.Infof("📊 Received %d klines from OKX", len(okxKlines))

	// 转换为通用 Kline 格式
	klines := make([]Kline, 0, len(okxKlines))

	for _, ok := range okxKlines {
		klines = append(klines, Kline{
			OpenTime:  ok.Timestamp,
			Open:      ok.Open,
			High:      ok.High,
			Low:       ok.Low,
			Close:     ok.Close,
			Volume:    ok.Volume,
			CloseTime: ok.Timestamp + int64(tfDuration.Milliseconds()) - 1,
		})
	}

	logger.Infof("📊 Converted %d klines to standard format", len(klines))

	if len(klines) == 0 {
		return nil, fmt.Errorf("no klines returned from OKX")
	}

	// 确保K线按时间升序排列（从旧到新）
	// 虽然OKX API应该返回排序的数据，但为了安全起见，我们再次排序
	sort.Slice(klines, func(i, j int) bool {
		return klines[i].OpenTime < klines[j].OpenTime
	})

	// 去重：移除重复的K线（基于OpenTime）
	if len(klines) > 1 {
		uniqueKlines := make([]Kline, 0, len(klines))
		uniqueKlines = append(uniqueKlines, klines[0])

		for i := 1; i < len(klines); i++ {
			// 只添加与前一根K线时间不同的K线
			if klines[i].OpenTime != klines[i-1].OpenTime {
				uniqueKlines = append(uniqueKlines, klines[i])
			}
		}

		if len(uniqueKlines) < len(klines) {
			logger.Infof("📊 Removed %d duplicate klines", len(klines)-len(uniqueKlines))
		}
		klines = uniqueKlines
	}

	return klines, nil
}
