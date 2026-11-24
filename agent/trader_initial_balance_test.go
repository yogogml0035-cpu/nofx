package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nofx/store"
	"nofx/trader"
	"nofx/trader/types"
)

// initialBalanceProbeStub 只实现 GetBalance 的探测适配器桩
type initialBalanceProbeStub struct {
	trader.Trader
	balance map[string]interface{}
	err     error
}

func (s *initialBalanceProbeStub) GetBalance() (map[string]interface{}, error) {
	return s.balance, s.err
}

func stubTraderExchangeProbe(t *testing.T, stub *initialBalanceProbeStub) {
	t.Helper()
	original := buildTraderExchangeProbe
	buildTraderExchangeProbe = func(*store.Exchange, string) (trader.Trader, error) {
		return stub, nil
	}
	t.Cleanup(func() { buildTraderExchangeProbe = original })
}

func TestDefaultTraderInitialBalanceFetcherTakesPrincipal(t *testing.T) {
	// 交易所报告的权益是 205.31：初始资金取本金 200
	stubTraderExchangeProbe(t, &initialBalanceProbeStub{balance: map[string]interface{}{
		types.BalanceKeyPrincipal:     200.0,
		types.BalanceKeyUnrealizedPnL: 5.31,
		types.BalanceKeyAvailable:     150.0,
		"totalEquity":                 205.31,
	}})

	balance, found, err := defaultTraderInitialBalanceFetcher(&store.Exchange{ExchangeType: "gate"}, "user-1")

	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, 200.0, balance)
}

func TestDefaultTraderInitialBalanceFetcherFailsClosed(t *testing.T) {
	tests := []struct {
		name string
		stub *initialBalanceProbeStub
	}{
		{name: "exchange request fails", stub: &initialBalanceProbeStub{err: assert.AnError}},
		{name: "balance is unreadable", stub: &initialBalanceProbeStub{balance: map[string]interface{}{
			types.BalanceKeyAvailable: 150.0,
		}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubTraderExchangeProbe(t, tt.stub)

			balance, found, err := defaultTraderInitialBalanceFetcher(&store.Exchange{ExchangeType: "gate"}, "user-1")

			require.Error(t, err)
			assert.False(t, found)
			assert.Zero(t, balance)
		})
	}
}

func TestDefaultTraderInitialBalanceFetcherWithoutExchangeConfig(t *testing.T) {
	_, found, err := defaultTraderInitialBalanceFetcher(nil, "user-1")

	require.Error(t, err)
	assert.False(t, found)
}
