package trader

// 修改止损止盈交易路径的单元测试：动作路由、字段读取、能力缺失时 fail-closed、
// 持仓不存在拒绝、保护丢失时立即平仓、每轮持仓信息带当前止损止盈。

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

// protectionStub 实现 Trader + ProtectiveOrderManager 的适配器桩
type protectionStub struct {
	types.Trader
	positions        []map[string]interface{}
	protections      map[string]types.PositionProtection
	protectionErrors map[string]error
	updateRequests   []types.ProtectionUpdateRequest
	updateResult     types.ProtectionUpdateResult
	updateError      error
	closedLong       []string
	closedShort      []string
}

func (s *protectionStub) GetPositions() ([]map[string]interface{}, error) {
	return s.positions, nil
}

func (s *protectionStub) GetBalance() (map[string]interface{}, error) {
	return map[string]interface{}{
		"totalWalletBalance":    100.0,
		"totalUnrealizedProfit": 0.0,
		"availableBalance":      100.0,
	}, nil
}

func (s *protectionStub) PositionProtection(symbol, side string) (types.PositionProtection, error) {
	key := symbol + "_" + strings.ToLower(side)
	if err := s.protectionErrors[key]; err != nil {
		return types.PositionProtection{}, err
	}
	if protection, ok := s.protections[key]; ok {
		return protection, nil
	}
	return types.PositionProtection{Reason: "not configured"}, nil
}

func (s *protectionStub) UpdateProtection(req types.ProtectionUpdateRequest) (types.ProtectionUpdateResult, error) {
	s.updateRequests = append(s.updateRequests, req)
	return s.updateResult, s.updateError
}

func (s *protectionStub) CloseLong(symbol string, quantity float64) (map[string]interface{}, error) {
	s.closedLong = append(s.closedLong, symbol)
	return map[string]interface{}{"orderId": "close-1"}, nil
}

func (s *protectionStub) CloseShort(symbol string, quantity float64) (map[string]interface{}, error) {
	s.closedShort = append(s.closedShort, symbol)
	return map[string]interface{}{"orderId": "close-2"}, nil
}

func newProtectionAutoTrader(stub types.Trader) *AutoTrader {
	cfg := store.GetDefaultStrategyConfig("en")
	engine := kernel.NewStrategyEngine(&cfg)
	at := &AutoTrader{
		id:                    "test-trader",
		name:                  "protection-test",
		exchange:              "gate",
		trader:                stub,
		strategyEngine:        engine,
		initialBalance:        100,
		positionFirstSeenTime: map[string]int64{},
		peakPnLCache:          map[string]float64{},
	}
	// 这些持仓都是本 trader 开出的（D40）：测试夹具没有 DB，用自开登记表达归属
	if positions, err := stub.GetPositions(); err == nil {
		for _, pos := range positions {
			at.markSelfOpened(pos["symbol"].(string), pos["side"].(string))
		}
	}
	return at
}

func longPosition() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"symbol": "DOGE_USDT", "side": "long", "entryPrice": 0.05, "markPrice": 0.056,
			"positionAmt": 170.0, "unRealizedProfit": 1.0, "liquidationPrice": 0.0,
			"leverage": 10.0,
		},
	}
}

func TestExecuteUpdateProtectionRoutesToAdapter(t *testing.T) {
	cases := []struct {
		name       string
		action     string
		stopLoss   float64
		takeProfit float64
		wantKind   types.ProtectionKind
		wantPrice  float64
		checkSL    bool
	}{
		{
			name: "update stop loss", action: "update_stop_loss", stopLoss: 0.052,
			wantKind: types.ProtectionStopLoss, wantPrice: 0.052, checkSL: true,
		},
		{
			name: "update take profit", action: "update_take_profit", takeProfit: 0.065,
			wantKind: types.ProtectionTakeProfit, wantPrice: 0.065,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := &protectionStub{
				positions:    longPosition(),
				updateResult: types.ProtectionUpdateResult{OrderID: "700001", Kind: tc.wantKind, OldPrice: 0.048, NewPrice: tc.wantPrice, ReplacedOrderIDs: []string{"551"}},
			}
			at := newProtectionAutoTrader(stub)
			decision := &kernel.Decision{Symbol: "DOGE_USDT", Action: tc.action, StopLoss: tc.stopLoss, TakeProfit: tc.takeProfit}
			actionRecord := &store.DecisionAction{Action: tc.action, Symbol: decision.Symbol}

			require.NoError(t, at.executeDecisionWithRecord(decision, actionRecord))

			require.Len(t, stub.updateRequests, 1)
			req := stub.updateRequests[0]
			assert.Equal(t, "DOGE_USDT", req.Symbol)
			assert.Equal(t, "long", req.Side)
			assert.Equal(t, tc.wantKind, req.Kind)
			assert.InDelta(t, tc.wantPrice, req.Price, 1e-12)
			assert.InDelta(t, 100.0, req.Principal, 1e-9, "本金来自账户读数")
			assert.InDelta(t, kernel.MaxLossPercentOfPrincipal, req.MaxLossPercent, 1e-12)

			assert.Equal(t, int64(700001), actionRecord.OrderID)
			assert.InDelta(t, tc.wantPrice, actionRecord.Price, 1e-12)
			if tc.checkSL {
				assert.InDelta(t, tc.wantPrice, actionRecord.StopLoss, 1e-12)
				assert.Zero(t, actionRecord.TakeProfit)
			} else {
				assert.InDelta(t, tc.wantPrice, actionRecord.TakeProfit, 1e-12)
				assert.Zero(t, actionRecord.StopLoss)
			}
		})
	}
}

func TestUpdateProtectionRefusedWithoutCapability(t *testing.T) {
	// 嵌入 Trader 但不实现 ProtectiveOrderManager
	at := newProtectionAutoTrader(&openRefusalStub{})
	decision := &kernel.Decision{Symbol: "DOGE_USDT", Action: "update_stop_loss", StopLoss: 0.052}

	err := at.executeDecisionWithRecord(decision, &store.DecisionAction{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not support protective order management")
}

func TestUpdateProtectionRefusedWithoutPosition(t *testing.T) {
	stub := &protectionStub{positions: nil}
	at := newProtectionAutoTrader(stub)
	decision := &kernel.Decision{Symbol: "DOGE_USDT", Action: "update_stop_loss", StopLoss: 0.052}

	err := at.executeDecisionWithRecord(decision, &store.DecisionAction{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "has no open position")
	assert.Empty(t, stub.updateRequests, "没有持仓不发改单请求")
}

func TestUpdateProtectionRefusesNonPositivePrice(t *testing.T) {
	stub := &protectionStub{positions: longPosition()}
	at := newProtectionAutoTrader(stub)
	decision := &kernel.Decision{Symbol: "DOGE_USDT", Action: "update_stop_loss"}

	err := at.executeDecisionWithRecord(decision, &store.DecisionAction{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be positive")
	assert.Empty(t, stub.updateRequests)
}

func TestUpdateProtectionClosesWhenProtectionLost(t *testing.T) {
	stub := &protectionStub{
		positions:   longPosition(),
		updateError: errors.Join(types.ErrPositionUnprotected, errors.New("restore failed")),
	}
	at := newProtectionAutoTrader(stub)
	decision := &kernel.Decision{Symbol: "DOGE_USDT", Action: "update_stop_loss", StopLoss: 0.052}

	err := at.executeDecisionWithRecord(decision, &store.DecisionAction{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "closed immediately")
	assert.Equal(t, []string{"DOGE_USDT"}, stub.closedLong, "保护无法确认时立即平仓")
}

func TestUpdateProtectionReportsEmergencyCloseFailure(t *testing.T) {
	stub := &failingCloseStub{protectionStub: protectionStub{
		positions:   longPosition(),
		updateError: types.ErrPositionUnprotected,
	}}
	at := newProtectionAutoTrader(stub)
	decision := &kernel.Decision{Symbol: "DOGE_USDT", Action: "update_stop_loss", StopLoss: 0.052}

	err := at.executeDecisionWithRecord(decision, &store.DecisionAction{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "emergency close FAILED")
}

type failingCloseStub struct {
	protectionStub
}

func (s *failingCloseStub) CloseLong(symbol string, quantity float64) (map[string]interface{}, error) {
	return nil, errors.New("close rejected")
}

func TestBuildTradingContextReadsProtection(t *testing.T) {
	stub := &protectionStub{
		positions: []map[string]interface{}{
			{
				"symbol": "DOGE_USDT", "side": "long", "entryPrice": 0.05, "markPrice": 0.056,
				"positionAmt": 170.0, "unRealizedProfit": 1.0, "liquidationPrice": 0.0, "leverage": 10.0,
			},
			{
				"symbol": "ETH_USDT", "side": "long", "entryPrice": 3000.0, "markPrice": 3100.0,
				"positionAmt": 0.01, "unRealizedProfit": 1.0, "liquidationPrice": 0.0, "leverage": 10.0,
			},
		},
		protections: map[string]types.PositionProtection{
			"DOGE_USDT_long": {StopLoss: 0.048, TakeProfit: 0.065, Known: true},
			"ETH_USDT_long":  {Reason: "no open stop-loss order for this side"},
		},
	}
	at := newProtectionAutoTrader(stub)

	ctx, err := at.buildTradingContext()
	require.NoError(t, err)
	require.Len(t, ctx.Positions, 2)

	doge := ctx.Positions[0]
	assert.InDelta(t, 0.048, doge.StopLoss, 1e-12)
	assert.InDelta(t, 0.065, doge.TakeProfit, 1e-12)
	assert.Empty(t, doge.ProtectionUnavailableReason)

	eth := ctx.Positions[1]
	assert.Zero(t, eth.StopLoss)
	assert.Zero(t, eth.TakeProfit)
	assert.Contains(t, eth.ProtectionUnavailableReason, "no open stop-loss order")
}

// noProtectionManagerStub 有持仓但不实现 ProtectiveOrderManager
type noProtectionManagerStub struct {
	types.Trader
}

func (s *noProtectionManagerStub) GetPositions() ([]map[string]interface{}, error) {
	return longPosition(), nil
}

func (s *noProtectionManagerStub) GetBalance() (map[string]interface{}, error) {
	return map[string]interface{}{
		"totalWalletBalance": 100.0, "totalUnrealizedProfit": 0.0, "availableBalance": 100.0,
	}, nil
}

func TestBuildTradingContextMarksUnsupportedProtectionManager(t *testing.T) {
	at := newProtectionAutoTrader(&noProtectionManagerStub{})
	ctx, err := at.buildTradingContext()
	require.NoError(t, err)
	require.Len(t, ctx.Positions, 1)
	assert.Zero(t, ctx.Positions[0].StopLoss)
	assert.Equal(t, "adapter does not support protective order management", ctx.Positions[0].ProtectionUnavailableReason)
}

func TestBuildTradingContextMarksProtectionReadFailure(t *testing.T) {
	stub := &protectionStub{
		positions:        longPosition(),
		protectionErrors: map[string]error{"DOGE_USDT_long": errors.New("trigger order list unavailable")},
	}
	at := newProtectionAutoTrader(stub)
	ctx, err := at.buildTradingContext()
	require.NoError(t, err)
	require.Len(t, ctx.Positions, 1)
	assert.Contains(t, ctx.Positions[0].ProtectionUnavailableReason, "status unavailable")
	assert.Contains(t, ctx.Positions[0].ProtectionUnavailableReason, "trigger order list unavailable")
}

func TestUpdateProtectionSymbolVariantsReachAdapter(t *testing.T) {
	// 交易所持仓是 Gate 原生 "DOGE_USDT"；模型可能输出 "DOGEUSDT"、"DOGE/USDT" 或只写 "DOGE"。
	// 三者都必须路由到同一持仓，并把交易所符号交给适配器。
	for _, symbol := range []string{"DOGE_USDT", "DOGEUSDT", "DOGE/USDT", "doge_usdt", "DOGE"} {
		t.Run(symbol, func(t *testing.T) {
			stub := &protectionStub{
				positions:    longPosition(),
				updateResult: types.ProtectionUpdateResult{OrderID: "700001", NewPrice: 0.052},
			}
			at := newProtectionAutoTrader(stub)
			decision := &kernel.Decision{Symbol: symbol, Action: "update_stop_loss", StopLoss: 0.052}
			require.NoError(t, at.executeDecisionWithRecord(decision, &store.DecisionAction{}))
			require.Len(t, stub.updateRequests, 1)
			assert.Equal(t, "DOGE_USDT", stub.updateRequests[0].Symbol, "适配器拿到交易所原生符号")
		})
	}
}

func TestSortDecisionsPutsProtectionUpdatesBeforeOpen(t *testing.T) {
	decisions := []kernel.Decision{
		{Symbol: "DOGE_USDT", Action: "wait"},
		{Symbol: "DOGE_USDT", Action: "open_long"},
		{Symbol: "DOGE_USDT", Action: "update_stop_loss", StopLoss: 0.052},
		{Symbol: "DOGE_USDT", Action: "close_long"},
		{Symbol: "DOGE_USDT", Action: "update_take_profit", TakeProfit: 0.065},
	}

	sorted := sortDecisionsByPriority(decisions)
	require.Len(t, sorted, 5)
	assert.Equal(t, "close_long", sorted[0].Action)
	assert.ElementsMatch(t, []string{"update_stop_loss", "update_take_profit"},
		[]string{sorted[1].Action, sorted[2].Action}, "保护修改先于开新仓")
	assert.Equal(t, "open_long", sorted[3].Action)
	assert.Equal(t, "wait", sorted[4].Action)
}
