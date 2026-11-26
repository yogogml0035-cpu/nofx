package kernel

// 系统提示词的金额契约（⑨ sizing-engine）：中英文都不再向模型索取金额，
// 开仓名义价值按本金 × 百分比展示并声明由代码强制；输出示例可直接解析。

import (
	"encoding/json"
	"strings"
	"testing"

	"nofx/store"
)

func TestSystemPromptUsesPrincipalSizingWithoutAmountFields(t *testing.T) {
	cases := []struct {
		name            string
		lang            string
		wantSizingLine  string
		wantLossCapLine string
	}{
		{
			name:            "中文",
			lang:            "zh",
			wantSizingLine:  "本金 1000.00 × 10.0%",
			wantLossCapLine: "本金的 2%",
		},
		{
			name:            "英文",
			lang:            "en",
			wantSizingLine:  "principal 1000.00 × 10.0%",
			wantLossCapLine: "Loss at Stop Loss: ≤ 2% of principal",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := store.GetDefaultStrategyConfig(c.lang)
			engine := NewStrategyEngine(&cfg)

			prompt := engine.BuildSystemPrompt(1000, "balanced")

			for _, forbidden := range []string{"position_size_usd", "risk_usd"} {
				if strings.Contains(prompt, forbidden) {
					t.Fatalf("system prompt still asks for %q", forbidden)
				}
			}
			if !strings.Contains(prompt, c.wantSizingLine) {
				t.Fatalf("system prompt does not show the principal-based notional (%q)", c.wantSizingLine)
			}
			if !strings.Contains(prompt, c.wantLossCapLine) {
				t.Fatalf("system prompt does not state the loss cap (%q)", c.wantLossCapLine)
			}
		})
	}
}

func TestOutputFormatExampleParsesWithoutAmountFields(t *testing.T) {
	cfg := store.GetDefaultStrategyConfig("zh")
	// 单标策略：示例使用实际标的，并且必须是合法 JSON
	cfg.CoinSource.StaticCoins = []string{"SOLUSDT"}
	engine := NewStrategyEngine(&cfg)

	prompt := engine.BuildSystemPrompt(1000, "balanced")
	example := jsonExampleFromPrompt(t, prompt)

	var decisions []Decision
	if err := json.Unmarshal([]byte(example), &decisions); err != nil {
		t.Fatalf("output example is not a valid decision array: %v\n%s", err, example)
	}
	if len(decisions) == 0 {
		t.Fatal("output example has no decisions")
	}
	if decisions[0].Symbol != "SOLUSDT" {
		t.Fatalf("output example symbol = %q, want SOLUSDT", decisions[0].Symbol)
	}
	encoded, err := json.Marshal(decisions)
	if err != nil {
		t.Fatalf("marshal example decisions: %v", err)
	}
	for _, field := range []string{"position_size_usd", "risk_usd"} {
		if strings.Contains(string(encoded), field) {
			t.Fatalf("output example still contains %q: %s", field, encoded)
		}
	}
}

// jsonExampleFromPrompt 取出系统提示词里 ```json 代码块的内容
func jsonExampleFromPrompt(t *testing.T, prompt string) string {
	t.Helper()
	const fence = "```json"
	start := strings.Index(prompt, fence)
	if start < 0 {
		t.Fatal("system prompt has no ```json block")
	}
	rest := prompt[start+len(fence):]
	end := strings.Index(rest, "```")
	if end < 0 {
		t.Fatal("system prompt json block is not closed")
	}
	return strings.TrimSpace(rest[:end])
}
