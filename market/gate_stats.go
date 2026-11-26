package market

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Gate 公开行情无需认证；限频为「单个接口 200r/10s（按 IP）」，由全部 trader、图表与助手共用，
// 取数经 gate_cache.go 做短 TTL 缓存与并发合并。
// var 而非 const，便于单测注入本地 httptest 服务。
var gateFuturesBaseURL = "https://api.gateio.ws/api/v4"

// gateContractStat GET /futures/usdt/contract_stats 单期统计。
// open_interest_usd 为 USD 计价的持仓量价值；last_funding_rate 为该期时点最近一次已结算资金费率。
// 数值字段用 json.Number：Gate 不带 X-Gate-Size-Decimal 头时返回 JSON number，带 `:1` 时返回字符串。
type gateContractStat struct {
	Time            int64       `json:"time"`
	OpenInterestUSD json.Number `json:"open_interest_usd"`
	LastFundingRate json.Number `json:"last_funding_rate"`
}

// toGateContractSymbol 把内部符号（BTCUSDT / BTC_USDT / BTC-USDTSWAP）转成 Gate 合约名（BTC_USDT）。
func toGateContractSymbol(symbol string) string {
	s := strings.ToUpper(strings.TrimSpace(symbol))
	s = strings.NewReplacer("_", "", "-", "").Replace(s)
	s = strings.TrimSuffix(s, "SWAP")
	if strings.HasSuffix(s, "USDT") {
		return strings.TrimSuffix(s, "USDT") + "_USDT"
	}
	return s + "_USDT"
}

// fetchGateContractStats 拉取 Gate USDT 永续合约统计的原始列表（5m 粒度，最多 12 期）。
// 经短 TTL 缓存与并发合并（每轮每币都会调用，请求量随 trader 数线性增长）；429 走 getGateJSON 的分类。
// 返回缓存切片的副本，调用方可以自由排序或改写。
func fetchGateContractStats(symbol string) ([]gateContractStat, string, error) {
	contract := toGateContractSymbol(symbol)
	cached, err := gateCached("stats|"+contract, 10*time.Second, func() (any, error) {
		return fetchGateContractStatsUncached(contract)
	})
	if err != nil {
		return nil, contract, err
	}
	stats := cached.([]gateContractStat)
	out := make([]gateContractStat, len(stats))
	copy(out, stats)
	return out, contract, nil
}

// fetchGateContractStatsUncached 真正访问 Gate 的合约统计取数。
func fetchGateContractStatsUncached(contract string) ([]gateContractStat, error) {
	url := fmt.Sprintf("%s/futures/usdt/contract_stats?contract=%s&interval=5m&limit=12",
		gateFuturesBaseURL, contract)

	var stats []gateContractStat
	if err := getGateJSON(url, &stats); err != nil {
		return nil, fmt.Errorf("gate contract_stats %s: %w", contract, err)
	}
	if len(stats) == 0 {
		return nil, fmt.Errorf("gate contract_stats empty for %s", contract)
	}
	return stats, nil
}

// statsOpenInterest 从统计列表取持仓量：Latest 取时间戳最大的一期，
// Average 为各期持仓价值的 USD 均值（跳过无效值）。
func statsOpenInterest(stats []gateContractStat, contract string) (*OIData, error) {
	latest := stats[0]
	sum := 0.0
	count := 0
	for _, s := range stats {
		if s.Time > latest.Time {
			latest = s
		}
		v, err := s.OpenInterestUSD.Float64()
		if err != nil || v <= 0 {
			continue
		}
		sum += v
		count++
	}
	if count == 0 {
		return nil, fmt.Errorf("gate contract_stats has no valid open_interest_usd for %s", contract)
	}
	latestOI, err := latest.OpenInterestUSD.Float64()
	if err != nil || latestOI <= 0 {
		return nil, fmt.Errorf("gate contract_stats latest open_interest_usd invalid for %s", contract)
	}
	return &OIData{Latest: latestOI, Average: sum / float64(count)}, nil
}

// getGateOpenInterestAndFunding 从 Gate USDT 永续合约统计接口获取持仓量与资金费率。
// 任一环节失败即返回 error，调用方据此把 OI 与资金费率置为不可用（nil）。
func getGateOpenInterestAndFunding(symbol string) (*OIData, *float64, error) {
	stats, contract, err := fetchGateContractStats(symbol)
	if err != nil {
		return nil, nil, err
	}
	oi, err := statsOpenInterest(stats, contract)
	if err != nil {
		return nil, nil, err
	}
	latest := stats[0]
	for _, s := range stats {
		if s.Time > latest.Time {
			latest = s
		}
	}
	rate, err := latest.LastFundingRate.Float64()
	if err != nil {
		return nil, nil, fmt.Errorf("gate contract_stats last_funding_rate invalid for %s: %w", contract, err)
	}
	return oi, &rate, nil
}

// errorBodySnippet 截取响应体片段用于错误信息，避免把整页 5xx HTML 写进日志。
func errorBodySnippet(body []byte) string {
	const maxLen = 200
	if len(body) > maxLen {
		return string(body[:maxLen])
	}
	return strings.TrimSpace(string(body))
}
