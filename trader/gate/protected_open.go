package gate

// 随单保护开仓：USDT 名义价值换算张数、市价滑点上限、下单请求随单附带止盈止损、
// 成交后核对保护单存在。保护不变量（计划文档第 5 节）：NOFX 开出的仓位从存在那一刻起
// 就带有止损与止盈；核对缺失由调用方立即平仓并报错，本层只报错不动仓位。
// 触发规则与字段语义见子任务 research/gate-price-order-docs.md。

import (
	"errors"
	"fmt"
	"math/big"
	"strconv"

	"nofx/logger"
	"nofx/trader/types"

	"github.com/gate/gateapi-go/v7"
)

// maxSlipRatio 市价滑点比率的上限：文档为滑点比率最大 50%
const maxSlipRatio = "0.5"

// validateSlipRatio 校验调用方给出的滑点比率：0 表示用交易所默认，(0, 0.5] 为自定义
func validateSlipRatio(ratio float64) (string, error) {
	if ratio == 0 {
		return "", nil
	}
	value, err := decimalFromFloat("slip ratio", ratio)
	if err != nil {
		return "", err
	}
	limit, err := parseDecimal("max slip ratio", maxSlipRatio)
	if err != nil {
		return "", err
	}
	if value.Sign() <= 0 || value.Cmp(limit) > 0 {
		return "", fmt.Errorf("slip ratio must be in (0, %s], got %v", maxSlipRatio, ratio)
	}
	return value.FloatString(6), nil
}

// negateDecimal 张数字符串取负（开空），保持原有小数位
func negateDecimal(contracts string) (string, error) {
	value, err := parseDecimal("contracts", contracts)
	if err != nil {
		return "", err
	}
	return formatDecimal(value.Neg(value), decimalScale(contracts))
}

// slipDisplay 滑点比率用于日志的展示；空字符串表示用交易所默认
func slipDisplay(slip string) string {
	if slip == "" {
		return "default"
	}
	return slip
}

// openProtected 多空共用的随单保护开仓：side 为 "LONG"/"SHORT"
func (t *GateTrader) openProtected(req types.ProtectedOpenRequest, side string) (types.ProtectedOpenResult, error) {
	if req.NotionalUSD <= 0 {
		return types.ProtectedOpenResult{}, fmt.Errorf("notional must be positive, got %v", req.NotionalUSD)
	}
	slip, err := validateSlipRatio(req.SlipRatio)
	if err != nil {
		return types.ProtectedOpenResult{}, err
	}

	symbol := t.convertSymbol(req.Symbol)
	spec, err := t.GetContractSpec(symbol)
	if err != nil {
		return types.ProtectedOpenResult{}, err
	}

	// 开仓取价的唯一来源：Gate 最新价（D8）。名义价值换算与成交值都基于它。
	price, err := t.GetMarketPrice(symbol)
	if err != nil {
		return types.ProtectedOpenResult{}, err
	}

	quote, err := spec.SizeForNotional(price, req.NotionalUSD, req.MinNotional)
	if err != nil {
		return types.ProtectedOpenResult{}, err
	}
	if !quote.Orderable() {
		return types.ProtectedOpenResult{}, fmt.Errorf(
			"open skipped for %s: %s (notional %.4f USDT, price %v, size step %s)",
			symbol, quote.Skip, req.NotionalUSD, price, spec.SizeStep)
	}

	// 张数：多正空负；数量换算一律用不带符号的值
	signedSize := quote.Contracts
	if side == "SHORT" {
		signedSize, err = negateDecimal(quote.Contracts)
		if err != nil {
			return types.ProtectedOpenResult{}, err
		}
	}

	order := gateapi.FuturesOrder{
		Contract:             symbol,
		Size:                 signedSize,
		Price:                "0", // 市价
		Tif:                  "ioc",
		Text:                 "t-nofx",
		MarketOrderSlipRatio: slip,
	}
	// 保护不变量：随开仓单一并提交止损止盈；两价合法且分居两侧，触发方向由交易所按委托方向推导。
	// 校验放在任何有副作用的调用之前：无效保护直接拒绝开仓。
	if err := ApplyAttachedTPSL(&order, side, req.StopLoss, req.TakeProfit); err != nil {
		return types.ProtectedOpenResult{}, fmt.Errorf("invalid protection for %s: %w", symbol, err)
	}

	// 保证金模式与杠杆按 trader 的设置一并生效并读回，失败不开仓（D38）。
	// 不显式设置时首次建仓的合约会落到全仓最大杠杆（W2/W3 实测），设置没有生效就不能开仓。
	if err := t.ApplyMarginSettings(symbol, req.IsCrossMargin, req.Leverage); err != nil {
		return types.ProtectedOpenResult{}, fmt.Errorf("refusing to open %s: %w", symbol, err)
	}

	logger.Infof("  [Gate] OpenProtected: side=%s, contract=%s, size=%s, sl=%v, tp=%v, slip=%s",
		side, symbol, order.Size, req.StopLoss, req.TakeProfit, slipDisplay(slip))

	result, _, err := t.client.FuturesApi.CreateFuturesOrder(t.ctx, "usdt", order, nil)
	if err != nil {
		return types.ProtectedOpenResult{}, fmt.Errorf("failed to open %s position with protection: %w", side, err)
	}
	t.clearCache()

	// action_mode 默认 FULL：返回值即终态（计划文档 F25），成交价必填可用
	fillPrice, err := strconv.ParseFloat(result.FillPrice, 64)
	if err != nil {
		return types.ProtectedOpenResult{}, fmt.Errorf(
			"order %d finished but fill price %q is not parseable: %w", result.Id, result.FillPrice, err)
	}
	if fillPrice <= 0 {
		return types.ProtectedOpenResult{}, fmt.Errorf(
			"order %d finished but fill price %q is not positive", result.Id, result.FillPrice)
	}

	// 实际名义价值 = 张数 × 每张币数量 × 成交价（与换算时的参考价可能不同）
	quantity, err := spec.QuantityFromContracts(quote.Contracts)
	if err != nil {
		return types.ProtectedOpenResult{}, err
	}
	unitValue, err := spec.unitValue(fillPrice)
	if err != nil {
		return types.ProtectedOpenResult{}, err
	}
	contractsValue, err := parseDecimal("contracts", quote.Contracts)
	if err != nil {
		return types.ProtectedOpenResult{}, err
	}
	notional := new(big.Rat).Mul(unitValue, contractsValue)

	res := types.ProtectedOpenResult{
		OrderID:   strconv.FormatInt(result.Id, 10),
		Contracts: quote.Contracts,
		Notional:  ratFloat(notional),
		FillPrice: fillPrice,
		Quantity:  quantity,
	}
	logger.Infof("  [Gate] Opened %s position with attached TP/SL: orderId=%s, contracts=%s, fillPrice=%.6f",
		side, res.OrderID, res.Contracts, res.FillPrice)
	return res, nil
}

// OpenLongProtected 实现随单保护开多
func (t *GateTrader) OpenLongProtected(req types.ProtectedOpenRequest) (types.ProtectedOpenResult, error) {
	return t.openProtected(req, "LONG")
}

// OpenShortProtected 实现随单保护开空
func (t *GateTrader) OpenShortProtected(req types.ProtectedOpenRequest) (types.ProtectedOpenResult, error) {
	return t.openProtected(req, "SHORT")
}

// VerifyOrderProtection 核对开仓委托的保护单存在（保护不变量，计划文档第 5 节）：
// 1) 委托回读的随单止损止盈触发价都在（随单 tpsl 已被交易所接受）；
// 2) 否则该合约的未触发条件单同时包含本次开仓方向的止损与止盈（随单 tpsl 在委托
//    完结后转为委托级条件单的形态，W2 实测确认实际行为）。
// 方向从委托张数符号推导（多正空负），列表里只统计同侧的条件单：
// 遗留的反向止损止盈对不能让新仓位被误判为受保护。
// 两条路径都不满足时报错，错误里带上实际观察到的条件单情况。
func (t *GateTrader) VerifyOrderProtection(symbol, orderID string) error {
	if orderID == "" {
		return errors.New("cannot verify protection without order id")
	}
	symbol = t.convertSymbol(symbol)

	// 委托方向：多正空负；查询失败时方向未知（空串），列表按同侧配对核对
	side := ""
	order, _, err := t.client.FuturesApi.GetFuturesOrder(t.ctx, "usdt", orderID)
	if err == nil {
		if order.TpslSlTriggerPrice != "" && order.TpslTpTriggerPrice != "" {
			logger.Infof("  [Gate] Protection verified via order tpsl: %s sl=%s tp=%s",
				orderID, order.TpslSlTriggerPrice, order.TpslTpTriggerPrice)
			return nil
		}
		if size, parseErr := parseDecimal("order size", order.Size); parseErr == nil && size.Sign() != 0 {
			side = "SHORT"
			if size.Sign() > 0 {
				side = "LONG"
			}
		}
	} else {
		// 已完结的委托在完结 10 分钟后不能按 ID 查询（计划文档 F25）；
		// 开仓后立即核对不受影响，查询失败不阻断条件单列表核对
		logger.Infof("  [Gate] Order %s lookup failed (%v), falling back to trigger order list", orderID, err)
	}

	views, err := t.ListTriggerOrders(symbol)
	if err != nil {
		return fmt.Errorf("protection verification for %s failed: %w", symbol, err)
	}
	slSides := map[string]bool{}
	tpSides := map[string]bool{}
	for _, view := range views {
		if view.Contract != symbol || !view.KindKnown || view.Side == "" {
			continue
		}
		if side != "" && view.Side != side {
			continue
		}
		switch view.Kind {
		case ProtectiveStopLoss:
			slSides[view.Side] = true
		case ProtectiveTakeProfit:
			tpSides[view.Side] = true
		}
	}
	hasSL, hasTP := false, false
	for viewSide := range slSides {
		if tpSides[viewSide] {
			hasSL, hasTP = true, true
		}
	}
	if hasSL && hasTP {
		logger.Infof("  [Gate] Protection verified via trigger orders: %s side=%s has %d open trigger orders (SL+TP)", symbol, side, len(views))
		return nil
	}
	return fmt.Errorf(
		"protection missing for %s (order %s, side %q): stop loss present=%v, take profit present=%v, open trigger orders=%d; caller must close immediately",
		symbol, orderID, side, hasSL, hasTP, len(views))
}
