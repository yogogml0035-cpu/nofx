package market

import (
	"context"
	"fmt"
	"nofx/logger"
	"nofx/provider/okx"
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

	// 计算需要的K线数量
	duration := end.Sub(start)
	tfDuration, err := TFDuration(normTF)
	if err != nil {
		return nil, fmt.Errorf("invalid timeframe: %w", err)
	}

	// 计算需要多少根K线（向上取整，多获取一些以确保覆盖整个时间范围）
	estimatedBars := int(duration / tfDuration)
	limit := estimatedBars + 100 // 多获取100根以确保覆盖

	// OKX API 限制：单次最多可以获取很多数据（通过分批）
	// 但为了避免过大的请求，我们限制最大值
	const maxLimit = 2000
	if limit > maxLimit {
		limit = maxLimit
	}
	if limit < 100 {
		limit = 100
	}

	logger.Infof("📊 Requesting %d klines from OKX", limit)

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

	return klines, nil
}
