package kernel

import (
	"strings"
	"testing"

	"nofx/mcp"
	"nofx/store"
)

// realSizePrompt reproduces the shape of the failing cycle: 2 coins × 4
// timeframes × 300 K-lines plus three 300-value indicator arrays. The provider
// counted the real rendering at 139,981 input tokens.
func realSizePrompt() string {
	line := "11-26 07:15    60447.3000 60523.2000 60441.0000 60510.1000 7617070.04  \n"
	return strings.Repeat(line, 2400) + strings.Repeat("60634.5750, ", 7200)
}

func TestCheckPromptBudgetAcceptsDeepSeekV41Flash(t *testing.T) {
	client := mcp.NewClient(
		mcp.WithProvider(mcp.ProviderDeepSeek),
		mcp.WithModel("DeepSeek-V4.1-Flash-1017"),
		mcp.WithMaxTokens(32000),
	)
	prompt := realSizePrompt()

	got, err := CheckPromptBudget(client, "", prompt)
	if err != nil {
		t.Fatalf("real-size prompt must fit DeepSeek V4.1-Flash: %v", err)
	}
	if want := store.EstimatePromptTokens("", prompt); got.InputTokens != want {
		t.Fatalf("InputTokens = %d, want %d", got.InputTokens, want)
	}
	if got.ContextWindow != 1_000_000 {
		t.Fatalf("ContextWindow = %d, want 1_000_000", got.ContextWindow)
	}
	if got.OutputBudget != 32000 {
		t.Fatalf("OutputBudget = %d, want 32000", got.OutputBudget)
	}
	if pct := got.UsagePercent(); pct < 15 || pct > 20 {
		t.Fatalf("usage = %d%%, want 15..20%% for the measured 140K prompt + 32K output", pct)
	}
}

func TestCheckPromptBudgetRejectsRealOverflowOnSmallerModel(t *testing.T) {
	client := mcp.NewClient(
		mcp.WithProvider(mcp.ProviderQwen),
		mcp.WithModel("qwen-max"),
		mcp.WithMaxTokens(32000),
	)
	_, err := CheckPromptBudget(client, "", realSizePrompt())
	if err == nil {
		t.Fatal("expected the 140K prompt to be rejected on a 128K model")
	}
	if !strings.Contains(err.Error(), "context window 131072") {
		t.Fatalf("error must name the real context window, got: %v", err)
	}
}

func TestCheckPromptBudgetRejectsOutputBudgetAboveModelCap(t *testing.T) {
	client := mcp.NewClient(
		mcp.WithProvider(mcp.ProviderDeepSeek),
		mcp.WithModel("DeepSeek-V4.1-Flash-1017"),
		mcp.WithMaxTokens(400000),
	)
	_, err := CheckPromptBudget(client, "short", "prompt")
	if err == nil {
		t.Fatal("expected the output budget above 384K to be rejected")
	}
	if !strings.Contains(err.Error(), "max output 384000") {
		t.Fatalf("error must name the documented max output, got: %v", err)
	}
}
