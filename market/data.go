package market

import (
	"fmt"
	"math"
	"nofx/logger"
	"strings"
	"time"
)

// klineLookbackFloorBars 指标回看下限：配置根数更小时仍多取，EMA50/MACD/1h/4h 涨跌
// 等计算需要比展示根数更长的历史（沿用既有 200 根取数行为）。
const klineLookbackFloorBars = 200

// klineFetchBars 把提示词展示根数换算成实际取数根数：
//   - 不低于指标回看下限；
//   - 不超过 Gate candlesticks 单次查询上限（文档与实测均为 2000 点，且不做分页拼接）。
func klineFetchBars(count int) int {
	if count < klineLookbackFloorBars {
		count = klineLookbackFloorBars
	}
	if count > gateKlineMaxLimit {
		count = gateKlineMaxLimit
	}
	return count
}

// GetWithTimeframes retrieves market data for specified multiple timeframes
// timeframes: list of timeframes, e.g. ["5m", "15m", "1h", "4h"]
// primaryTimeframe: primary timeframe (used for calculating current indicators), defaults to timeframes[0]
// count: number of K-lines for each timeframe
func GetWithTimeframes(symbol string, timeframes []string, primaryTimeframe string, count int) (*Data, error) {
	symbol = Normalize(symbol)

	if len(timeframes) == 0 {
		return nil, fmt.Errorf("at least one timeframe is required")
	}

	// If primary timeframe is not specified, use the first one
	if primaryTimeframe == "" {
		primaryTimeframe = timeframes[0]
	}

	// Ensure primary timeframe is in the list
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

	// Store data for all timeframes
	timeframeData := make(map[string]*TimeframeSeriesData)
	var primaryKlines []Kline
	fetchBars := klineFetchBars(count)

	// Get K-line data for each timeframe
	for _, tf := range timeframes {
		var klines []Kline
		var err error

		// 决策 K 线只走 Gate（D1/D6）：成交量是 USDT 成交额，周期为 D25 保留集合
		klines, err = GetGateKlines(symbol, tf, fetchBars)
		if err != nil {
			logger.Infof("⚠️ Failed to get %s %s K-line from Gate: %v", symbol, tf, err)
			continue
		}

		if len(klines) == 0 {
			logger.Infof("⚠️ %s %s K-line data is empty", symbol, tf)
			continue
		}
		// 末根必须新鲜：最后一根的开盘时间距今不超过两个周期。行情源滞后或返回了旧快照时，
		// 决策会基于收线前的数据（模型把刚收的 K 线当成未收线）。
		if err := klinesFresh(klines, tf); err != nil {
			logger.Infof("⚠️ %s %s K-line data is not fresh: %v", symbol, tf, err)
			continue
		}

		// Save primary timeframe K-lines for calculating base indicators
		if tf == primaryTimeframe {
			primaryKlines = klines
		}

		// Calculate series data for this timeframe (use count from config)
		seriesData := calculateTimeframeSeries(klines, tf, count)
		timeframeData[tf] = seriesData
	}

	// If primary timeframe data is empty, return error
	if len(primaryKlines) == 0 {
		return nil, fmt.Errorf("Primary timeframe %s K-line data is empty", primaryTimeframe)
	}

	// Data staleness detection
	if isStaleData(primaryKlines, symbol) {
		logger.Infof("⚠️  WARNING: %s detected stale data (consecutive price freeze), skipping symbol", symbol)
		return nil, fmt.Errorf("%s data is stale (consecutive price freeze)", symbol)
	}

	// Calculate current indicators (based on primary timeframe latest data)
	currentPrice := primaryKlines[len(primaryKlines)-1].Close
	currentEMA20 := calculateEMA(primaryKlines, 20)
	currentMACD := calculateMACD(primaryKlines)
	currentRSI7 := calculateRSI(primaryKlines, 7)

	// Calculate price changes
	priceChange1h := calculatePriceChangeByBars(primaryKlines, primaryTimeframe, 60)  // 1 hour
	priceChange4h := calculatePriceChangeByBars(primaryKlines, primaryTimeframe, 240) // 4 hours

	// Get OI and funding rate from Gate; failure leaves them nil (= 不可用，不渲染、不参与流动性过滤)
	oiData, fundingRate, err := getGateOpenInterestAndFunding(symbol)
	if err != nil {
		logger.Infof("⚠️ Gate OI/funding rate unavailable for %s: %v", symbol, err)
		oiData, fundingRate = nil, nil
	}

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

// Normalize normalizes symbol
// For crypto: ensures it's a USDT trading pair
func Normalize(symbol string) string {
	symbol = strings.ToUpper(symbol)

	// Remove exchange-specific separators (Gate uses BTC_USDT, OKX uses BTC-USDT-SWAP)
	symbol = strings.ReplaceAll(symbol, "_", "")
	symbol = strings.ReplaceAll(symbol, "-SWAP", "")
	symbol = strings.ReplaceAll(symbol, "-", "")

	// For regular crypto assets
	if strings.HasSuffix(symbol, "USDT") {
		return symbol
	}
	return symbol + "USDT"
}

// isStaleData 检测数据陈旧（连续一段时间的价格冻结 + 成交量为 0）。
// 判定窗口按 K 线间隔换算成至少 15 分钟（最少 5 根）：10s 周期需要 90 根、1m 需要 15 根，
// 避免短周期下 5 根（50 秒）无成交就被误判为数据源异常；30d 等长周期仍以 5 根为下限。
func isStaleData(klines []Kline, symbol string) bool {
	stalePriceThreshold := 5
	if len(klines) >= 2 {
		stepMillis := klines[len(klines)-1].OpenTime - klines[len(klines)-2].OpenTime
		if stepMillis > 0 {
			if bars := int((15 * time.Minute).Milliseconds() / stepMillis); bars > stalePriceThreshold {
				stalePriceThreshold = bars
			}
		}
	}
	if len(klines) < stalePriceThreshold {
		return false // Insufficient data to determine
	}

	const priceTolerancePct = 0.0001 // 0.01% fluctuation tolerance (avoid false positives)

	// Take the last stalePriceThreshold K-lines
	recentKlines := klines[len(klines)-stalePriceThreshold:]
	firstPrice := recentKlines[0].Close

	// Check if all prices are within tolerance
	for i := 1; i < len(recentKlines); i++ {
		priceDiff := math.Abs(recentKlines[i].Close-firstPrice) / firstPrice
		if priceDiff > priceTolerancePct {
			return false // Price fluctuation exists, data is normal
		}
	}

	// Additional check: MACD and volume
	// If price is unchanged but MACD/volume shows normal fluctuation, it might be a real market situation (extremely low volatility)
	// Check if volume is also 0 (data completely frozen)
	allVolumeZero := true
	for _, k := range recentKlines {
		if k.Volume > 0 {
			allVolumeZero = false
			break
		}
	}

	if allVolumeZero {
		logger.Infof("⚠️  %s stale data confirmed: price freeze + zero volume", symbol)
		return true
	}

	// Price frozen but has volume: might be extremely low volatility market, allow but log warning
	logger.Infof("⚠️  %s detected extreme price stability (no fluctuation for %d consecutive periods), but volume is normal", symbol, stalePriceThreshold)
	return false
}

// marketNow 取当前时间；测试替换成固定锚点，让本地 HTTP 服务返回的 K 线与「现在」对齐
var marketNow = time.Now

// klinesFresh 末根 K 线的新鲜度：最后一根的开盘时间距今不超过两个周期（末根通常是未收线的
// 当前根，年龄 0～1 个周期；行情源略滞后时最坏是上一根，年龄 1～2 个周期）。
func klinesFresh(klines []Kline, timeframe string) error {
	if len(klines) == 0 {
		return fmt.Errorf("no klines")
	}
	duration, err := GateIntervalDuration(timeframe)
	if err != nil {
		return err
	}
	last := klines[len(klines)-1]
	age := marketNow().Sub(time.UnixMilli(last.OpenTime))
	if limit := 2*duration + 5*time.Second; age > limit {
		return fmt.Errorf("last bar opened %s ago (limit %s)", age.Truncate(time.Second), limit)
	}
	return nil
}
