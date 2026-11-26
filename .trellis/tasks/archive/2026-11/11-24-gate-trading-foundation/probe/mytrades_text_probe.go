package main

// 只读探针：列出 Gate 合约账户最近的成交记录里带 text（下单备注）的条目，
// 用来确认「按下单备注区分 trader 开仓与其他入口的成交」在数据上成立。不下单、不改任何设置。
// 凭据从 GATE_KEY / GATE_SECRET 环境变量读取，不落盘、不打印。

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/gate/gateapi-go/v7"
)

func main() {
	key, secret := os.Getenv("GATE_KEY"), os.Getenv("GATE_SECRET")
	if key == "" || secret == "" {
		fmt.Fprintln(os.Stderr, "GATE_KEY / GATE_SECRET 未设置")
		os.Exit(2)
	}
	ctx := context.WithValue(context.Background(), gateapi.ContextGateAPIV4, gateapi.GateAPIV4{Key: key, Secret: secret})
	client := gateapi.NewAPIClient(gateapi.NewConfiguration())
	trades, _, err := client.FuturesApi.GetMyTrades(ctx, "usdt", nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "GetMyTrades:", err)
		os.Exit(1)
	}
	type row struct {
		ID         int64  `json:"id"`
		CreateTime string `json:"create_time"`
		Contract   string `json:"contract"`
		Text       string `json:"text"`
		Size       string `json:"size"`
		CloseSize  string `json:"close_size"`
		OrderID    string `json:"order_id"`
	}
	out := make([]row, 0, len(trades))
	texts := map[string]int{}
	for _, t := range trades {
		texts[t.Text]++
		out = append(out, row{ID: t.Id, CreateTime: time.Unix(int64(t.CreateTime), 0).UTC().Format(time.RFC3339),
			Contract: t.Contract, Text: t.Text, Size: t.Size, CloseSize: t.CloseSize, OrderID: t.OrderId})
	}
	enc := json.NewEncoder(os.Stdout)
	for _, r := range out {
		_ = enc.Encode(r)
	}
	summary, _ := json.Marshal(map[string]any{"count": len(out), "texts": texts})
	fmt.Fprintln(os.Stderr, string(summary))
}
