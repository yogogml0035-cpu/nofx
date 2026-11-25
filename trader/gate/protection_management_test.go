package gate

// 止损止盈管理（保护读取与撤旧建新改单）的单元测试。httptest 服务按 Gate 线上格式响应：
// 未触发条件单列表、持仓条目、条件单创建与撤销；价格步长来自合约的 order_price_round。
// 真实写操作与线上形态的证据在任务 probe/ 目录，不进单元测试。

import (
	"math/big"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nofx/trader/types"
)

// protectionMock 一组合法的多头持仓与随单保护单（DOGE：17 张 × 10 币，入场 0.56）
func protectionMock(t *testing.T) (*gateMock, *GateTrader) {
	t.Helper()
	mock := newGateMock()
	mock.mu.Lock()
	mock.positions = []map[string]interface{}{livePosition("DOGE_USDT", "17", "0.56")}
	mock.priceOrders = []map[string]interface{}{
		positionLevelTriggerOrder(551, "DOGE_USDT", "LONG", "SL"),
		positionLevelTriggerOrder(552, "DOGE_USDT", "LONG", "TP"),
	}
	mock.mu.Unlock()
	return mock, newLocalGateTrader(t, mock.handler)
}

func (m *gateMock) priceOrdersSnapshot(t *testing.T) []map[string]interface{} {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]map[string]interface{}(nil), m.priceOrders...)
}

func (m *gateMock) lastPriceOrderBody(t *testing.T) map[string]interface{} {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	require.NotEmpty(t, m.priceOrderBodies, "no trigger order submitted")
	return m.priceOrderBodies[len(m.priceOrderBodies)-1]
}

func TestPositionProtectionReadsEffectiveOrders(t *testing.T) {
	cases := []struct {
		name           string
		orders         func(m *gateMock)
		wantSL, wantTP float64
		wantSLActions  int
		wantTPActions  int
		wantKnown      bool
		wantReason     string
	}{
		{
			name: "合格的一对",
			orders: func(m *gateMock) {
				m.attachAttachedTriggers("DOGE_USDT", "17", "0.48", "0.65", 1)
			},
			wantSL: 0.48, wantTP: 0.65, wantSLActions: 1, wantTPActions: 1, wantKnown: true,
		},
		{
			name: "同类多条取距现价最近（最先触发）",
			orders: func(m *gateMock) {
				m.priceOrders = []map[string]interface{}{
					positionLevelTriggerOrder(551, "DOGE_USDT", "LONG", "SL"),
					positionLevelTriggerOrder(552, "DOGE_USDT", "LONG", "TP"),
				}
			},
			// positionLevelTriggerOrder 的止损价 0.48；再接一条更近的止损
			wantSL: 0.52, wantTP: 0.65, wantSLActions: 2, wantTPActions: 1, wantKnown: true,
		},
		{
			name: "缺止盈",
			orders: func(m *gateMock) {
				m.priceOrders = []map[string]interface{}{positionLevelTriggerOrder(551, "DOGE_USDT", "LONG", "SL")}
			},
			wantSL: 0.48, wantSLActions: 1, wantKnown: false, wantReason: "no open take-profit",
		},
		{
			name:       "两者都缺",
			orders:     func(m *gateMock) {},
			wantKnown:  false,
			wantReason: "no open stop-loss or take-profit",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mock := newGateMock()
			mock.mu.Lock()
			mock.priceOrders = nil
			c.orders(mock)
			if c.name == "同类多条取距现价最近（最先触发）" {
				// 更近的止损：0.52
				mock.priceOrders = append(mock.priceOrders, map[string]interface{}{
					"id": 554, "order_type": "close-long-position", "status": "open",
					"initial": map[string]interface{}{"contract": "DOGE_USDT", "size": 0, "price": "0", "close": true, "tif": "ioc"},
					"trigger": map[string]interface{}{"strategy_type": 0, "price_type": 0, "price": "0.52", "rule": PriceRuleLE},
				})
			}
			mock.mu.Unlock()
			gt := newLocalGateTrader(t, mock.handler)

			protection, err := gt.PositionProtection("DOGEUSDT", "LONG")
			require.NoError(t, err)
			assert.InDelta(t, c.wantSL, protection.StopLoss, 1e-9)
			assert.InDelta(t, c.wantTP, protection.TakeProfit, 1e-9)
			assert.Equal(t, c.wantSLActions, protection.StopLossOrders)
			assert.Equal(t, c.wantTPActions, protection.TakeProfitOrders)
			assert.Equal(t, c.wantKnown, protection.Known)
			if !c.wantKnown {
				assert.Contains(t, protection.Reason, c.wantReason)
			}
		})
	}
}

func TestPositionProtectionShortSide(t *testing.T) {
	mock := newGateMock()
	mock.mu.Lock()
	mock.positions = []map[string]interface{}{livePosition("DOGE_USDT", "-17", "0.56")}
	// 空头：止损在上（0.65，rule 1）、止盈在下（0.48，rule 2）
	shortOrder := func(id int64, price string, rule int32) map[string]interface{} {
		return map[string]interface{}{
			"id": id, "order_type": "close-short-position", "status": "open",
			"initial": map[string]interface{}{"contract": "DOGE_USDT", "size": 0, "price": "0", "close": true, "tif": "ioc"},
			"trigger": map[string]interface{}{"strategy_type": 0, "price_type": 0, "price": price, "rule": rule},
		}
	}
	mock.priceOrders = []map[string]interface{}{
		shortOrder(551, "0.65", PriceRuleGE),
		shortOrder(552, "0.48", PriceRuleLE),
	}
	mock.mu.Unlock()
	gt := newLocalGateTrader(t, mock.handler)

	protection, err := gt.PositionProtection("DOGE_USDT", "SHORT")
	require.NoError(t, err)
	assert.True(t, protection.Known)
	assert.InDelta(t, 0.65, protection.StopLoss, 1e-9, "空头止损在上")
	assert.InDelta(t, 0.48, protection.TakeProfit, 1e-9, "空头止盈在下")
}

func TestUpdateProtectionStopLoss(t *testing.T) {
	mock, gt := protectionMock(t)

	// 0.56 → 止损 0.55：数量 170 × 距离 0.01 = 1.7 USDT ≤ 本金 100 的 2%
	result, err := gt.UpdateProtection(types.ProtectionUpdateRequest{
		Symbol: "DOGEUSDT", Side: "long", Kind: types.ProtectionStopLoss,
		Price: 0.55, Principal: 100, MaxLossPercent: 2,
	})
	require.NoError(t, err)

	body := mock.lastPriceOrderBody(t)
	assert.Equal(t, "close-long-position", body["order_type"])
	initial, _ := body["initial"].(map[string]interface{})
	assert.Equal(t, "DOGE_USDT", initial["contract"])
	assert.Equal(t, "0", initial["price"], "保护单触发后市价出场")
	assert.Equal(t, true, initial["close"], "单向持仓模式全平")
	trigger, _ := body["trigger"].(map[string]interface{})
	assert.Equal(t, "0.55", trigger["price"], "对齐到 0.00001 价格步长（最短十进制表示）")
	assert.EqualValues(t, PriceRuleLE, trigger["rule"], "多头止损在价格回落时触发")

	assert.Equal(t, "700001", result.OrderID)
	assert.InDelta(t, 0.48, result.OldPrice, 1e-9)
	assert.InDelta(t, 0.55, result.NewPrice, 1e-9)
	assert.Equal(t, []string{"551"}, result.ReplacedOrderIDs)

	orders := mock.priceOrdersSnapshot(t)
	ids := map[int64]bool{}
	for _, order := range orders {
		if id, ok := order["id"].(int64); ok {
			ids[id] = true
		}
	}
	assert.True(t, ids[552], "止盈单不受影响")
	assert.True(t, ids[700001], "新止损单在列表里")
	assert.False(t, ids[551], "旧止损单已撤销")
}

func TestUpdateProtectionTakeProfitMovesTowardPrice(t *testing.T) {
	mock, gt := protectionMock(t)

	// 止盈从 0.65 向现价移到 0.62（D32 允许双向移动；止盈没有亏损上限）
	result, err := gt.UpdateProtection(types.ProtectionUpdateRequest{
		Symbol: "DOGE_USDT", Side: "long", Kind: types.ProtectionTakeProfit,
		Price: 0.62, Principal: 100, MaxLossPercent: 2,
	})
	require.NoError(t, err)
	assert.InDelta(t, 0.65, result.OldPrice, 1e-9)
	body := mock.lastPriceOrderBody(t)
	trigger, _ := body["trigger"].(map[string]interface{})
	assert.Equal(t, "0.62", trigger["price"])
	assert.EqualValues(t, PriceRuleGE, trigger["rule"], "多头止盈在价格上行时触发")
	assert.Equal(t, []string{"552"}, result.ReplacedOrderIDs)
}

func TestUpdateProtectionFailClosed(t *testing.T) {
	cases := []struct {
		name      string
		arrange   func(m *gateMock)
		req       types.ProtectionUpdateRequest
		wantErr   string
		wantPosts int // 期望发出的条件单创建请求数
	}{
		{
			name:    "持仓不存在",
			arrange: func(m *gateMock) { m.positions = nil },
			req:     types.ProtectionUpdateRequest{Symbol: "DOGE_USDT", Side: "long", Kind: types.ProtectionStopLoss, Price: 0.55, Principal: 100, MaxLossPercent: 2},
			wantErr: "position not found",
		},
		{
			name:    "止损亏损超过本金 2%",
			arrange: func(m *gateMock) {},
			req:     types.ProtectionUpdateRequest{Symbol: "DOGE_USDT", Side: "long", Kind: types.ProtectionStopLoss, Price: 0.50, Principal: 10, MaxLossPercent: 2},
			wantErr: "exceeds 2% of principal",
		},
		{
			name:    "本金不可用",
			arrange: func(m *gateMock) {},
			req:     types.ProtectionUpdateRequest{Symbol: "DOGE_USDT", Side: "long", Kind: types.ProtectionStopLoss, Price: 0.55, Principal: 0, MaxLossPercent: 2},
			wantErr: "principal must be positive",
		},
		{
			name:    "多头止损高于现价",
			arrange: func(m *gateMock) {},
			req:     types.ProtectionUpdateRequest{Symbol: "DOGE_USDT", Side: "long", Kind: types.ProtectionStopLoss, Price: 0.58, Principal: 1000, MaxLossPercent: 2},
			wantErr: "must be below current price",
		},
		{
			name:    "多头止盈低于现价",
			arrange: func(m *gateMock) {},
			req:     types.ProtectionUpdateRequest{Symbol: "DOGE_USDT", Side: "long", Kind: types.ProtectionTakeProfit, Price: 0.54, Principal: 1000, MaxLossPercent: 2},
			wantErr: "must be above current price",
		},
		{
			name:    "对齐后止损与现价重合",
			arrange: func(m *gateMock) {},
			// 0.55999 低于现价 0.56，但向上对齐到 0.56 → 越界
			req:     types.ProtectionUpdateRequest{Symbol: "DOGE_USDT", Side: "long", Kind: types.ProtectionStopLoss, Price: 0.559999, Principal: 1000, MaxLossPercent: 2},
			wantErr: "must be below current price",
		},
		{
			name: "当前保护不完整",
			arrange: func(m *gateMock) {
				m.priceOrders = []map[string]interface{}{positionLevelTriggerOrder(551, "DOGE_USDT", "LONG", "SL")}
			},
			req:     types.ProtectionUpdateRequest{Symbol: "DOGE_USDT", Side: "long", Kind: types.ProtectionStopLoss, Price: 0.55, Principal: 1000, MaxLossPercent: 2},
			wantErr: "not readable",
		},
		{
			name:    "新价格非正",
			arrange: func(m *gateMock) {},
			req:     types.ProtectionUpdateRequest{Symbol: "DOGE_USDT", Side: "long", Kind: types.ProtectionStopLoss, Price: 0, Principal: 100, MaxLossPercent: 2},
			wantErr: "must be a positive number",
		},
		{
			name:    "类型非法",
			arrange: func(m *gateMock) {},
			req:     types.ProtectionUpdateRequest{Symbol: "DOGE_USDT", Side: "long", Kind: "bogus", Price: 0.55, Principal: 100, MaxLossPercent: 2},
			wantErr: "invalid protection kind",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mock, gt := protectionMock(t)
			mock.mu.Lock()
			c.arrange(mock)
			mock.mu.Unlock()

			_, err := gt.UpdateProtection(c.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.wantErr)
			assert.Empty(t, mock.priceOrderBodies, "被拒绝的改单不得发出任何条件单请求")
			for _, request := range mock.requestsSnapshot(t) {
				assert.NotContains(t, request, "DELETE /futures/usdt/price_orders", "被拒绝的改单不得撤单")
			}
		})
	}
}

func TestUpdateProtectionRestoresOnFailures(t *testing.T) {
	t.Run("挂新单被交易所拒绝：原保护不动", func(t *testing.T) {
		mock, gt := protectionMock(t)
		mock.mu.Lock()
		mock.priceOrderFail = &gateAPIFailure{status: 400, label: "INVALID_PARAM_VALUE", message: "invalid trigger price"}
		mock.mu.Unlock()

		_, err := gt.UpdateProtection(types.ProtectionUpdateRequest{
			Symbol: "DOGE_USDT", Side: "long", Kind: types.ProtectionStopLoss,
			Price: 0.55, Principal: 100, MaxLossPercent: 2,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "INVALID_PARAM_VALUE")
		orders := mock.priceOrdersSnapshot(t)
		assert.Len(t, orders, 2, "原止损与止盈都在，未新增")
		for _, request := range mock.requestsSnapshot(t) {
			assert.NotContains(t, request, "DELETE /futures/usdt/price_orders", "挂新单失败时不得撤旧单")
		}
	})

	t.Run("新单未出现在列表：撤新单并保留旧保护", func(t *testing.T) {
		mock, gt := protectionMock(t)
		mock.mu.Lock()
		mock.omitPlacedPriceOrders = true
		mock.mu.Unlock()

		_, err := gt.UpdateProtection(types.ProtectionUpdateRequest{
			Symbol: "DOGE_USDT", Side: "long", Kind: types.ProtectionStopLoss,
			Price: 0.55, Principal: 100, MaxLossPercent: 2,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not visible in the open trigger order list")
		orders := mock.priceOrdersSnapshot(t)
		assert.Len(t, orders, 2, "旧止损与止盈仍在")
		foundDeleteNew := false
		for _, request := range mock.requestsSnapshot(t) {
			if strings.Contains(request, "DELETE /futures/usdt/price_orders/700001") {
				foundDeleteNew = true
			}
		}
		assert.True(t, foundDeleteNew, "核对失败后撤销新单")
	})

	t.Run("撤旧单失败：撤新单恢复原保护", func(t *testing.T) {
		mock, gt := protectionMock(t)
		mock.mu.Lock()
		mock.cancelPriceOrderFail = &gateAPIFailure{status: 400, label: "ORDER_NOT_FOUND", message: "cancel failed"}
		mock.mu.Unlock()

		_, err := gt.UpdateProtection(types.ProtectionUpdateRequest{
			Symbol: "DOGE_USDT", Side: "long", Kind: types.ProtectionStopLoss,
			Price: 0.55, Principal: 100, MaxLossPercent: 2,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to cancel old")
		orders := mock.priceOrdersSnapshot(t)
		// 旧止损仍在；新单的撤销也失败了（同一撤销接口被注入失败），但被注入的失败是交易所错误，不是无保护
		assert.Len(t, orders, 3, "旧止损、止盈与新单都在：保护仍有效，失败可见")
	})

	t.Run("撤销成功但旧单仍在列表：恢复并报错", func(t *testing.T) {
		mock, gt := protectionMock(t)
		mock.mu.Lock()
		mock.cancelIneffective = true
		mock.mu.Unlock()

		_, err := gt.UpdateProtection(types.ProtectionUpdateRequest{
			Symbol: "DOGE_USDT", Side: "long", Kind: types.ProtectionStopLoss,
			Price: 0.55, Principal: 100, MaxLossPercent: 2,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "still visible after cancellation")
		orders := mock.priceOrdersSnapshot(t)
		assert.Len(t, orders, 3, "撤销接口不生效：旧止损与新单都留在列表里，报错可见")
	})
}

func TestWithinLossCapTolerance(t *testing.T) {
	limit := big.NewRat(2, 1) // 本金 100 的 2%
	cases := []struct {
		name string
		loss *big.Rat
		want bool
	}{
		{name: "等于上限", loss: big.NewRat(2, 1), want: true},
		{name: "上限内极小浮点噪声（0.5e-12）", loss: new(big.Rat).Mul(limit, big.NewRat(2_000_000_000_001, 2_000_000_000_000)), want: true},
		{name: "超出容差（2e-12）", loss: new(big.Rat).Mul(limit, big.NewRat(1_000_000_000_002, 1_000_000_000_000)), want: false},
		{name: "明显超出", loss: big.NewRat(3, 1), want: false},
		{name: "上限非正", loss: big.NewRat(0, 1), want: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.name == "上限非正" {
				assert.False(t, withinLossCap(c.loss, new(big.Rat)))
				return
			}
			assert.Equal(t, c.want, withinLossCap(c.loss, limit))
		})
	}
}

func TestUpdateProtectionKeepsUnprotectedSentinel(t *testing.T) {
	// 极端场景：新单未进列表，恢复时撤销新单触发的钩子把全部条件单清空，恢复也挂不上单。
	// 外层错误必须保留 types.ErrPositionUnprotected，交易路径才能识别并立即平仓。
	mock, gt := protectionMock(t)
	mock.mu.Lock()
	mock.omitPlacedPriceOrders = true
	mock.afterPriceOrderDelete = func(m *gateMock) {
		m.priceOrders = nil
	}
	mock.mu.Unlock()

	_, err := gt.UpdateProtection(types.ProtectionUpdateRequest{
		Symbol: "DOGE_USDT", Side: "long", Kind: types.ProtectionStopLoss,
		Price: 0.55, Principal: 100, MaxLossPercent: 2,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, types.ErrPositionUnprotected, "恢复失败且保护不存在时必须保留哨兵")
}
