package gate

import (
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/gate/gateapi-go/v7"
)

// contractSpecCacheTTL 合约元数据的缓存时长。交易所会调整合约的张数精度，
// 常驻的 trader 不能一直用启动时读到的步长。
const contractSpecCacheTTL = 10 * time.Minute

// 币数量以 float64 传入时允许的浮点噪声（张）：绝对部分与相对部分（按张数）取较大者。
// 浮点噪声约为张数 × 10⁻¹⁶，只给绝对容差的话超大持仓会落不回步长上；两个值都远小于任何最小步长
var (
	snapToleranceAbs = big.NewRat(1, 1_000_000_000)
	snapToleranceRel = big.NewRat(1, 1_000_000_000_000)
)

var hundred = big.NewRat(100, 1)

// ErrCloseBelowStep 按比例换算后的平仓张数不足一个最小步长
var ErrCloseBelowStep = errors.New("close size below one size step")

// ContractSpec 下单数量换算所需的合约元数据。
// 取自 GET /futures/{settle}/contracts/{contract}：读取时带 X-Gate-Size-Decimal: 1，
// order_size_min 才保留小数部分。导出的字符串字段供展示与日志，构造后不要修改；方法可并发调用。
type ContractSpec struct {
	Name             string // 合约名，如 ETH_USDT
	QuantoMultiplier string // 一张合约对应的标的币数量
	// 最小下单张数，取作张数的最小步长。这是设计推论：官方资料没有写明张数必须是它的整数倍，
	// 见任务 design.md 的“文档依据”
	SizeStep           string
	SizeMax            string // 单笔订单的最大张数（order_size_max）
	MarketOrderSizeMax string // 市价单的最大张数（market_order_size_max）；交易所未设置为 "0"，此时按 SizeMax 计

	multiplier *big.Rat
	step       *big.Rat
	stepScale  int      // SizeStep 的小数位数，决定张数的输出精度
	sizeMax    *big.Rat // 市价单可用的最大张数：SizeMax 与 MarketOrderSizeMax 中较小者
}

// NewContractSpec 校验合约元数据并构造 ContractSpec。
// enableDecimal 为 false 的合约只接受整数步长；marketOrderSizeMax 留空表示交易所未设置市价单上限。
func NewContractSpec(name, quantoMultiplier, orderSizeMin, orderSizeMax, marketOrderSizeMax string, enableDecimal bool) (ContractSpec, error) {
	if name == "" {
		return ContractSpec{}, errors.New("contract name is empty")
	}
	multiplier, err := parseDecimal("quanto_multiplier", quantoMultiplier)
	if err != nil {
		return ContractSpec{}, fmt.Errorf("contract %s: %w", name, err)
	}
	if multiplier.Sign() <= 0 {
		return ContractSpec{}, fmt.Errorf("contract %s: quanto_multiplier %q is not positive", name, quantoMultiplier)
	}
	step, err := parseDecimal("order_size_min", orderSizeMin)
	if err != nil {
		return ContractSpec{}, fmt.Errorf("contract %s: %w", name, err)
	}
	if step.Sign() <= 0 {
		return ContractSpec{}, fmt.Errorf(
			"contract %s: order_size_min %q is not positive; send X-Gate-Size-Decimal: 1 to read fractional sizes",
			name, orderSizeMin)
	}
	if !enableDecimal && !step.IsInt() {
		return ContractSpec{}, fmt.Errorf(
			"contract %s: order_size_min %q is fractional but the contract supports whole contracts only",
			name, orderSizeMin)
	}
	sizeMax, err := parseDecimal("order_size_max", orderSizeMax)
	if err != nil {
		return ContractSpec{}, fmt.Errorf("contract %s: %w", name, err)
	}
	if sizeMax.Cmp(step) < 0 {
		return ContractSpec{}, fmt.Errorf(
			"contract %s: order_size_max %q is below order_size_min %q", name, orderSizeMax, orderSizeMin)
	}

	// 市价单另有最大张数；交易所未设置（留空或 0）时按单笔订单的最大张数计
	if marketOrderSizeMax == "" {
		marketOrderSizeMax = "0"
	}
	marketMax, err := parseDecimal("market_order_size_max", marketOrderSizeMax)
	if err != nil {
		return ContractSpec{}, fmt.Errorf("contract %s: %w", name, err)
	}
	if marketMax.Sign() < 0 {
		return ContractSpec{}, fmt.Errorf("contract %s: market_order_size_max %q is negative", name, marketOrderSizeMax)
	}
	if marketMax.Sign() == 0 {
		marketOrderSizeMax = "0" // "0.00" 之类统一成 "0"
	}
	if marketMax.Sign() > 0 && marketMax.Cmp(step) < 0 {
		return ContractSpec{}, fmt.Errorf(
			"contract %s: market_order_size_max %q is below order_size_min %q", name, marketOrderSizeMax, orderSizeMin)
	}
	if marketMax.Sign() > 0 && marketMax.Cmp(sizeMax) < 0 {
		sizeMax = marketMax
	}

	return ContractSpec{
		Name:               name,
		QuantoMultiplier:   quantoMultiplier,
		SizeStep:           orderSizeMin,
		SizeMax:            orderSizeMax,
		MarketOrderSizeMax: marketOrderSizeMax,
		multiplier:         multiplier,
		step:               step,
		stepScale:          decimalScale(orderSizeMin),
		sizeMax:            sizeMax,
	}, nil
}

// contractSpecFromGate 把 SDK 返回的合约信息转换为 ContractSpec
func contractSpecFromGate(contract *gateapi.Contract) (ContractSpec, error) {
	return NewContractSpec(
		contract.Name, contract.QuantoMultiplier, contract.OrderSizeMin, contract.OrderSizeMax,
		contract.MarketOrderSizeMax, contract.EnableDecimal)
}

// check 确认元数据经过构造函数；零值的 ContractSpec 不能参与换算
func (s ContractSpec) check() error {
	if s.multiplier == nil || s.step == nil || s.sizeMax == nil {
		return fmt.Errorf("contract spec for %q is not initialized", s.Name)
	}
	return nil
}

// unitValue 返回一张合约按指定价格折算的名义价值（USDT）
func (s ContractSpec) unitValue(price float64) (*big.Rat, error) {
	priceValue, err := decimalFromFloat("price", price)
	if err != nil {
		return nil, err
	}
	if priceValue.Sign() <= 0 {
		return nil, fmt.Errorf("price must be positive, got %v", price)
	}
	return new(big.Rat).Mul(priceValue, s.multiplier), nil
}

// contracts 把整数个最小步长格式化为张数字符串
func (s ContractSpec) contracts(steps *big.Int) (string, error) {
	return formatDecimal(new(big.Rat).Mul(s.step, new(big.Rat).SetInt(steps)), s.stepScale)
}

// SizeQuote USDT 名义价值换算后的下单数量
type SizeQuote struct {
	Contracts     string     // 向下取整到最小步长后的张数；只在 Orderable 为真时非空
	Notional      float64    // Contracts 按参考价折算的名义价值（USDT）
	MinOrderValue float64    // 当前价格下可下单的最小名义价值（USDT）；最少步数超过市价单最大张数时按它下单会被判为超过最大张数，能否下单以 Orderable 为准
	Skip          SkipReason // 不能下单的原因
}

// Orderable 报告这次换算的结果能否下单
func (q SizeQuote) Orderable() bool {
	return q.Skip == SizeOK
}

// SkipReason 名义价值换算后不能下单的原因
type SkipReason int

const (
	SizeOK                   SkipReason = iota // 可下单
	SizeSkipBelowStep                          // 取整前已不足一个最小步长
	SizeSkipBelowMinNotional                   // 取整后低于最小下单金额
	SizeSkipAboveMax                           // 超过市价单的单笔最大张数
)

// String 给出给用户与模型看的说明，使用中文；包内的错误信息沿用惯例用英文
func (r SkipReason) String() string {
	switch r {
	case SizeOK:
		return "可下单"
	case SizeSkipBelowStep:
		return "不足一个最小步长"
	case SizeSkipBelowMinNotional:
		return "取整后低于最小下单金额"
	case SizeSkipAboveMax:
		return "超过市价单最大张数"
	}
	return fmt.Sprintf("SkipReason(%d)", int(r))
}

// SizeForNotional 把 USDT 名义价值换算为下单张数，按最小步长向下取整。
// minNotional 是最小下单金额（USDT）。取整后不足一个最小步长、低于最小下单金额、
// 或超过市价单的单笔最大张数时不下单，原因见返回值的 Skip。
func (s ContractSpec) SizeForNotional(price, notional, minNotional float64) (SizeQuote, error) {
	if err := s.check(); err != nil {
		return SizeQuote{}, err
	}
	unitValue, err := s.unitValue(price)
	if err != nil {
		return SizeQuote{}, err
	}
	notionalValue, err := decimalFromFloat("notional", notional)
	if err != nil {
		return SizeQuote{}, err
	}
	if notionalValue.Sign() <= 0 {
		return SizeQuote{}, fmt.Errorf("notional must be positive, got %v", notional)
	}
	minNotionalValue, err := decimalFromFloat("min notional", minNotional)
	if err != nil {
		return SizeQuote{}, err
	}
	if minNotionalValue.Sign() < 0 {
		return SizeQuote{}, fmt.Errorf("min notional must not be negative, got %v", minNotional)
	}

	stepValue := new(big.Rat).Mul(unitValue, s.step) // 一个最小步长的名义价值
	minSteps := ceilDiv(minNotionalValue, stepValue)
	if minSteps.Sign() == 0 {
		minSteps = big.NewInt(1)
	}
	quote := SizeQuote{
		MinOrderValue: ratFloat(new(big.Rat).Mul(stepValue, new(big.Rat).SetInt(minSteps))),
	}

	steps := floorDiv(notionalValue, stepValue)
	contractsValue := new(big.Rat).Mul(s.step, new(big.Rat).SetInt(steps))
	switch {
	case steps.Sign() == 0:
		quote.Skip = SizeSkipBelowStep
	case steps.Cmp(minSteps) < 0:
		quote.Skip = SizeSkipBelowMinNotional
	case contractsValue.Cmp(s.sizeMax) > 0:
		quote.Skip = SizeSkipAboveMax
	default:
		contracts, err := s.contracts(steps)
		if err != nil {
			return SizeQuote{}, err
		}
		quote.Contracts = contracts
		quote.Notional = ratFloat(new(big.Rat).Mul(contractsValue, unitValue))
	}
	return quote, nil
}

// CloseQuote 分批平仓比例换算后的平仓张数
type CloseQuote struct {
	Contracts string // 平仓张数，不带符号
	Remaining string // 平仓后的剩余张数；全平为 "0"
	Full      bool   // 平仓量等于整个持仓：这是全平，不是分批平仓
}

// SizeForCloseRatio 把分批平仓比例换算为张数，按最小步长向下取整。
// position 是交易所返回的持仓张数（可带负号），percent 取 (0, 100]。
// 比例小于 100 时平仓量必然小于持仓，剩余至少一个最小步长；
// 比例达到 100% 时没有剩余，返回 Full 而不是分批结果。
func (s ContractSpec) SizeForCloseRatio(position string, percent float64) (CloseQuote, error) {
	if err := s.check(); err != nil {
		return CloseQuote{}, err
	}
	positionSize, err := parseDecimal("position size", position)
	if err != nil {
		return CloseQuote{}, err
	}
	positionSize.Abs(positionSize)
	positionSteps := new(big.Rat).Quo(positionSize, s.step)
	if !positionSteps.IsInt() || positionSteps.Sign() <= 0 {
		return CloseQuote{}, fmt.Errorf(
			"position size %s is not a positive multiple of size step %s", position, s.SizeStep)
	}
	ratio, err := decimalFromFloat("close percent", percent)
	if err != nil {
		return CloseQuote{}, err
	}
	if ratio.Sign() <= 0 || ratio.Cmp(hundred) > 0 {
		return CloseQuote{}, fmt.Errorf("close percent must be in (0, 100], got %v", percent)
	}

	total := positionSteps.Num()
	closeSteps := floorDiv(new(big.Rat).Mul(positionSteps, ratio), hundred)
	if closeSteps.Cmp(total) == 0 {
		contracts, err := s.contracts(total)
		if err != nil {
			return CloseQuote{}, err
		}
		return CloseQuote{Contracts: contracts, Remaining: "0", Full: true}, nil
	}
	if closeSteps.Sign() == 0 {
		return CloseQuote{}, fmt.Errorf(
			"%w: position %s contracts, close %v%%", ErrCloseBelowStep, position, percent)
	}

	contracts, err := s.contracts(closeSteps)
	if err != nil {
		return CloseQuote{}, err
	}
	remaining, err := s.contracts(new(big.Int).Sub(total, closeSteps))
	if err != nil {
		return CloseQuote{}, err
	}
	return CloseQuote{Contracts: contracts, Remaining: remaining}, nil
}

// validateMarketOrderSize 校验市价单张数不超过本合约的市价单最大张数：
// order_size_max 与 market_order_size_max 中较小者（后者为 0 表示交易所未设置）。
func (s ContractSpec) validateMarketOrderSize(contracts string) error {
	if err := s.check(); err != nil {
		return err
	}
	value, err := parseDecimal("contracts", contracts)
	if err != nil {
		return err
	}
	if value.Sign() <= 0 {
		return fmt.Errorf("contracts must be positive, got %s", contracts)
	}
	if value.Cmp(s.sizeMax) > 0 {
		return fmt.Errorf(
			"contracts %s exceed the market order maximum (order_size_max %s, market_order_size_max %s)",
			contracts, s.SizeMax, s.MarketOrderSizeMax)
	}
	return nil
}

// ContractsFromQuantity 把标的币数量换算为合约张数。
// 结果必须落在最小步长上，否则报错：不截断，也不把不足 1 张改成 1 张。
func (s ContractSpec) ContractsFromQuantity(quantity float64) (string, error) {
	if err := s.check(); err != nil {
		return "", err
	}
	quantityValue, err := decimalFromFloat("quantity", quantity)
	if err != nil {
		return "", err
	}
	if quantityValue.Sign() <= 0 {
		return "", fmt.Errorf("quantity must be positive, got %v", quantity)
	}

	// 数量常由“张数 × 每张币数量”的浮点乘法得来，带着乘法的误差；
	// 吸收得掉这种误差的才算是落在步长上
	contracts := new(big.Rat).Quo(quantityValue, s.multiplier)
	steps := new(big.Rat).Quo(contracts, s.step)
	numerator, denominator := steps.Num(), steps.Denom()
	remainder := new(big.Int).Mod(numerator, denominator)
	nearest := new(big.Int).Quo(numerator, denominator)
	if new(big.Int).Lsh(remainder, 1).Cmp(denominator) >= 0 {
		nearest.Add(nearest, big.NewInt(1))
	}
	if nearest.Sign() == 0 {
		return "", fmt.Errorf("quantity %v is below one size step (%s contracts)", quantity, s.SizeStep)
	}
	snapped := new(big.Rat).Mul(s.step, new(big.Rat).SetInt(nearest))
	tolerance := new(big.Rat).Mul(contracts, snapToleranceRel)
	if tolerance.Cmp(snapToleranceAbs) < 0 {
		tolerance = snapToleranceAbs
	}
	if new(big.Rat).Abs(new(big.Rat).Sub(contracts, snapped)).Cmp(tolerance) > 0 {
		return "", fmt.Errorf(
			"quantity %v is not a multiple of size step %s (%s contracts)",
			quantity, s.SizeStep, contracts.FloatString(12))
	}
	return formatDecimal(snapped, s.stepScale)
}

// ContractsFromQuantityFloor 把标的币数量换算为合约张数，按最小步长向下取整。
// 与 ContractsFromQuantity 不同：数量不落在步长上时向下取整而不是报错
// （部分平仓量由调用方给出，取整只向下，不放大订单）；
// 取整后不足一个最小步长时报错，不强制成一步。
// 数量带着"张数 × 每张币数量"浮点乘法的噪声时，容差内吸附回步长再取值，
// 向下噪声不会因此少算一步。
func (s ContractSpec) ContractsFromQuantityFloor(quantity float64) (string, error) {
	if err := s.check(); err != nil {
		return "", err
	}
	quantityValue, err := decimalFromFloat("quantity", quantity)
	if err != nil {
		return "", err
	}
	if quantityValue.Sign() <= 0 {
		return "", fmt.Errorf("quantity must be positive, got %v", quantity)
	}

	contracts := new(big.Rat).Quo(quantityValue, s.multiplier)
	stepsExact := new(big.Rat).Quo(contracts, s.step)
	nearest := new(big.Int).Quo(stepsExact.Num(), stepsExact.Denom())
	if new(big.Int).Lsh(new(big.Int).Mod(stepsExact.Num(), stepsExact.Denom()), 1).Cmp(stepsExact.Denom()) >= 0 {
		nearest.Add(nearest, big.NewInt(1))
	}
	if nearest.Sign() > 0 {
		snapped := new(big.Rat).Mul(s.step, new(big.Rat).SetInt(nearest))
		tolerance := new(big.Rat).Mul(contracts, snapToleranceRel)
		if tolerance.Cmp(snapToleranceAbs) < 0 {
			tolerance = snapToleranceAbs
		}
		if new(big.Rat).Abs(new(big.Rat).Sub(contracts, snapped)).Cmp(tolerance) <= 0 {
			return formatDecimal(snapped, s.stepScale)
		}
	}

	steps := floorDiv(contracts, s.step)
	if steps.Sign() == 0 {
		return "", fmt.Errorf("quantity %v is below one size step (%s contracts)", quantity, s.SizeStep)
	}
	return s.contracts(steps)
}

// QuantityFromContracts 把合约张数换算为标的币数量，用于展示与记账
func (s ContractSpec) QuantityFromContracts(contracts string) (float64, error) {
	if err := s.check(); err != nil {
		return 0, err
	}
	size, err := parseDecimal("contracts", contracts)
	if err != nil {
		return 0, err
	}
	if !new(big.Rat).Quo(size, s.step).IsInt() {
		return 0, fmt.Errorf("contracts %s is not a multiple of size step %s", contracts, s.SizeStep)
	}
	return ratFloat(new(big.Rat).Mul(size, s.multiplier)), nil
}

// contractQuantity 把已解析的张数换算为标的币数量（精确十进制，float64 仅用于展示与记账）。
// 张数可以带符号（持仓、委托的方向）。
func (s ContractSpec) contractQuantity(contracts *big.Rat) float64 {
	return ratFloat(new(big.Rat).Mul(contracts, s.multiplier))
}

// contractSpecEntry 缓存中的一条合约元数据
type contractSpecEntry struct {
	spec ContractSpec
	at   time.Time
}

// contractSpecCache 带有效期的合约元数据缓存；零值可用
type contractSpecCache struct {
	mu      sync.Mutex
	entries map[string]contractSpecEntry
}

// GetContractSpec 返回下单换算所需的合约元数据，有效期内复用缓存，读取失败不缓存
func (t *GateTrader) GetContractSpec(symbol string) (ContractSpec, error) {
	symbol = t.convertSymbol(symbol)

	t.contractSpecs.mu.Lock()
	if entry, ok := t.contractSpecs.entries[symbol]; ok && time.Since(entry.at) < contractSpecCacheTTL {
		t.contractSpecs.mu.Unlock()
		return entry.spec, nil
	}
	t.contractSpecs.mu.Unlock()

	contract, _, err := t.client.FuturesApi.GetFuturesContract(t.ctx, "usdt", symbol)
	if err != nil {
		return ContractSpec{}, fmt.Errorf("failed to get contract info: %w", err)
	}
	spec, err := contractSpecFromGate(&contract)
	if err != nil {
		// 构造函数的报错里已带合约名
		return ContractSpec{}, fmt.Errorf("failed to read contract info: %w", err)
	}

	t.contractSpecs.mu.Lock()
	if t.contractSpecs.entries == nil {
		t.contractSpecs.entries = make(map[string]contractSpecEntry)
	}
	t.contractSpecs.entries[symbol] = contractSpecEntry{spec: spec, at: time.Now()}
	t.contractSpecs.mu.Unlock()
	return spec, nil
}
