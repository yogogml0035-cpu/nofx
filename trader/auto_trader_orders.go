package trader

// AI 决策的执行路径。开仓必须走随单保护（ProtectedOpener，保护不变量：仓位从存在那一刻起
// 就带有止损与止盈，成交后核对保护单，缺失立即平仓并报错）；不支持随单保护的适配器
// 直接拒绝开仓，不存在无保护的开仓回退路径。取价统一走适配器的 GetMarketPrice（D8/D26），
// 开仓路径不再使用 market.GetWithExchange（CoinAnk 通道）。

import (
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

// minOrderNotional 策略配置的最小下单金额（USDT），与 enforceMinPositionSize 同源同默认（D19：默认 10）
func (at *AutoTrader) minOrderNotional() float64 {
	if at.config.StrategyConfig == nil {
		return 10
	}
	minSize := at.config.StrategyConfig.RiskControl.MinPositionSize
	if minSize <= 0 {
		return 10
	}
	return minSize
}

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
	case "hold", "wait":
		// No execution needed, just record
		return nil
	default:
		return fmt.Errorf("unknown action: %s", decision.Action)
	}
}

// prepareOpen 汇集开仓前的仓位、余额与风控检查，返回通过校验的名义价值。
// 交易判断集中在这里；换算与提交由适配器完成。
func (at *AutoTrader) prepareOpen(decision *kernel.Decision) (float64, error) {
	// ⚠️ Get current positions for multiple checks
	positions, err := at.trader.GetPositions()
	if err != nil {
		return 0, fmt.Errorf("failed to get positions: %w", err)
	}

	// [CODE ENFORCED] Check max positions limit
	if err := at.enforceMaxPositions(len(positions)); err != nil {
		return 0, err
	}

	// Check if there's already a position in the same symbol and direction
	for _, pos := range positions {
		if pos["symbol"] == decision.Symbol && pos["side"] == decisionSide(decision.Action) {
			return 0, fmt.Errorf("❌ %s already has %s position, close it first", decision.Symbol, decisionSide(decision.Action))
		}
	}

	// Get balance (needed for margin checks)
	account, err := types.ReadAccountBalance(at.trader)
	if err != nil {
		return 0, err
	}
	availableBalance := account.Available

	// The open notional is computed and validated by the engine from principal ×
	// position_percent (D22/D24); the model never supplies an amount.
	positionSize := decision.NotionalUSD
	if positionSize <= 0 {
		return 0, fmt.Errorf("decision for %s has no code-computed open notional", decision.Symbol)
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

	// [CODE ENFORCED] Minimum position size check
	if err := at.enforceMinPositionSize(positionSize); err != nil {
		return 0, err
	}
	return positionSize, nil
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

	positionSize, err := at.prepareOpen(decision)
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
		MinNotional:   at.minOrderNotional(),
		SlipRatio:     marketOrderSlipRatio,
	}
	var result types.ProtectedOpenResult
	if side == "SHORT" {
		result, err = opener.OpenShortProtected(req)
	} else {
		result, err = opener.OpenLongProtected(req)
	}
	if err != nil {
		return err
	}

	actionRecord.OrderID, err = strconv.ParseInt(result.OrderID, 10, 64)
	if err != nil {
		return fmt.Errorf("protected open returned non-numeric order id %q: %w", result.OrderID, err)
	}
	actionRecord.Quantity = result.Quantity
	actionRecord.Price = result.FillPrice

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

	logger.Infof("  ✓ Position closed successfully")
	return nil
}

// executeCloseShortWithRecord executes close short position and records detailed information
func (at *AutoTrader) executeCloseShortWithRecord(decision *kernel.Decision, actionRecord *store.DecisionAction) error {
	logger.Infof("  🔄 Close short: %s", decision.Symbol)

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

	logger.Infof("  ✓ Position closed successfully")
	return nil
}
