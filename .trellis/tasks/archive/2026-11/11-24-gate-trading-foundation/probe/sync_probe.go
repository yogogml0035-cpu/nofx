package main

// 按需触发一次 Gate 订单同步（人工测试用，不启动 AI、不下单）。
// 在容器内运行：挂载 dev-offline 数据卷与工作区，CGO_ENABLED=1。
// 用法：sync_probe -db /data/data.db -trader <id> -exchange <id>
// 凭据经 GATE_KEY / GATE_SECRET 环境变量传入，不落盘、不打印。

import (
	"flag"
	"fmt"
	"os"

	nofxgate "nofx/trader/gate"
	"nofx/store"
)

func main() {
	dbPath := flag.String("db", "/data/data.db", "sqlite database path")
	traderID := flag.String("trader", "", "trader id")
	exchangeID := flag.String("exchange", "", "exchange id")
	flag.Parse()

	key, secret := os.Getenv("GATE_KEY"), os.Getenv("GATE_SECRET")
	if key == "" || secret == "" {
		fmt.Fprintln(os.Stderr, "GATE_KEY / GATE_SECRET 未设置")
		os.Exit(2)
	}
	if *traderID == "" || *exchangeID == "" {
		fmt.Fprintln(os.Stderr, "-trader / -exchange 必填")
		os.Exit(2)
	}

	st, err := store.New(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open store:", err)
		os.Exit(1)
	}
	defer st.Close()

	gt := nofxgate.NewGateTrader(key, secret)
	if err := gt.SyncOrdersFromGate(*traderID, *exchangeID, "gate", st); err != nil {
		fmt.Fprintln(os.Stderr, "sync:", err)
		os.Exit(1)
	}
	fmt.Println("SYNC_OK")

	open, err := st.Position().GetOpenPositions(*traderID)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open positions:", err)
		os.Exit(1)
	}
	for _, p := range open {
		fmt.Printf("OPEN %s %s qty=%.10f entry=%.10f fee=%.10f\n", p.Symbol, p.Side, p.Quantity, p.EntryPrice, p.Fee)
	}
	closed, err := st.Position().GetClosedPositions(*traderID, 20)
	if err != nil {
		fmt.Fprintln(os.Stderr, "closed positions:", err)
		os.Exit(1)
	}
	fmt.Printf("closed_records=%d\n", len(closed))
	for _, p := range closed {
		fmt.Printf("CLOSED %s %s qty=%.10f entry=%.10f exit=%.10f pnl=%.4f fee=%.10f reason=%s\n",
			p.Symbol, p.Side, p.Quantity, p.EntryPrice, p.ExitPrice, p.RealizedPnL, p.Fee, p.CloseReason)
	}
}