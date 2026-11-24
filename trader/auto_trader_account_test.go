package trader

// 账户读数在交易路径上的单元测试（子任务 account-equity）：本金、可用于合约的额度、权益只经
// types.ParseAccountBalance 得出；初始资金不大于 0 时以当前本金重建并写回，读不到就不加载。

import (
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
	}{
		{name: "exchange read fails", balance: nil, err: assert.AnError},
		{name: "balance unreadable: principal key missing", balance: map[string]interface{}{
			types.BalanceKeyAvailable: 100.0, types.BalanceKeyUnrealizedPnL: 0.0,
		}},
		{name: "principal is zero", balance: canonicalBalance(0, 0, 0)},
		{name: "principal is negative (borrowed exceeds assets)", balance: canonicalBalance(-12.5, 3, 0)},
		{name: "reported equity is positive but principal is not", balance: map[string]interface{}{
			types.BalanceKeyPrincipal:     0.0,
			types.BalanceKeyUnrealizedPnL: 40.0,
			types.BalanceKeyAvailable:     10.0,
			"totalEquity":                 40.0,
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := newTraderStoreTestStore(t)
			stub := &balanceStub{balance: tt.balance, err: tt.err}
			config := &AutoTraderConfig{ID: "trader-1", Name: "t"}

			err := resolveInitialBalance(stub, st, "user-1", config)

			require.Error(t, err, "without a valid initial balance the trader must not load")
			assert.Contains(t, err.Error(), "initial balance 0.00 is not valid")
			assert.Zero(t, config.InitialBalance, "a failed rebuild must not leave a value behind")
			assert.Zero(t, storedInitialBalance(t, st), "a failed rebuild must not write to the database")
		})
	}
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

func TestOpenPositionValueCapUsesEquityFromPrincipal(t *testing.T) {
	// 单仓上限 = 权益 × 比例。Gate 的权益是本金 + 未实现盈亏（桩不报告 totalEquity）：
	// 本金 100 + 未实现盈亏 50 → 上限 150，而不是只按本金或只按可用余额算
	stub := &accountProtectedStub{
		protectedCaptureStub: &protectedCaptureStub{},
		balance:              canonicalBalance(100, 50, 1000),
	}
	at := &AutoTrader{
		exchange: "gate",
		config: AutoTraderConfig{StrategyConfig: &store.StrategyConfig{
			RiskControl: store.RiskControlConfig{BTCETHMaxPositionValueRatio: 1},
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
	assert.InDelta(t, 150.0, stub.requests[0].NotionalUSD, 1e-9)
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
