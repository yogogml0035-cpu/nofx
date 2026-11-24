package gate

// 仓位的保证金模式与杠杆：读回与一次性生效（计划文档 D38）。Gate 上两者是一次设置
// （POST /futures/{settle}/positions/{contract}/set_leverage），分开设置时各缺一半信息。
// 不显式设置时，首次建仓的合约在本账户下会落到全仓 + 该合约的最大杠杆（子任务 4 的 W2/W3 实测），
// 所以开仓前必须显式生效并读回，没有读回一致就不继续。
// 字段语义与接口原文见任务 research/gate-leverage-margin-docs.md。

import (
	"errors"
	"fmt"
	"nofx/logger"
	"nofx/trader/types"
	"strconv"

	"github.com/gate/gateapi-go/v7"
)

// Gate 仓位的保证金模式：set_leverage 的 margin_mode 参数与持仓条目 pos_margin_mode 字段的取值
const (
	marginModeCross    = "cross"
	marginModeIsolated = "isolated"
)

// labelPositionNotFound 从未设置过的合约没有持仓条目，查询时交易所返回这个 label（线上为 HTTP 400）
const labelPositionNotFound = "POSITION_NOT_FOUND"

// positionModeSingle 单向持仓模式。NOFX 的下单模型建立在它之上（张数带方向、close=true 全平），
// 设置接口因此不带 dual_side
const positionModeSingle = "single"

// ErrNoPositionSettings 合约还没有仓位设置条目：从未设置过杠杆，也没有开过仓
var ErrNoPositionSettings = errors.New("no position settings for contract")

// MarginSettings 合约当前的仓位设置
type MarginSettings struct {
	Cross    bool // 全仓为 true，逐仓为 false
	Leverage int  // 当前杠杆：lever 与同模式的旧字段，只有一个能读时用它，都能读时必须相等
}

// String 输出给日志看的「模式 杠杆」
func (s MarginSettings) String() string {
	mode := marginModeIsolated
	if s.Cross {
		mode = marginModeCross
	}
	return fmt.Sprintf("%s %dx", mode, s.Leverage)
}

// Ensure GateTrader implements MarginSettingsApplier
var _ types.MarginSettingsApplier = (*GateTrader)(nil)

// positiveIntField 把交易所返回的整数字段按精确十进制解析为正整数；
// 缺失、非整数、非正、超出 int 范围都是错误
func positiveIntField(field, raw string) (int, error) {
	value, err := parseDecimal(field, raw)
	if err != nil {
		return 0, err
	}
	if !value.IsInt() || value.Sign() <= 0 {
		return 0, fmt.Errorf("%s %q is not a positive integer", field, raw)
	}
	number := value.Num()
	if !number.IsInt64() {
		return 0, fmt.Errorf("%s %q is out of range", field, raw)
	}
	parsed := int(number.Int64())
	if int64(parsed) != number.Int64() {
		return 0, fmt.Errorf("%s %q is out of range", field, raw)
	}
	return parsed, nil
}

// marginSettingsFromPosition 解析持仓条目或 set_leverage 返回值里的仓位设置。
// 持仓模式（带 mode 字段时）必须是单向：设置接口不带 dual_side，双向持仓下的设置不属于任何一个方向。
// 模式只接受 cross / isolated。杠杆有两个来源：lever（文档写明它是「仓位当前杠杆，逐仓和全仓都可以
// 用该字段表示，逐步替换 leverage 和 cross_leverage_limit」）与同模式下的旧字段（全仓
// cross_leverage_limit、逐仓 leverage）。只有一个能读时用它；两个都能读时必须相等——对不上就是
// 自相矛盾的读数，不替交易所挑一个。旧字段只看同模式的那个：逐仓时 cross_leverage_limit
// 残留上一次全仓的值（W4 实测），全仓时 leverage 恒为 0。
func marginSettingsFromPosition(position *gateapi.Position) (MarginSettings, error) {
	if position.Mode != "" && position.Mode != positionModeSingle {
		return MarginSettings{}, fmt.Errorf("position mode %q is not %q: NOFX trades in one-way position mode",
			position.Mode, positionModeSingle)
	}
	var cross bool
	switch position.PosMarginMode {
	case marginModeCross:
		cross = true
	case marginModeIsolated:
		cross = false
	default:
		return MarginSettings{}, fmt.Errorf("pos_margin_mode %q is not %q or %q",
			position.PosMarginMode, marginModeCross, marginModeIsolated)
	}

	field, raw := "leverage", position.Leverage
	if cross {
		field, raw = "cross_leverage_limit", position.CrossLeverageLimit
	}
	lever, leverErr := positiveIntField("lever", position.Lever)
	legacy, legacyErr := positiveIntField(field, raw)
	switch {
	case leverErr != nil && legacyErr != nil:
		return MarginSettings{}, fmt.Errorf("position leverage is unreadable: %v; %v", leverErr, legacyErr)
	case leverErr != nil:
		return MarginSettings{Cross: cross, Leverage: legacy}, nil
	case legacyErr != nil:
		return MarginSettings{Cross: cross, Leverage: lever}, nil
	case lever != legacy:
		return MarginSettings{}, fmt.Errorf("position leverage is contradictory: lever %d, %s %d", lever, field, legacy)
	}
	return MarginSettings{Cross: cross, Leverage: lever}, nil
}

// PositionMarginSettings 读回合约的仓位设置（GET /futures/{settle}/positions/{contract}）。
// 合约没有仓位设置条目时返回 ErrNoPositionSettings；条目读不出来时返回错误。
func (t *GateTrader) PositionMarginSettings(symbol string) (MarginSettings, error) {
	symbol = t.convertSymbol(symbol)

	position, _, err := t.client.FuturesApi.GetPosition(t.ctx, "usdt", symbol)
	if err != nil {
		var gateErr gateapi.GateAPIError
		if errors.As(err, &gateErr) && gateErr.Label == labelPositionNotFound {
			return MarginSettings{}, fmt.Errorf("%s %q: %w", labelPositionNotFound, symbol, ErrNoPositionSettings)
		}
		return MarginSettings{}, fmt.Errorf("failed to read position settings for %s: %w", symbol, err)
	}

	settings, err := marginSettingsFromPosition(&position)
	if err != nil {
		return MarginSettings{}, fmt.Errorf("failed to read position settings for %s: %w", symbol, err)
	}
	return settings, nil
}

// ApplyMarginSettings 按 trader 的设置一次性生效保证金模式与杠杆（MarginSettingsApplier）。
// 走文档的新接口 set_leverage（leverage + margin_mode），不带 dual_side：NOFX 的下单模型
// 建立在单向持仓模式上。
//
// 成功的定义是读回的设置正是目标设置：先看设置接口的返回值；接口报错、返回值读不出或与目标
// 不一致时，再读回持仓条目，条目等于目标设置即成功（如请求超时但交易所已生效；W4 实测重复设置
// 同一组值时交易所直接返回 200，不走这条回退）。两处都对不上就返回错误并带上交易所的错误
// （可用 errors.As 取出 GateAPIError），调用方不得继续下单。
func (t *GateTrader) ApplyMarginSettings(symbol string, isCrossMargin bool, leverage int) error {
	if leverage < 1 {
		return fmt.Errorf("leverage must be at least 1, got %d", leverage)
	}
	symbol = t.convertSymbol(symbol)

	mode := marginModeIsolated
	if isCrossMargin {
		mode = marginModeCross
	}
	want := MarginSettings{Cross: isCrossMargin, Leverage: leverage}

	position, _, err := t.client.FuturesApi.UpdateContractPositionLeverage(
		t.ctx, "usdt", symbol, strconv.Itoa(leverage), mode, nil)
	if err == nil {
		applied, parseErr := marginSettingsFromPosition(&position)
		switch {
		case parseErr != nil:
			err = fmt.Errorf("setting response is not readable: %w", parseErr)
		case applied != want:
			err = fmt.Errorf("setting response shows %s %s", symbol, applied)
		default:
			logger.Infof("  [Gate] Margin settings applied: %s %s", symbol, want)
			return nil
		}
	}

	current, readErr := t.PositionMarginSettings(symbol)
	if readErr == nil && current == want {
		logger.Infof("  [Gate] Margin settings already in effect: %s %s", symbol, want)
		return nil
	}
	if readErr != nil {
		return fmt.Errorf("failed to apply margin settings %s %s: %w; reading back failed: %v",
			symbol, want, err, readErr)
	}
	return fmt.Errorf("failed to apply margin settings %s %s: %w; position shows %s %s",
		symbol, want, err, symbol, current)
}

// SetLeverage 保持该合约当前的保证金模式，只改杠杆（types.Trader 接口；带模式信息的调用走
// ApplyMarginSettings）。从未设置过的合约按逐仓：这类调用不带模式信息，逐仓把亏损限制在仓位保证金内。
func (t *GateTrader) SetLeverage(symbol string, leverage int) error {
	current, err := t.PositionMarginSettings(symbol)
	if errors.Is(err, ErrNoPositionSettings) {
		current = MarginSettings{Cross: false}
	} else if err != nil {
		return err
	}
	return t.ApplyMarginSettings(symbol, current.Cross, leverage)
}

// SetMarginMode 保持当前生效的杠杆，只改模式（types.Trader 接口）。
// 合约没有仓位设置条目时没有可保持的杠杆：报错并提示带上杠杆一并设置。
func (t *GateTrader) SetMarginMode(symbol string, isCrossMargin bool) error {
	current, err := t.PositionMarginSettings(symbol)
	if errors.Is(err, ErrNoPositionSettings) {
		return fmt.Errorf("%w: cannot switch margin mode without a leverage to keep; set it together with a leverage", err)
	}
	if err != nil {
		return err
	}
	return t.ApplyMarginSettings(symbol, isCrossMargin, current.Leverage)
}
