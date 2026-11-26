package trader

// 分批平仓交易路径的单元测试：动作路由、适配器能力缺失时 fail-closed、
// 持仓可分批标记与执行顺序。

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nofx/kernel"
	"nofx/store"
	"nofx/trader/types"
)

// partialCloseStub 实现 Trader + PartialCloser 的适配器桩
type partialCloseStub struct {
	types.Trader
	positions     []map[string]interface{}
	statuses      map[string]types.PartialCloseStatus
	statusErrors  map[string]error
	closeRequests []types.PartialCloseRequest
	closeResult   types.PartialCloseResult
	closeError    error
}

func (s *partialCloseStub) GetPositions() ([]map[string]interface{}, error) {
	return s.positions, nil
}

func (s *partialCloseStub) GetBalance() (map[string]interface{}, error) {
	return map[string]interface{}{
		"totalWalletBalance":    100.0,
		"totalUnrealizedProfit": 0.0,
		"availableBalance":      100.0,
	}, nil
}

func (s *partialCloseStub) PartialCloseStatus(symbol, side string) (types.PartialCloseStatus, error) {
	key := symbol + "_" + strings.ToLower(side)
	if err := s.statusErrors[key]; err != nil {
		return types.PartialCloseStatus{}, err
	}
	if status, ok := s.statuses[key]; ok {
		return status, nil
	}
	return types.PartialCloseStatus{Reason: "not configured"}, nil
}

func (s *partialCloseStub) ClosePartial(req types.PartialCloseRequest) (types.PartialCloseResult, error) {
	s.closeRequests = append(s.closeRequests, req)
	return s.closeResult, s.closeError
}

func newPartialCloseAutoTrader(stub *partialCloseStub) *AutoTrader {
	cfg := store.GetDefaultStrategyConfig("en")
	engine := kernel.NewStrategyEngine(&cfg)
	at := &AutoTrader{
		id:                    "test-trader",
		name:                  "partial-close-test",
		exchange:              "gate",
		trader:                stub,
		strategyEngine:        engine,
		initialBalance:        100,
		positionFirstSeenTime: map[string]int64{},
		peakPnLCache:          map[string]float64{},
	}
	// 这些持仓都是本 trader 开出的（D40）：测试夹具没有 DB，用自开登记表达归属
	for _, pos := range stub.positions {
		at.markSelfOpened(pos["symbol"].(string), pos["side"].(string))
	}
	return at
}

func TestExecutePartialCloseRoutesToAdapter(t *testing.T) {
	cases := []struct {
		name     string
		action   string
		wantSide string
		percent  float64
	}{
		{name: "close long partial", action: "close_long_partial", wantSide: "LONG", percent: 50},
		{name: "close short partial", action: "close_short_partial", wantSide: "SHORT", percent: 25},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// 分批平仓前要找到本 trader 开出的同向持仓（D40 的方向与归属断言）
			stub := &partialCloseStub{
				positions: []map[string]interface{}{{"symbol": "DOGE_USDT", "side": strings.ToLower(tc.wantSide)}},
				closeResult: types.PartialCloseResult{
					OrderID: "99123", Contracts: "8", Remaining: "9",
					FillPrice: 0.56, Quantity: 80, Notional: 44.8,
				},
			}
			at := newPartialCloseAutoTrader(stub)
			decision := &kernel.Decision{
				Symbol: "DOGE_USDT", Action: tc.action, ClosePercent: tc.percent,
			}
			actionRecord := &store.DecisionAction{Action: tc.action, Symbol: decision.Symbol}

			require.NoError(t, at.executeDecisionWithRecord(decision, actionRecord))

			require.Len(t, stub.closeRequests, 1)
			assert.Equal(t, tc.wantSide, stub.closeRequests[0].Side)
			assert.Equal(t, tc.percent, stub.closeRequests[0].Percent)
			assert.Equal(t, "DOGE_USDT", stub.closeRequests[0].Symbol)

			assert.Equal(t, int64(99123), actionRecord.OrderID)
			assert.InDelta(t, 80, actionRecord.Quantity, 1e-9)
			assert.InDelta(t, 0.56, actionRecord.Price, 1e-9)
		})
	}
}

func TestPartialCloseRefusedWithoutCapability(t *testing.T) {
	// 与真实适配器的能力边界一致：嵌入 Trader 但不实现 PartialCloser
	stub := &openRefusalStub{}
	at := newPartialCloseAutoTrader(&partialCloseStub{})
	at.trader = stub
	decision := &kernel.Decision{
		Symbol: "DOGE_USDT", Action: "close_long_partial", ClosePercent: 50,
	}

	err := at.executeDecisionWithRecord(decision, &store.DecisionAction{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not support partial close")
	assert.Contains(t, err.Error(), "gate")

	// 直接调用执行函数同样被拒绝，且不产生任何订单记录
	actionRecord := &store.DecisionAction{}
	err = at.executePartialCloseWithRecord(decision, actionRecord, "LONG")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not support partial close")
	assert.Zero(t, actionRecord.OrderID)
}

func TestPartialClosePropagatesAdapterError(t *testing.T) {
	stub := &partialCloseStub{
		positions:  []map[string]interface{}{{"symbol": "DOGE_USDT", "side": "long"}},
		closeError: errors.New("exchange refused"),
	}
	at := newPartialCloseAutoTrader(stub)
	decision := &kernel.Decision{
		Symbol: "DOGE_USDT", Action: "close_long_partial", ClosePercent: 50,
	}
	actionRecord := &store.DecisionAction{}

	err := at.executePartialCloseWithRecord(decision, actionRecord, "LONG")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exchange refused")
	assert.Zero(t, actionRecord.OrderID)
	assert.Zero(t, actionRecord.Quantity)
}

func TestBuildTradingContextMarksPartialClose(t *testing.T) {
	stub := &partialCloseStub{
		positions: []map[string]interface{}{
			{
				"symbol": "DOGE_USDT", "side": "long", "entryPrice": 0.05, "markPrice": 0.055,
				"positionAmt": 170.0, "unRealizedProfit": 0.85, "liquidationPrice": 0.0,
				"leverage": 75.0,
			},
			{
				"symbol": "ETH_USDT", "side": "long", "entryPrice": 3000.0, "markPrice": 3100.0,
				"positionAmt": 0.01, "unRealizedProfit": 1.0, "liquidationPrice": 0.0,
				"leverage": 10.0,
			},
		},
		statuses: map[string]types.PartialCloseStatus{
			"DOGE_USDT_long": {Supported: true, Contracts: "17", SizeStep: "1"},
		},
		statusErrors: map[string]error{
			"ETH_USDT_long": errors.New("contract info unavailable"),
		},
	}
	at := newPartialCloseAutoTrader(stub)

	ctx, err := at.buildTradingContext()
	require.NoError(t, err)
	require.Len(t, ctx.Positions, 2)

	doge := ctx.Positions[0]
	assert.True(t, doge.CanPartialClose)
	assert.Empty(t, doge.PartialCloseUnavailableReason)

	eth := ctx.Positions[1]
	assert.False(t, eth.CanPartialClose)
	assert.Contains(t, eth.PartialCloseUnavailableReason, "status unavailable")
	assert.Contains(t, eth.PartialCloseUnavailableReason, "contract info unavailable")
}

func TestBuildTradingContextMarksUnsupportedAdapter(t *testing.T) {
	// 未实现 PartialCloser 的适配器：即使有持仓也标为不可分批
	at := newPartialCloseAutoTrader(&partialCloseStub{})
	at.trader = &partialCloseNoCapabilityStub{}
	at.markSelfOpened("DOGE_USDT", "long")

	ctx, err := at.buildTradingContext()
	require.NoError(t, err)
	require.Len(t, ctx.Positions, 1)
	assert.False(t, ctx.Positions[0].CanPartialClose)
	assert.Equal(t, "adapter does not support partial close", ctx.Positions[0].PartialCloseUnavailableReason)
}

// partialCloseNoCapabilityStub 有持仓但不实现 PartialCloser
type partialCloseNoCapabilityStub struct {
	types.Trader
}

func (s *partialCloseNoCapabilityStub) GetPositions() ([]map[string]interface{}, error) {
	return []map[string]interface{}{
		{
			"symbol": "DOGE_USDT", "side": "long", "entryPrice": 0.05, "markPrice": 0.055,
			"positionAmt": 170.0, "unRealizedProfit": 0.85, "liquidationPrice": 0.0,
			"leverage": 75.0,
		},
	}, nil
}

func (s *partialCloseNoCapabilityStub) GetBalance() (map[string]interface{}, error) {
	return map[string]interface{}{
		"totalWalletBalance":    100.0,
		"totalUnrealizedProfit": 0.0,
		"availableBalance":      100.0,
	}, nil
}

func TestSortDecisionsPutsPartialCloseBeforeOpen(t *testing.T) {
	decisions := []kernel.Decision{
		{Symbol: "DOGE_USDT", Action: "wait"},
		{Symbol: "DOGE_USDT", Action: "open_long"},
		{Symbol: "DOGE_USDT", Action: "close_short_partial", ClosePercent: 50},
		{Symbol: "DOGE_USDT", Action: "close_long"},
	}

	sorted := sortDecisionsByPriority(decisions)
	require.Len(t, sorted, 4)
	assert.Contains(t, []string{"close_short_partial", "close_long"}, sorted[0].Action)
	assert.Contains(t, []string{"close_short_partial", "close_long"}, sorted[1].Action)
	assert.Equal(t, "open_long", sorted[2].Action)
	assert.Equal(t, "wait", sorted[3].Action)
}
