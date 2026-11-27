package kernel

import (
	"fmt"

	"nofx/mcp"
	"nofx/store"
)

// PromptBudgetCheck is the resolved token budget for one real model request.
// InputTokens is a conservative bound for the rendered prompt, OutputBudget is
// the max_tokens the client will send, and ContextWindow is shared by both.
type PromptBudgetCheck struct {
	Provider        string
	Model           string
	InputTokens     int
	OutputBudget    int
	ContextWindow   int
	MaxOutputTokens int
}

// UsagePercent is the share of the model context this request can occupy.
func (c PromptBudgetCheck) UsagePercent() int {
	if c.ContextWindow <= 0 {
		return 0
	}
	return (c.InputTokens + c.OutputBudget) * 100 / c.ContextWindow
}

// CheckPromptBudget compares the prompt that will actually be sent plus the
// configured maximum completion against the selected model's real capabilities.
// It fails at the error point instead of truncating content, dropping the
// output budget, or letting an impossible request reach the provider.
func CheckPromptBudget(client mcp.AIClient, systemPrompt, userPrompt string) (PromptBudgetCheck, error) {
	check := PromptBudgetCheck{
		InputTokens:   store.EstimatePromptTokens(systemPrompt, userPrompt),
		ContextWindow: store.GetContextLimit(""),
	}
	if embedder, ok := client.(mcp.ClientEmbedder); ok {
		base := embedder.BaseClient()
		check.Provider = base.Provider
		check.Model = base.Model
		check.OutputBudget = base.MaxTokens
		capability := store.GetModelCapabilities(base.Provider, base.Model)
		check.ContextWindow = capability.ContextWindow
		check.MaxOutputTokens = capability.MaxOutputTokens
	}

	if check.MaxOutputTokens > 0 && check.OutputBudget > check.MaxOutputTokens {
		return check, fmt.Errorf(
			"output budget %d exceeds model %s max output %d; lower AI_MAX_TOKENS or the reasoning effort",
			check.OutputBudget, check.modelLabel(), check.MaxOutputTokens)
	}
	if required := check.InputTokens + check.OutputBudget; required > check.ContextWindow {
		return check, fmt.Errorf(
			"rendered prompt estimate %d input tokens + %d output budget exceeds model %s context window %d; reduce coins, timeframes, K-line count, or output budget",
			check.InputTokens, check.OutputBudget, check.modelLabel(), check.ContextWindow)
	}
	return check, nil
}

func (c PromptBudgetCheck) modelLabel() string {
	switch {
	case c.Provider == "" && c.Model == "":
		return "unknown"
	case c.Provider == "":
		return c.Model
	case c.Model == "":
		return c.Provider
	default:
		return c.Provider + "/" + c.Model
	}
}
