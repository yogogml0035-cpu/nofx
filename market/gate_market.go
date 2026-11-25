package market

// Gate 合约公开行情取数（子任务 12）：K 线与 ticker。只走 Gate，不回退其他数据源（D1）。
// 成交量口径是成交额（sum，USDT），不是合约张数（D6）；周期只用 D25 的保留集合。
// 公开行情无需认证；接口字段以 Gate 官方 Go SDK v7.2.184 的模型为准（文档站本机不可达）：
// FuturesCandlestick{t,v,c,h,l,o,sum} 与 FuturesTicker{contract,last,change_percentage,volume_24h_quote}。
// 接入方（⑬ 引擎、⑭ 图表、⑮ 助手）在后续子任务替换旧路径；本文件不修改任何现有调用方。

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// gateKlineMaxLimit 文档限制：candlesticks 单次最多 2000 点。
const gateKlineMaxLimit = 2000

// gateRetainedIntervalDurations D25 保留的周期及其时长。
// 1w 是自然周（周一起算），7d 与 Unix 初始时间对齐；两者时长相同但起点不同。
// 30d 是自然月：当 limit 覆盖合约全部历史时，首根为每月 1 日 00:00 UTC（2026-11-25 实测
// limit=200 共 85 根、日期均为 1 号）；更小的 limit 返回非月初窗口（实测 limit=3 为 9/10/11 月
// 2 日），调用方不能依赖“每根都是月初”。时长按 30 天用于调用方的周期换算；每根 CloseTime
// 取下一根的开盘时间，最后一根才用 30 天近似（31 天月份会差 1 天，禁止用于精确收线判断）。
var gateRetainedIntervalDurations = map[string]time.Duration{
	"10s": 10 * time.Second,
	"1m":  time.Minute,
	"5m":  5 * time.Minute,
	"15m": 15 * time.Minute,
	"30m": 30 * time.Minute,
	"1h":  time.Hour,
	"4h":  4 * time.Hour,
	"8h":  8 * time.Hour,
	"1d":  24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
	"1w":  7 * 24 * time.Hour,
	"30d": 30 * 24 * time.Hour,
}

// gateRetainedIntervals 按固定顺序给出保留周期（返回副本，调用方可随意修改）。
func gateRetainedIntervals() []string {
	return []string{"10s", "1m", "5m", "15m", "30m", "1h", "4h", "8h", "1d", "7d", "1w", "30d"}
}

// GateIntervalDuration 返回保留周期的时长；不在保留集合内的周期返回错误。
func GateIntervalDuration(interval string) (time.Duration, error) {
	duration, ok := gateRetainedIntervalDurations[interval]
	if !ok {
		return 0, fmt.Errorf("unsupported gate kline interval %q (supported: %v)", interval, gateRetainedIntervals())
	}
	return duration, nil
}

// gateCandleGate candlesticks 的线上格式：所有数值字段按 json.Number 读（字符串或数字都能解析）。
// v（合约张数）不属于交付字段（D6 用 sum 成交额），不在此解析。
type gateCandleGate struct {
	T   json.Number `json:"t"`
	C   json.Number `json:"c"`
	H   json.Number `json:"h"`
	L   json.Number `json:"l"`
	O   json.Number `json:"o"`
	Sum json.Number `json:"sum"`
}

// gateNumber 解析 Gate 的十进制字段：缺失、非数字、NaN/Inf 都报错。
func gateNumber(field string, raw json.Number) (float64, error) {
	text := string(raw)
	if text == "" {
		return 0, fmt.Errorf("gate response is missing %s", field)
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("gate response has invalid %s %q", field, text)
	}
	return value, nil
}

// gatePositive 要求字段为正（价格类）。
func gatePositive(field string, raw json.Number) (float64, error) {
	value, err := gateNumber(field, raw)
	if err != nil {
		return 0, err
	}
	if value <= 0 {
		return 0, fmt.Errorf("gate response has non-positive %s %v", field, value)
	}
	return value, nil
}

// gateNonNegative 允许 0（成交量类）。
func gateNonNegative(field string, raw json.Number) (float64, error) {
	value, err := gateNumber(field, raw)
	if err != nil {
		return 0, err
	}
	if value < 0 {
		return 0, fmt.Errorf("gate response has negative %s %v", field, value)
	}
	return value, nil
}

// GetGateKlines 取 Gate USDT 永续合约 K 线，按时间升序返回。
// OpenTime/CloseTime 为毫秒；Volume 是成交额（USDT，来自 sum），不是合约张数。
// CloseTime 取下一根的开盘时间（30d 为自然月，间距 28–31 天）；最后一根用周期时长近似
// （30d 固定 30 天，31 天月份会早 1 天，禁止用于精确收线判断）。
// 时间戳必须严格递增；OHLC 必须为正且满足 low ≤ open/close ≤ high；sum 必须存在（允许 0）。
func GetGateKlines(symbol, interval string, limit int) ([]Kline, error) {
	if strings.TrimSpace(symbol) == "" {
		return nil, fmt.Errorf("gate kline symbol is empty")
	}
	duration, err := GateIntervalDuration(interval)
	if err != nil {
		return nil, err
	}
	if limit < 1 || limit > gateKlineMaxLimit {
		return nil, fmt.Errorf("gate kline limit must be in [1, %d], got %d", gateKlineMaxLimit, limit)
	}
	contract := toGateContractSymbol(symbol)
	url := fmt.Sprintf("%s/futures/usdt/candlesticks?contract=%s&interval=%s&limit=%d",
		gateFuturesBaseURL, contract, interval, limit)

	var rows []gateCandleGate
	if err := getGateJSON(url, &rows); err != nil {
		return nil, fmt.Errorf("gate candlesticks %s %s: %w", contract, interval, err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("gate candlesticks %s %s returned no data", contract, interval)
	}

	intervalMillis := duration.Milliseconds()
	klines := make([]Kline, 0, len(rows))
	var previousSecond int64
	for i, row := range rows {
		secondsValue, err := gatePositive("t", row.T)
		if err != nil {
			return nil, fmt.Errorf("gate candlesticks %s %s bar %d: %w", contract, interval, i, err)
		}
		seconds := int64(secondsValue)
		if float64(seconds) != secondsValue {
			return nil, fmt.Errorf("gate candlesticks %s %s bar %d: t %v is not an integer second", contract, interval, i, secondsValue)
		}
		if i > 0 && seconds <= previousSecond {
			return nil, fmt.Errorf(
				"gate candlesticks %s %s bar %d: t %d is not after previous %d", contract, interval, i, seconds, previousSecond)
		}
		previousSecond = seconds

		open, err := gatePositive("o", row.O)
		if err != nil {
			return nil, fmt.Errorf("gate candlesticks %s %s bar %d: %w", contract, interval, i, err)
		}
		high, err := gatePositive("h", row.H)
		if err != nil {
			return nil, fmt.Errorf("gate candlesticks %s %s bar %d: %w", contract, interval, i, err)
		}
		low, err := gatePositive("l", row.L)
		if err != nil {
			return nil, fmt.Errorf("gate candlesticks %s %s bar %d: %w", contract, interval, i, err)
		}
		closePrice, err := gatePositive("c", row.C)
		if err != nil {
			return nil, fmt.Errorf("gate candlesticks %s %s bar %d: %w", contract, interval, i, err)
		}
		volume, err := gateNonNegative("sum", row.Sum)
		if err != nil {
			return nil, fmt.Errorf("gate candlesticks %s %s bar %d: %w", contract, interval, i, err)
		}
		if low > open || open > high || low > closePrice || closePrice > high {
			return nil, fmt.Errorf(
				"gate candlesticks %s %s bar %d: inconsistent ohlc o=%v h=%v l=%v c=%v",
				contract, interval, i, open, high, low, closePrice)
		}

		openMillis := seconds * 1000
		klines = append(klines, Kline{
			OpenTime: openMillis,
			Open:     open,
			High:     high,
			Low:      low,
			Close:    closePrice,
			Volume:   volume, // 成交额（USDT）
		})
	}
	for i := range klines {
		if i+1 < len(klines) {
			klines[i].CloseTime = klines[i+1].OpenTime
		} else {
			klines[i].CloseTime = klines[i].OpenTime + intervalMillis
		}
	}
	return klines, nil
}

// GateTicker 一行合约行情。
type GateTicker struct {
	Contract         string  // Gate 合约名，如 BTC_USDT
	Symbol           string  // 内部符号（Normalize 后），如 BTCUSDT
	Last             float64 // 最新价
	ChangePercent24h float64 // 24h 涨跌幅（百分比数值：3.2 表示 +3.2%）
	// Change24h 24h 绝对涨跌（计价币）；Gate 的 change_price，缺省为 0
	Change24h float64
	// High24h / Low24h 24h 最高/最低价；新合约可能缺省，缺省为 0
	High24h float64
	Low24h  float64
	// QuoteVolume24h 24h 成交额（USDT）
	QuoteVolume24h float64
}

// gateTickerGate tickers 的线上格式；数值字段可能是字符串或数字。
type gateTickerGate struct {
	Contract         string      `json:"contract"`
	Last             json.Number `json:"last"`
	ChangePercentage json.Number `json:"change_percentage"`
	ChangePrice      json.Number `json:"change_price"`
	High24h          json.Number `json:"high_24h"`
	Low24h           json.Number `json:"low_24h"`
	Volume24hQuote   json.Number `json:"volume_24h_quote"`
}

// gateOptionalNumber 解析可选数值字段：缺省或为空返回 0，非数字仍报错。
func gateOptionalNumber(field string, raw json.Number) (float64, error) {
	if string(raw) == "" {
		return 0, nil
	}
	return gateNumber(field, raw)
}

// parseGateTicker 校验并解析一行 ticker；字段缺失或不可解析即失败。
func parseGateTicker(row gateTickerGate) (GateTicker, error) {
	if row.Contract == "" {
		return GateTicker{}, fmt.Errorf("gate ticker is missing contract")
	}
	last, err := gatePositive("last", row.Last)
	if err != nil {
		return GateTicker{}, fmt.Errorf("gate ticker %s: %w", row.Contract, err)
	}
	change, err := gateNumber("change_percentage", row.ChangePercentage)
	if err != nil {
		return GateTicker{}, fmt.Errorf("gate ticker %s: %w", row.Contract, err)
	}
	quoteVolume, err := gateNonNegative("volume_24h_quote", row.Volume24hQuote)
	if err != nil {
		return GateTicker{}, fmt.Errorf("gate ticker %s: %w", row.Contract, err)
	}
	changePrice, err := gateOptionalNumber("change_price", row.ChangePrice)
	if err != nil {
		return GateTicker{}, fmt.Errorf("gate ticker %s: %w", row.Contract, err)
	}
	high24h, err := gateOptionalNumber("high_24h", row.High24h)
	if err != nil {
		return GateTicker{}, fmt.Errorf("gate ticker %s: %w", row.Contract, err)
	}
	low24h, err := gateOptionalNumber("low_24h", row.Low24h)
	if err != nil {
		return GateTicker{}, fmt.Errorf("gate ticker %s: %w", row.Contract, err)
	}
	return GateTicker{
		Contract:         row.Contract,
		Symbol:           Normalize(row.Contract),
		Last:             last,
		ChangePercent24h: change,
		Change24h:        changePrice,
		High24h:          high24h,
		Low24h:           low24h,
		QuoteVolume24h:   quoteVolume,
	}, nil
}

// GateContractMetrics 合约级指标：标记价/指数价/资金费率/下次结算时间（毫秒）与持仓量（USD）。
type GateContractMetrics struct {
	MarkPrice       float64
	IndexPrice      float64
	FundingRate     float64
	NextFundingTime int64
	OpenInterestUSD float64
}

type gateContractGate struct {
	MarkPrice        json.Number `json:"mark_price"`
	IndexPrice       json.Number `json:"index_price"`
	FundingRate      json.Number `json:"funding_rate"`
	FundingNextApply json.Number `json:"funding_next_apply"`
}

// GetGateContractMetrics 取合约级指标（公开接口，无需认证）。
// 标记价/指数价必须为正；资金费率可为负；下次结算时间 Gate 给 Unix 秒，这里转毫秒。
// 持仓量取 contract_stats 的最新 open_interest_usd；任一取数失败即错误（快照 fail-closed）。
func GetGateContractMetrics(symbol string) (GateContractMetrics, error) {
	if strings.TrimSpace(symbol) == "" {
		return GateContractMetrics{}, fmt.Errorf("gate contract metrics symbol is empty")
	}
	contract := toGateContractSymbol(symbol)
	var row gateContractGate
	url := fmt.Sprintf("%s/futures/usdt/contracts/%s", gateFuturesBaseURL, contract)
	if err := getGateJSON(url, &row); err != nil {
		return GateContractMetrics{}, fmt.Errorf("gate contract %s: %w", contract, err)
	}
	mark, err := gatePositive("mark_price", row.MarkPrice)
	if err != nil {
		return GateContractMetrics{}, fmt.Errorf("gate contract %s: %w", contract, err)
	}
	index, err := gatePositive("index_price", row.IndexPrice)
	if err != nil {
		return GateContractMetrics{}, fmt.Errorf("gate contract %s: %w", contract, err)
	}
	funding, err := gateNumber("funding_rate", row.FundingRate)
	if err != nil {
		return GateContractMetrics{}, fmt.Errorf("gate contract %s: %w", contract, err)
	}
	nextFunding, err := gateOptionalNumber("funding_next_apply", row.FundingNextApply)
	if err != nil {
		return GateContractMetrics{}, fmt.Errorf("gate contract %s: %w", contract, err)
	}

	oi, _, err := getGateOpenInterestAndFunding(symbol)
	if err != nil {
		return GateContractMetrics{}, fmt.Errorf("gate open interest for %s: %w", contract, err)
	}
	return GateContractMetrics{
		MarkPrice:       mark,
		IndexPrice:      index,
		FundingRate:     funding,
		NextFundingTime: int64(nextFunding) * 1000,
		OpenInterestUSD: oi.Latest,
	}, nil
}

// GetGateTicker 取单个合约的最新价与 24h 行情。
func GetGateTicker(symbol string) (GateTicker, error) {
	if strings.TrimSpace(symbol) == "" {
		return GateTicker{}, fmt.Errorf("gate ticker symbol is empty")
	}
	contract := toGateContractSymbol(symbol)
	url := fmt.Sprintf("%s/futures/usdt/tickers?contract=%s", gateFuturesBaseURL, contract)

	var rows []gateTickerGate
	if err := getGateJSON(url, &rows); err != nil {
		return GateTicker{}, fmt.Errorf("gate tickers %s: %w", contract, err)
	}
	for _, row := range rows {
		if row.Contract != contract {
			continue
		}
		return parseGateTicker(row)
	}
	return GateTicker{}, fmt.Errorf("gate tickers returned no entry for %s", contract)
}

// ListGateTickers 取全部 USDT 永续合约的 24h 行情（供 ⑮ 的 24h 行情接口）。
// 任一条目字段缺失或不可解析即整体失败（fail-closed，不静默丢弃合约）；新上架且未成交的
// 合约若 last 为 0/空，会让整个列表失败，这是 ⑮ 需要评估的边界（2026-11-25 实测 1042 个
// 合约全部 last>0、字段齐全，未触发）。
func ListGateTickers() ([]GateTicker, error) {
	url := fmt.Sprintf("%s/futures/usdt/tickers", gateFuturesBaseURL)

	var rows []gateTickerGate
	if err := getGateJSON(url, &rows); err != nil {
		return nil, fmt.Errorf("gate tickers: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("gate tickers returned no contracts")
	}
	tickers := make([]GateTicker, 0, len(rows))
	for _, row := range rows {
		ticker, err := parseGateTicker(row)
		if err != nil {
			return nil, err
		}
		tickers = append(tickers, ticker)
	}
	return tickers, nil
}

// getGateJSON 发出公开行情 GET 并解析 JSON；非 200、读体失败与解析失败都返回错误。
func getGateJSON(url string, out interface{}) error {
	apiClient := NewAPIClient()
	resp, err := apiClient.client.Get(url)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read body failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("returned status %d: %s", resp.StatusCode, errorBodySnippet(body))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode failed: %w", err)
	}
	return nil
}
