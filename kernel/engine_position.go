package kernel

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	"nofx/logger"
	"nofx/market"
	"nofx/store"
)

// ============================================================================
// Decision Validation & Position Sizing
// ============================================================================

// MaxLossPercentOfPrincipal 价格到止损价时的亏损上限（本金的百分比，D34）。
// 开仓与每次修改止损共用这一上限。
const MaxLossPercentOfPrincipal = 2.0

// DecisionSizing 一次决策的金额与价格输入。
// Principal/PositionPercent 决定开仓名义价值；Prices 用于校验止损止盈方向与亏损上限，
// 取模型在同一轮提示词里看到的现价。
type DecisionSizing struct {
	Principal       float64
	PositionPercent float64
	BTCETHLeverage  int
	AltcoinLeverage int
	Prices          map[string]float64 // symbol → 当前价
}

func validateDecisions(decisions []Decision, sizing DecisionSizing) error {
	for i := range decisions {
		if err := validateDecision(&decisions[i], sizing); err != nil {
			return fmt.Errorf("decision #%d validation failed: %w", i+1, err)
		}
	}
	return nil
}

// validateDecision 校验一条 AI 决策。开仓的名义价值由代码按本金 × 百分比计算并写入
// Decision.NotionalUSD（D22/D24）；模型给出的任何金额字段都不参与。
// 校验包含随单保护的两项硬约束：止损止盈分居现价两侧（否则交易所条件单方向非法）、
// 价格到止损价的亏损不超过本金的 2%（D34）。
func validateDecision(d *Decision, sizing DecisionSizing) error {
	validActions := map[string]bool{
		"open_long":           true,
		"open_short":          true,
		"close_long":          true,
		"close_short":         true,
		"close_long_partial":  true,
		"close_short_partial": true,
		"hold":                true,
		"wait":                true,
	}

	if !validActions[d.Action] {
		return fmt.Errorf("invalid action: %s", d.Action)
	}

	if d.Action == "close_long_partial" || d.Action == "close_short_partial" {
		if math.IsNaN(d.ClosePercent) || math.IsInf(d.ClosePercent, 0) {
			return fmt.Errorf("close_percent must be a finite number, got %v", d.ClosePercent)
		}
		if d.ClosePercent == 100 {
			return fmt.Errorf(
				"close_percent 100 is a full close; use %s instead",
				strings.TrimSuffix(d.Action, "_partial"))
		}
		if d.ClosePercent <= 0 || d.ClosePercent >= 100 {
			return fmt.Errorf("close_percent must be in (0, 100), got %v", d.ClosePercent)
		}
		return nil
	}

	if d.Action != "open_long" && d.Action != "open_short" {
		return nil
	}

	// Asset tiering for the leverage limit:
	//   - BTC/ETH crypto perps use the BTC/ETH leverage tier.
	//   - Everything else uses the altcoin tier.
	maxLeverage := sizing.AltcoinLeverage
	if d.Symbol == "BTCUSDT" || d.Symbol == "ETHUSDT" {
		maxLeverage = sizing.BTCETHLeverage
	}

	if d.Leverage <= 0 {
		return fmt.Errorf("leverage must be greater than 0: %d", d.Leverage)
	}
	if d.Leverage > maxLeverage {
		logger.Infof("⚠️  [Leverage Fallback] %s leverage exceeded (%dx > %dx), auto-adjusting to limit %dx",
			d.Symbol, d.Leverage, maxLeverage, maxLeverage)
		d.Leverage = maxLeverage
	}

	notional, err := openNotional(sizing.Principal, sizing.PositionPercent)
	if err != nil {
		return err
	}

	if !finitePositive(d.StopLoss) || !finitePositive(d.TakeProfit) {
		return fmt.Errorf("stop loss and take profit must be greater than 0")
	}
	if d.Action == "open_long" {
		if d.StopLoss >= d.TakeProfit {
			return fmt.Errorf("for long positions, stop loss price must be less than take profit price")
		}
	} else {
		if d.StopLoss <= d.TakeProfit {
			return fmt.Errorf("for short positions, stop loss price must be greater than take profit price")
		}
	}

	// The attached protective orders are conditional orders on the exchange: a long
	// stop must sit below the market and its take profit above it (short reversed).
	// A wrong-side protection would be rejected by the exchange, leaving the
	// position unprotected — refuse it before anything is sent.
	price, ok := currentPrice(sizing.Prices, d.Symbol)
	if !ok {
		return fmt.Errorf("%s has no valid current price to validate stop loss and take profit", d.Symbol)
	}
	if d.Action == "open_long" {
		if d.StopLoss >= price {
			return fmt.Errorf("%s long stop loss %v must be below current price %v", d.Symbol, d.StopLoss, price)
		}
		if d.TakeProfit <= price {
			return fmt.Errorf("%s long take profit %v must be above current price %v", d.Symbol, d.TakeProfit, price)
		}
	} else {
		if d.StopLoss <= price {
			return fmt.Errorf("%s short stop loss %v must be above current price %v", d.Symbol, d.StopLoss, price)
		}
		if d.TakeProfit >= price {
			return fmt.Errorf("%s short take profit %v must be below current price %v", d.Symbol, d.TakeProfit, price)
		}
	}

	if err := ValidateStopLossLoss(notional, price, d.StopLoss, sizing.Principal); err != nil {
		return fmt.Errorf("%s refused to open: %w", d.Symbol, err)
	}

	d.NotionalUSD = notional
	return nil
}

// openNotional 开仓名义价值 = 本金 × position_percent（D22/D24）。
// 两个入参先按浮点的最短十进制表示转成精确有理数再相乘，避免 `principal ×
// percent / 100` 的二进制噪声（F33：10000 × 1.13 不能变成 112.99999999999999，
// 否则按张数取整时会少一格）。结果转回 float64 后由适配器按最短十进制表示解析。
func openNotional(principal, percent float64) (float64, error) {
	if !finitePositive(principal) {
		return 0, fmt.Errorf("account principal must be a positive number to size an open position, got %v", principal)
	}
	if !finitePositive(percent) {
		return 0, fmt.Errorf("position percent must be a positive number, got %v", percent)
	}
	if percent < store.MinPositionPercent || percent > store.MaxPositionPercent {
		return 0, fmt.Errorf("position percent %v is outside the allowed %v%%-%v%% range",
			percent, store.MinPositionPercent, store.MaxPositionPercent)
	}

	principalValue, err := decimalValue("account principal", principal)
	if err != nil {
		return 0, err
	}
	percentValue, err := decimalValue("position percent", percent)
	if err != nil {
		return 0, err
	}

	notional := new(big.Rat).Mul(principalValue, percentValue)
	notional.Quo(notional, big.NewRat(100, 1))
	value, _ := notional.Float64()
	if !finitePositive(value) {
		return 0, fmt.Errorf("open notional is not a positive number: %v", value)
	}
	return value, nil
}

// decimalValue 把 float64 按其最短十进制表示转成精确有理数（与适配器解析入参的方式一致）
func decimalValue(name string, v float64) (*big.Rat, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil, fmt.Errorf("%s is not a finite number: %v", name, v)
	}
	value, ok := new(big.Rat).SetString(strconv.FormatFloat(v, 'f', -1, 64))
	if !ok {
		return nil, fmt.Errorf("%s %v is not a parseable decimal", name, v)
	}
	return value, nil
}

// LossAtStop 价格到止损价时的亏损（USDT）：名义价值 × |入场价 − 止损价| ÷ 入场价。
// 入场价不是正数时返回 +Inf，交由调用方拒绝。
func LossAtStop(notional, entry, stop float64) float64 {
	if !finitePositive(entry) {
		return math.Inf(1)
	}
	return notional * math.Abs(entry-stop) / entry
}

// ValidateStopLossLoss 校验价格到止损价时的亏损不超过本金的 2%（D34）。
// 开仓时入场价还不可知，用当前价（模型看到的现价）作为最接近的估计。
// 等于上限允许；比较带极小相对容差，只吸收浮点噪声。
func ValidateStopLossLoss(notional, entry, stop, principal float64) error {
	if !finitePositive(notional) {
		return fmt.Errorf("notional must be a positive number, got %v", notional)
	}
	if !finitePositive(entry) {
		return fmt.Errorf("entry price must be a positive number, got %v", entry)
	}
	if !finitePositive(stop) {
		return fmt.Errorf("stop loss price must be a positive number, got %v", stop)
	}
	if !finitePositive(principal) {
		return fmt.Errorf("account principal must be a positive number, got %v", principal)
	}

	loss := LossAtStop(notional, entry, stop)
	limit := principal * MaxLossPercentOfPrincipal / 100
	if loss > limit*(1+1e-12) {
		return fmt.Errorf(
			"loss at stop %.4f USDT exceeds %.0f%% of principal (%.4f USDT): notional %.4f USDT, entry %.6f, stop %.6f",
			loss, MaxLossPercentOfPrincipal, limit, notional, entry, stop)
	}
	return nil
}

func finitePositive(v float64) bool {
	return v > 0 && !math.IsNaN(v) && !math.IsInf(v, 0)
}

// currentPrice 在 Prices 中找到决策标的的现价。模型偶尔把候选写成 "BTC"、
// "BTC-USDT"、"BTC/USDT" 或小写；交易路径的 filterDecisionsToStrategyUniverse
// 会把它们重写成候选里的规范符号后执行。校验用同一口径取价，否则一个变体就会让
// 整批决策（包括平仓）被拒。
func currentPrice(prices map[string]float64, symbol string) (float64, bool) {
	if price, ok := prices[symbol]; ok && finitePositive(price) {
		return price, true
	}
	trimmed := strings.TrimSpace(symbol)
	if price, ok := prices[market.Normalize(trimmed)]; ok && finitePositive(price) {
		return price, true
	}
	// market.Normalize 不处理 "/" 分隔符（"BTC/USDT"）
	if price, ok := prices[market.Normalize(strings.ReplaceAll(trimmed, "/", ""))]; ok && finitePositive(price) {
		return price, true
	}
	base := symbolBaseKey(symbol)
	if base == "" {
		return 0, false
	}
	for key, price := range prices {
		if symbolBaseKey(key) == base && finitePositive(price) {
			return price, true
		}
	}
	return 0, false
}

// symbolBaseKey 与交易路径的 universeBaseKey 同口径：去掉常见计价后缀后的基础币种
func symbolBaseKey(symbol string) string {
	s := strings.ToUpper(strings.TrimSpace(symbol))
	s = strings.ReplaceAll(s, "/", "")
	if s == "" {
		return ""
	}
	for _, suffix := range []string{"-USDC", "-USDT", "USDC", "USDT", "USD"} {
		if strings.HasSuffix(s, suffix) && len(s) > len(suffix) {
			return strings.TrimSuffix(s, suffix)
		}
	}
	return s
}
