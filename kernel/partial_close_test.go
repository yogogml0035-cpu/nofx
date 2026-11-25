package kernel

// 分批平仓契约测试：动作与比例校验、JSON 解析、提示词动作清单与持仓可分批标记。

import (
	"math"
	"strings"
	"testing"

	"nofx/store"
)

func TestPartialCloseDecisionValidation(t *testing.T) {
	cases := []struct {
		name    string
		d       Decision
		wantErr string
	}{
		{name: "long partial 50", d: Decision{Symbol: "DOGE_USDT", Action: "close_long_partial", ClosePercent: 50}},
		{name: "short partial 0.1", d: Decision{Symbol: "DOGE_USDT", Action: "close_short_partial", ClosePercent: 0.1}},
		{
			name: "100 percent must use full close", d: Decision{Symbol: "DOGE_USDT", Action: "close_long_partial", ClosePercent: 100},
			wantErr: "full close",
		},
		{name: "zero percent", d: Decision{Symbol: "DOGE_USDT", Action: "close_long_partial", ClosePercent: 0}, wantErr: "must be in (0, 100)"},
		{name: "negative percent", d: Decision{Symbol: "DOGE_USDT", Action: "close_short_partial", ClosePercent: -1}, wantErr: "must be in (0, 100)"},
		{name: "over 100 percent", d: Decision{Symbol: "DOGE_USDT", Action: "close_long_partial", ClosePercent: 100.1}, wantErr: "must be in (0, 100)"},
		{name: "NaN percent", d: Decision{Symbol: "DOGE_USDT", Action: "close_long_partial", ClosePercent: math.NaN()}, wantErr: "finite number"},
		{name: "Inf percent", d: Decision{Symbol: "DOGE_USDT", Action: "close_long_partial", ClosePercent: math.Inf(1)}, wantErr: "finite number"},
		{
			name: "same payload as JSON with 100", d: Decision{Symbol: "DOGE_USDT", Action: "close_short_partial", ClosePercent: 100},
			wantErr: "close_short",
		},
		{name: "full close stays valid", d: Decision{Symbol: "DOGE_USDT", Action: "close_long"}},
		{name: "full close ignores close_percent", d: Decision{Symbol: "DOGE_USDT", Action: "close_short", ClosePercent: 50}},
		{name: "unknown action still invalid", d: Decision{Symbol: "DOGE_USDT", Action: "partial_close"}, wantErr: "invalid action"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateDecision(&tc.d, DecisionSizing{})
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("validateDecision() error = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateDecision() error = nil, want %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %q, want it to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestPartialCloseDecisionParsesJSON(t *testing.T) {
	response := `<decision>
[
  {"symbol":"DOGE_USDT","action":"close_long_partial","close_percent":50},
  {"symbol":"DOGE_USDT","action":"close_short_partial","close_percent":25}
]
</decision>`

	full, err := parseFullDecisionResponse(response, DecisionSizing{})
	if err != nil {
		t.Fatalf("parseFullDecisionResponse() error = %v", err)
	}
	if len(full.Decisions) != 2 {
		t.Fatalf("decisions = %d, want 2", len(full.Decisions))
	}
	if full.Decisions[0].Action != "close_long_partial" || full.Decisions[0].ClosePercent != 50 {
		t.Fatalf("first decision = %+v, want close_long_partial 50", full.Decisions[0])
	}
	if full.Decisions[1].Action != "close_short_partial" || full.Decisions[1].ClosePercent != 25 {
		t.Fatalf("second decision = %+v, want close_short_partial 25", full.Decisions[1])
	}
}

func TestPartialCloseDecisionParseRejectsFullPercent(t *testing.T) {
	response := `<decision>
[
  {"symbol":"DOGE_USDT","action":"close_long_partial","close_percent":100}
]
</decision>`

	_, err := parseFullDecisionResponse(response, DecisionSizing{})
	if err == nil {
		t.Fatal("close_percent 100 must be refused at parse time")
	}
	if !strings.Contains(err.Error(), "full close") {
		t.Fatalf("error = %q, want a full-close hint", err.Error())
	}
}

func TestPromptDocumentsPartialClose(t *testing.T) {
	cases := []struct {
		lang        string
		wantAction  string
		wantPercent string
		wantFull    string
	}{
		{lang: "zh", wantAction: "close_long_partial", wantPercent: "`close_percent`", wantFull: "100% 是全平"},
		{lang: "en", wantAction: "close_short_partial", wantPercent: "`close_percent`", wantFull: "100% is a full close"},
	}

	for _, tc := range cases {
		t.Run(tc.lang, func(t *testing.T) {
			cfg := store.GetDefaultStrategyConfig(tc.lang)
			engine := NewStrategyEngine(&cfg)
			prompt := engine.BuildSystemPrompt(1000, "balanced")

			if !strings.Contains(prompt, tc.wantAction) {
				t.Fatalf("system prompt does not list %q", tc.wantAction)
			}
			if !strings.Contains(prompt, tc.wantPercent) {
				t.Fatalf("system prompt does not document close_percent")
			}
			if !strings.Contains(prompt, tc.wantFull) {
				t.Fatalf("system prompt does not document the 100%% full-close rule")
			}
			if !strings.Contains(prompt, "Partial Close: available") {
				t.Fatalf("system prompt does not reference the position partial-close marker")
			}
			if !strings.Contains(prompt, "\"close_percent\": 50") {
				t.Fatalf("system prompt does not include a partial-close format example")
			}
		})
	}
}

func TestUserPromptMarksPartialCloseAvailability(t *testing.T) {
	cfg := store.GetDefaultStrategyConfig("en")
	engine := NewStrategyEngine(&cfg)
	ctx := &Context{
		CurrentTime: "2026-11-25 12:00",
		Account: AccountInfo{
			TotalEquity:      100,
			AvailableBalance: 90,
			PositionCount:    2,
		},
		Positions: []PositionInfo{
			{
				Symbol: "DOGE_USDT", Side: "long", EntryPrice: 0.05, MarkPrice: 0.055,
				Quantity: 170, Leverage: 75, CanPartialClose: true,
			},
			{
				Symbol: "ETH_USDT", Side: "long", EntryPrice: 3000, MarkPrice: 3100,
				Quantity: 0.01, Leverage: 10,
				CanPartialClose:               false,
				PartialCloseUnavailableReason: "position is a single size step",
			},
		},
	}

	prompt := engine.BuildUserPrompt(ctx)
	if !strings.Contains(prompt, "Partial Close: available") {
		t.Fatalf("DOGE position is not marked available:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Partial Close: unavailable (position is a single size step)") {
		t.Fatalf("ETH position is not marked unavailable with the reason:\n%s", prompt)
	}
	if !strings.Contains(prompt, "close_long_partial / close_short_partial") {
		t.Fatalf("user prompt does not explain how to use partial close:\n%s", prompt)
	}
}
