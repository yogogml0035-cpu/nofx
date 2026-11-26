package main

// 立即平掉上一步探针留下的 DOGE 仓位，并打印最近成交的 text/order_id 以核对归属。

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/gate/gateapi-go/v7"

	nofxgate "nofx/trader/gate"
)

func main() {
	key, secret := os.Getenv("GATE_KEY"), os.Getenv("GATE_SECRET")
	gt := nofxgate.NewGateTrader(key, secret)
	ctx := context.WithValue(context.Background(), gateapi.ContextGateAPIV4, gateapi.GateAPIV4{Key: key, Secret: secret})
	raw := gateapi.NewAPIClient(gateapi.NewConfiguration())
	positions, _, err := raw.FuturesApi.ListPositions(ctx, "usdt", nil)
	if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
	for _, p := range positions {
		if p.Contract == "DOGE_USDT" && p.Size != "0" {
			fmt.Printf("closing DOGE position size=%s\n", p.Size)
			order, err := gt.CloseLong("DOGE_USDT", 0)
			fmt.Printf("close result=%v err=%v\n", order, err)
		}
	}
	time.Sleep(800 * time.Millisecond)
	trades, _, err := raw.FuturesApi.GetMyTrades(ctx, "usdt", nil)
	if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
	for i := 0; i < 6 && i < len(trades); i++ {
		t := trades[i]
		fmt.Printf("trade id=%d order_id=%s text=%q size=%s close_size=%s\n", t.Id, t.OrderId, t.Text, t.Size, t.CloseSize)
	}
	positions, _, _ = raw.FuturesApi.ListPositions(ctx, "usdt", nil)
	n := 0
	for _, p := range positions { if p.Size != "0" { n++ } }
	fmt.Printf("remaining_open_positions=%d\n", n)
	acc, _, accErr := raw.FuturesApi.ListFuturesAccounts(ctx, "usdt")
	if accErr == nil { fmt.Printf("available=%s\n", acc.Available) }
}
