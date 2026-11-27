package mcp

import "strings"

// Reasoning effort levels exposed by the model configuration UI and persisted
// with each AI model. They form one ordered scale:
//
//	low < medium < high < xhigh < max
//
// The value is provider-neutral on purpose. Each provider speaks its own
// dialect: DeepSeek accepts the full scale (medium/xhigh collapse to high on
// their side), OpenAI-compatible gateways stop at high, and Anthropic has no
// reasoning_effort field at all and needs a thinking budget instead.
const (
	ReasoningEffortLow    = "low"
	ReasoningEffortMedium = "medium"
	ReasoningEffortHigh   = "high"
	ReasoningEffortXHigh  = "xhigh"
	ReasoningEffortMax    = "max"
)

// NormalizeReasoningEffort canonicalizes a user-supplied level. An empty value
// means "provider default" at this layer; persistence maps it to the default
// level instead. Unknown values are rejected so a typo cannot silently change
// how much the model thinks.
func NormalizeReasoningEffort(value string) string {
	switch normalized := strings.ToLower(strings.TrimSpace(value)); normalized {
	case ReasoningEffortLow, ReasoningEffortMedium, ReasoningEffortHigh,
		ReasoningEffortXHigh, ReasoningEffortMax:
		return normalized
	default:
		return ""
	}
}

// reasoningEffortFields returns the provider-native request fields for one
// level. Providers that document only low/medium/high get deeper levels
// collapsed to high so a valid UI choice never turns into a 400.
func reasoningEffortFields(provider, effort string) map[string]any {
	normalized := NormalizeReasoningEffort(effort)
	if normalized == "" || provider == ProviderClaude {
		return nil
	}
	if provider != ProviderDeepSeek && provider != ProviderCustom {
		normalized = clampReasoningEffort(normalized, ReasoningEffortHigh)
	}
	return map[string]any{"reasoning_effort": normalized}
}

// clampReasoningEffort caps an effort level at ceiling while preserving order.
func clampReasoningEffort(effort, ceiling string) string {
	if reasoningEffortRank(effort) > reasoningEffortRank(ceiling) {
		return ceiling
	}
	return effort
}

func reasoningEffortRank(effort string) int {
	switch effort {
	case ReasoningEffortLow:
		return 1
	case ReasoningEffortMedium:
		return 2
	case ReasoningEffortHigh:
		return 3
	case ReasoningEffortXHigh:
		return 4
	case ReasoningEffortMax:
		return 5
	default:
		return 0
	}
}

// ReasoningEffortOutputFloor returns the minimum output-token budget that can
// hold both hidden reasoning and the visible answer. Reasoning tokens share
// max_tokens with the answer, so keeping the historical 8000 cap while asking
// for deeper thinking would cut the answer off (or return empty content with
// finish_reason=length). The floor only raises the cap; unused tokens are not
// billed.
func ReasoningEffortOutputFloor(effort string) int {
	switch NormalizeReasoningEffort(effort) {
	case ReasoningEffortLow:
		return 10000
	case ReasoningEffortMedium:
		return 12000
	case ReasoningEffortHigh:
		return 16000
	case ReasoningEffortXHigh:
		return 24000
	case ReasoningEffortMax:
		return 32000
	default:
		return 0
	}
}

// applyReasoningEffort writes the provider-native fields into an
// OpenAI-compatible request body. Anthropic overrides this in its own client.
func (client *Client) applyReasoningEffort(body map[string]any) {
	for key, value := range reasoningEffortFields(client.Provider, client.Cfg.ReasoningEffort) {
		body[key] = value
	}
}
