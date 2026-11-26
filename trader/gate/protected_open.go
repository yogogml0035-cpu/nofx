package gate

// 随单保护开仓：USDT 名义价值换算张数、市价滑点上限、下单请求随单附带止盈止损、
// 成交后核对保护单存在。保护不变量（计划文档第 5 节）：NOFX 开出的仓位从存在那一刻起
// 就带有止损与止盈；核对缺失由调用方立即平仓并报错，本层只报错不动仓位。
// 触发规则与字段语义见子任务 research/gate-price-order-docs.md。

import (
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"time"

	"nofx/logger"
	"nofx/trader/types"

	"github.com/gate/gateapi-go/v7"
)

// maxSlipRatio 市价滑点比率的上限：文档为滑点比率最大 50%
const maxSlipRatio = "0.5"

// validateSlipRatio 校验调用方给出的滑点比率：0 表示用交易所默认，(0, 0.5] 为自定义
func validateSlipRatio(ratio float64) (string, error) {
	if ratio == 0 {
		return "", nil
	}
	value, err := decimalFromFloat("slip ratio", ratio)
	if err != nil {
		return "", err
	}
	limit, err := parseDecimal("max slip ratio", maxSlipRatio)
	if err != nil {
		return "", err
	}
	if value.Sign() <= 0 || value.Cmp(limit) > 0 {
		return "", fmt.Errorf("slip ratio must be in (0, %s], got %v", maxSlipRatio, ratio)
	}
	return value.FloatString(6), nil
}

// negateDecimal 张数字符串取负（开空），保持原有小数位
func negateDecimal(contracts string) (string, error) {
	value, err := parseDecimal("contracts", contracts)
	if err != nil {
		return "", err
	}
	return formatDecimal(value.Neg(value), decimalScale(contracts))
}

// slipDisplay 滑点比率用于日志的展示；空字符串表示用交易所默认
func slipDisplay(slip string) string {
	if slip == "" {
		return "default"
	}
	return slip
}

// openProtected 多空共用的随单保护开仓：side 为 "LONG"/"SHORT"
func (t *GateTrader) openProtected(req types.ProtectedOpenRequest, side string) (types.ProtectedOpenResult, error) {
	if req.NotionalUSD <= 0 {
		return types.ProtectedOpenResult{}, fmt.Errorf("notional must be positive, got %v", req.NotionalUSD)
	}
	slip, err := validateSlipRatio(req.SlipRatio)
	if err != nil {
		return types.ProtectedOpenResult{}, err
	}

	symbol := t.convertSymbol(req.Symbol)
	spec, err := t.GetContractSpec(symbol)
	if err != nil {
		return types.ProtectedOpenResult{}, err
	}

	// 开仓取价的唯一来源：Gate 最新价（D8）。名义价值换算与成交值都基于它。
	price, err := t.GetMarketPrice(symbol)
	if err != nil {
		return types.ProtectedOpenResult{}, err
	}

	quote, err := spec.SizeForNotional(price, req.NotionalUSD, req.MinNotional)
	if err != nil {
		return types.ProtectedOpenResult{}, err
	}
	if !quote.Orderable() {
		return types.ProtectedOpenResult{}, fmt.Errorf(
			"open skipped for %s: %s (notional %.4f USDT, price %v, size step %s)",
			symbol, quote.Skip, req.NotionalUSD, price, spec.SizeStep)
	}

	// 张数：多正空负；数量换算一律用不带符号的值
	signedSize, err := signedContracts(quote.Contracts, side)
	if err != nil {
		return types.ProtectedOpenResult{}, err
	}

	order := gateapi.FuturesOrder{
		Contract:             symbol,
		Size:                 signedSize,
		Price:                "0", // 市价
		Tif:                  "ioc",
		Text:                 "t-nofx",
		MarketOrderSlipRatio: slip,
	}
	// 保护不变量：随开仓单一并提交止损止盈；两价合法且分居两侧，触发方向由交易所按委托方向推导。
	// 触发价先对齐到合约价格步长——不对齐时交易所接受 tpsl 字段却不创建条件单（F44），
	// 仓位会在没有保护的情况下开出。多单向上对齐（止损收紧、止盈远离），空单向下对齐。
	align := PriceAlignUp
	if side == "SHORT" {
		align = PriceAlignDown
	}
	slText, slValue, err := spec.AlignTriggerPrice(req.StopLoss, align)
	if err != nil {
		return types.ProtectedOpenResult{}, fmt.Errorf("invalid stop loss for %s: %w", symbol, err)
	}
	tpText, tpValue, err := spec.AlignTriggerPrice(req.TakeProfit, align)
	if err != nil {
		return types.ProtectedOpenResult{}, fmt.Errorf("invalid take profit for %s: %w", symbol, err)
	}
	if err := validateProtectionSides(side, price, slValue, tpValue); err != nil {
		return types.ProtectedOpenResult{}, fmt.Errorf("invalid protection for %s: %w", symbol, err)
	}
	// 校验放在任何有副作用的调用之前：无效保护直接拒绝开仓。
	if err := ApplyAttachedTPSL(&order, side, slText, tpText); err != nil {
		return types.ProtectedOpenResult{}, fmt.Errorf("invalid protection for %s: %w", symbol, err)
	}

	// 提交前按最坏成交价复核亏损上限（D34）：决策校验用的是取数时刻的 K 线收盘价，模型调用
	// 期间价格会漂移，IOC 市价单还可能成交在滑点上限上。超过上限时等比收缩名义价值。
	if req.Principal > 0 && req.MaxLossPercent > 0 {
		resized, err := t.capNotionalForWorstCaseLoss(spec, req, side, price, slip, slValue, quote)
		if err != nil {
			return types.ProtectedOpenResult{}, fmt.Errorf("refusing to open %s: %w", symbol, err)
		}
		if resized.Contracts != quote.Contracts {
			// 名义价值被收缩：下单张数必须跟着重算（多正空负）
			quote = resized
			order.Size, err = signedContracts(quote.Contracts, side)
			if err != nil {
				return types.ProtectedOpenResult{}, err
			}
		}
	}

	// 保证金模式与杠杆按 trader 的设置一并生效并读回，失败不开仓（D38）。
	// 不显式设置时首次建仓的合约会落到全仓最大杠杆（W2/W3 实测），设置没有生效就不能开仓。
	if err := t.ApplyMarginSettings(symbol, req.IsCrossMargin, req.Leverage); err != nil {
		return types.ProtectedOpenResult{}, fmt.Errorf("refusing to open %s: %w", symbol, err)
	}

	logger.Infof("  [Gate] OpenProtected: side=%s, contract=%s, size=%s, sl=%s, tp=%s, slip=%s",
		side, symbol, order.Size, slText, tpText, slipDisplay(slip))

	result, _, err := t.client.FuturesApi.CreateFuturesOrder(t.ctx, "usdt", order, nil)
	if err != nil {
		return types.ProtectedOpenResult{}, fmt.Errorf("failed to open %s position with protection: %w", side, err)
	}
	t.clearCache()

	// action_mode 默认 FULL：返回值即终态（计划文档 F25），成交价必填可用
	fillPrice, err := strconv.ParseFloat(result.FillPrice, 64)
	if err != nil {
		return types.ProtectedOpenResult{}, fmt.Errorf(
			"order %d finished but fill price %q is not parseable: %w", result.Id, result.FillPrice, err)
	}
	if fillPrice <= 0 {
		return types.ProtectedOpenResult{}, fmt.Errorf(
			"order %d finished but fill price %q is not positive", result.Id, result.FillPrice)
	}

	// 实际成交张数 = 下单张数 − 未成交的 left：IOC 部分成交时按成交部分记账（计划张数会高估仓位）
	filledContracts, err := filledContractsOf(result, quote.Contracts)
	if err != nil {
		return types.ProtectedOpenResult{}, err
	}
	if filledContracts != quote.Contracts {
		logger.Warnf("  [Gate] Order %d partially filled: contracts=%s of %s (finish_as=%q, left=%s)",
			result.Id, filledContracts, quote.Contracts, result.FinishAs, result.Left)
	}

	// 实际名义价值 = 张数 × 每张币数量 × 成交价（与换算时的参考价可能不同）
	quantity, err := spec.QuantityFromContracts(filledContracts)
	if err != nil {
		return types.ProtectedOpenResult{}, err
	}
	unitValue, err := spec.unitValue(fillPrice)
	if err != nil {
		return types.ProtectedOpenResult{}, err
	}
	contractsValue, err := parseDecimal("contracts", filledContracts)
	if err != nil {
		return types.ProtectedOpenResult{}, err
	}
	notional := new(big.Rat).Mul(unitValue, contractsValue)

	res := types.ProtectedOpenResult{
		OrderID:    strconv.FormatInt(result.Id, 10),
		Contracts:  filledContracts,
		Notional:   ratFloat(notional),
		FillPrice:  fillPrice,
		Quantity:   quantity,
		StopLoss:   ratFloat(slValue),
		TakeProfit: ratFloat(tpValue),
	}
	logger.Infof("  [Gate] Opened %s position with attached TP/SL: orderId=%s, contracts=%s, fillPrice=%.6f, sl=%s, tp=%s",
		side, res.OrderID, res.Contracts, res.FillPrice, slText, tpText)
	return res, nil
}

// OpenLongProtected 实现随单保护开多
func (t *GateTrader) OpenLongProtected(req types.ProtectedOpenRequest) (types.ProtectedOpenResult, error) {
	return t.openProtected(req, "LONG")
}

// OpenShortProtected 实现随单保护开空
func (t *GateTrader) OpenShortProtected(req types.ProtectedOpenRequest) (types.ProtectedOpenResult, error) {
	return t.openProtected(req, "SHORT")
}

// VerifyOrderProtection 核对开仓委托的保护单存在（保护不变量，计划文档第 5 节）。
// 结论以该合约未触发条件单列表为准：同方向（由开仓委托张数符号推导，多正空负）的止损与止盈
// 各至少 1 条才算通过。开仓委托的 tpsl_* 字段只记日志——真实探针（2026-11-25，F44）确认
// 交易所接受 tpsl 字段但触发价未对齐价格步长时不会创建条件单，只读委托字段会误判为受保护。
// 列表查询最多重试 3 次（间隔 300ms）吸收条件单创建的延迟；该方向持仓已不存在（已被保护单
// 触发平掉）时视为无需保护。遗留的反向止损止盈对不能让新仓位被误判为受保护。
func (t *GateTrader) VerifyOrderProtection(symbol, orderID string) error {
	if orderID == "" {
		return errors.New("cannot verify protection without order id")
	}
	symbol = t.convertSymbol(symbol)

	// 委托方向：多正空负；查询失败时方向未知（空串），列表按同侧配对核对
	side := ""
	order, _, err := t.client.FuturesApi.GetFuturesOrder(t.ctx, "usdt", orderID)
	if err == nil {
		if order.TpslSlTriggerPrice != "" && order.TpslTpTriggerPrice != "" {
			logger.Infof("  [Gate] Order %s carries tpsl fields sl=%s tp=%s (corroboration only)",
				orderID, order.TpslSlTriggerPrice, order.TpslTpTriggerPrice)
		}
		if size, parseErr := parseDecimal("order size", order.Size); parseErr == nil && size.Sign() != 0 {
			side = "SHORT"
			if size.Sign() > 0 {
				side = "LONG"
			}
		}
	} else {
		// 已完结的委托在完结 10 分钟后不能按 ID 查询（计划文档 F25）；
		// 开仓后立即核对不受影响，查询失败不阻断条件单列表核对
		logger.Infof("  [Gate] Order %s lookup failed (%v), falling back to trigger order list", orderID, err)
	}

	// 成交后持仓读数可能有延迟：连续 3 次都读不到持仓才认定「仓位已不存在」（如已被保护单
	// 平掉）。读不到就通过会让核对空过，仓位在没有保护的情况下留着。
	positionSide, positionExists := "", false
	for attempt := 1; attempt <= 3; attempt++ {
		positionSide, positionExists = t.livePositionSide(symbol)
		if positionExists {
			break
		}
		if attempt < 3 {
			time.Sleep(300 * time.Millisecond)
		}
	}
	if !positionExists {
		logger.Infof("  [Gate] %s position is already closed, nothing to protect (order %s)", symbol, orderID)
		return nil
	}
	if side == "" {
		// 委托查询失败（如完结超过 10 分钟）时用交易所持仓方向判定，避免反向遗留保护对误判
		side = positionSide
	}

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		views, err := t.ListTriggerOrders(symbol)
		if err != nil {
			lastErr = fmt.Errorf("protection verification for %s failed: %w", symbol, err)
		} else {
			slSides := map[string]bool{}
			tpSides := map[string]bool{}
			for _, view := range views {
				if view.Contract != symbol || !view.KindKnown || view.Side == "" {
					continue
				}
				if side != "" && view.Side != side {
					continue
				}
				switch view.Kind {
				case ProtectiveStopLoss:
					slSides[view.Side] = true
				case ProtectiveTakeProfit:
					tpSides[view.Side] = true
				}
			}
			hasSL, hasTP := false, false
			for viewSide := range slSides {
				if tpSides[viewSide] {
					hasSL, hasTP = true, true
				}
			}
			if hasSL && hasTP {
				logger.Infof("  [Gate] Protection verified via trigger orders: %s side=%s has %d open trigger orders (SL+TP)",
					symbol, side, len(views))
				return nil
			}
			lastErr = fmt.Errorf(
				"protection missing for %s (order %s, side %q): stop loss present=%v, take profit present=%v, open trigger orders=%d; caller must close immediately",
				symbol, orderID, side, hasSL, hasTP, len(views))
		}
		if attempt < 3 {
			time.Sleep(300 * time.Millisecond)
		}
	}
	return lastErr
}

// livePositionSide 返回该合约当前持仓的方向（"LONG"/"SHORT"）与是否存在持仓。
// 读取失败按“存在但方向未知”（"", true）处理，让调用方走列表核对而不是跳过保护检查。
func (t *GateTrader) livePositionSide(symbol string) (string, bool) {
	positions, _, err := t.client.FuturesApi.ListPositions(t.ctx, "usdt", nil)
	if err != nil {
		return "", true
	}
	for _, pos := range positions {
		if pos.Contract != symbol {
			continue
		}
		size, err := parseDecimal("position size", pos.Size)
		if err != nil || size.Sign() == 0 {
			continue
		}
		if size.Sign() < 0 {
			return "SHORT", true
		}
		return "LONG", true
	}
	return "", false
}

// filledContractsOf 订单实际成交的张数（不带符号）= 下单张数 − 未成交的 left。零成交返回错误。
func filledContractsOf(order gateapi.FuturesOrder, contracts string) (string, error) {
	size, err := parseDecimal("contracts", contracts)
	if err != nil {
		return "", err
	}
	size.Abs(size)
	left := new(big.Rat)
	if order.Left != "" {
		if left, err = parseDecimal("order left", order.Left); err != nil {
			return "", err
		}
	}
	filled := new(big.Rat).Sub(size, left)
	if filled.Sign() <= 0 {
		return "", fmt.Errorf("order %d filled no contracts (finish_as=%q, left=%s)", order.Id, order.FinishAs, order.Left)
	}
	text, err := formatDecimal(filled, decimalScale(contracts))
	if err != nil {
		return "", err
	}
	return text, nil
}

// worstCaseEntryPrice 最坏成交价：最新价 × (1 + 滑点比率)（多单）或最新价 × (1 − 滑点比率)（空单）。
// 滑点比率留空表示用交易所默认（数值未知），按最新价复核。
func worstCaseEntryPrice(price float64, side, slip string) (*big.Rat, error) {
	entry, err := decimalFromFloat("market price", price)
	if err != nil {
		return nil, err
	}
	if slip == "" {
		return entry, nil
	}
	ratio, err := parseDecimal("slip ratio", slip)
	if err != nil {
		return nil, err
	}
	one := big.NewRat(1, 1)
	if side == "SHORT" {
		return new(big.Rat).Mul(entry, new(big.Rat).Sub(one, ratio)), nil
	}
	return new(big.Rat).Mul(entry, new(big.Rat).Add(one, ratio)), nil
}

// worstCaseLossAtStop 按给定入场价计算到止损价的亏损（USDT）：
// 亏损 = |张数| × quanto_multiplier × |入场价 − 止损价|；止损在盈利一侧时为 0。
func worstCaseLossAtStop(spec ContractSpec, side, contracts string, entry, stop *big.Rat) (*big.Rat, error) {
	size, err := parseDecimal("contracts", contracts)
	if err != nil {
		return nil, err
	}
	size.Abs(size)
	quantity := new(big.Rat).Mul(size, spec.multiplier)
	var distance *big.Rat
	if side == "LONG" {
		if stop.Cmp(entry) >= 0 {
			return new(big.Rat), nil
		}
		distance = new(big.Rat).Sub(entry, stop)
	} else {
		if stop.Cmp(entry) <= 0 {
			return new(big.Rat), nil
		}
		distance = new(big.Rat).Sub(stop, entry)
	}
	return new(big.Rat).Mul(quantity, distance), nil
}

// capNotionalForWorstCaseLoss 让「到止损价的亏损 ≤ 本金 × MaxLossPercent%」在最坏成交价下仍成立：
// 超过时按 上限 ÷ 亏损 等比收缩名义价值并重新换算张数（向下取整只会让亏损更小）。
// 收缩后不足最小下单金额时返回错误——止损太远，这个仓位开不了。
func (t *GateTrader) capNotionalForWorstCaseLoss(spec ContractSpec, req types.ProtectedOpenRequest, side string,
	price float64, slip string, stop *big.Rat, quote SizeQuote) (SizeQuote, error) {
	entry, err := worstCaseEntryPrice(price, side, slip)
	if err != nil {
		return quote, err
	}
	loss, err := worstCaseLossAtStop(spec, side, quote.Contracts, entry, stop)
	if err != nil {
		return quote, err
	}
	principal, err := decimalFromFloat("account principal", req.Principal)
	if err != nil {
		return quote, err
	}
	percent, err := decimalFromFloat("max loss percent", req.MaxLossPercent)
	if err != nil {
		return quote, err
	}
	limit := new(big.Rat).Mul(principal, new(big.Rat).Quo(percent, big.NewRat(100, 1)))
	if loss.Cmp(limit) <= 0 {
		return quote, nil
	}

	scale := new(big.Rat).Quo(limit, loss)
	adjusted, err := decimalFromFloat("adjusted notional", req.NotionalUSD*ratFloat(scale))
	if err != nil {
		return quote, err
	}
	adjustedNotional := ratFloat(adjusted)
	adjustedQuote, err := spec.SizeForNotional(price, adjustedNotional, req.MinNotional)
	if err != nil {
		return quote, err
	}
	if !adjustedQuote.Orderable() {
		return quote, fmt.Errorf(
			"stop loss %v is too far for the %.2f%% principal loss cap: notional would shrink from %.4f to %.4f USDT (%s)",
			ratFloat(stop), req.MaxLossPercent, req.NotionalUSD, adjustedNotional, adjustedQuote.Skip)
	}
	// 张数向下取整只会让亏损更小；仍超过上限说明换算有误，宁可拒绝
	recheck, err := worstCaseLossAtStop(spec, side, adjustedQuote.Contracts, entry, stop)
	if err != nil {
		return quote, err
	}
	if recheck.Cmp(limit) > 0 {
		return quote, fmt.Errorf("loss at stop %.6f USDT still exceeds the %.6f USDT cap after resizing", ratFloat(recheck), ratFloat(limit))
	}
	logger.Infof("  [Gate] Notional reduced %.4f -> %.4f USDT: worst-case loss at stop %.6f exceeded the %.2f%% cap %.6f USDT (price %v, slip %s)",
		req.NotionalUSD, adjustedNotional, ratFloat(loss), req.MaxLossPercent, ratFloat(limit), price, slipDisplay(slip))
	return adjustedQuote, nil
}

// signedContracts 张数的带符号写法：多头为正、空头为负
func signedContracts(contracts, side string) (string, error) {
	if side == "SHORT" {
		return negateDecimal(contracts)
	}
	return contracts, nil
}
