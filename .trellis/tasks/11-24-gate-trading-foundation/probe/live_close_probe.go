package main

// 人工下单测试的受控平仓工具（真实账户，测试授权）。
// 用法：
//   live_close_probe partial  —— 把 BTC_USDT 多头平掉 50%（验证分批平仓成交在同步中只应用一次）
//   live_close_probe full     —— 把账户内所有非零仓位市价全平（验证全平成交只应用一次）
// 不下新开仓、不改设置；凭据经 GATE_KEY / GATE_SECRET 环境变量传入，不落盘、不打印。

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/gate/gateapi-go/v7"

	nofxgate "nofx/trader/gate"
	"nofx/trader/types"
)

func main() {
	key, secret := os.Getenv("GATE_KEY"), os.Getenv("GATE_SECRET")
	if key == "" || secret == "" {
		fmt.Fprintln(os.Stderr, "GATE_KEY / GATE_SECRET 未设置")
		os.Exit(2)
	}
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: live_close_probe partial|full")
		os.Exit(2)
	}
	ctx := context.WithValue(context.Background(), gateapi.ContextGateAPIV4, gateapi.GateAPIV4{Key: key, Secret: secret})
	raw := gateapi.NewAPIClient(gateapi.NewConfiguration())
	gt := nofxgate.NewGateTrader(key, secret)

	switch os.Args[1] {
	case "open":
		// 受控开 2 张 BTC 多头（随单 SL/TP，备注 t-nofx-ai），用于验证带修复的分批平仓。
		price, err := gt.GetMarketPrice("BTC_USDT")
		if err != nil {
			fmt.Fprintln(os.Stderr, "price:", err)
			os.Exit(1)
		}
		result, err := gt.OpenLongProtected(types.ProtectedOpenRequest{
			Symbol: "BTC_USDT", NotionalUSD: 12.4, Leverage: 5, IsCrossMargin: true,
			StopLoss: price * 0.90, TakeProfit: price * 1.10,
			MinNotional: 10, SlipRatio: 0.01,
			Principal: 107.36, MaxLossPercent: 2,
		})
		fmt.Printf("open_protected result=%+v err=%v price=%.2f\n", result, err, price)
	case "partial":
		res, err := gt.ClosePartial(types.PartialCloseRequest{Symbol: "BTC_USDT", Side: "long", Percent: 50})
		fmt.Printf("partial_close result=%+v err=%v\n", res, err)
	case "full":
		positions, _, err := raw.FuturesApi.ListPositions(ctx, "usdt", nil)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		for _, p := range positions {
			if p.Size == "" || p.Size == "0" {
				continue
			}
			if p.Size[0] == '-' {
				order, err := gt.CloseShort(p.Contract, 0)
				fmt.Printf("close_short %s order=%v err=%v\n", p.Contract, order, err)
			} else {
				order, err := gt.CloseLong(p.Contract, 0)
				fmt.Printf("close_long %s order=%v err=%v\n", p.Contract, order, err)
			}
		}
	default:
		fmt.Fprintln(os.Stderr, "unknown mode")
		os.Exit(2)
	}

	time.Sleep(1200 * time.Millisecond)
	trades, _, err := raw.FuturesApi.GetMyTrades(ctx, "usdt", nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for i := 0; i < 6 && i < len(trades); i++ {
		t := trades[i]
		fmt.Printf("trade id=%d order_id=%s text=%q size=%s close_size=%s price=%s\n",
			t.Id, t.OrderId, t.Text, t.Size, t.CloseSize, t.Price)
	}
	positions, _, _ := raw.FuturesApi.ListPositions(ctx, "usdt", nil)
	n := 0
	for _, p := range positions {
		if p.Size != "" && p.Size != "0" {
			fmt.Printf("remaining %s size=%s entry=%s\n", p.Contract, p.Size, p.EntryPrice)
			n++
		}
	}
	fmt.Printf("remaining_open_positions=%d\n", n)
	acc, _, accErr := raw.FuturesApi.ListFuturesAccounts(ctx, "usdt")
	if accErr == nil {
		fmt.Printf("available=%s\n", acc.Available)
	}
}