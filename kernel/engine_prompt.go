package kernel

import (
	"fmt"
	"nofx/market"
	"nofx/store"
	"strconv"
	"strings"
	"time"
)

// ============================================================================
// Prompt Building - System Prompt
// ============================================================================

// BuildSystemPrompt builds the System Prompt according to the strategy
// configuration. principal is the account principal that positions are sized
// from (D22: open notional = principal × position_percent), not the equity.
func (e *StrategyEngine) BuildSystemPrompt(principal float64, variant string) string {
	var sb strings.Builder
	riskControl := e.config.RiskControl
	promptSections := e.config.PromptSections
	lang := e.GetLanguage()
	zh := lang == LangChinese
	singleSymbol, primarySymbol := e.singleSymbolInfo()

	// 0. Data Dictionary & Schema (ensure AI understands all fields)
	sb.WriteString(GetSchemaPrompt(lang))
	sb.WriteString("\n\n")
	sb.WriteString("---\n\n")

	// 1. Role definition (editable; falls back to a generic intro in the
	//    correct language so we don't mix EN headings with ZH custom text).
	if promptSections.RoleDefinition != "" {
		sb.WriteString(promptSections.RoleDefinition)
		sb.WriteString("\n\n")
	} else if zh {
		sb.WriteString("# 你是一名专业的加密货币永续合约交易 AI\n\n")
		sb.WriteString("你的任务是基于提供的市场数据做出交易决策。\n\n")
	} else {
		sb.WriteString("# You are a professional crypto perpetual futures trading AI\n\n")
		sb.WriteString("Your task is to make trading decisions based on the provided market data.\n\n")
	}

	// 2. Trading mode variant
	writeModeVariant(&sb, variant, zh)

	// 3. Hard constraints (risk control).
	//
	// Open-position notional is principal × position_percent, computed and
	// enforced by the engine (D22/D23). ClampLimits guarantees a sane percent;
	// the guard below only covers hand-built configs that skipped normalization.
	positionPercent := riskControl.PositionPercent
	if positionPercent <= 0 {
		positionPercent = store.DefaultPositionPercent
	}

	writeHardConstraints(&sb, principal, riskControl, positionPercent, singleSymbol, primarySymbol, zh)

	// 4. Trading frequency (editable)
	if promptSections.TradingFrequency != "" {
		sb.WriteString(promptSections.TradingFrequency)
		sb.WriteString("\n\n")
	} else if zh {
		sb.WriteString("# ⏱️ 交易频率提醒\n\n")
		sb.WriteString("- 优秀交易员: 每日 2-4 单 ≈ 每小时 0.1-0.2 单\n")
		sb.WriteString("- 每小时 > 2 单 = 过度交易\n")
		sb.WriteString("- 单笔持仓时长 ≥ 30-60 分钟\n")
		sb.WriteString("如果你发现自己每个周期都在交易 → 入场标准过低; 如果不到 30 分钟就平仓 → 太冲动。\n\n")
	} else {
		sb.WriteString("# ⏱️ Trading Frequency Awareness\n\n")
		sb.WriteString("- Excellent traders: 2-4 trades/day ≈ 0.1-0.2 trades/hour\n")
		sb.WriteString("- >2 trades/hour = overtrading\n")
		sb.WriteString("- Single position hold time ≥ 30-60 minutes\n")
		sb.WriteString("If you find yourself trading every cycle → standards too low; if closing positions < 30 minutes → too impulsive.\n\n")
	}

	// 5. Entry standards (editable)
	if promptSections.EntryStandards != "" {
		sb.WriteString(promptSections.EntryStandards)
		if zh {
			sb.WriteString("\n\n你拥有以下指标数据:\n")
		} else {
			sb.WriteString("\n\nYou have the following indicator data:\n")
		}
		e.writeAvailableIndicators(&sb, zh)
		if zh {
			sb.WriteString(fmt.Sprintf("\n**置信度 ≥ %d** 才能开仓。\n\n", riskControl.MinConfidence))
		} else {
			sb.WriteString(fmt.Sprintf("\n**Confidence ≥ %d** required to open positions.\n\n", riskControl.MinConfidence))
		}
	} else if zh {
		sb.WriteString("# 🎯 入场标准 (严格)\n\n")
		sb.WriteString("只有当多重信号共振时才开仓。你拥有:\n")
		e.writeAvailableIndicators(&sb, zh)
		sb.WriteString(fmt.Sprintf("\n请自由使用任何有效的分析方法, 但**置信度 ≥ %d** 才能开仓; 避免低质量行为, 如单一指标、信号矛盾、横盘震荡、平仓后立刻再开等。\n\n", riskControl.MinConfidence))
	} else {
		sb.WriteString("# 🎯 Entry Standards (Strict)\n\n")
		sb.WriteString("Only open positions when multiple signals resonate. You have:\n")
		e.writeAvailableIndicators(&sb, zh)
		sb.WriteString(fmt.Sprintf("\nFeel free to use any effective analysis method, but **confidence ≥ %d** is required to open positions; avoid low-quality behaviors such as single-indicator entries, contradictory signals, sideways chop, or re-entering immediately after a close.\n\n", riskControl.MinConfidence))
	}

	// 6. Decision process (editable)
	if promptSections.DecisionProcess != "" {
		sb.WriteString(promptSections.DecisionProcess)
		sb.WriteString("\n\n")
	} else if zh {
		sb.WriteString("# 📋 决策流程\n\n")
		sb.WriteString("1. 检查持仓 → 是否需要止盈止损\n")
		sb.WriteString("2. 扫描候选标的 + 多周期 → 是否有强信号\n")
		sb.WriteString("3. 先写思维链, 再输出结构化 JSON\n\n")
	} else {
		sb.WriteString("# 📋 Decision Process\n\n")
		sb.WriteString("1. Check positions → take profit / stop loss?\n")
		sb.WriteString("2. Scan candidates + multi-timeframe → are there strong signals?\n")
		sb.WriteString("3. Write chain of thought first, then output structured JSON\n\n")
	}

	// 7. Output format — schema spec stays in English (this is a parser
	//    contract; reasoning copy is localized below).
	writeOutputFormat(&sb, riskControl, singleSymbol, primarySymbol, zh)

	// 8. Custom Prompt.
	customPrompt := e.config.CustomPrompt

	if customPrompt != "" {
		if zh {
			sb.WriteString("# 📌 个性化交易策略\n\n")
		} else {
			sb.WriteString("# 📌 Personalized Trading Strategy\n\n")
		}
		sb.WriteString(customPrompt)
		sb.WriteString("\n\n")
		if zh {
			sb.WriteString("说明: 上述个性化策略是基础规则的补充, 不能违反基础风控原则。\n")
		} else {
			sb.WriteString("Note: the above personalized strategy supplements the basic rules and may not violate the core risk controls.\n")
		}
	}

	return sb.String()
}

// singleSymbolInfo returns (true, "ARM-USDC") for static-coin strategies that
// trade exactly one instrument. Multi-symbol strategies return (false, "").
// The flag is used to drop crypto-specific "BTC/ETH vs Altcoin" labeling and
// to put the actual trading symbol into the JSON example.
func (e *StrategyEngine) singleSymbolInfo() (bool, string) {
	coinSource := e.config.CoinSource
	if len(coinSource.StaticCoins) == 1 {
		return true, strings.ToUpper(strings.TrimSpace(coinSource.StaticCoins[0]))
	}
	return false, ""
}

func writeModeVariant(sb *strings.Builder, variant string, zh bool) {
	switch strings.ToLower(strings.TrimSpace(variant)) {
	case "aggressive":
		if zh {
			sb.WriteString("## 模式: 激进\n- 优先捕捉趋势突破, 置信度 ≥ 70 时可分批建仓\n- 允许更高仓位, 但必须严格止损并说明风险回报比\n\n")
		} else {
			sb.WriteString("## Mode: Aggressive\n- Prioritize capturing trend breakouts; may scale in when confidence ≥ 70\n- Allow larger positions, but must strictly set stop-loss and explain the risk-reward ratio\n\n")
		}
	case "conservative":
		if zh {
			sb.WriteString("## 模式: 保守\n- 只有当多重信号共振时才开仓\n- 优先保本, 连亏后必须暂停多个周期\n\n")
		} else {
			sb.WriteString("## Mode: Conservative\n- Open positions only when multiple signals resonate\n- Prioritize capital preservation; pause for multiple periods after consecutive losses\n\n")
		}
	case "scalping":
		if zh {
			sb.WriteString("## 模式: 短线\n- 关注短期动量, 利润目标较小但要求迅速行动\n- 价格两根 K 线内未按预期走 → 立即减仓或止损\n\n")
		} else {
			sb.WriteString("## Mode: Scalping\n- Focus on short-term momentum, smaller profit targets but require quick action\n- If price doesn't move as expected within two bars, immediately reduce position or stop-loss\n\n")
		}
	}
}

func writeHardConstraints(sb *strings.Builder, principal float64, riskControl store.RiskControlConfig, positionPercent float64, singleSymbol bool, primarySymbol string, zh bool) {
	if zh {
		sb.WriteString("# 风控硬约束\n\n")
		sb.WriteString("## 代码强制 (后端校验, 无法绕过):\n")
		sb.WriteString(fmt.Sprintf("- 最大持仓数: 同时 %d 个标的\n", riskControl.MaxPositions))
	} else {
		sb.WriteString("# Hard Constraints (Risk Control)\n\n")
		sb.WriteString("## CODE ENFORCED (backend validation, cannot be bypassed):\n")
		sb.WriteString(fmt.Sprintf("- Max Positions: %d instruments simultaneously\n", riskControl.MaxPositions))
	}

	// Open-position notional is computed and enforced by the engine from the
	// account principal (D22/D24). The prompt mirrors the number so the model
	// can reason about risk; it never outputs an amount.
	notional, err := openNotional(principal, positionPercent)
	if err != nil {
		notional = 0
	}
	// The USDT form of the 2% loss cap helps the model pick a protection distance;
	// the decision is refused when the loss at the stop exceeds it.
	lossLimit := 0.0
	if finitePositive(principal) {
		lossLimit = principal * MaxLossPercentOfPrincipal / 100
	}
	if zh {
		sb.WriteString(fmt.Sprintf("- 开仓名义价值: %.2f USDT (= 本金 %.2f × %.1f%%, 由代码计算并强制)\n", notional, principal, positionPercent))
		sb.WriteString(fmt.Sprintf("- 价格到止损价的亏损: ≤ 本金的 %.0f%% (≈%.2f USDT, 代码强制, 超过即拒绝开仓)\n",
			MaxLossPercentOfPrincipal, lossLimit))
	} else {
		sb.WriteString(fmt.Sprintf("- Open Position Notional: %.2f USDT (= principal %.2f × %.1f%%, computed and enforced by code)\n", notional, principal, positionPercent))
		sb.WriteString(fmt.Sprintf("- Loss at Stop Loss: ≤ %.0f%% of principal (≈%.2f USDT, code enforced, the decision is refused otherwise)\n",
			MaxLossPercentOfPrincipal, lossLimit))
	}

	if zh {
		sb.WriteString(fmt.Sprintf("- 最大保证金占用: ≤%.0f%%\n", riskControl.MaxMarginUsage*100))
		sb.WriteString(fmt.Sprintf("- 最小下单金额: ≥%.0f USDT (按合约最小步长取整后校验)\n\n", riskControl.MinPositionSize))
		sb.WriteString("## AI 建议 (推荐遵循):\n")
	} else {
		sb.WriteString(fmt.Sprintf("- Max Margin Usage: ≤%.0f%%\n", riskControl.MaxMarginUsage*100))
		sb.WriteString(fmt.Sprintf("- Min Position Size: ≥%.0f USDT (checked after rounding to the contract size step)\n\n", riskControl.MinPositionSize))
		sb.WriteString("## AI GUIDED (recommended):\n")
	}

	if singleSymbol {
		lev := riskControl.AltcoinMaxLeverage
		if riskControl.BTCETHMaxLeverage > lev {
			lev = riskControl.BTCETHMaxLeverage
		}
		if zh {
			sb.WriteString(fmt.Sprintf("- 交易杠杆 (%s): 最高 %dx\n", primarySymbol, lev))
		} else {
			sb.WriteString(fmt.Sprintf("- Trading Leverage (%s): max %dx\n", primarySymbol, lev))
		}
	} else {
		if zh {
			sb.WriteString(fmt.Sprintf("- 交易杠杆: 山寨币/股票 最高 %dx | BTC/ETH 最高 %dx\n", riskControl.AltcoinMaxLeverage, riskControl.BTCETHMaxLeverage))
		} else {
			sb.WriteString(fmt.Sprintf("- Trading Leverage: Altcoin/Stock max %dx | BTC/ETH max %dx\n", riskControl.AltcoinMaxLeverage, riskControl.BTCETHMaxLeverage))
		}
	}
	if zh {
		sb.WriteString(fmt.Sprintf("- 风险回报比: ≥1:%.1f (take_profit / stop_loss)\n", riskControl.MinRiskRewardRatio))
		sb.WriteString(fmt.Sprintf("- 最小置信度: ≥%d 才开仓\n\n", riskControl.MinConfidence))
	} else {
		sb.WriteString(fmt.Sprintf("- Risk-Reward Ratio: ≥1:%.1f (take_profit / stop_loss)\n", riskControl.MinRiskRewardRatio))
		sb.WriteString(fmt.Sprintf("- Min Confidence: ≥%d to open position\n\n", riskControl.MinConfidence))
	}

	// Position sizing guidance — the amount comes from the account principal
	// and the strategy percent, not from the model; confidence decides whether
	// to act at all.
	if zh {
		sb.WriteString("## 仓位大小指引\n")
		sb.WriteString("开仓名义价值由代码计算并强制, 不来自你的输出:\n")
		sb.WriteString(fmt.Sprintf("- 名义价值 = 本金 %.2f × 策略百分比 %.1f%% = %.2f USDT\n", principal, positionPercent, notional))
		sb.WriteString("- 开仓决策只给出方向、杠杆、止损价、止盈价与置信度; 不要输出任何金额字段\n")
		sb.WriteString(fmt.Sprintf("- 价格到止损价的亏损不得超过本金的 %.0f%%, 否则该决策会被系统拒绝\n", MaxLossPercentOfPrincipal))
		sb.WriteString("- 置信不足或行情不利时选择等待, 不要试图放大或缩小金额\n\n")
	} else {
		sb.WriteString("## Position Sizing Guidance\n")
		sb.WriteString("The open notional is computed and enforced by code — it does not come from your output:\n")
		sb.WriteString(fmt.Sprintf("- Notional = principal %.2f × strategy percent %.1f%% = %.2f USDT\n", principal, positionPercent, notional))
		sb.WriteString("- An opening decision supplies only direction, leverage, stop loss, take profit and confidence; never output an amount field\n")
		sb.WriteString(fmt.Sprintf("- The loss at the stop loss must not exceed %.0f%% of the principal, otherwise the decision is refused\n", MaxLossPercentOfPrincipal))
		sb.WriteString("- When confidence is low or the setup is unfavourable, wait — do not resize the amount on your own\n\n")
	}
}

func writeOutputFormat(sb *strings.Builder, riskControl store.RiskControlConfig, singleSymbol bool, primarySymbol string, zh bool) {
	// Output format schema MUST stay English/structural; parser depends on it.
	sb.WriteString("# Output Format (Strictly Follow)\n\n")
	if zh {
		sb.WriteString("**必须使用 XML 标签 <reasoning> 和 <decision> 分隔思维链和决策 JSON, 避免解析错误**\n\n")
	} else {
		sb.WriteString("**Must use XML tags <reasoning> and <decision> to separate chain of thought and decision JSON, avoiding parsing errors**\n\n")
	}
	sb.WriteString("## Format Requirements\n\n")
	sb.WriteString("<reasoning>\n")
	if zh {
		sb.WriteString("你的思维链分析...\n- 简明分析你的思考过程\n")
	} else {
		sb.WriteString("Your chain of thought analysis...\n- Briefly analyze your thinking process\n")
	}
	sb.WriteString("</reasoning>\n\n")
	sb.WriteString("<decision>\n")
	if zh {
		sb.WriteString("步骤 2: JSON 决策数组\n\n")
	} else {
		sb.WriteString("Step 2: JSON decision array\n\n")
	}
	sb.WriteString("```json\n[\n")

	// Build a JSON example using the actual trading symbol when the strategy
	// is single-symbol. Falls back to the legacy BTC/ETH two-line example
	// only for multi-symbol strategies that genuinely have BTC/ETH on tap.
	if singleSymbol {
		lev := riskControl.AltcoinMaxLeverage
		if riskControl.BTCETHMaxLeverage > lev {
			lev = riskControl.BTCETHMaxLeverage
		}
		sb.WriteString(fmt.Sprintf("  {\"symbol\": \"%s\", \"action\": \"open_long\", \"leverage\": %d, \"stop_loss\": 0, \"take_profit\": 0, \"confidence\": 85},\n", primarySymbol, lev))
		sb.WriteString(fmt.Sprintf("  {\"symbol\": \"%s\", \"action\": \"wait\"}\n", primarySymbol))
	} else {
		sb.WriteString(fmt.Sprintf("  {\"symbol\": \"BTCUSDT\", \"action\": \"open_short\", \"leverage\": %d, \"stop_loss\": 97000, \"take_profit\": 91000, \"confidence\": 85},\n",
			riskControl.BTCETHMaxLeverage))
		sb.WriteString("  {\"symbol\": \"ETHUSDT\", \"action\": \"close_long\"}\n")
	}
	sb.WriteString("]\n```\n")
	sb.WriteString("</decision>\n\n")

	// 分批平仓单独给格式示例：不放进上面的决策数组，避免示例看起来像
	// “同一轮先开仓再分批平仓”的连续动作（它只应在持仓标记 available 时使用）。
	partialSymbol, partialAction := "BTCUSDT", "close_short_partial"
	if singleSymbol {
		partialSymbol, partialAction = primarySymbol, "close_long_partial"
	}
	// 修改止损止盈同样单独给格式示例，避免示例看起来像“开仓同一轮就改保护”。
	protectionSymbol := "BTCUSDT"
	if singleSymbol {
		protectionSymbol = primarySymbol
	}

	if zh {
		sb.WriteString("## 字段说明\n\n")
		sb.WriteString("- `action`: open_long | open_short | close_long | close_short | close_long_partial | close_short_partial | update_stop_loss | update_take_profit | hold | wait\n")
		sb.WriteString(fmt.Sprintf("- `confidence`: 0-100 (开仓建议 ≥ %d)\n", riskControl.MinConfidence))
		sb.WriteString("- 开仓时必填: leverage, stop_loss, take_profit, confidence\n")
		sb.WriteString("- 开仓金额由系统按本金 × 百分比计算, 决策 JSON 不需要也无法指定金额字段\n")
		sb.WriteString("- 分批平仓: `close_long_partial` / `close_short_partial` 仅用于持仓行标注 `Partial Close: available` 的仓位; `close_percent` 取 0-100 (不含 100), 系统按合约最小步长向下取整换算张数, 过小的比例会被拒绝; 100% 是全平, 请用 `close_long` / `close_short`\n")
		sb.WriteString(fmt.Sprintf("- 分批平仓示例: {\"symbol\": \"%s\", \"action\": \"%s\", \"close_percent\": 50} (仅格式示例, 只在持仓标记 available 时使用)\n", partialSymbol, partialAction))
		sb.WriteString("- 修改止损止盈: `update_stop_loss` / `update_take_profit` 仅用于持仓行显示具体 SL / TP 价格的仓位; 新价格填在 `stop_loss` / `take_profit` (另一字段忽略); 多单止损须低于现价、止盈须高于现价, 空单相反; 到止损价的亏损不得超过本金的 2%, 否则被拒绝; 系统按合约价格步长对齐 (止损只会更紧、止盈不会更近)\n")
		sb.WriteString(fmt.Sprintf("- 修改止损示例: {\"symbol\": \"%s\", \"action\": \"update_stop_loss\", \"stop_loss\": 0.05} (仅格式示例, 只在持仓显示 SL / TP 时使用)\n", protectionSymbol))
		sb.WriteString("- 示例里的 stop_loss / take_profit 数值只是占位, 必须替换为本轮行情的真实价格 (0 会被系统拒绝)\n")
		sb.WriteString("- **重要**: 所有数值必须是算好的数字, 不能是公式/表达式 (例如写 `27.76`, 不要写 `3000 * 0.01`)\n")
		if singleSymbol {
			sb.WriteString(fmt.Sprintf("- **本策略只交易 %s**, JSON 中的 `symbol` 必须**完全等于** `%s`, 不要写成 `%s` 去掉后缀或加 USDT 的变体。\n", primarySymbol, primarySymbol, primarySymbol))
		}
		sb.WriteString("\n")
	} else {
		sb.WriteString("## Field Description\n\n")
		sb.WriteString("- `action`: open_long | open_short | close_long | close_short | close_long_partial | close_short_partial | update_stop_loss | update_take_profit | hold | wait\n")
		sb.WriteString(fmt.Sprintf("- `confidence`: 0-100 (opening recommended ≥ %d)\n", riskControl.MinConfidence))
		sb.WriteString("- Required when opening: leverage, stop_loss, take_profit, confidence\n")
		sb.WriteString("- The open notional is computed by the system from principal × percent; the decision JSON neither needs nor accepts an amount field\n")
		sb.WriteString("- Partial close: use `close_long_partial` / `close_short_partial` only for positions marked `Partial Close: available`; `close_percent` is in 0-100 (excluding 100), the system floors the resulting contracts to the contract size step and refuses ratios that round below one step; 100% is a full close, use `close_long` / `close_short`\n")
		sb.WriteString(fmt.Sprintf("- Partial-close example: {\"symbol\": \"%s\", \"action\": \"%s\", \"close_percent\": 50} (format only; use it only when the position is marked available)\n", partialSymbol, partialAction))
		sb.WriteString("- Update protection: `update_stop_loss` / `update_take_profit` only for positions that show concrete SL / TP prices; put the new price in `stop_loss` / `take_profit` (the other field is ignored); a long stop must be below the current price and a long take profit above it (short reversed); the loss at the stop must not exceed 2% of the principal or the decision is refused; the system aligns the price to the contract price step (a stop only gets tighter, a take profit never gets closer)\n")
		sb.WriteString(fmt.Sprintf("- Update-stop-loss example: {\"symbol\": \"%s\", \"action\": \"update_stop_loss\", \"stop_loss\": 0.05} (format only; use it only when the position shows SL / TP)\n", protectionSymbol))
		sb.WriteString("- The stop_loss / take_profit values in the example are placeholders; replace them with real prices from this cycle (0 is refused by the system)\n")
		sb.WriteString("- **IMPORTANT**: all numeric values must be calculated numbers, NOT formulas/expressions (e.g. use `27.76`, not `3000 * 0.01`)\n")
		if singleSymbol {
			sb.WriteString(fmt.Sprintf("- **This strategy trades only %s.** The JSON `symbol` MUST match `%s` exactly — do not add USDT/USDC suffix variants.\n", primarySymbol, primarySymbol))
		}
		sb.WriteString("\n")
	}
}

func (e *StrategyEngine) writeAvailableIndicators(sb *strings.Builder, zh bool) {
	indicators := e.config.Indicators
	kline := indicators.Klines

	label := func(en, zhStr string) string {
		if zh {
			return zhStr
		}
		return en
	}

	if zh {
		sb.WriteString(fmt.Sprintf("- %s 价格序列", kline.PrimaryTimeframe))
		if kline.EnableMultiTimeframe {
			sb.WriteString(fmt.Sprintf(" + %s K 线序列\n", kline.LongerTimeframe))
		} else {
			sb.WriteString("\n")
		}
	} else {
		sb.WriteString(fmt.Sprintf("- %s price series", kline.PrimaryTimeframe))
		if kline.EnableMultiTimeframe {
			sb.WriteString(fmt.Sprintf(" + %s K-line series\n", kline.LongerTimeframe))
		} else {
			sb.WriteString("\n")
		}
	}

	if indicators.EnableEMA {
		sb.WriteString("- " + label("EMA indicators", "EMA 指标"))
		if len(indicators.EMAPeriods) > 0 {
			sb.WriteString(fmt.Sprintf(" (%s: %v)", label("periods", "周期"), indicators.EMAPeriods))
		}
		sb.WriteString("\n")
	}
	if indicators.EnableMACD {
		sb.WriteString("- " + label("MACD indicators", "MACD 指标") + "\n")
	}
	if indicators.EnableRSI {
		sb.WriteString("- " + label("RSI indicators", "RSI 指标"))
		if len(indicators.RSIPeriods) > 0 {
			sb.WriteString(fmt.Sprintf(" (%s: %v)", label("periods", "周期"), indicators.RSIPeriods))
		}
		sb.WriteString("\n")
	}
	if indicators.EnableATR {
		sb.WriteString("- " + label("ATR indicators", "ATR 指标"))
		if len(indicators.ATRPeriods) > 0 {
			sb.WriteString(fmt.Sprintf(" (%s: %v)", label("periods", "周期"), indicators.ATRPeriods))
		}
		sb.WriteString("\n")
	}
	if indicators.EnableBOLL {
		sb.WriteString("- " + label("Bollinger Bands (BOLL) - Upper/Middle/Lower bands", "布林带 (BOLL) - 上/中/下轨"))
		if len(indicators.BOLLPeriods) > 0 {
			sb.WriteString(fmt.Sprintf(" (%s: %v)", label("periods", "周期"), indicators.BOLLPeriods))
		}
		sb.WriteString("\n")
	}
	if indicators.EnableVolume {
		sb.WriteString("- " + label("Volume data (USDT quote volume)", "成交量数据（USDT 成交额）") + "\n")
	}
	if indicators.EnableOI {
		sb.WriteString("- " + label("Open Interest (OI) data", "持仓量 (OI) 数据") + "\n")
	}
	if indicators.EnableFundingRate {
		sb.WriteString("- " + label("Funding rate", "资金费率") + "\n")
	}
}

// ============================================================================
// Prompt Building - User Prompt
// ============================================================================

// BuildUserPrompt builds User Prompt based on strategy configuration
func (e *StrategyEngine) BuildUserPrompt(ctx *Context) string {
	var sb strings.Builder

	// System status
	sb.WriteString(fmt.Sprintf("Time: %s | Period: #%d | Runtime: %d minutes\n\n",
		ctx.CurrentTime, ctx.CallCount, ctx.RuntimeMinutes))

	// BTC market
	if btcData, hasBTC := ctx.MarketDataMap["BTCUSDT"]; hasBTC {
		sb.WriteString(fmt.Sprintf("BTC: %.2f (1h: %+.2f%%, 4h: %+.2f%%) | MACD: %.4f | RSI: %.2f\n\n",
			btcData.CurrentPrice, btcData.PriceChange1h, btcData.PriceChange4h,
			btcData.CurrentMACD, btcData.CurrentRSI7))
	}

	// Account information
	sb.WriteString(fmt.Sprintf("Account: Equity %.2f | Balance %.2f (%.1f%%) | PnL %+.2f%% | Margin %.1f%% | Positions %d\n\n",
		ctx.Account.TotalEquity,
		ctx.Account.AvailableBalance,
		(ctx.Account.AvailableBalance/ctx.Account.TotalEquity)*100,
		ctx.Account.TotalPnLPct,
		ctx.Account.MarginUsedPct,
		ctx.Account.PositionCount))

	// Recently completed orders (placed before positions to ensure visibility)
	if len(ctx.RecentOrders) > 0 {
		sb.WriteString("## Recent Completed Trades\n")
		for i, order := range ctx.RecentOrders {
			resultStr := "Profit"
			if order.RealizedPnL < 0 {
				resultStr = "Loss"
			}
			sb.WriteString(fmt.Sprintf("%d. %s %s | Entry %.4f Exit %.4f | %s: %+.2f USDT (%+.2f%%) | %s→%s (%s)\n",
				i+1, order.Symbol, order.Side,
				order.EntryPrice, order.ExitPrice,
				resultStr, order.RealizedPnL, order.PnLPct,
				order.EntryTime, order.ExitTime, order.HoldDuration))
		}
		sb.WriteString("\n")
	}

	// Historical trading statistics (helps AI understand past performance)
	if ctx.TradingStats != nil && ctx.TradingStats.TotalTrades > 0 {
		// Get language from strategy config
		lang := e.GetLanguage()

		// Win/Loss ratio
		var winLossRatio float64
		if ctx.TradingStats.AvgLoss > 0 {
			winLossRatio = ctx.TradingStats.AvgWin / ctx.TradingStats.AvgLoss
		}

		if lang == LangChinese {
			sb.WriteString("## 历史交易统计\n")
			sb.WriteString(fmt.Sprintf("总交易: %d 笔 | 盈利因子: %.2f | 夏普比率: %.2f | 盈亏比: %.2f\n",
				ctx.TradingStats.TotalTrades,
				ctx.TradingStats.ProfitFactor,
				ctx.TradingStats.SharpeRatio,
				winLossRatio))
			sb.WriteString(fmt.Sprintf("总盈亏: %+.2f USDT | 平均盈利: +%.2f | 平均亏损: -%.2f | 最大回撤: %.1f%%\n",
				ctx.TradingStats.TotalPnL,
				ctx.TradingStats.AvgWin,
				ctx.TradingStats.AvgLoss,
				ctx.TradingStats.MaxDrawdownPct))

			// Performance hints based on profit factor, sharpe, and drawdown
			if ctx.TradingStats.ProfitFactor >= 1.5 && ctx.TradingStats.SharpeRatio >= 1 {
				sb.WriteString("表现: 良好 - 保持当前策略\n")
			} else if ctx.TradingStats.ProfitFactor < 1 {
				sb.WriteString("表现: 需改进 - 提高盈亏比，优化止盈止损\n")
			} else if ctx.TradingStats.MaxDrawdownPct > 30 {
				sb.WriteString("表现: 风险偏高 - 减少仓位，控制回撤\n")
			} else {
				sb.WriteString("表现: 正常 - 有优化空间\n")
			}
		} else {
			sb.WriteString("## Historical Trading Statistics\n")
			sb.WriteString(fmt.Sprintf("Total Trades: %d | Profit Factor: %.2f | Sharpe: %.2f | Win/Loss Ratio: %.2f\n",
				ctx.TradingStats.TotalTrades,
				ctx.TradingStats.ProfitFactor,
				ctx.TradingStats.SharpeRatio,
				winLossRatio))
			sb.WriteString(fmt.Sprintf("Total PnL: %+.2f USDT | Avg Win: +%.2f | Avg Loss: -%.2f | Max Drawdown: %.1f%%\n",
				ctx.TradingStats.TotalPnL,
				ctx.TradingStats.AvgWin,
				ctx.TradingStats.AvgLoss,
				ctx.TradingStats.MaxDrawdownPct))

			// Performance hints based on profit factor, sharpe, and drawdown
			if ctx.TradingStats.ProfitFactor >= 1.5 && ctx.TradingStats.SharpeRatio >= 1 {
				sb.WriteString("Performance: GOOD - maintain current strategy\n")
			} else if ctx.TradingStats.ProfitFactor < 1 {
				sb.WriteString("Performance: NEEDS IMPROVEMENT - improve win/loss ratio, optimize TP/SL\n")
			} else if ctx.TradingStats.MaxDrawdownPct > 30 {
				sb.WriteString("Performance: HIGH RISK - reduce position size, control drawdown\n")
			} else {
				sb.WriteString("Performance: NORMAL - room for optimization\n")
			}
		}
		sb.WriteString("\n")
	}

	// Position information
	if len(ctx.Positions) > 0 {
		sb.WriteString("## Current Positions\n")
		for i, pos := range ctx.Positions {
			sb.WriteString(e.formatPositionInfo(i+1, pos, ctx))
		}
		sb.WriteString("Partial close: only positions marked \"Partial Close: available\" may be reduced via close_long_partial / close_short_partial. close_percent is the percentage of the current position (0-100, excluding 100); the system floors it to the contract size step and refuses ratios that round below one step. 100% is a full close — use close_long / close_short.\n\n")
		sb.WriteString("Protection update: only positions that show concrete SL / TP prices may be changed via update_stop_loss / update_take_profit. The new price goes in stop_loss / take_profit; a long stop must stay below the current price and a long take profit above it (short reversed); the loss at the stop must not exceed 2% of the principal. Positions marked \"Protection: unknown\" must not be modified.\n\n")
	} else {
		sb.WriteString("Current Positions: None\n\n")
	}

	// Market environment context (between Current Positions and Candidate Coins;
	// only for strategies with market_context enabled and ai_trading)
	WriteMarketEnv(&sb, ctx.MarketEnv, *e.config, e.GetLanguage())

	// Candidate coins (exclude coins already in positions to avoid duplicate data)
	positionSymbols := make(map[string]bool)
	for _, pos := range ctx.Positions {
		// Normalize symbol to handle both "ETH" and "ETHUSDT" formats
		normalizedSymbol := market.Normalize(pos.Symbol)
		positionSymbols[normalizedSymbol] = true
	}

	sb.WriteString(fmt.Sprintf("## Candidate Coins (%d coins)\n\n", len(ctx.MarketDataMap)))
	displayedCount := 0
	for _, coin := range ctx.CandidateCoins {
		// Skip if this coin is already a position (data already shown in positions section)
		normalizedCoinSymbol := market.Normalize(coin.Symbol)
		if positionSymbols[normalizedCoinSymbol] {
			continue
		}

		marketData, hasData := ctx.MarketDataMap[coin.Symbol]
		if !hasData {
			continue
		}
		displayedCount++

		sb.WriteString(fmt.Sprintf("### %d. %s\n\n", displayedCount, coin.Symbol))
		sb.WriteString(e.formatMarketData(marketData))

		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	sb.WriteString("---\n\n")
	sb.WriteString("Now please analyze and output your decision (Chain of Thought + JSON)\n")

	return sb.String()
}

func (e *StrategyEngine) formatPositionInfo(index int, pos PositionInfo, ctx *Context) string {
	var sb strings.Builder

	holdingDuration := ""
	if pos.UpdateTime > 0 {
		durationMs := time.Now().UnixMilli() - pos.UpdateTime
		durationMin := durationMs / (1000 * 60)
		if durationMin < 60 {
			holdingDuration = fmt.Sprintf(" | Holding Duration %d min", durationMin)
		} else {
			durationHour := durationMin / 60
			durationMinRemainder := durationMin % 60
			holdingDuration = fmt.Sprintf(" | Holding Duration %dh %dm", durationHour, durationMinRemainder)
		}
	}

	positionValue := pos.Quantity * pos.MarkPrice
	if positionValue < 0 {
		positionValue = -positionValue
	}

	partialClose := " | Partial Close: unavailable"
	if pos.CanPartialClose {
		partialClose = " | Partial Close: available"
	} else if reason := strings.TrimSpace(pos.PartialCloseUnavailableReason); reason != "" {
		partialClose = fmt.Sprintf(" | Partial Close: unavailable (%s)", reason)
	}

	// 当前止损止盈：每轮从交易所条件单读取；不可读时明确标注，模型不得修改该持仓的保护。
	protection := ""
	if pos.StopLoss > 0 && pos.TakeProfit > 0 {
		protection = fmt.Sprintf(" | SL %s | TP %s",
			strconv.FormatFloat(pos.StopLoss, 'f', -1, 64), strconv.FormatFloat(pos.TakeProfit, 'f', -1, 64))
	} else {
		reason := strings.TrimSpace(pos.ProtectionUnavailableReason)
		if reason == "" {
			reason = "not readable"
		}
		protection = fmt.Sprintf(" | Protection: unknown (%s)", reason)
	}

	sb.WriteString(fmt.Sprintf("%d. %s %s | Entry %.4f Current %.4f | Qty %.4f | Position Value %.2f USDT | PnL%+.2f%% | PnL Amount%+.2f USDT | Peak PnL%.2f%% | Leverage %dx | Margin %.0f | Liq Price %.4f%s%s%s\n\n",
		index, pos.Symbol, strings.ToUpper(pos.Side),
		pos.EntryPrice, pos.MarkPrice, pos.Quantity, positionValue, pos.UnrealizedPnLPct, pos.UnrealizedPnL, pos.PeakPnLPct,
		pos.Leverage, pos.MarginUsed, pos.LiquidationPrice, holdingDuration, partialClose, protection))

	if marketData, ok := ctx.MarketDataMap[pos.Symbol]; ok {
		sb.WriteString(e.formatMarketData(marketData))

		sb.WriteString("\n")
	}

	return sb.String()
}

// ============================================================================
// Market Data Formatting
// ============================================================================

func (e *StrategyEngine) formatMarketData(data *market.Data) string {
	var sb strings.Builder
	indicators := e.config.Indicators

	// Clearly label the coin symbol
	sb.WriteString(fmt.Sprintf("=== %s Market Data ===\n\n", data.Symbol))
	sb.WriteString(fmt.Sprintf("current_price = %.4f", data.CurrentPrice))

	if indicators.EnableEMA {
		sb.WriteString(fmt.Sprintf(", current_ema20 = %.3f", data.CurrentEMA20))
	}

	if indicators.EnableMACD {
		sb.WriteString(fmt.Sprintf(", current_macd = %.3f", data.CurrentMACD))
	}

	if indicators.EnableRSI {
		sb.WriteString(fmt.Sprintf(", current_rsi7 = %.3f", data.CurrentRSI7))
	}

	sb.WriteString("\n\n")

	if indicators.EnableOI || indicators.EnableFundingRate {
		sb.WriteString(fmt.Sprintf("Additional data for %s:\n\n", data.Symbol))

		if indicators.EnableOI {
			if data.OpenInterest != nil {
				sb.WriteString(fmt.Sprintf("Open Interest (USD): Latest: %.2f Average: %.2f (1h average)\n\n",
					data.OpenInterest.Latest, data.OpenInterest.Average))
			} else {
				sb.WriteString("Open Interest (USD): [unavailable]\n\n")
			}
		}

		if indicators.EnableFundingRate {
			if data.FundingRate != nil {
				sb.WriteString(fmt.Sprintf("Funding Rate: %.2e\n\n", *data.FundingRate))
			} else {
				sb.WriteString("Funding Rate: [unavailable]\n\n")
			}
		}
	}

	if len(data.TimeframeData) > 0 {
		// D25 保留周期（与市场层 Gate 取数一致；10s/7d/30d 必须出现在提示词里，F41）
		timeframeOrder := []string{"10s", "1m", "5m", "15m", "30m", "1h", "4h", "8h", "1d", "7d", "1w", "30d"}
		for _, tf := range timeframeOrder {
			if tfData, ok := data.TimeframeData[tf]; ok {
				sb.WriteString(fmt.Sprintf("=== %s Timeframe (oldest → latest) ===\n\n", strings.ToUpper(tf)))
				e.formatTimeframeSeriesData(&sb, tfData, indicators)
			}
		}
	} else {
		// Compatible with old data format
		if data.IntradaySeries != nil {
			klineConfig := indicators.Klines
			sb.WriteString(fmt.Sprintf("Intraday series (%s intervals, oldest → latest):\n\n", klineConfig.PrimaryTimeframe))

			if len(data.IntradaySeries.MidPrices) > 0 {
				sb.WriteString(fmt.Sprintf("Mid prices: %s\n\n", formatFloatSlice(data.IntradaySeries.MidPrices)))
			}

			if indicators.EnableEMA && len(data.IntradaySeries.EMA20Values) > 0 {
				sb.WriteString(fmt.Sprintf("EMA indicators (20-period): %s\n\n", formatFloatSlice(data.IntradaySeries.EMA20Values)))
			}

			if indicators.EnableMACD && len(data.IntradaySeries.MACDValues) > 0 {
				sb.WriteString(fmt.Sprintf("MACD indicators: %s\n\n", formatFloatSlice(data.IntradaySeries.MACDValues)))
			}

			if indicators.EnableRSI {
				if len(data.IntradaySeries.RSI7Values) > 0 {
					sb.WriteString(fmt.Sprintf("RSI indicators (7-Period): %s\n\n", formatFloatSlice(data.IntradaySeries.RSI7Values)))
				}
				if len(data.IntradaySeries.RSI14Values) > 0 {
					sb.WriteString(fmt.Sprintf("RSI indicators (14-Period): %s\n\n", formatFloatSlice(data.IntradaySeries.RSI14Values)))
				}
			}

			if indicators.EnableVolume && len(data.IntradaySeries.Volume) > 0 {
				sb.WriteString(fmt.Sprintf("Volume (USDT): %s\n\n", formatFloatSlice(data.IntradaySeries.Volume)))
			}

			if indicators.EnableATR {
				sb.WriteString(fmt.Sprintf("3m ATR (14-period): %.3f\n\n", data.IntradaySeries.ATR14))
			}
		}

		if data.LongerTermContext != nil && indicators.Klines.EnableMultiTimeframe {
			sb.WriteString(fmt.Sprintf("Longer-term context (%s timeframe):\n\n", indicators.Klines.LongerTimeframe))

			if indicators.EnableEMA {
				sb.WriteString(fmt.Sprintf("20-Period EMA: %.3f vs. 50-Period EMA: %.3f\n\n",
					data.LongerTermContext.EMA20, data.LongerTermContext.EMA50))
			}

			if indicators.EnableATR {
				sb.WriteString(fmt.Sprintf("3-Period ATR: %.3f vs. 14-Period ATR: %.3f\n\n",
					data.LongerTermContext.ATR3, data.LongerTermContext.ATR14))
			}

			if indicators.EnableVolume {
				sb.WriteString(fmt.Sprintf("Current Volume (USDT): %.3f vs. Average Volume (USDT): %.3f\n\n",
					data.LongerTermContext.CurrentVolume, data.LongerTermContext.AverageVolume))
			}

			if indicators.EnableMACD && len(data.LongerTermContext.MACDValues) > 0 {
				sb.WriteString(fmt.Sprintf("MACD indicators: %s\n\n", formatFloatSlice(data.LongerTermContext.MACDValues)))
			}

			if indicators.EnableRSI && len(data.LongerTermContext.RSI14Values) > 0 {
				sb.WriteString(fmt.Sprintf("RSI indicators (14-Period): %s\n\n", formatFloatSlice(data.LongerTermContext.RSI14Values)))
			}
		}
	}

	return sb.String()
}

func (e *StrategyEngine) formatTimeframeSeriesData(sb *strings.Builder, data *market.TimeframeSeriesData, indicators store.IndicatorConfig) {
	if len(data.Klines) > 0 {
		sb.WriteString("Time(UTC)      Open      High      Low       Close     Volume(USDT)\n")
		for i, k := range data.Klines {
			t := time.Unix(k.Time/1000, 0).UTC()
			timeStr := t.Format("01-02 15:04")
			marker := ""
			if i == len(data.Klines)-1 {
				marker = "  <- current"
			}
			sb.WriteString(fmt.Sprintf("%-14s %-9.4f %-9.4f %-9.4f %-9.4f %-12.2f%s\n",
				timeStr, k.Open, k.High, k.Low, k.Close, k.Volume, marker))
		}
		sb.WriteString("\n")
	} else if len(data.MidPrices) > 0 {
		sb.WriteString(fmt.Sprintf("Mid prices: %s\n\n", formatFloatSlice(data.MidPrices)))
		if indicators.EnableVolume && len(data.Volume) > 0 {
			sb.WriteString(fmt.Sprintf("Volume (USDT): %s\n\n", formatFloatSlice(data.Volume)))
		}
	}

	if indicators.EnableEMA {
		if len(data.EMA20Values) > 0 {
			sb.WriteString(fmt.Sprintf("EMA20: %s\n", formatFloatSlice(data.EMA20Values)))
		}
		if len(data.EMA50Values) > 0 {
			sb.WriteString(fmt.Sprintf("EMA50: %s\n", formatFloatSlice(data.EMA50Values)))
		}
	}

	if indicators.EnableMACD && len(data.MACDValues) > 0 {
		sb.WriteString(fmt.Sprintf("MACD: %s\n", formatFloatSlice(data.MACDValues)))
	}

	if indicators.EnableRSI {
		if len(data.RSI7Values) > 0 {
			sb.WriteString(fmt.Sprintf("RSI7: %s\n", formatFloatSlice(data.RSI7Values)))
		}
		if len(data.RSI14Values) > 0 {
			sb.WriteString(fmt.Sprintf("RSI14: %s\n", formatFloatSlice(data.RSI14Values)))
		}
	}

	if indicators.EnableATR && data.ATR14 > 0 {
		sb.WriteString(fmt.Sprintf("ATR14: %.4f\n", data.ATR14))
	}

	if indicators.EnableBOLL && len(data.BOLLUpper) > 0 {
		sb.WriteString(fmt.Sprintf("BOLL Upper: %s\n", formatFloatSlice(data.BOLLUpper)))
		sb.WriteString(fmt.Sprintf("BOLL Middle: %s\n", formatFloatSlice(data.BOLLMiddle)))
		sb.WriteString(fmt.Sprintf("BOLL Lower: %s\n", formatFloatSlice(data.BOLLLower)))
	}

	sb.WriteString("\n")
}

func formatFloatSlice(values []float64) string {
	strValues := make([]string, len(values))
	for i, v := range values {
		strValues[i] = fmt.Sprintf("%.4f", v)
	}
	return "[" + strings.Join(strValues, ", ") + "]"
}
