package agent

// 助手开仓的风险说明与 Gate 优先符号路由的接线测试（对抗审查视角 D）。

import (
	"errors"
	"strings"
	"testing"

	"nofx/market"
	"nofx/store"
)

// 有加密 trader 时按 Gate 合约清单判定；清单不可用时回落启发式；只有股票 trader 时不查 Gate
func TestResolveStockSymbolUsesGateListWithCryptoTraders(t *testing.T) {
	previousExchanges, previousTickers := configuredTraderExchanges, assistantListTickers
	t.Cleanup(func() { configuredTraderExchanges, assistantListTickers = previousExchanges, previousTickers })

	listCalls := 0
	configuredTraderExchanges = func(*Agent) []string { return []string{"gate"} }
	assistantListTickers = func() ([]market.GateTicker, error) {
		listCalls++
		return []market.GateTicker{{Symbol: "HYPEUSDT"}, {Symbol: "BTCUSDT"}}, nil
	}
	a := &Agent{}
	if a.resolveStockSymbol("HYPE") {
		t.Fatal("HYPE is a listed Gate contract even though it is missing from the hardcoded list")
	}
	if !a.resolveStockSymbol("AAPL") {
		t.Fatal("AAPL is not a Gate contract and must stay a stock")
	}
	if listCalls != 2 {
		t.Fatalf("gate ticker list queried %d times, want 2", listCalls)
	}

	assistantListTickers = func() ([]market.GateTicker, error) { return nil, errors.New("gate down") }
	if !a.resolveStockSymbol("HYPE") {
		t.Fatal("when the Gate list is unavailable the heuristic must apply (HYPE looks like a stock)")
	}

	configuredTraderExchanges = func(*Agent) []string { return []string{"alpaca"} }
	listCalls = 0
	assistantListTickers = func() ([]market.GateTicker, error) { listCalls++; return nil, nil }
	if !a.resolveStockSymbol("HYPE") {
		t.Fatal("stock-only setups must not route through Gate")
	}
	if listCalls != 0 {
		t.Fatal("the Gate ticker list must not be queried when no crypto trader is configured")
	}
}

// 助手开仓确认必须带风险说明（不带止损止盈、不由 trader 管理）；平仓不带
func TestFormatTradeConfirmationRiskNotice(t *testing.T) {
	for _, tc := range []struct {
		lang     string
		openWord string
	}{
		{"zh", "不附带止损止盈"},
		{"en", "no stop loss or take profit"},
	} {
		open := &TradeAction{ID: "trade_1", Action: "open_long", Symbol: "BTCUSDT", Quantity: 0.01, EstimatedNotional: 500}
		msg := formatTradeConfirmation(open, tc.lang)
		if !strings.Contains(msg, tc.openWord) {
			t.Fatalf("%s open confirmation must carry the risk notice: %s", tc.lang, msg)
		}
		if !strings.Contains(msg, "确认 trade_1") && !strings.Contains(msg, "confirm trade_1") {
			t.Fatalf("%s confirmation must still carry the confirmation command: %s", tc.lang, msg)
		}
		closeTrade := &TradeAction{ID: "trade_2", Action: "close_long", Symbol: "BTCUSDT"}
		if msg := formatTradeConfirmation(closeTrade, tc.lang); strings.Contains(msg, tc.openWord) {
			t.Fatalf("%s close confirmation must not carry the open risk notice: %s", tc.lang, msg)
		}
	}
}

// manualTradeTraderBase 手动下单校验用得到的适配器方法
type manualTradeTraderBase struct{ price float64 }

func (s *manualTradeTraderBase) OpenLong(string, float64, int) (map[string]interface{}, error) {
	return nil, nil
}
func (s *manualTradeTraderBase) OpenShort(string, float64, int) (map[string]interface{}, error) {
	return nil, nil
}
func (s *manualTradeTraderBase) CloseLong(string, float64) (map[string]interface{}, error) {
	return nil, nil
}
func (s *manualTradeTraderBase) CloseShort(string, float64) (map[string]interface{}, error) {
	return nil, nil
}
func (s *manualTradeTraderBase) GetMarketPrice(string) (float64, error) { return s.price, nil }

// manualTradeSizeCheckingTrader 带 OrderSizeChecker 能力的适配器（如 Gate）
type manualTradeSizeCheckingTrader struct {
	manualTradeTraderBase
	checkErr   error
	checkCalls int
}

func (s *manualTradeSizeCheckingTrader) CheckOrderSize(string, float64) error {
	s.checkCalls++
	return s.checkErr
}

type manualTradeSelectedTrader struct{}

func (manualTradeSelectedTrader) GetStrategyConfig() *store.StrategyConfig { return nil }

func (manualTradeSelectedTrader) GetAccountInfo() (map[string]interface{}, error) {
	return map[string]interface{}{"total_equity": 1000.0}, nil
}

// 手动下单没有金额下限：名义价值很小的订单只要够合约最小张数就通过校验
func TestValidateTradeActionHasNoAmountFloor(t *testing.T) {
	trader := &manualTradeSizeCheckingTrader{manualTradeTraderBase: manualTradeTraderBase{price: 59635.2}}
	trade := &TradeAction{Action: "open_short", Symbol: "BTCUSDT", Quantity: 0.0001, Leverage: 5} // 一张 ≈ 5.96 USDT

	if err := validateTradeAction(trade, false, manualTradeSelectedTrader{}, trader); err != nil {
		t.Fatalf("a one-contract order must pass validation, got: %v", err)
	}
	if trader.checkCalls != 1 {
		t.Fatalf("order size checked %d times, want 1", trader.checkCalls)
	}
	if trade.EstimatedNotional < 5.9 || trade.EstimatedNotional > 6.0 {
		t.Fatalf("estimated notional = %v, want about 5.96", trade.EstimatedNotional)
	}
}

// 数量不足合约最小张数：创建待确认订单前就被拒绝，而不是确认之后才在下单时被挡下
func TestValidateTradeActionRejectsBelowContractStep(t *testing.T) {
	trader := &manualTradeSizeCheckingTrader{
		manualTradeTraderBase: manualTradeTraderBase{price: 59635.2},
		checkErr:              errors.New("open BTC_USDT skipped: quantity 0.00005 is below one size step (1 contracts)"),
	}
	trade := &TradeAction{Action: "open_long", Symbol: "BTCUSDT", Quantity: 0.00005, Leverage: 5}

	err := validateTradeAction(trade, false, manualTradeSelectedTrader{}, trader)
	if err == nil || !strings.Contains(err.Error(), "below one size step") {
		t.Fatalf("expected the below-step quantity to be refused, got: %v", err)
	}
}

// 适配器没有前置校验能力时不校验（下单路径仍会按最小步长换算）
func TestValidateTradeActionToleratesTraderWithoutSizeCheck(t *testing.T) {
	trader := &manualTradeTraderBase{price: 0.56}
	trade := &TradeAction{Action: "open_long", Symbol: "DOGEUSDT", Quantity: 5, Leverage: 5}

	if err := validateTradeAction(trade, false, nil, trader); err != nil {
		t.Fatalf("validation must not require the optional size check, got: %v", err)
	}
}
