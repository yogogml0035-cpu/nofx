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

func (s *openRefusalStub) SetMarginMode(symbol string, isCrossMargin bool) error {
	return nil
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
