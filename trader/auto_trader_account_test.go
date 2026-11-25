package trader

// 账户读数在交易路径上的单元测试（子任务 account-equity）：本金、可用于合约的额度、权益只经
// types.ParseAccountBalance 得出；初始资金不大于 0 时以当前本金重建并写回，读不到就不加载。

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nofx/kernel"
	"nofx/store"
	"nofx/trader/types"
)

// balanceStub 返回指定余额映射、没有持仓的适配器桩
type balanceStub struct {
	types.Trader
	balance map[string]interface{}
	err     error
	calls   int
}

func (s *balanceStub) GetBalance() (map[string]interface{}, error) {
	s.calls++
	return s.balance, s.err
}

func (s *balanceStub) GetPositions() ([]map[string]interface{}, error) {
	return nil, nil
}

// accountProtectedStub 在随单保护开仓的桩上换成可配置的余额
type accountProtectedStub struct {
	*protectedCaptureStub
	balance map[string]interface{}
}

func (s *accountProtectedStub) GetBalance() (map[string]interface{}, error) {
	return s.balance, nil
}

func canonicalBalance(principal, unrealized, available float64) map[string]interface{} {
	return map[string]interface{}{
		types.BalanceKeyPrincipal:     principal,
		types.BalanceKeyUnrealizedPnL: unrealized,
		types.BalanceKeyAvailable:     available,
	}
}

// newTraderStoreTestStore 建一个临时 sqlite 库并放入一个初始资金为 0 的 trader
func newTraderStoreTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.New(filepath.Join(t.TempDir(), "account.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	require.NoError(t, st.Trader().Create(&store.Trader{
		ID: "trader-1", UserID: "user-1", Name: "t", AIModelID: "model", ExchangeID: "exchange", InitialBalance: 0,
	}))
	return st
}

func storedInitialBalance(t *testing.T, st *store.Store) float64 {
	t.Helper()
	trader, err := st.Trader().GetByID("trader-1")
	require.NoError(t, err)
	require.NotNil(t, trader)
	return trader.InitialBalance
}

func TestResolveInitialBalanceKeepsValidValue(t *testing.T) {
	// 大于 0 的初始资金（界面或配置里用户给的值）照用，不读交易所、不改数据库
	st := newTraderStoreTestStore(t)
	require.NoError(t, st.Trader().UpdateInitialBalance("user-1", "trader-1", 500))
	stub := &balanceStub{balance: canonicalBalance(107.5, 0, 107.5)}
	config := &AutoTraderConfig{ID: "trader-1", Name: "t", InitialBalance: 500}

	require.NoError(t, resolveInitialBalance(stub, st, "user-1", config))

	assert.Equal(t, 500.0, config.InitialBalance)
	assert.Zero(t, stub.calls, "a valid initial balance must not read the exchange")
	assert.Equal(t, 500.0, storedInitialBalance(t, st))
}

func TestResolveInitialBalanceRebuildsFromPrincipal(t *testing.T) {
	// 存量 trader 的初始资金被记成 0：以当前本金重建、写回数据库。
	// 桩同时报告了权益（205）：初始资金取本金（200），不是权益
	st := newTraderStoreTestStore(t)
	stub := &balanceStub{balance: map[string]interface{}{
		types.BalanceKeyPrincipal:     200.0,
		types.BalanceKeyUnrealizedPnL: 5.0,
		types.BalanceKeyAvailable:     150.0,
		"totalEquity":                 205.0,
	}}
	config := &AutoTraderConfig{ID: "trader-1", Name: "t"}

	require.NoError(t, resolveInitialBalance(stub, st, "user-1", config))

	assert.Equal(t, 200.0, config.InitialBalance)
	assert.Equal(t, 1, stub.calls)
	assert.Equal(t, 200.0, storedInitialBalance(t, st), "the rebuilt value is persisted")
}

func TestResolveInitialBalanceNegativeValueIsRebuilt(t *testing.T) {
	st := newTraderStoreTestStore(t)
	require.NoError(t, st.Trader().UpdateInitialBalance("user-1", "trader-1", -3))
	stub := &balanceStub{balance: canonicalBalance(107.49769421, 0, 107.49769)}
	config := &AutoTraderConfig{ID: "trader-1", Name: "t", InitialBalance: -3}

	require.NoError(t, resolveInitialBalance(stub, st, "user-1", config))

	assert.Equal(t, 107.49769421, config.InitialBalance)
	assert.Equal(t, 107.49769421, storedInitialBalance(t, st))
}

func TestResolveInitialBalanceWithoutStore(t *testing.T) {
	// 没有数据库（st 为 nil）时只重建内存里的值
	stub := &balanceStub{balance: canonicalBalance(300, 0, 300)}
	config := &AutoTraderConfig{ID: "trader-1", Name: "t"}

	require.NoError(t, resolveInitialBalance(stub, nil, "user-1", config))

	assert.Equal(t, 300.0, config.InitialBalance)
}

func TestResolveInitialBalanceFailsClosed(t *testing.T) {
	tests := []struct {
		name    string
		balance map[string]interface{}
		err     error
		wantErr error // 错误链里必须有的类型化错误：上层按它归类失败原因
	}{
		{name: "exchange read fails", balance: nil, err: assert.AnError, wantErr: types.ErrBalanceUnreadable},
		{name: "balance unreadable: principal key missing", balance: map[string]interface{}{
			types.BalanceKeyAvailable: 100.0, types.BalanceKeyUnrealizedPnL: 0.0,
		}, wantErr: types.ErrBalanceUnreadable},
		{name: "principal is zero", balance: canonicalBalance(0, 0, 0), wantErr: types.ErrPrincipalNotPositive},
		{name: "principal is negative (borrowed exceeds assets)", balance: canonicalBalance(-12.5, 3, 0), wantErr: types.ErrPrincipalNotPositive},
		{name: "reported equity is positive but principal is not", balance: map[string]interface{}{
			types.BalanceKeyPrincipal:     0.0,
			types.BalanceKeyUnrealizedPnL: 40.0,
			types.BalanceKeyAvailable:     10.0,
			"totalEquity":                 40.0,
		}, wantErr: types.ErrPrincipalNotPositive},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := newTraderStoreTestStore(t)
			stub := &balanceStub{balance: tt.balance, err: tt.err}
			config := &AutoTraderConfig{ID: "trader-1", Name: "t"}

			err := resolveInitialBalance(stub, st, "user-1", config)

			require.Error(t, err, "without a valid initial balance the trader must not load")
			assert.Contains(t, err.Error(), "initial balance 0.00 is not valid")
			assert.ErrorIs(t, err, tt.wantErr, "the failure reason must be identifiable by type")
			if tt.err != nil {
				assert.ErrorIs(t, err, tt.err, "the exchange error stays in the error chain")
			}
			assert.Zero(t, config.InitialBalance, "a failed rebuild must not leave a value behind")
			assert.Zero(t, storedInitialBalance(t, st), "a failed rebuild must not write to the database")
		})
	}
}

func TestResolveInitialBalanceNaNIsRebuilt(t *testing.T) {
	// NaN 与 0 一样是无效值：不能当成「大于 0 的有效值」照用
	st := newTraderStoreTestStore(t)
	require.NoError(t, st.Trader().UpdateInitialBalance("user-1", "trader-1", 250))
	stub := &balanceStub{balance: canonicalBalance(107.49769421, 0, 107.49769)}
	config := &AutoTraderConfig{ID: "trader-1", Name: "t", InitialBalance: math.NaN()}

	require.NoError(t, resolveInitialBalance(stub, st, "user-1", config))

	assert.Equal(t, 107.49769421, config.InitialBalance, "NaN must be rebuilt from the account principal")
	assert.Equal(t, 107.49769421, storedInitialBalance(t, st))
	assert.Equal(t, 1, stub.calls)
}

func TestResolveInitialBalanceFailureSurvivesWrapping(t *testing.T) {
	// 上层会包上自己的前缀（manager: "failed to create trader: …"；NewAutoTrader: "[名称] …"），
	// 然后才归类失败原因：类型化的错误必须穿过这些包装
	stub := &balanceStub{balance: canonicalBalance(0, 40, 10)}
	config := &AutoTraderConfig{InitialBalance: 0}

	err := resolveInitialBalance(stub, nil, "user-1", config)
	require.Error(t, err)
	wrapped := fmt.Errorf("failed to create trader: [demo] %w", err)

	assert.ErrorIs(t, wrapped, types.ErrPrincipalNotPositive)
	assert.False(t, errors.Is(wrapped, types.ErrBalanceUnreadable), "a readable account is not reported as unreadable")
}

func TestResolveInitialBalanceFailureKeepsInvalidValueInMessage(t *testing.T) {
	stub := &balanceStub{balance: nil, err: assert.AnError}

	err := resolveInitialBalance(stub, nil, "user-1", &AutoTraderConfig{InitialBalance: -3})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "initial balance -3.00 is not valid")
	assert.ErrorIs(t, err, assert.AnError, "the exchange error stays in the error chain")
}

func TestBuildTradingContextUsesParsedAccount(t *testing.T) {
	// Gate 不单独报告权益：权益 = 本金 + 未实现盈亏
	stub := &balanceStub{balance: canonicalBalance(107.49769421, -0.31, 60.25)}
	at := &AutoTrader{
		exchange:              "gate",
		trader:                stub,
		initialBalance:        100,
		startTime:             time.Now(),
		positionFirstSeenTime: map[string]int64{},
		strategyEngine:        kernel.NewStrategyEngine(&store.StrategyConfig{}),
	}

	ctx, err := at.buildTradingContext()
	require.NoError(t, err)

	account := ctx.Account
	assert.InDelta(t, 107.18769421, account.TotalEquity, 1e-9, "equity = principal + unrealized pnl")
	assert.InDelta(t, -0.31, account.UnrealizedPnL, 1e-9)
	assert.InDelta(t, 60.25, account.AvailableBalance, 1e-9)
	assert.InDelta(t, 107.18769421-100, account.TotalPnL, 1e-9, "total pnl = equity − initial balance")
	assert.Equal(t, 0, account.PositionCount)
}

func TestBuildTradingContextRejectsUnreadableBalance(t *testing.T) {
	tests := []struct {
		name    string
		balance map[string]interface{}
	}{
		{name: "empty balance", balance: map[string]interface{}{}},
		{name: "only available balance", balance: map[string]interface{}{types.BalanceKeyAvailable: 100.0}},
		{name: "principal is a string", balance: map[string]interface{}{
			types.BalanceKeyPrincipal: "107.5", types.BalanceKeyUnrealizedPnL: 0.0, types.BalanceKeyAvailable: 1.0,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			at := &AutoTrader{
				exchange:              "gate",
				trader:                &balanceStub{balance: tt.balance},
				strategyEngine:        kernel.NewStrategyEngine(&store.StrategyConfig{}),
				positionFirstSeenTime: map[string]int64{},
			}

			ctx, err := at.buildTradingContext()

			require.Error(t, err)
			assert.Nil(t, ctx)
			assert.Contains(t, err.Error(), "unreadable account balance")
		})
	}
}

func TestBuildTradingContextPropagatesBalanceError(t *testing.T) {
	at := &AutoTrader{
		exchange:       "gate",
		trader:         &balanceStub{err: assert.AnError},
		strategyEngine: kernel.NewStrategyEngine(&store.StrategyConfig{}),
	}

	_, err := at.buildTradingContext()

	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
}

func TestGetAccountInfoUsesParsedAccount(t *testing.T) {
	stub := &balanceStub{balance: canonicalBalance(107.49769421, -0.31, 60.25)}
	at := &AutoTrader{exchange: "gate", trader: stub, initialBalance: 100}

	info, err := at.GetAccountInfo()
	require.NoError(t, err)

	assert.InDelta(t, 107.18769421, info["total_equity"], 1e-9)
	assert.InDelta(t, 107.49769421, info["wallet_balance"], 1e-9, "wallet_balance is the principal")
	assert.InDelta(t, -0.31, info["unrealized_profit"], 1e-9)
	assert.InDelta(t, 60.25, info["available_balance"], 1e-9)
	assert.InDelta(t, 100.0, info["initial_balance"], 1e-9)
}

func TestGetAccountInfoRejectsUnreadableBalance(t *testing.T) {
	at := &AutoTrader{exchange: "gate", trader: &balanceStub{balance: map[string]interface{}{}}}

	info, err := at.GetAccountInfo()

	require.Error(t, err)
	assert.Nil(t, info)
	assert.Contains(t, err.Error(), "unreadable")
}

func TestOpenNotionalIsNoLongerCappedByEquityRatio(t *testing.T) {
	// 权益×倍数的单仓上限随倍数字段整体删除（⑧ strategy-config）：决策金额不再被
	// 权益封顶，只剩可用保证金自动缩减与最小下单门槛。本金 100 + 浮盈 50（权益 150）
	// 时 500 USDT 的决策金额原样进入下单请求。
	stub := &accountProtectedStub{
		protectedCaptureStub: &protectedCaptureStub{},
		balance:              canonicalBalance(100, 50, 1000),
	}
	at := &AutoTrader{
		exchange: "gate",
		config: AutoTraderConfig{StrategyConfig: &store.StrategyConfig{
			RiskControl: store.RiskControlConfig{PositionPercent: 10, MinPositionSize: 10},
		}},
		trader:                stub,
		positionFirstSeenTime: map[string]int64{},
	}
	decision := &kernel.Decision{
		Action: "open_long", Symbol: "BTCUSDT", Leverage: 10,
		PositionSizeUSD: 500, StopLoss: 60000, TakeProfit: 70000,
	}

	require.NoError(t, at.executeOpenLongWithRecord(decision, &store.DecisionAction{}))

	require.Len(t, stub.requests, 1)
	assert.InDelta(t, 500.0, stub.requests[0].NotionalUSD, 1e-9)
}

func TestOpenRefusedWhenBalanceIsUnreadable(t *testing.T) {
	// 读数缺本金时不开仓：不再有「权益退回可用余额」的回退
	stub := &accountProtectedStub{
		protectedCaptureStub: &protectedCaptureStub{},
		balance:              map[string]interface{}{types.BalanceKeyAvailable: 1000.0},
	}
	at := &AutoTrader{
		exchange:              "gate",
		config:                AutoTraderConfig{StrategyConfig: &store.StrategyConfig{}},
		trader:                stub,
		positionFirstSeenTime: map[string]int64{},
	}
	decision := &kernel.Decision{
		Action: "open_long", Symbol: "BTCUSDT", Leverage: 10,
		PositionSizeUSD: 15, StopLoss: 60000, TakeProfit: 70000,
	}
	actionRecord := &store.DecisionAction{}

	err := at.executeOpenLongWithRecord(decision, actionRecord)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unreadable")
	assert.Empty(t, stub.requests, "no open request may reach the adapter")
	assert.Zero(t, actionRecord.OrderID)
}

// mutableBalanceStub 余额可以在用例中途改掉的适配器桩（回撤检查要跨多次读数）
type mutableBalanceStub struct {
	types.Trader
	balance map[string]interface{}
	calls   int
}

func (s *mutableBalanceStub) GetBalance() (map[string]interface{}, error) {
	s.calls++
	return s.balance, nil
}

func (s *mutableBalanceStub) GetPositions() ([]map[string]interface{}, error) {
	return nil, nil
}

func newGridTrader(stub types.Trader, maxDrawdownPct float64) *AutoTrader {
	return &AutoTrader{
		exchange: "gate",
		trader:   stub,
		config: AutoTraderConfig{StrategyConfig: &store.StrategyConfig{
			StrategyType: "grid_trading",
			GridConfig:   &store.GridStrategyConfig{Symbol: "ETHUSDT", MaxDrawdownPct: maxDrawdownPct},
		}},
		gridState: &GridState{},
	}
}

func TestGridDrawdownUsesEquityFromPrincipalPlusUnrealized(t *testing.T) {
	// 回撤 =（峰值权益 − 当前权益）/ 峰值权益。权益 = 本金 + 未实现盈亏：
	// 浮盈推高权益记为峰值，浮盈消失后权益回到本金
	stub := &mutableBalanceStub{balance: canonicalBalance(100, 50, 80)}
	at := newGridTrader(stub, 10)

	exceeded, drawdown := at.checkMaxDrawdown()
	assert.False(t, exceeded)
	assert.InDelta(t, 0, drawdown, 1e-9)
	assert.InDelta(t, 150.0, at.gridState.PeakEquity, 1e-9, "peak equity = principal + unrealized pnl")

	// 浮盈消失：权益从 150 回到 100，回撤 = 50 / 150 = 33%，超过 10% 的上限。
	// 只按本金算的话两次读数都是 100，不会触发——这条用例专门区分「权益含浮盈」
	stub.balance = canonicalBalance(100, 0, 80)
	exceeded, drawdown = at.checkMaxDrawdown()
	assert.True(t, exceeded, "the drawdown limit must trigger once equity has fallen far enough below its peak")
	assert.InDelta(t, 50.0/150.0*100, drawdown, 1e-9)
	assert.InDelta(t, 50.0/150.0*100, at.gridState.MaxDrawdown, 1e-9)
	assert.InDelta(t, 150.0, at.gridState.PeakEquity, 1e-9, "the peak is kept")
}

func TestGridDrawdownNotTriggeredNearPeak(t *testing.T) {
	stub := &mutableBalanceStub{balance: canonicalBalance(100, 50, 80)}
	at := newGridTrader(stub, 10)

	at.checkMaxDrawdown()
	stub.balance = canonicalBalance(100, 45, 80) // 回撤 3.3%
	exceeded, drawdown := at.checkMaxDrawdown()

	assert.False(t, exceeded)
	assert.InDelta(t, 5.0/150.0*100, drawdown, 1e-9)
	assert.InDelta(t, 150.0, at.gridState.PeakEquity, 1e-9, "the peak is kept")
}

func TestGridDrawdownSkippedWhenBalanceUnreadable(t *testing.T) {
	// 读不到账户读数时不知道回撤是多少：不触发，也不改动峰值（告警日志见实现）
	stub := &mutableBalanceStub{balance: map[string]interface{}{types.BalanceKeyAvailable: 100.0}}
	at := newGridTrader(stub, 10)
	at.gridState.PeakEquity = 120

	exceeded, drawdown := at.checkMaxDrawdown()

	assert.False(t, exceeded, "an unreadable account balance must not trigger a drawdown close")
	assert.Zero(t, drawdown)
	assert.InDelta(t, 120.0, at.gridState.PeakEquity, 1e-9, "the peak is not touched")
	assert.Equal(t, 1, stub.calls)
}

func TestGridDrawdownDisabledDoesNotReadAccount(t *testing.T) {
	stub := &mutableBalanceStub{balance: canonicalBalance(100, -1000, 80)}
	at := newGridTrader(stub, 0)

	exceeded, drawdown := at.checkMaxDrawdown()

	assert.False(t, exceeded)
	assert.Zero(t, drawdown)
	assert.Zero(t, stub.calls, "the drawdown check is off: the account is not read")
}

func TestApplyGridAccountUsesParsedBalance(t *testing.T) {
	stub := &mutableBalanceStub{balance: canonicalBalance(100, 10, 60)}
	at := newGridTrader(stub, 0)
	ctx := &kernel.GridContext{}

	at.applyGridAccount(ctx)

	assert.InDelta(t, 110.0, ctx.TotalEquity, 1e-9, "equity = principal + unrealized pnl")
	assert.InDelta(t, 60.0, ctx.AvailableBalance, 1e-9)
	assert.InDelta(t, 10.0, ctx.UnrealizedPnL, 1e-9)
}

func TestApplyGridAccountKeepsFieldsWhenUnreadable(t *testing.T) {
	// 网格上下文的失败不阻断：读不到时保持原样
	stub := &mutableBalanceStub{balance: map[string]interface{}{types.BalanceKeyAvailable: 60.0}}
	at := newGridTrader(stub, 0)
	ctx := &kernel.GridContext{TotalEquity: 5, AvailableBalance: 6, UnrealizedPnL: 7}

	at.applyGridAccount(ctx)

	assert.InDelta(t, 5.0, ctx.TotalEquity, 1e-9)
	assert.InDelta(t, 6.0, ctx.AvailableBalance, 1e-9)
	assert.InDelta(t, 7.0, ctx.UnrealizedPnL, 1e-9)
}
