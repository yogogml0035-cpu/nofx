package okx

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

// OKXMarketClient OKX市场数据客户端
type OKXMarketClient struct {
	apiKey     string
	secretKey  string
	passphrase string
	baseURL    string
	httpClient *http.Client
}

// KlineData K线数据结构
type KlineData struct {
	Timestamp int64   // 时间戳（毫秒）
	Open      float64 // 开盘价
	High      float64 // 最高价
	Low       float64 // 最低价
	Close     float64 // 收盘价
	Volume    float64 // 成交量
}

// NewOKXMarketClient 创建OKX市场数据客户端
func NewOKXMarketClient(apiKey, secretKey, passphrase string) *OKXMarketClient {
	// 创建自定义Transport，配置DNS和连接参数
	transport := &http.Transport{
		// 强制使用IPv4，避免IPv6 DNS解析问题
		DialContext: (&net.Dialer{
			Timeout:   60 * time.Second, // 增加连接超时
			KeepAlive: 30 * time.Second,
			// 强制使用IPv4
			FallbackDelay: -1,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   60 * time.Second, // 增加 TLS 握手超时，应对代理不稳定
		ExpectContinueTimeout: 1 * time.Second,
		// 使用系统代理设置
		Proxy: http.ProxyFromEnvironment,
	}

	// 检查是否配置了代理
	proxyURL := os.Getenv("HTTPS_PROXY")
	if proxyURL == "" {
		proxyURL = os.Getenv("HTTP_PROXY")
	}
	if proxyURL == "" {
		proxyURL = os.Getenv("https_proxy")
	}
	if proxyURL == "" {
		proxyURL = os.Getenv("http_proxy")
	}

	if proxyURL != "" {
		if proxy, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(proxy)
			fmt.Printf("🌐 Using proxy: %s\n", proxyURL)
		}
	}

	// 创建 HTTP 客户端
	client := &http.Client{
		Timeout:   90 * time.Second, // 增加超时时间以应对代理连接不稳定的情况
		Transport: transport,
	}

	return &OKXMarketClient{
		apiKey:     apiKey,
		secretKey:  secretKey,
		passphrase: passphrase,
		baseURL:    "https://www.okx.com",
		httpClient: client,
	}
}

// generateSignature 生成OKX API签名
func (c *OKXMarketClient) generateSignature(timestamp, method, requestPath, body string) string {
	message := timestamp + method + requestPath + body
	h := hmac.New(sha256.New, []byte(c.secretKey))
	h.Write([]byte(message))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// doRequest 执行HTTP请求
func (c *OKXMarketClient) doRequest(ctx context.Context, method, endpoint string, params map[string]string, needAuth bool) ([]byte, error) {
	url := c.baseURL + endpoint

	// 构建查询参数
	if len(params) > 0 && method == "GET" {
		url += "?"
		first := true
		for k, v := range params {
			if !first {
				url += "&"
			}
			url += k + "=" + v
			first = false
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")

	// 如果需要认证，添加签名
	if needAuth && c.apiKey != "" {
		timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
		requestPath := endpoint
		if len(params) > 0 && method == "GET" {
			requestPath += "?"
			first := true
			for k, v := range params {
				if !first {
					requestPath += "&"
				}
				requestPath += k + "=" + v
				first = false
			}
		}

		signature := c.generateSignature(timestamp, method, requestPath, "")

		req.Header.Set("OK-ACCESS-KEY", c.apiKey)
		req.Header.Set("OK-ACCESS-SIGN", signature)
		req.Header.Set("OK-ACCESS-TIMESTAMP", timestamp)
		req.Header.Set("OK-ACCESS-PASSPHRASE", c.passphrase)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

// GetKlines 获取K线数据（支持大量历史数据）
// symbol: 交易对，如 BTCUSDT
// interval: K线周期，如 1m, 3m, 5m, 15m, 30m, 1H, 2H, 4H, 6H, 12H, 1D, 1W
// limit: 数据条数，最大300（会自动分批获取）
func (c *OKXMarketClient) GetKlines(ctx context.Context, symbol, interval string, limit int) ([]KlineData, error) {
	// 转换symbol格式：BTCUSDT -> BTC-USDT
	instId := convertSymbolToOKX(symbol)

	// 转换interval格式
	bar := convertIntervalToOKX(interval)

	// OKX API单次最多返回100条，需要分批获取
	const maxBatchSize = 100

	// 如果请求的数量小于等于100，直接使用candles端点（包含最新数据）
	if limit <= maxBatchSize {
		endpoint := "/api/v5/market/candles"
		params := map[string]string{
			"instId": instId,
			"bar":    bar,
			"limit":  strconv.Itoa(limit),
		}

		needAuth := c.apiKey != ""
		data, err := c.doRequest(ctx, "GET", endpoint, params, needAuth)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch klines: %w", err)
		}

		klines, err := c.parseKlineResponse(data)
		if err != nil {
			return nil, err
		}

		return klines, nil
	}

	// 如果请求超过100条，需要分批获取
	// 策略：先获取最新的100条，然后向前获取更早的数据
	var allBatches [][]KlineData
	remaining := limit
	var before string // 用于分页的参数（获取更早的数据）

	for remaining > 0 {
		batchSize := maxBatchSize
		if remaining < maxBatchSize {
			batchSize = remaining
		}

		// 第一批使用candles获取最新数据，后续批次使用history-candles
		var endpoint string
		if before == "" {
			endpoint = "/api/v5/market/candles"
		} else {
			endpoint = "/api/v5/market/history-candles"
		}

		params := map[string]string{
			"instId": instId,
			"bar":    bar,
			"limit":  strconv.Itoa(batchSize),
		}

		// 如果有before参数，添加到请求中（用于获取更早的数据）
		if before != "" {
			params["before"] = before
		}

		needAuth := c.apiKey != ""
		data, err := c.doRequest(ctx, "GET", endpoint, params, needAuth)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch klines: %w", err)
		}

		klines, err := c.parseKlineResponse(data)
		if err != nil {
			return nil, err
		}

		if len(klines) == 0 {
			break // 没有更多数据了
		}

		// 将这批数据添加到批次列表（注意：每批内部已经是从旧到新排序）
		allBatches = append(allBatches, klines)
		remaining -= len(klines)

		// 如果返回的数据少于请求的数量，说明没有更多数据了
		if len(klines) < batchSize {
			break
		}

		// 设置before参数为这批数据中最早的时间戳，用于获取更早的数据
		// OKX的before参数是"请求此时间戳之前的数据"
		before = strconv.FormatInt(klines[0].Timestamp, 10)
	}

	// 合并所有批次：从最后一批开始（最早的数据）到第一批（最新的数据）
	var allKlines []KlineData
	for i := len(allBatches) - 1; i >= 0; i-- {
		allKlines = append(allKlines, allBatches[i]...)
	}

	return allKlines, nil
}

// parseKlineResponse 解析K线响应数据
func (c *OKXMarketClient) parseKlineResponse(data []byte) ([]KlineData, error) {
	var response struct {
		Code string          `json:"code"`
		Msg  string          `json:"msg"`
		Data [][]interface{} `json:"data"`
	}

	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if response.Code != "0" {
		return nil, fmt.Errorf("API error: %s", response.Msg)
	}

	// 转换数据格式
	klines := make([]KlineData, 0, len(response.Data))
	for _, item := range response.Data {
		if len(item) < 6 {
			continue
		}

		kline := KlineData{
			Timestamp: parseFloat64ToInt64(item[0]),
			Open:      parseFloat64(item[1]),
			High:      parseFloat64(item[2]),
			Low:       parseFloat64(item[3]),
			Close:     parseFloat64(item[4]),
			Volume:    parseFloat64(item[5]),
		}
		klines = append(klines, kline)
	}

	// OKX返回的数据是从新到旧，需要反转
	for i, j := 0, len(klines)-1; i < j; i, j = i+1, j-1 {
		klines[i], klines[j] = klines[j], klines[i]
	}

	return klines, nil
}

// GetTicker 获取实时行情
func (c *OKXMarketClient) GetTicker(ctx context.Context, symbol string) (map[string]interface{}, error) {
	instId := convertSymbolToOKX(symbol)

	endpoint := "/api/v5/market/ticker"
	params := map[string]string{
		"instId": instId,
	}

	// 使用认证以获得更高的请求限额
	needAuth := c.apiKey != ""
	data, err := c.doRequest(ctx, "GET", endpoint, params, needAuth)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ticker: %w", err)
	}

	var response struct {
		Code string                   `json:"code"`
		Msg  string                   `json:"msg"`
		Data []map[string]interface{} `json:"data"`
	}

	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if response.Code != "0" {
		return nil, fmt.Errorf("API error: %s", response.Msg)
	}

	if len(response.Data) == 0 {
		return nil, fmt.Errorf("no ticker data found")
	}

	return response.Data[0], nil
}

// convertSymbolToOKX 转换交易对格式
// BTCUSDT -> BTC-USDT-SWAP (永续合约)
func convertSymbolToOKX(symbol string) string {
	// 移除USDT后缀
	if len(symbol) > 4 && symbol[len(symbol)-4:] == "USDT" {
		base := symbol[:len(symbol)-4]
		return base + "-USDT-SWAP"
	}
	return symbol + "-USDT-SWAP"
}

// convertIntervalToOKX 转换K线周期格式
func convertIntervalToOKX(interval string) string {
	// 映射关系
	mapping := map[string]string{
		"1m":  "1m",
		"3m":  "3m",
		"5m":  "5m",
		"15m": "15m",
		"30m": "30m",
		"1h":  "1H",
		"1H":  "1H",
		"2h":  "2H",
		"2H":  "2H",
		"4h":  "4H",
		"4H":  "4H",
		"6h":  "6H",
		"6H":  "6H",
		"8h":  "8H",
		"8H":  "8H",
		"12h": "12H",
		"12H": "12H",
		"1d":  "1D",
		"1D":  "1D",
		"1w":  "1W",
		"1W":  "1W",
	}

	if okxInterval, ok := mapping[interval]; ok {
		return okxInterval
	}
	return interval
}

// parseFloat64 解析interface{}为float64
func parseFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	default:
		return 0
	}
}

// parseFloat64ToInt64 解析interface{}为int64（时间戳）
func parseFloat64ToInt64(v interface{}) int64 {
	switch val := v.(type) {
	case string:
		i, _ := strconv.ParseInt(val, 10, 64)
		return i
	case float64:
		return int64(val)
	case int:
		return int64(val)
	case int64:
		return val
	default:
		return 0
	}
}

// GetOpenInterest 获取持仓量数据
// symbol: 交易对，如 BTCUSDT
// 返回当前的持仓量（以合约张数计）
func (c *OKXMarketClient) GetOpenInterest(ctx context.Context, symbol string) (float64, error) {
	instId := convertSymbolToOKX(symbol)

	endpoint := "/api/v5/public/open-interest"
	params := map[string]string{
		"instId": instId,
	}

	// 公开接口，不需要认证
	data, err := c.doRequest(ctx, "GET", endpoint, params, false)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch open interest: %w", err)
	}

	var response struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			InstId string `json:"instId"` // 产品ID
			Oi     string `json:"oi"`     // 持仓量（张）
			OiCcy  string `json:"oiCcy"`  // 持仓量（币）
			Ts     string `json:"ts"`     // 数据返回时间戳
		} `json:"data"`
	}

	if err := json.Unmarshal(data, &response); err != nil {
		return 0, fmt.Errorf("failed to parse response: %w", err)
	}

	if response.Code != "0" {
		return 0, fmt.Errorf("API error: %s", response.Msg)
	}

	if len(response.Data) == 0 {
		return 0, fmt.Errorf("no open interest data found")
	}

	// 解析持仓量（使用币本位的持仓量）
	oiCcy := parseFloat64(response.Data[0].OiCcy)

	return oiCcy, nil
}
