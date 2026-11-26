package main

// 最终候选的真实随单保护开平验证（W2 授权范围，单笔名义 ≤10 USDT，每一步后立即还原）：
//   1. 预检：账户无持仓、无未完结委托/条件单；记录可用余额与 my_trades 基线
//   2. 经交付适配器 OpenLongProtected 开 17 张 DOGE（约 9.5U），随单止损/止盈 ±10%
//   3. VerifyOrderProtection 核对未触发条件单
//   4. 读 my_trades：本次开仓成交的 text 必须为 t-nofx-ai（仓位归属依据）
//   5. CloseLong(0) 全平（校验成交确认）
//   6. 终态：无持仓、无未完结委托/条件单，可用余额只差手续费
// 同步层的归属规则（只认 t-nofx-ai 的开仓、平仓只作用于已有 OPEN 记录）由单测覆盖：
// 宿主机没有 cgo，store/sqlite 在 darwin 交叉编译下不可用。
//
// 凭据从 GATE_KEY / GATE_SECRET 环境变量读取，不落盘、不打印。容器出口走 VPN 不在白名单：
// Docker 内交叉编译 darwin/arm64，宿主机运行。证据 JSONL 写到 -out 指定的文件。

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/gate/gateapi-go/v7"

	nofxgate "nofx/trader/gate"
	"nofx/trader/types"
)

const (
	symbol   = "DOGE_USDT"
	notional = 9.5
	slip     = 0.01
)

var failures int

func emit(out *os.File, event map[string]interface{}) {
	event["ts"] = time.Now().UTC().Format(time.RFC3339)
	data, err := json.Marshal(event)
	if err != nil {
		panic(err)
	}
	fmt.Fprintln(out, string(data))
}

func step(out *os.File, name string, ok bool, detail map[string]interface{}) {
	if detail == nil {
		detail = map[string]interface{}{}
	}
	detail["step"] = name
	if ok {
		detail["result"] = "PASS"
	} else {
		detail["result"] = "FAIL"
		failures++
	}
	emit(out, detail)
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	var gateErr gateapi.GateAPIError
	if errors.As(err, &gateErr) {
		return fmt.Sprintf("label=%s message=%s", gateErr.Label, gateErr.GetMessage())
	}
	return err.Error()
}

func main() {
	os.Exit(run())
}

func run() int {
	outPath := flag.String("out", "", "证据文件（JSONL）")
	flag.Parse()
	if *outPath == "" {
		fmt.Fprintln(os.Stderr, "-out is required")
		return 2
	}
	out, err := os.Create(*outPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "无法创建证据文件:", err)
		return 2
	}
	defer out.Close()

	key, secret := os.Getenv("GATE_KEY"), os.Getenv("GATE_SECRET")
	if key == "" || secret == "" {
		fmt.Fprintln(os.Stderr, "GATE_KEY / GATE_SECRET 未设置")
		return 2
	}

	gt := nofxgate.NewGateTrader(key, secret)
	ctx := context.WithValue(context.Background(), gateapi.ContextGateAPIV4,
		gateapi.GateAPIV4{Key: key, Secret: secret})
	rawClient := gateapi.NewAPIClient(gateapi.NewConfiguration())

	// ===== 预检 =====
	account, _, err := rawClient.FuturesApi.ListFuturesAccounts(ctx, "usdt")
	if err != nil {
		step(out, "preflight account", false, map[string]interface{}{"error": errText(err)})
		return 1
	}
	availableBefore := account.Available
	positions, _, err := rawClient.FuturesApi.ListPositions(ctx, "usdt", nil)
	if err != nil {
		step(out, "preflight positions", false, map[string]interface{}{"error": errText(err)})
		return 1
	}
	openPositions := 0
	for _, pos := range positions {
		if size, err := strconv.ParseFloat(pos.Size, 64); err == nil && size != 0 {
			openPositions++
		}
	}
	// 不动用户的手动持仓：账户必须完全空闲
	if openPositions != 0 {
		step(out, "preflight positions", false, map[string]interface{}{"open_positions": openPositions})
		return 1
	}
	orders, _, err := rawClient.FuturesApi.ListFuturesOrders(ctx, "usdt", "open", nil)
	if err != nil {
		step(out, "preflight orders", false, map[string]interface{}{"error": errText(err)})
		return 1
	}
	triggers, _, err := rawClient.FuturesApi.ListPriceTriggeredOrders(ctx, "usdt", "open", nil)
	if err != nil {
		step(out, "preflight price orders", false, map[string]interface{}{"error": errText(err)})
		return 1
	}
	idle := len(orders) == 0 && len(triggers) == 0
	step(out, "preflight idle", idle, map[string]interface{}{
		"available": availableBefore, "open_orders": len(orders), "open_price_orders": len(triggers)})
	if !idle {
		return 1
	}

	tradesBefore, _, err := rawClient.FuturesApi.GetMyTrades(ctx, "usdt", nil)
	if err != nil {
		step(out, "preflight my_trades", false, map[string]interface{}{"error": errText(err)})
		return 1
	}
	// 成交 ID 按合约递增（不同合约不共享序列），归属匹配必须按下单 ID，不能用全局最大 ID 过滤
	step(out, "preflight my_trades", true, map[string]interface{}{"count": len(tradesBefore)})

	price, err := gt.GetMarketPrice(symbol)
	if err != nil {
		step(out, "preflight price", false, map[string]interface{}{"error": errText(err)})
		return 1
	}

	// ===== 开仓（随单保护）=====
	principal, _ := strconv.ParseFloat(availableBefore, 64)
	req := types.ProtectedOpenRequest{
		Symbol: symbol, NotionalUSD: notional, Leverage: 10, IsCrossMargin: true,
		StopLoss: price * 0.90, TakeProfit: price * 1.10,
		// 探针是适配器级验证：名义保持 9.5U（≤10U 授权上限）。DOGE 当前价格下 9.5U 取整后
		// 低于策略默认最小下单 10U，最小下单策略由单测与既有 W2 覆盖，这里不重复。
		MinNotional: 0, SlipRatio: slip,
		Principal: principal, MaxLossPercent: 2,
	}
	result, err := gt.OpenLongProtected(req)
	if err != nil {
		step(out, "1 open protected", false, map[string]interface{}{"error": errText(err), "price": price})
		return 1
	}
	step(out, "1 open protected", true, map[string]interface{}{
		"order_id": result.OrderID, "contracts": result.Contracts, "fill_price": result.FillPrice,
		"notional": result.Notional, "sl": result.StopLoss, "tp": result.TakeProfit, "price": price})

	closed := false
	cleanup := func(name string) {
		if closed {
			return
		}
		closed = true
		if _, closeErr := gt.CloseLong(symbol, 0); closeErr != nil {
			step(out, name, false, map[string]interface{}{"error": errText(closeErr)})
			return
		}
		step(out, name, true, nil)
	}
	defer cleanup("cleanup close (deferred)")

	// ===== 保护核对 =====
	verifyErr := gt.VerifyOrderProtection(symbol, result.OrderID)
	step(out, "2 verify protection", verifyErr == nil, map[string]interface{}{"error": errText(verifyErr)})

	// ===== my_trades 的 text 归属 =====
	var openTrade *gateapi.MyFuturesTrade
	for attempt := 1; attempt <= 20; attempt++ {
		trades, _, err := rawClient.FuturesApi.GetMyTrades(ctx, "usdt", nil)
		if err != nil {
			step(out, "3 my_trades text", false, map[string]interface{}{"error": errText(err)})
			break
		}
		for i := range trades {
			if trades[i].OrderId == result.OrderID && trades[i].Contract == symbol {
				openTrade = &trades[i]
				break
			}
		}
		if openTrade != nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	textOK := openTrade != nil && openTrade.Text == "t-nofx-ai"
	detail := map[string]interface{}{}
	if openTrade != nil {
		detail["trade_id"] = openTrade.Id
		detail["order_id"] = openTrade.OrderId
		detail["text"] = openTrade.Text
		detail["size"] = openTrade.Size
	}
	step(out, "3 my_trades text == t-nofx-ai", textOK, detail)

	// ===== 全平（校验成交确认）=====
	closed = true
	if _, closeErr := gt.CloseLong(symbol, 0); closeErr != nil {
		step(out, "4 close all", false, map[string]interface{}{"error": errText(closeErr)})
	} else {
		step(out, "4 close all", true, nil)
	}

	// ===== 终态 =====
	time.Sleep(1 * time.Second)
	positions, _, err = rawClient.FuturesApi.ListPositions(ctx, "usdt", nil)
	openPositions = 0
	if err == nil {
		for _, pos := range positions {
			if size, err := strconv.ParseFloat(pos.Size, 64); err == nil && size != 0 {
				openPositions++
			}
		}
	}
	orders, _, err = rawClient.FuturesApi.ListFuturesOrders(ctx, "usdt", "open", nil)
	triggers, _, err = rawClient.FuturesApi.ListPriceTriggeredOrders(ctx, "usdt", "open", nil)
	account, _, err = rawClient.FuturesApi.ListFuturesAccounts(ctx, "usdt")
	availableAfter := ""
	if err == nil {
		availableAfter = account.Available
	}
	clean := openPositions == 0 && len(orders) == 0 && len(triggers) == 0
	diff := ""
	if before, err1 := strconv.ParseFloat(availableBefore, 64); err1 == nil {
		if after, err2 := strconv.ParseFloat(availableAfter, 64); err2 == nil {
			diff = strconv.FormatFloat(after-before, 'f', 8, 64)
		}
	}
	step(out, "5 final state", clean, map[string]interface{}{
		"open_positions": openPositions, "open_orders": len(orders), "open_price_orders": len(triggers),
		"available_before": availableBefore, "available_after": availableAfter, "available_diff": diff})

	if failures > 0 {
		fmt.Fprintf(out, "LIVECHECK_FAIL failures=%d\n", failures)
		fmt.Fprintf(os.Stderr, "LIVECHECK_FAIL failures=%d\n", failures)
		return 1
	}
	fmt.Fprintln(out, "LIVECHECK_PASS")
	fmt.Fprintln(os.Stderr, "LIVECHECK_PASS")
	return 0
}
