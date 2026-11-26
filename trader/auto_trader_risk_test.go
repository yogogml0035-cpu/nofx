package trader

// 回撤监控的单元测试（子任务 drawdown-monitor）：杠杆取持仓的实际值、不可用回落 10 倍（F12）；
// 阈值判定（收益率 > 5% 且回撤 ≥ 40% 触发市价全平，计划文档 3.6 节）；仓位归属（D40）——
// 只处理本 trader 开出的仓位，归属不了的直接无视（不管理、不平仓、不记峰值）。

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nofx/store"
	"nofx/trader/types"
)

// riskStub 可配置持仓列表并记录全平调用的适配器桩
type riskStub struct {
	types.Trader
	positions  [][]map[string]interface{} // 每次 GetPositions 依次返回一组
	posErr     error
	closeCalls []string
	closeErr   error
}

func (s *riskStub) GetPositions() ([]map[string]interface{}, error) {
	if s.posErr != nil {
		return nil, s.posErr
	}
	if len(s.positions) == 0 {
		return nil, nil
	}
	first := s.positions[0]
	s.positions = s.positions[1:]
	return first, nil
}

func (s *riskStub) CloseLong(symbol string, quantity float64) (map[string]interface{}, error) {
	s.closeCalls = append(s.closeCalls, fmt.Sprintf("CloseLong %s %g", symbol, quantity))
	if s.closeErr != nil {
		return nil, s.closeErr
	}
	return map[string]interface{}{"orderId": "1"}, nil
}

func (s *riskStub) CloseShort(symbol string, quantity float64) (map[string]interface{}, error) {
	s.closeCalls = append(s.closeCalls, fmt.Sprintf("CloseShort %s %g", symbol, quantity))
	if s.closeErr != nil {
		return nil, s.closeErr
	}
	return map[string]interface{}{"orderId": "2"}, nil
}

// drawdownPosition 构造一个仓位读数。杠杆跟随 F12 的适配层契约：float64 实际值，0 表示不可用；
// leverage == nil 表示键缺失。
func drawdownPosition(symbol, side string, entryPrice, markPrice float64, leverage *float64) map[string]interface{} {
	pos := map[string]interface{}{
		"symbol":           symbol,
		"side":             side,
		"entryPrice":       entryPrice,
		"markPrice":        markPrice,
		"positionAmt":      17.0,
		"unRealizedProfit": 0.0,
		"liquidationPrice": 0.0,
	}
	if leverage != nil {
		pos["leverage"] = *leverage
	}
	return pos
}

func leverageOf(n float64) *float64 { return &n }

// newRiskTestTrader 构造只差 store / 策略配置的 AutoTrader（峰值缓存必须初始化，
// 正式构造函数之外直接建结构体时不会自动创建）
func newRiskTestTrader(t *testing.T, stub *riskStub) *AutoTrader {
	t.Helper()
	return &AutoTrader{
		id:                "trader-risk-1",
		config:            AutoTraderConfig{},
		trader:            stub,
		peakPnLCache:      make(map[string]float64),
		peakPnLCacheMutex: sync.RWMutex{},
	}
}

// newRiskTestStore 建临时 sqlite 库、落一个 trader 记录，并可写一条 OPEN 持仓记录
func newRiskTestStore(t *testing.T, positions ...*store.TraderPosition) *store.Store {
	t.Helper()
	st, err := store.New(filepath.Join(t.TempDir(), "risk.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	require.NoError(t, st.Trader().Create(&store.Trader{
		ID: "trader-risk-1", UserID: "user-1", Name: "t", AIModelID: "model", ExchangeID: "exchange", InitialBalance: 0,
	}))
	nowMs := time.Now().UTC().UnixMilli()
	for _, pos := range positions {
		pos.TraderID = "trader-risk-1"
		pos.ExchangeID = "exchange"
		pos.ExchangeType = "gate"
		pos.EntryTime = nowMs
		pos.Status = "OPEN"
		pos.CreatedAt = nowMs
		pos.UpdatedAt = nowMs
		require.NoError(t, st.Position().Create(pos))
	}
	return st
}

// ownDOGERecord 给 DOGEUSDT 落一条 OPEN 持仓记录（AI 开仓路径经 recordAndConfirmOrder 落库的形态），
// 方向由调用方指定
func ownDOGERecord(t *testing.T, side string) *store.Store {
	t.Helper()
	return newRiskTestStore(t, &store.TraderPosition{
		Symbol: "DOGEUSDT", Side: side, Quantity: 17, EntryPrice: 100, Leverage: 25,
	})
}

// runTwoTicks 连跑两次监控：第一轮记录峰值，第二轮触发回撤判定
func runTwoTicks(t *testing.T, at *AutoTrader) {
	t.Helper()
	at.checkPositionDrawdown()
	at.checkPositionDrawdown()
}

func TestDrawdownClosesWithActualLeverage(t *testing.T) {
	// 25 倍实际杠杆：第一轮 mark 100.9 → 收益率 22.5%，第二轮 mark 100.45 → 11.25%，
	// 回撤 50% ≥ 40% 且 11.25% > 5%，触发市价全平；同样的价格在 10 倍下不会触发（见下一个测试）
	stub := &riskStub{positions: [][]map[string]interface{}{
		{drawdownPosition("DOGE_USDT", "long", 100, 100.9, leverageOf(25))},
		{drawdownPosition("DOGE_USDT", "long", 100, 100.45, leverageOf(25))},
	}}
	at := newRiskTestTrader(t, stub)
	at.store = ownDOGERecord(t, "LONG")

	runTwoTicks(t, at)

	assert.Equal(t, []string{"CloseLong DOGE_USDT 0"}, stub.closeCalls,
		"实际杠杆参与收益率计算，达到阈值即按 close 全平原语平掉全部仓位")
	assert.Empty(t, at.GetPeakPnLCache(), "全平成功后清理该仓位的峰值缓存")
}

func TestDrawdownSamePricesDoNotTriggerAtTenX(t *testing.T) {
	// 与上一个测试完全相同的价格序列，10 倍杠杆下第二轮收益率 4.5% ≤ 5%，不触发
	// （钉住「杠杆取实际值」：恒按 10 倍计的旧逻辑会对这个仓位漏判，取实际值的逻辑必须触发）
	stub := &riskStub{positions: [][]map[string]interface{}{
		{drawdownPosition("DOGE_USDT", "long", 100, 100.9, leverageOf(10))},
		{drawdownPosition("DOGE_USDT", "long", 100, 100.45, leverageOf(10))},
	}}
	at := newRiskTestTrader(t, stub)
	at.store = ownDOGERecord(t, "LONG")

	runTwoTicks(t, at)

	assert.Empty(t, stub.closeCalls)
}

func TestDrawdownShortWithActualLeverage(t *testing.T) {
	// 空头：第一轮 mark 99.1 → 收益率 22.5%，第二轮 mark 99.55 → 11.25%，回撤 50% 触发
	stub := &riskStub{positions: [][]map[string]interface{}{
		{drawdownPosition("DOGE_USDT", "short", 100, 99.1, leverageOf(25))},
		{drawdownPosition("DOGE_USDT", "short", 100, 99.55, leverageOf(25))},
	}}
	at := newRiskTestTrader(t, stub)
	at.store = ownDOGERecord(t, "SHORT")

	runTwoTicks(t, at)

	assert.Equal(t, []string{"CloseShort DOGE_USDT 0"}, stub.closeCalls)
}

func TestDrawdownFallsBackToTenXWhenLeverageUnavailable(t *testing.T) {
	// 适配层读不出杠杆（0 值）按 F12 回落 10 倍：第一轮 mark 101.5 → 15%，第二轮 mark 100.85 → 8.5%，
	// 回撤 43.3% ≥ 40% 且 8.5% > 5%，按 10 倍触发（阈值取值留出浮点余量，不贴 40% 边界）
	stub := &riskStub{positions: [][]map[string]interface{}{
		{drawdownPosition("DOGE_USDT", "long", 100, 101.5, leverageOf(0))},
		{drawdownPosition("DOGE_USDT", "long", 100, 100.85, leverageOf(0))},
	}}
	at := newRiskTestTrader(t, stub)
	at.store = ownDOGERecord(t, "LONG")

	runTwoTicks(t, at)

	assert.Equal(t, []string{"CloseLong DOGE_USDT 0"}, stub.closeCalls)
}

func TestDrawdownNoTriggerBelowThresholds(t *testing.T) {
	// 回撤不足 40%：第一轮 20%，第二轮 15%（回撤 25%）→ 不平仓，峰值继续保留
	drawdownStub := &riskStub{positions: [][]map[string]interface{}{
		{drawdownPosition("DOGE_USDT", "long", 100, 102, leverageOf(10))},
		{drawdownPosition("DOGE_USDT", "long", 100, 101.5, leverageOf(10))},
	}}
	at := newRiskTestTrader(t, drawdownStub)
	at.store = ownDOGERecord(t, "LONG")
	runTwoTicks(t, at)
	assert.Empty(t, drawdownStub.closeCalls)
	assert.InDeltaMapValues(t, map[string]float64{"DOGE_USDT_long": 20}, at.GetPeakPnLCache(), 1e-9,
		"未触发时峰值缓存保留，继续跟踪回撤")

	// 当前收益率 ≤ 5%：第一轮 30%，第二轮 4%——回撤很大但不满足「收益率 > 5%」→ 不平仓
	lowProfitStub := &riskStub{positions: [][]map[string]interface{}{
		{drawdownPosition("DOGE_USDT", "long", 100, 103, leverageOf(10))},
		{drawdownPosition("DOGE_USDT", "long", 100, 100.4, leverageOf(10))},
	}}
	at2 := newRiskTestTrader(t, lowProfitStub)
	at2.store = ownDOGERecord(t, "LONG")
	runTwoTicks(t, at2)
	assert.Empty(t, lowProfitStub.closeCalls)
	assert.InDeltaMapValues(t, map[string]float64{"DOGE_USDT_long": 30}, at2.GetPeakPnLCache(), 1e-9)
}

func TestDrawdownProcessesOnlyOwnPositions(t *testing.T) {
	// D40：DB 里有 OPEN 记录的仓位被处理；没有记录的仓位直接无视——不平、不记峰值
	st := newRiskTestStore(t, &store.TraderPosition{
		Symbol: "DOGEUSDT", Side: "LONG", Quantity: 17, EntryPrice: 100, Leverage: 25,
	})
	stub := &riskStub{positions: [][]map[string]interface{}{
		{
			drawdownPosition("DOGE_USDT", "long", 100, 100.9, leverageOf(25)), // 有记录 → 处理
			drawdownPosition("ETH_USDT", "long", 100, 100.9, leverageOf(25)),  // 无记录 → 无视
		},
		{
			drawdownPosition("DOGE_USDT", "long", 100, 100.45, leverageOf(25)),
			drawdownPosition("ETH_USDT", "long", 100, 100.45, leverageOf(25)),
		},
	}}
	at := newRiskTestTrader(t, stub)
	at.store = st

	runTwoTicks(t, at)

	assert.Equal(t, []string{"CloseLong DOGE_USDT 0"}, stub.closeCalls,
		"只全平 DB 有 OPEN 记录的仓位")
	_, hasETH := at.GetPeakPnLCache()["ETH_USDT_long"]
	assert.False(t, hasETH, "不可归属的仓位不进峰值缓存（对它完全无视）")
	_, hasDOGE := at.GetPeakPnLCache()["DOGE_USDT_long"]
	assert.False(t, hasDOGE, "已平仓位的峰值缓存被清理")
}

func TestDrawdownSkipsEverythingWithoutStore(t *testing.T) {
	// 无 store 时归属不了任何仓位：全平是高风险动作，归属不了就不处理
	stub := &riskStub{positions: [][]map[string]interface{}{
		{drawdownPosition("DOGE_USDT", "long", 100, 100.9, leverageOf(25))},
		{drawdownPosition("DOGE_USDT", "long", 100, 100.45, leverageOf(25))},
	}}
	at := newRiskTestTrader(t, stub)

	runTwoTicks(t, at)

	assert.Empty(t, stub.closeCalls)
	assert.Empty(t, at.GetPeakPnLCache())
}

func TestDrawdownSkipsWhenOwnershipCheckFails(t *testing.T) {
	// DB 查询失败（这里用已关闭的库模拟）：视为不可归属，跳过且不误平
	st := newRiskTestStore(t)
	require.NoError(t, st.Close())
	stub := &riskStub{positions: [][]map[string]interface{}{
		{drawdownPosition("DOGE_USDT", "long", 100, 100.9, leverageOf(25))},
		{drawdownPosition("DOGE_USDT", "long", 100, 100.45, leverageOf(25))},
	}}
	at := newRiskTestTrader(t, stub)
	at.store = st

	runTwoTicks(t, at)

	assert.Empty(t, stub.closeCalls, "归属判定失败时不平仓")
	assert.Empty(t, at.GetPeakPnLCache())
}

func TestDrawdownGridTraderOwnsItsGridSymbol(t *testing.T) {
	// 网格 trader：网格品种上的持仓是自己的（网格由本 trader 的限价单成交建仓，不写 trader_positions），
	// 网格品种以外的持仓无视
	stub := &riskStub{positions: [][]map[string]interface{}{
		{
			drawdownPosition("DOGE_USDT", "long", 100, 100.9, leverageOf(25)), // 网格品种 → 处理
			drawdownPosition("ETH_USDT", "long", 100, 100.9, leverageOf(25)),  // 非网格品种 → 无视
		},
		{
			drawdownPosition("DOGE_USDT", "long", 100, 100.45, leverageOf(25)),
			drawdownPosition("ETH_USDT", "long", 100, 100.45, leverageOf(25)),
		},
	}}
	at := newRiskTestTrader(t, stub)
	at.config.StrategyConfig = &store.StrategyConfig{
		StrategyType: "grid_trading",
		GridConfig:   &store.GridStrategyConfig{Symbol: "DOGEUSDT"},
	}

	runTwoTicks(t, at)

	assert.Equal(t, []string{"CloseLong DOGE_USDT 0"}, stub.closeCalls,
		"网格品种上的持仓由回撤监控管理")
	_, hasETH := at.GetPeakPnLCache()["ETH_USDT_long"]
	assert.False(t, hasETH)
}

func TestDrawdownUpdatesPeakToNewHigh(t *testing.T) {
	// 峰值「更新」半边：第一轮 10%（记峰值），第二轮涨到 22.5%（峰值上调），第三轮回落到 11.25%——
	// 回撤按上调后的新高峰值 22.5% 计（(22.5−11.25)/22.5 = 50% ≥ 40%）触发全平；
	// 峰值若不上调（恒留 10%），第三轮收益率高于峰值、回撤为 0，不会触发
	stub := &riskStub{positions: [][]map[string]interface{}{
		{drawdownPosition("DOGE_USDT", "long", 100, 100.4, leverageOf(25))},
		{drawdownPosition("DOGE_USDT", "long", 100, 100.9, leverageOf(25))},
		{drawdownPosition("DOGE_USDT", "long", 100, 100.45, leverageOf(25))},
	}}
	at := newRiskTestTrader(t, stub)
	at.store = ownDOGERecord(t, "LONG")

	at.checkPositionDrawdown()
	assert.InDelta(t, 10.0, at.GetPeakPnLCache()["DOGE_USDT_long"], 1e-9, "第一轮记录初始峰值")

	at.checkPositionDrawdown()
	assert.InDelta(t, 22.5, at.GetPeakPnLCache()["DOGE_USDT_long"], 1e-9, "收益率创新高时峰值被上调")

	at.checkPositionDrawdown()
	assert.Equal(t, []string{"CloseLong DOGE_USDT 0"}, stub.closeCalls,
		"回撤按上调后的峰值计算，达到阈值即全平")
	assert.Empty(t, at.GetPeakPnLCache(), "全平成功后清理峰值缓存")
}

func TestDrawdownFallbackTenXBoundaryAtFivePercent(t *testing.T) {
	// 杠杆不可用回落 10 倍 + 收益率阈值「严格大于」边界：第一轮 Δ+1.5% → 15%（峰值），
	// 第二轮 Δ+0.5% → 恰 5.0%（不满足 > 5%）→ 不平仓。
	// 回落值若偏大（如 25 倍）：峰值 37.5%、第二轮 12.5%、回撤 66.7% 会触发——回落值被钉住
	stub := &riskStub{positions: [][]map[string]interface{}{
		{drawdownPosition("DOGE_USDT", "long", 100, 101.5, leverageOf(0))},
		{drawdownPosition("DOGE_USDT", "long", 100, 100.5, leverageOf(0))},
	}}
	at := newRiskTestTrader(t, stub)
	at.store = ownDOGERecord(t, "LONG")

	runTwoTicks(t, at)

	assert.Empty(t, stub.closeCalls, "恰 5.0% 不满足严格大于，且回落值恰为 10 倍时不可触发")
	assert.InDelta(t, 15.0, at.GetPeakPnLCache()["DOGE_USDT_long"], 1e-9, "峰值缓存保留")
}

func TestDrawdownNoTriggerInDrawdownBandThirtyToForty(t *testing.T) {
	// 回撤 30%–40% 区间：峰值 20%（Δ+2%）、当前 13%（Δ+1.3%）→ 回撤 35%，不满足 ≥ 40% → 不平仓
	stub := &riskStub{positions: [][]map[string]interface{}{
		{drawdownPosition("DOGE_USDT", "long", 100, 102, leverageOf(10))},
		{drawdownPosition("DOGE_USDT", "long", 100, 101.3, leverageOf(10))},
	}}
	at := newRiskTestTrader(t, stub)
	at.store = ownDOGERecord(t, "LONG")

	runTwoTicks(t, at)

	assert.Empty(t, stub.closeCalls, "回撤 35% 在 30–40 区间，不触发")
	assert.InDelta(t, 20.0, at.GetPeakPnLCache()["DOGE_USDT_long"], 1e-9)
}

func TestDrawdownTriggersAtExactlyFortyPercentDrawdown(t *testing.T) {
	// 回撤阈值「≥」边界：25 倍下 Δ+2.5% → 峰值 62.5%，回落到 Δ+1.5% → 37.5%，
	// (62.5−37.5)/62.5 恰为 40.0 → 触发全平（≥ 语义；严格大于会漏掉这个边界）
	stub := &riskStub{positions: [][]map[string]interface{}{
		{drawdownPosition("DOGE_USDT", "long", 100, 102.5, leverageOf(25))},
		{drawdownPosition("DOGE_USDT", "long", 100, 101.5, leverageOf(25))},
	}}
	at := newRiskTestTrader(t, stub)
	at.store = ownDOGERecord(t, "LONG")

	runTwoTicks(t, at)

	assert.Equal(t, []string{"CloseLong DOGE_USDT 0"}, stub.closeCalls, "恰 40% 回撤满足 ≥ 语义，触发全平")
	assert.Empty(t, at.GetPeakPnLCache())
}

func TestClearPeakPnLCacheMatchesSymbolVariants(t *testing.T) {
	at := &AutoTrader{peakPnLCache: map[string]float64{
		"DOGE_USDT_long":  12.3,
		"DOGE_USDT_short": 4.5,
		"BTC_USDT_long":   1.0,
	}}
	at.ClearPeakPnLCache("DOGEUSDT", "long")

	assert.NotContains(t, at.peakPnLCache, "DOGE_USDT_long", "决策里的 DOGEUSDT 也要清掉交易所符号键")
	assert.Contains(t, at.peakPnLCache, "DOGE_USDT_short", "同合约反方向不受影响")
	assert.Contains(t, at.peakPnLCache, "BTC_USDT_long", "其他合约不受影响")
}
