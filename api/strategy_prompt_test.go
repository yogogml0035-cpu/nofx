package api

// 提示词接口的本金口径（⑨ sizing-engine）：preview-prompt 与 test-run
// 都以本金 × position_percent 展示开仓名义价值，且不再向模型索取金额字段。

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"nofx/store"
)

func TestPreviewPromptUsesPrincipalSizing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &Server{}

	cfg := store.GetDefaultStrategyConfig("zh")
	cfg.RiskControl.PositionPercent = 10
	body, err := json.Marshal(map[string]any{
		"config":            cfg,
		"account_principal": 1000,
	})
	require.NoError(t, err)

	w := callHandler(t, s.handlePreviewPrompt, http.MethodPost, "/api/strategies/preview-prompt", string(body), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		SystemPrompt string `json:"system_prompt"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Contains(t, resp.SystemPrompt, "本金 1000.00 × 10.0%")
	require.Contains(t, resp.SystemPrompt, "本金的 2%")
	require.NotContains(t, resp.SystemPrompt, "position_size_usd")
	require.NotContains(t, resp.SystemPrompt, "risk_usd")
}

func TestStrategyTestRunUsesPrincipalSizing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &Server{}

	// 空选币列表：不触发行情取数，只走提示词构建（不使用 AI）
	cfg := store.GetDefaultStrategyConfig("zh")
	cfg.RiskControl.PositionPercent = 10
	cfg.CoinSource.StaticCoins = nil
	body, err := json.Marshal(map[string]any{
		"config":         cfg,
		"run_real_ai":    false,
		"prompt_variant": "balanced",
	})
	require.NoError(t, err)

	w := callHandler(t, s.handleStrategyTestRun, http.MethodPost, "/api/strategies/test-run", string(body), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		SystemPrompt string `json:"system_prompt"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Contains(t, resp.SystemPrompt, "本金 1000.00 × 10.0%")
	require.NotContains(t, resp.SystemPrompt, "position_size_usd")
	require.NotContains(t, resp.SystemPrompt, "risk_usd")
}
