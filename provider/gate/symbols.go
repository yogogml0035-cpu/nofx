package gate

import (
	"context"
	"fmt"
	"net/http"
	"nofx/logger"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gateio/gateapi-go/v6"
)

// SymbolInfo Gate USDT 永续合约的符号与行情信息
type SymbolInfo struct {
	Symbol       string  `json:"symbol"`     // Gate 原生合约名，如 BTC_USDT
	Display      string  `json:"display"`    // 展示名，同合约名
	Name         string  `json:"name"`       // 基础币种，如 BTC
	Category     string  `json:"category"`   // 固定 crypto
	Exchange     string  `json:"exchange"`   // 固定 gate
	Volume24h    float64 `json:"volume_24h"` // 24h 成交额（USDT 结算币种计价）
	MarkPrice    float64 `json:"mark_price"` // 最新价（回退标记价）
	Change24hPct float64 `json:"change_24h_pct,omitempty"`
	MaxLeverage  int     `json:"maxLeverage,omitempty"`
	SzDecimals   int     `json:"sz_decimals,omitempty"` // 最小下单张数（Gate 语义）
}

// ListSymbols 拉取 Gate USDT 永续合约清单与行情（公开接口，免鉴权）。
// 数据源：GET /futures/usdt/contracts + GET /futures/usdt/tickers；
// 过滤 in_delisting；结果按 24h 成交额降序。
func ListSymbols(ctx context.Context) ([]SymbolInfo, error) {
	config := gateapi.NewConfiguration()
	config.AddDefaultHeader("X-Gate-Channel-Id", "nofx")
	config.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	client := gateapi.NewAPIClient(config)

	// 公开行情接口无需 APIv4 凭证，使用空 context
	apiCtx := context.Background()

	contracts, _, err := client.FuturesApi.ListFuturesContracts(apiCtx, "usdt", nil)
	if err != nil {
		return nil, fmt.Errorf("gate list futures contracts: %w", err)
	}

	tickers, _, err := client.FuturesApi.ListFuturesTickers(apiCtx, "usdt", nil)
	if err != nil {
		return nil, fmt.Errorf("gate list futures tickers: %w", err)
	}

	tickerByContract := make(map[string]gateapi.FuturesTicker, len(tickers))
	for _, t := range tickers {
		tickerByContract[t.Contract] = t
	}

	symbols := make([]SymbolInfo, 0, len(contracts))
	for _, contract := range contracts {
		ticker, hasTicker := tickerByContract[contract.Name]
		if info, ok := SymbolInfoFromContract(contract, ticker, hasTicker); ok {
			symbols = append(symbols, info)
		}
	}

	sort.SliceStable(symbols, func(i, j int) bool {
		return symbols[i].Volume24h > symbols[j].Volume24h
	})

	logger.Infof("📊 Gate symbols loaded: %d USDT perp contracts", len(symbols))
	return symbols, nil
}

// SymbolInfoFromContract 把 Gate 合约与对应 ticker 映射为 SymbolInfo。
// in_delisting 的合约返回 ok=false（下架过渡期合约不进入标的清单）。
func SymbolInfoFromContract(contract gateapi.Contract, ticker gateapi.FuturesTicker, hasTicker bool) (SymbolInfo, bool) {
	if contract.InDelisting {
		return SymbolInfo{}, false
	}

	info := SymbolInfo{
		Symbol:   contract.Name,
		Display:  contract.Name,
		Name:     baseSymbol(contract.Name),
		Category: "crypto",
		Exchange: "gate",
	}

	if maxLev, err := strconv.ParseFloat(contract.LeverageMax, 64); err == nil {
		info.MaxLeverage = int(maxLev)
	}
	info.SzDecimals = int(contract.OrderSizeMin)

	if hasTicker {
		info.MarkPrice = parseFirstFloat(ticker.Last, ticker.MarkPrice)
		info.Volume24h = parseFloat(ticker.Volume24hSettle)
		info.Change24hPct = parseFloat(ticker.ChangePercentage)
	}

	return info, true
}

// baseSymbol 从 Gate 合约名提取基础币种（BTC_USDT -> BTC）
func baseSymbol(contract string) string {
	if idx := strings.Index(contract, "_"); idx > 0 {
		return contract[:idx]
	}
	return contract
}

// parseFirstFloat 依次取第一个非零可解析值
func parseFirstFloat(values ...string) float64 {
	for _, v := range values {
		if f := parseFloat(v); f != 0 {
			return f
		}
	}
	return 0
}

func parseFloat(v string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(v), 64)
	return f
}
