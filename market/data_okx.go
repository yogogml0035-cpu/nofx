package market

import (
	"context"
	"fmt"
	"nofx/logger"
	"nofx/provider/okx"
	"os"
)

// getKlinesFromOKX 从OKX获取K线数据（替代CoinAnk）
func getKlinesFromOKX(symbol, interval string, limit int) ([]Kline, error) {
	// 创建OKX客户端
	apiKey := os.Getenv("OKX_API_KEY")
	secretKey := os.Getenv("OKX_SECRET_KEY")
	passphrase := os.Getenv("OKX_PASSPHRASE")
	
	// OKX市场数据API不需要认证，但如果提供了密钥也可以使用
	client := okx.NewOKXMarketClient(apiKey, secretKey, passphrase)
	
	// 调用OKX API获取K线数据
	ctx := context.Background()
	okxKlines, err := client.GetKlines(ctx, symbol, interval, limit)
	if err != nil {
		return nil, fmt.Errorf("OKX API error: %w", err)
	}
	
	// 转换为market.Kline格式
	klines := make([]Kline, len(okxKlines))
	for i, ok := range okxKlines {
		klines[i] = Kline{
			OpenTime:  ok.Timestamp,
			Open:      ok.Open,
			High:      ok.High,
			Low:       ok.Low,
			Close:     ok.Close,
			Volume:    ok.Volume,
			CloseTime: ok.Timestamp + getIntervalMillis(interval),
		}
	}
	
	return klines, nil
}

// getIntervalMillis 获取K线周期对应的毫秒数
func getIntervalMillis(interval string) int64 {
	switch interval {
	case "1m":
		return 60 * 1000
	case "3m":
		return 3 * 60 * 1000
	case "5m":
		return 5 * 60 * 1000
	case "15m":
		return 15 * 60 * 1000
	case "30m":
		return 30 * 60 * 1000
	case "1h", "1H":
		return 60 * 60 * 1000
	case "2h", "2H":
		return 2 * 60 * 60 * 1000
	case "4h", "4H":
		return 4 * 60 * 60 * 1000
	case "6h", "6H":
		return 6 * 60 * 60 * 1000
	case "8h", "8H":
		return 8 * 60 * 60 * 1000
	case "12h", "12H":
		return 12 * 60 * 60 * 1000
	case "1d", "1D":
		return 24 * 60 * 60 * 1000
	case "1w", "1W":
		return 7 * 24 * 60 * 60 * 1000
	default:
		return 60 * 1000
	}
}

// GetWithOKX 使用OKX API获取市场数据（替代CoinAnk）
func GetWithOKX(symbol string, timeframes []string, primaryTimeframe string, count int) (*Data, error) {
	symbol = Normalize(symbol)

	if len(timeframes) == 0 {
		return nil, fmt.Errorf("at least one timeframe is required")
	}

	if primaryTimeframe == "" {
		primaryTimeframe = timeframes[0]
	}

	// 确保主时间周期在列表中
	hasPrimary := false
	for _, tf := range timeframes {
		if tf == primaryTimeframe {
			hasPrimary = true
			break
		}
	}
	if !hasPrimary {
		timeframes = append([]string{primaryTimeframe}, timeframes...)
	}

	// 存储所有时间周期的数据
	timeframeData := make(map[string]*TimeframeSeriesData)
	var primaryKlines []Kline

	// 检查是否是xyz dex资产（仍使用Hyperliquid）
	isXyzAsset := IsXyzDexAsset(symbol)

	// 为每个时间周期获取K线数据
	for _, tf := range timeframes {
		var klines []Kline
		var err error

		if isXyzAsset {
			// xyz资产仍使用Hyperliquid API
			klines, err = getKlinesFromHyperliquid(symbol, tf, 200)
			if err != nil {
				logger.Infof("⚠️ Failed to get %s %s K-line from Hyperliquid: %v", symbol, tf, err)
				continue
			}
		} else {
			// 常规加密货币使用OKX API（替代CoinAnk）
			klines, err = getKlinesFromOKX(symbol, tf, 200)
			if err != nil {
				logger.Infof("⚠️ Failed to get %s %s K-line from OKX: %v", symbol, tf, err)
				continue
			}
		}

		if len(klines) == 0 {
			logger.Infof("⚠️ %s %s K-line data is empty", symbol, tf)
			continue
		}

		// 保存主时间周期的K线数据用于计算基础指标
		if tf == primaryTimeframe {
			primaryKlines = klines
		}

		// 计算该时间周期的序列数据
		seriesData := calculateTimeframeSeries(klines, tf, count)
		timeframeData[tf] = seriesData
	}

	// 如果主时间周期数据为空，返回错误
	if len(primaryKlines) == 0 {
		return nil, fmt.Errorf("Primary timeframe %s K-line data is empty", primaryTimeframe)
	}

	// 数据陈旧检测
	if isStaleData(primaryKlines, symbol) {
		logger.Infof("⚠️  WARNING: %s detected stale data (consecutive price freeze), skipping symbol", symbol)
		return nil, fmt.Errorf("%s data is stale, possible cache failure", symbol)
	}

	// 计算当前指标（基于主时间周期的最新数据）
	currentPrice := primaryKlines[len(primaryKlines)-1].Close
	currentEMA20 := calculateEMA(primaryKlines, 20)
	currentMACD := calculateMACD(primaryKlines)
	currentRSI7 := calculateRSI(primaryKlines, 7)

	// 计算价格变化
	priceChange1h := calculatePriceChangeByBars(primaryKlines, primaryTimeframe, 60)  // 1小时
	priceChange4h := calculatePriceChangeByBars(primaryKlines, primaryTimeframe, 240) // 4小时

	// 获取OI数据
	oiData, err := getOpenInterestData(symbol)
	if err != nil {
		oiData = &OIData{Latest: 0, Average: 0}
	}

	// 获取资金费率
	fundingRate, _ := getFundingRate(symbol)

	return &Data{
		Symbol:        symbol,
		CurrentPrice:  currentPrice,
		PriceChange1h: priceChange1h,
		PriceChange4h: priceChange4h,
		CurrentEMA20:  currentEMA20,
		CurrentMACD:   currentMACD,
		CurrentRSI7:   currentRSI7,
		OpenInterest:  oiData,
		FundingRate:   fundingRate,
		TimeframeData: timeframeData,
	}, nil
}
