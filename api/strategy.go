package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"nofx/kernel"
	"nofx/logger"
	"nofx/market"
	"nofx/mcp"
	_ "nofx/mcp/provider"
	"nofx/store"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// validateStrategyConfig validates strategy configuration and returns warnings
func validateStrategyConfig(config *store.StrategyConfig) []string {
	var warnings []string
	if config.StrategyType == "grid_trading" {
		return warnings
	}

	return warnings
}

// strategyRequiresAIPrompt reports whether the config drives an AI prompt (and
// therefore consumes the model context window).
func strategyRequiresAIPrompt(config *store.StrategyConfig) bool {
	return config.StrategyType == "" || config.StrategyType == "ai_trading"
}

// strategyTokenWarnings returns a warning when the config fits no registered
// model, or exceeds some of them, so save responses do not silently imply the
// config is runnable everywhere.
func strategyTokenWarnings(estimate store.TokenEstimate, lang string) []string {
	names := estimate.ExceededModelNames()
	if len(names) == 0 {
		return nil
	}
	if lang == "zh" {
		return []string{fmt.Sprintf(
			"当前配置估算 %d tokens，超过以下模型的上下文上限：%s；绑定这些模型运行时会被 token 门禁拦截",
			estimate.Total, strings.Join(names, "、"))}
	}
	return []string{fmt.Sprintf(
		"Estimated %d tokens exceeds the context limit of: %s; runs bound to these models will be blocked by the token gate",
		estimate.Total, strings.Join(names, ", "))}
}

// handleEstimateTokens estimates token usage for a strategy config (no auth required, pure computation)
func (s *Server) handleEstimateTokens(c *gin.Context) {
	var req struct {
		Config store.StrategyConfig `json:"config" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}

	estimate := req.Config.EstimateTokens()
	c.JSON(http.StatusOK, estimate)
}

// handleGetStrategies Get strategy list
func (s *Server) handleGetStrategies(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	strategies, err := s.store.Strategy().List(userID)
	if err != nil {
		SafeInternalError(c, "Failed to get strategy list", err)
		return
	}

	// Convert to frontend format
	result := make([]gin.H, 0, len(strategies))
	for _, st := range strategies {
		var config store.StrategyConfig
		json.Unmarshal([]byte(st.Config), &config)

		result = append(result, gin.H{
			"id":          st.ID,
			"name":        st.Name,
			"description": st.Description,
			"is_active":   st.IsActive,
			"is_default":  st.IsDefault,
			"config":      config,
			"created_at":  st.CreatedAt,
			"updated_at":  st.UpdatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"strategies": result,
	})
}

// handleGetStrategy Get single strategy
func (s *Server) handleGetStrategy(c *gin.Context) {
	userID := c.GetString("user_id")
	strategyID := c.Param("id")

	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	strategy, err := s.store.Strategy().Get(userID, strategyID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Strategy not found"})
		return
	}

	var config store.StrategyConfig
	json.Unmarshal([]byte(strategy.Config), &config)

	c.JSON(http.StatusOK, gin.H{
		"id":          strategy.ID,
		"name":        strategy.Name,
		"description": strategy.Description,
		"is_active":   strategy.IsActive,
		"is_default":  strategy.IsDefault,
		"config":      config,
		"created_at":  strategy.CreatedAt,
		"updated_at":  strategy.UpdatedAt,
	})
}

// handleCreateStrategy Create strategy.
// If "config" is omitted from the request body, the system default config is used automatically.
func (s *Server) handleCreateStrategy(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		Name        string                `json:"name" binding:"required"`
		Description string                `json:"description"`
		Lang        string                `json:"lang"`   // "zh" or "en", used when config is omitted
		Config      *store.StrategyConfig `json:"config"` // optional — uses default if omitted
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}

	// Use default config when none provided
	if req.Config == nil {
		lang := req.Lang
		if lang == "" {
			lang = "zh"
		}
		defaultCfg := store.GetDefaultStrategyConfig(lang)
		req.Config = &defaultCfg
	}
	beforeClamp := *req.Config
	req.Config.ClampLimits()

	// Reject configs that cannot run on any registered model before writing.
	if strategyRequiresAIPrompt(req.Config) {
		if estimate, exceedsAll := req.Config.ExceedsAllModelLimits(); exceedsAll {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":          fmt.Sprintf("Estimated %d tokens exceeds all known model context limits. Reduce coins, timeframes, or K-line count.", estimate.Total),
				"token_estimate": estimate,
			})
			return
		}
	}

	// Serialize configuration
	configJSON, err := json.Marshal(req.Config)
	if err != nil {
		SafeInternalError(c, "Serialize configuration", err)
		return
	}

	strategy := &store.Strategy{
		ID:          uuid.New().String(),
		UserID:      userID,
		Name:        req.Name,
		Description: req.Description,
		IsActive:    false,
		IsDefault:   false,
		Config:      string(configJSON),
	}

	if err := s.store.Strategy().Create(strategy); err != nil {
		SafeInternalError(c, "Failed to create strategy", err)
		return
	}

	// Validate configuration and collect warnings
	warnings := validateStrategyConfig(req.Config)
	warnings = append(warnings, store.StrategyClampWarnings(beforeClamp, *req.Config, req.Config.Language)...)
	if strategyRequiresAIPrompt(req.Config) {
		warnings = append(warnings, strategyTokenWarnings(req.Config.EstimateTokens(), req.Config.Language)...)
	}

	response := gin.H{
		"id":      strategy.ID,
		"message": "Strategy created successfully",
	}
	if len(warnings) > 0 {
		response["warnings"] = warnings
	}

	c.JSON(http.StatusOK, response)
}

// handleUpdateStrategy Update strategy.
// The incoming config is merged with the existing one: top-level sections present in the
// request overwrite the corresponding existing sections; absent sections are preserved.
// This prevents partial updates from zeroing out unmentioned fields.
func (s *Server) handleUpdateStrategy(c *gin.Context) {
	userID := c.GetString("user_id")
	strategyID := c.Param("id")

	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Check if it's a system default strategy
	existing, err := s.store.Strategy().Get(userID, strategyID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Strategy not found"})
		return
	}
	if existing.IsDefault {
		c.JSON(http.StatusForbidden, gin.H{"error": "Cannot modify system default strategy"})
		return
	}

	var req struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Config      json.RawMessage `json:"config"` // raw JSON so we can merge
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}

	// Start with the existing config as base — preserves all unmentioned fields.
	var mergedConfig store.StrategyConfig
	if err := json.Unmarshal([]byte(existing.Config), &mergedConfig); err != nil {
		// If existing config is corrupt, start from zero
		mergedConfig = store.StrategyConfig{}
	}

	// Apply incoming config on top while preserving nested fields that were not sent.
	if len(req.Config) > 0 && string(req.Config) != "null" {
		var patch map[string]any
		if err := json.Unmarshal(req.Config, &patch); err != nil {
			SafeBadRequest(c, "Invalid config JSON")
			return
		}
		mergedConfig, err = store.MergeStrategyConfig(mergedConfig, patch)
		if err != nil {
			SafeBadRequest(c, "Invalid config JSON")
			return
		}
	}
	beforeClamp := mergedConfig
	mergedConfig.ClampLimits()

	// Token overflow check — reject before writing so a 400 never leaves the DB changed.
	if strategyRequiresAIPrompt(&mergedConfig) {
		if estimate, exceedsAll := mergedConfig.ExceedsAllModelLimits(); exceedsAll {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":          fmt.Sprintf("Estimated %d tokens exceeds all known model context limits. Reduce coins, timeframes, or K-line count.", estimate.Total),
				"token_estimate": estimate,
			})
			return
		}
	}

	// Preserve existing name/description when not supplied
	name := req.Name
	if name == "" {
		name = existing.Name
	}
	description := req.Description
	if description == "" {
		description = existing.Description
	}

	configJSON, err := json.Marshal(mergedConfig)
	if err != nil {
		SafeInternalError(c, "Serialize configuration", err)
		return
	}

	strategy := &store.Strategy{
		ID:          strategyID,
		UserID:      userID,
		Name:        name,
		Description: description,
		Config:      string(configJSON),
	}

	if err := s.store.Strategy().Update(strategy); err != nil {
		SafeInternalError(c, "Failed to update strategy", err)
		return
	}

	// Validate merged configuration and collect warnings
	warnings := validateStrategyConfig(&mergedConfig)
	warnings = append(warnings, store.StrategyClampWarnings(beforeClamp, mergedConfig, mergedConfig.Language)...)
	if strategyRequiresAIPrompt(&mergedConfig) {
		warnings = append(warnings, strategyTokenWarnings(mergedConfig.EstimateTokens(), mergedConfig.Language)...)
	}

	response := gin.H{"message": "Strategy updated successfully"}
	if len(warnings) > 0 {
		response["warnings"] = warnings
	}

	c.JSON(http.StatusOK, response)
}

// handleDeleteStrategy Delete strategy
func (s *Server) handleDeleteStrategy(c *gin.Context) {
	userID := c.GetString("user_id")
	strategyID := c.Param("id")

	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	if err := s.store.Strategy().Delete(userID, strategyID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": SanitizeError(err, "Failed to delete strategy")})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Strategy deleted successfully"})
}

// handleActivateStrategy Activate strategy
func (s *Server) handleActivateStrategy(c *gin.Context) {
	userID := c.GetString("user_id")
	strategyID := c.Param("id")

	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	if err := s.store.Strategy().SetActive(userID, strategyID); err != nil {
		SafeInternalError(c, "Failed to activate strategy", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Strategy activated successfully"})
}

// handleDuplicateStrategy Duplicate strategy
func (s *Server) handleDuplicateStrategy(c *gin.Context) {
	userID := c.GetString("user_id")
	sourceID := c.Param("id")

	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		Name string `json:"name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}

	newID := uuid.New().String()
	if err := s.store.Strategy().Duplicate(userID, sourceID, newID, req.Name); err != nil {
		SafeInternalError(c, "Failed to duplicate strategy", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":      newID,
		"message": "Strategy duplicated successfully",
	})
}

// handleGetActiveStrategy Get currently active strategy
func (s *Server) handleGetActiveStrategy(c *gin.Context) {
	userID := c.GetString("user_id")

	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	strategy, err := s.store.Strategy().GetActive(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No active strategy"})
		return
	}

	var config store.StrategyConfig
	json.Unmarshal([]byte(strategy.Config), &config)

	c.JSON(http.StatusOK, gin.H{
		"id":          strategy.ID,
		"name":        strategy.Name,
		"description": strategy.Description,
		"is_active":   strategy.IsActive,
		"is_default":  strategy.IsDefault,
		"config":      config,
		"created_at":  strategy.CreatedAt,
		"updated_at":  strategy.UpdatedAt,
	})
}

// handleGetDefaultStrategyConfig Get default strategy configuration template
func (s *Server) handleGetDefaultStrategyConfig(c *gin.Context) {
	// Get language from query parameter, default to "en"
	lang := c.Query("lang")
	if lang != "zh" {
		lang = "en"
	}

	// Return default configuration with i18n support
	defaultConfig := store.GetDefaultStrategyConfig(lang)
	c.JSON(http.StatusOK, defaultConfig)
}

// handlePreviewPrompt Preview prompt generated by strategy
func (s *Server) handlePreviewPrompt(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		Config store.StrategyConfig `json:"config" binding:"required"`
		// AccountPrincipal is the simulated account principal that open
		// positions are sized from (principal × position_percent).
		AccountPrincipal float64 `json:"account_principal"`
		PromptVariant    string  `json:"prompt_variant"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}

	// Use default values
	if req.AccountPrincipal <= 0 {
		req.AccountPrincipal = 1000.0 // Default simulated account principal
	}
	if req.PromptVariant == "" {
		req.PromptVariant = "balanced"
	}
	// The runtime path clamps the strategy before building the prompt; the preview
	// must show the same numbers (position percent 1%-20%, leverage limits, ...).
	req.Config.ClampLimits()

	// Create strategy engine to build prompt
	engine := kernel.NewStrategyEngine(&req.Config)

	// Build system prompt (using built-in method from strategy engine)
	systemPrompt := engine.BuildSystemPrompt(
		req.AccountPrincipal,
		req.PromptVariant,
	)

	c.JSON(http.StatusOK, gin.H{
		"system_prompt":  systemPrompt,
		"prompt_variant": req.PromptVariant,
		"config_summary": gin.H{
			"static_coins":     req.Config.CoinSource.StaticCoins,
			"primary_tf":       req.Config.Indicators.Klines.PrimaryTimeframe,
			"btc_eth_leverage": req.Config.RiskControl.BTCETHMaxLeverage,
			"altcoin_leverage": req.Config.RiskControl.AltcoinMaxLeverage,
			"max_positions":    req.Config.RiskControl.MaxPositions,
		},
	})
}

// handleStrategyTestRun AI test run (does not execute trades, only returns AI analysis results)
func (s *Server) handleStrategyTestRun(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		Config        store.StrategyConfig `json:"config" binding:"required"`
		PromptVariant string               `json:"prompt_variant"`
		AIModelID     string               `json:"ai_model_id"`
		RunRealAI     bool                 `json:"run_real_ai"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}

	if req.PromptVariant == "" {
		req.PromptVariant = "balanced"
	}
	// Match the runtime path: clamp the strategy before rendering the prompt.
	req.Config.ClampLimits()

	// 试运行同样受模型上下文约束：先估算再取行情/构建提示词，避免发起注定失败的大请求。
	if strategyRequiresAIPrompt(&req.Config) {
		estimate := req.Config.EstimateTokens()
		contextLimit := 131072 // safe default matching kernel.GetFullDecisionWithStrategy
		if req.AIModelID != "" && s.store != nil {
			if model, modelErr := s.store.AIModel().Get(userID, req.AIModelID); modelErr == nil {
				contextLimit = store.GetContextLimitForClient(model.Provider, model.CustomModelName)
			}
		}
		if estimate.Total > contextLimit {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":          fmt.Sprintf("Estimated %d tokens exceeds model context limit of %d. Reduce coins, timeframes, or K-line count.", estimate.Total, contextLimit),
				"token_estimate": estimate,
			})
			return
		}
	}

	// Create strategy engine to build prompt
	engine := kernel.NewStrategyEngine(&req.Config)

	// Get candidate coins
	candidates, err := engine.GetCandidateCoins()
	if err != nil {
		logger.Errorf("[API Error] Failed to get candidate coins: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":       "Failed to get candidate coins",
			"ai_response": "",
		})
		return
	}

	// Get timeframe configuration
	timeframes := req.Config.Indicators.Klines.SelectedTimeframes
	primaryTimeframe := req.Config.Indicators.Klines.PrimaryTimeframe
	klineCount := req.Config.Indicators.Klines.PrimaryCount

	// If no timeframes selected, use default values
	if len(timeframes) == 0 {
		// Backward compatibility: use primary and longer timeframes
		if primaryTimeframe != "" {
			timeframes = append(timeframes, primaryTimeframe)
		} else {
			timeframes = append(timeframes, "5m")
		}
		if req.Config.Indicators.Klines.LongerTimeframe != "" {
			timeframes = append(timeframes, req.Config.Indicators.Klines.LongerTimeframe)
		}
	}
	if primaryTimeframe == "" {
		primaryTimeframe = timeframes[0]
	}

	fmt.Printf("📊 Using timeframes: %v, primary: %s, kline count: %d\n", timeframes, primaryTimeframe, klineCount)

	// Get real market data (using multiple timeframes)
	marketDataMap := make(map[string]*market.Data)
	for _, coin := range candidates {
		data, err := market.GetWithTimeframes(coin.Symbol, timeframes, primaryTimeframe, klineCount)
		if err != nil {
			// If getting data for a coin fails, log but continue
			fmt.Printf("⚠️  Failed to get market data for %s: %v\n", coin.Symbol, err)
			continue
		}
		marketDataMap[coin.Symbol] = data
	}

	// Build real context (for generating User Prompt). The simulated account is
	// flat, so principal = equity = 1000 USDT.
	testContext := &kernel.Context{
		CurrentTime:    time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
		RuntimeMinutes: 0,
		CallCount:      1,
		Account: kernel.AccountInfo{
			Principal:        1000.0,
			TotalEquity:      1000.0,
			AvailableBalance: 1000.0,
			UnrealizedPnL:    0,
			TotalPnL:         0,
			TotalPnLPct:      0,
			MarginUsed:       0,
			MarginUsedPct:    0,
			PositionCount:    0,
		},
		Positions:      []kernel.PositionInfo{},
		CandidateCoins: candidates,
		PromptVariant:  req.PromptVariant,
		MarketDataMap:  marketDataMap,
	}

	// Build System Prompt（模拟账户本金 1000 USDT）
	systemPrompt := engine.BuildSystemPrompt(1000.0, req.PromptVariant)

	// Build User Prompt (using real market data)
	userPrompt := engine.BuildUserPrompt(testContext)

	// If requesting real AI call
	if req.RunRealAI && req.AIModelID != "" {
		aiResponse, aiErr := s.runRealAITest(userID, req.AIModelID, systemPrompt, userPrompt)
		if aiErr != nil {
			c.JSON(http.StatusOK, gin.H{
				"system_prompt":   systemPrompt,
				"user_prompt":     userPrompt,
				"candidate_count": len(candidates),
				"candidates":      candidates,
				"prompt_variant":  req.PromptVariant,
				"ai_response":     fmt.Sprintf("❌ AI call failed: %s", aiErr.Error()),
				"ai_error":        aiErr.Error(),
				"note":            "AI call error",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"system_prompt":   systemPrompt,
			"user_prompt":     userPrompt,
			"candidate_count": len(candidates),
			"candidates":      candidates,
			"prompt_variant":  req.PromptVariant,
			"ai_response":     aiResponse,
			"note":            "✅ Real AI test run successful",
		})
		return
	}

	// Return result (without actually calling AI, only return built prompt)
	c.JSON(http.StatusOK, gin.H{
		"system_prompt":   systemPrompt,
		"user_prompt":     userPrompt,
		"candidate_count": len(candidates),
		"candidates":      candidates,
		"prompt_variant":  req.PromptVariant,
		"ai_response":     "Please select an AI model and click 'Run Test' to perform real AI analysis.",
		"note":            "AI model not selected or real AI call not enabled",
	})
}

// runRealAITest Execute real AI test call
func (s *Server) runRealAITest(userID, modelID, systemPrompt, userPrompt string) (string, error) {
	// Get AI model configuration
	model, err := s.store.AIModel().Get(userID, modelID)
	if err != nil {
		return "", fmt.Errorf("failed to get AI model: %w", err)
	}

	if !model.Enabled {
		return "", fmt.Errorf("AI model %s is not enabled", model.Name)
	}

	if model.APIKey == "" {
		return "", fmt.Errorf("AI model %s is missing API Key", model.Name)
	}

	// Create AI client via registry
	provider := model.Provider
	apiKey := string(model.APIKey)

	aiClient := mcp.NewAIClientByProvider(provider, mcp.WithReasoningEffort(model.ReasoningEffort))
	if aiClient == nil {
		aiClient = mcp.NewClient(mcp.WithReasoningEffort(model.ReasoningEffort))
	}

	aiClient.SetAPIKey(apiKey, model.CustomAPIURL, model.CustomModelName)

	// Call AI API
	response, err := aiClient.CallWithMessages(systemPrompt, userPrompt)
	if err != nil {
		return "", fmt.Errorf("AI API call failed: %w", err)
	}

	return response, nil
}
