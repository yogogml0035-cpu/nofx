package trader

// 仓位归属（D40）与保护不变量每轮兜底的单元测试。

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nofx/kernel"
	"nofx/store"
	"nofx/trader/types"
)

// ownershipStub 实现了分批平仓与保护单管理能力：这样用例里唯一会拒绝动作的理由只剩「归属」
type ownershipStub struct {
	types.Trader
	positions     []map[string]interface{}
	closeLongs    []string
	closeShorts   []string
	partialCloses []types.PartialCloseRequest
	updates       []types.ProtectionUpdateRequest
	closeErr      error
	// protection / protectionErr：PositionProtection 的返回（nil 表示默认的「止损止盈都在」）
	protection    *types.PositionProtection
	protectionErr error
}

func (s *ownershipStub) PartialCloseStatus(symbol, side string) (types.PartialCloseStatus, error) {
	return types.PartialCloseStatus{Supported: true}, nil
}

func (s *ownershipStub) ClosePartial(req types.PartialCloseRequest) (types.PartialCloseResult, error) {
	s.partialCloses = append(s.partialCloses, req)
	return types.PartialCloseResult{}, nil
}

func (s *ownershipStub) PositionProtection(symbol, side string) (types.PositionProtection, error) {
	if s.protectionErr != nil {
		return types.PositionProtection{}, s.protectionErr
	}
	if s.protection != nil {
		return *s.protection, nil
	}
	return types.PositionProtection{StopLoss: 0.04, TakeProfit: 0.07, Known: true}, nil
}

func (s *ownershipStub) UpdateProtection(req types.ProtectionUpdateRequest) (types.ProtectionUpdateResult, error) {
	s.updates = append(s.updates, req)
	return types.ProtectionUpdateResult{}, nil
}

func (s *ownershipStub) GetPositions() ([]map[string]interface{}, error) {
	return s.positions, nil
}

func (s *ownershipStub) GetBalance() (map[string]interface{}, error) {
	return map[string]interface{}{
		"totalWalletBalance": 1000.0, "totalUnrealizedProfit": 0.0, "availableBalance": 1000.0,
	}, nil
}

func (s *ownershipStub) CloseLong(symbol string, quantity float64) (map[string]interface{}, error) {
	s.closeLongs = append(s.closeLongs, symbol)
	return map[string]interface{}{"orderId": "1"}, s.closeErr
}

func (s *ownershipStub) CloseShort(symbol string, quantity float64) (map[string]interface{}, error) {
	s.closeShorts = append(s.closeShorts, symbol)
	return map[string]interface{}{"orderId": "2"}, s.closeErr
}

func ownPosition(symbol, side string, amount float64) map[string]interface{} {
	return map[string]interface{}{
		"symbol": symbol, "side": side, "entryPrice": 0.05, "markPrice": 0.055,
		"positionAmt": amount, "unRealizedProfit": 0.5, "liquidationPrice": 0.0, "leverage": 10.0,
	}
}

func newOwnershipTrader(stub *ownershipStub) *AutoTrader {
	cfg := store.GetDefaultStrategyConfig("en")
	return &AutoTrader{
		id: "test-trader", name: "ownership-test", exchange: "gate", trader: stub,
		strategyEngine: kernel.NewStrategyEngine(&cfg), initialBalance: 1000,
		positionFirstSeenTime: map[string]int64{}, peakPnLCache: map[string]float64{},
	}
}

// 模型只能看到本 trader 开出的仓位（D40）
func TestBuildTradingContextHidesForeignPositions(t *testing.T) {
	stub := &ownershipStub{positions: []map[string]interface{}{
		ownPosition("DOGE_USDT", "long", 17),
		ownPosition("BTC_USDT", "short", 1), // 用户手动开的，不是本 trader 的
	}}
	at := newOwnershipTrader(stub)
	at.markSelfOpened("DOGE_USDT", "long")

	ctx, err := at.buildTradingContext()
	require.NoError(t, err)
	require.Len(t, ctx.Positions, 1)
	assert.Equal(t, "DOGE_USDT", ctx.Positions[0].Symbol)
	assert.Equal(t, 1, ctx.Account.PositionCount)
}

func TestCloseRefusesForeignPosition(t *testing.T) {
	for _, tc := range []struct {
		name   string
		action string
		side   string
	}{
		{"close_long", "close_long", "long"},
		{"close_short", "close_short", "short"},
		{"close_long_partial", "close_long_partial", "long"},
		{"update_stop_loss", "update_stop_loss", "long"},
		{"update_take_profit", "update_take_profit", "short"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &ownershipStub{positions: []map[string]interface{}{ownPosition("DOGE_USDT", tc.side, 17)}}
			at := newOwnershipTrader(stub)
			decision := &kernel.Decision{Symbol: "DOGE_USDT", Action: tc.action, StopLoss: 0.04, TakeProfit: 0.07, ClosePercent: 50}
			actionRecord := &store.DecisionAction{Action: tc.action}

			err := at.executeDecisionWithRecord(decision, actionRecord)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "not opened by this trader")
			assert.Empty(t, stub.closeLongs, "foreign positions must not be closed")
			assert.Empty(t, stub.closeShorts, "foreign positions must not be closed")
			assert.Empty(t, stub.partialCloses, "foreign positions must not be reduced")
			assert.Empty(t, stub.updates, "protection of foreign positions must not be modified")
		})
	}
}

// 自开登记补上「成交 → OrderSync 落库」之间的归属空窗：没有 DB 记录时也认得出自己的仓位
func TestSelfOpenedPositionIsManageable(t *testing.T) {
	stub := &ownershipStub{positions: []map[string]interface{}{ownPosition("DOGE_USDT", "long", 17)}}
	at := newOwnershipTrader(stub)
	at.markSelfOpened("DOGE_USDT", "long")

	posSymbol, err := at.requireOwnOpenPosition("DOGEUSDT", "long")
	require.NoError(t, err)
	assert.Equal(t, "DOGE_USDT", posSymbol)

	// 仓位从交易所消失后登记被清掉
	at.forgetClosedSelfOpened(func(key string) bool { return false })
	assert.False(t, at.isSelfOpened("DOGE_USDT", "long"))
}

func TestPrepareOpenOwnershipRules(t *testing.T) {
	decision := func(action string) *kernel.Decision {
		return &kernel.Decision{Symbol: "DOGEUSDT", Action: action, Leverage: 5, NotionalUSD: 50}
	}
	t.Run("foreign position on the same contract blocks opening", func(t *testing.T) {
		stub := &ownershipStub{positions: []map[string]interface{}{ownPosition("DOGE_USDT", "long", 17)}}
		at := newOwnershipTrader(stub)
		_, _, err := at.prepareOpen(decision("open_long"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not opened by this trader")
	})
	t.Run("own opposite position blocks opening", func(t *testing.T) {
		stub := &ownershipStub{positions: []map[string]interface{}{ownPosition("DOGE_USDT", "long", 17)}}
		at := newOwnershipTrader(stub)
		at.markSelfOpened("DOGE_USDT", "long")
		_, _, err := at.prepareOpen(decision("open_short"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "opposite")
	})
	t.Run("foreign positions do not count towards the max positions limit", func(t *testing.T) {
		stub := &ownershipStub{positions: []map[string]interface{}{
			ownPosition("BTC_USDT", "long", 1), ownPosition("ETH_USDT", "long", 1), ownPosition("SOL_USDT", "long", 1),
		}}
		at := newOwnershipTrader(stub)
		size, principal, err := at.prepareOpen(decision("open_long"))
		require.NoError(t, err)
		assert.Equal(t, 50.0, size)
		assert.InDelta(t, 1000.0, principal, 1e-9)
	})
}

// 每轮兜底：确认没有止损的本 trader 仓位立即平仓；只是读不到保护时不平仓。
// 平仓前会再读一次保护（破坏性动作不靠单次读数）。
func TestEnforceProtectionInvariant(t *testing.T) {
	unprotected := func() *types.PositionProtection {
		return &types.PositionProtection{Known: false, Reason: "no open stop-loss order for this side"}
	}
	t.Run("confirmed missing stop loss is closed", func(t *testing.T) {
		stub := &ownershipStub{positions: []map[string]interface{}{ownPosition("DOGE_USDT", "long", 17)}, protection: unprotected()}
		at := newOwnershipTrader(stub)
		remaining, closed, failures := at.enforceProtectionInvariant([]kernel.PositionInfo{
			{Symbol: "DOGE_USDT", Side: "long", ProtectionConfirmed: true},
			{Symbol: "ETH_USDT", Side: "short", StopLoss: 2000, ProtectionConfirmed: true},
		})
		require.Empty(t, failures)
		assert.Equal(t, 1, closed)
		require.Len(t, remaining, 1)
		assert.Equal(t, "ETH_USDT", remaining[0].Symbol)
		assert.Equal(t, []string{"DOGE_USDT"}, stub.closeLongs)
	})
	t.Run("unreadable protection is left alone", func(t *testing.T) {
		stub := &ownershipStub{}
		at := newOwnershipTrader(stub)
		remaining, closed, failures := at.enforceProtectionInvariant([]kernel.PositionInfo{
			{Symbol: "DOGE_USDT", Side: "long", ProtectionUnavailableReason: "status unavailable: timeout"},
		})
		require.Empty(t, failures)
		assert.Zero(t, closed)
		require.Len(t, remaining, 1)
		assert.Empty(t, stub.closeLongs)
	})
	t.Run("second read showing a stop loss vetoes the close", func(t *testing.T) {
		// 第一轮读到没有止损，平仓前的第二次读数看到止损又在了（交易所列表短暂不一致）：不能平仓
		stub := &ownershipStub{positions: []map[string]interface{}{ownPosition("DOGE_USDT", "long", 17)}}
		at := newOwnershipTrader(stub)
		remaining, closed, failures := at.enforceProtectionInvariant([]kernel.PositionInfo{
			{Symbol: "DOGE_USDT", Side: "long", ProtectionConfirmed: true},
		})
		require.Empty(t, failures)
		assert.Zero(t, closed)
		require.Len(t, remaining, 1)
		assert.Empty(t, stub.closeLongs)
	})
	t.Run("second read failing vetoes the close", func(t *testing.T) {
		stub := &ownershipStub{positions: []map[string]interface{}{ownPosition("DOGE_USDT", "long", 17)},
			protectionErr: errors.New("gate timeout")}
		at := newOwnershipTrader(stub)
		remaining, closed, failures := at.enforceProtectionInvariant([]kernel.PositionInfo{
			{Symbol: "DOGE_USDT", Side: "long", ProtectionConfirmed: true},
		})
		require.Empty(t, failures)
		assert.Zero(t, closed)
		require.Len(t, remaining, 1)
		assert.Empty(t, stub.closeLongs)
	})
	t.Run("position already gone counts as resolved", func(t *testing.T) {
		// 两次读数之间仓位被止损单平掉：平仓报「持仓不存在」，按已解决处理，不算失败
		stub := &ownershipStub{closeErr: fmt.Errorf("%w: long position not found for DOGE_USDT", types.ErrPositionNotFound),
			protection: unprotected()}
		at := newOwnershipTrader(stub)
		remaining, closed, failures := at.enforceProtectionInvariant([]kernel.PositionInfo{
			{Symbol: "DOGE_USDT", Side: "long", ProtectionConfirmed: true},
		})
		require.Empty(t, failures)
		assert.Equal(t, 1, closed)
		assert.Empty(t, remaining)
	})
	t.Run("failed emergency close keeps the position and reports it", func(t *testing.T) {
		stub := &ownershipStub{closeErr: errors.New("exchange down"), protection: unprotected()}
		at := newOwnershipTrader(stub)
		remaining, closed, failures := at.enforceProtectionInvariant([]kernel.PositionInfo{
			{Symbol: "DOGE_USDT", Side: "long", ProtectionConfirmed: true},
			{Symbol: "ETH_USDT", Side: "short", StopLoss: 2000, ProtectionConfirmed: true},
		})
		require.Len(t, failures, 1)
		assert.Contains(t, failures[0].Error(), "emergency close FAILED")
		assert.Zero(t, closed)
		require.Len(t, remaining, 2, "the position stays visible to the model; the other one is untouched")
		assert.Equal(t, []string{"DOGE_USDT"}, stub.closeLongs)
	})
}

// 兜底平不掉时本轮禁止开仓，其余动作照常
func TestPrepareOpenIsBlockedWhenProtectionCouldNotBeRestored(t *testing.T) {
	stub := &ownershipStub{}
	at := newOwnershipTrader(stub)
	at.openBlockReason = "a position lost its stop loss and could not be closed"
	_, _, err := at.prepareOpen(&kernel.Decision{Symbol: "DOGEUSDT", Action: "open_long", Leverage: 5, NotionalUSD: 50})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "opening is blocked this cycle")
}

// 开仓请求报错后交易所上出现了仓位：必须立即平掉，原始错误保留
func TestClosePositionOpenedByFailedOpen(t *testing.T) {
	stub := &ownershipStub{positions: []map[string]interface{}{ownPosition("DOGE_USDT", "long", 6)}}
	at := newOwnershipTrader(stub)
	openErr := errors.New("request timed out")

	err := at.closePositionOpenedByFailedOpen("DOGEUSDT", openErr)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), openErr.Error()), "the open error must be preserved")
	assert.Equal(t, []string{"DOGE_USDT"}, stub.closeLongs)
}

func TestClosePositionOpenedByFailedOpenWithoutPosition(t *testing.T) {
	stub := &ownershipStub{}
	at := newOwnershipTrader(stub)
	openErr := errors.New("rejected")

	assert.Equal(t, openErr, at.closePositionOpenedByFailedOpen("DOGEUSDT", openErr))
	assert.Empty(t, stub.closeLongs)
}

func TestNoteRejectedDecisions(t *testing.T) {
	at := newOwnershipTrader(&ownershipStub{})
	for i := 0; i < 3; i++ {
		at.noteRejectedDecisions([]kernel.RejectedDecision{{Index: 1, Action: "open_long", Symbol: "DOGE_USDT", Reason: "x"}})
	}
	assert.Equal(t, 3, at.consecutiveRejectedRounds)
	at.noteRejectedDecisions(nil)
	assert.Zero(t, at.consecutiveRejectedRounds)
}
