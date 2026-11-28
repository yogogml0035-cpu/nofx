package store

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Product-level limits for AI requests. Candidate/timeframe caps bound the
// prompt shape; MaxKlineCount follows the Gate fetch contract (2000 points per
// query). The authoritative token budget is enforced by EstimateTokens gates
// against the bound model's context limit.
const (
	MaxCandidateCoins = 10
	MaxPositions      = 3
	MaxTimeframes     = 4
	MinKlineCount     = 10
	// MaxKlineCount is the Gate candlesticks per-query ceiling: the official API
	// returns at most 2000 points in one query (limit=2001 is rejected with
	// INVALID_PARAM_VALUE; verified 2026-11-26 across all retained intervals).
	// Keep in sync with market.gateKlineMaxLimit.
	MaxKlineCount          = 2000
	MinLeverage            = 1
	MaxBTCETHLeverage      = 20
	MaxAltLeverage         = 20
	MinPositionPercent     = 1.0
	MaxPositionPercent     = 20.0
	DefaultPositionPercent = 10.0
	MinRiskReward          = 1.0
	MaxRiskReward          = 10.0
	MinMarginUsage         = 0.1
	MaxMarginUsage         = 1.0
	MinPositionSize        = 10.0
	MaxPositionSize        = 1000.0
	MinConfidence          = 50
	MaxConfidence          = 100
)

// ClampLimits enforces product-level limits on strategy config to prevent token overflow.
func (c *StrategyConfig) ClampLimits() {
	c.NormalizeProductSchema()

	// Clamp static coins
	if len(c.CoinSource.StaticCoins) > MaxCandidateCoins {
		c.CoinSource.StaticCoins = c.CoinSource.StaticCoins[:MaxCandidateCoins]
	}

	// Clamp kline count
	if c.Indicators.Klines.PrimaryCount < MinKlineCount {
		c.Indicators.Klines.PrimaryCount = MinKlineCount
	}
	if c.Indicators.Klines.PrimaryCount > MaxKlineCount {
		c.Indicators.Klines.PrimaryCount = MaxKlineCount
	}
	if c.Indicators.Klines.LongerCount > MaxKlineCount {
		c.Indicators.Klines.LongerCount = MaxKlineCount
	}

	// Clamp timeframes
	if len(c.Indicators.Klines.SelectedTimeframes) > MaxTimeframes {
		c.Indicators.Klines.SelectedTimeframes = c.Indicators.Klines.SelectedTimeframes[:MaxTimeframes]
	}

	// Clamp max positions
	if c.RiskControl.MaxPositions < 1 {
		c.RiskControl.MaxPositions = 1
	}
	if c.RiskControl.MaxPositions > MaxPositions {
		c.RiskControl.MaxPositions = MaxPositions
	}

	// Clamp leverage limits to the same bounds as the manual config UI.
	if c.RiskControl.BTCETHMaxLeverage < MinLeverage {
		c.RiskControl.BTCETHMaxLeverage = MinLeverage
	}
	if c.RiskControl.BTCETHMaxLeverage > MaxBTCETHLeverage {
		c.RiskControl.BTCETHMaxLeverage = MaxBTCETHLeverage
	}
	if c.RiskControl.AltcoinMaxLeverage < MinLeverage {
		c.RiskControl.AltcoinMaxLeverage = MinLeverage
	}
	if c.RiskControl.AltcoinMaxLeverage > MaxAltLeverage {
		c.RiskControl.AltcoinMaxLeverage = MaxAltLeverage
	}

	// Clamp open-position amount percent (D23: 1%-20%, default 10%). Missing
	// values (legacy configs without the field) fall back to the default.
	if c.RiskControl.PositionPercent <= 0 {
		c.RiskControl.PositionPercent = DefaultPositionPercent
	}
	if c.RiskControl.PositionPercent < MinPositionPercent {
		c.RiskControl.PositionPercent = MinPositionPercent
	}
	if c.RiskControl.PositionPercent > MaxPositionPercent {
		c.RiskControl.PositionPercent = MaxPositionPercent
	}

	// Clamp risk parameters and entry requirements.
	if c.RiskControl.MinRiskRewardRatio < MinRiskReward {
		c.RiskControl.MinRiskRewardRatio = MinRiskReward
	}
	if c.RiskControl.MinRiskRewardRatio > MaxRiskReward {
		c.RiskControl.MinRiskRewardRatio = MaxRiskReward
	}
	if c.RiskControl.MaxMarginUsage < MinMarginUsage {
		c.RiskControl.MaxMarginUsage = MinMarginUsage
	}
	if c.RiskControl.MaxMarginUsage > MaxMarginUsage {
		c.RiskControl.MaxMarginUsage = MaxMarginUsage
	}
	if c.RiskControl.MinPositionSize < MinPositionSize {
		c.RiskControl.MinPositionSize = MinPositionSize
	}
	if c.RiskControl.MinPositionSize > MaxPositionSize {
		c.RiskControl.MinPositionSize = MaxPositionSize
	}
	if c.RiskControl.MinConfidence < MinConfidence {
		c.RiskControl.MinConfidence = MinConfidence
	}
	if c.RiskControl.MinConfidence > MaxConfidence {
		c.RiskControl.MinConfidence = MaxConfidence
	}
}

// NormalizeProductSchema keeps saved strategy JSON aligned with the product
// editor schema. Persistence must use the exact frontend/backend enum values.
func (c *StrategyConfig) NormalizeProductSchema() {
	c.StrategyType = normalizeStrategyType(c.StrategyType)

	c.CoinSource.StaticCoins = normalizeSymbols(c.CoinSource.StaticCoins)
	c.CoinSource.ExcludedCoins = normalizeSymbols(c.CoinSource.ExcludedCoins)
	c.Indicators.Klines.PrimaryTimeframe = normalizeTimeframe(c.Indicators.Klines.PrimaryTimeframe)
	c.Indicators.Klines.LongerTimeframe = normalizeTimeframe(c.Indicators.Klines.LongerTimeframe)
	c.Indicators.Klines.SelectedTimeframes = normalizeTimeframes(c.Indicators.Klines.SelectedTimeframes)
	if len(c.Indicators.Klines.SelectedTimeframes) > 0 {
		c.Indicators.Klines.EnableMultiTimeframe = true
	}
}

func normalizeStrategyType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "grid", "grid_strategy", "grid-trading", "grid trading", "grid_trading", "网格", "网格策略", "网格交易":
		return "grid_trading"
	case "", "ai", "ai_strategy", "ai-trading", "ai trading", "ai_trading", "ai策略", "ai 策略", "ai交易策略", "ai智能策略":
		return "ai_trading"
	default:
		return value
	}
}

func normalizeSymbols(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range splitLooseStringList(values) {
		value = strings.ToUpper(strings.TrimSpace(value))
		value = strings.Trim(value, "，,;； ")
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func normalizeTimeframes(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range splitLooseStringList(values) {
		tf := normalizeTimeframe(value)
		if tf == "" || seen[tf] {
			continue
		}
		seen[tf] = true
		out = append(out, tf)
	}
	return out
}

func splitLooseStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	joined := strings.TrimSpace(strings.Join(values, ","))
	if strings.HasPrefix(joined, "[") && strings.HasSuffix(joined, "]") {
		var parsed []string
		if err := json.Unmarshal([]byte(joined), &parsed); err == nil {
			return parsed
		}
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
			var parsed []string
			if err := json.Unmarshal([]byte(value), &parsed); err == nil {
				parts = append(parts, parsed...)
				continue
			}
		}
		value = strings.Trim(value, "[]")
		for _, part := range strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == '，' || r == ';' || r == '；' || r == '\n'
		}) {
			part = strings.Trim(strings.TrimSpace(part), "\"'")
			if part != "" {
				parts = append(parts, part)
			}
		}
	}
	return parts
}

func normalizeTimeframe(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.Trim(value, "\"'，,。 ")
	if value == "" {
		return ""
	}
	aliases := map[string]string{
		"10秒":  "10s",
		"1分钟":  "1m",
		"5分钟":  "5m",
		"15分钟": "15m",
		"30分钟": "30m",
		"1小时":  "1h",
		"4小时":  "4h",
		"8小时":  "8h",
		"1天":   "1d",
		"7天":   "7d",
		"1周":   "1w",
		"30天":  "30d",
		"1个月":  "30d",
	}
	if alias, ok := aliases[value]; ok {
		return alias
	}
	// D25: strategy timeframe options are exactly the intervals listed in the
	// Gate candlestick doc enum plus 1w and 30d (see plan §3.3 archive).
	allowed := map[string]bool{
		"10s": true, "1m": true, "5m": true, "15m": true, "30m": true,
		"1h": true, "4h": true, "8h": true, "1d": true,
		"7d": true, "1w": true, "30d": true,
	}
	if !allowed[value] {
		return ""
	}
	return value
}

// RetainedTimeframes 策略可选的 K 线周期（D25）：Gate 合约 K 线文档枚举加 1w 与 30d。
// 周期越短，单次分析覆盖的时间窗越短；决策频率由交易员的扫描间隔决定。
var RetainedTimeframes = []string{"10s", "1m", "5m", "15m", "30m", "1h", "4h", "8h", "1d", "7d", "1w", "30d"}

// NormalizeTimeframe 把周期归一成 D25 集合内的值（别名 1小时→1h、大小写与首尾空白）；
// 不在集合内的值返回空串。
func NormalizeTimeframe(value string) string {
	return normalizeTimeframe(value)
}

// IsValidTimeframe 判断周期是否可用：保留集合内的值或其别名（1小时→1h）都算可用。
func IsValidTimeframe(value string) bool {
	return normalizeTimeframe(value) != ""
}

// RetainedTimeframeList 供错误与告警文案使用的周期清单。
func RetainedTimeframeList() string {
	return strings.Join(RetainedTimeframes, " / ")
}

// MergeStrategyConfig applies a partial JSON-style patch onto a full strategy config.
// Nested objects are merged recursively so omitted fields keep their previous values.
func MergeStrategyConfig(base StrategyConfig, patch map[string]any) (StrategyConfig, error) {
	baseJSON, err := json.Marshal(base)
	if err != nil {
		return StrategyConfig{}, err
	}

	var mergedMap map[string]any
	if err := json.Unmarshal(baseJSON, &mergedMap); err != nil {
		return StrategyConfig{}, err
	}

	normalizeStrategyConfigPatch(patch)
	if fmt.Sprint(patch["strategy_type"]) == "grid_trading" {
		ensureDefaultGridConfigMap(mergedMap)
	}
	mergeJSONMaps(mergedMap, patch)

	mergedJSON, err := json.Marshal(mergedMap)
	if err != nil {
		return StrategyConfig{}, err
	}

	var merged StrategyConfig
	if err := json.Unmarshal(mergedJSON, &merged); err != nil {
		return StrategyConfig{}, err
	}
	return merged, nil
}

func DefaultGridStrategyConfig() GridStrategyConfig {
	return GridStrategyConfig{
		Symbol:                "BTCUSDT",
		GridCount:             10,
		TotalInvestment:       1000,
		Leverage:              5,
		UpperPrice:            0,
		LowerPrice:            0,
		UseATRBounds:          true,
		ATRMultiplier:         2.0,
		Distribution:          "gaussian",
		MaxDrawdownPct:        15,
		StopLossPct:           5,
		DailyLossLimitPct:     10,
		UseMakerOnly:          true,
		EnableDirectionAdjust: false,
		DirectionBiasRatio:    0.7,
	}
}

func ensureDefaultGridConfigMap(config map[string]any) {
	if config == nil {
		return
	}
	if existing, ok := config["grid_config"].(map[string]any); ok && len(existing) > 0 {
		return
	}
	defaultGrid := DefaultGridStrategyConfig()
	raw, err := json.Marshal(defaultGrid)
	if err != nil {
		return
	}
	var gridMap map[string]any
	if err := json.Unmarshal(raw, &gridMap); err != nil {
		return
	}
	config["grid_config"] = gridMap
}

func normalizeStrategyConfigPatch(patch map[string]any) {
	if patch == nil {
		return
	}

	if gridConfig, hasGrid := patch["grid_config"]; hasGrid && gridConfig != nil {
		if _, hasType := patch["strategy_type"]; !hasType {
			patch["strategy_type"] = "grid_trading"
		}
	}

	aiKeys := []string{"coin_source", "indicators", "risk_control", "prompt_sections", "market_context", "custom_prompt"}
	for _, key := range aiKeys {
		value, ok := patch[key]
		if !ok {
			continue
		}
		aiConfig, _ := patch["ai_config"].(map[string]any)
		if aiConfig == nil {
			aiConfig = map[string]any{}
			patch["ai_config"] = aiConfig
		}
		aiConfig[key] = value
		delete(patch, key)
	}

	if fmt.Sprint(patch["strategy_type"]) == "grid_trading" {
		delete(patch, "ai_config")
	}

	if _, hasType := patch["strategy_type"]; hasType {
		return
	}
	if gridConfig, hasGrid := patch["grid_config"]; hasGrid && gridConfig != nil {
		patch["strategy_type"] = "grid_trading"
	}
}

func mergeJSONMaps(dst, src map[string]any) {
	for key, srcVal := range src {
		srcMap, srcIsMap := srcVal.(map[string]any)
		dstMap, dstIsMap := dst[key].(map[string]any)
		if srcIsMap && dstIsMap {
			mergeJSONMaps(dstMap, srcMap)
			continue
		}
		dst[key] = srcVal
	}
}

func StrategyClampWarnings(before, after StrategyConfig, lang string) []string {
	if lang != "zh" {
		lang = "en"
	}
	warnings := make([]string, 0, 8)
	appendInt := func(labelZH, labelEN string, from, to int) {
		if from == to {
			return
		}
		if lang == "zh" {
			warnings = append(warnings, fmt.Sprintf("%s 已从 %d 调整为 %d", labelZH, from, to))
			return
		}
		warnings = append(warnings, fmt.Sprintf("%s adjusted from %d to %d", labelEN, from, to))
	}
	appendFloat := func(labelZH, labelEN string, from, to float64) {
		if from == to {
			return
		}
		if lang == "zh" {
			warnings = append(warnings, fmt.Sprintf("%s 已从 %.2f 调整为 %.2f", labelZH, from, to))
			return
		}
		warnings = append(warnings, fmt.Sprintf("%s adjusted from %.2f to %.2f", labelEN, from, to))
	}

	appendInt("最大持仓数", "max_positions", before.RiskControl.MaxPositions, after.RiskControl.MaxPositions)
	appendInt("BTC/ETH 最大杠杆", "btc_eth_max_leverage", before.RiskControl.BTCETHMaxLeverage, after.RiskControl.BTCETHMaxLeverage)
	appendInt("山寨币最大杠杆", "altcoin_max_leverage", before.RiskControl.AltcoinMaxLeverage, after.RiskControl.AltcoinMaxLeverage)
	appendFloat("开仓金额百分比", "position_percent", before.RiskControl.PositionPercent, after.RiskControl.PositionPercent)
	appendFloat("最小盈亏比", "min_risk_reward_ratio", before.RiskControl.MinRiskRewardRatio, after.RiskControl.MinRiskRewardRatio)
	appendFloat("最大保证金使用率", "max_margin_usage", before.RiskControl.MaxMarginUsage, after.RiskControl.MaxMarginUsage)
	appendFloat("最小开仓金额", "min_position_size", before.RiskControl.MinPositionSize, after.RiskControl.MinPositionSize)
	appendInt("最低置信度", "min_confidence", before.RiskControl.MinConfidence, after.RiskControl.MinConfidence)
	appendInt("K 线数量", "K-line count", before.Indicators.Klines.PrimaryCount, after.Indicators.Klines.PrimaryCount)

	// 周期：别名归一（1小时→1h）或不在保留集合的值被清空时给出提示（D25）
	beforeTF, afterTF := before.Indicators.Klines.PrimaryTimeframe, after.Indicators.Klines.PrimaryTimeframe
	if beforeTF != afterTF {
		if lang == "zh" {
			if afterTF == "" {
				warnings = append(warnings, fmt.Sprintf("主周期 %q 不在支持范围（%s），已清空：运行时改用多周期的第一个周期（都没有则 5m）", beforeTF, strings.Join(RetainedTimeframes, "、")))
			} else {
				warnings = append(warnings, fmt.Sprintf("主周期已从 %q 调整为 %q", beforeTF, afterTF))
			}
		} else {
			if afterTF == "" {
				warnings = append(warnings, fmt.Sprintf("primary timeframe %q is not supported (%s) and was cleared: the first selected timeframe is used instead (5m when none)", beforeTF, strings.Join(RetainedTimeframes, ", ")))
			} else {
				warnings = append(warnings, fmt.Sprintf("primary timeframe adjusted from %q to %q", beforeTF, afterTF))
			}
		}
	}
	beforeSelected, afterSelected := before.Indicators.Klines.SelectedTimeframes, after.Indicators.Klines.SelectedTimeframes
	if strings.Join(beforeSelected, ",") != strings.Join(afterSelected, ",") {
		if lang == "zh" {
			warnings = append(warnings, fmt.Sprintf("多周期已按支持范围调整为 [%s]", strings.Join(afterSelected, ", ")))
		} else {
			warnings = append(warnings, fmt.Sprintf("selected timeframes adjusted to the supported set: [%s]", strings.Join(afterSelected, ", ")))
		}
	}
	return warnings
}

// StrategyStore strategy storage
type StrategyStore struct {
	db *gorm.DB
}

// Strategy strategy configuration
type Strategy struct {
	ID          string    `gorm:"primaryKey" json:"id"`
	UserID      string    `gorm:"column:user_id;not null;default:'';index" json:"user_id"`
	Name        string    `gorm:"not null" json:"name"`
	Description string    `gorm:"default:''" json:"description"`
	IsActive    bool      `gorm:"column:is_active;default:false;index" json:"is_active"`
	IsDefault   bool      `gorm:"column:is_default;default:false" json:"is_default"`
	Config      string    `gorm:"not null;default:'{}'" json:"config"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (Strategy) TableName() string { return "strategies" }

// StrategyConfig strategy configuration details (JSON structure)
type StrategyConfig struct {
	// Strategy type: "ai_trading" (default) or "grid_trading"
	StrategyType string `json:"strategy_type,omitempty"`

	// language setting: "zh" for Chinese, "en" for English
	// This determines the language used for data formatting and prompt generation
	Language string `json:"language,omitempty"`
	// AI trading configuration fields are kept on the Go struct for engine
	// compatibility, but JSON persistence nests them under ai_config.
	CoinSource     CoinSourceConfig     `json:"-"`
	Indicators     IndicatorConfig      `json:"-"`
	CustomPrompt   string               `json:"-"`
	RiskControl    RiskControlConfig    `json:"-"`
	PromptSections PromptSectionsConfig `json:"-"`
	MarketContext  MarketContextConfig  `json:"-"`

	// Grid trading configuration (only used when StrategyType == "grid_trading")
	GridConfig *GridStrategyConfig `json:"grid_config,omitempty"`
}

// AIStrategyConfig contains fields only used by AI trading strategies.
type AIStrategyConfig struct {
	CoinSource     CoinSourceConfig     `json:"coin_source"`
	Indicators     IndicatorConfig      `json:"indicators"`
	CustomPrompt   string               `json:"custom_prompt,omitempty"`
	RiskControl    RiskControlConfig    `json:"risk_control"`
	PromptSections PromptSectionsConfig `json:"prompt_sections,omitempty"`
	MarketContext  MarketContextConfig  `json:"market_context"`
}

// MarketContextConfig 市场环境上下文勾选（决策时注入对应 Gate 数据段）。
// 零值 = 全关 = 不注入（默认关闭是产品语义，非历史兼容）。仅 ai_trading 有效。
type MarketContextConfig struct {
	Overview      bool `json:"overview,omitempty"`       // 市场总览
	FearGreed     bool `json:"fear_greed,omitempty"`     // 恐惧贪婪指数
	NewsSentiment bool `json:"news_sentiment,omitempty"` // 新闻+社交情绪
}

// MarketContextEnabled 任一勾选即启用
func (m MarketContextConfig) MarketContextEnabled() bool {
	return m.Overview || m.FearGreed || m.NewsSentiment
}

// MarshalJSON writes the product-facing strategy schema:
// strategy_type + grid_config or ai_config.
func (c StrategyConfig) MarshalJSON() ([]byte, error) {
	strategyType := strings.TrimSpace(c.StrategyType)
	if strategyType == "" {
		strategyType = "ai_trading"
	}

	out := struct {
		StrategyType string              `json:"strategy_type"`
		Language     string              `json:"language,omitempty"`
		AIConfig     *AIStrategyConfig   `json:"ai_config,omitempty"`
		GridConfig   *GridStrategyConfig `json:"grid_config,omitempty"`
	}{
		StrategyType: strategyType,
		Language:     c.Language,
	}

	if strategyType == "grid_trading" {
		out.GridConfig = c.GridConfig
	} else {
		out.AIConfig = &AIStrategyConfig{
			CoinSource:     c.CoinSource,
			Indicators:     c.Indicators,
			CustomPrompt:   c.CustomPrompt,
			RiskControl:    c.RiskControl,
			PromptSections: c.PromptSections,
			MarketContext:  c.MarketContext,
		}
	}

	return json.Marshal(out)
}

// UnmarshalJSON accepts both the new nested schema and old flat configs. Old
// top-level AI fields are normalized into the Go compatibility fields.
func (c *StrategyConfig) UnmarshalJSON(data []byte) error {
	type rawStrategyConfig struct {
		StrategyType string              `json:"strategy_type"`
		Language     string              `json:"language"`
		AIConfig     *AIStrategyConfig   `json:"ai_config"`
		GridConfig   *GridStrategyConfig `json:"grid_config"`

		CoinSource     *CoinSourceConfig     `json:"coin_source"`
		Indicators     *IndicatorConfig      `json:"indicators"`
		CustomPrompt   *string               `json:"custom_prompt"`
		RiskControl    *RiskControlConfig    `json:"risk_control"`
		PromptSections *PromptSectionsConfig `json:"prompt_sections"`
	}

	var raw rawStrategyConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	c.StrategyType = raw.StrategyType
	c.Language = raw.Language
	c.GridConfig = raw.GridConfig

	if raw.AIConfig != nil {
		c.CoinSource = raw.AIConfig.CoinSource
		c.Indicators = raw.AIConfig.Indicators
		c.CustomPrompt = raw.AIConfig.CustomPrompt
		c.RiskControl = raw.AIConfig.RiskControl
		c.PromptSections = raw.AIConfig.PromptSections
		c.MarketContext = raw.AIConfig.MarketContext
	} else {
		if raw.CoinSource != nil {
			c.CoinSource = *raw.CoinSource
		}
		if raw.Indicators != nil {
			c.Indicators = *raw.Indicators
		}
		if raw.CustomPrompt != nil {
			c.CustomPrompt = *raw.CustomPrompt
		}
		if raw.RiskControl != nil {
			c.RiskControl = *raw.RiskControl
		}
		if raw.PromptSections != nil {
			c.PromptSections = *raw.PromptSections
		}
	}

	if strings.TrimSpace(c.StrategyType) == "" && c.GridConfig != nil {
		c.StrategyType = "grid_trading"
	}
	return nil
}

// GridStrategyConfig grid trading specific configuration
type GridStrategyConfig struct {
	// Trading pair (e.g., "BTCUSDT")
	Symbol string `json:"symbol"`
	// Number of grid levels (5-50)
	GridCount int `json:"grid_count"`
	// Total investment in USDT
	TotalInvestment float64 `json:"total_investment"`
	// Leverage (1-20)
	Leverage int `json:"leverage"`
	// Upper price boundary (0 = auto-calculate from ATR)
	UpperPrice float64 `json:"upper_price"`
	// Lower price boundary (0 = auto-calculate from ATR)
	LowerPrice float64 `json:"lower_price"`
	// Use ATR to auto-calculate bounds
	UseATRBounds bool `json:"use_atr_bounds"`
	// ATR multiplier for bound calculation (default 2.0)
	ATRMultiplier float64 `json:"atr_multiplier"`
	// Position distribution: "uniform" | "gaussian" | "pyramid"
	Distribution string `json:"distribution"`
	// Maximum drawdown percentage before emergency exit
	MaxDrawdownPct float64 `json:"max_drawdown_pct"`
	// Stop loss percentage per position
	StopLossPct float64 `json:"stop_loss_pct"`
	// Daily loss limit percentage
	DailyLossLimitPct float64 `json:"daily_loss_limit_pct"`
	// Use maker-only orders for lower fees
	UseMakerOnly bool `json:"use_maker_only"`
	// Enable automatic grid direction adjustment based on box breakouts
	EnableDirectionAdjust bool `json:"enable_direction_adjust"`
	// Direction bias ratio for long_bias/short_bias modes (default 0.7 = 70%/30%)
	DirectionBiasRatio float64 `json:"direction_bias_ratio"`
}

// PromptSectionsConfig editable sections of System Prompt
type PromptSectionsConfig struct {
	// role definition (title + description)
	RoleDefinition string `json:"role_definition,omitempty"`
	// trading frequency awareness
	TradingFrequency string `json:"trading_frequency,omitempty"`
	// entry standards
	EntryStandards string `json:"entry_standards,omitempty"`
	// decision process
	DecisionProcess string `json:"decision_process,omitempty"`
}

// CoinSourceConfig coin source configuration
type CoinSourceConfig struct {
	// static coin list (the only coin source: hand-picked Gate perpetual symbols)
	StaticCoins []string `json:"static_coins,omitempty"`
	// excluded coins list (filtered out from the static list)
	ExcludedCoins []string `json:"excluded_coins,omitempty"`
}

// IndicatorConfig indicator configuration
type IndicatorConfig struct {
	// K-line configuration
	Klines KlineConfig `json:"klines"`
	// raw kline data (OHLCV) - always enabled, required for AI analysis
	EnableRawKlines bool `json:"enable_raw_klines"`
	// technical indicator switches
	EnableEMA         bool `json:"enable_ema"`
	EnableMACD        bool `json:"enable_macd"`
	EnableRSI         bool `json:"enable_rsi"`
	EnableATR         bool `json:"enable_atr"`
	EnableBOLL        bool `json:"enable_boll"` // Bollinger Bands
	EnableVolume      bool `json:"enable_volume"`
	EnableOI          bool `json:"enable_oi"`           // open interest
	EnableFundingRate bool `json:"enable_funding_rate"` // funding rate
	// EMA period configuration
	EMAPeriods []int `json:"ema_periods,omitempty"` // default [20, 50]
	// RSI period configuration
	RSIPeriods []int `json:"rsi_periods,omitempty"` // default [7, 14]
	// ATR period configuration
	ATRPeriods []int `json:"atr_periods,omitempty"` // default [14]
	// BOLL period configuration (period, standard deviation multiplier is fixed at 2)
	BOLLPeriods []int `json:"boll_periods,omitempty"` // default [20] - can select multiple timeframes
	// external data sources
	ExternalDataSources []ExternalDataSource `json:"external_data_sources,omitempty"`
}

// KlineConfig K-line configuration
type KlineConfig struct {
	// primary timeframe: Gate candlestick intervals (D25), e.g. "10s", "1m", "5m", "15m", "1h", "4h", "1d", "7d", "1w", "30d"
	PrimaryTimeframe string `json:"primary_timeframe"`
	// primary timeframe K-line count
	PrimaryCount int `json:"primary_count"`
	// longer timeframe
	LongerTimeframe string `json:"longer_timeframe,omitempty"`
	// longer timeframe K-line count
	LongerCount int `json:"longer_count,omitempty"`
	// whether to enable multi-timeframe analysis
	EnableMultiTimeframe bool `json:"enable_multi_timeframe"`
	// selected timeframe list (new: supports multi-timeframe selection)
	SelectedTimeframes []string `json:"selected_timeframes,omitempty"`
}

// ExternalDataSource external data source configuration
type ExternalDataSource struct {
	Name        string            `json:"name"`   // data source name
	Type        string            `json:"type"`   // type: "api" | "webhook"
	URL         string            `json:"url"`    // API URL
	Method      string            `json:"method"` // HTTP method
	Headers     map[string]string `json:"headers,omitempty"`
	DataPath    string            `json:"data_path,omitempty"`    // JSON data path
	RefreshSecs int               `json:"refresh_secs,omitempty"` // refresh interval (seconds)
}

// RiskControlConfig risk control configuration
type RiskControlConfig struct {
	// Max number of coins held simultaneously (CODE ENFORCED)
	MaxPositions int `json:"max_positions"`

	// BTC/ETH exchange leverage for opening positions (AI guided)
	BTCETHMaxLeverage int `json:"btc_eth_max_leverage"`
	// Altcoin exchange leverage for opening positions (AI guided)
	AltcoinMaxLeverage int `json:"altcoin_max_leverage"`

	// Open position notional = principal × this percent (D23: 1-20, step 0.1,
	// default 10). The kernel computes and enforces the amount; the AI decision
	// contract carries no amount field.
	PositionPercent float64 `json:"position_percent"`

	// Max margin utilization (e.g. 0.9 = 90%) (CODE ENFORCED)
	MaxMarginUsage float64 `json:"max_margin_usage"`
	// Min position size in USDT (user configurable, default 10; D19)
	MinPositionSize float64 `json:"min_position_size"`

	// Min take_profit / stop_loss ratio (AI guided)
	MinRiskRewardRatio float64 `json:"min_risk_reward_ratio"`
	// Min AI confidence to open position (AI guided)
	MinConfidence int `json:"min_confidence"`
}

// NewStrategyStore creates a new StrategyStore
func NewStrategyStore(db *gorm.DB) *StrategyStore {
	return &StrategyStore{db: db}
}

func (s *StrategyStore) initTables() error {
	// AutoMigrate will add missing columns without dropping existing data
	return s.db.AutoMigrate(&Strategy{})
}

func (s *StrategyStore) initDefaultData() error {
	// No longer pre-populate strategies - create on demand when user configures
	return nil
}

// GetDefaultStrategyConfig returns the default strategy configuration for the given language
func GetDefaultStrategyConfig(lang string) StrategyConfig {
	// Normalize language to "zh" or "en"
	normalizedLang := "en"
	if lang == "zh" {
		normalizedLang = "zh"
	}

	config := StrategyConfig{
		Language:   normalizedLang,
		CoinSource: CoinSourceConfig{},
		Indicators: IndicatorConfig{
			Klines: KlineConfig{
				PrimaryTimeframe:     "5m",
				PrimaryCount:         20,
				LongerTimeframe:      "4h",
				LongerCount:          10,
				EnableMultiTimeframe: true,
				SelectedTimeframes:   []string{"5m", "15m", "1h"},
			},
			EnableRawKlines:   true, // Required - raw OHLCV data for AI analysis
			EnableEMA:         false,
			EnableMACD:        false,
			EnableRSI:         false,
			EnableATR:         false,
			EnableBOLL:        false,
			EnableVolume:      true,
			EnableOI:          true,
			EnableFundingRate: true,
			EMAPeriods:        []int{20, 50},
			RSIPeriods:        []int{7, 14},
			ATRPeriods:        []int{14},
			BOLLPeriods:       []int{20},
		},
		RiskControl: RiskControlConfig{
			MaxPositions:       3,   // Max 3 coins simultaneously (CODE ENFORCED)
			BTCETHMaxLeverage:  5,   // BTC/ETH exchange leverage (AI guided)
			AltcoinMaxLeverage: 5,   // Altcoin exchange leverage (AI guided)
			PositionPercent:    10,  // Notional = principal × 10% (D23)
			MaxMarginUsage:     0.9, // Max 90% margin usage (CODE ENFORCED)
			MinPositionSize:    10,  // Min 10 USDT per position (user configurable, D19)
			MinRiskRewardRatio: 3.0, // Min 3:1 profit/loss ratio (AI guided)
			MinConfidence:      75,  // Min 75% confidence (AI guided)
		},
	}

	if lang == "zh" {
		config.PromptSections = PromptSectionsConfig{
			RoleDefinition: `# 你是一个专业的加密货币永续合约交易AI

你的任务是根据提供的市场数据做出交易决策。你可以分析并交易各交易所上线的加密货币 USDT 永续合约。你是一个经验丰富的量化交易员，擅长技术分析和风险管理。`,
			TradingFrequency: `# ⏱️ 交易频率意识

- 优秀交易员：每天2-4笔 ≈ 每小时0.1-0.2笔
- 每小时超过2笔 = 过度交易
- 单笔持仓时间 ≥ 30-60分钟
如果你发现自己每个周期都在交易 → 标准太低；如果持仓不到30分钟就平仓 → 太冲动。`,
			EntryStandards: `# 🎯 入场标准（严格）

只在多个信号共振时入场。自由使用任何有效的分析方法，避免单一指标、信号矛盾、横盘震荡、或平仓后立即重新开仓等低质量行为。`,
			DecisionProcess: `# 📋 决策流程

1. 检查持仓 → 是否止盈/止损
2. 扫描候选币种 + 多时间框架 → 是否存在强信号
3. 先写思维链，再输出结构化JSON`,
		}
	} else {
		config.PromptSections = PromptSectionsConfig{
			RoleDefinition: `# You are a professional crypto perpetual futures trading AI

Your task is to make trading decisions based on the provided market data. You can analyze and trade crypto USDT perpetual futures listed on supported exchanges. You are an experienced quantitative trader skilled in technical analysis and risk management.`,
			TradingFrequency: `# ⏱️ Trading Frequency Awareness

- Excellent trader: 2-4 trades per day ≈ 0.1-0.2 trades per hour
- >2 trades per hour = overtrading
- Single position holding time ≥ 30-60 minutes
If you find yourself trading every cycle → standards are too low; if closing positions in <30 minutes → too impulsive.`,
			EntryStandards: `# 🎯 Entry Standards (Strict)

Only enter positions when multiple signals resonate. Freely use any effective analysis methods, avoid low-quality behaviors such as single indicators, contradictory signals, sideways oscillation, or immediately restarting after closing positions.`,
			DecisionProcess: `# 📋 Decision Process

1. Check positions → whether to take profit/stop loss
2. Scan candidate coins + multi-timeframe → whether strong signals exist
3. Write chain of thought first, then output structured JSON`,
		}
	}

	return config
}

// Create create a strategy
func (s *StrategyStore) Create(strategy *Strategy) error {
	return s.db.Create(strategy).Error
}

// Update update a strategy
func (s *StrategyStore) Update(strategy *Strategy) error {
	return s.db.Model(&Strategy{}).
		Where("id = ? AND user_id = ?", strategy.ID, strategy.UserID).
		Updates(map[string]interface{}{
			"name":        strategy.Name,
			"description": strategy.Description,
			"config":      strategy.Config,
			"updated_at":  time.Now().UTC(),
		}).Error
}

// Delete delete a strategy
func (s *StrategyStore) Delete(userID, id string) error {
	// do not allow deleting system default strategy
	var st Strategy
	if err := s.db.Where("id = ?", id).First(&st).Error; err == nil {
		if st.IsDefault {
			return fmt.Errorf("cannot delete system default strategy")
		}
		if st.IsActive {
			return fmt.Errorf("cannot delete active strategy")
		}
	}

	// Check if any trader references this strategy
	var count int64
	if err := s.db.Model(&Trader{}).
		Where("user_id = ? AND strategy_id = ?", userID, id).
		Count(&count).Error; err == nil && count > 0 {
		return fmt.Errorf("cannot delete strategy in use by %d trader(s) - reassign those traders first", count)
	}

	return s.db.Where("id = ? AND user_id = ?", id, userID).Delete(&Strategy{}).Error
}

// List get user's strategy list
func (s *StrategyStore) List(userID string) ([]*Strategy, error) {
	var strategies []*Strategy
	err := s.db.Where("user_id = ? OR is_default = ?", userID, true).
		Order("is_default DESC, created_at DESC").
		Find(&strategies).Error
	if err != nil {
		return nil, err
	}
	return strategies, nil
}

// Get get a single strategy
func (s *StrategyStore) Get(userID, id string) (*Strategy, error) {
	var st Strategy
	err := s.db.Where("id = ? AND (user_id = ? OR is_default = ?)", id, userID, true).
		First(&st).Error
	if err != nil {
		return nil, err
	}
	return &st, nil
}

// GetActive get user's currently active strategy
func (s *StrategyStore) GetActive(userID string) (*Strategy, error) {
	var st Strategy
	err := s.db.Where("user_id = ? AND is_active = ?", userID, true).First(&st).Error
	if err == gorm.ErrRecordNotFound {
		// no active strategy, return system default strategy
		return s.GetDefault()
	}
	if err != nil {
		return nil, err
	}
	return &st, nil
}

// GetDefault get system default strategy
func (s *StrategyStore) GetDefault() (*Strategy, error) {
	var st Strategy
	err := s.db.Where("is_default = ?", true).First(&st).Error
	if err != nil {
		return nil, err
	}
	return &st, nil
}

// SetActive set active strategy (will first deactivate other strategies)
func (s *StrategyStore) SetActive(userID, strategyID string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// first deactivate all strategies for the user
		if err := tx.Model(&Strategy{}).Where("user_id = ?", userID).
			Update("is_active", false).Error; err != nil {
			return err
		}

		// activate specified strategy
		return tx.Model(&Strategy{}).
			Where("id = ? AND (user_id = ? OR is_default = ?)", strategyID, userID, true).
			Update("is_active", true).Error
	})
}

// Duplicate duplicate a strategy (used to create custom strategy based on default strategy)
func (s *StrategyStore) Duplicate(userID, sourceID, newID, newName string) error {
	// get source strategy
	source, err := s.Get(userID, sourceID)
	if err != nil {
		return fmt.Errorf("failed to get source strategy: %w", err)
	}

	// create new strategy
	newStrategy := &Strategy{
		ID:          newID,
		UserID:      userID,
		Name:        newName,
		Description: "Created based on [" + source.Name + "]",
		IsActive:    false,
		IsDefault:   false,
		Config:      source.Config,
	}

	return s.Create(newStrategy)
}

// ParseConfig parse strategy configuration JSON
func (s *Strategy) ParseConfig() (*StrategyConfig, error) {
	var config StrategyConfig
	if err := json.Unmarshal([]byte(s.Config), &config); err != nil {
		return nil, fmt.Errorf("failed to parse strategy configuration: %w", err)
	}
	return &config, nil
}

// SetConfig set strategy configuration
func (s *Strategy) SetConfig(config *StrategyConfig) error {
	data, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to serialize strategy configuration: %w", err)
	}
	s.Config = string(data)
	return nil
}

// ============================================================================
// Token Estimation
// ============================================================================

// TokenEstimate holds the result of token estimation
type TokenEstimate struct {
	Total       int            `json:"total"`
	Breakdown   TokenBreakdown `json:"breakdown"`
	ModelLimits []ModelLimit   `json:"model_limits"`
	Suggestions []string       `json:"suggestions"`
}

// TokenBreakdown shows estimated tokens per component
type TokenBreakdown struct {
	SystemPrompt  int `json:"system_prompt"`
	MarketData    int `json:"market_data"`
	FixedOverhead int `json:"fixed_overhead"`
}

// ModelLimit shows token usage against a specific model's context limit
type ModelLimit struct {
	Name         string `json:"name"`
	ContextLimit int    `json:"context_limit"`
	UsagePct     int    `json:"usage_pct"`
	Level        string `json:"level"` // "ok" | "warning" | "danger"
}

// Context window sizes (tokens) for each provider's current generation.
// DeepSeek V4.1-Flash/V4-Pro publish a 1M window and 384K max output; the
// retired model ids are served by V4.1-Flash (DeepSeek API docs, 2026-11-27).
const (
	contextLimitDeepSeek = 1_000_000 // 1M
	contextLimitOpenAI   = 128_000   // 128K
	contextLimitClaude   = 200_000   // 200K
	contextLimitQwen     = 131_072   // 128K
	contextLimitGemini   = 1_000_000 // 1M
	contextLimitGrok     = 131_072   // 128K
	contextLimitKimi     = 131_072   // 128K
	contextLimitMinimax  = 1_000_000 // 1M

	// defaultContextLimit is the conservative fallback for an unknown
	// provider or a custom endpoint whose model id does not declare a window.
	defaultContextLimit = 131_072
)

// ModelContextLimits maps provider names to their default context window.
// Model-specific capabilities below take precedence when the model id is
// known; this table remains the fallback for custom endpoints.
var ModelContextLimits = map[string]int{
	"deepseek": contextLimitDeepSeek,
	"openai":   contextLimitOpenAI,
	"claude":   contextLimitClaude,
	"qwen":     contextLimitQwen,
	"gemini":   contextLimitGemini,
	"grok":     contextLimitGrok,
	"kimi":     contextLimitKimi,
	"minimax":  contextLimitMinimax,
}

// GetContextLimit returns the provider-level fallback context limit.
func GetContextLimit(provider string) int {
	if limit, ok := ModelContextLimits[provider]; ok {
		return limit
	}
	return defaultContextLimit
}

// ModelCapability is the budget the selected model actually enforces. The
// context window is shared by prompt + completion; MaxOutputTokens is the
// provider-documented completion ceiling (0 means unknown).
type ModelCapability struct {
	ContextWindow   int
	MaxOutputTokens int
}

// modelCapabilities lists model ids whose capacity is known to differ from a
// single provider-wide number. Match by lower-case prefix so dated snapshots
// such as DeepSeek-V4.1-Flash-1017 resolve to the same generation.
var modelCapabilities = []struct {
	Prefix     string
	Capability ModelCapability
}{
	{"deepseek-v4.1-flash", ModelCapability{ContextWindow: contextLimitDeepSeek, MaxOutputTokens: 384_000}},
	{"deepseek-v4-pro", ModelCapability{ContextWindow: contextLimitDeepSeek, MaxOutputTokens: 384_000}},
}

// GetModelCapabilities resolves the budget for a provider+model pair. A custom
// BaseURL can route to a different model than the provider id suggests, so the
// model id is authoritative when it is known and the provider table is only a
// fallback.
func GetModelCapabilities(provider, model string) ModelCapability {
	normalizedModel := strings.ToLower(strings.TrimSpace(model))
	for _, entry := range modelCapabilities {
		if normalizedModel == entry.Prefix || strings.HasPrefix(normalizedModel, entry.Prefix+"-") {
			return entry.Capability
		}
	}
	return ModelCapability{ContextWindow: GetContextLimit(provider)}
}

// EstimateTokens estimates the total token count for a strategy configuration.
// This is a pure computation based on config fields — no network calls.
func (c *StrategyConfig) EstimateTokens() TokenEstimate {
	breakdown := TokenBreakdown{}

	// --- System Prompt ---
	// Base system prompt: schema + role + rules + output format
	baseChars := 4000 // English default
	if c.Language == "zh" {
		baseChars = 3000
	}
	// Add prompt sections
	baseChars += len(c.PromptSections.RoleDefinition)
	baseChars += len(c.PromptSections.TradingFrequency)
	baseChars += len(c.PromptSections.EntryStandards)
	baseChars += len(c.PromptSections.DecisionProcess)
	baseChars += len(c.CustomPrompt)

	if c.Language == "zh" {
		breakdown.SystemPrompt = baseChars / 2 // CJK: ~2 chars per token
	} else {
		breakdown.SystemPrompt = baseChars / 4 // English: ~4 chars per token
	}

	// --- Fixed Overhead ---
	// Time, BTC price, account info, section headers
	breakdown.FixedOverhead = 800 / 4 // ~200 tokens

	// --- Market Data ---
	numCoins := c.getEffectiveCoinCount()
	numTimeframes := c.getEffectiveTimeframeCount()
	klineCount := c.Indicators.Klines.PrimaryCount
	if klineCount <= 0 {
		klineCount = MinKlineCount
	}

	// Per coin per timeframe: one rendered OHLCV row plus the per-row share of
	// the indicator arrays. Real 4-decimal numeric tables tokenize at ~35
	// tokens per K-line row and ~5 tokens per indicator value; the 15% safety
	// margin below covers formatting drift such as the current marker.
	klineTokensPerLine := 35
	indicatorTokensPerLine := 0
	if c.Indicators.EnableEMA {
		indicatorTokensPerLine += 10 // EMA20 + EMA50 arrays
	}
	if c.Indicators.EnableMACD {
		indicatorTokensPerLine += 5
	}
	if c.Indicators.EnableRSI {
		indicatorTokensPerLine += 10 // RSI7 + RSI14 arrays
	}
	if c.Indicators.EnableBOLL {
		indicatorTokensPerLine += 15 // upper/middle/lower arrays
	}
	// ATR is one scalar per timeframe and volume is already in the OHLCV row;
	// both stay inside the safety margin instead of a per-line addend.
	tokensPerCoinTF := klineCount * (klineTokensPerLine + indicatorTokensPerLine)

	breakdown.MarketData = numCoins * numTimeframes * tokensPerCoinTF
	if c.Indicators.EnableOI || c.Indicators.EnableFundingRate {
		breakdown.MarketData += numCoins * 25 // OI + funding lines
	}

	// --- Market Environment Context（勾选即计入，软上限约 600 tokens）---
	if c.MarketContext.MarketContextEnabled() && c.StrategyType != "grid_trading" {
		breakdown.MarketData += 600
	}

	// --- Total with 15% safety margin ---
	subtotal := breakdown.SystemPrompt + breakdown.MarketData + breakdown.FixedOverhead
	total := subtotal * 115 / 100

	// --- Model limits ---
	modelLimits := make([]ModelLimit, 0, len(ModelContextLimits))
	for name, limit := range ModelContextLimits {
		pct := total * 100 / limit
		level := "ok"
		if pct >= 100 {
			level = "danger"
		} else if pct >= 80 {
			level = "warning"
		}
		modelLimits = append(modelLimits, ModelLimit{
			Name:         name,
			ContextLimit: limit,
			UsagePct:     pct,
			Level:        level,
		})
	}

	// Sort by usage_pct desc, then name asc for deterministic order
	sort.Slice(modelLimits, func(i, j int) bool {
		if modelLimits[i].UsagePct != modelLimits[j].UsagePct {
			return modelLimits[i].UsagePct > modelLimits[j].UsagePct
		}
		return modelLimits[i].Name < modelLimits[j].Name
	})

	// --- Suggestions ---
	var suggestions []string
	// Find the strictest model (smallest context)
	minLimit := 0
	for _, limit := range ModelContextLimits {
		if minLimit == 0 || limit < minLimit {
			minLimit = limit
		}
	}
	if minLimit > 0 && total > minLimit {
		if numTimeframes > 1 {
			savedPerTF := numCoins * tokensPerCoinTF * 115 / 100
			suggestions = append(suggestions, fmt.Sprintf("Reduce 1 timeframe to save ~%d tokens", savedPerTF))
		}
		if numCoins > 1 {
			savedPerCoin := numTimeframes * tokensPerCoinTF * 115 / 100
			suggestions = append(suggestions, fmt.Sprintf("Reduce 1 coin to save ~%d tokens", savedPerCoin))
		}
		if klineCount > MinKlineCount {
			suggestions = append(suggestions, fmt.Sprintf(
				"Reduce K-line count (currently %d) to fit the strictest model context limit (%d tokens)",
				klineCount, minLimit))
		}
	}

	return TokenEstimate{
		Total:       total,
		Breakdown:   breakdown,
		ModelLimits: modelLimits,
		Suggestions: suggestions,
	}
}

// The rendered market data is a dense numeric table, not prose: the real
// 2-coin x 4-timeframe x 300-bar prompt measured 139,981 provider tokens for
// 256,432 bytes (~1.83 ASCII bytes per token). Using 1.7 keeps the runtime
// bound above the provider count without a tokenizer dependency; non-ASCII
// (CJK) text is counted as one token per rune.
const (
	asciiBytesPerTokenNumerator   = 10
	asciiBytesPerTokenDenominator = 17
	promptMessageOverheadTokens   = 64
)

// EstimatePromptTokens returns a conservative token bound for an already
// rendered prompt. Use it immediately before the model call; EstimateTokens
// remains the config-level precheck used before market data exists.
func EstimatePromptTokens(systemPrompt, userPrompt string) int {
	if systemPrompt == "" && userPrompt == "" {
		return 0
	}
	asciiBytes := 0
	nonASCII := 0
	for _, r := range systemPrompt + "\n" + userPrompt {
		if r <= 0x7F {
			asciiBytes++
		} else {
			nonASCII++
		}
	}
	return asciiBytes*asciiBytesPerTokenNumerator/asciiBytesPerTokenDenominator + nonASCII + promptMessageOverheadTokens
}

// ExceedsAllModelLimits reports whether the estimated prompt exceeds every
// known model context limit — such a config cannot run on any registered model.
func (c *StrategyConfig) ExceedsAllModelLimits() (TokenEstimate, bool) {
	estimate := c.EstimateTokens()
	if len(estimate.ModelLimits) == 0 {
		return estimate, false
	}
	for _, limit := range estimate.ModelLimits {
		if limit.UsagePct <= 100 {
			return estimate, false
		}
	}
	return estimate, true
}

// ExceededModelNames returns the models whose context limit the estimate exceeds,
// ordered from the worst overflow to the mildest (same order as ModelLimits).
func (e TokenEstimate) ExceededModelNames() []string {
	names := make([]string, 0, len(e.ModelLimits))
	for _, limit := range e.ModelLimits {
		if limit.UsagePct > 100 {
			names = append(names, limit.Name)
		}
	}
	return names
}

// getEffectiveCoinCount returns the estimated number of coins that will be analyzed.
// Excluded coins never reach the prompt, so they do not count towards the estimate.
func (c *StrategyConfig) getEffectiveCoinCount() int {
	excluded := make(map[string]struct{}, len(c.CoinSource.ExcludedCoins))
	for _, symbol := range c.CoinSource.ExcludedCoins {
		if key := strings.ToUpper(strings.TrimSpace(symbol)); key != "" {
			excluded[key] = struct{}{}
		}
	}
	count := 0
	for _, symbol := range c.CoinSource.StaticCoins {
		if _, skip := excluded[strings.ToUpper(strings.TrimSpace(symbol))]; !skip {
			count++
		}
	}
	if count <= 0 {
		count = 3 // no static selection: the engine falls back to the dynamic candidate list
	}
	return count
}

// getEffectiveTimeframeCount returns the number of timeframes that will be fetched and rendered.
// GetWithTimeframes prepends the primary timeframe when it is missing from the selection.
func (c *StrategyConfig) getEffectiveTimeframeCount() int {
	selected := c.Indicators.Klines.SelectedTimeframes
	primary := c.Indicators.Klines.PrimaryTimeframe
	if len(selected) > 0 {
		count := len(selected)
		if primary != "" {
			for _, tf := range selected {
				if tf == primary {
					return count
				}
			}
			count++ // primary timeframe is fetched and rendered in addition to the selection
		}
		return count
	}
	count := 1
	if c.Indicators.Klines.LongerTimeframe != "" {
		count++
	}
	return count
}
