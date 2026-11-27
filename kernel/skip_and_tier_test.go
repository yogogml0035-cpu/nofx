package kernel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nofx/market"
)

// 没有行情也没有仓位时不调用模型；有仓位时仍然调用（平仓不依赖候选币行情）
func TestDecisionSkippable(t *testing.T) {
	assert.True(t, decisionSkippable(&Context{}))
	assert.False(t, decisionSkippable(&Context{MarketDataMap: map[string]*market.Data{"BTCUSDT": {}}}))
	assert.False(t, decisionSkippable(&Context{Positions: []PositionInfo{{Symbol: "BTCUSDT"}}}))
}

// 杠杆档位按符号的基础币种判断：BTC/USDT、BTC-USDT、BTC 都落在 BTC/ETH 档
func TestLeverageTierMatchesSymbolVariants(t *testing.T) {
	for _, tc := range []struct {
		symbol string
		want   int
	}{
		{"BTCUSDT", 20}, {"BTC/USDT", 20}, {"BTC-USDT", 20}, {"BTC_USDT", 20},
		{"BTC-USDT-SWAP", 20}, {"BTC", 20}, {"btcusdt", 20},
		{"ETH_USDT", 20}, {"ETH", 20},
		{"SOLUSDT", 5}, {"DOGE_USDT", 5}, {"BTCUSDC", 20}, // 计价币不同仍是 BTC
		{"XSWAPUSDT", 5}, // 基础币种以 SWAP 结尾不能被当成衍生品后缀剥掉
	} {
		t.Run(tc.symbol, func(t *testing.T) {
			d := &Decision{Symbol: tc.symbol, Action: "open_long", Leverage: 50, StopLoss: 90, TakeProfit: 150}
			sizing := DecisionSizing{Principal: 1000, PositionPercent: 10, BTCETHLeverage: 20, AltcoinLeverage: 5,
				Prices: map[string]float64{tc.symbol: 100}}
			require.NoError(t, validateDecision(d, sizing))
			assert.Equal(t, tc.want, d.Leverage, "leverage above the tier max falls back to the tier max")
		})
	}
}
