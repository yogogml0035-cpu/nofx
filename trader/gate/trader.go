package gate

import (
	"context"
	"fmt"
	"math/big"
	"net/http"
	"nofx/trader/types"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gate/gateapi-go/v7"
)

// GateTrader implements types.Trader interface for Gate.io Futures
type GateTrader struct {
	apiKey    string
	secretKey string
	client    *gateapi.APIClient
	ctx       context.Context

	// Cache fields
	cachedBalance       map[string]interface{}
	balanceCacheTime    time.Time
	balanceCacheMutex   sync.RWMutex
	cachedPositions     []map[string]interface{}
	positionsCacheTime  time.Time
	positionsCacheMutex sync.RWMutex
	cacheDuration       time.Duration
	contractSpecs       contractSpecCache
}

// parseRequiredSize 解析 Gate 必然返回的数量字段（SDK 以字符串给出，小数张合约带小数部分）
func parseRequiredSize(field, raw string) (float64, error) {
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", field, raw, err)
	}
	return v, nil
}

// parseOptionalSize 解析 Gate 在数值为 0 时不返回的数量字段，缺省按 0 计
func parseOptionalSize(field, raw string) (float64, error) {
	if raw == "" {
		return 0, nil
	}
	return parseRequiredSize(field, raw)
}

// parseOptionalRat 同 parseOptionalSize，但保留精确十进制值
func parseOptionalRat(field, raw string) (*big.Rat, error) {
	if raw == "" {
		return big.NewRat(0, 1), nil
	}
	return parseDecimal(field, raw)
}

// NewGateTrader creates a new Gate trader instance
func NewGateTrader(apiKey, secretKey string) *GateTrader {
	config := gateapi.NewConfiguration()
	config.AddDefaultHeader("X-Gate-Channel-Id", "nofx")
	// The SDK default HTTP client has no timeout — a hung connection would
	// stall the trading loop indefinitely.
	config.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	client := gateapi.NewAPIClient(config)

	ctx := context.WithValue(context.Background(),
		gateapi.ContextGateAPIV4,
		gateapi.GateAPIV4{
			Key:    apiKey,
			Secret: secretKey,
		},
	)

	return &GateTrader{
		apiKey:        apiKey,
		secretKey:     secretKey,
		client:        client,
		ctx:           ctx,
		cacheDuration: 15 * time.Second,
	}
}

// convertSymbol converts symbol format (e.g., BTCUSDT -> BTC_USDT)
func (t *GateTrader) convertSymbol(symbol string) string {
	// If already in correct format
	if strings.Contains(symbol, "_") {
		return symbol
	}
	// Convert BTCUSDT to BTC_USDT
	if strings.HasSuffix(symbol, "USDT") {
		base := strings.TrimSuffix(symbol, "USDT")
		return base + "_USDT"
	}
	return symbol
}

// revertSymbol converts symbol back to standard format (e.g., BTC_USDT -> BTCUSDT)
func (t *GateTrader) revertSymbol(symbol string) string {
	return strings.ReplaceAll(symbol, "_", "")
}

// clearCache clears all caches
func (t *GateTrader) clearCache() {
	t.balanceCacheMutex.Lock()
	t.cachedBalance = nil
	t.balanceCacheMutex.Unlock()

	t.positionsCacheMutex.Lock()
	t.cachedPositions = nil
	t.positionsCacheMutex.Unlock()
}

// Ensure GateTrader implements Trader interface
var _ types.Trader = (*GateTrader)(nil)
