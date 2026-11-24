package gate

// 开平仓原语与条件单撤销。AI 自动交易的开仓不经过本文件的 OpenLong/OpenShort
// （它们是助手手动交易与网格使用的普通市价原语，不带保护）；
// 随单保护开仓见 protected_open.go，保护单构造与撤销见 protective_orders.go。
// 数量换算统一走 GetContractSpec 的精确十进制（禁止浮点截断与强制 1 张），
// 旧的 getContract 与按浮点读取 quanto_multiplier 已删除（计划文档 F34）。

import (
	"errors"
	"fmt"
	"math/big"
	"nofx/logger"
	"nofx/trader/types"
	"strconv"
	"strings"

	"github.com/antihax/optional"
	"github.com/gate/gateapi-go/v7"
)

// SetLeverage sets the leverage for a symbol
func (t *GateTrader) SetLeverage(symbol string, leverage int) error {
	symbol = t.convertSymbol(symbol)

	_, _, err := t.client.FuturesApi.UpdatePositionLeverage(t.ctx, "usdt", symbol, fmt.Sprintf("%d", leverage), nil)
	if err != nil {
		// Gate.io may return error if leverage is already set
		if strings.Contains(err.Error(), "RISK_LIMIT_EXCEEDED") {
			logger.Warnf("  [Gate] Leverage %d exceeds limit for %s", leverage, symbol)
			return nil
		}
		return fmt.Errorf("failed to set leverage: %w", err)
	}

	logger.Infof("  [Gate] Leverage set to %dx for %s", leverage, symbol)
	return nil
}

// SetMarginMode sets margin mode (cross or isolated)
func (t *GateTrader) SetMarginMode(symbol string, isCrossMargin bool) error {
	// Gate.io uses leverage=0 for cross margin, positive number for isolated
	// This is handled through UpdatePositionLeverage with cross_leverage_limit
	// For now, we'll skip explicit margin mode setting as it's tied to leverage
	logger.Infof("  [Gate] Margin mode is set through leverage (0=cross)")
	return nil
}

// openMarket 多空共用的普通市价开仓原语：币数量按最小步长向下取整换算张数，
// 不足一个最小步长时报错（删除旧的浮点截断与强制 1 张，计划文档 F3/F23）。
func (t *GateTrader) openMarket(symbol string, quantity float64, leverage int, side string) (map[string]interface{}, error) {
	symbol = t.convertSymbol(symbol)

	spec, err := t.GetContractSpec(symbol)
	if err != nil {
		return nil, err
	}
	contracts, err := spec.ContractsFromQuantityFloor(quantity)
	if err != nil {
		return nil, fmt.Errorf("open %s %s skipped: %w", side, symbol, err)
	}

	size := contracts
	if side == "SHORT" {
		size, err = negateDecimal(contracts)
		if err != nil {
			return nil, err
		}
	}

	// 沿用现状：杠杆 best-effort 设置，失败不阻断开仓（子任务 5 重写杠杆与保证金模式）
	if leverage > 0 {
		if err := t.SetLeverage(symbol, leverage); err != nil {
			logger.Warnf("  [Gate] Failed to set leverage: %v", err)
		}
	}

	order := gateapi.FuturesOrder{
		Contract: symbol,
		Size:     size,
		Price:    "0", // 市价单
		Tif:      "ioc",
		Text:     "t-nofx",
	}

	logger.Infof("  [Gate] Open%s: symbol=%s, size=%s, leverage=%d", side, symbol, size, leverage)

	result, _, err := t.client.FuturesApi.CreateFuturesOrder(t.ctx, "usdt", order, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s position: %w", side, err)
	}
	t.clearCache()

	fillPrice, _ := strconv.ParseFloat(result.FillPrice, 64)

	logger.Infof("  [Gate] Opened %s position: orderId=%d, fillPrice=%.4f", side, result.Id, fillPrice)

	return map[string]interface{}{
		"orderId":   strconv.FormatInt(result.Id, 10),
		"symbol":    t.revertSymbol(symbol),
		"status":    "FILLED",
		"fillPrice": fillPrice,
		"avgPrice":  fillPrice,
	}, nil
}

// OpenLong opens a long position
func (t *GateTrader) OpenLong(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	return t.openMarket(symbol, quantity, leverage, "LONG")
}

// OpenShort opens a short position
func (t *GateTrader) OpenShort(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	return t.openMarket(symbol, quantity, leverage, "SHORT")
}

// findPositionSize 读取该合约指定方向的当前持仓张数（字符串，带方向符号）。
// 直接查交易所不经 GetPositions 缓存：全平前的持仓状态必须是当前值。
func (t *GateTrader) findPositionSize(symbol, side string) (string, error) {
	positions, _, err := t.client.FuturesApi.ListPositions(t.ctx, "usdt", nil)
	if err != nil {
		return "", fmt.Errorf("failed to get positions: %w", err)
	}
	for _, pos := range positions {
		if pos.Contract != symbol {
			continue
		}
		size, err := parseDecimal("position size", pos.Size)
		if err != nil {
			return "", fmt.Errorf("failed to parse position %s: %w", pos.Contract, err)
		}
		if size.Sign() == 0 {
			continue
		}
		// 多头持仓为正、空头为负（单向持仓模式）
		if (side == "SHORT") == (size.Sign() < 0) {
			return pos.Size, nil
		}
	}
	return "", fmt.Errorf("%s position not found for %s", strings.ToLower(side), symbol)
}

// closePosition 多空共用的平仓原语。quantity=0 为全平：按文档语义提交 close=true + size=0
// （C.1 节：单仓模式下平仓需要设置 size 为 0、close 为 true），不再换算张数；
// quantity>0 为部分平（网格/助手使用）：币数量按最小步长向下取整 + reduce_only，
// 不足一个最小步长时报错。
func (t *GateTrader) closePosition(symbol string, quantity float64, side string) (map[string]interface{}, error) {
	symbol = t.convertSymbol(symbol)

	order := gateapi.FuturesOrder{
		Contract: symbol,
		Price:    "0", // 市价单
		Tif:      "ioc",
		Text:     "t-nofx-close",
	}

	if quantity == 0 {
		// 确认仓位存在与方向，给出明确错误而不是依赖交易所的模糊拒绝
		if _, err := t.findPositionSize(symbol, side); err != nil {
			return nil, err
		}
		order.Close = true
		order.Size = "0"
		logger.Infof("  [Gate] CloseAll%s: symbol=%s (close=true, size=0)", side, symbol)
	} else {
		spec, err := t.GetContractSpec(symbol)
		if err != nil {
			return nil, err
		}
		contracts, err := spec.ContractsFromQuantityFloor(quantity)
		if err != nil {
			return nil, fmt.Errorf("close %s %s skipped: %w", side, symbol, err)
		}
		// 平多卖出为负、平空买入为正
		size := contracts
		if side == "LONG" {
			size, err = negateDecimal(contracts)
			if err != nil {
				return nil, err
			}
		}
		order.Size = size
		order.ReduceOnly = true
		logger.Infof("  [Gate] Close%s: symbol=%s, size=%s (reduce_only)", side, symbol, size)
	}

	result, _, err := t.client.FuturesApi.CreateFuturesOrder(t.ctx, "usdt", order, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to close %s position: %w", side, err)
	}
	t.clearCache()

	fillPrice, _ := strconv.ParseFloat(result.FillPrice, 64)

	logger.Infof("  [Gate] Closed %s position: orderId=%d, fillPrice=%.4f", side, result.Id, fillPrice)

	return map[string]interface{}{
		"orderId":   strconv.FormatInt(result.Id, 10),
		"symbol":    t.revertSymbol(symbol),
		"status":    "FILLED",
		"fillPrice": fillPrice,
		"avgPrice":  fillPrice,
	}, nil
}

// CloseLong closes a long position (quantity=0 means close all)
func (t *GateTrader) CloseLong(symbol string, quantity float64) (map[string]interface{}, error) {
	return t.closePosition(symbol, quantity, "LONG")
}

// CloseShort closes a short position (quantity=0 means close all)
func (t *GateTrader) CloseShort(symbol string, quantity float64) (map[string]interface{}, error) {
	return t.closePosition(symbol, quantity, "SHORT")
}

// GetMarketPrice gets the current market price
func (t *GateTrader) GetMarketPrice(symbol string) (float64, error) {
	symbol = t.convertSymbol(symbol)

	opts := &gateapi.ListFuturesTickersOpts{
		Contract: optional.NewString(symbol),
	}

	tickers, _, err := t.client.FuturesApi.ListFuturesTickers(t.ctx, "usdt", opts)
	if err != nil {
		return 0, fmt.Errorf("failed to get market price: %w", err)
	}

	if len(tickers) == 0 {
		return 0, fmt.Errorf("no ticker data for %s", symbol)
	}

	// 开/平仓取价的唯一来源：脏数据直接报错（带字段名与原始值），不回落 0
	price, err := strconv.ParseFloat(tickers[0].Last, 64)
	if err != nil || price <= 0 {
		return 0, fmt.Errorf("invalid last price for %s: %q", symbol, tickers[0].Last)
	}
	return price, nil
}

// errStandaloneProtective 独立止损止盈委托在 Gate 上不再支持：AI 开仓的止损止盈随开仓单
// 一并提交（protected_open.go），修改走撤旧建新（protective_orders.go）。
// 旧的独立实现四个方向的触发规则都与文档相反、close=true 带非零 size、无 order_type
// （计划文档 F6/F8），已删除；方法因 types.Trader 接口被网格路径占用而保留。
var errStandaloneProtective = errors.New(
	"standalone stop-loss/take-profit orders are not supported on gate: " +
		"open with attached TP/SL via ProtectedOpener, amend via cancel-and-replace")

// SetStopLoss sets a stop loss order
func (t *GateTrader) SetStopLoss(symbol string, positionSide string, quantity, stopPrice float64) error {
	return errStandaloneProtective
}

// SetTakeProfit sets a take profit order
func (t *GateTrader) SetTakeProfit(symbol string, positionSide string, quantity, takeProfitPrice float64) error {
	return errStandaloneProtective
}

// CancelStopLossOrders cancels stop loss orders (only classified stop-loss trigger orders, F10)
func (t *GateTrader) CancelStopLossOrders(symbol string) error {
	_, err := t.CancelProtectiveOrdersByKind(symbol, ProtectiveStopLoss)
	return err
}

// CancelTakeProfitOrders cancels take profit orders (only classified take-profit trigger orders, F10)
func (t *GateTrader) CancelTakeProfitOrders(symbol string) error {
	_, err := t.CancelProtectiveOrdersByKind(symbol, ProtectiveTakeProfit)
	return err
}

// CancelAllOrders cancels all pending orders for a symbol: resting orders and all
// open trigger orders (类型不区分，全撤是该接口的语义；按类型撤销走 CancelProtectiveOrdersByKind)
func (t *GateTrader) CancelAllOrders(symbol string) error {
	symbol = t.convertSymbol(symbol)

	// Cancel regular orders
	cancelOpts := &gateapi.CancelFuturesOrdersOpts{
		Contract: optional.NewString(symbol),
	}
	_, _, err := t.client.FuturesApi.CancelFuturesOrders(t.ctx, "usdt", cancelOpts)
	if err != nil {
		// Ignore if no orders to cancel
		if !strings.Contains(err.Error(), "ORDER_NOT_FOUND") {
			logger.Warnf("  [Gate] Error canceling orders: %v", err)
		}
	}

	// Cancel trigger orders
	views, err := t.ListTriggerOrders(symbol)
	if err != nil {
		logger.Warnf("  [Gate] Failed to list trigger orders for cancel: %v", err)
		return nil
	}
	for _, view := range views {
		if err := t.CancelProtectiveOrder(view.OrderID()); err != nil {
			logger.Warnf("  [Gate] Failed to cancel trigger order %s: %v", view.OrderID(), err)
		}
	}

	return nil
}

// CancelStopOrders cancels all stop orders (stop loss and take profit)
func (t *GateTrader) CancelStopOrders(symbol string) error {
	if _, err := t.CancelProtectiveOrdersByKind(symbol, ProtectiveStopLoss); err != nil {
		return err
	}
	_, err := t.CancelProtectiveOrdersByKind(symbol, ProtectiveTakeProfit)
	return err
}

// FormatQuantity formats quantity to correct precision (contracts, floor to size step)
func (t *GateTrader) FormatQuantity(symbol string, quantity float64) (string, error) {
	spec, err := t.GetContractSpec(symbol)
	if err != nil {
		return "", err
	}
	return spec.ContractsFromQuantityFloor(quantity)
}

// GetOrderStatus gets the status of an order
func (t *GateTrader) GetOrderStatus(symbol string, orderID string) (map[string]interface{}, error) {
	symbol = t.convertSymbol(symbol)

	order, _, err := t.client.FuturesApi.GetFuturesOrder(t.ctx, "usdt", orderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get order status: %w", err)
	}

	spec, err := t.GetContractSpec(symbol)
	if err != nil {
		return nil, err
	}

	orderSize, err := parseDecimal("order size", order.Size)
	if err != nil {
		return nil, fmt.Errorf("failed to parse order %s: %w", orderID, err)
	}
	orderLeft, err := parseOptionalRat("order left", order.Left)
	if err != nil {
		return nil, fmt.Errorf("failed to parse order %s: %w", orderID, err)
	}

	fillPrice, _ := strconv.ParseFloat(order.FillPrice, 64)
	tkFee, _ := strconv.ParseFloat(order.Tkfr, 64)
	mkFee, _ := strconv.ParseFloat(order.Mkfr, 64)
	totalFee := tkFee + mkFee

	// Map status
	status := "NEW"
	switch order.Status {
	case "finished":
		if order.FinishAs == "filled" {
			status = "FILLED"
		} else if order.FinishAs == "cancelled" {
			status = "CANCELED"
		} else {
			status = "CLOSED"
		}
	case "open":
		status = "NEW"
	}

	side := "BUY"
	if orderSize.Sign() < 0 {
		side = "SELL"
	}

	// 成交张数 = |size| − left（卖单的 size 为负数），乘每张的币数量得到实际成交量。
	// 张数差为负时按 0 计（left 不应超过 |size|，防脏数据放大）
	executedContracts := new(big.Rat).Abs(orderSize)
	executedContracts.Sub(executedContracts, orderLeft)
	if executedContracts.Sign() < 0 {
		executedContracts.SetInt64(0)
	}
	executedQty := spec.contractQuantity(executedContracts)

	return map[string]interface{}{
		"orderId":     orderID,
		"symbol":      t.revertSymbol(symbol),
		"status":      status,
		"avgPrice":    fillPrice,
		"executedQty": executedQty,
		"side":        side,
		"type":        order.Tif,
		"time":        int64(order.CreateTime * 1000),
		"updateTime":  int64(order.FinishTime * 1000),
		"commission":  totalFee,
	}, nil
}

// GetOpenOrders gets open/pending orders
func (t *GateTrader) GetOpenOrders(symbol string) ([]types.OpenOrder, error) {
	symbol = t.convertSymbol(symbol)

	spec, err := t.GetContractSpec(symbol)
	if err != nil {
		return nil, err
	}

	opts := &gateapi.ListFuturesOrdersOpts{
		Contract: optional.NewString(symbol),
	}

	orders, _, err := t.client.FuturesApi.ListFuturesOrders(t.ctx, "usdt", "open", opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get open orders: %w", err)
	}

	var result []types.OpenOrder
	for _, order := range orders {
		price, _ := strconv.ParseFloat(order.Price, 64)

		orderSize, err := parseDecimal("order size", order.Size)
		if err != nil {
			return nil, fmt.Errorf("failed to parse order %d: %w", order.Id, err)
		}

		side := "BUY"
		if orderSize.Sign() < 0 {
			side = "SELL"
		}

		// Convert contract count to actual token quantity (display only)
		quantity := spec.contractQuantity(new(big.Rat).Abs(orderSize))

		result = append(result, types.OpenOrder{
			OrderID:  strconv.FormatInt(order.Id, 10),
			Symbol:   t.revertSymbol(order.Contract),
			Side:     side,
			Type:     "LIMIT",
			Price:    price,
			Quantity: quantity,
			Status:   "NEW",
		})
	}

	// Also get trigger orders
	views, err := t.ListTriggerOrders(symbol)
	if err != nil {
		return result, nil
	}
	for _, view := range views {
		// 小数张条件单的初始委托在 amount（字符串），整数张在 size（F32）；
		// amount 非空但解析失败是脏数据，报错而不是回落 size
		quantityRat, err := parseOptionalRat("trigger order amount", view.InitialAmount)
		if err != nil {
			return nil, fmt.Errorf("failed to parse trigger order %s: %w", view.OrderID(), err)
		}
		if quantityRat.Sign() == 0 && view.InitialAmount == "" {
			quantityRat.SetInt64(view.InitialSize)
		}

		side := "BUY"
		if quantityRat.Sign() < 0 {
			side = "SELL"
		}

		// 类型展示以可判定的止损/止盈为准；方向未知的普通条件单按触发规则近似
		orderType := "STOP_MARKET"
		switch {
		case view.KindKnown && view.Kind == ProtectiveTakeProfit:
			orderType = "TAKE_PROFIT_MARKET"
		case !view.KindKnown && view.TriggerRule == PriceRuleGE:
			orderType = "TAKE_PROFIT_MARKET"
		}

		quantity := spec.contractQuantity(new(big.Rat).Abs(quantityRat))

		result = append(result, types.OpenOrder{
			OrderID:   view.OrderID(),
			Symbol:    t.revertSymbol(view.Contract),
			Side:      side,
			Type:      orderType,
			StopPrice: view.TriggerPrice,
			Quantity:  quantity,
			Status:    "NEW",
		})
	}

	return result, nil
}
