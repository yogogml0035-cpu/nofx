package kernel

// 修改止损止盈的决策校验：目标持仓必须存在且当前保护可读；新价格在现价正确一侧；
// 移动止损的亏损不超过本金 2%（D34）；动作只读取自己的价格字段。

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nofx/store"
)

func protectionSizing() DecisionSizing {
	return DecisionSizing{
		Principal:       100,
		PositionPercent: 10,
		BTCETHLeverage:  10,
		AltcoinLeverage: 5,
		Prices: map[string]float64{
			"DOGEUSDT": 0.056,
			"SOLUSDT":  1.05,
		},
		Positions: []PositionInfo{
			{
				Symbol: "DOGEUSDT", Side: "long", EntryPrice: 0.05, MarkPrice: 0.056, Quantity: 170,
				StopLoss: 0.048, TakeProfit: 0.06,
			},
			{
				// 亏损上限的精确边界：数量 10 × 距离 0.2 = 2.0 USDT = 本金 2%
				Symbol: "SOLUSDT", Side: "short", EntryPrice: 1.0, MarkPrice: 1.05, Quantity: 10,
				StopLoss: 1.1, TakeProfit: 0.9,
			},
		},
	}
}

func TestValidateUpdateStopLoss(t *testing.T) {
	sizing := protectionSizing()
	cases := []struct {
		name    string
		d       Decision
		wantErr string
	}{
		{name: "多单收紧止损", d: Decision{Symbol: "DOGEUSDT", Action: "update_stop_loss", StopLoss: 0.052}},
		{name: "多单止损移到入场价上方锁定利润（不消耗风险额度）", d: Decision{Symbol: "DOGEUSDT", Action: "update_stop_loss", StopLoss: 0.054}},
		{name: "多单止损向下移动仍在 2% 以内", d: Decision{Symbol: "DOGEUSDT", Action: "update_stop_loss", StopLoss: 0.043}},
		{name: "符号变体", d: Decision{Symbol: "DOGE/USDT", Action: "update_stop_loss", StopLoss: 0.052}},
		{
			name:    "止损高于现价",
			d:       Decision{Symbol: "DOGEUSDT", Action: "update_stop_loss", StopLoss: 0.057},
			wantErr: "must be below current price",
		},
		{
			name:    "止损等于现价",
			d:       Decision{Symbol: "DOGEUSDT", Action: "update_stop_loss", StopLoss: 0.056},
			wantErr: "must be below current price",
		},
		{
			name:    "亏损超过本金 2%",
			d:       Decision{Symbol: "DOGEUSDT", Action: "update_stop_loss", StopLoss: 0.03},
			wantErr: "exceeds 2% of principal",
		},
		{
			name:    "没有该持仓",
			d:       Decision{Symbol: "BTCUSDT", Action: "update_stop_loss", StopLoss: 0.9},
			wantErr: "has no open position",
		},
		{
			name:    "价格字段缺失",
			d:       Decision{Symbol: "DOGEUSDT", Action: "update_stop_loss", TakeProfit: 0.06},
			wantErr: "stop_loss must be greater than 0",
		},
		{
			name:    "价格非正",
			d:       Decision{Symbol: "DOGEUSDT", Action: "update_stop_loss", StopLoss: -1},
			wantErr: "stop_loss must be greater than 0",
		},
		{
			name:    "价格 NaN",
			d:       Decision{Symbol: "DOGEUSDT", Action: "update_stop_loss", StopLoss: math.NaN()},
			wantErr: "stop_loss must be greater than 0",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateDecision(&c.d, sizing)
			if c.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), c.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Zero(t, c.d.NotionalUSD, "修改保护不产生开仓名义价值")
		})
	}
}

func TestValidateUpdateTakeProfit(t *testing.T) {
	sizing := protectionSizing()
	cases := []struct {
		name    string
		d       Decision
		wantErr string
	}{
		{name: "多单止盈上调", d: Decision{Symbol: "DOGEUSDT", Action: "update_take_profit", TakeProfit: 0.065}},
		{name: "多单止盈向现价下调", d: Decision{Symbol: "DOGEUSDT", Action: "update_take_profit", TakeProfit: 0.058}},
		{
			name:    "止盈低于现价",
			d:       Decision{Symbol: "DOGEUSDT", Action: "update_take_profit", TakeProfit: 0.055},
			wantErr: "must be above current price",
		},
		{
			name:    "止盈字段缺失",
			d:       Decision{Symbol: "DOGEUSDT", Action: "update_take_profit", StopLoss: 0.05},
			wantErr: "take_profit must be greater than 0",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateDecision(&c.d, sizing)
			if c.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), c.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateUpdateProtectionShortPosition(t *testing.T) {
	sizing := protectionSizing()
	cases := []struct {
		name    string
		action  string
		sl      float64
		tp      float64
		wantErr string
	}{
		{name: "空单止损上移", action: "update_stop_loss", sl: 1.07},
		{name: "空单止损在现价下方", action: "update_stop_loss", sl: 1.04, wantErr: "must be above current price"},
		{name: "空单止损亏损超过上限", action: "update_stop_loss", sl: 1.3, wantErr: "exceeds 2% of principal"},
		{name: "空单止盈下移", action: "update_take_profit", tp: 0.95},
		{name: "空单止盈高于现价", action: "update_take_profit", tp: 1.08, wantErr: "must be below current price"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := Decision{Symbol: "SOLUSDT", Action: c.action, StopLoss: c.sl, TakeProfit: c.tp}
			err := validateDecision(&d, sizing)
			if c.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), c.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateUpdateProtectionLossCapBoundary(t *testing.T) {
	// 数量 10、入场 1.0、本金 100 → 上限 2.0 USDT：距离 0.2（止损 1.2）恰好等于上限，允许
	sizing := protectionSizing()
	equal := Decision{Symbol: "SOLUSDT", Action: "update_stop_loss", StopLoss: 1.2}
	require.NoError(t, validateDecision(&equal, sizing))

	// 高出一个最小价位即拒绝（现价 1.05，止损 1.2001 仍在现价上方）
	over := Decision{Symbol: "SOLUSDT", Action: "update_stop_loss", StopLoss: 1.2001}
	err := validateDecision(&over, sizing)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds 2% of principal")
}

func TestValidateUpdateProtectionUnreadable(t *testing.T) {
	sizing := protectionSizing()
	sizing.Positions[0].StopLoss = 0
	sizing.Positions[0].ProtectionUnavailableReason = "no open stop-loss order for this side"

	err := validateDecision(&Decision{Symbol: "DOGEUSDT", Action: "update_stop_loss", StopLoss: 0.05}, sizing)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not readable")
	assert.Contains(t, err.Error(), "no open stop-loss order")

	// 止盈同样不可读（两个都必须在）
	sizing.Positions[0].StopLoss = 0.048
	sizing.Positions[0].TakeProfit = 0
	err = validateDecision(&Decision{Symbol: "DOGEUSDT", Action: "update_take_profit", TakeProfit: 0.06}, sizing)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not readable")
}

func TestValidateUpdateProtectionMissingCurrentPrice(t *testing.T) {
	sizing := protectionSizing()
	delete(sizing.Prices, "DOGEUSDT")
	err := validateDecision(&Decision{Symbol: "DOGEUSDT", Action: "update_stop_loss", StopLoss: 0.052}, sizing)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no valid current price")
}

func TestLossAtPositionStop(t *testing.T) {
	long := PositionInfo{Symbol: "X", Side: "long", EntryPrice: 1.0, Quantity: 100}
	assert.InDelta(t, 5.0, LossAtPositionStop(long, 0.95), 1e-12)
	assert.Zero(t, LossAtPositionStop(long, 1.0), "止损在入场价上：不亏损")
	assert.Zero(t, LossAtPositionStop(long, 1.05), "止损在盈利一侧：不占用风险额度")
	assert.True(t, math.IsInf(LossAtPositionStop(long, 0), 1), "非法止损价返回 +Inf")

	short := PositionInfo{Symbol: "X", Side: "short", EntryPrice: 1.0, Quantity: 100}
	assert.InDelta(t, 5.0, LossAtPositionStop(short, 1.05), 1e-12)
	assert.Zero(t, LossAtPositionStop(short, 0.95))
}
func TestPromptMentionsProtectionUpdates(t *testing.T) {
	cases := []struct {
		lang     string
		wantText []string
	}{
		{
			lang: "en",
			wantText: []string{
				"update_stop_loss", "update_take_profit",
				"loss at the stop must not exceed 2% of the principal",
				"Update protection:",
			},
		},
		{
			lang: "zh",
			wantText: []string{
				"update_stop_loss", "update_take_profit",
				"到止损价的亏损不得超过本金的 2%",
				"修改止损止盈:",
			},
		},
	}
	for _, c := range cases {
		t.Run(c.lang, func(t *testing.T) {
			cfg := store.GetDefaultStrategyConfig(c.lang)
			engine := NewStrategyEngine(&cfg)
			prompt := engine.BuildSystemPrompt(100, "balanced")
			for _, text := range c.wantText {
				assert.Contains(t, prompt, text)
			}
		})
	}
}

func TestFormatPositionInfoShowsProtection(t *testing.T) {
	cfg := store.GetDefaultStrategyConfig("en")
	engine := NewStrategyEngine(&cfg)

	row := engine.formatPositionInfo(1, PositionInfo{
		Symbol: "DOGEUSDT", Side: "long", EntryPrice: 0.05, MarkPrice: 0.056, Quantity: 170,
		StopLoss: 0.048, TakeProfit: 0.065,
	}, &Context{})
	assert.Contains(t, row, "SL 0.048")
	assert.Contains(t, row, "TP 0.065")

	unknown := engine.formatPositionInfo(1, PositionInfo{
		Symbol: "DOGEUSDT", Side: "long", EntryPrice: 0.05, MarkPrice: 0.056, Quantity: 170,
		ProtectionUnavailableReason: "no open stop-loss order for this side",
	}, &Context{})
	assert.Contains(t, unknown, "Protection: unknown (no open stop-loss order for this side)")
}
