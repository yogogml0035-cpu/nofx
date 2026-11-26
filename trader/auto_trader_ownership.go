package trader

// 仓位归属（计划文档 D40）：不是本 trader 开出的持仓对模型不可见、不管理、不平仓，
// 也不因此拒绝 trader 启动。归属判定见 auto_trader_risk.go 的 isOwnPosition；这里维护
// 「本进程刚开出、OrderSync 还没把持仓落到 DB」这段时间的归属（Gate 的持仓记录由 OrderSync
// → PositionBuilder 落库，决策路径不直接写持仓）。仓位从交易所消失后记录即清掉。

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"nofx/kernel"
	"nofx/logger"
	"nofx/trader/types"
)

// ownPositionKey 归属记录的键：归一化交易对 + 小写方向（交易所原生符号与模型写法得到同一个键）
func ownPositionKey(symbol, side string) string {
	return positionSymbolKey(symbol) + "_" + strings.ToLower(side)
}

// markSelfOpened 在开仓成交后登记仓位归属，直到 DB 里有它的 OPEN 记录或仓位消失
func (at *AutoTrader) markSelfOpened(symbol, side string) {
	key := ownPositionKey(symbol, side)
	at.selfOpenedMutex.Lock()
	if at.selfOpenedPositions == nil {
		at.selfOpenedPositions = make(map[string]bool)
	}
	at.selfOpenedPositions[key] = true
	at.selfOpenedMutex.Unlock()
}

// isSelfOpened 仓位是否由本进程刚开出
func (at *AutoTrader) isSelfOpened(symbol, side string) bool {
	key := ownPositionKey(symbol, side)
	at.selfOpenedMutex.RLock()
	defer at.selfOpenedMutex.RUnlock()
	return at.selfOpenedPositions[key]
}

// forgetClosedSelfOpened 清掉交易所里已经不存在的自开仓位记录；stillOpen 用归一化键判断仓位是否还在
func (at *AutoTrader) forgetClosedSelfOpened(stillOpen func(ownPositionKey string) bool) {
	at.selfOpenedMutex.Lock()
	for key := range at.selfOpenedPositions {
		if !stillOpen(key) {
			delete(at.selfOpenedPositions, key)
		}
	}
	at.selfOpenedMutex.Unlock()
}

// requireOwnOpenPosition 按决策标的找到交易所持仓，校验方向与归属（D40），返回交易所的符号。
// wantSide 是交易所返回的小写方向（long / short）。
func (at *AutoTrader) requireOwnOpenPosition(symbol, wantSide string) (string, error) {
	_, side, posSymbol, err := at.findOpenPosition(symbol)
	if err != nil {
		return "", err
	}
	if side != wantSide {
		return "", fmt.Errorf("%s has a %s position, not %s; refusing to touch it", symbol, side, wantSide)
	}
	if err := at.requireOwnPosition(posSymbol, side); err != nil {
		return "", err
	}
	return posSymbol, nil
}

// requireOwnPosition 执行平仓、分批平仓、改保护前的归属断言：不是本 trader 开出的持仓不碰（D40）。
// side 是交易所返回的小写方向。
func (at *AutoTrader) requireOwnPosition(symbol, side string) error {
	if at.isOwnPosition(symbol, side) {
		return nil
	}
	return fmt.Errorf("%s %s position was not opened by this trader; refusing to touch it (D40)", strings.ToUpper(side), symbol)
}

// findOpenPositionOwned 按决策标的找到交易所持仓并校验归属（D40），返回交易所的符号与小写方向。
// 用于动作本身不带方向的情况（修改止损止盈）。
func (at *AutoTrader) findOpenPositionOwned(symbol string) (string, string, error) {
	_, side, posSymbol, err := at.findOpenPosition(symbol)
	if err != nil {
		return "", "", err
	}
	if err := at.requireOwnPosition(posSymbol, side); err != nil {
		return "", "", err
	}
	return posSymbol, side, nil
}

// enforceProtectionInvariant 保护不变量的每轮兜底：本 trader 的持仓在交易所上确认没有止损
// → 立即市价全平。保护丢失可能来自用户在交易所手动撤单、止损触发后市价单没有成交完，
// 或同账户的其他操作撤掉了条件单；改保护路径此时会拒绝修改（止损读不到），补挂路径也不存在，
// 所以这里必须确定性地处置，而不是靠模型自愿平仓。
//
// 只处理 ProtectionConfirmed 的仓位：保护读不到（接口失败）时缺失只是「不知道」，不得据此平仓。
// 平仓是破坏性动作，不靠单次读数：平仓前再读一次，第二次读到了止损或读失败就不平。
// 平仓失败不中止本轮（其余仓位照常交给模型管理），但本轮禁止开仓（openBlockReason）；
// 平仓时发现持仓已不存在（刚被止损/止盈单平掉）按已解决处理。
//
// 返回剩余仓位、本轮平掉的仓位数与失败的处置。
func (at *AutoTrader) enforceProtectionInvariant(positions []kernel.PositionInfo) (remaining []kernel.PositionInfo, closed int, failures []error) {
	manager, hasManager := at.trader.(types.ProtectiveOrderManager)
	remaining = make([]kernel.PositionInfo, 0, len(positions))
	for _, position := range positions {
		if !position.ProtectionConfirmed || position.StopLoss > 0 {
			remaining = append(remaining, position)
			continue
		}
		confirmed := true
		if hasManager {
			confirmed = false
			if protection, err := manager.PositionProtection(position.Symbol, position.Side); err == nil {
				confirmed = protection.StopLoss <= 0
			}
		}
		if !confirmed {
			logger.Warnf("⚠️ [%s] %s %s looked unprotected but the second read did not confirm it; leaving it to the model",
				at.name, strings.ToUpper(position.Side), position.Symbol)
			remaining = append(remaining, position)
			continue
		}
		logger.Errorf("🚨 [%s] %s %s has no stop loss on the exchange (take profit %v); closing it immediately (protection invariant)",
			at.name, strings.ToUpper(position.Side), position.Symbol, position.TakeProfit)
		if err := at.emergencyClosePosition(position.Symbol, position.Side); err != nil {
			if errors.Is(err, types.ErrPositionNotFound) {
				// 两次读数之间仓位已被止损/止盈单平掉：问题已解决
				logger.Infof("  ✅ [%s] %s %s was already closed on the exchange", at.name, strings.ToUpper(position.Side), position.Symbol)
				closed++
				continue
			}
			logger.Errorf("🚨 [%s] %s", at.name, err)
			failures = append(failures, fmt.Errorf("%s %s lost its stop loss and the emergency close FAILED: %w",
				strings.ToUpper(position.Side), position.Symbol, err))
			remaining = append(remaining, position)
			continue
		}
		closed++
	}
	return remaining, closed, failures
}

// noteRejectedDecisions 记录连续出现被拒决策的轮数：模型每轮都输出违规决策时，只靠 Warn 日志看不出来
// （有任意一条通过就算本轮成功），连续 3 轮升格为 Error。
func (at *AutoTrader) noteRejectedDecisions(rejected []kernel.RejectedDecision) {
	if len(rejected) == 0 {
		at.consecutiveRejectedRounds = 0
		return
	}
	at.consecutiveRejectedRounds++
	if at.consecutiveRejectedRounds >= 3 {
		actions := make([]string, 0, len(rejected))
		for _, r := range rejected {
			actions = append(actions, fmt.Sprintf("#%d %s %s: %s", r.Index, r.Action, r.Symbol, r.Reason))
		}
		logger.Errorf("🚨 [%s] Decisions rejected for %d consecutive rounds; the latest: %s",
			at.name, at.consecutiveRejectedRounds, strings.Join(actions, " | "))
	}
}

// closePositionOpenedByFailedOpen 开仓请求报错后的兜底：若交易所上出现了这个合约的仓位
// （请求超时但已成交、或部分成交后报错），立即平掉，避免留下没核对过保护的仓位。
// 成交后持仓读数可能有延迟，所以每次读数前清掉持仓缓存并最多读 3 次（间隔 300ms）。
// 原始错误与平仓错误都保留在返回值里。
func (at *AutoTrader) closePositionOpenedByFailedOpen(symbol string, openErr error) error {
	key := positionSymbolKey(symbol)
	base := positionSymbolBaseKey(symbol)
	refresher, canRefresh := at.trader.(interface{ ClearPositionsCache() })
	for attempt := 1; attempt <= 3; attempt++ {
		if canRefresh {
			refresher.ClearPositionsCache()
		}
		positions, err := at.trader.GetPositions()
		if err != nil {
			return fmt.Errorf("protected open failed (%v) and the position readback failed: %w", openErr, err)
		}
		for _, pos := range positions {
			posSymbol, _ := pos["symbol"].(string)
			posSide, _ := pos["side"].(string)
			if posSymbol != symbol && (key == "" || positionSymbolKey(posSymbol) != key) &&
				(base == "" || positionSymbolBaseKey(posSymbol) != base) {
				continue
			}
			logger.Errorf("🚨 [%s] protected open failed and %s %s now has a position; closing it (unverified protection)",
				at.name, posSide, posSymbol)
			var closeErr error
			if posSide == "short" {
				_, closeErr = at.trader.CloseShort(posSymbol, 0)
			} else {
				_, closeErr = at.trader.CloseLong(posSymbol, 0)
			}
			if closeErr != nil {
				return fmt.Errorf("protected open failed (%v), the emergency close FAILED: %w", openErr, closeErr)
			}
			return fmt.Errorf("protected open failed; the position that appeared was closed immediately: %w", openErr)
		}
		if attempt < 3 {
			time.Sleep(300 * time.Millisecond)
		}
	}
	return openErr
}
