package trader

// AI 决策的执行路径。开仓必须走随单保护（ProtectedOpener，保护不变量：仓位从存在那一刻起
// 就带有止损与止盈，成交后核对保护单，缺失立即平仓并报错）；不支持随单保护的适配器
// 直接拒绝开仓，不存在无保护的开仓回退路径。取价统一走适配器的 GetMarketPrice（D8/D26），
// 开仓路径不再使用旧内嵌行情路径（⑮ 已删除）。

import (
	"errors"
	"fmt"
	"nofx/kernel"
	"nofx/logger"
	"nofx/market"
	"nofx/store"
	"nofx/trader/types"
	"strconv"
	"strings"
	"time"
)

const (
	// marginOverheadFactor and takerFeeRate approximate the total funds an
	// exchange reserves when opening a position:
	// totalRequired ≈ positionSize/leverage + positionSize*takerFeeRate + positionSize/leverage*1%
	//              = positionSize * (marginOverheadFactor/leverage + takerFeeRate)
	marginOverheadFactor = 1.01
	takerFeeRate         = 0.001

	// positionSizeSafetyFactor leaves a buffer below the maximum affordable
	// position size so a price move between sizing and execution cannot
	// trigger an insufficient-margin rejection.
	positionSizeSafetyFactor = 0.98

	// marketOrderSlipRatio 市价开仓的滑点上限（D8：1%）
	marketOrderSlipRatio = 0.01
)

// protectedOpener 返回随单保护开仓能力；适配器未实现时开仓被拒绝（fail-closed：
// 父分支不得出现"能开仓但没有有效止损"的状态）
func (at *AutoTrader) protectedOpener() (types.ProtectedOpener, error) {
	opener, ok := at.trader.(types.ProtectedOpener)
	if !ok {
		return nil, fmt.Errorf(
			"exchange adapter %s does not support protected open (attached TP/SL); refusing to open unprotected positions",
			at.exchange)
	}
	return opener, nil
}

// partialCloser 返回精确分批平仓能力；适配器未实现时拒绝（fail-closed：不回落全平，
// 也不允许没有精确张数换算的部分平仓）
func (at *AutoTrader) partialCloser() (types.PartialCloser, error) {
	closer, ok := at.trader.(types.PartialCloser)
	if !ok {
		return nil, fmt.Errorf(
			"exchange adapter %s does not support partial close; refusing to reduce the position partially",
			at.exchange)
	}
	return closer, nil
}

// protectiveOrderManager 返回止损止盈管理能力；适配器未实现时拒绝（fail-closed：
// 不能让修改决策落到没有保护单管理能力、无法核对旧单的适配器上）
func (at *AutoTrader) protectiveOrderManager() (types.ProtectiveOrderManager, error) {
	manager, ok := at.trader.(types.ProtectiveOrderManager)
	if !ok {
		return nil, fmt.Errorf(
			"exchange adapter %s does not support protective order management; refusing to modify stop loss or take profit",
			at.exchange)
	}
	return manager, nil
}

// positionSymbolKey 把交易对归一成比较键：大写、去掉 "_"、"-"、"/" 分隔符。
// 交易所原生格式（Gate "DOGE_USDT"）与模型写法（"DOGEUSDT"、"DOGE/USDT"）得到同一个键。
func positionSymbolKey(symbol string) string {
	s := strings.ToUpper(strings.TrimSpace(symbol))
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "/", "")
	return s
}

// positionSymbolBaseKey 去掉比较键末尾的计价币后缀（USDT/USDC/USD）
func positionSymbolBaseKey(symbol string) string {
	key := positionSymbolKey(symbol)
	for _, suffix := range []string{"USDT", "USDC", "USD"} {
		if strings.HasSuffix(key, suffix) && len(key) > len(suffix) {
			return strings.TrimSuffix(key, suffix)
		}
	}
	return key
}

// findOpenPosition 按决策标的找到交易所持仓，返回仓位条目、方向与规范符号。
// 符号变体（大小写、分隔符、只写基础币种）都归一到比较键后再匹配，返回交易所的符号。
func (at *AutoTrader) findOpenPosition(symbol string) (map[string]interface{}, string, string, error) {
	positions, err := at.trader.GetPositions()
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to get positions: %w", err)
	}
	key := positionSymbolKey(symbol)
	base := positionSymbolBaseKey(symbol)
	for _, pos := range positions {
		posSymbol, _ := pos["symbol"].(string)
		if posSymbol != symbol && (key == "" || positionSymbolKey(posSymbol) != key) && (base == "" || positionSymbolBaseKey(posSymbol) != base) {
			continue
		}
		side, _ := pos["side"].(string)
		if side != "long" && side != "short" {
			continue
		}
		return pos, side, posSymbol, nil
	}
	return nil, "", "", fmt.Errorf("%s has no open position; refusing to modify protection", symbol)
}

// executeProtectionUpdateWithRecord 修改止损或止盈：适配器先挂新单并核对，再撤旧单并核对，
// 失败时恢复原保护。保护已无法确认时立即市价全平并报错（保护不变量：不允许无保护持仓）。
func (at *AutoTrader) executeProtectionUpdateWithRecord(decision *kernel.Decision, actionRecord *store.DecisionAction, kind types.ProtectionKind) error {
	logger.Infof("  🛡️ Update protection %v: %s", kind, decision.Symbol)

	manager, err := at.protectiveOrderManager()
	if err != nil {
		return err
	}
	// 方向由动作隐含（update_* 不带方向）；归属断言（D40）：不是本 trader 开出的持仓不改保护
	posSymbol, side, err := at.findOpenPositionOwned(decision.Symbol)
	if err != nil {
		return err
	}

	newPrice := decision.TakeProfit
	if kind == types.ProtectionStopLoss {
		newPrice = decision.StopLoss
	}
	if newPrice <= 0 {
		return fmt.Errorf("new %v price must be positive, got %v", kind, newPrice)
	}

	account, err := types.ReadAccountBalance(at.trader)
	if err != nil {
		return err
	}

	result, err := manager.UpdateProtection(types.ProtectionUpdateRequest{
		Symbol:         posSymbol,
		Side:           side,
		Kind:           kind,
		Price:          newPrice,
		Principal:      account.Principal,
		MaxLossPercent: kernel.MaxLossPercentOfPrincipal,
	})
	if err != nil {
		if errors.Is(err, types.ErrPositionUnprotected) {
			logger.Infof("  🚨 %v", err)
			var closeErr error
			if side == "short" {
				_, closeErr = at.trader.CloseShort(posSymbol, 0)
			} else {
				_, closeErr = at.trader.CloseLong(posSymbol, 0)
			}
			if closeErr != nil {
				return fmt.Errorf("protection update lost protection (%v) and emergency close FAILED: %w", err, closeErr)
			}
			at.ClearPeakPnLCache(posSymbol, side)
			return fmt.Errorf("protection update lost protection, closed immediately: %w", err)
		}
		return err
	}

	recordOrderID(actionRecord, map[string]interface{}{"orderId": result.OrderID})
	actionRecord.Price = result.NewPrice
	if kind == types.ProtectionStopLoss {
		actionRecord.StopLoss = result.NewPrice
	} else {
		actionRecord.TakeProfit = result.NewPrice
	}

	logger.Infof("  ✓ Protection updated: %v %v -> %v, new order %s, replaced %v",
		kind, result.OldPrice, result.NewPrice, result.OrderID, result.ReplacedOrderIDs)
	return nil
}

// executeDecisionWithRecord executes AI decision and records detailed information
func (at *AutoTrader) executeDecisionWithRecord(decision *kernel.Decision, actionRecord *store.DecisionAction) error {
	switch decision.Action {
	case "open_long":
		return at.executeOpenLongWithRecord(decision, actionRecord)
	case "open_short":
		return at.executeOpenShortWithRecord(decision, actionRecord)
	case "close_long":
		return at.executeCloseLongWithRecord(decision, actionRecord)
	case "close_short":
		return at.executeCloseShortWithRecord(decision, actionRecord)
	case "close_long_partial":
		return at.executePartialCloseWithRecord(decision, actionRecord, "LONG")
	case "close_short_partial":
		return at.executePartialCloseWithRecord(decision, actionRecord, "SHORT")
	case "update_stop_loss":
		return at.executeProtectionUpdateWithRecord(decision, actionRecord, types.ProtectionStopLoss)
	case "update_take_profit":
		return at.executeProtectionUpdateWithRecord(decision, actionRecord, types.ProtectionTakeProfit)
	case "hold", "wait":
		// No execution needed, just record
		return nil
	default:
		return fmt.Errorf("unknown action: %s", decision.Action)
	}
}

// prepareOpen 汇集开仓前的仓位、余额与风控检查，返回通过校验的名义价值与账户本金
// （本金交给适配器在提交前按最坏成交价复核亏损上限，D34）。交易判断集中在这里；换算与提交由适配器完成。
func (at *AutoTrader) prepareOpen(decision *kernel.Decision) (float64, float64, error) {
	if at.openBlockReason != "" {
		return 0, 0, fmt.Errorf("opening is blocked this cycle: %s", at.openBlockReason)
	}
	// ⚠️ Get current positions for multiple checks
	positions, err := at.trader.GetPositions()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get positions: %w", err)
	}

	// 只数本 trader 开出的仓位（D40）；同一合约上只要有仓位就不开：
	// 不是本 trader 的仓位 → 归属不了，开进去会与它净额合并（拒绝）；自己的同向仓位 → 先平；
	// 自己的反向仓位 → 单向持仓模式下反向开仓是净额减仓，随单保护的语义没有验证过（拒绝）。
	wantSide := decisionSide(decision.Action)
	ownPositions := 0
	key := positionSymbolKey(decision.Symbol)
	base := positionSymbolBaseKey(decision.Symbol)
	for _, pos := range positions {
		posSymbol, _ := pos["symbol"].(string)
		posSide, _ := pos["side"].(string)
		if !at.isOwnPosition(posSymbol, posSide) {
			continue
		}
		ownPositions++
		if posSymbol != decision.Symbol && (key == "" || positionSymbolKey(posSymbol) != key) &&
			(base == "" || positionSymbolBaseKey(posSymbol) != base) {
			continue
		}
		if posSide == wantSide {
			return 0, 0, fmt.Errorf("❌ %s already has %s position, close it first", decision.Symbol, wantSide)
		}
		return 0, 0, fmt.Errorf("❌ %s has an opposite %s position; close it before opening %s (netting mode)",
			decision.Symbol, posSide, wantSide)
	}
	// 同一合约上存在不是本 trader 开出的仓位：开进去会与它合并，之后无法归属
	for _, pos := range positions {
		posSymbol, _ := pos["symbol"].(string)
		posSide, _ := pos["side"].(string)
		if at.isOwnPosition(posSymbol, posSide) {
			continue
		}
		if posSymbol == decision.Symbol || (key != "" && positionSymbolKey(posSymbol) == key) ||
			(base != "" && positionSymbolBaseKey(posSymbol) == base) {
			return 0, 0, fmt.Errorf("❌ %s has a position not opened by this trader; refusing to open on it (D40)", decision.Symbol)
		}
	}

	// [CODE ENFORCED] Check max positions limit (own positions only, D40)
	if err := at.enforceMaxPositions(ownPositions); err != nil {
		return 0, 0, err
	}

	// Get balance (needed for margin checks)
	account, err := types.ReadAccountBalance(at.trader)
	if err != nil {
		return 0, 0, err
	}
	availableBalance := account.Available

	// The open notional is computed and validated by the engine from principal ×
	// position_percent (D22/D24); the model never supplies an amount. The only
	// lower bound on the order is the exchange's minimum contract size, enforced
	// by the adapter when it rounds the notional down to the size step.
	positionSize := decision.NotionalUSD
	if positionSize <= 0 {
		return 0, 0, fmt.Errorf("decision for %s has no code-computed open notional", decision.Symbol)
	}

	// ⚠️ Auto-adjust position size if insufficient margin
	marginFactor := marginOverheadFactor/float64(decision.Leverage) + takerFeeRate
	maxAffordablePositionSize := availableBalance / marginFactor

	if positionSize > maxAffordablePositionSize {
		adjustedSize := maxAffordablePositionSize * positionSizeSafetyFactor
		logger.Infof("  ⚠️ Position size %.2f exceeds max affordable %.2f, auto-reducing to %.2f",
			positionSize, maxAffordablePositionSize, adjustedSize)
		positionSize = adjustedSize
		decision.NotionalUSD = positionSize
	}

	return positionSize, account.Principal, nil
}

// decisionSide 决策动作对应的持仓方向
func decisionSide(action string) string {
	if action == "open_short" {
		return "short"
	}
	return "long"
}

// executeProtectedOpen 随单保护开仓的共用路径：提交后核对保护单存在，
// 缺失立即市价全平并报错（保护不变量，计划文档第 5 节）
func (at *AutoTrader) executeProtectedOpen(decision *kernel.Decision, actionRecord *store.DecisionAction, side string) error {
	logger.Infof("  📈 Open %s: %s", side, decision.Symbol)

	positionSize, principal, err := at.prepareOpen(decision)
	if err != nil {
		return err
	}

	opener, err := at.protectedOpener()
	if err != nil {
		return err
	}

	// 保证金模式与杠杆随开仓请求交给适配器，在提交订单前一并生效；生效失败适配器返回错误，不开仓
	req := types.ProtectedOpenRequest{
		Symbol:        decision.Symbol,
		NotionalUSD:   positionSize,
		Leverage:      decision.Leverage,
		IsCrossMargin: at.config.IsCrossMargin,
		StopLoss:      decision.StopLoss,
		TakeProfit:    decision.TakeProfit,
		SlipRatio:     marketOrderSlipRatio,
		// 提交前按最坏成交价复核「到止损价的亏损 ≤ 本金 2%」（D34）：决策校验用的是取数时刻的收盘价
		Principal:      principal,
		MaxLossPercent: kernel.MaxLossPercentOfPrincipal,
	}
	var result types.ProtectedOpenResult
	if side == "SHORT" {
		result, err = opener.OpenShortProtected(req)
	} else {
		result, err = opener.OpenLongProtected(req)
	}
	if err != nil {
		// 请求可能超时但订单已成交（或部分成交）：仓位此刻没有核对过保护。
		// prepareOpen 已保证这个合约上原先没有仓位，所以现在出现的仓位一定来自这次下单——立即平掉。
		return at.closePositionOpenedByFailedOpen(decision.Symbol, err)
	}

	// 成交即登记归属：Gate 的持仓记录要等 OrderSync 落库，这段时间模型仍必须看得到、管得了这个仓位
	at.markSelfOpened(decision.Symbol, strings.ToLower(side))

	actionRecord.OrderID, err = strconv.ParseInt(result.OrderID, 10, 64)
	if err != nil {
		return fmt.Errorf("protected open returned non-numeric order id %q: %w", result.OrderID, err)
	}
	actionRecord.Quantity = result.Quantity
	actionRecord.Price = result.FillPrice
	// 记录实际生效的（对齐后的）保护价，而不是模型原始值：它们可能差一个价格步长
	if result.StopLoss > 0 {
		actionRecord.StopLoss = result.StopLoss
	}
	if result.TakeProfit > 0 {
		actionRecord.TakeProfit = result.TakeProfit
	}

	logger.Infof("  ✓ Position opened with attached TP/SL, order ID: %s, contracts: %s, notional: %.4f",
		result.OrderID, result.Contracts, result.Notional)

	// Record order to database and poll for confirmation
	orderResult := map[string]interface{}{"orderId": result.OrderID}
	at.recordAndConfirmOrder(orderResult, decision.Symbol, "open_"+strings.ToLower(side), result.Quantity, result.FillPrice, decision.Leverage, 0)

	// Record position opening time（循环侧的 key 用 GetPositions 的小写方向，保持一致）
	posKey := decision.Symbol + "_" + strings.ToLower(side)
	at.positionFirstSeenTime[posKey] = time.Now().UnixMilli()

	// 保护不变量：成交后核对保护单存在；缺失立即平仓并报错
	if err := opener.VerifyOrderProtection(decision.Symbol, result.OrderID); err != nil {
		logger.Infof("  🚨 %v", err)
		var closeErr error
		if side == "SHORT" {
			_, closeErr = at.trader.CloseShort(decision.Symbol, 0)
		} else {
			_, closeErr = at.trader.CloseLong(decision.Symbol, 0)
		}
		if closeErr != nil {
			// 两条信息都必须保留：核对为什么失败、平仓为什么失败（仓位此刻仍无保护留在交易所）
			return fmt.Errorf("opened without protection (%v) and emergency close FAILED: %w", err, closeErr)
		}
		at.ClearPeakPnLCache(decision.Symbol, strings.ToLower(side))
		return fmt.Errorf("opened without protection, closed immediately: %w", err)
	}

	return nil
}

// recordOrderID 把适配器返回的订单 ID 落库。适配器以字符串返回（Gate 十进制字符串），
// 旧代码的 .(int64) 类型断言对 gate 恒失败、平仓订单 ID 从未落库，这里统一修复
func recordOrderID(actionRecord *store.DecisionAction, orderResult map[string]interface{}) {
	raw, ok := orderResult["orderId"]
	if !ok {
		return
	}
	switch v := raw.(type) {
	case int64:
		actionRecord.OrderID = v
	case string:
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			actionRecord.OrderID = id
		}
	}
}

// executeOpenLongWithRecord executes open long position and records detailed information
func (at *AutoTrader) executeOpenLongWithRecord(decision *kernel.Decision, actionRecord *store.DecisionAction) error {
	return at.executeProtectedOpen(decision, actionRecord, "LONG")
}

// executeOpenShortWithRecord executes open short position and records detailed information
func (at *AutoTrader) executeOpenShortWithRecord(decision *kernel.Decision, actionRecord *store.DecisionAction) error {
	return at.executeProtectedOpen(decision, actionRecord, "SHORT")
}

// executeCloseLongWithRecord executes close long position and records detailed information
func (at *AutoTrader) executeCloseLongWithRecord(decision *kernel.Decision, actionRecord *store.DecisionAction) error {
	logger.Infof("  🔄 Close long: %s", decision.Symbol)

	// 方向与归属断言（D40）：不是本 trader 开出的持仓不平
	if _, err := at.requireOwnOpenPosition(decision.Symbol, "long"); err != nil {
		return err
	}

	// Get current price（适配器自己的最新价，D26）
	price, err := at.trader.GetMarketPrice(decision.Symbol)
	if err != nil {
		return fmt.Errorf("failed to get market price for %s: %w", decision.Symbol, err)
	}
	actionRecord.Price = price

	// Normalize symbol for database lookup
	normalizedSymbol := market.Normalize(decision.Symbol)

	// Get entry price and quantity - prioritize local database for accurate quantity
	var entryPrice float64
	var quantity float64

	// First try to get from local database (more accurate for quantity)
	if at.store != nil {
		if openPos, err := at.store.Position().GetOpenPositionBySymbol(at.id, normalizedSymbol, "LONG"); err == nil && openPos != nil {
			quantity = openPos.Quantity
			entryPrice = openPos.EntryPrice
			logger.Infof("  📊 Using local position data: qty=%.8f, entry=%.2f", quantity, entryPrice)
		}
	}

	// Fallback to exchange API if local data not found
	if quantity == 0 {
		positions, err := at.trader.GetPositions()
		if err == nil {
			for _, pos := range positions {
				if pos["symbol"] == decision.Symbol && pos["side"] == "long" {
					if ep, ok := pos["entryPrice"].(float64); ok {
						entryPrice = ep
					}
					if amt, ok := pos["positionAmt"].(float64); ok && amt > 0 {
						quantity = amt
					}
					break
				}
			}
		}
		logger.Infof("  📊 Using exchange position data: qty=%.8f, entry=%.2f", quantity, entryPrice)
	}

	// Close position
	order, err := at.trader.CloseLong(decision.Symbol, 0) // 0 = close all
	if err != nil {
		return fmt.Errorf("failed to close long position for %s: %w", decision.Symbol, err)
	}

	// Record order ID
	recordOrderID(actionRecord, order)

	// Record order to database and poll for confirmation
	at.recordAndConfirmOrder(order, decision.Symbol, "close_long", quantity, price, 0, entryPrice)
	at.ClearPeakPnLCache(decision.Symbol, "long")

	logger.Infof("  ✓ Position closed successfully")
	return nil
}

// executeCloseShortWithRecord executes close short position and records detailed information
func (at *AutoTrader) executeCloseShortWithRecord(decision *kernel.Decision, actionRecord *store.DecisionAction) error {
	logger.Infof("  🔄 Close short: %s", decision.Symbol)

	// 方向与归属断言（D40）：不是本 trader 开出的持仓不平
	if _, err := at.requireOwnOpenPosition(decision.Symbol, "short"); err != nil {
		return err
	}

	// Get current price（适配器自己的最新价，D26）
	price, err := at.trader.GetMarketPrice(decision.Symbol)
	if err != nil {
		return fmt.Errorf("failed to get market price for %s: %w", decision.Symbol, err)
	}
	actionRecord.Price = price

	// Normalize symbol for database lookup
	normalizedSymbol := market.Normalize(decision.Symbol)

	// Get entry price and quantity - prioritize local database for accurate quantity
	var entryPrice float64
	var quantity float64

	// First try to get from local database (more accurate for quantity)
	if at.store != nil {
		if openPos, err := at.store.Position().GetOpenPositionBySymbol(at.id, normalizedSymbol, "SHORT"); err == nil && openPos != nil {
			quantity = openPos.Quantity
			entryPrice = openPos.EntryPrice
			logger.Infof("  📊 Using local position data: qty=%.8f, entry=%.2f", quantity, entryPrice)
		}
	}

	// Fallback to exchange API if local data not found
	if quantity == 0 {
		positions, err := at.trader.GetPositions()
		if err == nil {
			for _, pos := range positions {
				if pos["symbol"] == decision.Symbol && pos["side"] == "short" {
					if ep, ok := pos["entryPrice"].(float64); ok {
						entryPrice = ep
					}
					if amt, ok := pos["positionAmt"].(float64); ok {
						quantity = -amt // positionAmt is negative for short
					}
					break
				}
			}
		}
		logger.Infof("  📊 Using exchange position data: qty=%.8f, entry=%.2f", quantity, entryPrice)
	}

	// Close position
	order, err := at.trader.CloseShort(decision.Symbol, 0) // 0 = close all
	if err != nil {
		return fmt.Errorf("failed to close short position for %s: %w", decision.Symbol, err)
	}

	// Record order ID
	recordOrderID(actionRecord, order)

	// Record order to database and poll for confirmation
	at.recordAndConfirmOrder(order, decision.Symbol, "close_short", quantity, price, 0, entryPrice)
	at.ClearPeakPnLCache(decision.Symbol, "short")

	logger.Infof("  ✓ Position closed successfully")
	return nil
}

// executePartialCloseWithRecord 按模型给出的比例平掉部分持仓。张数换算与
// reduce-only 市价单由适配器完成；适配器未实现精确分批能力时在发单前拒绝。
func (at *AutoTrader) executePartialCloseWithRecord(decision *kernel.Decision, actionRecord *store.DecisionAction, side string) error {
	logger.Infof("  ✂️ Partial close %s: %s %.1f%%", strings.ToLower(side), decision.Symbol, decision.ClosePercent)

	closer, err := at.partialCloser()
	if err != nil {
		return err
	}

	// 方向与归属断言（D40）：不是本 trader 开出的持仓不分批平
	if _, err := at.requireOwnOpenPosition(decision.Symbol, strings.ToLower(side)); err != nil {
		return err
	}

	result, err := closer.ClosePartial(types.PartialCloseRequest{
		Symbol:  decision.Symbol,
		Side:    side,
		Percent: decision.ClosePercent,
	})
	if err != nil {
		return fmt.Errorf("failed to partially close %s position for %s: %w", strings.ToLower(side), decision.Symbol, err)
	}

	recordOrderID(actionRecord, map[string]interface{}{"orderId": result.OrderID})
	actionRecord.Quantity = result.Quantity
	actionRecord.Price = result.FillPrice

	// Gate 的成交由 OrderSync → PositionBuilder 按实际成交数量缩减仓位；其余实现
	// PartialCloser 的适配器按 close_<side> 记账，数量为本次平掉的部分。
	at.recordAndConfirmOrder(map[string]interface{}{"orderId": result.OrderID}, decision.Symbol,
		"close_"+strings.ToLower(side), result.Quantity, result.FillPrice, 0, 0)

	logger.Infof("  ✓ Partially closed %s %s: contracts=%s remaining=%s fillPrice=%.6g",
		strings.ToLower(side), decision.Symbol, result.Contracts, result.Remaining, result.FillPrice)
	return nil
}
