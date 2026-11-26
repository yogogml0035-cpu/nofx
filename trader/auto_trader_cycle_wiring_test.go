package trader

// runCycle 的接线测试（对抗审查视角 D：辅助函数有单测，但调用点被删掉时全套测试仍然通过）。
// 通过 requestFullDecision / decisionSavedHook 两个测试缝替换模型调用与决策记录落库，
// 不访问网络、不写数据库。

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nofx/kernel"
	"nofx/mcp"
	"nofx/store"
	"nofx/trader/types"
)

// withDecisionSeam 替换决策入口与保存钩子，返回调用次数与最后一次决策记录
func withDecisionSeam(t *testing.T, decision *kernel.FullDecision) (*int, **store.DecisionRecord) {
	t.Helper()
	calls := 0
	var saved *store.DecisionRecord
	previousDecision, previousHook := requestFullDecision, decisionSavedHook
	requestFullDecision = func(_ *kernel.Context, _ mcp.AIClient, _ *kernel.StrategyEngine, _ string) (*kernel.FullDecision, error) {
		calls++
		return decision, nil
	}
	decisionSavedHook = func(_ *AutoTrader, record *store.DecisionRecord) {
		copied := *record
		saved = &copied
	}
	t.Cleanup(func() {
		requestFullDecision, decisionSavedHook = previousDecision, previousHook
	})
	return &calls, &saved
}

// 每轮兜底在 runCycle 里被调用：确认失去止损的仓位被平掉，决策记录里可见
func TestRunCycleEnforcesProtectionInvariant(t *testing.T) {
	stub := &ownershipStub{
		positions:  []map[string]interface{}{ownPosition("DOGE_USDT", "long", 17)},
		protection: &types.PositionProtection{Known: false, Reason: "no open stop-loss order for this side"},
	}
	at := newOwnershipTrader(stub)
	at.markSelfOpened("DOGE_USDT", "long")
	calls, saved := withDecisionSeam(t, &kernel.FullDecision{})

	require.NoError(t, at.runCycle())
	assert.Equal(t, 1, *calls)
	assert.Equal(t, []string{"DOGE_USDT"}, stub.closeLongs)
	require.NotNil(t, *saved)
	assert.True(t, containsLine((*saved).ExecutionLog, "PROTECTION LOST: 1 position(s)"))
}

// 兜底平仓失败：本轮禁止开仓，其余动作照常，失败写进决策记录
func TestRunCycleBlocksOpeningWhenProtectionCloseFails(t *testing.T) {
	stub := &ownershipStub{
		positions:  []map[string]interface{}{ownPosition("DOGE_USDT", "long", 17)},
		protection: &types.PositionProtection{Known: false, Reason: "no open stop-loss order for this side"},
		closeErr:   errTestExchangeDown,
	}
	at := newOwnershipTrader(stub)
	at.markSelfOpened("DOGE_USDT", "long")
	decision := &kernel.FullDecision{Decisions: []kernel.Decision{
		{Symbol: "BTCUSDT", Action: "open_long", Leverage: 5, NotionalUSD: 50, StopLoss: 90, TakeProfit: 150},
	}}
	_, saved := withDecisionSeam(t, decision)

	require.NoError(t, at.runCycle())
	require.NotNil(t, *saved)
	assert.True(t, containsLine((*saved).ExecutionLog, "PROTECTION LOST:"))
	require.Len(t, (*saved).Decisions, 1)
	assert.False(t, (*saved).Decisions[0].Success)
	assert.Contains(t, (*saved).Decisions[0].Error, "opening is blocked this cycle")
	assert.Empty(t, stub.openLongs, "兜底失败时本轮不开仓")
}

// 被校验丢弃的决策进决策记录
func TestRunCycleRecordsRejectedDecisions(t *testing.T) {
	stub := &ownershipStub{}
	at := newOwnershipTrader(stub)
	decision := &kernel.FullDecision{
		Decisions: []kernel.Decision{{Symbol: "BTCUSDT", Action: "wait"}},
		Rejected:  []kernel.RejectedDecision{{Index: 1, Action: "open_long", Symbol: "BTCUSDT", Reason: "stop loss above current price"}},
	}
	_, saved := withDecisionSeam(t, decision)

	require.NoError(t, at.runCycle())
	require.NotNil(t, *saved)
	assert.True(t, containsLine((*saved).ExecutionLog, "REJECTED decision #1 (open_long BTCUSDT): stop loss above current price"))
}

// 无数据无仓位时跳过模型：跳过原因进决策记录
func TestRunCycleRecordsSkippedDecision(t *testing.T) {
	stub := &ownershipStub{}
	at := newOwnershipTrader(stub)
	decision := &kernel.FullDecision{SkippedReason: "no market data and no positions to manage"}
	calls, saved := withDecisionSeam(t, decision)

	require.NoError(t, at.runCycle())
	require.NotNil(t, *saved)
	assert.Equal(t, 1, *calls)
	assert.True(t, containsLine((*saved).ExecutionLog, "AI call skipped: no market data and no positions to manage"))
	assert.Empty(t, stub.openLongs)
}

// buildTradingContext 把「条件单列表读取成功」写进持仓信息（兜底的前提）
func TestBuildTradingContextSetsProtectionConfirmed(t *testing.T) {
	stub := &ownershipStub{positions: []map[string]interface{}{ownPosition("DOGE_USDT", "long", 17)}}
	at := newOwnershipTrader(stub)
	at.markSelfOpened("DOGE_USDT", "long")
	ctx, err := at.buildTradingContext()
	require.NoError(t, err)
	require.Len(t, ctx.Positions, 1)
	assert.True(t, ctx.Positions[0].ProtectionConfirmed)

	// 适配器没有保护管理能力：不能声称「确认没有止损」，兜底不得触发
	at2 := newOwnershipTrader(&ownershipStub{})
	at2.trader = &partialCloseNoCapabilityStub{}
	at2.markSelfOpened("DOGE_USDT", "long")
	ctx2, err := at2.buildTradingContext()
	require.NoError(t, err)
	require.Len(t, ctx2.Positions, 1)
	assert.False(t, ctx2.Positions[0].ProtectionConfirmed)
}

// 开仓成功后登记自开仓位；仓位消失后清登记
func TestOpenRegistersSelfOpenedAndContextForgetsIt(t *testing.T) {
	stub := &ownershipStub{}
	at := newOwnershipTrader(stub)
	decision := &kernel.Decision{Symbol: "DOGEUSDT", Action: "open_long", Leverage: 5, NotionalUSD: 50, StopLoss: 0.04, TakeProfit: 0.07}
	actionRecord := &store.DecisionAction{Action: "open_long", Symbol: "DOGEUSDT"}
	require.NoError(t, at.executeDecisionWithRecord(decision, actionRecord))
	require.Len(t, stub.openLongs, 1)
	assert.True(t, at.isSelfOpened("DOGE_USDT", "long"), "刚开出的仓位必须登记归属")

	// 交易所上还有仓位：登记保留
	stub.positions = []map[string]interface{}{ownPosition("DOGE_USDT", "long", 2)}
	_, err := at.buildTradingContext()
	require.NoError(t, err)
	assert.True(t, at.isSelfOpened("DOGE_USDT", "long"))

	// 仓位消失：登记清掉，之后同合约同向的手动仓不会被误认
	stub.positions = nil
	_, err = at.buildTradingContext()
	require.NoError(t, err)
	assert.False(t, at.isSelfOpened("DOGE_USDT", "long"))
}

// 开仓请求报错后交易所上出现了仓位：立即平掉
func TestOpenErrorTriggersEmergencyClose(t *testing.T) {
	stub := &ownershipStub{
		openErr:              errTestRequestTimeout,
		positionsOnOpenError: []map[string]interface{}{ownPosition("DOGE_USDT", "long", 2)},
	}
	at := newOwnershipTrader(stub)
	decision := &kernel.Decision{Symbol: "DOGEUSDT", Action: "open_long", Leverage: 5, NotionalUSD: 50, StopLoss: 0.04, TakeProfit: 0.07}
	actionRecord := &store.DecisionAction{Action: "open_long", Symbol: "DOGEUSDT"}

	err := at.executeDecisionWithRecord(decision, actionRecord)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
	assert.Equal(t, []string{"DOGE_USDT"}, stub.closeLongs)
}

var (
	errTestExchangeDown   = testError("exchange down")
	errTestRequestTimeout = testError("request timed out")
)

type testError string

func (e testError) Error() string { return string(e) }

func containsLine(lines []string, substr string) bool {
	for _, line := range lines {
		if strings.Contains(line, substr) {
			return true
		}
	}
	return false
}
