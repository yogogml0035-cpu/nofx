package main

// 真实写操作验证（子任务 protective-order-adapter，D35 授权范围内）：
// 在 DOGE_USDT 上验证「挂 → 查 → 改单尝试 → 撤旧建新 → 查 → 撤 → 确认无残留」，
// 每一步立即还原：
//  1. 仓位级止损单（close-long-position）先试——实测预期被拒（research H.1，
//     AUTO_USER_NO_LONG_POSITION）；被拒后改用普通条件单（无 order_type、size=+1）完成链路
//  2. 改单尝试预期被拒（research H.2：新路径 404 未部署、旧路径 code 1077 拒绝 API 单）；
//     随后验证实际路径「撤旧建新」：撤销原单、按新触发价重建、查询核对
//  3. 按类型撤销：仓位级单挂不上、普通单方向未知，跳过（判定逻辑由单测覆盖），W2 补测
//  4. 随单止盈止损：远离现价的 gtc 限价单带 tpsl（ApplyAttachedTPSL），
//     确认交易所接受后撤单
//
// 触发价取最新价的 −50%/−45%（远离子成交），名义价值 ≤ 1 张 DOGE（≈0.6 USDT）。
// 凭据从 GATE_KEY / GATE_SECRET 环境变量读取，不落盘、不打印凭据。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/antihax/optional"
	"github.com/gate/gateapi-go/v7"

	nofxgate "nofx/trader/gate"
)

const contract = "DOGE_USDT"

type stepLine struct {
	Step       string `json:"step"`
	OK         bool   `json:"ok"`
	Detail     string `json:"detail,omitempty"`
	OrderID    string `json:"order_id,omitempty"`
	Error      string `json:"error,omitempty"`
	Skipped    bool   `json:"skipped,omitempty"`
	SkipReason string `json:"skip_reason,omitempty"`
}

var stepNo int

func emit(v stepLine) {
	stepNo++
	v.Step = fmt.Sprintf("%02d_%s", stepNo, v.Step)
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(raw))
	if !v.OK && !v.Skipped {
		os.Exit(1)
	}
}

func mustParseFloat(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		panic(err)
	}
	return v
}

// plainOrder 构造普通条件单（多头方向、远离现价、1 张）
func plainOrder(trigger string) gateapi.FuturesPriceTriggeredOrder {
	return gateapi.FuturesPriceTriggeredOrder{
		Initial: gateapi.FuturesInitialOrder{
			Contract: contract,
			Size:     1,
			Price:    "0",
			Tif:      "ioc",
			Text:     "t-nofx",
		},
		Trigger: gateapi.FuturesPriceTrigger{
			StrategyType: 0,
			PriceType:    0,
			Price:        trigger,
			Rule:         nofxgate.PriceRuleLE,
		},
	}
}

func main() {
	gt := nofxgate.NewGateTrader(os.Getenv("GATE_KEY"), os.Getenv("GATE_SECRET"))

	price, err := gt.GetMarketPrice(contract)
	if err != nil {
		emit(stepLine{Step: "price", OK: false, Error: err.Error()})
		return
	}
	emit(stepLine{Step: "price", OK: true, Detail: fmt.Sprintf("last=%v", price)})

	low := fmt.Sprintf("%.5f", price*0.50)        // 首次挂单的触发价（多头止损，rule 2 须低于最新价）
	lowAmended := fmt.Sprintf("%.5f", price*0.45) // 改动目标触发价

	// ---- 挂：先试仓位级止损单（预期被拒，research H.1） ----
	order, err := nofxgate.BuildPositionProtectiveOrder(contract, "LONG", nofxgate.ProtectiveStopLoss, mustParseFloat(low))
	if err != nil {
		emit(stepLine{Step: "build_position_sl", OK: false, Error: err.Error()})
		return
	}
	orderID, placeErr := gt.PlaceProtectiveOrder(order)
	if placeErr != nil {
		emit(stepLine{Step: "place_position_sl", OK: true, Skipped: true,
			SkipReason: "仓位级条件单无持仓被拒（实测回答研究缺口 H.1）: " + placeErr.Error()})

		orderID, err = gt.PlaceProtectiveOrder(plainOrder(low))
		if err != nil {
			emit(stepLine{Step: "place_plain", OK: false, Error: err.Error()})
			return
		}
		emit(stepLine{Step: "place_plain", OK: true, OrderID: orderID, Detail: "trigger=" + low})
	} else {
		emit(stepLine{Step: "place_position_sl", OK: true, OrderID: orderID,
			Detail: "无持仓时仓位级条件单可挂（与 H.1 实测记录不符，需复查）, trigger=" + low})
	}

	// ---- 查：列表读回并核对 ----
	view, err := findOrder(gt, orderID)
	if err != nil {
		emit(stepLine{Step: "query_after_place", OK: false, Error: err.Error()})
		return
	}
	emit(stepLine{Step: "query_after_place", OK: true, Detail: fmt.Sprintf(
		"trigger=%v rule=%d order_type=%q side=%q kind_known=%v", view.TriggerPrice, view.TriggerRule, view.OrderType, view.Side, view.KindKnown)})

	// ---- 改单尝试（预期被拒，research H.2） ----
	if _, err := gt.AmendProtectiveOrderTrigger(orderID, mustParseFloat(lowAmended)); err != nil {
		emit(stepLine{Step: "amend_attempt", OK: true, Skipped: true,
			SkipReason: "改单被交易所拒绝（实测回答研究缺口 H.2）: " + err.Error()})
	} else {
		emit(stepLine{Step: "amend_attempt", OK: true,
			Detail: "改单成功（与 H.2 实测记录不符，需复查）"})
	}

	// ---- 撤旧建新：撤销原单、按新触发价重建、查询核对（修改止损的实际路径） ----
	if err := gt.CancelProtectiveOrder(orderID); err != nil {
		emit(stepLine{Step: "rebuild_cancel_old", OK: false, Error: err.Error()})
		return
	}
	rebuiltID, err := gt.PlaceProtectiveOrder(plainOrder(lowAmended))
	if err != nil {
		emit(stepLine{Step: "rebuild_place_new", OK: false, Error: err.Error()})
		return
	}
	view, err = findOrder(gt, rebuiltID)
	if err != nil {
		emit(stepLine{Step: "query_after_rebuild", OK: false, Error: err.Error()})
		return
	}
	want := mustParseFloat(lowAmended)
	if diff := view.TriggerPrice - want; diff < -1e-9 || diff > 1e-9 {
		emit(stepLine{Step: "query_after_rebuild", OK: false,
			Error: fmt.Sprintf("rebuilt trigger price mismatch: got %v want %v", view.TriggerPrice, want)})
		return
	}
	emit(stepLine{Step: "query_after_rebuild", OK: true, OrderID: rebuiltID,
		Detail: fmt.Sprintf("撤旧建新生效，trigger=%v", view.TriggerPrice)})

	// ---- 撤：按 ID 撤销并确认 ----
	if err := gt.CancelProtectiveOrder(rebuiltID); err != nil {
		emit(stepLine{Step: "cancel_by_id", OK: false, Error: err.Error()})
		return
	}
	if err := assertOrderGone(gt, rebuiltID); err != nil {
		emit(stepLine{Step: "query_after_cancel", OK: false, Error: err.Error()})
		return
	}
	emit(stepLine{Step: "query_after_cancel", OK: true, OrderID: rebuiltID})

	// ---- 按类型撤销（仓位级单挂不上，普通单方向未知，跳过） ----
	emit(stepLine{Step: "cancel_by_kind", OK: true, Skipped: true,
		SkipReason: "仓位级条件单需有持仓（H.1），普通条件单无 order_type 方向未知按设计跳过；判定逻辑由单测覆盖，W2 补测"})

	// ---- 随单止盈止损：远离现价的 gtc 限价单带 tpsl，确认接受后撤单 ----
	if err := verifyAttachedTPSL(price); err != nil {
		emit(stepLine{Step: "attached_tpsl", OK: false, Error: err.Error()})
		return
	}
	emit(stepLine{Step: "attached_tpsl", OK: true})

	// ---- 终态：合约上无残留未触发条件单 ----
	views, err := gt.ListTriggerOrders(contract)
	if err != nil {
		emit(stepLine{Step: "final_state", OK: false, Error: err.Error()})
		return
	}
	if len(views) != 0 {
		emit(stepLine{Step: "final_state", OK: false, Detail: fmt.Sprintf("%d remaining", len(views))})
		return
	}
	emit(stepLine{Step: "final_state", OK: true, Detail: "no open trigger orders left"})
	fmt.Printf("LIVECHECK_PASS steps=%d\n", stepNo)
}

// findOrder 从未触发条件单列表中按订单 ID 找到目标
func findOrder(gt *nofxgate.GateTrader, orderID string) (nofxgate.TriggerOrderView, error) {
	views, err := gt.ListTriggerOrders(contract)
	if err != nil {
		return nofxgate.TriggerOrderView{}, err
	}
	for _, view := range views {
		if view.OrderID() == orderID {
			return view, nil
		}
	}
	return nofxgate.TriggerOrderView{}, fmt.Errorf("order %s not found in open trigger orders (%d entries)", orderID, len(views))
}

// assertOrderGone 确认订单已不在未触发列表中
func assertOrderGone(gt *nofxgate.GateTrader, orderID string) error {
	views, err := gt.ListTriggerOrders(contract)
	if err != nil {
		return err
	}
	for _, view := range views {
		if view.OrderID() == orderID {
			return fmt.Errorf("order %s still open after cancel", orderID)
		}
	}
	return nil
}

// verifyAttachedTPSL 挂一张远离现价的 gtc 限价买单（1 张，不会成交），带上随单止盈止损，
// 从交易所返回确认 tpsl 字段被接受，然后撤单还原。限价单的创建/撤销用独立 SDK 客户端发
// 原始请求：不在本子任务适配层范围内（适配层只提供 ApplyAttachedTPSL）。
func verifyAttachedTPSL(price float64) error {
	key := os.Getenv("GATE_KEY")
	secret := os.Getenv("GATE_SECRET")
	if key == "" || secret == "" {
		return fmt.Errorf("GATE_KEY/GATE_SECRET required for attached tpsl verification")
	}
	config := gateapi.NewConfiguration()
	config.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	client := gateapi.NewAPIClient(config)
	ctx := context.WithValue(context.Background(),
		gateapi.ContextGateAPIV4, gateapi.GateAPIV4{Key: key, Secret: secret})

	limitPrice := fmt.Sprintf("%.5f", price*0.5) // 远离现价，不会成交
	order := gateapi.FuturesOrder{
		Contract: contract,
		Size:     "1",
		Price:    limitPrice,
		Tif:      "gtc",
		Text:     "t-nofx",
	}
	// 多头：止损价低于限价，止盈价高于限价
	if err := nofxgate.ApplyAttachedTPSL(&order, "LONG",
		mustParseFloat(fmt.Sprintf("%.5f", price*0.40)),
		mustParseFloat(fmt.Sprintf("%.5f", price*0.60))); err != nil {
		return fmt.Errorf("apply attached tpsl: %w", err)
	}

	created, _, err := client.FuturesApi.CreateFuturesOrder(ctx, "usdt", order, nil)
	if err != nil {
		return fmt.Errorf("create limit order with tpsl: %w", err)
	}
	cancelOpts := &gateapi.CancelFuturesOrdersOpts{Contract: optional.NewString(contract)}
	if _, _, err := client.FuturesApi.CancelFuturesOrders(ctx, "usdt", cancelOpts); err != nil {
		return fmt.Errorf("cancel limit order %d after tpsl check: %w", created.Id, err)
	}
	if created.TpslSlTriggerPrice == "" && created.TpslTpTriggerPrice == "" {
		return fmt.Errorf("exchange response carried no tpsl fields back (sl=%q tp=%q)",
			created.TpslSlTriggerPrice, created.TpslTpTriggerPrice)
	}
	fmt.Printf("attached_tpsl: order=%d sl=%q tp=%q\n", created.Id, created.TpslSlTriggerPrice, created.TpslTpTriggerPrice)
	return nil
}
