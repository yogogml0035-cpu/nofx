package gate

// 止损止盈管理：每轮读取持仓当前的止损止盈（来源是未触发条件单），按模型给出的新价格
// 撤旧建新。条件单改单接口不可用（子任务 3 research H.2：新路径 404、旧路径 code 1077），
// 改单只能撤销旧单再挂新单；执行顺序是「先挂新单并核对 → 撤旧单 → 核对旧单不存在」，
// 失败时撤销新单恢复原保护——任何时刻仓位都保留同类型的保护单（保护不变量）。
// 触发价必须对齐合约价格步长：不对齐的触发价随单 tpsl 会被接受但不创建条件单（F44），
// 仓位级条件单会被交易所直接拒绝。
// 文档依据：子任务 3 research/gate-price-order-docs.md；本子任务 probe/ 的真实探针。

import (
	"fmt"
	"math"
	"math/big"
	"time"

	"nofx/logger"
	"nofx/trader/types"

	"github.com/gate/gateapi-go/v7"
)

// protectiveKindLabel 错误信息里的类型名（英文，包内错误沿用英文）
func protectiveKindLabel(kind ProtectiveKind) string {
	switch kind {
	case ProtectiveStopLoss:
		return "stop loss"
	case ProtectiveTakeProfit:
		return "take profit"
	}
	return fmt.Sprintf("protective kind %d", int(kind))
}

// effectiveProtection 从同侧条件单中取每类距现价最近（最先触发）的一条；
// 多条并存时计数如实给出，改单会替换掉该类型的全部旧单。
func effectiveProtection(views []TriggerOrderView, market float64) types.PositionProtection {
	protection := types.PositionProtection{}
	var slDist, tpDist float64
	for _, view := range views {
		distance := math.Abs(view.TriggerPrice - market)
		switch view.Kind {
		case ProtectiveStopLoss:
			protection.StopLossOrders++
			if protection.StopLoss == 0 || distance < slDist {
				protection.StopLoss, slDist = view.TriggerPrice, distance
			}
		case ProtectiveTakeProfit:
			protection.TakeProfitOrders++
			if protection.TakeProfit == 0 || distance < tpDist {
				protection.TakeProfit, tpDist = view.TriggerPrice, distance
			}
		}
	}
	protection.Known = protection.StopLoss > 0 && protection.TakeProfit > 0
	if !protection.Known {
		switch {
		case protection.StopLoss == 0 && protection.TakeProfit == 0:
			protection.Reason = "no open stop-loss or take-profit orders for this side"
		case protection.StopLoss == 0:
			protection.Reason = "no open stop-loss order for this side"
		default:
			protection.Reason = "no open take-profit order for this side"
		}
	}
	return protection
}

// listProtectiveViews 列出该合约上同侧、类型可判定的未触发条件单
func (t *GateTrader) listProtectiveViews(symbol, side string) ([]TriggerOrderView, error) {
	normalized, err := normalizeSide(side)
	if err != nil {
		return nil, err
	}
	symbol = t.convertSymbol(symbol)
	views, err := t.ListTriggerOrders(symbol)
	if err != nil {
		return nil, err
	}
	filtered := make([]TriggerOrderView, 0, len(views))
	for _, view := range views {
		if view.Contract != symbol || !view.KindKnown || view.Side != normalized {
			continue
		}
		filtered = append(filtered, view)
	}
	return filtered, nil
}

// PositionProtection 读取该持仓当前的止损止盈：取同侧条件单中每类距现价最近的一条作为
// 有效触发价。读取失败返回错误，由调用方标为不可读；不做猜测。
func (t *GateTrader) PositionProtection(symbol, side string) (types.PositionProtection, error) {
	symbol = t.convertSymbol(symbol)
	views, err := t.listProtectiveViews(symbol, side)
	if err != nil {
		return types.PositionProtection{}, err
	}
	market, err := t.GetMarketPrice(symbol)
	if err != nil {
		return types.PositionProtection{}, err
	}
	return effectiveProtection(views, market), nil
}

// gateProtectiveKind 把能力层的保护类型映射到 Gate 条件单类型
func gateProtectiveKind(kind types.ProtectionKind) (ProtectiveKind, error) {
	switch kind {
	case types.ProtectionStopLoss:
		return ProtectiveStopLoss, nil
	case types.ProtectionTakeProfit:
		return ProtectiveTakeProfit, nil
	}
	return 0, fmt.Errorf("invalid protection kind %q", string(kind))
}

// validateKindSide 校验（对齐后的）新触发价位于现价的正确一侧
func validateKindSide(side string, kind types.ProtectionKind, market float64, price *big.Rat) error {
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
	text := price.FloatString(8)
	switch normalized {
	case "LONG":
		if kind == types.ProtectionStopLoss && price.Cmp(marketValue) >= 0 {
			return fmt.Errorf("long stop loss %s must be below current price %v", text, market)
		}
		if kind == types.ProtectionTakeProfit && price.Cmp(marketValue) <= 0 {
			return fmt.Errorf("long take profit %s must be above current price %v", text, market)
		}
	case "SHORT":
		if kind == types.ProtectionStopLoss && price.Cmp(marketValue) <= 0 {
			return fmt.Errorf("short stop loss %s must be above current price %v", text, market)
		}
		if kind == types.ProtectionTakeProfit && price.Cmp(marketValue) >= 0 {
			return fmt.Errorf("short take profit %s must be below current price %v", text, market)
		}
	}
	return nil
}

// lossAtStopOnPosition 价格到止损价时的亏损（USDT），按交易所读数精确计算：
// 数量 = |张数| × quanto_multiplier，亏损 = 数量 × 入场价到止损价的距离。
// 止损在盈利一侧（多单止损高于入场价、空单止损低于入场价）时亏损为 0。
func lossAtStopOnPosition(spec ContractSpec, position gateapi.Position, side string, stop *big.Rat) (*big.Rat, error) {
	size, err := parseDecimal("position size", position.Size)
	if err != nil {
		return nil, err
	}
	size.Abs(size)
	quantity := new(big.Rat).Mul(size, spec.multiplier)
	entry, err := parseDecimal("position entry price", position.EntryPrice)
	if err != nil {
		return nil, err
	}
	if entry.Sign() <= 0 {
		return nil, fmt.Errorf("position entry price must be positive, got %q", position.EntryPrice)
	}
	var distance *big.Rat
	if side == "LONG" {
		if stop.Cmp(entry) >= 0 {
			return new(big.Rat), nil
		}
		distance = new(big.Rat).Sub(entry, stop)
	} else {
		if stop.Cmp(entry) <= 0 {
			return new(big.Rat), nil
		}
		distance = new(big.Rat).Sub(stop, entry)
	}
	return new(big.Rat).Mul(quantity, distance), nil
}

// protectiveKindViews 筛选指定类型的同侧条件单
func protectiveKindViews(views []TriggerOrderView, kind ProtectiveKind) []TriggerOrderView {
	matched := make([]TriggerOrderView, 0, len(views))
	for _, view := range views {
		if view.Kind == kind {
			matched = append(matched, view)
		}
	}
	return matched
}

// verifyProtectiveOrder 核对新保护单出现在未触发列表中且触发价正确。
// 交易所创建条件单后列表可能略有延迟，最多核对 3 次（间隔 200ms）。
func (t *GateTrader) verifyProtectiveOrder(symbol, side string, kind ProtectiveKind, orderID string, price float64) error {
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		views, err := t.listProtectiveViews(symbol, side)
		if err != nil {
			lastErr = err
		} else {
			for _, view := range views {
				if view.OrderID() == orderID && view.Kind == kind && math.Abs(view.TriggerPrice-price) <= 1e-9*math.Max(1, math.Abs(price)) {
					return nil
				}
			}
			lastErr = fmt.Errorf(
				"new %s order %s at %v is not visible in the open trigger order list", protectiveKindLabel(kind), orderID, price)
		}
		if attempt < 3 {
			time.Sleep(200 * time.Millisecond)
		}
	}
	return lastErr
}

// restoreReplacement 撤销新单并确认仓位仍有同类保护；没有时尝试按原价重建，
// 重建失败返回包装了 types.ErrPositionUnprotected 的错误（调用方必须立即平仓）。
func (t *GateTrader) restoreReplacement(symbol, side string, kind ProtectiveKind, newOrderID string, oldPrice float64) error {
	if newOrderID != "" {
		if err := t.CancelProtectiveOrder(newOrderID); err != nil {
			logger.Warnf("  [Gate] Failed to cancel replacement %s order %s: %v", protectiveKindLabel(kind), newOrderID, err)
		}
	}
	views, err := t.listProtectiveViews(symbol, side)
	if err != nil {
		return fmt.Errorf("cannot confirm %s protection after failed replacement: %w (%w)",
			protectiveKindLabel(kind), err, types.ErrPositionUnprotected)
	}
	if len(protectiveKindViews(views, kind)) > 0 {
		return nil
	}
	if oldPrice <= 0 {
		return fmt.Errorf("no %s protection remains on %s after failed replacement (%w)",
			protectiveKindLabel(kind), symbol, types.ErrPositionUnprotected)
	}
	order, err := BuildPositionProtectiveOrder(symbol, side, kind, oldPrice)
	if err != nil {
		return fmt.Errorf("restore %s protection at %v failed: %w (%w)", protectiveKindLabel(kind), oldPrice, err, types.ErrPositionUnprotected)
	}
	restoredID, err := t.PlaceProtectiveOrder(order)
	if err != nil {
		return fmt.Errorf("restore %s protection at %v failed: %w (%w)", protectiveKindLabel(kind), oldPrice, err, types.ErrPositionUnprotected)
	}
	if err := t.verifyProtectiveOrder(symbol, side, kind, restoredID, oldPrice); err != nil {
		return fmt.Errorf("restored %s order %s not verified: %w (%w)", protectiveKindLabel(kind), restoredID, err, types.ErrPositionUnprotected)
	}
	logger.Infof("  [Gate] Restored %s protection at %v (id=%s) after failed replacement", protectiveKindLabel(kind), oldPrice, restoredID)
	return nil
}

// UpdateProtection 把止损或止盈改到新价格：校验持仓与亏损上限、对齐价格步长、
// 先挂新单并核对，再撤销旧单并核对旧单不存在；任一步失败都恢复调用前的保护状态。
// 失败时不会留下无保护的持仓；保护已无法确认时返回包装了 types.ErrPositionUnprotected 的错误。
func (t *GateTrader) UpdateProtection(req types.ProtectionUpdateRequest) (types.ProtectionUpdateResult, error) {
	normalized, err := normalizeSide(req.Side)
	if err != nil {
		return types.ProtectionUpdateResult{}, err
	}
	gateKind, err := gateProtectiveKind(req.Kind)
	if err != nil {
		return types.ProtectionUpdateResult{}, err
	}
	if math.IsNaN(req.Price) || math.IsInf(req.Price, 0) || req.Price <= 0 {
		return types.ProtectionUpdateResult{}, fmt.Errorf("new trigger price must be a positive number, got %v", req.Price)
	}
	symbol := t.convertSymbol(req.Symbol)

	spec, err := t.GetContractSpec(symbol)
	if err != nil {
		return types.ProtectionUpdateResult{}, err
	}
	// 持仓必须先读取：不存在时任何请求都不发
	position, err := t.findPosition(symbol, normalized)
	if err != nil {
		return types.ProtectionUpdateResult{}, err
	}
	market, err := t.GetMarketPrice(symbol)
	if err != nil {
		return types.ProtectionUpdateResult{}, err
	}

	// 对齐方向与开仓一致：多单向上、空单向下——止损只会更紧（不放大亏损），止盈不会更近
	align := PriceAlignUp
	if normalized == "SHORT" {
		align = PriceAlignDown
	}
	alignedText, alignedValue, err := spec.AlignTriggerPrice(req.Price, align)
	if err != nil {
		return types.ProtectionUpdateResult{}, err
	}
	if err := validateKindSide(normalized, req.Kind, market, alignedValue); err != nil {
		return types.ProtectionUpdateResult{}, err
	}

	// 执行时按最新持仓复核亏损上限（D34、F17）：不依赖决策解析时的旧快照
	if req.Kind == types.ProtectionStopLoss {
		if req.Principal <= 0 || math.IsNaN(req.Principal) || math.IsInf(req.Principal, 0) {
			return types.ProtectionUpdateResult{}, fmt.Errorf("principal must be positive to validate the stop loss, got %v", req.Principal)
		}
		if req.MaxLossPercent <= 0 || math.IsNaN(req.MaxLossPercent) || math.IsInf(req.MaxLossPercent, 0) {
			return types.ProtectionUpdateResult{}, fmt.Errorf("max loss percent must be positive, got %v", req.MaxLossPercent)
		}
		loss, err := lossAtStopOnPosition(spec, position, normalized, alignedValue)
		if err != nil {
			return types.ProtectionUpdateResult{}, err
		}
		principalValue, err := decimalFromFloat("principal", req.Principal)
		if err != nil {
			return types.ProtectionUpdateResult{}, err
		}
		percentValue, err := decimalFromFloat("max loss percent", req.MaxLossPercent)
		if err != nil {
			return types.ProtectionUpdateResult{}, err
		}
		limit := new(big.Rat).Mul(principalValue, percentValue)
		limit.Quo(limit, big.NewRat(100, 1))
		if loss.Cmp(limit) > 0 {
			return types.ProtectionUpdateResult{}, fmt.Errorf(
				"loss at stop %s USDT exceeds %v%% of principal (%s USDT): refusing to move the stop loss",
				loss.FloatString(4), req.MaxLossPercent, limit.FloatString(4))
		}
	}

	// 当前保护必须可读：旧单无法枚举时不能保证它被替换
	views, err := t.listProtectiveViews(symbol, normalized)
	if err != nil {
		return types.ProtectionUpdateResult{}, err
	}
	current := effectiveProtection(views, market)
	if !current.Known {
		return types.ProtectionUpdateResult{}, fmt.Errorf(
			"current protection for %s %s is not readable (%s); refusing to modify", normalized, symbol, current.Reason)
	}
	oldOrders := protectiveKindViews(views, gateKind)
	if len(oldOrders) == 0 {
		return types.ProtectionUpdateResult{}, fmt.Errorf(
			"no open %s orders for %s %s; refusing to modify", protectiveKindLabel(gateKind), normalized, symbol)
	}
	oldPrice := current.StopLoss
	if gateKind == ProtectiveTakeProfit {
		oldPrice = current.TakeProfit
	}

	order, err := BuildPositionProtectiveOrder(symbol, normalized, gateKind, ratFloat(alignedValue))
	if err != nil {
		return types.ProtectionUpdateResult{}, err
	}
	newOrderID, err := t.PlaceProtectiveOrder(order)
	if err != nil {
		return types.ProtectionUpdateResult{}, err
	}
	if err := t.verifyProtectiveOrder(symbol, normalized, gateKind, newOrderID, ratFloat(alignedValue)); err != nil {
		restoreErr := t.restoreReplacement(symbol, normalized, gateKind, newOrderID, oldPrice)
		return types.ProtectionUpdateResult{}, fmt.Errorf(
			"new %s order %s did not appear in the trigger order list: %w (restore: %v)",
			protectiveKindLabel(gateKind), newOrderID, err, restoreErr)
	}

	replaced := make([]string, 0, len(oldOrders))
	for _, old := range oldOrders {
		if old.OrderID() == newOrderID {
			continue
		}
		if err := t.CancelProtectiveOrder(old.OrderID()); err != nil {
			restoreErr := t.restoreReplacement(symbol, normalized, gateKind, newOrderID, oldPrice)
			return types.ProtectionUpdateResult{}, fmt.Errorf(
				"failed to cancel old %s order %s: %w (restore: %v)", protectiveKindLabel(gateKind), old.OrderID(), err, restoreErr)
		}
		replaced = append(replaced, old.OrderID())
	}

	// 核对旧单已不存在：列表里该类型只剩新单
	after, err := t.listProtectiveViews(symbol, normalized)
	if err != nil {
		restoreErr := t.restoreReplacement(symbol, normalized, gateKind, newOrderID, oldPrice)
		return types.ProtectionUpdateResult{}, fmt.Errorf(
			"cannot verify %s replacement: %w (restore: %v)", protectiveKindLabel(gateKind), err, restoreErr)
	}
	for _, view := range protectiveKindViews(after, gateKind) {
		if view.OrderID() == newOrderID {
			continue
		}
		restoreErr := t.restoreReplacement(symbol, normalized, gateKind, newOrderID, oldPrice)
		return types.ProtectionUpdateResult{}, fmt.Errorf(
			"old %s order %s still visible after cancellation (restore: %v)", protectiveKindLabel(gateKind), view.OrderID(), restoreErr)
	}

	logger.Infof("  [Gate] Protection updated: %s %s %v %v -> %s (new id=%s, replaced=%v)",
		normalized, symbol, gateKind, oldPrice, alignedText, newOrderID, replaced)
	return types.ProtectionUpdateResult{
		OrderID:          newOrderID,
		Kind:             req.Kind,
		OldPrice:         oldPrice,
		NewPrice:         ratFloat(alignedValue),
		ReplacedOrderIDs: replaced,
	}, nil
}

// 能力漂移必须在编译期暴露，不能静默退化为“不能修改保护”
var _ types.ProtectiveOrderManager = (*GateTrader)(nil)