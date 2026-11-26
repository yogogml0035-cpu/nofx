package main

// 真实写操作验证（子任务 ⑩ partial-close，D35 授权范围内：W2 分批平仓）：
//
//   预检：DOGE_USDT 无持仓、无未触发条件单、无挂单；记录当前仓位设置
//   1. 随单保护开多 17 张（≈9.5U，止损止盈现价 ±10%），核对仓位与保护单
//   2. PartialCloseStatus 报告可分批（17 张、步长 1）
//   3. 50% 分批平仓：8 张、剩 9 张；核对仓位与剩余保护
//   4. 比例 100% 与过小比例 1% 被拒绝，仓位不变
//   5. 市价全平，确认无持仓、无条件单、无挂单；仓位设置不变
//
// 每一步打印结果并在异常时尽力还原（残留仓位市价全平）。
// 凭据从 GATE_KEY / GATE_SECRET 环境变量读取，不落盘、不打印凭据。
// 容器出口走 VPN 不在 key 白名单：在 Docker 内 CGO_ENABLED=0 GOOS=darwin GOARCH=arm64
// 交叉编译，宿主机运行。

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	nofxgate "nofx/trader/gate"
	"nofx/trader/types"
)

const symbol = "DOGE_USDT"
const contractsPerStep = int64(10) // DOGE_USDT 每张 10 币

type evidence struct {
	Step   string `json:"step"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

var (
	failures     []string
	evidenceRows []evidence
)

func step(name string, ok bool, detail string) {
	mark := "✅"
	if !ok {
		mark = "❌"
		failures = append(failures, name+": "+detail)
	}
	// 空 detail（例如“终态零残留”通过时）不留行尾空格：证据文件要过 git diff --check
	line := strings.TrimRight(fmt.Sprintf("%s %s %s", mark, name, detail), " ")
	fmt.Println(line)
	evidenceRows = append(evidenceRows, evidence{Step: name, OK: ok, Detail: detail})
}

func main() {
	out := flag.String("out", "", "write JSONL evidence to this path")
	flag.Parse()

	key, secret := os.Getenv("GATE_KEY"), os.Getenv("GATE_SECRET")
	if key == "" || secret == "" {
		fmt.Println("GATE_KEY / GATE_SECRET 未设置")
		os.Exit(1)
	}

	gt := nofxgate.NewGateTrader(key, secret)

	// ===== 预检 =====
	positions, err := gt.GetPositions()
	if err != nil {
		fmt.Println("预检失败（读持仓）:", err)
		os.Exit(1)
	}
	residue := ""
	for _, pos := range positions {
		if pos["symbol"] == symbol {
			residue += fmt.Sprintf("[已有持仓 %v]", pos["positionAmt"])
		}
	}
	views, err := gt.ListTriggerOrders(symbol)
	if err != nil {
		fmt.Println("预检失败（读条件单）:", err)
		os.Exit(1)
	}
	if len(views) != 0 {
		residue += fmt.Sprintf("[已有条件单 %d 条]", len(views))
	}
	orders, err := gt.GetOpenOrders(symbol)
	if err != nil {
		fmt.Println("预检失败（读挂单）:", err)
		os.Exit(1)
	}
	if len(orders) != 0 {
		residue += fmt.Sprintf("[已有挂单 %d 条]", len(orders))
	}
	if residue != "" {
		fmt.Println("预检失败：", residue, "（不在其上执行 W2）")
		os.Exit(1)
	}

	settings, err := gt.PositionMarginSettings(symbol)
	if err != nil {
		fmt.Println("预检失败（读仓位设置）:", err)
		os.Exit(1)
	}
	step("预检", true, fmt.Sprintf("无持仓/无条件单/无挂单；仓位设置 %s", settings))

	// 开仓前记录余额，终态比对手续费
	balanceBefore, _ := gt.GetBalance()

	// ===== 1. 随单保护开多 17 张 =====
	price, err := gt.GetMarketPrice(symbol)
	if err != nil {
		step("取价", false, err.Error())
		finish(gt, *out)
		return
	}
	notional := price * float64(contractsPerStep) * 17 * 1.05 // 5% 裕度，避免价格微动少一步
	openRes, err := gt.OpenLongProtected(types.ProtectedOpenRequest{
		Symbol:        symbol,
		NotionalUSD:   notional,
		Leverage:      settings.Leverage,
		IsCrossMargin: settings.Cross,
		StopLoss:      price * 0.9,
		TakeProfit:    price * 1.1,
		MinNotional:   0,
		SlipRatio:     0.01,
	})
	if err != nil {
		step("随单保护开仓", false, err.Error())
		finish(gt, *out)
		return
	}
	step("随单保护开仓", true, fmt.Sprintf("orderId=%s contracts=%s notional=%.4f fillPrice=%.6g",
		openRes.OrderID, openRes.Contracts, openRes.Notional, openRes.FillPrice))

	openedContracts, err := strconv.ParseInt(strings.TrimPrefix(openRes.Contracts, "-"), 10, 64)
	if err != nil || openedContracts < 2 {
		step("开仓张数核对", false, fmt.Sprintf("contracts=%q 不是可分批的正整数张", openRes.Contracts))
		finish(gt, *out)
		return
	}
	step("开仓张数核对", true, fmt.Sprintf("%d 张（目标 17）", openedContracts))

	time.Sleep(2 * time.Second)
	positions, err = gt.GetPositions()
	positionOK := false
	positionDetail := ""
	if err != nil {
		positionDetail = err.Error()
	} else {
		for _, pos := range positions {
			if pos["symbol"] == symbol && pos["side"] == "long" {
				positionOK = true
				positionDetail = fmt.Sprintf("positionAmt=%v entry=%v leverage=%v",
					pos["positionAmt"], pos["entryPrice"], pos["leverage"])
			}
		}
	}
	step("开仓仓位核对", positionOK, positionDetail)

	if err := gt.VerifyOrderProtection(symbol, openRes.OrderID); err != nil {
		step("开仓保护核对", false, err.Error())
	} else {
		step("开仓保护核对", true, "随单止损止盈已确认")
	}

	// ===== 2. 可分批状态 =====
	status, err := gt.PartialCloseStatus(symbol, "long")
	if err != nil {
		step("可分批状态", false, err.Error())
		finish(gt, *out)
		return
	}
	step("可分批状态", status.Supported && status.Contracts == openRes.Contracts,
		fmt.Sprintf("supported=%v contracts=%s step=%s reason=%s",
			status.Supported, status.Contracts, status.SizeStep, status.Reason))

	// ===== 3. 50% 分批平仓 =====
	wantClosed := openedContracts / 2
	wantRemaining := openedContracts - wantClosed
	partial, err := gt.ClosePartial(types.PartialCloseRequest{Symbol: symbol, Side: "long", Percent: 50})
	if err != nil {
		step("50% 分批平仓", false, err.Error())
		finish(gt, *out)
		return
	}
	step("50% 分批平仓", true, fmt.Sprintf("orderId=%s contracts=%s remaining=%s fillPrice=%.6g notional=%.4f",
		partial.OrderID, partial.Contracts, partial.Remaining, partial.FillPrice, partial.Notional))
	step("分批张数核对", partial.Contracts == strconv.FormatInt(wantClosed, 10) &&
		partial.Remaining == strconv.FormatInt(wantRemaining, 10),
		fmt.Sprintf("期望 平%s/剩%s，实际 平%s/剩%s",
			strconv.FormatInt(wantClosed, 10), strconv.FormatInt(wantRemaining, 10),
			partial.Contracts, partial.Remaining))

	time.Sleep(2 * time.Second)
	positions, err = gt.GetPositions()
	remainingOK := false
	remainingDetail := ""
	if err != nil {
		remainingDetail = err.Error()
	} else {
		for _, pos := range positions {
			if pos["symbol"] == symbol && pos["side"] == "long" {
				amount, _ := pos["positionAmt"].(float64)
				remainingOK = int64(amount/float64(contractsPerStep)+0.5) == wantRemaining
				remainingDetail = fmt.Sprintf("positionAmt=%v（期望 %d 币）", amount, wantRemaining*contractsPerStep)
			}
		}
	}
	step("分批后仓位核对", remainingOK, remainingDetail)

	statusAfter, err := gt.PartialCloseStatus(symbol, "long")
	if err != nil {
		step("分批后可分批状态", false, err.Error())
	} else {
		step("分批后可分批状态", statusAfter.Supported && statusAfter.Contracts == strconv.FormatInt(wantRemaining, 10),
			fmt.Sprintf("supported=%v contracts=%s step=%s", statusAfter.Supported, statusAfter.Contracts, statusAfter.SizeStep))
	}

	// 保护不变量：分批后剩余仓位仍有保护（委托回读；条件单列表只观察）
	if err := gt.VerifyOrderProtection(symbol, openRes.OrderID); err != nil {
		step("分批后保护核对", false, err.Error())
	} else {
		step("分批后保护核对", true, "开仓委托的随单止损止盈仍可读回")
	}
	views, err = gt.ListTriggerOrders(symbol)
	if err != nil {
		fmt.Printf("    条件单列表读取失败: %v\n", err)
	} else {
		fmt.Printf("    条件单列表 status=open 共 %d 条（随单 tpsl 为委托级，不经此列表）\n", len(views))
	}

	// ===== 4. 拒绝用例（无副作用） =====
	_, err = gt.ClosePartial(types.PartialCloseRequest{Symbol: symbol, Side: "long", Percent: 100})
	step("100% 分批被拒", err != nil && strings.Contains(err.Error(), "full close"), fmt.Sprintf("%v", err))

	_, err = gt.ClosePartial(types.PartialCloseRequest{Symbol: symbol, Side: "long", Percent: 1})
	step("过小比例被拒", err != nil && strings.Contains(err.Error(), "below one size step"), fmt.Sprintf("%v", err))

	// 拒绝用例之后用适配器的未缓存持仓读取（findPositionSize 直接查交易所）核对，
	// 避免 GetPositions 的 15 秒缓存把副作用掩盖掉
	statusAfterRejects, err := gt.PartialCloseStatus(symbol, "long")
	unchanged := err == nil && statusAfterRejects.Contracts == strconv.FormatInt(wantRemaining, 10)
	step("拒绝用例无副作用", unchanged, fmt.Sprintf("未缓存读数 contracts=%s（期望 %d）", statusAfterRejects.Contracts, wantRemaining))

	// ===== 5. 市价全平与终态 =====
	if _, err := gt.CloseLong(symbol, 0); err != nil {
		step("市价全平", false, err.Error())
		finish(gt, *out)
		return
	}
	step("市价全平", true, "close=true, size=0")

	time.Sleep(3 * time.Second)
	positions, err = gt.GetPositions()
	residue = ""
	if err != nil {
		residue = "读持仓失败: " + err.Error()
	} else {
		for _, pos := range positions {
			if pos["symbol"] == symbol {
				residue += fmt.Sprintf("[残留持仓 %v]", pos["positionAmt"])
			}
		}
	}
	views, err = gt.ListTriggerOrders(symbol)
	if err != nil {
		residue += " 读条件单失败: " + err.Error()
	} else if len(views) != 0 {
		residue += fmt.Sprintf("[残留条件单 %d 条]", len(views))
	}
	orders, err = gt.GetOpenOrders(symbol)
	if err != nil {
		residue += " 读挂单失败: " + err.Error()
	} else if len(orders) != 0 {
		residue += fmt.Sprintf("[残留挂单 %d 条]", len(orders))
	}
	step("终态零残留", residue == "", residue)

	settingsAfter, err := gt.PositionMarginSettings(symbol)
	step("仓位设置不变", err == nil && settingsAfter == settings,
		fmt.Sprintf("before=%s after=%s err=%v", settings, settingsAfter, err))

	if balanceAfter, err := gt.GetBalance(); err == nil && balanceBefore != nil {
		if before, ok := balanceBefore["availableBalance"].(float64); ok {
			if after, ok := balanceAfter["availableBalance"].(float64); ok {
				fmt.Printf("    可用余额 %.5f → %.5f（差 %+.5f，含开平仓 taker 手续费）\n",
					before, after, after-before)
			}
		}
	}

	finish(gt, *out)
}

// finish 尽最大努力还原（残留持仓市价全平），写证据并给出最终结论
func finish(gt *nofxgate.GateTrader, out string) {
	if positions, err := gt.GetPositions(); err == nil {
		for _, pos := range positions {
			if pos["symbol"] == symbol {
				fmt.Println("    还原：发现残留仓位，市价全平")
				_, _ = gt.CloseLong(symbol, 0)
			}
		}
	}

	if out != "" {
		f, err := os.Create(out)
		if err == nil {
			enc := json.NewEncoder(f)
			for _, row := range evidenceRows {
				enc.Encode(row)
			}
			f.Close()
		}
	}

	if len(failures) > 0 {
		fmt.Printf("\nLIVECHECK_FAIL steps=%d\n", len(failures))
		for _, f := range failures {
			fmt.Println("  -", f)
		}
		os.Exit(1)
	}
	fmt.Println("\nLIVECHECK_PASS")
}
