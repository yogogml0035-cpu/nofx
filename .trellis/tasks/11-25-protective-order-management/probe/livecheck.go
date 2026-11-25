//go:build ignore

// 子任务 11 最终验收探针（W2 改止损止盈 + W3 级 ETH）：
//
//	DOGE：随单保护开 17 张 → 列表核对 → 分批 50% → 读当前保护 → 改止损（收紧）→ 改止盈（向现价）
//	     → 列表核对 → 全平 → 零残留
//	ETH：随单保护开 0.1 张 → 列表核对（委托级 close-long-order 可见）→ 改止损 → 列表核对 → 全平
//
// 全部走子任务 11 交付的适配器接口（AlignTriggerPrice / PositionProtection / UpdateProtection）。
// 凭据经环境变量传入，不落盘、不打印。运行前须无持仓、无条件单、无挂单。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"

	"nofx/trader/gate"
	"nofx/trader/types"

	"github.com/antihax/optional"
	"github.com/gate/gateapi-go/v7"
)

var (
	key    = os.Getenv("GATE_KEY")
	secret = os.Getenv("GATE_SECRET")
	out    *os.File
	step   int
)

const (
	doge = "DOGE_USDT"
	eth  = "ETH_USDT"
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

func rawCtx() context.Context {
	return context.WithValue(context.Background(), gateapi.ContextGateAPIV4, gateapi.GateAPIV4{Key: key, Secret: secret})
}

func client() *gateapi.APIClient {
	cfg := gateapi.NewConfiguration()
	cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	return gateapi.NewAPIClient(cfg)
}

type triggerDump struct {
	ID         int64  `json:"id"`
	IDString   string `json:"id_string"`
	OrderType  string `json:"order_type"`
	Status     string `json:"status"`
	Rule       int32  `json:"rule"`
	TriggerPx  string `json:"trigger_price"`
	Size       int64  `json:"size"`
	Amount     string `json:"amount"`
	Close      bool   `json:"close"`
	ReduceOnly bool   `json:"is_reduce_only"`
	MeOrderID  int64  `json:"me_order_id"`
}

func listTriggers(c *gateapi.APIClient, contract string) []gateapi.FuturesPriceTriggeredOrder {
	orders, _, err := c.FuturesApi.ListPriceTriggeredOrders(rawCtx(), "usdt", "open",
		&gateapi.ListPriceTriggeredOrdersOpts{Contract: optional.NewString(contract)})
	if err != nil {
		fail("list trigger orders for %s: %v", contract, err)
	}
	return orders
}

func dumpTriggers(c *gateapi.APIClient, contract, label string) []gateapi.FuturesPriceTriggeredOrder {
	orders := listTriggers(c, contract)
	dumps := make([]triggerDump, 0, len(orders))
	for _, o := range orders {
		dumps = append(dumps, triggerDump{
			ID: o.Id, IDString: o.IdString, OrderType: o.OrderType, Status: o.Status,
			Rule: o.Trigger.Rule, TriggerPx: o.Trigger.Price, Size: o.Initial.Size,
			Amount: o.Initial.Amount, Close: o.Initial.Close, ReduceOnly: o.Initial.IsReduceOnly,
			MeOrderID: o.MeOrderId,
		})
	}
	raw, _ := json.Marshal(dumps)
	logf("trigger orders (%s, %s, %d): %s", contract, label, len(dumps), string(raw))
	return orders
}

func positionOf(c *gateapi.APIClient, contract string) gateapi.Position {
	list, _, err := c.FuturesApi.ListPositions(rawCtx(), "usdt", nil)
	if err != nil {
		fail("list positions: %v", err)
	}
	for _, p := range list {
		if p.Contract == contract {
			if size, _ := strconv.ParseFloat(p.Size, 64); size != 0 {
				raw, _ := json.Marshal(map[string]string{"size": p.Size, "entry": p.EntryPrice, "leverage": p.Lever, "mode": p.PosMarginMode})
				logf("position %s: %s", contract, string(raw))
				return p
			}
		}
	}
	logf("position %s: none", contract)
	return gateapi.Position{}
}

func balances(c *gateapi.APIClient) string {
	wallet, _, err := c.WalletApi.GetTotalBalance(rawCtx(), &gateapi.GetTotalBalanceOpts{Currency: optional.NewString("USDT")})
	if err != nil {
		fail("wallet total balance: %v", err)
	}
	futures, _, err := c.FuturesApi.ListFuturesAccounts(rawCtx(), "usdt")
	if err != nil {
		fail("futures account: %v", err)
	}
	raw, _ := json.Marshal(map[string]interface{}{"wallet_total_amount": wallet.Total.Amount, "wallet_borrowed": wallet.Total.Borrowed, "futures_available": futures.Available})
	return string(raw)
}

func closeAll(t *gate.GateTrader, contract, size string) {
	if size == "" || size == "0" {
		return
	}
	if size[0] == '-' {
		if _, err := t.CloseShort(contract, 0); err != nil {
			fail("close short %s: %v", contract, err)
		}
		return
	}
	if _, err := t.CloseLong(contract, 0); err != nil {
		fail("close long %s: %v", contract, err)
	}
}

// cleanup 还原：平掉两个合约上的任何持仓并撤销所有未触发条件单
func cleanup(c *gateapi.APIClient, t *gate.GateTrader) {
	for _, contract := range []string{doge, eth} {
		if p := positionOf(c, contract); p.Size != "" {
			closeAll(t, contract, p.Size)
		}
		time.Sleep(500 * time.Millisecond)
		for _, o := range listTriggers(c, contract) {
			if _, _, err := c.FuturesApi.CancelPriceTriggeredOrder(rawCtx(), "usdt", o.Id); err != nil {
				logf("cleanup cancel %d on %s failed: %v", o.Id, contract, err)
			}
		}
		time.Sleep(500 * time.Millisecond)
		if len(listTriggers(c, contract)) != 0 {
			fail("cleanup: %s still has trigger orders", contract)
		}
		logf("cleanup %s: clear", contract)
	}
}

func main() {
	cleanupOnly := flag.Bool("cleanup", false, "restore the account (close test positions, cancel trigger orders) and exit")
	flag.Parse()
	if key == "" || secret == "" {
		fail("GATE_KEY and GATE_SECRET must be set")
	}
	if path := flag.Arg(0); path != "" && !*cleanupOnly {
		f, err := os.Create(path)
		if err != nil {
			fail("create out: %v", err)
		}
		out = f
		defer out.Close()
	}

	c := client()
	t := gate.NewGateTrader(key, secret)

	if *cleanupOnly {
		cleanup(c, t)
		logf("CLEANUP_PASS")
		return
	}

	// 预检：两个合约均无持仓、无条件单、无挂单
	for _, contract := range []string{doge, eth} {
		if p := positionOf(c, contract); p.Size != "" {
			fail("precheck: %s has position %s (run -cleanup first)", contract, p.Size)
		}
		if len(listTriggers(c, contract)) != 0 {
			fail("precheck: %s has open trigger orders (run -cleanup first)", contract)
		}
	}
	logf("precheck: no positions, no trigger orders; balances=%s", balances(c))

	account, err := types.ReadAccountBalance(t)
	if err != nil {
		fail("read account balance: %v", err)
	}
	logf("account principal=%.4f available=%.4f", account.Principal, account.Available)

	// ============================ W2：DOGE 17 张 ============================
	price, err := t.GetMarketPrice(doge)
	if err != nil {
		fail("DOGE price: %v", err)
	}
	sl := price * 0.90
	tp := price * 1.10
	opened, err := t.OpenLongProtected(types.ProtectedOpenRequest{
		Symbol: doge, NotionalUSD: 9.3, Leverage: 10, IsCrossMargin: true,
		StopLoss: sl, TakeProfit: tp, MinNotional: 1, SlipRatio: 0.05,
	})
	if err != nil {
		fail("open DOGE: %v", err)
	}
	logf("DOGE opened: orderId=%s contracts=%s notional=%.4f fill=%.6f effectiveSL=%.8f effectiveTP=%.8f",
		opened.OrderID, opened.Contracts, opened.Notional, opened.FillPrice, opened.StopLoss, opened.TakeProfit)
	if err := t.VerifyOrderProtection(doge, opened.OrderID); err != nil {
		fail("DOGE protection verify: %v", err)
	}
	afterOpen := dumpTriggers(c, doge, "after open")
	if len(afterOpen) != 2 {
		fail("after open: expected 2 trigger orders, got %d", len(afterOpen))
	}

	initialProtection, err := t.PositionProtection(doge, "LONG")
	if err != nil {
		fail("read protection after open: %v", err)
	}
	if !initialProtection.Known {
		fail("protection not known after open: %s", initialProtection.Reason)
	}
	logf("protection after open: SL=%.8f (x%d) TP=%.8f (x%d)",
		initialProtection.StopLoss, initialProtection.StopLossOrders,
		initialProtection.TakeProfit, initialProtection.TakeProfitOrders)

	// 分批 50%：保护单张数不变、reduce-only
	partial, err := t.ClosePartial(types.PartialCloseRequest{Symbol: doge, Side: "LONG", Percent: 50})
	if err != nil {
		fail("partial close: %v", err)
	}
	logf("partial close: contracts=%s remaining=%s fill=%.6f", partial.Contracts, partial.Remaining, partial.FillPrice)
	dumpTriggers(c, doge, "after partial")
	afterPartial, err := t.PositionProtection(doge, "LONG")
	if err != nil {
		fail("read protection after partial: %v", err)
	}
	if !afterPartial.Known {
		fail("protection not known after partial: %s", afterPartial.Reason)
	}
	logf("protection after partial: SL=%.8f TP=%.8f", afterPartial.StopLoss, afterPartial.TakeProfit)

	// 改止损：向现价收紧（0.97）
	newSL := price * 0.97
	slResult, err := t.UpdateProtection(types.ProtectionUpdateRequest{
		Symbol: doge, Side: "long", Kind: types.ProtectionStopLoss,
		Price: newSL, Principal: account.Principal, MaxLossPercent: 2,
	})
	if err != nil {
		fail("update stop loss: %v", err)
	}
	logf("stop loss updated: old=%.8f new=%.8f orderId=%s replaced=%v",
		slResult.OldPrice, slResult.NewPrice, slResult.OrderID, slResult.ReplacedOrderIDs)
	if math.Abs(slResult.NewPrice-newSL) > 1e-5 {
		logf("note: aligned stop loss %.8f (requested %.8f)", slResult.NewPrice, newSL)
	}
	afterSL := dumpTriggers(c, doge, "after stop-loss update")
	if len(afterSL) != 2 {
		fail("after stop-loss update: expected SL+TP, got %d orders", len(afterSL))
	}

	// 改止盈：向现价方向（1.05，D32 允许双向）
	newTP := price * 1.05
	tpResult, err := t.UpdateProtection(types.ProtectionUpdateRequest{
		Symbol: doge, Side: "long", Kind: types.ProtectionTakeProfit,
		Price: newTP, Principal: account.Principal, MaxLossPercent: 2,
	})
	if err != nil {
		fail("update take profit: %v", err)
	}
	logf("take profit updated: old=%.8f new=%.8f orderId=%s replaced=%v",
		tpResult.OldPrice, tpResult.NewPrice, tpResult.OrderID, tpResult.ReplacedOrderIDs)
	afterTP := dumpTriggers(c, doge, "after take-profit update")
	if len(afterTP) != 2 {
		fail("after take-profit update: expected SL+TP, got %d orders", len(afterTP))
	}

	finalProtection, err := t.PositionProtection(doge, "LONG")
	if err != nil {
		fail("read final protection: %v", err)
	}
	if !finalProtection.Known || finalProtection.StopLossOrders != 1 || finalProtection.TakeProfitOrders != 1 {
		fail("final protection shape: %+v", finalProtection)
	}
	if math.Abs(finalProtection.StopLoss-slResult.NewPrice) > 1e-9 || math.Abs(finalProtection.TakeProfit-tpResult.NewPrice) > 1e-9 {
		fail("final protection prices: %+v", finalProtection)
	}
	logf("final protection: SL=%.8f TP=%.8f (one each)", finalProtection.StopLoss, finalProtection.TakeProfit)

	// 全平
	if _, err := t.CloseLong(doge, 0); err != nil {
		fail("full close DOGE: %v", err)
	}
	time.Sleep(2 * time.Second)
	if p := positionOf(c, doge); p.Size != "" {
		fail("DOGE position remains after close: %s", p.Size)
	}
	if len(dumpTriggers(c, doge, "after close")) != 0 {
		fail("DOGE trigger orders remain after close")
	}
	logf("DOGE closed: zero residue; balances=%s", balances(c))

	// ============================ W3 级：ETH 0.1 张 ============================
	ethPrice, err := t.GetMarketPrice(eth)
	if err != nil {
		fail("ETH price: %v", err)
	}
	ethSL := ethPrice * 0.99
	ethTP := ethPrice * 1.01
	ethOpened, err := t.OpenLongProtected(types.ProtectedOpenRequest{
		Symbol: eth, NotionalUSD: 1.8, Leverage: 10, IsCrossMargin: true,
		StopLoss: ethSL, TakeProfit: ethTP, MinNotional: 1, SlipRatio: 0.01,
	})
	if err != nil {
		fail("open ETH: %v", err)
	}
	logf("ETH opened: orderId=%s contracts=%s notional=%.4f fill=%.6f effectiveSL=%.8f effectiveTP=%.8f",
		ethOpened.OrderID, ethOpened.Contracts, ethOpened.Notional, ethOpened.FillPrice, ethOpened.StopLoss, ethOpened.TakeProfit)
	if ethOpened.Contracts != "0.1" {
		fail("ETH contracts = %s, want 0.1", ethOpened.Contracts)
	}
	if err := t.VerifyOrderProtection(eth, ethOpened.OrderID); err != nil {
		fail("ETH protection verify: %v", err)
	}
	ethAfterOpen := dumpTriggers(c, eth, "after open")
	if len(ethAfterOpen) != 2 {
		fail("ETH after open: expected 2 trigger orders, got %d", len(ethAfterOpen))
	}
	ethProtection, err := t.PositionProtection(eth, "LONG")
	if err != nil || !ethProtection.Known {
		fail("ETH protection read: %+v err=%v", ethProtection, err)
	}
	logf("ETH protection: SL=%.8f TP=%.8f", ethProtection.StopLoss, ethProtection.TakeProfit)

	ethNewSL := ethPrice * 0.995
	ethSLResult, err := t.UpdateProtection(types.ProtectionUpdateRequest{
		Symbol: eth, Side: "long", Kind: types.ProtectionStopLoss,
		Price: ethNewSL, Principal: account.Principal, MaxLossPercent: 2,
	})
	if err != nil {
		fail("ETH update stop loss: %v", err)
	}
	logf("ETH stop loss updated: old=%.8f new=%.8f replaced=%v", ethSLResult.OldPrice, ethSLResult.NewPrice, ethSLResult.ReplacedOrderIDs)
	ethAfterSL := dumpTriggers(c, eth, "after stop-loss update")
	if len(ethAfterSL) != 2 {
		fail("ETH after stop-loss update: expected SL+TP, got %d", len(ethAfterSL))
	}

	if _, err := t.CloseLong(eth, 0); err != nil {
		fail("full close ETH: %v", err)
	}
	time.Sleep(2 * time.Second)
	if p := positionOf(c, eth); p.Size != "" {
		fail("ETH position remains after close: %s", p.Size)
	}
	if len(dumpTriggers(c, eth, "after close")) != 0 {
		fail("ETH trigger orders remain after close")
	}

	// 终态兜底
	cleanup(c, t)
	logf("final balances=%s", balances(c))
	logf("LIVECHECK_PASS")
}
