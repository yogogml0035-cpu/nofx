package gate

import (
	"fmt"
	"math/big"
	"nofx/trader/types"
	"strconv"
	"time"

	"github.com/antihax/optional"
	"github.com/gate/gateapi-go/v7"
)

// balanceCurrency 本金与可用于合约额度的计价币种（钱包总额接口的换算目标、合约的结算币种）
const balanceCurrency = "USDT"

// GetBalance 按统一账户的口径取数（计划文档第 5 节）：
//
//	本金           = 钱包总额 total.amount − total.borrowed（不含未实现盈亏，已实现盈亏计入）
//	未实现盈亏     = 钱包总额 total.unrealised_pnl
//	可用于合约的额度 = 合约账户 available
//
// 合约账户的 total 只适用于经典合约账户，统一账户下恒为 0，不能作钱包余额。
// 权益（本金 + 未实现盈亏）由 types.ParseAccountBalance 得出，这里不单独报告。
func (t *GateTrader) GetBalance() (map[string]interface{}, error) {
	// Check cache
	t.balanceCacheMutex.RLock()
	if t.cachedBalance != nil && time.Since(t.balanceCacheTime) < t.cacheDuration {
		cached := t.cachedBalance
		t.balanceCacheMutex.RUnlock()
		return cached, nil
	}
	t.balanceCacheMutex.RUnlock()

	wallet, _, err := t.client.WalletApi.GetTotalBalance(t.ctx, &gateapi.GetTotalBalanceOpts{
		Currency: optional.NewString(balanceCurrency),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet total balance: %w", err)
	}
	accounts, _, err := t.client.FuturesApi.ListFuturesAccounts(t.ctx, "usdt")
	if err != nil {
		return nil, fmt.Errorf("failed to get futures account: %w", err)
	}

	principal, err := walletPrincipal(&wallet.Total)
	if err != nil {
		return nil, err
	}
	unrealizedPnl, err := parseOptionalRat("total.unrealised_pnl", wallet.Total.UnrealisedPnl)
	if err != nil {
		return nil, err
	}
	available, err := parseOptionalRat("available", accounts.Available)
	if err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		types.BalanceKeyPrincipal:     principal,
		types.BalanceKeyAvailable:     ratFloat(available),
		types.BalanceKeyUnrealizedPnL: ratFloat(unrealizedPnl),
	}

	// Update cache
	t.balanceCacheMutex.Lock()
	t.cachedBalance = result
	t.balanceCacheTime = time.Now()
	t.balanceCacheMutex.Unlock()

	return result, nil
}

// walletPrincipal 本金 = total.amount − total.borrowed，按精确十进制相减后再转为 float64。
// total.amount 必然存在：缺失或解析失败都报错，不按 0 计——统一账户下它是本金的唯一来源。
// total.borrowed 在没有借款时可能不返回，缺省按 0 计（Gate 对数值为 0 的字段不总是返回）。
// 响应的币种带值时必须是请求的币种，否则读数的单位不对。
func walletPrincipal(total *gateapi.AccountBalance) (float64, error) {
	if total.Currency != "" && total.Currency != balanceCurrency {
		return 0, fmt.Errorf("wallet total balance currency is %q, want %q", total.Currency, balanceCurrency)
	}
	amount, err := parseDecimal("total.amount", total.Amount)
	if err != nil {
		return 0, err
	}
	borrowed, err := parseOptionalRat("total.borrowed", total.Borrowed)
	if err != nil {
		return 0, err
	}
	return ratFloat(new(big.Rat).Sub(amount, borrowed)), nil
}

// GetPositions retrieves all open positions
func (t *GateTrader) GetPositions() ([]map[string]interface{}, error) {
	// Check cache
	t.positionsCacheMutex.RLock()
	if t.cachedPositions != nil && time.Since(t.positionsCacheTime) < t.cacheDuration {
		cached := t.cachedPositions
		t.positionsCacheMutex.RUnlock()
		return cached, nil
	}
	t.positionsCacheMutex.RUnlock()

	// Fetch from API
	positions, _, err := t.client.FuturesApi.ListPositions(t.ctx, "usdt", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get positions: %w", err)
	}

	var result []map[string]interface{}
	for _, pos := range positions {
		size, err := parseDecimal("position size", pos.Size)
		if err != nil {
			return nil, fmt.Errorf("failed to parse position %s: %w", pos.Contract, err)
		}
		if size.Sign() == 0 {
			continue // Skip empty positions
		}

		entryPrice, _ := strconv.ParseFloat(pos.EntryPrice, 64)
		markPrice, _ := strconv.ParseFloat(pos.MarkPrice, 64)
		liqPrice, _ := strconv.ParseFloat(pos.LiqPrice, 64)
		unrealizedPnl, _ := strconv.ParseFloat(pos.UnrealisedPnl, 64)
		leverage, _ := strconv.ParseFloat(pos.Leverage, 64)

		// Gate 返回的持仓张数换算为标的币数量：元数据经 GetContractSpec 精确解析，
		// 失败即报错，不再回落 multiplier=1（猜测值）
		spec, err := t.GetContractSpec(pos.Contract)
		if err != nil {
			return nil, fmt.Errorf("failed to get contract info for %s: %w", pos.Contract, err)
		}
		positionAmt := spec.contractQuantity(new(big.Rat).Abs(size))

		// Determine side based on position size
		side := "long"
		if size.Sign() < 0 {
			side = "short"
		}

		result = append(result, map[string]interface{}{
			"symbol":           pos.Contract,
			"positionAmt":      positionAmt,
			"entryPrice":       entryPrice,
			"markPrice":        markPrice,
			"unRealizedProfit": unrealizedPnl,
			"leverage":         int(leverage),
			"liquidationPrice": liqPrice,
			"side":             side,
		})
	}

	// Update cache
	t.positionsCacheMutex.Lock()
	t.cachedPositions = result
	t.positionsCacheTime = time.Now()
	t.positionsCacheMutex.Unlock()

	return result, nil
}

// GetClosedPnL retrieves closed position PnL records
func (t *GateTrader) GetClosedPnL(startTime time.Time, limit int) ([]types.ClosedPnLRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 100 {
		limit = 100
	}

	opts := &gateapi.ListPositionCloseOpts{
		Limit: optional.NewInt32(int32(limit)),
		From:  optional.NewInt64(startTime.Unix()),
	}

	closedPositions, _, err := t.client.FuturesApi.ListPositionClose(t.ctx, "usdt", opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get closed positions: %w", err)
	}

	records := make([]types.ClosedPnLRecord, 0, len(closedPositions))
	for _, pos := range closedPositions {
		pnl, _ := strconv.ParseFloat(pos.Pnl, 64)

		record := types.ClosedPnLRecord{
			Symbol:      t.revertSymbol(pos.Contract),
			Side:        pos.Side,
			RealizedPnL: pnl,
			ExitTime:    time.Unix(int64(pos.Time), 0).UTC(),
			CloseType:   "unknown",
		}

		records = append(records, record)
	}

	return records, nil
}
