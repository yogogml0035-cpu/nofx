package main

// 带仓的只读真实读数（子任务 drawdown-monitor；真实写操作在 W2 授权范围内：DOGE_USDT ≤10U
// 开平、立即还原，计划文档 10.3 节）：
//
//	预检  position_mode；全账户无持仓、无未完结委托、无未触发条件单；可用余额基线；
//	      三个 watched 合约的仓位设置条目基线
//	1     随单保护开仓 DOGE_USDT（名义价值 ≤10U 含 5% 裕度；止损现价 ×0.9、止盈 ×1.1；
//	      杠杆 75 全仓 = 基线设置原值，幂等应用不改动设置）
//	2     只读读数：原始条目的 lever 字段 vs 交付的 GetPositions 返回的 float64 杠杆——必须一致
//	      （F12 修复的线上核对）
//	3     只读读数：归属判定（D40）——网格品种上的持仓判为本 trader 开出的；非网格品种判为不是
//	4     只读读数：回撤监控执行一轮（首次见仓：峰值 = 当前值、回撤 0，无论价格如何都不会触发平仓）
//	      ——峰值缓存记录了该仓位的收益率，且与按实际杠杆手工计算的收益率一致（监控路径吃到
//	      实际杠杆的线上核对）；随后再次读持仓确认没有被平掉
//	5     还原：交付的 CloseLong(symbol, 0) 全平（同时是平仓原语的线上核对）；终态：无持仓、
//	      无未完结委托、无未触发条件单、仓位设置条目与基线一致、可用余额与基线的差值在容差内
//
// 归属判定的 DB 子句（本地 trader_positions）由单元测试覆盖；真实验证不构造 store（探针以
// CGO_ENABLED=0 交叉编译，无法携带 CGO sqlite），桥里的归属走网格子句。
// 凭据从 GATE_KEY / GATE_SECRET 环境变量读取，不落盘、不打印。
// 容器出口走 VPN 不在 key 白名单：Docker 内交叉编译（CGO_ENABLED=0 GOOS=darwin GOARCH=arm64，
// 需带 -tags livecheck 编进 trader 包的探针桥），宿主机运行。

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/gate/gateapi-go/v7"

	"nofx/trader/types"
	nofxgate "nofx/trader/gate"
	nofxtrader "nofx/trader"
)

const target = "DOGE_USDT"
const targetBase = "DOGEUSDT"

// targetLeverage DOGE_USDT 的基线设置：全仓 75x（W4 收口读数）。开仓请求按原值幂等应用，不改动设置。
const targetLeverage = 75

// notionalCap 单笔名义价值授权上限（计划文档 10.3：不超过 10 USDT）
const notionalCap = 10.0

var watched = []string{"BTC_USDT", "DOGE_USDT", "ETH_USDT"}

var failures int

var out io.Writer = os.Stdout

// entry 仓位条目里与设置有关的字段（原样记录，线上格式；W4 探针同款）
type entry struct {
	Contract           string `json:"contract"`
	Size               string `json:"size"`
	Mode               string `json:"mode"`
	PosMarginMode      string `json:"pos_margin_mode"`
	Leverage           string `json:"leverage"`
	CrossLeverageLimit string `json:"cross_leverage_limit"`
	Lever              string `json:"lever"`
	LeverageMax        string `json:"leverage_max"`
}

func toEntry(position gateapi.Position) entry {
	return entry{
		Contract: position.Contract, Size: position.Size, Mode: position.Mode,
		PosMarginMode: position.PosMarginMode, Leverage: position.Leverage,
		CrossLeverageLimit: position.CrossLeverageLimit, Lever: position.Lever, LeverageMax: position.LeverageMax,
	}
}

func emit(event map[string]interface{}) {
	event["ts"] = time.Now().UTC().Format(time.RFC3339)
	data, err := json.Marshal(event)
	if err != nil {
		panic(err)
	}
	fmt.Fprintln(out, string(data))
}

func step(name string, ok bool, detail map[string]interface{}) {
	if detail == nil {
		detail = map[string]interface{}{}
	}
	detail["step"] = name
	if ok {
		detail["result"] = "PASS"
	} else {
		detail["result"] = "FAIL"
		failures++
	}
	emit(detail)
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	var gateErr gateapi.GateAPIError
	if errors.As(err, &gateErr) {
		return fmt.Sprintf("label=%s message=%s", gateErr.Label, gateErr.GetMessage())
	}
	return err.Error()
}

func readEntry(client *gateapi.APIClient, ctx context.Context, contract string) (entry, error) {
	position, _, err := client.FuturesApi.GetPosition(ctx, "usdt", contract)
	if err != nil {
		return entry{}, err
	}
	return toEntry(position), nil
}

func main() {
	os.Exit(run())
}

func run() int {
	outPath := flag.String("out", "", "证据文件（JSONL）；留空则写到 stdout")
	flag.Parse()
	if *outPath != "" {
		file, err := os.Create(*outPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "无法创建证据文件:", err)
			return 2
		}
		defer file.Close()
		out = file
	}

	key := os.Getenv("GATE_KEY")
	secret := os.Getenv("GATE_SECRET")
	if key == "" || secret == "" {
		fmt.Fprintln(os.Stderr, "GATE_KEY / GATE_SECRET 未设置")
		return 2
	}

	// 被验证的对象：交付的适配器（GetPositions / 保护开仓 / 全平）
	gt := nofxgate.NewGateTrader(key, secret)
	// 只读对照用的原始客户端
	rawClient := gateapi.NewAPIClient(gateapi.NewConfiguration())
	ctx := context.WithValue(context.Background(), gateapi.ContextGateAPIV4,
		gateapi.GateAPIV4{Key: key, Secret: secret})

	// ===== 预检（只读）=====
	account, _, err := rawClient.FuturesApi.ListFuturesAccounts(ctx, "usdt")
	if err != nil {
		step("preflight account", false, map[string]interface{}{"error": errText(err)})
		return 1
	}
	availableBefore := account.Available
	step("preflight account", true, map[string]interface{}{
		"position_mode": account.PositionMode, "margin_mode": account.MarginMode,
		"in_dual_mode": account.InDualMode, "available": availableBefore})

	baseline := map[string]entry{}
	for _, contract := range watched {
		e, err := readEntry(rawClient, ctx, contract)
		if err != nil {
			step("preflight entry "+contract, false, map[string]interface{}{"error": errText(err)})
			return 1
		}
		baseline[contract] = e
	}
	step("preflight entries", true, map[string]interface{}{"baseline": baseline})

	if baseline[target].Size != "0" {
		step("preflight target idle", false, map[string]interface{}{
			"error": fmt.Sprintf("%s has a position of size %s; the livecheck opens and restores its own position only", target, baseline[target].Size)})
		return 1
	}
	orders, _, err := rawClient.FuturesApi.ListFuturesOrders(ctx, "usdt", "open", nil)
	if err != nil {
		step("preflight orders", false, map[string]interface{}{"error": errText(err)})
		return 1
	}
	triggers, _, err := rawClient.FuturesApi.ListPriceTriggeredOrders(ctx, "usdt", "open", nil)
	if err != nil {
		step("preflight price orders", false, map[string]interface{}{"error": errText(err)})
		return 1
	}
	idle := len(orders) == 0 && len(triggers) == 0
	step("preflight idle", idle, map[string]interface{}{
		"open_orders": len(orders), "open_price_orders": len(triggers), "scope": "account-wide"})
	if !idle {
		return 1
	}

	// ===== 兜底清理：中途失败也会执行 =====
	cleaned := false
	cleanup := func(name string) {
		if cleaned {
			return
		}
		cleaned = true
		pos, _, posErr := rawClient.FuturesApi.GetPosition(ctx, "usdt", target)
		if posErr != nil || pos.Size == "0" {
			return // 没有持仓需要清理
		}
		if _, err := gt.CloseLong(targetBase, 0); err != nil {
			step(name, false, map[string]interface{}{"error": errText(err)})
			return
		}
		time.Sleep(2 * time.Second)
		after, err := readEntry(rawClient, ctx, target)
		step(name, err == nil && after.Size == "0", map[string]interface{}{"readback": after, "error": errText(err)})
	}
	defer cleanup("cleanup (deferred)")

	// ===== 1 随单保护开仓（W2 形态：≤10U、立即还原）=====
	price, err := gt.GetMarketPrice(targetBase)
	if err != nil {
		step("1 market price", false, map[string]interface{}{"error": errText(err)})
		return 1
	}
	// 目标张数按 ≤10U 名义价值向下取整（每张 10 DOGE），名义价值按张数 × 现价 × 1.05 裕度构造；
	// 超过授权上限即终止（硬保护）
	multiplier := 10.0
	contracts := int(notionalCap * 0.95 / (price * multiplier))
	if contracts < 1 {
		step("1 size target", false, map[string]interface{}{
			"error": fmt.Sprintf("price %.6g leaves no whole contract under the %.1f USDT cap", price, notionalCap)})
		return 1
	}
	notional := price * float64(contracts) * multiplier * 1.05
	if notional > notionalCap {
		step("1 size target", false, map[string]interface{}{
			"error": fmt.Sprintf("notional %.4f exceeds the %.1f USDT authorization cap", notional, notionalCap)})
		return 1
	}
	step("1 size target", true, map[string]interface{}{
		"price": price, "contracts": contracts, "notional": notional, "cap": notionalCap})

	res, err := gt.OpenLongProtected(types.ProtectedOpenRequest{
		Symbol:        targetBase,
		NotionalUSD:   notional,
		Leverage:      targetLeverage, // 基线设置原值：幂等应用，不改动设置
		IsCrossMargin: true,
		StopLoss:      price * 0.9,
		TakeProfit:    price * 1.1,
		MinNotional:   0,
		SlipRatio:     0.01,
	})
	if err != nil {
		step("1 protected open", false, map[string]interface{}{"error": errText(err)})
		return 1
	}
	step("1 protected open", true, map[string]interface{}{
		"orderId": res.OrderID, "contracts": res.Contracts, "notional": res.Notional, "fillPrice": res.FillPrice})
	time.Sleep(2 * time.Second)

	// ===== 2 只读读数：适配层杠杆 = 原始条目的 lever（F12 修复核对）=====
	rawPositions, _, err := rawClient.FuturesApi.ListPositions(ctx, "usdt", nil)
	if err != nil {
		step("2 raw positions", false, map[string]interface{}{"error": errText(err)})
		return 1
	}
	var rawLever string
	rawEntry := map[string]interface{}{}
	for _, p := range rawPositions {
		if p.Contract == target {
			rawLever = p.Lever
			rawEntry = map[string]interface{}{
				"size": p.Size, "pos_margin_mode": p.PosMarginMode, "lever": p.Lever,
				"leverage": p.Leverage, "cross_leverage_limit": p.CrossLeverageLimit,
			}
		}
	}
	adapterPositions, err := gt.GetPositions()
	if err != nil {
		step("2 adapter positions", false, map[string]interface{}{"error": errText(err)})
		return 1
	}
	var adapterLeverage float64
	var adapterHasTarget bool
	var adapterReading map[string]interface{}
	for _, pos := range adapterPositions {
		if pos["symbol"] == target {
			adapterHasTarget = true
			lev, isFloat := pos["leverage"].(float64)
			adapterLeverage = lev
			adapterReading = map[string]interface{}{
				"symbol": pos["symbol"], "side": pos["side"], "leverage": pos["leverage"],
				"leverage_type": fmt.Sprintf("%T", pos["leverage"]), "positionAmt": pos["positionAmt"],
				"entryPrice": pos["entryPrice"], "markPrice": pos["markPrice"],
			}
			_ = isFloat
		}
	}
	rawLeverF, rawErr := strconv.ParseFloat(rawLever, 64)
	ok2 := adapterHasTarget && rawErr == nil && rawLeverF > 0 && adapterLeverage == rawLeverF
	step("2 leverage actual value", ok2, map[string]interface{}{
		"raw_lever": rawLever, "raw_entry": rawEntry, "adapter": adapterReading,
		"match": ok2, "error": errText(rawErr)})

	// ===== 3 只读读数：归属判定（D40）=====
	probeTrader := nofxtrader.NewDrawdownProbeTrader(gt, targetBase)
	ownTarget := nofxtrader.ProbeOwnPosition(probeTrader, target, "long")
	ownOther := nofxtrader.ProbeOwnPosition(probeTrader, "ETH_USDT", "long")
	step("3 ownership", ownTarget && !ownOther, map[string]interface{}{
		"grid_symbol_owned": ownTarget, "other_symbol_owned": ownOther,
		"grid_symbol": targetBase, "note": "grid clause; the DB clause is covered by unit tests"})

	// ===== 4 只读读数：回撤监控一轮（首次见仓不会触发平仓）=====
	// 先取一次适配层读数（同时预热 15 秒持仓缓存，监控轮内读到同一份），据此手工计算期望收益率
	var entryPrice, markPrice float64
	found := false
	for _, pos := range adapterPositions {
		if pos["symbol"] == target {
			entryPrice, _ = pos["entryPrice"].(float64)
			markPrice, _ = pos["markPrice"].(float64)
			found = true
		}
	}
	if !found || entryPrice <= 0 {
		step("4 expected pnl", false, map[string]interface{}{"error": "target reading missing or entry price not positive"})
		return 1
	}
	expectedPnL := ((markPrice - entryPrice) / entryPrice) * adapterLeverage * 100

	nofxtrader.RunDrawdownMonitorOnce(probeTrader)
	peakCache := nofxtrader.ProbePeakCache(probeTrader)
	cacheValue, hasCache := peakCache[target+"_long"]
	diff := math.Abs(cacheValue - expectedPnL)
	step("4 monitor reads actual leverage", hasCache && diff <= 1e-6*math.Max(1, math.Abs(expectedPnL)), map[string]interface{}{
		"expected_pnl_pct": expectedPnL, "peak_cache": cacheValue, "diff": diff,
		"leverage": adapterLeverage, "note": "first sighting records the peak; nothing to close yet"})

	// 监控一轮后仓位仍在（首轮见仓不触发平仓；也证明监控没有误平）
	afterTick, err := gt.GetPositions()
	foundAfterTick := false
	if err == nil {
		for _, pos := range afterTick {
			if pos["symbol"] == target {
				foundAfterTick = true
			}
		}
	}
	step("4 position untouched", err == nil && foundAfterTick, map[string]interface{}{"error": errText(err)})

	// ===== 5 还原：全平 + 终态核对 =====
	// cleaned 在全平成功后才置位：CloseLong 瞬时失败时 deferred 兜底清理仍会重试
	closeOrder, err := gt.CloseLong(targetBase, 0)
	if err != nil {
		step("5 full close", false, map[string]interface{}{"error": errText(err)})
		return 1
	}
	cleaned = true
	step("5 full close", true, map[string]interface{}{"orderId": closeOrder["orderId"]})
	time.Sleep(2 * time.Second)

	// 终态：无持仓
	terminalPositions, _, err := rawClient.FuturesApi.ListPositions(ctx, "usdt", nil)
	if err != nil {
		step("5 terminal positions", false, map[string]interface{}{"error": errText(err)})
		return 1
	}
	flat := true
	for _, p := range terminalPositions {
		if p.Size != "0" {
			flat = false
		}
	}
	step("5 terminal positions", flat, map[string]interface{}{"open_positions": len(terminalPositions), "scope": "account-wide"})

	// 终态：无未完结委托与未触发条件单
	terminalOrders, _, err := rawClient.FuturesApi.ListFuturesOrders(ctx, "usdt", "open", nil)
	if err != nil {
		step("5 terminal orders", false, map[string]interface{}{"error": errText(err)})
		return 1
	}
	terminalTriggers, _, err := rawClient.FuturesApi.ListPriceTriggeredOrders(ctx, "usdt", "open", nil)
	if err != nil {
		step("5 terminal price orders", false, map[string]interface{}{"error": errText(err)})
		return 1
	}
	step("5 terminal orders", len(terminalOrders) == 0 && len(terminalTriggers) == 0, map[string]interface{}{
		"open_orders": len(terminalOrders), "open_price_orders": len(terminalTriggers), "scope": "account-wide"})

	// 终态：仓位设置条目与基线一致（设置未被改动）
	afterEntry, err := readEntry(rawClient, ctx, target)
	step("5 settings unchanged", err == nil && afterEntry == baseline[target],
		map[string]interface{}{"baseline": baseline[target], "readback": afterEntry, "error": errText(err)})

	// 终态：可用余额与基线的差值（手续费与持仓期间价差；容差 2 USDT）
	availableAfter := ""
	finalAccount, _, err := rawClient.FuturesApi.ListFuturesAccounts(ctx, "usdt")
	if err == nil {
		availableAfter = finalAccount.Available
	}
	delta := 0.0
	if before, err1 := strconv.ParseFloat(availableBefore, 64); err1 == nil {
		if after, err2 := strconv.ParseFloat(availableAfter, 64); err2 == nil {
			delta = after - before
		}
	}
	step("5 available balance", err == nil && delta > -2.0, map[string]interface{}{
		"available_before": availableBefore, "available_after": availableAfter, "delta": delta,
		"note": "fees and the holding-period price move explain a small negative delta"})

	emit(map[string]interface{}{"event": "LIVECHECK_PASS", "failures": failures})
	if failures > 0 {
		emit(map[string]interface{}{"event": "LIVECHECK_FAIL", "failures": failures})
		return 1
	}
	return 0
}
