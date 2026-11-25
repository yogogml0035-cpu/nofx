//go:build ignore

// 真实探针（子任务 11）：分批平仓后，随单止盈止损（close-*-order）按原订单张数触发，
// 还是按剩余仓位收口？用小仓位（3 张 DOGE ≈ 1.6U）与紧贴现价的 SL/TP 触发验证。
// 若触发后出现反向持仓，立即平掉并记录。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"nofx/trader/gate"
	"nofx/trader/types"

	"github.com/antihax/optional"
	"github.com/gate/gateapi-go/v7"
)

const contract = "DOGE_USDT"

var (
	key    = os.Getenv("GATE_KEY")
	secret = os.Getenv("GATE_SECRET")
)

func fail(format string, args ...interface{}) {
	fmt.Printf("FAIL: "+format+"\n", args...)
	os.Exit(1)
}

func logf(format string, args ...interface{}) {
	fmt.Printf("%s "+format+"\n", append([]interface{}{time.Now().Format("15:04:05")}, args...)...)
}

func rawCtx() context.Context {
	return context.WithValue(context.Background(), gateapi.ContextGateAPIV4, gateapi.GateAPIV4{Key: key, Secret: secret})
}

func client() *gateapi.APIClient {
	cfg := gateapi.NewConfiguration()
	cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	return gateapi.NewAPIClient(cfg)
}

func positions(c *gateapi.APIClient) gateapi.Position {
	list, _, err := c.FuturesApi.ListPositions(rawCtx(), "usdt", nil)
	if err != nil {
		fail("list positions: %v", err)
	}
	for _, p := range list {
		if p.Contract == contract {
			size, _ := strconv.ParseFloat(p.Size, 64)
			if size != 0 {
				return p
			}
		}
	}
	return gateapi.Position{}
}

func triggers(c *gateapi.APIClient) []gateapi.FuturesPriceTriggeredOrder {
	list, _, err := c.FuturesApi.ListPriceTriggeredOrders(rawCtx(), "usdt", "open",
		&gateapi.ListPriceTriggeredOrdersOpts{Contract: optional.NewString(contract)})
	if err != nil {
		fail("list triggers: %v", err)
	}
	return list
}

func dump(label string, p gateapi.Position, ts []gateapi.FuturesPriceTriggeredOrder) {
	pos := "none"
	if p.Size != "" {
		raw, _ := json.Marshal(map[string]string{"size": p.Size, "entry": p.EntryPrice, "mode": p.Mode})
		pos = string(raw)
	}
	rawT, _ := json.Marshal(ts)
	logf("%s: position=%s triggers=%s", label, pos, string(rawT))
}

// alignUp/alignDown 对齐到 tick
func align(price float64, tick string, up bool) float64 {
	t, err := strconv.ParseFloat(tick, 64)
	if err != nil || t <= 0 {
		fail("bad tick %q", tick)
	}
	steps := price / t
	if up {
		n := int64(steps)
		if steps > float64(n) {
			n++
		}
		return float64(n) * t
	}
	n := int64(steps)
	return float64(n) * t
}

func main() {
	if key == "" || secret == "" {
		fail("GATE_KEY/GATE_SECRET missing")
	}
	c := client()
	t := gate.NewGateTrader(key, secret)

	pre := positions(c)
	if pre.Size != "" {
		fail("precheck: position exists size=%s", pre.Size)
	}
	if len(triggers(c)) != 0 {
		fail("precheck: trigger orders exist")
	}

	info, _, err := c.FuturesApi.GetFuturesContract(rawCtx(), "usdt", contract)
	if err != nil {
		fail("contract: %v", err)
	}
	tick := info.OrderPriceRound
	price, err := t.GetMarketPrice(contract)
	if err != nil {
		fail("price: %v", err)
	}
	sl := align(price-5*tickToFloat(tick), tick, false)
	tp := align(price+5*tickToFloat(tick), tick, true)
	logf("price=%.6f tick=%s sl=%.6f tp=%.6f", price, tick, sl, tp)

	opened, err := t.OpenLongProtected(types.ProtectedOpenRequest{
		Symbol:        contract,
		NotionalUSD:   1.7,
		Leverage:      10,
		IsCrossMargin: true,
		StopLoss:      sl,
		TakeProfit:    tp,
		MinNotional:   1,
		SlipRatio:     0.05,
	})
	if err != nil {
		fail("open: %v", err)
	}
	logf("opened contracts=%s fill=%.6f orderId=%s", opened.Contracts, opened.FillPrice, opened.OrderID)
	if opened.Contracts != "3" {
		logf("note: contracts=%s (expected 3 for the size-mismatch test)", opened.Contracts)
	}
	dump("after open", positions(c), triggers(c))

	// 分批平掉 1 张，剩 2 张；随单保护仍是原订单张数
	partial, err := t.ClosePartial(types.PartialCloseRequest{Symbol: contract, Side: "LONG", Percent: 50})
	if err != nil {
		logf("partial 50%% failed: %v", err)
	} else {
		logf("partial: contracts=%s remaining=%s", partial.Contracts, partial.Remaining)
	}
	dump("after partial", positions(c), triggers(c))

	// 等待触发（SL/TP 均距现价 1 tick）
	deadline := time.Now().Add(120 * time.Second)
	triggered := false
	for time.Now().Before(deadline) {
		p := positions(c)
		ts := triggers(c)
		if p.Size == "" && len(ts) == 0 {
			// 持仓与条件单都消失；再等 1s 取最终仓位（可能反向）
			time.Sleep(time.Second)
			p = positions(c)
			logf("triggered: position after=%q", p.Size)
			triggered = true
			break
		}
		if p.Size != "" && p.Size != "2" && p.Size != "-2" {
			logf("position changed: size=%s", p.Size)
		}
		time.Sleep(3 * time.Second)
	}
	final := positions(c)
	dump("after wait", final, triggers(c))

	// 还原：任何残留仓位（含反向）立即平掉
	if final.Size != "" {
		size, _ := strconv.ParseFloat(final.Size, 64)
		if size > 0 {
			if _, err := t.CloseLong(contract, 0); err != nil {
				fail("cleanup close long: %v", err)
			}
		} else {
			if _, err := t.CloseShort(contract, 0); err != nil {
				fail("cleanup close short: %v", err)
			}
		}
		time.Sleep(2 * time.Second)
	}
	if p := positions(c); p.Size != "" {
		fail("cleanup: position remains %s", p.Size)
	}
	for _, o := range triggers(c) {
		if _, _, err := c.FuturesApi.CancelPriceTriggeredOrder(rawCtx(), "usdt", o.Id); err != nil {
			fail("cleanup cancel %d: %v", o.Id, err)
		}
	}
	if len(triggers(c)) != 0 {
		fail("cleanup: triggers remain")
	}
	if triggered {
		logf("TRIGGER_PROBE_DONE")
	} else {
		logf("TRIGGER_PROBE_INCONCLUSIVE (no trigger within 120s; state restored)")
	}
}

func tickToFloat(tick string) float64 {
	v, err := strconv.ParseFloat(tick, 64)
	if err != nil {
		fail("tick parse: %v", err)
	}
	return v
}
