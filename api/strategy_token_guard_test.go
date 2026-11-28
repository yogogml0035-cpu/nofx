package api

// Token 契约回归：超大 K 线配置不得先落库再报错；试运行在取数前拒绝；
// 启动时按绑定模型预检，避免进入永久 safe mode。

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"nofx/store"
)

func oversizedAIConfig() store.StrategyConfig {
	cfg := store.GetDefaultStrategyConfig("zh")
	cfg.CoinSource.StaticCoins = []string{
		"BTCUSDT", "ETHUSDT", "SOLUSDT", "DOGEUSDT", "XRPUSDT",
		"BNBUSDT", "ADAUSDT", "LTCUSDT", "TRXUSDT", "DOTUSDT",
	}
	cfg.Indicators.Klines.SelectedTimeframes = []string{"5m", "15m", "1h", "4h"}
	cfg.Indicators.Klines.PrimaryTimeframe = "5m"
	cfg.Indicators.Klines.PrimaryCount = 2000
	return cfg
}

func TestStrategyTestRunRejectsOversizedPrompt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &Server{}
	body, err := json.Marshal(map[string]any{"config": oversizedAIConfig()})
	require.NoError(t, err)

	w := callHandler(t, s.handleStrategyTestRun, http.MethodPost, "/api/strategies/test-run", string(body), nil)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	var resp struct {
		TokenEstimate store.TokenEstimate `json:"token_estimate"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Greater(t, resp.TokenEstimate.Total, 0)
}

func TestUpdateStrategyRejectsOversizedConfigWithoutWriting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	st, err := store.New(filepath.Join(t.TempDir(), "api.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	base := store.GetDefaultStrategyConfig("zh")
	baseJSON, err := json.Marshal(base)
	require.NoError(t, err)
	require.NoError(t, st.Strategy().Create(&store.Strategy{
		ID: "strategy-1", UserID: apiTestUser, Name: "s", Config: string(baseJSON),
	}))

	s := &Server{store: st}
	body, err := json.Marshal(map[string]any{"name": "s", "description": "", "config": oversizedAIConfig()})
	require.NoError(t, err)

	w := callHandler(t, s.handleUpdateStrategy, http.MethodPut, "/api/strategies/strategy-1", string(body),
		gin.Params{{Key: "id", Value: "strategy-1"}})
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	stored, err := st.Strategy().Get(apiTestUser, "strategy-1")
	require.NoError(t, err)
	var got store.StrategyConfig
	require.NoError(t, json.Unmarshal([]byte(stored.Config), &got))
	require.Equal(t, base.Indicators.Klines.PrimaryCount, got.Indicators.Klines.PrimaryCount,
		"400 must not persist the oversized config")
	require.Equal(t, len(base.CoinSource.StaticCoins), len(got.CoinSource.StaticCoins))
}

func TestStrategyTokenBlockerRejectsTraderOverModelLimit(t *testing.T) {
	cfg := store.GetDefaultStrategyConfig("zh")
	cfg.CoinSource.StaticCoins = []string{"BTCUSDT"}
	cfg.Indicators.Klines.SelectedTimeframes = []string{"5m", "15m", "1h"}
	cfg.Indicators.Klines.PrimaryTimeframe = "5m"
	cfg.Indicators.Klines.PrimaryCount = 2000
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)

	fullCfg := &store.TraderFullConfig{
		Strategy: &store.Strategy{Config: string(raw)},
		AIModel:  &store.AIModel{Name: "qwen", Provider: "qwen"},
	}
	reason, blocked := strategyTokenBlocker(fullCfg)
	require.True(t, blocked)
	require.Contains(t, reason, "131072")

	// 同一配置在真实的 DeepSeek V4.1-Flash 上必须放行：该模型上下文是 1M，
	// 旧 provider 级 128K 元数据会把一次合法启动误判为永久超限。
	fullCfg.AIModel = &store.AIModel{
		Name:            "DeepSeek-V4.1-Flash-1017",
		Provider:        "deepseek",
		CustomModelName: "DeepSeek-V4.1-Flash-1017",
	}
	if reason, blocked := strategyTokenBlocker(fullCfg); blocked {
		t.Fatalf("DeepSeek V4.1-Flash must allow the 1M-context config, got blocked: %s", reason)
	}

	cfg.Indicators.Klines.PrimaryCount = 200
	raw, err = json.Marshal(cfg)
	require.NoError(t, err)
	fullCfg.Strategy.Config = string(raw)
	fullCfg.AIModel = &store.AIModel{Name: "qwen", Provider: "qwen"}
	if reason, blocked := strategyTokenBlocker(fullCfg); blocked {
		t.Fatalf("200 bars should fit qwen, got blocked: %s", reason)
	}
}

// 启动预检必须和运行时一样预留输出预算：否则估算刚好放下的配置会在启动后
// 每轮被 kernel.CheckPromptBudget 拒绝，进入永久失败。
func TestStrategyTokenBlockerReservesOutputBudget(t *testing.T) {
	cfg := store.GetDefaultStrategyConfig("zh")
	cfg.CoinSource.StaticCoins = []string{"BTCUSDT"}
	cfg.Indicators.Klines.SelectedTimeframes = []string{"5m", "15m", "1h"}
	cfg.Indicators.Klines.PrimaryTimeframe = "5m"
	cfg.Indicators.Klines.PrimaryCount = 900
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)

	fullCfg := &store.TraderFullConfig{
		Strategy: &store.Strategy{Config: string(raw)},
		AIModel:  &store.AIModel{Name: "qwen", Provider: "qwen", ReasoningEffort: store.DefaultReasoningEffort},
	}
	if reason, blocked := strategyTokenBlocker(fullCfg); blocked {
		t.Fatalf("default output budget must fit qwen, got blocked: %s", reason)
	}

	fullCfg.AIModel.ReasoningEffort = store.ReasoningEffortMax
	reason, blocked := strategyTokenBlocker(fullCfg)
	require.True(t, blocked, "max effort reserves 32K output and must not start")
	require.Contains(t, reason, "输出预算 32000")
}
