package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"nofx/logger"
	"nofx/market"
	"nofx/provider/alpaca"
	"nofx/provider/gate"
	"nofx/provider/twelvedata"

	"github.com/gin-gonic/gin"
)

// 默认行情数据源为 Gate（D7）：/api/klines 与 /api/symbols 无参时均走 gate
const defaultExchange = "gate"

// cryptoKlines 取加密货币 K 线（测试可替换）；默认 Gate 直连（D1）。
// 成交量由 GetGateKlines 返回 USDT 成交额（sum），QuoteVolume 在取数层填好（缓存切片不可原地改写）。
// 包级变量是测试缝：api 包测试必须保持非并行（t.Parallel 会造成竞态）。
var cryptoKlines = market.GetGateKlines

// handleKlines K-line data：加密货币只走 Gate；股票（Alpaca）与外汇/金属（TwelveData）保留。
func (s *Server) handleKlines(c *gin.Context) {
	symbol := c.Query("symbol")
	if symbol == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "symbol parameter is required"})
		return
	}

	interval := c.DefaultQuery("interval", "5m")
	limitStr := c.DefaultQuery("limit", "1000")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 1000
	}
	if limit > 2000 { // Gate candlesticks 单次上限
		limit = 2000
	}

	var klines []market.Kline
	exchangeLower := strings.ToLower(c.DefaultQuery("exchange", defaultExchange))

	switch exchangeLower {
	case "alpaca":
		// 美股：Alpaca
		klines, err = s.getKlinesFromAlpaca(symbol, interval, limit)
		if err != nil {
			SafeInternalError(c, "Get klines from Alpaca", err)
			return
		}
	case "forex", "metals":
		// 外汇与金属：Twelve Data
		klines, err = s.getKlinesFromTwelveData(symbol, interval, limit)
		if err != nil {
			SafeInternalError(c, "Get klines from TwelveData", err)
			return
		}
	default:
		// 加密货币：Gate 合约 K 线（exchange 参数不再参与路由；无其他数据源回退）。
		// 非保留周期属于客户端参数错误，先在发请求前拒绝为 400。
		if _, err := market.GateIntervalDuration(interval); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		klines, err = cryptoKlines(symbol, interval, limit)
		if err != nil {
			// Gate 公共接口按 IP 限频（单个接口 200r/10s）：上游 429 是暂时不可用，不是服务端故障；
			// 未知合约是客户端参数错误。两者都不记成 500。
			if errors.Is(err, market.ErrGateRateLimited) {
				c.Header("Retry-After", "1")
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": "market data source is rate limited, retry shortly"})
				return
			}
			if errors.Is(err, market.ErrGateContractNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "unknown symbol"})
				return
			}
			SafeInternalError(c, "Get klines from Gate", err)
			return
		}
	}

	c.JSON(http.StatusOK, klines)
}

// getKlinesFromAlpaca fetches kline data from Alpaca API for US stocks
func (s *Server) getKlinesFromAlpaca(symbol, interval string, limit int) ([]market.Kline, error) {
	// Create Alpaca client
	client := alpaca.NewClient()

	// Map interval to Alpaca timeframe format
	timeframe := alpaca.MapTimeframe(interval)

	// Fetch bars from Alpaca
	ctx := context.Background()
	bars, err := client.GetBars(ctx, symbol, timeframe, limit)
	if err != nil {
		return nil, fmt.Errorf("alpaca API error: %w", err)
	}

	// Convert Alpaca bars to market.Kline format
	klines := make([]market.Kline, len(bars))
	for i, bar := range bars {
		klines[i] = market.Kline{
			OpenTime:    bar.Timestamp.UnixMilli(),
			Open:        bar.Open,
			High:        bar.High,
			Low:         bar.Low,
			Close:       bar.Close,
			Volume:      float64(bar.Volume),             // share count
			QuoteVolume: float64(bar.Volume) * bar.Close, // turnover = shares * close price (USD)
			CloseTime:   bar.Timestamp.UnixMilli(),
		}
	}

	return klines, nil
}

// getKlinesFromTwelveData fetches kline data from Twelve Data API for forex and metals
func (s *Server) getKlinesFromTwelveData(symbol, interval string, limit int) ([]market.Kline, error) {
	// Create Twelve Data client
	client := twelvedata.NewClient()

	// Map interval to Twelve Data timeframe format
	timeframe := twelvedata.MapTimeframe(interval)

	// Fetch time series from Twelve Data
	ctx := context.Background()
	result, err := client.GetTimeSeries(ctx, symbol, timeframe, limit)
	if err != nil {
		return nil, fmt.Errorf("twelvedata API error: %w", err)
	}

	// Convert Twelve Data bars to market.Kline format
	// Note: Twelve Data returns bars in reverse order (newest first)
	klines := make([]market.Kline, len(result.Values))
	for i, bar := range result.Values {
		open, high, low, close, volume, timestamp, err := twelvedata.ParseBar(bar)
		if err != nil {
			logger.Warnf("⚠️ Failed to parse TwelveData bar: %v", err)
			continue
		}

		// Reverse order: put oldest first
		idx := len(result.Values) - 1 - i
		klines[idx] = market.Kline{
			OpenTime:  timestamp,
			Open:      open,
			High:      high,
			Low:       low,
			Close:     close,
			Volume:    volume,
			CloseTime: timestamp,
		}
	}

	return klines, nil
}

// handleSymbols returns available symbols for a given exchange
func (s *Server) handleSymbols(c *gin.Context) {
	exchange := c.DefaultQuery("exchange", defaultExchange)

	if strings.ToLower(exchange) != "gate" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported exchange for symbol listing"})
		return
	}

	// Gate USDT 永续合约清单（公开行情接口，免鉴权）
	symbols, err := gate.ListSymbols(c.Request.Context())
	if err != nil {
		SafeInternalError(c, "Get Gate symbols", err)
		return
	}

	// 单一 crypto 类别：按 24h 成交额降序
	sort.SliceStable(symbols, func(i, j int) bool {
		return symbols[i].Volume24h > symbols[j].Volume24h
	})

	c.JSON(http.StatusOK, gin.H{
		"exchange": exchange,
		"symbols":  symbols,
		"count":    len(symbols),
	})
}
