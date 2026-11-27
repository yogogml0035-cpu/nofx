package provider

import (
	"testing"

	"nofx/mcp"
)

func TestClaudeReasoningEffortMapsToThinkingBudget(t *testing.T) {
	cases := []struct {
		effort string
		budget int
	}{
		{mcp.ReasoningEffortLow, 2048},
		{mcp.ReasoningEffortMedium, 4096},
		{mcp.ReasoningEffortHigh, 8192},
		{mcp.ReasoningEffortXHigh, 16384},
		{mcp.ReasoningEffortMax, 32768},
	}

	for _, tc := range cases {
		client := NewClaudeClientWithOptions(mcp.WithReasoningEffort(tc.effort)).(*ClaudeClient)
		body := client.BuildMCPRequestBody("system", "user")

		thinking, ok := body["thinking"].(map[string]any)
		if !ok {
			t.Fatalf("effort %q: thinking block missing: %#v", tc.effort, body["thinking"])
		}
		if thinking["type"] != "enabled" || thinking["budget_tokens"] != tc.budget {
			t.Fatalf("effort %q: unexpected thinking block %#v", tc.effort, thinking)
		}
		if body["temperature"] != 1 {
			t.Fatalf("effort %q: extended thinking requires temperature=1, got %v", tc.effort, body["temperature"])
		}
		maxTokens, _ := body["max_tokens"].(int)
		if maxTokens <= tc.budget {
			t.Fatalf("effort %q: max_tokens %d must leave room beyond budget %d", tc.effort, maxTokens, tc.budget)
		}
	}
}

func TestClaudeReasoningEffortOmittedByDefault(t *testing.T) {
	client := NewClaudeClientWithOptions().(*ClaudeClient)
	body := client.BuildMCPRequestBody("system", "user")
	if _, ok := body["thinking"]; ok {
		t.Fatalf("thinking must stay off without a configured effort: %#v", body["thinking"])
	}
}

func TestClaudeReasoningEffortAppliesToRequestPath(t *testing.T) {
	client := NewClaudeClientWithOptions(mcp.WithReasoningEffort(mcp.ReasoningEffortHigh)).(*ClaudeClient)
	body := client.BuildRequestBodyFromRequest(&mcp.Request{
		Model:    DefaultClaudeModel,
		Messages: []mcp.Message{mcp.NewUserMessage("hi")},
	})
	thinking, ok := body["thinking"].(map[string]any)
	if !ok || thinking["budget_tokens"] != 8192 {
		t.Fatalf("request path lost the thinking budget: %#v", body["thinking"])
	}
}
