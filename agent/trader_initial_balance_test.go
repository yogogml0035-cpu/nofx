package agent

import (
	"encoding/json"
	"log/slog"
	"path/filepath"
	"strings"
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

	balance, err := defaultTraderInitialBalanceFetcher(&store.Exchange{ExchangeType: "gate"}, "user-1")

	require.NoError(t, err)
	assert.Equal(t, 200.0, balance)
}

func TestDefaultTraderInitialBalanceFetcherFailsClosed(t *testing.T) {
	// 助手创建 trader 时一律自动读取本金：读不到、或账户本金不为正，都不能带着无效的初始资金创建。
	// 错误里带着类型化的原因，工具把它原样告诉用户
	tests := []struct {
		name    string
		stub    *initialBalanceProbeStub
		wantErr error
	}{
		{name: "exchange request fails", stub: &initialBalanceProbeStub{err: assert.AnError}, wantErr: types.ErrBalanceUnreadable},
		{name: "balance is unreadable", stub: &initialBalanceProbeStub{balance: map[string]interface{}{
			types.BalanceKeyAvailable: 150.0,
		}}, wantErr: types.ErrBalanceUnreadable},
		{name: "principal is not positive while equity is", stub: &initialBalanceProbeStub{balance: map[string]interface{}{
			types.BalanceKeyPrincipal: 0.0, types.BalanceKeyUnrealizedPnL: 30.0, types.BalanceKeyAvailable: 10.0, "totalEquity": 30.0,
		}}, wantErr: types.ErrPrincipalNotPositive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubTraderExchangeProbe(t, tt.stub)

			balance, err := defaultTraderInitialBalanceFetcher(&store.Exchange{ExchangeType: "gate"}, "user-1")

			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr, "the failure reason must be identifiable by type")
			assert.Zero(t, balance)
		})
	}
}

func TestDefaultTraderInitialBalanceFetcherWithoutExchangeConfig(t *testing.T) {
	balance, err := defaultTraderInitialBalanceFetcher(nil, "user-1")

	require.Error(t, err)
	assert.Zero(t, balance)
}

// 助手更新 trader 时换绑交易所：旧初始余额属于原来那个账户，与页面编辑走同一条规则——
// 重置为 0，加载时以新账户的本金重建；不换绑时保持原值
func TestToolUpdateTraderRebindingExchangeResetsInitialBalance(t *testing.T) {
	newEnv := func(t *testing.T) (*Agent, *store.Store, string, string) {
		t.Helper()
		st, err := store.New(filepath.Join(t.TempDir(), "trader-rebind.db"))
		if err != nil {
			t.Fatalf("create store: %v", err)
		}
		a := New(nil, st, DefaultConfig(), slog.Default())
		if err := st.AIModel().UpdateWithName("default", "default_deepseek", "DeepSeek", true, "sk-test-12345", "", "deepseek-chat"); err != nil {
			t.Fatalf("seed model: %v", err)
		}
		firstExchange, err := st.Exchange().Create("default", "binance", "First", true, "api-test", "secret-test", "", false, "", "", "", "", "", "", 0)
		if err != nil {
			t.Fatalf("seed exchange: %v", err)
		}
		secondExchange, err := st.Exchange().Create("default", "binance", "Second", true, "api-test-2", "secret-test-2", "", false, "", "", "", "", "", "", 0)
		if err != nil {
			t.Fatalf("seed exchange: %v", err)
		}
		return a, st, firstExchange, secondExchange
	}

	t.Run("rebinding resets the stored initial balance", func(t *testing.T) {
		a, st, firstExchange, secondExchange := newEnv(t)
		if err := st.Trader().Create(&store.Trader{
			ID: "trader-1", UserID: "default", Name: "demo", AIModelID: "default_deepseek",
			ExchangeID: firstExchange, InitialBalance: 200, ScanIntervalMinutes: 3,
		}); err != nil {
			t.Fatalf("seed trader: %v", err)
		}

		resp := a.toolManageTrader("default", `{"action":"update","trader_id":"trader-1","exchange_id":"`+secondExchange+`"}`)
		if strings.Contains(resp, `"error"`) {
			t.Fatalf("expected the update to succeed, got: %s", resp)
		}
		traders, err := st.Trader().List("default")
		if err != nil || len(traders) != 1 {
			t.Fatalf("list traders: %v (%d)", err, len(traders))
		}
		if traders[0].ExchangeID != secondExchange {
			t.Fatalf("expected the trader to be rebound, got exchange %q", traders[0].ExchangeID)
		}
		if traders[0].InitialBalance != 0 {
			t.Fatalf("expected the initial balance of the previous exchange to be reset, got %.2f", traders[0].InitialBalance)
		}
	})

	t.Run("updating other fields keeps the stored initial balance", func(t *testing.T) {
		a, st, firstExchange, _ := newEnv(t)
		if err := st.Trader().Create(&store.Trader{
			ID: "trader-1", UserID: "default", Name: "demo", AIModelID: "default_deepseek",
			ExchangeID: firstExchange, InitialBalance: 200, ScanIntervalMinutes: 3,
		}); err != nil {
			t.Fatalf("seed trader: %v", err)
		}

		resp := a.toolManageTrader("default", `{"action":"update","trader_id":"trader-1","exchange_id":"`+firstExchange+`","scan_interval_minutes":10}`)
		if strings.Contains(resp, `"error"`) {
			t.Fatalf("expected the update to succeed, got: %s", resp)
		}
		var result struct {
			Trader struct {
				InitialBalance      float64 `json:"initial_balance"`
				ScanIntervalMinutes int     `json:"scan_interval_minutes"`
			} `json:"trader"`
		}
		if err := json.Unmarshal([]byte(resp), &result); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if result.Trader.InitialBalance != 200 {
			t.Fatalf("expected the initial balance to be kept, got %.2f", result.Trader.InitialBalance)
		}
		if result.Trader.ScanIntervalMinutes != 10 {
			t.Fatalf("expected the scan interval to be updated, got %d", result.Trader.ScanIntervalMinutes)
		}
	})
}
