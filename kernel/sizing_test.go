package kernel

// 金额计算与开仓校验的单元测试（⑨ sizing-engine）：
// 名义价值 = 本金 × position_percent（精确十进制、无浮点噪声），
// 止损止盈方向与「到止损价的亏损 ≤ 本金 2%」的边界，以及模型金额字段被忽略。

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestOpenNotionalUsesExactDecimal(t *testing.T) {
	principal := 10000.0
	percent := 1.13 // 不在 0.1 步长网格上也能精确换算

	// 同样的算式在 float64 下带噪声；函数必须给出精确的 113
	naive := principal * percent / 100
	if naive == 113 {
		t.Fatalf("runtime float math unexpectedly exact (%v); the test would not demonstrate the F33 case", naive)
	}

	got, err := openNotional(principal, percent)
	if err != nil {
		t.Fatalf("openNotional() error = %v", err)
	}
	if got != 113 {
		t.Fatalf("openNotional(%v, %v) = %v, want exactly 113", principal, percent, got)
	}

	cases := []struct {
		principal float64
		percent   float64
		want      float64
	}{
		{principal: 1000, percent: 10, want: 100},
		{principal: 107.5, percent: 10, want: 10.75},
		{principal: 100, percent: 20, want: 20},
		{principal: 1234.5, percent: 1.1, want: 13.5795},
	}
	for _, c := range cases {
		got, err := openNotional(c.principal, c.percent)
		if err != nil {
			t.Fatalf("openNotional(%v, %v) error = %v", c.principal, c.percent, err)
		}
		if got != c.want {
			t.Errorf("openNotional(%v, %v) = %v, want %v", c.principal, c.percent, got, c.want)
		}
	}
}

func TestOpenNotionalRejectsInvalidInputs(t *testing.T) {
	cases := []struct {
		name      string
		principal float64
		percent   float64
	}{
		{name: "本金为 0", principal: 0, percent: 10},
		{name: "本金为负", principal: -5, percent: 10},
		{name: "本金 NaN", principal: math.NaN(), percent: 10},
		{name: "本金 +Inf", principal: math.Inf(1), percent: 10},
		{name: "百分比为 0", principal: 1000, percent: 0},
		{name: "百分比为负", principal: 1000, percent: -1},
		{name: "百分比 NaN", principal: 1000, percent: math.NaN()},
		{name: "百分比低于下限", principal: 1000, percent: 0.5},
		{name: "百分比高于上限", principal: 1000, percent: 25},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := openNotional(c.principal, c.percent); err == nil {
				t.Fatalf("openNotional(%v, %v) accepted an invalid input", c.principal, c.percent)
			}
		})
	}
}

func sizingFor(principal, percent, price float64) DecisionSizing {
	return DecisionSizing{
		Principal:       principal,
		PositionPercent: percent,
		BTCETHLeverage:  10,
		AltcoinLeverage: 5,
		Prices:          map[string]float64{"SOLUSDT": price},
	}
}

func TestValidateDecisionProtectionDirectionAndLossCap(t *testing.T) {
	// 本金 1000、10% → 名义价值 100 USDT；2% 亏损上限 = 20 USDT。
	const price = 100.0
	sizing := sizingFor(1000, 10, price)

	cases := []struct {
		name     string
		decision Decision
		wantErr  string
	}{
		{
			name:     "多单正常",
			decision: Decision{Symbol: "SOLUSDT", Action: "open_long", Leverage: 3, StopLoss: 90, TakeProfit: 120},
		},
		{
			name:     "空单正常",
			decision: Decision{Symbol: "SOLUSDT", Action: "open_short", Leverage: 3, StopLoss: 110, TakeProfit: 80},
		},
		{
			name:     "多单亏损恰好 2% 允许",
			decision: Decision{Symbol: "SOLUSDT", Action: "open_long", Leverage: 3, StopLoss: 80, TakeProfit: 120},
		},
		{
			name:     "空单亏损恰好 2% 允许",
			decision: Decision{Symbol: "SOLUSDT", Action: "open_short", Leverage: 3, StopLoss: 120, TakeProfit: 80},
		},
		{
			name:     "多单亏损超过 2% 拒绝",
			decision: Decision{Symbol: "SOLUSDT", Action: "open_long", Leverage: 3, StopLoss: 79.9, TakeProfit: 120},
			wantErr:  "exceeds 2% of principal",
		},
		{
			name:     "空单亏损超过 2% 拒绝",
			decision: Decision{Symbol: "SOLUSDT", Action: "open_short", Leverage: 3, StopLoss: 120.1, TakeProfit: 80},
			wantErr:  "exceeds 2% of principal",
		},
		{
			name:     "多单止损在现价上方",
			decision: Decision{Symbol: "SOLUSDT", Action: "open_long", Leverage: 3, StopLoss: 101, TakeProfit: 120},
			wantErr:  "must be below current price",
		},
		{
			name:     "多单止盈在现价下方",
			decision: Decision{Symbol: "SOLUSDT", Action: "open_long", Leverage: 3, StopLoss: 90, TakeProfit: 99},
			wantErr:  "must be above current price",
		},
		{
			name:     "空单止损在现价下方",
			decision: Decision{Symbol: "SOLUSDT", Action: "open_short", Leverage: 3, StopLoss: 99, TakeProfit: 80},
			wantErr:  "must be above current price",
		},
		{
			name:     "空单止盈在现价上方",
			decision: Decision{Symbol: "SOLUSDT", Action: "open_short", Leverage: 3, StopLoss: 110, TakeProfit: 101},
			wantErr:  "must be below current price",
		},
		{
			name:     "缺失现价",
			decision: Decision{Symbol: "BTCUSDT", Action: "open_long", Leverage: 3, StopLoss: 90, TakeProfit: 120},
			wantErr:  "no valid current price",
		},
		{
			name:     "止损止盈为 0",
			decision: Decision{Symbol: "SOLUSDT", Action: "open_long", Leverage: 3, StopLoss: 0, TakeProfit: 120},
			wantErr:  "must be greater than 0",
		},
		{
			name:     "多单止损价高于止盈价",
			decision: Decision{Symbol: "SOLUSDT", Action: "open_long", Leverage: 3, StopLoss: 90, TakeProfit: 85},
			wantErr:  "stop loss price must be less than take profit price",
		},
		{
			name:     "本金为 0 拒绝开仓",
			decision: Decision{Symbol: "SOLUSDT", Action: "open_long", Leverage: 3, StopLoss: 90, TakeProfit: 120},
			wantErr:  "principal",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			decision := c.decision
			s := sizing
			if c.name == "本金为 0 拒绝开仓" {
				s = sizingFor(0, 10, price)
			}
			err := validateDecision(&decision, s)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("validateDecision() unexpected error: %v", err)
				}
				if decision.NotionalUSD != 100 {
					t.Fatalf("NotionalUSD = %v, want 100", decision.NotionalUSD)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("validateDecision() error = %v, want it to contain %q", err, c.wantErr)
			}
			if decision.NotionalUSD != 0 {
				t.Fatalf("rejected decision must not carry an open notional, got %v", decision.NotionalUSD)
			}
		})
	}
}

func TestValidateStopLossLossBoundary(t *testing.T) {
	// 名义价值 100、本金 1000 → 上限 20 USDT
	if err := ValidateStopLossLoss(100, 100, 80, 1000); err != nil {
		t.Fatalf("loss exactly 2%% must be allowed, got %v", err)
	}
	if err := ValidateStopLossLoss(100, 100, 120, 1000); err != nil {
		t.Fatalf("short-side loss exactly 2%% must be allowed, got %v", err)
	}
	if err := ValidateStopLossLoss(100, 100, 79.9, 1000); err == nil {
		t.Fatal("loss above 2%% must be rejected")
	}
	if err := ValidateStopLossLoss(100, 0, 80, 1000); err == nil {
		t.Fatal("non-positive entry price must be rejected")
	}
	if err := ValidateStopLossLoss(0, 100, 80, 1000); err == nil {
		t.Fatal("non-positive notional must be rejected")
	}
	if got, want := LossAtStop(100, 100, 80), 20.0; got != want {
		t.Fatalf("LossAtStop = %v, want %v", got, want)
	}
}

// TestValidateDecisionResolvesSymbolVariants 锁定与执行层一致的符号容错：
// 模型把候选写成 "SOL"、"sol-usdt"、"SOL/USDT" 或小写时，校验必须能取到现价，
// 不能因此拒绝整批决策（含同一批里的平仓）。
func TestValidateDecisionResolvesSymbolVariants(t *testing.T) {
	sizing := sizingFor(1000, 10, 100) // SOLUSDT 现价 100

	for _, symbol := range []string{"SOLUSDT", "SOL", "sol-usdt", "SOL/USDT", "solusdt"} {
		t.Run(symbol, func(t *testing.T) {
			decision := Decision{
				Symbol: symbol, Action: "open_long", Leverage: 3,
				StopLoss: 90, TakeProfit: 120,
			}
			if err := validateDecision(&decision, sizing); err != nil {
				t.Fatalf("validateDecision(%q) unexpected error: %v", symbol, err)
			}
			if decision.NotionalUSD != 100 {
				t.Fatalf("NotionalUSD = %v, want 100", decision.NotionalUSD)
			}
		})
	}
}

// TestValidateDecisionAcceptsNotionalBelowOldThresholds 锁定旧硬编码最小下单（12/60）
// 已删除：本金 110 × 10% = 11 USDT，山寨与 BTC 都应通过（只受策略最小下单与适配器
// 取整后校验）。
func TestValidateDecisionAcceptsNotionalBelowOldThresholds(t *testing.T) {
	sizing := DecisionSizing{
		Principal:       110,
		PositionPercent: 10,
		BTCETHLeverage:  10,
		AltcoinLeverage: 5,
		Prices:          map[string]float64{"SOLUSDT": 100, "BTCUSDT": 100000},
	}
	cases := []Decision{
		{Symbol: "SOLUSDT", Action: "open_long", Leverage: 3, StopLoss: 95, TakeProfit: 120},
		{Symbol: "BTCUSDT", Action: "open_long", Leverage: 3, StopLoss: 98000, TakeProfit: 105000},
	}
	for _, decision := range cases {
		d := decision
		if err := validateDecision(&d, sizing); err != nil {
			t.Fatalf("%s notional 11 USDT must pass the old 12/60 thresholds, got %v", d.Symbol, err)
		}
		if d.NotionalUSD != 11 {
			t.Fatalf("%s NotionalUSD = %v, want 11", d.Symbol, d.NotionalUSD)
		}
	}
}

func TestModelSuppliedAmountIsIgnored(t *testing.T) {
	// 模型即使仍输出旧金额字段，也不得影响代码计算的 NotionalUSD。
	response := `<decision>
[
  {"symbol":"BTCUSDT","action":"open_long","leverage":5,"position_size_usd":999999,"risk_usd":500,"stop_loss":95000,"take_profit":105000,"confidence":80}
]
</decision>`
	sizing := DecisionSizing{
		Principal:       1000,
		PositionPercent: 10,
		BTCETHLeverage:  10,
		AltcoinLeverage: 5,
		Prices:          map[string]float64{"BTCUSDT": 100000},
	}

	full, err := parseFullDecisionResponse(response, sizing)
	if err != nil {
		t.Fatalf("parseFullDecisionResponse() error = %v", err)
	}
	if len(full.Decisions) != 1 {
		t.Fatalf("decisions = %d, want 1", len(full.Decisions))
	}
	if got := full.Decisions[0].NotionalUSD; got != 100 {
		t.Fatalf("NotionalUSD = %v, want the code-computed 100", got)
	}

	encoded, err := json.Marshal(full.Decisions)
	if err != nil {
		t.Fatalf("marshal decisions: %v", err)
	}
	for _, field := range []string{"position_size_usd", "risk_usd"} {
		if strings.Contains(string(encoded), field) {
			t.Fatalf("serialized decision still contains %q: %s", field, encoded)
		}
	}
}
