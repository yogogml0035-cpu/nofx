package agent

import (
	"fmt"
	"strings"
)

type modelProviderSpec struct {
	ID                    string
	DisplayName           string
	DefaultModel          string
	CredentialLabelZH     string
	CredentialLabelEN     string
	SupportsCustomAPIURL  bool
	SupportsCustomModel   bool
}

func supportedModelProviders() []modelProviderSpec {
	return []modelProviderSpec{
		{ID: "deepseek", DisplayName: "DeepSeek", DefaultModel: "deepseek-chat", CredentialLabelZH: "API Key", CredentialLabelEN: "API key", SupportsCustomAPIURL: true, SupportsCustomModel: true},
		{ID: "qwen", DisplayName: "Qwen", DefaultModel: "qwen3-max", CredentialLabelZH: "API Key", CredentialLabelEN: "API key", SupportsCustomAPIURL: true, SupportsCustomModel: true},
		{ID: "openai", DisplayName: "OpenAI", DefaultModel: "gpt-5.1", CredentialLabelZH: "API Key", CredentialLabelEN: "API key", SupportsCustomAPIURL: true, SupportsCustomModel: true},
		{ID: "claude", DisplayName: "Claude", DefaultModel: "claude-opus-4-6", CredentialLabelZH: "API Key", CredentialLabelEN: "API key", SupportsCustomAPIURL: true, SupportsCustomModel: true},
		{ID: "gemini", DisplayName: "Google Gemini", DefaultModel: "gemini-3-pro-preview", CredentialLabelZH: "API Key", CredentialLabelEN: "API key", SupportsCustomAPIURL: true, SupportsCustomModel: true},
		{ID: "grok", DisplayName: "Grok (xAI)", DefaultModel: "grok-3-latest", CredentialLabelZH: "API Key", CredentialLabelEN: "API key", SupportsCustomAPIURL: true, SupportsCustomModel: true},
		{ID: "kimi", DisplayName: "Kimi (Moonshot)", DefaultModel: "moonshot-v1-auto", CredentialLabelZH: "API Key", CredentialLabelEN: "API key", SupportsCustomAPIURL: true, SupportsCustomModel: true},
		{ID: "minimax", DisplayName: "MiniMax", DefaultModel: "MiniMax-M2.7", CredentialLabelZH: "API Key", CredentialLabelEN: "API key", SupportsCustomAPIURL: true, SupportsCustomModel: true},
	}
}

func modelProviderSpecByID(provider string) (modelProviderSpec, bool) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	for _, spec := range supportedModelProviders() {
		if spec.ID == provider {
			return spec, true
		}
	}
	return modelProviderSpec{}, false
}

func supportedModelProviderIDs() []string {
	specs := supportedModelProviders()
	out := make([]string, 0, len(specs))
	for _, spec := range specs {
		out = append(out, spec.ID)
	}
	return out
}

func defaultModelNameForProvider(provider string) string {
	spec, ok := modelProviderSpecByID(provider)
	if !ok {
		return ""
	}
	return strings.TrimSpace(spec.DefaultModel)
}

func defaultModelConfigName(provider string) string {
	spec, ok := modelProviderSpecByID(provider)
	if !ok {
		provider = strings.TrimSpace(provider)
		if provider == "" {
			return ""
		}
		return provider + " AI"
	}
	return spec.DisplayName
}

func modelProviderSupportsCustomAPIURL(provider string) bool {
	spec, ok := modelProviderSpecByID(provider)
	return ok && spec.SupportsCustomAPIURL
}

func modelProviderSupportsCustomModel(provider string) bool {
	spec, ok := modelProviderSpecByID(provider)
	return ok && spec.SupportsCustomModel
}

func modelProviderCredentialLabel(lang, provider string) string {
	spec, ok := modelProviderSpecByID(provider)
	if !ok {
		if lang == "zh" {
			return "API Key"
		}
		return "API key"
	}
	if lang == "zh" {
		return spec.CredentialLabelZH
	}
	return spec.CredentialLabelEN
}

func modelProviderSummaryList(lang string) string {
	parts := make([]string, 0, len(supportedModelProviders()))
	for _, spec := range supportedModelProviders() {
		if lang == "zh" {
			parts = append(parts, fmt.Sprintf("%s（默认 %s）", spec.DisplayName, spec.DefaultModel))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s (default %s)", spec.DisplayName, spec.DefaultModel))
	}
	if lang == "zh" {
		return strings.Join(parts, "、")
	}
	return strings.Join(parts, ", ")
}

func modelProviderChoicePrompt(lang string) string {
	if lang == "zh" {
		return "可选模型 provider：" + modelProviderSummaryList(lang) + "。这些都是标准 API Key provider：只需在对应平台申请 API Key 填入即可。请先告诉我你想用哪个 provider。"
	}
	return "Available model providers: " + modelProviderSummaryList(lang) + ". These are standard API-key providers: just apply for an API key on the provider's platform and fill it in. Tell me which provider you want first."
}

func modelProviderDetailedGuidance(lang, provider string) string {
	spec, ok := modelProviderSpecByID(provider)
	if !ok {
		return ""
	}
	if lang == "zh" {
		lines := []string{
			fmt.Sprintf("你现在选的是 %s。", spec.DisplayName),
			fmt.Sprintf("- 默认模型名：%s", spec.DefaultModel),
			fmt.Sprintf("- 凭证类型：%s", spec.CredentialLabelZH),
		}
		if spec.SupportsCustomModel {
			lines = append(lines, "- `custom_model_name` 可选；留空时默认用上面的默认模型。")
		} else {
			lines = append(lines, "- 这个 provider 不需要单独填写 `custom_model_name`。")
		}
		if spec.SupportsCustomAPIURL {
			lines = append(lines, "- `custom_api_url` 可选；留空时使用官方默认地址。")
		} else {
			lines = append(lines, "- 这个 provider 不需要 `custom_api_url`。")
		}
		return strings.Join(lines, "\n")
	}
	lines := []string{
		fmt.Sprintf("You selected %s.", spec.DisplayName),
		fmt.Sprintf("- Default model: %s", spec.DefaultModel),
		fmt.Sprintf("- Credential type: %s", spec.CredentialLabelEN),
	}
	if spec.SupportsCustomModel {
		lines = append(lines, "- `custom_model_name` is optional; if omitted, the default model will be used.")
	} else {
		lines = append(lines, "- This provider does not need a separate `custom_model_name`.")
	}
	if spec.SupportsCustomAPIURL {
		lines = append(lines, "- `custom_api_url` is optional; if omitted, the official default endpoint will be used.")
	} else {
		lines = append(lines, "- This provider does not need `custom_api_url`.")
	}
	return strings.Join(lines, "\n")
}

func modelProviderCredentialGuidance(lang, provider string) string {
	spec, ok := modelProviderSpecByID(provider)
	if !ok {
		return ""
	}
	if lang == "zh" {
		return fmt.Sprintf("%s 这里要填的是 %s。你把完整值发我就行，我会继续当前模型草稿。", spec.DisplayName, spec.CredentialLabelZH)
	}
	return fmt.Sprintf("For %s, this field expects your %s. Send me the full value and I'll continue the current model draft.", spec.DisplayName, spec.CredentialLabelEN)
}
