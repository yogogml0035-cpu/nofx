package market

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Gate 公开行情无需认证；合约行情限频 200r/10s（按 IP），远高于本服务的调用密度。
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

// getGateOpenInterestAndFunding 从 Gate USDT 永续合约统计接口获取持仓量与资金费率。
// 返回值为 USD 口径：Latest/Average 为最近一期与近 12 期（5 分钟粒度）持仓价值均值；
// fundingRate 为最近一期时点的已结算资金费率。任一字段取不到即整体视为不可用，由调用方置 nil。
func getGateOpenInterestAndFunding(symbol string) (*OIData, *float64, error) {
	url := fmt.Sprintf("%s/futures/usdt/contract_stats?contract=%s&interval=5m&limit=12",
		gateFuturesBaseURL, toGateContractSymbol(symbol))

	apiClient := NewAPIClient()
	resp, err := apiClient.client.Get(url)
	if err != nil {
		return nil, nil, fmt.Errorf("gate contract_stats request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("gate contract_stats read body failed: %w", err)
	}

	var stats []gateContractStat
	if err := json.Unmarshal(body, &stats); err != nil {
		return nil, nil, fmt.Errorf("gate contract_stats decode failed: %w", err)
	}
	if len(stats) == 0 {
		return nil, nil, fmt.Errorf("gate contract_stats empty for %s", symbol)
	}

	// 返回按时间升序，最后一期为最新
	sum := 0.0
	count := 0
	for _, s := range stats {
		v, err := s.OpenInterestUSD.Float64()
		if err != nil || v <= 0 {
			continue
		}
		sum += v
		count++
	}
	if count == 0 {
		return nil, nil, fmt.Errorf("gate contract_stats has no valid open_interest_usd for %s", symbol)
	}
	latest, err := stats[len(stats)-1].OpenInterestUSD.Float64()
	if err != nil || latest <= 0 {
		return nil, nil, fmt.Errorf("gate contract_stats latest open_interest_usd invalid for %s", symbol)
	}

	rate, err := stats[len(stats)-1].LastFundingRate.Float64()
	if err != nil {
		return nil, nil, fmt.Errorf("gate contract_stats last_funding_rate invalid for %s: %w", symbol, err)
	}

	return &OIData{
		Latest:  latest,
		Average: sum / float64(count),
	}, &rate, nil
}
