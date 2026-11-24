package gate

// 开平仓原语重写后的单元测试：全平 payload、向下取整换算、独立止损止盈不再支持。
// 复用 protected_open_test.go 的 gateMock（DOGE：1 张 = 10 币，0.56 USDT/张）。

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCloseLongAllPayload(t *testing.T) {
	mock := newGateMock()
	gt := newLocalGateTrader(t, mock.handler)
	mock.mu.Lock()
	mock.positions = []map[string]interface{}{
		{"contract": "DOGE_USDT", "size": "17", "entry_price": "0.55", "leverage": "10"},
	}
	mock.mu.Unlock()

	res, err := gt.CloseLong("DOGE_USDT", 0)
	require.NoError(t, err)
	assert.Equal(t, "99123", res["orderId"], "订单 ID 以字符串返回（int64 断言修复后仍可读）")

	body := mock.lastOrderBody(t)
	assert.Equal(t, true, body["close"], "全平按文档用 close=true（C.1 节）")
	assert.Equal(t, "0", body["size"], "全平 size=0，不再换算张数（F8）")
	assert.Equal(t, "t-nofx-close", body["text"])
}

func TestCloseShortAllPayload(t *testing.T) {
	mock := newGateMock()
	gt := newLocalGateTrader(t, mock.handler)
	mock.mu.Lock()
	mock.positions = []map[string]interface{}{
		{"contract": "DOGE_USDT", "size": "-17", "entry_price": "0.55", "leverage": "10"},
	}
	mock.mu.Unlock()

	_, err := gt.CloseShort("DOGE_USDT", 0)
	require.NoError(t, err)
	body := mock.lastOrderBody(t)
	assert.Equal(t, true, body["close"])
	assert.Equal(t, "0", body["size"])
}

func TestCloseWithoutPositionFails(t *testing.T) {
	mock := newGateMock()
	gt := newLocalGateTrader(t, mock.handler)
	mock.mu.Lock()
	mock.positions = []map[string]interface{}{
		{"contract": "DOGE_USDT", "size": "17"}, // 只有多头
	}
	mock.mu.Unlock()

	_, err := gt.CloseShort("DOGE_USDT", 0)
	require.Error(t, err, "无空头仓位时全平报错")
	assert.Contains(t, err.Error(), "short position not found")

	mock.mu.Lock()
	assert.Empty(t, mock.orderBodies, "未提交订单")
	mock.mu.Unlock()
}

func TestClosePartialFloorsToStep(t *testing.T) {
	mock := newGateMock()
	gt := newLocalGateTrader(t, mock.handler)

	// 165 币 ÷ 10 币/张 = 16.5 张 → 向下取整 16 张，平多为负
	_, err := gt.CloseLong("DOGE_USDT", 165)
	require.NoError(t, err)
	body := mock.lastOrderBody(t)
	assert.Equal(t, "-16", body["size"])
	assert.Equal(t, true, body["reduce_only"])
	_, hasClose := body["close"]
	assert.False(t, hasClose, "部分平仓不是全平委托")
}

func TestClosePartialBelowStepFails(t *testing.T) {
	gt := newLocalGateTrader(t, newGateMock().handler)

	_, err := gt.CloseLong("DOGE_USDT", 9) // 0.9 张，不足一步
	require.Error(t, err)
	assert.Contains(t, err.Error(), "below one size step")
}

func TestOpenLongFloorsToStep(t *testing.T) {
	mock := newGateMock()
	gt := newLocalGateTrader(t, mock.handler)

	_, err := gt.OpenLong("DOGE_USDT", 170, 10) // 17 张整
	require.NoError(t, err)
	assert.Equal(t, "17", mock.lastOrderBody(t)["size"])

	_, err = gt.OpenLong("DOGE_USDT", 169.9, 10) // 16.99 张 → 16 张
	require.NoError(t, err)
	assert.Equal(t, "16", mock.lastOrderBody(t)["size"])

	_, err = gt.OpenShort("DOGE_USDT", 170, 10)
	require.NoError(t, err)
	assert.Equal(t, "-17", mock.lastOrderBody(t)["size"])

	// 旧实现把不足 1 张强制成 1 张（F3：放大订单）；现在直接拒绝
	_, err = gt.OpenLong("DOGE_USDT", 9, 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "below one size step")
}

func TestOpenMarketNoCancelAllOrders(t *testing.T) {
	mock := newGateMock()
	gt := newLocalGateTrader(t, mock.handler)
	mock.mu.Lock()
	mock.priceOrders = []map[string]interface{}{
		positionLevelTriggerOrder(551, "DOGE_USDT", "LONG", "SL"),
	}
	mock.mu.Unlock()

	_, err := gt.OpenLong("DOGE_USDT", 170, 10)
	require.NoError(t, err)
	// 开仓是 IOC 市价单：不再预撤全部条件单（避免误撤已有仓位的保护单）
	mock.mu.Lock()
	assert.Len(t, mock.orderBodies, 1, "只提交开仓单，没有先发撤销请求")
	mock.mu.Unlock()
}

func TestStandaloneProtectiveOrdersUnsupported(t *testing.T) {
	gt := newLocalGateTrader(t, newGateMock().handler)

	err := gt.SetStopLoss("DOGE_USDT", "LONG", 170, 0.48)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not supported on gate")

	err = gt.SetTakeProfit("DOGE_USDT", "LONG", 170, 0.65)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not supported on gate")
}

func TestFormatQuantityFloor(t *testing.T) {
	gt := newLocalGateTrader(t, newGateMock().handler)

	qty, err := gt.FormatQuantity("DOGE_USDT", 165)
	require.NoError(t, err)
	assert.Equal(t, "16", qty)
}

func TestContractsFromQuantityFloor(t *testing.T) {
	spec, err := NewContractSpec("DOGE_USDT", "10", "1", "1000000", "0", false)
	require.NoError(t, err)

	contracts, err := spec.ContractsFromQuantityFloor(170)
	require.NoError(t, err)
	assert.Equal(t, "17", contracts)

	// 16.99 张 → 向下 16 张（浮点噪声不得放大订单）
	contracts, err = spec.ContractsFromQuantityFloor(169.9)
	require.NoError(t, err)
	assert.Equal(t, "16", contracts)

	// "张数 × 每张币数量" 的浮点乘法噪声（运行时变量，常量会被编译器精确折叠）：
	// 容差内吸附回 17 张，向上噪声不放大、向下噪声不少算
	multiplier := 10.0
	steps := 17
	noisyUp := float64(steps) * multiplier * (1 + 1e-15)
	contracts, err = spec.ContractsFromQuantityFloor(noisyUp)
	require.NoError(t, err)
	assert.Equal(t, "17", contracts, "向上噪声吸附，不放大订单")

	noisyDown := float64(steps) * multiplier * (1 - 1e-15)
	contracts, err = spec.ContractsFromQuantityFloor(noisyDown)
	require.NoError(t, err)
	assert.Equal(t, "17", contracts, "向下噪声吸附，不少算一步")

	// 0.9 张不足一步 → 报错，不强制成 1 张
	_, err = spec.ContractsFromQuantityFloor(9)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "below one size step")

	_, err = spec.ContractsFromQuantityFloor(0)
	require.Error(t, err)
	_, err = spec.ContractsFromQuantityFloor(-170)
	require.Error(t, err)

	// 小数步长合约（ETH：0.1 张步长）：0.0017 币 = 0.17 张 → 0.1 张
	ethSpec, err := NewContractSpec("ETH_USDT", "0.01", "0.1", "10000000", "8000000", true)
	require.NoError(t, err)
	contracts, err = ethSpec.ContractsFromQuantityFloor(0.0017)
	require.NoError(t, err)
	assert.Equal(t, "0.1", contracts)
}
