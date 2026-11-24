package main

// 只读验证（子任务 contract-math）：经项目适配层读取 7 个合约的元数据，与 SDK 原始字段逐项对照，
// 并用当前最新价换算下单张数与最小可交易额。只调用公开的合约查询与 ticker，脚本里没有任何写操作。
// 凭据只用于按生产路径构造适配器，经环境变量 GATE_KEY / GATE_SECRET 传入，不落盘、不打印。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gate/gateapi-go/v7"

	nofxgate "nofx/trader/gate"
)

var contracts = []string{"BTC_USDT", "ETH_USDT", "SOL_USDT", "ZEC_USDT", "HYPE_USDT", "DOGE_USDT", "XAU_USDT"}

type quoteLine struct {
	Notional      float64 `json:"notional_usdt"`
	Orderable     bool    `json:"orderable"`
	Contracts     string  `json:"contracts,omitempty"`
	OrderNotional float64 `json:"order_notional_usdt"`
	Skip          string  `json:"skip,omitempty"`
}

type contractLine struct {
	Contract          string         `json:"contract"`
	Raw               map[string]any `json:"raw"`
	Spec              map[string]any `json:"spec"`
	MatchesRaw        bool           `json:"matches_raw"`
	LastPrice         float64        `json:"last_price"`
	ValuePerContract  float64        `json:"value_per_contract_usdt"`
	MinOrderValue     float64        `json:"min_order_value_usdt"`
	MinOrderContracts string         `json:"min_order_contracts"`
	Quotes            []quoteLine    `json:"quotes"`
	Error             string         `json:"error,omitempty"`
}

func main() {
	key, secret := os.Getenv("GATE_KEY"), os.Getenv("GATE_SECRET")
	if key == "" || secret == "" {
		panic("GATE_KEY / GATE_SECRET 未设置")
	}

	gt := nofxgate.NewGateTrader(key, secret)

	// 不带凭据的客户端，用来取 SDK 的原始返回做对照（公开接口）
	cfg := gateapi.NewConfiguration()
	cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	client := gateapi.NewAPIClient(cfg)
	ctx := context.Background()

	const minNotional = 10
	// 计划文档 6.1 节的 10U、20U，以及当前本金 107.5U 下的 10% 与 20%
	notionals := []float64{10, 10.75, 20, 21.5}

	failed, mismatched := 0, 0
	for _, name := range contracts {
		line := contractLine{Contract: name, Quotes: []quoteLine{}}

		raw, _, err := client.FuturesApi.GetFuturesContract(ctx, "usdt", name)
		if err != nil {
			line.Error = fmt.Sprintf("raw: %v", err)
			failed++
			print(line)
			continue
		}
		line.Raw = map[string]any{
			"quanto_multiplier": raw.QuantoMultiplier, "order_size_min": raw.OrderSizeMin,
			"order_size_max": raw.OrderSizeMax, "market_order_size_max": raw.MarketOrderSizeMax,
			"enable_decimal": raw.EnableDecimal,
		}

		spec, err := gt.GetContractSpec(name)
		if err != nil {
			line.Error = fmt.Sprintf("spec: %v", err)
			failed++
			print(line)
			continue
		}
		line.Spec = map[string]any{
			"quanto_multiplier": spec.QuantoMultiplier, "size_step": spec.SizeStep,
			"size_max": spec.SizeMax, "market_order_size_max": spec.MarketOrderSizeMax,
		}
		line.MatchesRaw = spec.QuantoMultiplier == raw.QuantoMultiplier &&
			spec.SizeStep == raw.OrderSizeMin && spec.SizeMax == raw.OrderSizeMax &&
			(spec.MarketOrderSizeMax == raw.MarketOrderSizeMax || (raw.MarketOrderSizeMax == "" && spec.MarketOrderSizeMax == "0"))
		if !line.MatchesRaw {
			mismatched++
		}

		price, err := gt.GetMarketPrice(name)
		if err != nil {
			line.Error = fmt.Sprintf("price: %v", err)
			failed++
			print(line)
			continue
		}
		line.LastPrice = price

		one, err := spec.QuantityFromContracts("1") // 一张合约对应的标的币数量
		if err != nil {
			line.Error = fmt.Sprintf("one contract: %v", err)
			failed++
			print(line)
			continue
		}
		line.ValuePerContract = one * price

		for _, notional := range notionals {
			quote, err := spec.SizeForNotional(price, notional, minNotional)
			if err != nil {
				line.Error = fmt.Sprintf("quote(%v): %v", notional, err)
				break
			}
			line.MinOrderValue = quote.MinOrderValue
			entry := quoteLine{
				Notional: notional, Orderable: quote.Orderable(), Contracts: quote.Contracts, OrderNotional: quote.Notional,
			}
			if !entry.Orderable {
				entry.Skip = quote.Skip.String()
			}
			line.Quotes = append(line.Quotes, entry)
		}
		if line.Error == "" {
			// 按最小可交易额下单必须成功，由此得到最小可下单的张数
			minQuote, err := spec.SizeForNotional(price, line.MinOrderValue, minNotional)
			if err != nil || !minQuote.Orderable() {
				line.Error = fmt.Sprintf("min order value %v is not orderable: err=%v skip=%v",
					line.MinOrderValue, err, minQuote.Skip)
				failed++
			} else {
				line.MinOrderContracts = minQuote.Contracts
			}
		}
		print(line)
	}

	fmt.Printf("contracts=%d failed=%d mismatched=%d\n", len(contracts), failed, mismatched)
	if failed > 0 || mismatched > 0 {
		os.Exit(1)
	}
}

func print(line contractLine) {
	raw, err := json.Marshal(line)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(raw))
}
