package agent

import (
	"strings"
	"testing"
)

func TestModelProviderChoicePromptListsAPIKeyProviders(t *testing.T) {
	msg := modelProviderChoicePrompt("zh")
	for _, want := range []string{
		"可选模型 provider",
		"DeepSeek",
		"OpenAI",
		"API Key",
		"请先告诉我你想用哪个 provider",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected prompt to contain %q, got: %s", want, msg)
		}
	}
	if strings.Contains(msg, "把私钥发给我") {
		t.Fatalf("provider choice prompt should not jump ahead to credential collection: %s", msg)
	}
}

func TestModelProviderCredentialGuidanceForAPIKeyProvider(t *testing.T) {
	msg := modelProviderCredentialGuidance("zh", "deepseek")
	if !strings.Contains(msg, "API Key") {
		t.Fatalf("expected guidance to mention API Key, got: %s", msg)
	}
}

func TestModelProviderDetailedGuidanceForAPIKeyProvider(t *testing.T) {
	msg := modelProviderDetailedGuidance("zh", "deepseek")
	for _, want := range []string{
		"DeepSeek",
		"deepseek-chat",
		"API Key",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected detailed guidance to contain %q, got: %s", want, msg)
		}
	}
}

func TestSupportedModelProvidersAreAPIKeyOnly(t *testing.T) {
	ids := supportedModelProviderIDs()
	expected := []string{"deepseek", "qwen", "openai", "claude", "gemini", "grok", "kimi", "minimax"}
	if len(ids) != len(expected) {
		t.Fatalf("provider count = %d, want %d: %v", len(ids), len(expected), ids)
	}
	for _, want := range expected {
		found := false
		for _, id := range ids {
			if id == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected provider %q in %v", want, ids)
		}
	}
}
