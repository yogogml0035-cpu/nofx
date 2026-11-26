package kernel

import (
	"testing"
)

// TestLeverageFallback tests automatic correction when leverage exceeds the
// configured limit. Notional sizing and protection validation live in
// sizing_test.go; the cases here stay inside the loss cap so only the
// leverage behavior is under test.
func TestLeverageFallback(t *testing.T) {
	sizing := DecisionSizing{
		Principal:       1000,
		PositionPercent: 10, // notional 100 USDT, loss cap 20 USDT
		BTCETHLeverage:  10,
		AltcoinLeverage: 5,
		Prices: map[string]float64{
			"SOLUSDT": 100,
			"BTCUSDT": 100000,
			"ETHUSDT": 3500,
		},
	}

	tests := []struct {
		name            string
		decision        Decision
		wantLeverage    int // Expected leverage after correction
		wantNotionalUSD float64
		wantError       bool
	}{
		{
			name: "Altcoin leverage exceeded - auto-correct to limit",
			decision: Decision{
				Symbol: "SOLUSDT", Action: "open_long",
				Leverage: 20, StopLoss: 90, TakeProfit: 200,
			},
			wantLeverage:    5,
			wantNotionalUSD: 100,
		},
		{
			name: "BTC leverage exceeded - auto-correct to limit",
			decision: Decision{
				Symbol: "BTCUSDT", Action: "open_long",
				Leverage: 20, StopLoss: 95000, TakeProfit: 110000,
			},
			wantLeverage:    10,
			wantNotionalUSD: 100,
		},
		{
			name: "Leverage within limit - no correction",
			decision: Decision{
				Symbol: "ETHUSDT", Action: "open_short",
				Leverage: 5, StopLoss: 3700, TakeProfit: 3000,
			},
			wantLeverage:    5,
			wantNotionalUSD: 100,
		},
		{
			name: "Leverage is 0 - should error",
			decision: Decision{
				Symbol: "SOLUSDT", Action: "open_long",
				Leverage: 0, StopLoss: 90, TakeProfit: 200,
			},
			wantError: true,
		},
		{
			name: "Non-open action skips opening validation",
			decision: Decision{
				Symbol: "SOLUSDT", Action: "close_long", Leverage: 0,
			},
			wantLeverage: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDecision(&tt.decision, sizing)
			if (err != nil) != tt.wantError {
				t.Errorf("validateDecision() error = %v, wantError %v", err, tt.wantError)
				return
			}
			if !tt.wantError && tt.decision.Leverage != tt.wantLeverage {
				t.Errorf("Leverage not corrected: got %d, want %d", tt.decision.Leverage, tt.wantLeverage)
			}
			if !tt.wantError && tt.wantNotionalUSD != 0 && tt.decision.NotionalUSD != tt.wantNotionalUSD {
				t.Errorf("NotionalUSD = %v, want %v", tt.decision.NotionalUSD, tt.wantNotionalUSD)
			}
		})
	}
}
