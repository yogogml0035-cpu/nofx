package market

import (
	"fmt"
	"time"
)

// calculateTimeframeSeries calculates series data for a single timeframe
func calculateTimeframeSeries(klines []Kline, timeframe string, count int) *TimeframeSeriesData {
	if count <= 0 {
		count = 10 // default
	}

	data := &TimeframeSeriesData{
		Timeframe:   timeframe,
		Klines:      make([]KlineBar, 0, count),
		MidPrices:   make([]float64, 0, count),
		EMA20Values: make([]float64, 0, count),
		EMA50Values: make([]float64, 0, count),
		MACDValues:  make([]float64, 0, count),
		RSI7Values:  make([]float64, 0, count),
		RSI14Values: make([]float64, 0, count),
		Volume:      make([]float64, 0, count),
		BOLLUpper:   make([]float64, 0, count),
		BOLLMiddle:  make([]float64, 0, count),
		BOLLLower:   make([]float64, 0, count),
	}

	// Get latest N data points based on count from config
	start := len(klines) - count
	if start < 0 {
		start = 0
	}

	for i := start; i < len(klines); i++ {
		// Store full OHLCV kline data
		data.Klines = append(data.Klines, KlineBar{
			Time:   klines[i].OpenTime,
			Open:   klines[i].Open,
			High:   klines[i].High,
			Low:    klines[i].Low,
			Close:  klines[i].Close,
			Volume: klines[i].Volume,
		})

		// Keep MidPrices and Volume for backward compatibility
		data.MidPrices = append(data.MidPrices, klines[i].Close)
		data.Volume = append(data.Volume, klines[i].Volume)

		// Calculate EMA20 for each point
		if i >= 19 {
			ema20 := calculateEMA(klines[:i+1], 20)
			data.EMA20Values = append(data.EMA20Values, ema20)
		}

		// Calculate EMA50 for each point
		if i >= 49 {
			ema50 := calculateEMA(klines[:i+1], 50)
			data.EMA50Values = append(data.EMA50Values, ema50)
		}

		// Calculate MACD for each point
		if i >= 25 {
			macd := calculateMACD(klines[:i+1])
			data.MACDValues = append(data.MACDValues, macd)
		}

		// Calculate RSI for each point
		if i >= 7 {
			rsi7 := calculateRSI(klines[:i+1], 7)
			data.RSI7Values = append(data.RSI7Values, rsi7)
		}
		if i >= 14 {
			rsi14 := calculateRSI(klines[:i+1], 14)
			data.RSI14Values = append(data.RSI14Values, rsi14)
		}

		// Calculate Bollinger Bands (period 20, std dev multiplier 2)
		if i >= 19 {
			upper, middle, lower := calculateBOLL(klines[:i+1], 20, 2.0)
			data.BOLLUpper = append(data.BOLLUpper, upper)
			data.BOLLMiddle = append(data.BOLLMiddle, middle)
			data.BOLLLower = append(data.BOLLLower, lower)
		}
	}

	// Calculate ATR14
	data.ATR14 = calculateATR(klines, 14)

	return data
}

// calculatePriceChangeByBars 按 Gate 周期时长计算回看根数（秒级口径，10s 等周期也适用）。
func calculatePriceChangeByBars(klines []Kline, timeframe string, targetMinutes int) float64 {
	if len(klines) < 2 {
		return 0
	}

	duration, err := GateIntervalDuration(timeframe)
	if err != nil {
		return 0
	}
	stepSeconds := int64(duration / time.Second)
	if stepSeconds <= 0 {
		return 0
	}

	// Calculate how many K-lines to look back（不足时按可用窗口）
	barsBack := int(int64(targetMinutes*60) / stepSeconds)
	if barsBack < 1 {
		barsBack = 1
	}

	currentPrice := klines[len(klines)-1].Close
	idx := len(klines) - 1 - barsBack
	if idx < 0 {
		idx = 0
	}

	oldPrice := klines[idx].Close
	if oldPrice > 0 {
		return ((currentPrice - oldPrice) / oldPrice) * 100
	}
	return 0
}

// GetBoxData fetches 1h klines and calculates box data for a symbol
func GetBoxData(symbol string) (*BoxData, error) {
	symbol = Normalize(symbol)

	// Fetch 500 1h klines from Gate（D1/D6）
	klines, err := GetGateKlines(symbol, "1h", LongBoxPeriod)
	if err != nil {
		return nil, fmt.Errorf("failed to get 1h klines: %w", err)
	}

	if len(klines) == 0 {
		return nil, fmt.Errorf("no kline data available")
	}

	currentPrice := klines[len(klines)-1].Close

	return calculateBoxData(klines, currentPrice), nil
}
