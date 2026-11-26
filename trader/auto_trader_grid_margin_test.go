package trader

// 网格开跑前按 trader 的设置生效保证金模式与杠杆（D38）的单元测试。

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nofx/store"
	"nofx/trader/types"
)

// marginApplierStub 实现一次性生效能力（MarginSettingsApplier）的适配器桩
type marginApplierStub struct {
	types.Trader
	err      error
	price    float64
	calls    int
	cross    bool
	leverage int
}

func (s *marginApplierStub) GetMarketPrice(symbol string) (float64, error) {
	return s.price, nil
}

func (s *marginApplierStub) ApplyMarginSettings(symbol string, isCrossMargin bool, leverage int) error {
	s.calls++
	s.cross = isCrossMargin
	s.leverage = leverage
	return s.err
}

// legacyMarginStub 只有两个旧方法的适配器桩（其他交易所）
type legacyMarginStub struct {
	types.Trader
	modeCalls     int
	leverageCalls int
	cross         bool
	leverage      int
}

func (s *legacyMarginStub) SetMarginMode(symbol string, isCrossMargin bool) error {
	s.modeCalls++
	s.cross = isCrossMargin
	return nil
}

func (s *legacyMarginStub) SetLeverage(symbol string, leverage int) error {
	s.leverageCalls++
	s.leverage = leverage
	return nil
}

func TestGridMarginSettingsAppliedTogether(t *testing.T) {
	// 能一次性生效的适配器：模式与杠杆来自 trader 的设置，一次调用
	stub := &marginApplierStub{}
	at := &AutoTrader{config: AutoTraderConfig{IsCrossMargin: false}, trader: stub}

	require.NoError(t, at.applyMarginSettings("DOGEUSDT", 5))

	assert.Equal(t, 1, stub.calls)
	assert.False(t, stub.cross)
	assert.Equal(t, 5, stub.leverage)
}

func TestGridMarginSettingsFailureIsReturned(t *testing.T) {
	// 设置没有生效：错误上抛，网格初始化据此终止
	stub := &marginApplierStub{err: errors.New("label: LEVERAGE_EXCEEDED, message: limit [1, 75]")}
	at := &AutoTrader{config: AutoTraderConfig{}, trader: stub}

	err := at.applyMarginSettings("DOGEUSDT", 5)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "LEVERAGE_EXCEEDED")
	assert.Contains(t, err.Error(), "DOGEUSDT")
}

func TestGridMarginSettingsLegacyAdaptersUnchanged(t *testing.T) {
	// 其他适配器沿用两次调用，设置失败不阻断（行为不变）
	stub := &legacyMarginStub{}
	at := &AutoTrader{config: AutoTraderConfig{IsCrossMargin: true}, trader: stub}

	require.NoError(t, at.applyMarginSettings("DOGEUSDT", 5))

	assert.Equal(t, 1, stub.modeCalls)
	assert.True(t, stub.cross)
	assert.Equal(t, 1, stub.leverageCalls)
	assert.Equal(t, 5, stub.leverage)
}

func gridConfigForMarginTest() *store.GridStrategyConfig {
	return &store.GridStrategyConfig{
		Symbol: "DOGEUSDT", GridCount: 5, TotalInvestment: 100, Leverage: 5,
		UpperPrice: 0.6, LowerPrice: 0.5,
	}
}

func TestInitializeGridRefusesWhenMarginSettingsFail(t *testing.T) {
	// 设置没有生效的网格不得标记为已初始化
	stub := &marginApplierStub{price: 0.55, err: errors.New("label: LEVERAGE_EXCEEDED, message: limit [1, 75]")}
	at := &AutoTrader{
		config: AutoTraderConfig{IsCrossMargin: true, StrategyConfig: &store.StrategyConfig{GridConfig: gridConfigForMarginTest()}},
		trader: stub,
	}

	err := at.InitializeGrid()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "LEVERAGE_EXCEEDED")
	require.NotNil(t, at.gridState)
	assert.False(t, at.gridState.IsInitialized)
	assert.Equal(t, 1, stub.calls)
	assert.True(t, stub.cross, "模式取 trader 界面的设置")
	assert.Equal(t, 5, stub.leverage, "杠杆取网格配置")
}

func TestInitializeGridAppliesMarginSettings(t *testing.T) {
	stub := &marginApplierStub{price: 0.55}
	at := &AutoTrader{
		config: AutoTraderConfig{IsCrossMargin: false, StrategyConfig: &store.StrategyConfig{GridConfig: gridConfigForMarginTest()}},
		trader: stub,
	}

	require.NoError(t, at.InitializeGrid())

	assert.True(t, at.gridState.IsInitialized)
	assert.Equal(t, 1, stub.calls)
	assert.False(t, stub.cross)
	assert.Equal(t, 5, stub.leverage)
}
