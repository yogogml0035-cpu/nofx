//go:build ignore

// W2 真实探针 v2（子任务 11）：价格步长对齐、随单止盈止损在列表中的出现时机、
// 分批平仓后的列表形态（复现 F43）、撤销尝试，以及仓位级撤旧建新改单路径。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"

	"nofx/trader/gate"
	"nofx/trader/types"

	"github.com/antihax/optional"
	"github.com/gate/gateapi-go/v7"
)

const (
	symbol   = "DOGE_USDT"
	contract = "DOGE_USDT"
)

var (
	key    = os.Getenv("GATE_KEY")
	secret = os.Getenv("GATE_SECRET")
	out    *os.File
	step   int
)

func fail(format string, args ...interface{}) {
	fmt.Printf("FAIL: "+format+"\n", args...)
	os.Exit(1)
}

func logf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Println(msg)
	if out != nil {
		step++
		line, _ := json.Marshal(map[string]interface{}{"step": step, "ts": time.Now().Format(time.RFC3339), "msg": msg})
		out.Write(append(line, '\n'))
		out.Sync()
	}
}

func newRawClient() *gateapi.APIClient {
	cfg := gateapi.NewConfiguration()
	cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	return gateapi.NewAPIClient(cfg)
}

func rawCtx() context.Context {
	return context.WithValue(context.Background(), gateapi.ContextGateAPIV4, gateapi.GateAPIV4{Key: key, Secret: secret})
}

// alignPrice 把价格向下/向上对齐到 order_price_round；direction 为 -1 向下、+1 向上
func alignPrice(price float64, tick string, direction int) float64 {
	r, ok := new(big.Rat).SetString(strconv.FormatFloat(price, 'f', -1, 64))
	if !ok {
		fail("parse price %v", price)
	}
	t, ok := new(big.Rat).SetString(tick)
	if !ok {
		fail("parse tick %q", tick)
	}
	q := new(big.Rat).Quo(r, t)
	// 全整除时原样返回
	if q.IsInt() {
		v, _ := r.Float64()
		return v
	}
	steps := new(big.Int).Quo(q.Num(), q.Denom())
	if direction > 0 {
		steps.Add(steps, big.NewInt(1))
	}
	aligned := new(big.Rat).Mul(new(big.Rat).SetInt(steps), t)
	v, _ := aligned.Float64()
	return v
}

type triggerDump struct {
	ID          int64  `json:"id"`
	OrderType   string `json:"order_type"`
	Status      string `json:"status"`
	FinishAs    string `json:"finish_as"`
	TriggerRule int32  `json:"rule"`
	TriggerPx   string `json:"trigger_price"`
	PriceType   int32  `json:"price_type"`
	Strategy    int32  `json:"strategy_type"`
	Size        int64  `json:"size"`
	Amount      string `json:"amount"`
	Close       bool   `json:"close"`
	ReduceOnly  bool   `json:"reduce_only"`
	Tif         string `json:"tif"`
	MeOrderID   int64  `json:"me_order_id"`
}

func dumpTriggerOrders(client *gateapi.APIClient, label string) []gateapi.FuturesPriceTriggeredOrder {
	orders, _, err := client.FuturesApi.ListPriceTriggeredOrders(rawCtx(), "usdt", "open",
		&gateapi.ListPriceTriggeredOrdersOpts{Contract: optional.NewString(contract)})
	if err != nil {
		fail("list trigger orders (%s): %v", label, err)
	}
	dumps := make([]triggerDump, 0, len(orders))
	for _, o := range orders {
		dumps = append(dumps, triggerDump{
			ID: o.Id, OrderType: o.OrderType, Status: o.Status, FinishAs: o.FinishAs,
			TriggerRule: o.Trigger.Rule, TriggerPx: o.Trigger.Price, PriceType: o.Trigger.PriceType,
			Strategy: o.Trigger.StrategyType, Size: o.Initial.Size, Amount: o.Initial.Amount,
			Close: o.Initial.Close, ReduceOnly: o.Initial.ReduceOnly, Tif: o.Initial.Tif,
			MeOrderID: o.MeOrderId,
		})
	}
	sort.Slice(dumps, func(i, j int) bool { return dumps[i].ID < dumps[j].ID })
	raw, _ := json.Marshal(dumps)
	logf("trigger orders (%s, %d): %s", label, len(dumps), string(raw))
	return orders
}

func listOpenFuturesOrders(client *gateapi.APIClient, label string) {
	orders, _, err := client.FuturesApi.ListFuturesOrders(rawCtx(), "usdt", "open",
		&gateapi.ListFuturesOrdersOpts{Contract: optional.NewString(contract)})
	if err != nil {
		fail("list open orders (%s): %v", label, err)
	}
	raw, _ := json.Marshal(orders)
	logf("open futures orders (%s): %s", label, string(raw))
}

func positionSize(client *gateapi.APIClient) string {
	positions, _, err := client.FuturesApi.ListPositions(rawCtx(), "usdt", nil)
	if err != nil {
		fail("list positions: %v", err)
	}
	for _, p := range positions {
		if p.Contract == contract {
			size, _ := strconv.ParseFloat(p.Size, 64)
			if size != 0 {
				raw, _ := json.Marshal(p)
				logf("position: %s", string(raw))
				return p.Size
			}
		}
	}
	logf("position: none")
	return "0"
}

func main() {
	cleanup := flag.Bool("cleanup", false, "close the leftover DOGE position from an aborted probe run and exit")
	flag.Parse()
	if key == "" || secret == "" {
		fail("GATE_KEY and GATE_SECRET must be set")
	}
	if path := flag.Arg(0); path != "" {
		f, err := os.Create(path)
		if err != nil {
			fail("create out: %v", err)
		}
		out = f
		defer out.Close()
	}

	client := newRawClient()
	t := gate.NewGateTrader(key, secret)

	if *cleanup {
		if positionSize(client) != "0" {
			if _, err := t.CloseLong(symbol, 0); err != nil {
				fail("cleanup close: %v", err)
			}
			time.Sleep(2 * time.Second)
		}
		if positionSize(client) != "0" {
			fail("cleanup: position still present")
		}
		for _, o := range dumpTriggerOrders(client, "cleanup") {
			if _, _, err := client.FuturesApi.CancelPriceTriggeredOrder(rawCtx(), "usdt", o.Id); err != nil {
				fail("cleanup cancel %d: %v", o.Id, err)
			}
		}
		if len(dumpTriggerOrders(client, "cleanup final")) != 0 {
			fail("cleanup: trigger orders remain")
		}
		logf("CLEANUP_PASS")
		return
	}

	if positionSize(client) != "0" {
		fail("precheck: position exists")
	}
	if len(dumpTriggerOrders(client, "precheck")) != 0 {
		fail("precheck: trigger orders exist")
	}
	listOpenFuturesOrders(client, "precheck")

	info, _, err := client.FuturesApi.GetFuturesContract(rawCtx(), "usdt", contract)
	if err != nil {
		fail("get contract: %v", err)
	}
	logf("contract: order_price_round=%q mark_price_round=%q quanto=%q order_size_min=%q enable_decimal=%v",
		info.OrderPriceRound, info.MarkPriceRound, info.QuantoMultiplier, info.OrderSizeMin, info.EnableDecimal)
	tick := info.OrderPriceRound
	if tick == "" {
		fail("contract has no order_price_round")
	}

	price, err := t.GetMarketPrice(symbol)
	if err != nil {
		fail("get price: %v", err)
	}
	// 多头止损在下方、止盈在上方；对齐方向：止损向下、止盈向上（远离现价，保守）
	slPrice := alignPrice(price*0.90, tick, -1)
	tpPrice := alignPrice(price*1.10, tick, +1)
	logf("price=%.8f tick=%s sl=%.8f tp=%.8f", price, tick, slPrice, tpPrice)

	opened, err := t.OpenLongProtected(types.ProtectedOpenRequest{
		Symbol:        symbol,
		NotionalUSD:   9.3,
		Leverage:      10,
		IsCrossMargin: true,
		StopLoss:      slPrice,
		TakeProfit:    tpPrice,
		MinNotional:   1,
		SlipRatio:     0.05,
	})
	if err != nil {
		fail("open: %v", err)
	}
	logf("opened: orderId=%s contracts=%s notional=%.4f fill=%.6f sl=%.8f tp=%.8f",
		opened.OrderID, opened.Contracts, opened.Notional, opened.FillPrice, slPrice, tpPrice)

	dumpTriggerOrders(client, "after open immediate")
	time.Sleep(5 * time.Second)
	dumpTriggerOrders(client, "after open +5s")
	time.Sleep(10 * time.Second)
	dumpTriggerOrders(client, "after open +15s")
	time.Sleep(15 * time.Second)
	dumpTriggerOrders(client, "after open +30s")
	listOpenFuturesOrders(client, "after open +30s")

	// 分批平仓 50%，复现 F43 的 2 条条件单
	partial, err := t.ClosePartial(types.PartialCloseRequest{Symbol: symbol, Side: "LONG", Percent: 50})
	if err != nil {
		fail("partial close: %v", err)
	}
	logf("partial close: orderId=%s contracts=%s remaining=%s fill=%v",
		partial.OrderID, partial.Contracts, partial.Remaining, partial.FillPrice)
	afterPartial := dumpTriggerOrders(client, "after partial close")
	positionSize(client)

	// 对列表里的每条条件单尝试撤销（记录交易所是否允许）
	for _, o := range afterPartial {
		label := "cancel " + strconv.FormatInt(o.Id, 10) + " type=" + o.OrderType
		_, _, err := client.FuturesApi.CancelPriceTriggeredOrder(rawCtx(), "usdt", o.Id)
		if err != nil {
			logf("%s rejected: %v", label, err)
		} else {
			logf("%s succeeded", label)
		}
	}
	dumpTriggerOrders(client, "after cancel attempts")

	// 仓位级保护单：对齐价格，撤销后重建
	placeKind := func(kind gate.ProtectiveKind, trigger float64) string {
		order, err := gate.BuildPositionProtectiveOrder(contract, "LONG", kind, trigger)
		if err != nil {
			fail("build %v at %v: %v", kind, trigger, err)
		}
		id, err := t.PlaceProtectiveOrder(order)
		if err != nil {
			fail("place %v at %v: %v", kind, trigger, err)
		}
		logf("placed %v at %v id=%s", kind, trigger, id)
		return id
	}
	placeKind(gate.ProtectiveStopLoss, slPrice)
	placeKind(gate.ProtectiveTakeProfit, tpPrice)
	dumpTriggerOrders(client, "after position-level SL+TP")

	// 改止损（向上收紧），撤旧建新
	newSL := alignPrice(price*0.97, tick, +1)
	cancelled, err := t.CancelProtectiveOrdersByKind(symbol, gate.ProtectiveStopLoss)
	if err != nil {
		fail("cancel SL: %v", err)
	}
	logf("cancelled SL ids: %v", cancelled)
	views, _ := t.ListTriggerOrders(symbol)
	for _, v := range views {
		logf("after SL cancel: id=%s kind=%v trigger=%.8f", v.OrderID(), v.Kind, v.TriggerPrice)
	}
	placeKind(gate.ProtectiveStopLoss, newSL)
	dumpTriggerOrders(client, "after SL replace")

	// 改止盈（向下移动，D32 允许双向），撤旧建新
	newTP := alignPrice(price*1.05, tick, -1)
	cancelled, err = t.CancelProtectiveOrdersByKind(symbol, gate.ProtectiveTakeProfit)
	if err != nil {
		fail("cancel TP: %v", err)
	}
	logf("cancelled TP ids: %v", cancelled)
	placeKind(gate.ProtectiveTakeProfit, newTP)
	dumpTriggerOrders(client, "after TP replace")

	// 全平并核对终态
	if _, err := t.CloseLong(symbol, 0); err != nil {
		fail("full close: %v", err)
	}
	time.Sleep(2 * time.Second)
	positionSize(client)
	if len(dumpTriggerOrders(client, "after close")) != 0 {
		fail("after close: open trigger orders remain")
	}
	listOpenFuturesOrders(client, "after close")
	if positionSize(client) != "0" {
		fail("after close: position remains")
	}
	logf("LIVECHECK_PASS")
}
