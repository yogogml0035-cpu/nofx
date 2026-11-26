package gate

// 分批平仓：按模型给出的比例，用精确十进制把持仓张数换算为平仓张数，提交 reduce-only
// 市价单。比例 100% 属于全平，走 CloseLong/CloseShort；换算后不足一个最小步长时拒绝。
// 平仓张数不超过市价单最大张数（F35）。本文件不管理保护单：随单止盈止损由开仓时提交，
// 分批平仓不撤销、不改单（修改止损止盈见 protection_management.go）。

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	"nofx/logger"
	"nofx/trader/types"

	"github.com/gate/gateapi-go/v7"
)

// PartialCloseStatus 报告指定方向持仓能否分批平仓。
func (t *GateTrader) PartialCloseStatus(symbol, side string) (types.PartialCloseStatus, error) {
	normalized, err := normalizeSide(side)
	if err != nil {
		return types.PartialCloseStatus{}, err
	}
	symbol = t.convertSymbol(symbol)

	position, err := t.findPositionSize(symbol, normalized)
	if err != nil {
		return types.PartialCloseStatus{}, err
	}
	spec, err := t.GetContractSpec(symbol)
	if err != nil {
		return types.PartialCloseStatus{}, err
	}

	size, err := parseDecimal("position size", position)
	if err != nil {
		return types.PartialCloseStatus{}, err
	}
	size.Abs(size)
	steps := new(big.Rat).Quo(size, spec.step)
	contracts := strings.TrimPrefix(position, "-")
	status := types.PartialCloseStatus{
		Contracts: contracts,
		SizeStep:  spec.SizeStep,
	}
	if !steps.IsInt() || steps.Sign() <= 0 {
		status.Reason = fmt.Sprintf("position size %s is not a positive multiple of size step %s", contracts, spec.SizeStep)
		return status, nil
	}
	if steps.Cmp(big.NewRat(1, 1)) <= 0 {
		status.Reason = fmt.Sprintf("position is a single size step (%s contracts)", spec.SizeStep)
		return status, nil
	}
	status.Supported = true
	return status, nil
}

// ClosePartial 按比例平掉部分持仓：换算张数、提交 reduce-only 市价单并返回成交结果。
func (t *GateTrader) ClosePartial(req types.PartialCloseRequest) (types.PartialCloseResult, error) {
	normalized, err := normalizeSide(req.Side)
	if err != nil {
		return types.PartialCloseResult{}, err
	}
	if math.IsNaN(req.Percent) || math.IsInf(req.Percent, 0) {
		return types.PartialCloseResult{}, fmt.Errorf("close percent is not a finite number: %v", req.Percent)
	}
	if req.Percent == 100 {
		return types.PartialCloseResult{}, fmt.Errorf(
			"close percent 100 is a full close; use the full-close action instead of a partial close")
	}
	if req.Percent <= 0 || req.Percent >= 100 {
		return types.PartialCloseResult{}, fmt.Errorf("close percent must be in (0, 100), got %v", req.Percent)
	}

	symbol := t.convertSymbol(req.Symbol)
	position, err := t.findPositionSize(symbol, normalized)
	if err != nil {
		return types.PartialCloseResult{}, err
	}
	spec, err := t.GetContractSpec(symbol)
	if err != nil {
		return types.PartialCloseResult{}, err
	}
	quote, err := spec.SizeForCloseRatio(position, req.Percent)
	if err != nil {
		return types.PartialCloseResult{}, fmt.Errorf("partial close %s %s skipped: %w", normalized, symbol, err)
	}
	if quote.Full {
		return types.PartialCloseResult{}, fmt.Errorf(
			"close percent %v closes the entire position; use the full-close action instead of a partial close",
			req.Percent)
	}
	if err := spec.validateMarketOrderSize(quote.Contracts); err != nil {
		return types.PartialCloseResult{}, fmt.Errorf("partial close %s %s skipped: %w", normalized, symbol, err)
	}

	// 平多卖出为负、平空买入为正
	size := quote.Contracts
	if normalized == "LONG" {
		size, err = negateDecimal(quote.Contracts)
		if err != nil {
			return types.PartialCloseResult{}, err
		}
	}

	order := gateapi.FuturesOrder{
		Contract:   symbol,
		Size:       size,
		Price:      "0", // 市价单
		Tif:        "ioc",
		ReduceOnly: true,
		Text:       partialCloseText,
	}

	logger.Infof("  [Gate] PartialClose%s: symbol=%s, size=%s, percent=%v, remaining=%s",
		normalized, symbol, size, req.Percent, quote.Remaining)

	result, _, err := t.client.FuturesApi.CreateFuturesOrder(t.ctx, "usdt", order, nil)
	if err != nil {
		return types.PartialCloseResult{}, fmt.Errorf("failed to partially close %s position: %w", normalized, err)
	}
	t.clearCache()

	fillPrice, err := strconv.ParseFloat(result.FillPrice, 64)
	if err != nil {
		return types.PartialCloseResult{}, fmt.Errorf(
			"order %d finished but fill price %q is not parseable: %w", result.Id, result.FillPrice, err)
	}
	if fillPrice <= 0 {
		return types.PartialCloseResult{}, fmt.Errorf(
			"order %d finished but fill price %q is not positive", result.Id, result.FillPrice)
	}
	// 分批平仓要求一次成交完：部分成交时实际平掉的张数与计划不符，不能按计划张数记账
	if !orderFilled(result) {
		return types.PartialCloseResult{}, fmt.Errorf(
			"order %d was not fully filled (finish_as=%q, left=%s); refusing to record the planned contracts",
			result.Id, result.FinishAs, result.Left)
	}

	quantity, err := spec.QuantityFromContracts(quote.Contracts)
	if err != nil {
		return types.PartialCloseResult{}, err
	}

	logger.Infof("  [Gate] Partially closed %s: orderId=%d, contracts=%s, remaining=%s, fillPrice=%.6g",
		normalized, result.Id, quote.Contracts, quote.Remaining, fillPrice)

	return types.PartialCloseResult{
		OrderID:   strconv.FormatInt(result.Id, 10),
		Contracts: quote.Contracts,
		Remaining: quote.Remaining,
		FillPrice: fillPrice,
		Quantity:  quantity,
		Notional:  quantity * fillPrice,
	}, nil
}
