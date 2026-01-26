package market

import (
	"context"
	"fmt"
	"nofx/logger"
	"nofx/provider/coinank/coinank_api"
	"nofx/provider/coinank/coinank_enum"
	"nofx/provider/okx"
	"sort"
	"strconv"
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

	duration := end.Sub(start)
	tfDuration, err := TFDuration(normTF)
	if err != nil {
		return nil, fmt.Errorf("invalid timeframe: %w", err)
	}

	estimatedBars := int(duration/tfDuration) + 1
	limit := estimatedBars + 50

	const maxLimit = 6000
	if limit > maxLimit {
		limit = maxLimit
	}
	if limit < 100 {
		limit = 100
	}

	ctx := context.Background()
	startMs := start.UnixMilli()
	endMs := end.UnixMilli()
	stepMs := int64(tfDuration.Milliseconds())

	coinankKlines, err := getKlinesRangeFromCoinAnk(ctx, symbol, normTF, startMs, endMs, stepMs, limit)
	if err != nil {
		return nil, err
	}
	if len(coinankKlines) == 0 {
		return nil, fmt.Errorf("no klines in range returned from CoinAnk")
	}
	return coinankKlines, nil
}

func getKlinesRangeFromOKX(ctx context.Context, symbol, normTF string, startMs, endMs, stepMs int64, limit int) ([]Kline, error) {
	logger.Infof("📊 Fetching klines from OKX: %s %s from %d to %d (limit=%d)", symbol, normTF, startMs, endMs, limit)

	okxClient := okx.NewOKXMarketClient("", "", "")
	klineByOpenTime := make(map[int64]Kline, limit)
	cursor := endMs + stepMs
	prevCursor := int64(-1)

	for len(klineByOpenTime) < limit {
		if cursor <= 0 || cursor == prevCursor {
			break
		}
		prevCursor = cursor

		batchLimit := 300
		remaining := limit - len(klineByOpenTime)
		if remaining < batchLimit {
			batchLimit = remaining
		}

		okxKlines, err := okxClient.GetHistoryKlines(ctx, symbol, normTF, batchLimit, strconv.FormatInt(cursor, 10))
		if err != nil {
			return nil, fmt.Errorf("OKX API error: %w", err)
		}
		if len(okxKlines) == 0 {
			break
		}

		for _, okK := range okxKlines {
			if okK.Timestamp < startMs || okK.Timestamp > endMs {
				continue
			}
			klineByOpenTime[okK.Timestamp] = Kline{
				OpenTime:  okK.Timestamp,
				Open:      okK.Open,
				High:      okK.High,
				Low:       okK.Low,
				Close:     okK.Close,
				Volume:    okK.Volume,
				CloseTime: okK.Timestamp + stepMs - 1,
			}
		}

		earliest := okxKlines[0].Timestamp
		if earliest <= startMs {
			break
		}
		cursor = earliest
	}

	if len(klineByOpenTime) == 0 {
		return nil, fmt.Errorf("no klines returned from OKX")
	}

	klines := make([]Kline, 0, len(klineByOpenTime))
	for _, k := range klineByOpenTime {
		klines = append(klines, k)
	}
	sort.Slice(klines, func(i, j int) bool {
		return klines[i].OpenTime < klines[j].OpenTime
	})

	return klines, nil
}

func getKlinesRangeFromCoinAnk(ctx context.Context, symbol, normTF string, startMs, endMs, stepMs int64, limit int) ([]Kline, error) {
	interval, err := coinankIntervalFromTimeframe(normTF)
	if err != nil {
		return nil, err
	}

	const maxLimit = 6000
	if limit > maxLimit {
		limit = maxLimit
	}
	if limit < 100 {
		limit = 100
	}

	expectedBars := int((endMs-startMs)/stepMs) + 1
	if expectedBars < 1 {
		expectedBars = 1
	}
	if expectedBars > limit {
		expectedBars = limit
	}

	const coinankBatchMax = 1000
	batchSize := limit
	if batchSize > coinankBatchMax {
		batchSize = coinankBatchMax
	}

	klineByOpenTime := make(map[int64]Kline, expectedBars)
	cursor := endMs
	prevCursor := int64(-1)

	for page := 0; page < 64; page++ {
		if cursor <= 0 || cursor == prevCursor {
			break
		}
		prevCursor = cursor

		logger.Infof("📊 Fetching klines from CoinAnk: %s %s ts=%d size=%d", symbol, normTF, cursor, batchSize)

		resp, err := coinank_api.Kline(ctx, symbol, coinank_enum.Binance, cursor, coinank_enum.To, batchSize, interval)
		if err != nil {
			return nil, fmt.Errorf("CoinAnk API error: %w", err)
		}
		if len(resp) == 0 {
			break
		}

		earliest := int64(-1)
		for _, ck := range resp {
			openTime := ck.StartTime
			if earliest < 0 || openTime < earliest {
				earliest = openTime
			}

			if openTime < startMs || openTime > endMs {
				continue
			}

			klineByOpenTime[openTime] = Kline{
				OpenTime:  openTime,
				Open:      ck.Open,
				High:      ck.High,
				Low:       ck.Low,
				Close:     ck.Close,
				Volume:    ck.Volume,
				CloseTime: openTime + stepMs - 1,
			}
		}

		if len(klineByOpenTime) >= expectedBars {
			break
		}
		if earliest <= startMs {
			break
		}
		if earliest <= 0 || earliest >= cursor {
			break
		}

		cursor = earliest - 1
	}

	if len(klineByOpenTime) == 0 {
		return nil, fmt.Errorf("no klines in range returned from CoinAnk")
	}

	klines := make([]Kline, 0, len(klineByOpenTime))
	for _, k := range klineByOpenTime {
		klines = append(klines, k)
	}
	sort.Slice(klines, func(i, j int) bool {
		return klines[i].OpenTime < klines[j].OpenTime
	})
	return klines, nil
}

func coinankIntervalFromTimeframe(tf string) (coinank_enum.Interval, error) {
	switch tf {
	case "1m":
		return coinank_enum.Minute1, nil
	case "3m":
		return coinank_enum.Minute3, nil
	case "5m":
		return coinank_enum.Minute5, nil
	case "15m":
		return coinank_enum.Minute15, nil
	case "30m":
		return coinank_enum.Minute30, nil
	case "1h":
		return coinank_enum.Hour1, nil
	case "2h":
		return coinank_enum.Hour2, nil
	case "4h":
		return coinank_enum.Hour4, nil
	case "6h":
		return coinank_enum.Hour6, nil
	case "8h":
		return coinank_enum.Hour8, nil
	case "12h":
		return coinank_enum.Hour12, nil
	case "1d":
		return coinank_enum.Day1, nil
	case "3d":
		return coinank_enum.Day3, nil
	case "1w":
		return coinank_enum.Week1, nil
	default:
		return "", fmt.Errorf("unsupported interval: %s", tf)
	}
}
