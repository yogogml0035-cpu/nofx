package backtest

import (
	"fmt"
	"sort"
	"time"

	"nofx/logger"
	"nofx/market"
)

type timeframeSeries struct {
	klines     []market.Kline
	closeTimes []int64
}

type symbolSeries struct {
	byTF map[string]*timeframeSeries
}

func buildTimeframeSeries(klines []market.Kline) *timeframeSeries {
	if len(klines) == 0 {
		return &timeframeSeries{
			klines:     nil,
			closeTimes: nil,
		}
	}

	sorted := append([]market.Kline(nil), klines...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].CloseTime == sorted[j].CloseTime {
			return sorted[i].OpenTime < sorted[j].OpenTime
		}
		return sorted[i].CloseTime < sorted[j].CloseTime
	})

	closeTimes := make([]int64, len(sorted))
	for i, k := range sorted {
		closeTimes[i] = k.CloseTime
	}

	return &timeframeSeries{
		klines:     sorted,
		closeTimes: closeTimes,
	}
}

// DataFeed manages historical kline data and provides time-progressive snapshots for backtesting.
type DataFeed struct {
	cfg           BacktestConfig
	symbols       []string
	timeframes    []string
	symbolSeries  map[string]*symbolSeries
	decisionTimes []int64
	primaryTF     string
	longerTF      string
}

func NewDataFeed(cfg BacktestConfig) (*DataFeed, error) {
	// Determine timeframes to use: prefer strategy config over backtest config
	timeframes := cfg.Timeframes
	primaryTF := cfg.DecisionTimeframe

	// If strategy is loaded, use its timeframe configuration
	if cfg.loadedStrategy != nil {
		if len(cfg.loadedStrategy.Indicators.Klines.SelectedTimeframes) > 0 {
			timeframes = cfg.loadedStrategy.Indicators.Klines.SelectedTimeframes
			logger.Infof("📊 DataFeed: using strategy timeframes: %v", timeframes)
		}
		if cfg.loadedStrategy.Indicators.Klines.PrimaryTimeframe != "" {
			primaryTF = cfg.loadedStrategy.Indicators.Klines.PrimaryTimeframe
			logger.Infof("📊 DataFeed: using strategy primary timeframe: %s", primaryTF)
		}
	}

	// Fallback to defaults if still empty
	if len(timeframes) == 0 {
		timeframes = []string{"5m", "4h"}
		logger.Infof("⚠️  DataFeed: no timeframes configured, using defaults: %v", timeframes)
	}
	if primaryTF == "" {
		primaryTF = timeframes[0]
		logger.Infof("⚠️  DataFeed: no primary timeframe configured, using first: %s", primaryTF)
	}

	seen := make(map[string]bool, len(timeframes)+1)
	normalizedTF := make([]string, 0, len(timeframes))
	for _, tf := range timeframes {
		norm, err := market.NormalizeTimeframe(tf)
		if err != nil {
			return nil, err
		}
		if !seen[norm] {
			seen[norm] = true
			normalizedTF = append(normalizedTF, norm)
		}
	}
	timeframes = normalizedTF

	normPrimary, err := market.NormalizeTimeframe(primaryTF)
	if err != nil {
		return nil, err
	}
	primaryTF = normPrimary
	if !seen[primaryTF] {
		timeframes = append(timeframes, primaryTF)
	}

	logger.Infof("📊 DataFeed initialized: symbols=%v, timeframes=%v, primary=%s",
		cfg.Symbols, timeframes, primaryTF)

	df := &DataFeed{
		cfg:          cfg,
		symbols:      make([]string, len(cfg.Symbols)),
		timeframes:   append([]string(nil), timeframes...),
		symbolSeries: make(map[string]*symbolSeries),
		primaryTF:    primaryTF,
	}
	copy(df.symbols, cfg.Symbols)

	if err := df.loadAll(); err != nil {
		return nil, err
	}

	return df, nil
}

func (df *DataFeed) loadAll() error {
	start := time.Unix(df.cfg.StartTS, 0)
	end := time.Unix(df.cfg.EndTS, 0)

	// longest timeframe used for auxiliary indicators
	var longestDur time.Duration
	for _, tf := range df.timeframes {
		dur, err := market.TFDuration(tf)
		if err != nil {
			return err
		}
		if dur > longestDur {
			longestDur = dur
			df.longerTF = tf
		}
	}

	for _, symbol := range df.symbols {
		ss := &symbolSeries{byTF: make(map[string]*timeframeSeries)}
		for _, tf := range df.timeframes {
			dur, _ := market.TFDuration(tf)

			// Calculate buffer: ensure at least 200 bars, but cap the time range
			// For short timeframes (< 1h), use a minimum buffer to cover the backtest range
			buffer := dur * 200
			minBuffer := end.Sub(start) + (24 * time.Hour) // At least cover the full range + 1 day
			if buffer < minBuffer {
				buffer = minBuffer
			}
			// Cap maximum buffer to avoid excessive data fetching
			maxBuffer := 90 * 24 * time.Hour // 90 days max
			if buffer > maxBuffer {
				buffer = maxBuffer
			}

			fetchStart := start.Add(-buffer)
			if fetchStart.Before(time.Unix(0, 0)) {
				fetchStart = time.Unix(0, 0)
			}
			fetchEnd := end.Add(dur)

			klines, err := market.GetKlinesRange(symbol, tf, fetchStart, fetchEnd)
			if err != nil {
				return fmt.Errorf("fetch klines for %s %s: %w", symbol, tf, err)
			}
			if len(klines) == 0 {
				return fmt.Errorf("no klines for %s %s", symbol, tf)
			}

			series := buildTimeframeSeries(klines)
			ss.byTF[tf] = series
		}
		df.symbolSeries[symbol] = ss
	}

	// Generate backtest progress timeline using the primary timeframe of the first symbol
	firstSymbol := df.symbols[0]
	primarySeries := df.symbolSeries[firstSymbol].byTF[df.primaryTF]
	if primarySeries == nil {
		return fmt.Errorf("primary timeframe %s not found for symbol %s (available: %v)",
			df.primaryTF, firstSymbol, getAvailableTimeframes(df.symbolSeries[firstSymbol]))
	}
	startMs := start.UnixMilli()
	endMs := end.UnixMilli()
	for _, ts := range primarySeries.closeTimes {
		if ts < startMs {
			continue
		}
		if ts > endMs {
			break
		}
		df.decisionTimes = append(df.decisionTimes, ts)
		// Align other symbols; report error early if data is missing
		for _, symbol := range df.symbols[1:] {
			if _, ok := df.symbolSeries[symbol].byTF[df.primaryTF]; !ok {
				return fmt.Errorf("symbol %s missing timeframe %s", symbol, df.primaryTF)
			}
		}
	}
	if len(df.decisionTimes) == 0 {
		return fmt.Errorf("no decision bars in range")
	}
	return nil
}

func (df *DataFeed) DecisionBarCount() int {
	return len(df.decisionTimes)
}

func (df *DataFeed) DecisionTimestamp(index int) int64 {
	// Bounds check to prevent panic
	if index < 0 || index >= len(df.decisionTimes) {
		return 0
	}
	return df.decisionTimes[index]
}

// getMapKeys returns the keys of a map for debugging
func getMapKeys(m map[string][]market.Kline) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func (df *DataFeed) sliceUpTo(symbol, tf string, ts int64) []market.Kline {
	// Nil checks to prevent panic
	ss, ok := df.symbolSeries[symbol]
	if !ok || ss == nil {
		return nil
	}
	series, ok := ss.byTF[tf]
	if !ok || series == nil {
		return nil
	}
	idx := sort.Search(len(series.closeTimes), func(i int) bool {
		return series.closeTimes[i] > ts
	})
	if idx <= 0 {
		return nil
	}
	return series.klines[:idx]
}

func (df *DataFeed) BuildMarketData(ts int64) (map[string]*market.Data, map[string]map[string]*market.Data, error) {
	result := make(map[string]*market.Data, len(df.symbols))
	multi := make(map[string]map[string]*market.Data, len(df.symbols))

	logger.Infof("📊 [DEBUG] BuildMarketData called with %d symbols, %d timeframes: %v",
		len(df.symbols), len(df.timeframes), df.timeframes)

	for _, symbol := range df.symbols {
		// Collect all timeframe klines for this symbol
		klinesMap := make(map[string][]market.Kline)
		for _, tf := range df.timeframes {
			series := df.sliceUpTo(symbol, tf, ts)
			if len(series) > 0 {
				klinesMap[tf] = series
				logger.Infof("📊 [DEBUG] Symbol %s, timeframe %s: collected %d klines", symbol, tf, len(series))
			} else {
				logger.Warnf("⚠️  [DEBUG] Symbol %s, timeframe %s: NO klines collected", symbol, tf)
			}
		}

		if len(klinesMap) == 0 {
			return nil, nil, fmt.Errorf("no kline data for %s at %d", symbol, ts)
		}

		logger.Infof("📊 [DEBUG] Symbol %s: klinesMap has %d timeframes: %v",
			symbol, len(klinesMap), getMapKeys(klinesMap))

		// Use the new function that builds complete multi-timeframe data
		// This ensures backtest has the same TimeframeData structure as live trading
		// Get indicator periods and kline count from loaded strategy config, or use defaults
		emaPeriods := []int{20, 50}
		rsiPeriods := []int{7, 14}
		atrPeriods := []int{14}
		klineCount := 30 // default
		if df.cfg.loadedStrategy != nil {
			if len(df.cfg.loadedStrategy.Indicators.EMAPeriods) > 0 {
				emaPeriods = df.cfg.loadedStrategy.Indicators.EMAPeriods
			}
			if len(df.cfg.loadedStrategy.Indicators.RSIPeriods) > 0 {
				rsiPeriods = df.cfg.loadedStrategy.Indicators.RSIPeriods
			}
			if len(df.cfg.loadedStrategy.Indicators.ATRPeriods) > 0 {
				atrPeriods = df.cfg.loadedStrategy.Indicators.ATRPeriods
			}
			// Use primary_count from strategy config
			if df.cfg.loadedStrategy.Indicators.Klines.PrimaryCount > 0 {
				klineCount = df.cfg.loadedStrategy.Indicators.Klines.PrimaryCount
			}
		}

		data, err := market.BuildDataFromKlinesWithTimeframes(symbol, klinesMap, df.primaryTF, klineCount, emaPeriods, rsiPeriods, atrPeriods)
		if err != nil {
			return nil, nil, fmt.Errorf("build market data for %s: %w", symbol, err)
		}

		result[symbol] = data

		// Build multi map: each timeframe gets its own Data object for backward compatibility
		perTF := make(map[string]*market.Data, len(df.timeframes))
		for _, tf := range df.timeframes {
			if _, ok := klinesMap[tf]; ok {
				// For each timeframe, create a Data object with that timeframe as primary
				// Use the same klineCount from strategy config
				tfData, err := market.BuildDataFromKlinesWithTimeframes(symbol, klinesMap, tf, klineCount, emaPeriods, rsiPeriods, atrPeriods)
				if err != nil {
					continue
				}
				perTF[tf] = tfData
			}
		}
		multi[symbol] = perTF
	}

	return result, multi, nil
}

func (df *DataFeed) decisionBarSnapshot(symbol string, ts int64) (*market.Kline, *market.Kline) {
	ss, ok := df.symbolSeries[symbol]
	if !ok {
		return nil, nil
	}
	series, ok := ss.byTF[df.primaryTF]
	if !ok {
		return nil, nil
	}
	idx := sort.Search(len(series.closeTimes), func(i int) bool {
		return series.closeTimes[i] >= ts
	})
	if idx >= len(series.closeTimes) || series.closeTimes[idx] != ts {
		return nil, nil
	}
	curr := &series.klines[idx]
	var next *market.Kline
	if idx+1 < len(series.klines) {
		next = &series.klines[idx+1]
	}
	return curr, next
}

func getAvailableTimeframes(ss *symbolSeries) []string {
	if ss == nil || ss.byTF == nil {
		return []string{}
	}
	tfs := make([]string, 0, len(ss.byTF))
	for tf := range ss.byTF {
		tfs = append(tfs, tf)
	}
	return tfs
}
