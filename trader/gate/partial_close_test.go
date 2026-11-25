package gate

// 分批平仓适配层的单元测试：httptest 按 Gate 线上响应格式返回合约元数据、持仓与
// 成交委托（后端质量规范：适配层测试用真实响应格式；真实接口检查放在任务 probe/）。

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nofx/trader/types"
)

func TestPartialCloseStatusMatrix(t *testing.T) {
	cases := []struct {
		name       string
		contract   string
		size       string
		side       string
		wantOK     bool
		wantStep   string
		wantReason string
		wantErr    bool
	}{
		{
			name: "DOGE 17 contracts can be split", contract: "DOGE_USDT", size: "17",
			side: "long", wantOK: true, wantStep: "1",
		},
		{
			name: "DOGE one contract is a single step", contract: "DOGE_USDT", size: "1",
			side: "long", wantOK: false, wantStep: "1", wantReason: "single size step",
		},
		{
			name: "ETH 0.2 contracts can be split", contract: "ETH_USDT", size: "0.2",
			side: "long", wantOK: true, wantStep: "0.1",
		},
		{
			name: "ETH 0.1 contracts is a single step", contract: "ETH_USDT", size: "0.1",
			side: "long", wantOK: false, wantStep: "0.1", wantReason: "single size step",
		},
		{
			name: "position size must be a step multiple", contract: "DOGE_USDT", size: "17.5",
			side: "long", wantOK: false, wantStep: "1", wantReason: "not a positive multiple",
		},
		{
			name: "opposite side is not a position", contract: "DOGE_USDT", size: "17",
			side: "short", wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := newGateMock()
			gt := newLocalGateTrader(t, mock.handler)
			mock.mu.Lock()
			mock.positions = []map[string]interface{}{
				{"contract": tc.contract, "size": tc.size, "entry_price": "0.5"},
			}
			mock.mu.Unlock()

			status, err := gt.PartialCloseStatus(tc.contract, tc.side)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantOK, status.Supported)
			assert.Equal(t, tc.wantStep, status.SizeStep)
			if tc.wantReason != "" {
				assert.Contains(t, status.Reason, tc.wantReason)
			} else {
				assert.Empty(t, status.Reason)
			}
		})
	}
}

func TestPartialCloseStatusWithoutPositionFails(t *testing.T) {
	mock := newGateMock()
	gt := newLocalGateTrader(t, mock.handler)

	_, err := gt.PartialCloseStatus("DOGE_USDT", "long")
	require.Error(t, err, "没有持仓时不能报告可分批")
	assert.Contains(t, err.Error(), "long position not found")
}

func TestClosePartialLongPayloadAndResult(t *testing.T) {
	mock := newGateMock()
	gt := newLocalGateTrader(t, mock.handler)
	mock.mu.Lock()
	mock.positions = []map[string]interface{}{
		{"contract": "DOGE_USDT", "size": "17", "entry_price": "0.55"},
	}
	mock.mu.Unlock()

	result, err := gt.ClosePartial(types.PartialCloseRequest{
		Symbol: "DOGE_USDT", Side: "long", Percent: 50,
	})
	require.NoError(t, err)

	body := mock.lastOrderBody(t)
	assert.Equal(t, "-8", body["size"], "17 张 × 50% 向下取整为 8 张，平多卖出为负")
	assert.Equal(t, true, body["reduce_only"])
	assert.Equal(t, "0", body["price"], "市价单")
	assert.Equal(t, "ioc", body["tif"])
	assert.Equal(t, "t-nofx-partial-close", body["text"])
	_, hasClose := body["close"]
	assert.False(t, hasClose, "分批平仓不是全平委托")

	assert.Equal(t, "99123", result.OrderID)
	assert.Equal(t, "8", result.Contracts)
	assert.Equal(t, "9", result.Remaining)
	assert.InDelta(t, 0.56, result.FillPrice, 1e-9)
	assert.InDelta(t, 80, result.Quantity, 1e-9, "8 张 × 10 币/张")
	assert.InDelta(t, 80*0.56, result.Notional, 1e-9)
}

func TestClosePartialShortPayload(t *testing.T) {
	mock := newGateMock()
	gt := newLocalGateTrader(t, mock.handler)
	mock.mu.Lock()
	mock.positions = []map[string]interface{}{
		{"contract": "DOGE_USDT", "size": "-17", "entry_price": "0.55"},
	}
	mock.mu.Unlock()

	_, err := gt.ClosePartial(types.PartialCloseRequest{
		Symbol: "DOGE_USDT", Side: "SHORT", Percent: 50,
	})
	require.NoError(t, err)
	assert.Equal(t, "8", mock.lastOrderBody(t)["size"], "平空买入为正")
}

func TestClosePartialRejections(t *testing.T) {
	cases := []struct {
		name    string
		size    string
		percent float64
		want    string
	}{
		{name: "100 percent is a full close", size: "17", percent: 100, want: "full close"},
		{name: "zero percent", size: "17", percent: 0, want: "must be in (0, 100)"},
		{name: "negative percent", size: "17", percent: -5, want: "must be in (0, 100)"},
		{name: "over 100 percent", size: "17", percent: 101, want: "must be in (0, 100)"},
		{name: "NaN percent", size: "17", percent: math.NaN(), want: "not a finite number"},
		{name: "ratio floors below one step", size: "17", percent: 1, want: "below one size step"},
		{name: "two step position with tiny ratio", size: "0.2", percent: 1, want: "below one size step"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := newGateMock()
			gt := newLocalGateTrader(t, mock.handler)
			contract := "DOGE_USDT"
			if tc.size == "0.2" {
				contract = "ETH_USDT"
			}
			mock.mu.Lock()
			mock.positions = []map[string]interface{}{
				{"contract": contract, "size": tc.size, "entry_price": "0.5"},
			}
			mock.mu.Unlock()

			_, err := gt.ClosePartial(types.PartialCloseRequest{
				Symbol: contract, Side: "long", Percent: tc.percent,
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			mock.mu.Lock()
			assert.Empty(t, mock.orderBodies, "被拒绝的分批平仓不得提交订单")
			mock.mu.Unlock()
		})
	}
}

func TestClosePartialRejectsPositionNotOnStep(t *testing.T) {
	mock := newGateMock()
	gt := newLocalGateTrader(t, mock.handler)
	mock.mu.Lock()
	mock.positions = []map[string]interface{}{
		{"contract": "DOGE_USDT", "size": "17.5"},
	}
	mock.mu.Unlock()

	_, err := gt.ClosePartial(types.PartialCloseRequest{
		Symbol: "DOGE_USDT", Side: "long", Percent: 50,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a positive multiple")
	mock.mu.Lock()
	assert.Empty(t, mock.orderBodies)
	mock.mu.Unlock()
}

func TestClosePartialRespectsMarketOrderMax(t *testing.T) {
	mock := newGateMock()
	mock.contractOverride = map[string]map[string]interface{}{
		"ETH_USDT": {
			"name": "ETH_USDT", "quanto_multiplier": "0.01", "order_size_min": "0.1",
			"order_size_max": "10000000", "market_order_size_max": "0.2", "enable_decimal": true,
			"order_price_round": "0.01",
		},
	}
	gt := newLocalGateTrader(t, mock.handler)
	mock.mu.Lock()
	mock.positions = []map[string]interface{}{
		{"contract": "ETH_USDT", "size": "1", "entry_price": "3000"},
	}
	mock.mu.Unlock()

	_, err := gt.ClosePartial(types.PartialCloseRequest{
		Symbol: "ETH_USDT", Side: "long", Percent: 50, // 0.5 张 > 市价单上限 0.2 张
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceed the market order maximum")
	mock.mu.Lock()
	assert.Empty(t, mock.orderBodies)
	mock.mu.Unlock()
}

func TestClosePartialConversionMatrix(t *testing.T) {
	// PRD 验收矩阵中的精确换算用例（端到端走 ClosePartial：读持仓 → 换算 → 下单 → 结果）
	cases := []struct {
		name         string
		contract     string
		size         string
		percent      float64
		wantClose    string
		wantRemain   string
		wantBodySize string
	}{
		{
			name:     "17 contracts at 6 percent floors to 1 and leaves 16",
			contract: "DOGE_USDT", size: "17", percent: 6,
			wantClose: "1", wantRemain: "16", wantBodySize: "-1",
		},
		{
			name:     "0.2 ETH at 50 percent closes 0.1 and leaves 0.1",
			contract: "ETH_USDT", size: "0.2", percent: 50,
			wantClose: "0.1", wantRemain: "0.1", wantBodySize: "-0.1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := newGateMock()
			gt := newLocalGateTrader(t, mock.handler)
			mock.mu.Lock()
			mock.positions = []map[string]interface{}{
				{"contract": tc.contract, "size": tc.size, "entry_price": "0.5"},
			}
			mock.mu.Unlock()

			result, err := gt.ClosePartial(types.PartialCloseRequest{
				Symbol: tc.contract, Side: "long", Percent: tc.percent,
			})
			require.NoError(t, err)
			assert.Equal(t, tc.wantClose, result.Contracts)
			assert.Equal(t, tc.wantRemain, result.Remaining)
			assert.Equal(t, tc.wantBodySize, mock.lastOrderBody(t)["size"])
		})
	}
}

func TestPartialCloseStatusContractErrorIsAnError(t *testing.T) {
	mock := newGateMock()
	mock.contractOverride = map[string]map[string]interface{}{
		"DOGE_USDT": {
			"name": "DOGE_USDT", "quanto_multiplier": "10", "order_size_min": "0",
			"order_size_max": "1000000", "market_order_size_max": "0", "enable_decimal": false,
		},
	}
	gt := newLocalGateTrader(t, mock.handler)
	mock.mu.Lock()
	mock.positions = []map[string]interface{}{
		{"contract": "DOGE_USDT", "size": "17"},
	}
	mock.mu.Unlock()

	_, err := gt.PartialCloseStatus("DOGE_USDT", "long")
	require.Error(t, err, "读不到有效步长时不能报告可分批")
	assert.Contains(t, err.Error(), "order_size_min")
}
