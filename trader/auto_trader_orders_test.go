package trader

// 开仓路径 fail-closed 的单元测试：适配器未实现随单保护开仓能力（ProtectedOpener）时，
// AI 开仓直接被拒绝，不允许无保护开仓（保护不变量）。
// stub 经接口嵌入满足 Trader，但不实现 ProtectedOpener —— 与真实适配器的能力边界一致。

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nofx/kernel"
	"nofx/store"
	"nofx/trader/types"
)

// openRefusalStub 不实现 ProtectedOpener 的适配器桩
type openRefusalStub struct {
	types.Trader
}

func (s *openRefusalStub) GetPositions() ([]map[string]interface{}, error) {
	return nil, nil
}

func (s *openRefusalStub) GetBalance() (map[string]interface{}, error) {
	return map[string]interface{}{
		"availableBalance":   100.0,
		"totalWalletBalance": 100.0,
		"totalEquity":        100.0,
	}, nil
}

func newRefusalTrader() *AutoTrader {
	return &AutoTrader{
		exchange:              "binance",
		config:                AutoTraderConfig{},
		trader:                &openRefusalStub{},
		positionFirstSeenTime: map[string]int64{},
	}
}

func TestOpenRefusedWithoutProtectedOpener(t *testing.T) {
	at := newRefusalTrader()

	decision := &kernel.Decision{
		Action:          "open_long",
		Symbol:          "DOGEUSDT",
		Leverage:        10,
		PositionSizeUSD: 15,
		StopLoss:        0.48,
		TakeProfit:      0.65,
	}
	actionRecord := &store.DecisionAction{}

	err := at.executeOpenLongWithRecord(decision, actionRecord)
	require.Error(t, err, "无随单保护能力的适配器不得开仓")
	assert.Contains(t, err.Error(), "does not support protected open")
	assert.Contains(t, err.Error(), "binance")
	assert.Zero(t, actionRecord.OrderID, "被拒绝的开仓不得记录订单")

	decision.Action = "open_short"
	err = at.executeOpenShortWithRecord(decision, actionRecord)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not support protected open")
}

func TestMinOrderNotional(t *testing.T) {
	at := newRefusalTrader()

	// 无策略配置时默认 12（与 enforceMinPositionSize 同源）
	assert.InDelta(t, 12, at.minOrderNotional(), 1e-9)

	minSize := 10.0
	at.config.StrategyConfig = &store.StrategyConfig{}
	at.config.StrategyConfig.RiskControl.MinPositionSize = minSize
	assert.InDelta(t, minSize, at.minOrderNotional(), 1e-9)
}

// protectedCaptureStub 实现随单保护开仓的适配器桩：记录收到的开仓请求
type protectedCaptureStub struct {
	types.Trader
	requests        []types.ProtectedOpenRequest
	marginModeCalls int // 交易路径不应再单独调用 SetMarginMode
}

func (s *protectedCaptureStub) GetPositions() ([]map[string]interface{}, error) {
	return nil, nil
}

func (s *protectedCaptureStub) GetBalance() (map[string]interface{}, error) {
	return map[string]interface{}{
		"availableBalance":   100.0,
		"totalWalletBalance": 100.0,
		"totalEquity":        100.0,
	}, nil
}

func (s *protectedCaptureStub) SetMarginMode(symbol string, isCrossMargin bool) error {
	s.marginModeCalls++
	return nil
}

func (s *protectedCaptureStub) OpenLongProtected(req types.ProtectedOpenRequest) (types.ProtectedOpenResult, error) {
	s.requests = append(s.requests, req)
	return types.ProtectedOpenResult{OrderID: "123", Contracts: "17", Notional: 15, FillPrice: 0.5, Quantity: 170}, nil
}

func (s *protectedCaptureStub) OpenShortProtected(req types.ProtectedOpenRequest) (types.ProtectedOpenResult, error) {
	s.requests = append(s.requests, req)
	return types.ProtectedOpenResult{OrderID: "124", Contracts: "17", Notional: 15, FillPrice: 0.5, Quantity: 170}, nil
}

func (s *protectedCaptureStub) VerifyOrderProtection(symbol, orderID string) error {
	return nil
}

func TestProtectedOpenCarriesTraderMarginMode(t *testing.T) {
	// trader 界面的全仓 / 逐仓设置随开仓请求交给适配器，与杠杆一并生效；
	// 交易路径不再单独设置保证金模式（分开设置时模式不知道杠杆，F16）
	cases := []struct {
		name   string
		cross  bool
		action string
	}{
		{name: "全仓开多", cross: true, action: "open_long"},
		{name: "逐仓开多", cross: false, action: "open_long"},
		{name: "全仓开空", cross: true, action: "open_short"},
		{name: "逐仓开空", cross: false, action: "open_short"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stub := &protectedCaptureStub{}
			at := &AutoTrader{
				exchange:              "gate",
				config:                AutoTraderConfig{IsCrossMargin: c.cross},
				trader:                stub,
				positionFirstSeenTime: map[string]int64{},
			}
			// 杠杆用一个不常见的值：断言必须来自决策，而不是恰好与桩里的常量相同
			decision := &kernel.Decision{
				Action: c.action, Symbol: "DOGEUSDT", Leverage: 7,
				PositionSizeUSD: 15, StopLoss: 0.48, TakeProfit: 0.65,
			}

			var err error
			if c.action == "open_short" {
				err = at.executeOpenShortWithRecord(decision, &store.DecisionAction{})
			} else {
				err = at.executeOpenLongWithRecord(decision, &store.DecisionAction{})
			}
			require.NoError(t, err)

			require.Len(t, stub.requests, 1)
			assert.Equal(t, c.cross, stub.requests[0].IsCrossMargin, "trader 的保证金模式设置必须随开仓请求传出")
			assert.Equal(t, 7, stub.requests[0].Leverage, "杠杆取自决策")
			assert.Equal(t, "DOGEUSDT", stub.requests[0].Symbol)
			assert.Zero(t, stub.marginModeCalls, "保证金模式由适配器与杠杆一并生效，交易路径不再单独设置")
		})
	}
}
