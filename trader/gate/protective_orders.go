package gate

// 保护单适配：仓位级止损止盈构造、随单附带止盈止损、条件单查询/挂单/改单/撤销。
// 纯新增，不接入交易路径；旧的 SetStopLoss/SetTakeProfit 触发规则四个方向与 Gate 文档
// 相反（计划文档 F6），由子任务 4 在接入时删除，本文件是语义修正后的新实现。
// 文档依据见子任务 research/gate-price-order-docs.md。

import (
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"nofx/logger"

	"github.com/antihax/optional"
	"github.com/gate/gateapi-go/v7"
)

// 触发规则取 Gate 文档语义，不是 NOFX 的方向语义：
// 1 = 价格高于等于触发价时触发，触发价必须高于最新价；
// 2 = 价格低于等于触发价时触发，触发价必须低于最新价
const (
	PriceRuleGE int32 = 1
	PriceRuleLE int32 = 2
)

// ProtectiveKind 保护单类型：止损或止盈
type ProtectiveKind int

const (
	ProtectiveStopLoss ProtectiveKind = iota + 1
	ProtectiveTakeProfit
)

// String 给出给用户与模型看的说明，使用中文；包内的错误信息沿用惯例用英文
func (k ProtectiveKind) String() string {
	switch k {
	case ProtectiveStopLoss:
		return "止损"
	case ProtectiveTakeProfit:
		return "止盈"
	}
	return fmt.Sprintf("ProtectiveKind(%d)", int(k))
}

// normalizeSide 校验持仓方向，"LONG"/"SHORT" 大小写不敏感、允许首尾空白
func normalizeSide(side string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(side)) {
	case "LONG":
		return "LONG", nil
	case "SHORT":
		return "SHORT", nil
	}
	return "", fmt.Errorf("invalid position side %q", side)
}

// validatedPrice 把价格按 float64 的最短十进制表示取值（0.0775 就是 "0.0775"），
// 非正数与 NaN、无穷大报错。返回规范字符串与解析后的精确值。
// 触发价不按 tick size 取整：官方资料未见触发价精度的校验行为，由交易所校验并回传错误，
// 调用方应给 tick 对齐的价格。
func validatedPrice(field string, price float64) (string, *big.Rat, error) {
	raw := strconv.FormatFloat(price, 'f', -1, 64)
	value, err := parseDecimal(field, raw)
	if err != nil {
		return "", nil, err
	}
	if value.Sign() <= 0 {
		return "", nil, fmt.Errorf("%s must be positive, got %v", field, price)
	}
	return raw, value, nil
}

// protectiveTriggerRule 返回仓位级保护单的触发规则：
// 多头止损、空头止盈在价格回落到触发价时触发（rule 2）；
// 多头止盈、空头止损在价格涨到触发价时触发（rule 1）。
func protectiveTriggerRule(side string, kind ProtectiveKind) (int32, error) {
	normalized, err := normalizeSide(side)
	if err != nil {
		return 0, err
	}
	switch {
	case normalized == "LONG" && kind == ProtectiveStopLoss,
		normalized == "SHORT" && kind == ProtectiveTakeProfit:
		return PriceRuleLE, nil
	case normalized == "LONG" && kind == ProtectiveTakeProfit,
		normalized == "SHORT" && kind == ProtectiveStopLoss:
		return PriceRuleGE, nil
	}
	return 0, fmt.Errorf("invalid protective kind %d", int(kind))
}

// protectiveOrderType 仓位级止盈止损的 order_type；随单类型（close-*-order）是只读字段，
// 不能在创建时传入
func protectiveOrderType(side string) (string, error) {
	normalized, err := normalizeSide(side)
	if err != nil {
		return "", err
	}
	if normalized == "LONG" {
		return "close-long-position", nil
	}
	return "close-short-position", nil
}

// BuildPositionProtectiveOrder 构造仓位级止盈止损条件单，触发后市价平掉整个仓位。
// 仓位级保护单在分批平仓后继续覆盖剩余仓位（计划文档 F8）。单向持仓模式的全平按文档用
// size=0 + close=true 表达，不需 auto_size；不设 reduce_only（size=0 时该字段无意义）。
// 触发固定为价格触发（strategy_type=0）与最新成交价（price_type=0），不设 expiration：
// 保护单必须存活到被撤销或触发。
// 触发价须与最新价保持规则要求的一侧关系（rule 1 须高于最新价、rule 2 须低于最新价），
// 该校验依赖行情，适配层不负责，由调用方保证、交易所兜底。
func BuildPositionProtectiveOrder(contract, side string, kind ProtectiveKind, triggerPrice float64) (gateapi.FuturesPriceTriggeredOrder, error) {
	if contract == "" {
		return gateapi.FuturesPriceTriggeredOrder{}, errors.New("contract is empty")
	}
	rule, err := protectiveTriggerRule(side, kind)
	if err != nil {
		return gateapi.FuturesPriceTriggeredOrder{}, err
	}
	orderType, err := protectiveOrderType(side)
	if err != nil {
		return gateapi.FuturesPriceTriggeredOrder{}, err
	}
	price, _, err := validatedPrice("trigger price", triggerPrice)
	if err != nil {
		return gateapi.FuturesPriceTriggeredOrder{}, err
	}
	return gateapi.FuturesPriceTriggeredOrder{
		Initial: gateapi.FuturesInitialOrder{
			Contract: contract,
			Price:    "0", // 市价出场
			Tif:      "ioc",
			Close:    true,
			Text:     "t-nofx",
		},
		Trigger: gateapi.FuturesPriceTrigger{
			StrategyType: 0, // 价格触发
			PriceType:    0, // 最新成交价
			Price:        price,
			Rule:         rule,
		},
		OrderType: orderType,
	}, nil
}

// ApplyAttachedTPSL 把随单止盈止损写入开仓委托：触发方向由交易所按委托方向推导，
// 这里校验两价合法且分居两侧（多头止损价必须低于止盈价，空头相反）。
// 价格必须是十进制字符串（经 AlignTriggerPrice 对齐到合约价格步长；不对齐时交易所接受
// 字段但不创建条件单，计划文档 F44）。满足保护不变量「保护单随开仓单一并提交」：
// 仓位从存在那一刻起就带有止损与止盈。
func ApplyAttachedTPSL(order *gateapi.FuturesOrder, side, stopLossPrice, takeProfitPrice string) error {
	if order == nil {
		return errors.New("order is nil")
	}
	if order.Contract == "" {
		return errors.New("order contract is empty")
	}
	normalized, err := normalizeSide(side)
	if err != nil {
		return err
	}
	slValue, err := parseDecimal("stop loss trigger price", stopLossPrice)
	if err != nil {
		return err
	}
	if slValue.Sign() <= 0 {
		return fmt.Errorf("stop loss trigger price must be positive, got %q", stopLossPrice)
	}
	tpValue, err := parseDecimal("take profit trigger price", takeProfitPrice)
	if err != nil {
		return err
	}
	if tpValue.Sign() <= 0 {
		return fmt.Errorf("take profit trigger price must be positive, got %q", takeProfitPrice)
	}
	if normalized == "LONG" && slValue.Cmp(tpValue) >= 0 {
		return fmt.Errorf("long stop loss %s must be below take profit %s", stopLossPrice, takeProfitPrice)
	}
	if normalized == "SHORT" && tpValue.Cmp(slValue) >= 0 {
		return fmt.Errorf("short take profit %s must be below stop loss %s", takeProfitPrice, stopLossPrice)
	}
	order.TpslSlTriggerPrice = stopLossPrice
	order.TpslTpTriggerPrice = takeProfitPrice
	return nil
}

// validateProtectionSides 校验（对齐后的）止损止盈位于现价的正确一侧：
// 多单止损必须低于现价、止盈必须高于现价；空单相反。触发规则在创建时要求这个关系
// （rule 1 的触发价须高于最新价、rule 2 须低于最新价），越界的保护单会被交易所拒绝。
func validateProtectionSides(side string, market float64, stopLoss, takeProfit *big.Rat) error {
	normalized, err := normalizeSide(side)
	if err != nil {
		return err
	}
	marketValue, err := decimalFromFloat("current price", market)
	if err != nil {
		return err
	}
	if marketValue.Sign() <= 0 {
		return fmt.Errorf("current price must be positive, got %v", market)
	}
	if stopLoss == nil || takeProfit == nil {
		return errors.New("stop loss and take profit are required")
	}
	stopText, takeText := stopLoss.FloatString(8), takeProfit.FloatString(8)
	if normalized == "LONG" {
		if stopLoss.Cmp(marketValue) >= 0 {
			return fmt.Errorf("long stop loss %s must be below current price %v", stopText, market)
		}
		if takeProfit.Cmp(marketValue) <= 0 {
			return fmt.Errorf("long take profit %s must be above current price %v", takeText, market)
		}
		return nil
	}
	if stopLoss.Cmp(marketValue) <= 0 {
		return fmt.Errorf("short stop loss %s must be above current price %v", stopText, market)
	}
	if takeProfit.Cmp(marketValue) >= 0 {
		return fmt.Errorf("short take profit %s must be below current price %v", takeText, market)
	}
	return nil
}

// TriggerOrderView 未触发条件单的解析视图。TriggerPrice 是 float64，仅用于展示与日志；
// 需要精确值时由调用方经原始接口取字符串。解析失败即报错，不回落猜测值。
type TriggerOrderView struct {
	ID            int64
	IDString      string // 交易所返回的字符串形式订单 ID，展示与二次调用优先用它
	Contract      string
	OrderType     string // 交易所返回的原始值；普通条件单为空
	Side          string // "LONG"/"SHORT"，由 order_type 判定；未知为 ""
	Kind          ProtectiveKind
	KindKnown     bool
	TriggerRule   int32
	TriggerPrice  float64
	InitialSize   int64
	InitialAmount string // 小数张经此字段提交与返回（计划文档 F32）
	Status        string
}

// OrderID 返回可用于二次调用的订单 ID 字符串
func (v TriggerOrderView) OrderID() string {
	if v.IDString != "" {
		return v.IDString
	}
	return strconv.FormatInt(v.ID, 10)
}

// classifyTriggerOrder 按 order_type 与触发规则判定条件单的方向与类型：
// 多头条件 rule 2 = 止损、rule 1 = 止盈；空头相反。order_type 为空（普通条件单）或
// 无法识别时方向未知，不判定类型；识别了方向但规则不是 1/2 时只给方向。
func classifyTriggerOrder(orderType string, rule int32) (side string, kind ProtectiveKind, known bool) {
	var long bool
	switch {
	case strings.HasPrefix(orderType, "close-long"), strings.HasPrefix(orderType, "plan-close-long"):
		long = true
	case strings.HasPrefix(orderType, "close-short"), strings.HasPrefix(orderType, "plan-close-short"):
		long = false
	default:
		return "", 0, false
	}
	side = "SHORT"
	if long {
		side = "LONG"
	}
	switch rule {
	case PriceRuleLE:
		if long {
			kind = ProtectiveStopLoss
		} else {
			kind = ProtectiveTakeProfit
		}
		return side, kind, true
	case PriceRuleGE:
		if long {
			kind = ProtectiveTakeProfit
		} else {
			kind = ProtectiveStopLoss
		}
		return side, kind, true
	}
	return side, 0, false
}

// parseTriggerOrder 把 SDK 返回的条件单解析为视图；触发价解析失败即报错
func parseTriggerOrder(order gateapi.FuturesPriceTriggeredOrder) (TriggerOrderView, error) {
	price, err := parseDecimal("trigger price", order.Trigger.Price)
	if err != nil {
		return TriggerOrderView{}, err
	}
	view := TriggerOrderView{
		ID:            order.Id,
		IDString:      order.IdString,
		Contract:      order.Initial.Contract,
		OrderType:     order.OrderType,
		TriggerRule:   order.Trigger.Rule,
		TriggerPrice:  ratFloat(price),
		InitialSize:   order.Initial.Size,
		InitialAmount: order.Initial.Amount,
		Status:        order.Status,
	}
	view.Side, view.Kind, view.KindKnown = classifyTriggerOrder(order.OrderType, order.Trigger.Rule)
	return view, nil
}

// ListTriggerOrders 列出合约上未触发（status=open）的条件单，逐条解析为视图。
// 已完结的条件单在完结 10 分钟后不能按 ID 查询（计划文档 F25），列表是唯一稳定入口，
// 本适配层不封装按 ID 的单条查询。
func (t *GateTrader) ListTriggerOrders(symbol string) ([]TriggerOrderView, error) {
	symbol = t.convertSymbol(symbol)

	opts := &gateapi.ListPriceTriggeredOrdersOpts{
		Contract: optional.NewString(symbol),
	}
	orders, _, err := t.client.FuturesApi.ListPriceTriggeredOrders(t.ctx, "usdt", "open", opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list trigger orders: %w", err)
	}

	views := make([]TriggerOrderView, 0, len(orders))
	for _, order := range orders {
		view, err := parseTriggerOrder(order)
		if err != nil {
			return nil, fmt.Errorf("failed to parse trigger order %d: %w", order.Id, err)
		}
		views = append(views, view)
	}
	return views, nil
}

// PlaceProtectiveOrder 提交构造好的条件单，返回订单 ID 字符串（优先 IdString）。
// 交易所未回传任何 ID 时报错：拿不到 ID 的保护单无法核对与撤销，不能当作挂单成功。
func (t *GateTrader) PlaceProtectiveOrder(order gateapi.FuturesPriceTriggeredOrder) (string, error) {
	if order.Initial.Contract == "" {
		return "", errors.New("order contract is empty")
	}

	resp, _, err := t.client.FuturesApi.CreatePriceTriggeredOrder(t.ctx, "usdt", order)
	if err != nil {
		return "", fmt.Errorf("failed to place protective order: %w", err)
	}

	switch {
	case resp.IdString != "":
		logger.Infof("  [Gate] Protective order placed: %s %s trigger=%s id=%s",
			order.OrderType, order.Initial.Contract, order.Trigger.Price, resp.IdString)
		return resp.IdString, nil
	case resp.Id != 0:
		id := strconv.FormatInt(resp.Id, 10)
		logger.Infof("  [Gate] Protective order placed: %s %s trigger=%s id=%s",
			order.OrderType, order.Initial.Contract, order.Trigger.Price, id)
		return id, nil
	}
	return "", errors.New("exchange returned no order id for protective order")
}

// parseOrderID 校验订单 ID：非空、十进制整数、规范形式（不含前导零）且为正。
// 改单接口要求路径与请求体携带同一订单 ID，规范形式保证两边字面一致。
func parseOrderID(orderID string) (string, int64, error) {
	id, err := strconv.ParseInt(orderID, 10, 64)
	if err != nil || id <= 0 || orderID != strconv.FormatInt(id, 10) {
		return "", 0, fmt.Errorf("invalid order id %q", orderID)
	}
	return orderID, id, nil
}

// buildAmendTriggerBody 构造改触发价的请求：路径与请求体携带同一订单 ID，
// 只带 trigger_price，其余字段缺省表示不修改
func buildAmendTriggerBody(orderID string, newTriggerPrice float64) (string, gateapi.FuturesUpdatePriceTriggeredOrder, error) {
	id, idNum, err := parseOrderID(orderID)
	if err != nil {
		return "", gateapi.FuturesUpdatePriceTriggeredOrder{}, err
	}
	price, _, err := validatedPrice("trigger price", newTriggerPrice)
	if err != nil {
		return "", gateapi.FuturesUpdatePriceTriggeredOrder{}, err
	}
	return id, gateapi.FuturesUpdatePriceTriggeredOrder{
		OrderId:      idNum,
		TriggerPrice: price,
	}, nil
}

// buildAmendSizeBody 构造改委托张数的请求：小数张经 amount 提交（计划文档 F32），
// size 不随请求发送（omitempty），两字段同给时以 amount 为准。
// 正负号按方向约定：平多仓为负、平空仓为正；全平由 BuildPositionProtectiveOrder 的
// size=0 + close=true 表达，不走本函数。
func buildAmendSizeBody(orderID, contracts string) (string, gateapi.FuturesUpdatePriceTriggeredOrder, error) {
	id, idNum, err := parseOrderID(orderID)
	if err != nil {
		return "", gateapi.FuturesUpdatePriceTriggeredOrder{}, err
	}
	amount, err := parseDecimal("contracts", contracts)
	if err != nil {
		return "", gateapi.FuturesUpdatePriceTriggeredOrder{}, err
	}
	if amount.Sign() == 0 {
		return "", gateapi.FuturesUpdatePriceTriggeredOrder{}, fmt.Errorf("contracts must not be zero")
	}
	return id, gateapi.FuturesUpdatePriceTriggeredOrder{
		OrderId: idNum,
		Amount:  contracts,
	}, nil
}

// AmendProtectiveOrderTrigger 修改未触发条件单的触发价。
// 触发方向在创建时已固定（改单请求没有 rule 字段）。注意实测状态（2026-11-24，research
// H.2）：API 创建的触价单一律被交易所拒绝——SDK 的新路径（changelog v4.106.160）线上
// 404 未部署，旧路径返回 code 1077 APIOrderNotSupportUpdateTouchOrder。修改止损止盈的
// 实际路径是撤旧建新（CancelProtectiveOrdersByKind + BuildPositionProtectiveOrder +
// PlaceProtectiveOrder）；本封装保留规范实现，仓位级条件单能否改单在 W2 有仓位时补测。
func (t *GateTrader) AmendProtectiveOrderTrigger(orderID string, newTriggerPrice float64) (string, error) {
	id, body, err := buildAmendTriggerBody(orderID, newTriggerPrice)
	if err != nil {
		return "", err
	}
	resp, _, err := t.client.FuturesApi.UpdatePriceTriggeredOrder(t.ctx, "usdt", id, body)
	if err != nil {
		return "", fmt.Errorf("failed to amend protective order %s: %w", orderID, err)
	}
	if resp.IdString != "" {
		return resp.IdString, nil
	}
	if resp.Id != 0 {
		return strconv.FormatInt(resp.Id, 10), nil
	}
	return id, nil
}

// AmendProtectiveOrderSize 修改未触发条件单的委托张数：小数张经 amount 提交（计划文档 F32）。
// 改单的实测状态见 AmendProtectiveOrderTrigger：当前 API 创建的触价单一律被拒。
// 返回交易所回传的订单 ID。
func (t *GateTrader) AmendProtectiveOrderSize(orderID, contracts string) (string, error) {
	id, body, err := buildAmendSizeBody(orderID, contracts)
	if err != nil {
		return "", err
	}
	resp, _, err := t.client.FuturesApi.UpdatePriceTriggeredOrder(t.ctx, "usdt", id, body)
	if err != nil {
		return "", fmt.Errorf("failed to amend protective order %s: %w", orderID, err)
	}
	if resp.IdString != "" {
		return resp.IdString, nil
	}
	if resp.Id != 0 {
		return strconv.FormatInt(resp.Id, 10), nil
	}
	return id, nil
}

// CancelProtectiveOrder 撤销单个未触发条件单
func (t *GateTrader) CancelProtectiveOrder(orderID string) error {
	_, idNum, err := parseOrderID(orderID)
	if err != nil {
		return err
	}
	if _, _, err := t.client.FuturesApi.CancelPriceTriggeredOrder(t.ctx, "usdt", idNum); err != nil {
		return fmt.Errorf("failed to cancel protective order %s: %w", orderID, err)
	}
	logger.Infof("  [Gate] Protective order cancelled: id=%s", orderID)
	return nil
}

// CancelProtectiveOrdersByKind 撤销合约上指定类型的未触发保护单：先查询再逐条判定，
// 只撤匹配类型者（计划文档 F10 的语义修正：撤止损不带动止盈）。无法判定类型的条件单
// 不撤（fail-closed）。单个撤销失败即中止并返回错误，返回已成功撤销的订单 ID 列表。
func (t *GateTrader) CancelProtectiveOrdersByKind(symbol string, kind ProtectiveKind) ([]string, error) {
	if kind != ProtectiveStopLoss && kind != ProtectiveTakeProfit {
		return nil, fmt.Errorf("invalid protective kind %d", int(kind))
	}
	views, err := t.ListTriggerOrders(symbol)
	if err != nil {
		return nil, err
	}

	cancelled := []string{}
	for _, view := range views {
		if !view.KindKnown || view.Kind != kind {
			continue
		}
		if err := t.CancelProtectiveOrder(view.OrderID()); err != nil {
			return cancelled, err
		}
		cancelled = append(cancelled, view.OrderID())
	}
	return cancelled, nil
}
