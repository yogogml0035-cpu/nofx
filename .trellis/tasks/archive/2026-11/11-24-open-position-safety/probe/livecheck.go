package main

// 真实写操作验证（子任务 open-position-safety，D35 授权范围内：W2 开仓与全平、W3）：
//
//   W2  DOGE_USDT 随单保护开多 17 张（≈9.5U，随单止损止盈设现价 ±10%）→ 读仓位核对张数与方向
//       → VerifyOrderProtection → 条件单列表核对止损止盈在列 → 市价全平（close=true+size=0）
//       → 确认无持仓、无未触发条件单、无挂单
//   W3  ETH_USDT 以 0.1 张（最小步长，≈1.7U）开多并全平，验证小数张与随单止盈止损
//
// 每一步打印结果并在异常时尽力还原（残留仓位市价全平）。
// 凭据从 GATE_KEY / GATE_SECRET 环境变量读取，不落盘、不打印凭据。
// 容器出口走 VPN 不在 key 白名单：在 Docker 内 CGO_ENABLED=0 GOOS=darwin GOARCH=arm64
// 交叉编译，宿主机运行。

import (
	"fmt"
	"os"
	"time"

	nofxgate "nofx/trader/gate"
	"nofx/trader/types"
)

const settleFunding = "USDT"

var failures []string

func step(name string, ok bool, detail string) {
	mark := "✅"
	if !ok {
		mark = "❌"
		failures = append(failures, name+": "+detail)
	}
	fmt.Printf("%s %s %s\n", mark, name, detail)
}

func main() {
	key := os.Getenv("GATE_KEY")
	secret := os.Getenv("GATE_SECRET")
	if key == "" || secret == "" {
		fmt.Println("GATE_KEY / GATE_SECRET 未设置")
		os.Exit(1)
	}

	gt := nofxgate.NewGateTrader(key, secret)

	// ===== 预检：测试合约上不得有已有持仓或未触发条件单 =====
	for _, symbol := range []string{"DOGE_USDT", "ETH_USDT"} {
		positions, err := gt.GetPositions()
		if err != nil {
			fmt.Println("预检失败（读持仓）:", err)
			os.Exit(1)
		}
		for _, pos := range positions {
			if pos["symbol"] == symbol {
				fmt.Printf("预检失败：%s 已有持仓 %v，不在其上开新仓（D35 约束）\n", symbol, pos["positionAmt"])
				os.Exit(1)
			}
		}
		views, err := gt.ListTriggerOrders(symbol)
		if err != nil {
			fmt.Println("预检失败（读条件单）:", err)
			os.Exit(1)
		}
		if len(views) != 0 {
			fmt.Printf("预检失败：%s 已有 %d 张未触发条件单\n", symbol, len(views))
			os.Exit(1)
		}
	}

	// ===== W2: DOGE_USDT 17 张 =====
	runW2(gt)
	// ===== W3: ETH_USDT 0.1 张 =====
	runW3(gt)

	if len(failures) > 0 {
		fmt.Printf("\nLIVECHECK_FAIL steps=%d\n", len(failures))
		for _, f := range failures {
			fmt.Println("  -", f)
		}
		os.Exit(1)
	}
	fmt.Println("\nLIVECHECK_PASS")
}

// contractsNotional 计算 N 张按现价的名义价值（USDT）
func contractsNotional(gt *nofxgate.GateTrader, symbol string, contracts float64) float64 {
	price, err := gt.GetMarketPrice(symbol)
	if err != nil {
		fmt.Println("取价失败:", err)
		os.Exit(1)
	}
	return price * contracts * contractsMultiplier(symbol)
}

// contractsMultiplier 每张合约的标的币数量（与 3.2 节实测元数据一致）
func contractsMultiplier(symbol string) float64 {
	if symbol == "DOGE_USDT" {
		return 10
	}
	if symbol == "ETH_USDT" {
		return 0.01
	}
	return 1
}

// runProtectedOpenFlow 一轮「随单保护开仓 → 核对 → 全平 → 确认无残留」的完整流程
func runProtectedOpenFlow(gt *nofxgate.GateTrader, symbol string, contracts float64, multiplier float64) {
	symbolBase := symbol
	side := "LONG"

	price, err := gt.GetMarketPrice(symbolBase)
	if err != nil {
		step(symbolBase+" 取价", false, err.Error())
		return
	}
	// 名义价值按目标张数再加 5% 裕度：取价与换算之间价格会动，压在一步的边界上
	// 会因价格微动少一步而被拒（2026-11-24 首轮实测的教训）
	notional := price * contracts * multiplier * 1.05
	fmt.Printf("\n[%s] 现价 %.6g，目标 %v 张（%v 币/张），名义价值 %.4f USDT（含 5%% 裕度）\n",
		symbolBase, price, contracts, multiplier, notional)

	// 随单保护触发价：现价 ±10%，多头止损在下、止盈在上
	stopLoss := price * 0.9
	takeProfit := price * 1.1

	res, err := gt.OpenLongProtected(types.ProtectedOpenRequest{
		Symbol:      symbolBase,
		NotionalUSD: notional,
		Leverage:    0, // 不改账户杠杆设置（杠杆与保证金模式由子任务 5 按用户配置管理）
		StopLoss:    stopLoss,
		TakeProfit:  takeProfit,
		MinNotional: 0, // 真实验证按目标张数构造名义价值，不套用策略的最小下单金额
		SlipRatio:   0.01,
	})
	if err != nil {
		step(symbolBase+" 随单保护开仓", false, err.Error())
		return
	}
	step(symbolBase+" 随单保护开仓", true,
		fmt.Sprintf("orderId=%s contracts=%s notional=%.4f fillPrice=%.6g", res.OrderID, res.Contracts, res.Notional, res.FillPrice))
	contractsOK := fmt.Sprintf("%v", contracts) == trimTrailingZero(res.Contracts)
	step(symbolBase+" 张数核对", contractsOK,
		fmt.Sprintf("期望 %v 张，实际 %s 张", contracts, res.Contracts))

	// 开仓成交核对以下单返回值为准（F25）：读仓位确认方向与数量
	time.Sleep(2 * time.Second)
	positions, err := gt.GetPositions()
	if err != nil {
		step(symbolBase+" 读仓位", false, err.Error())
	} else {
		found := false
		for _, pos := range positions {
			if pos["symbol"] == symbolBase && pos["side"] == "long" {
				found = true
				step(symbolBase+" 仓位核对", true,
					fmt.Sprintf("side=long positionAmt=%v entry=%v leverage=%v", pos["positionAmt"], pos["entryPrice"], pos["leverage"]))
			}
		}
		if !found {
			step(symbolBase+" 仓位核对", false, "未找到多头仓位")
		}
	}

	// 成交后核对保护单存在（保护不变量）
	if err := gt.VerifyOrderProtection(symbolBase, res.OrderID); err != nil {
		step(symbolBase+" 保护核对", false, err.Error())
	} else {
		step(symbolBase+" 保护核对", true, "随单止损止盈已确认")
	}

	// 条件单列表观察（2026-11-24 实测发现：随单 tpsl 是委托级保护，不出现在未触发
	// 条件单列表 status=open 里——核对以 VerifyOrderProtection 的委托回读为准；
	// 列表步骤只记录观察，不作断言）
	views, err := gt.ListTriggerOrders(symbolBase)
	if err != nil {
		fmt.Printf("    条件单列表读取失败: %v\n", err)
	} else {
		for _, v := range views {
			fmt.Printf("    条件单 id=%s orderType=%s rule=%d trigger=%v kind=%v(%v)\n",
				v.OrderID(), v.OrderType, v.TriggerRule, v.TriggerPrice, v.Kind, v.KindKnown)
		}
		fmt.Printf("    条件单列表 status=open 共 %d 条（随单 tpsl 为委托级，不经此列表）\n", len(views))
	}

	// 市价全平（close=true + size=0）
	if _, err := gt.CloseLong(symbolBase, 0); err != nil {
		step(symbolBase+" 市价全平", false, err.Error())
	} else {
		step(symbolBase+" 市价全平", true, "close=true, size=0")
	}

	// 确认无残留：持仓、条件单、挂单
	time.Sleep(3 * time.Second)
	positions, err = gt.GetPositions()
	residue := ""
	if err != nil {
		residue = "读持仓失败: " + err.Error()
	} else {
		for _, pos := range positions {
			if pos["symbol"] == symbolBase {
				residue += fmt.Sprintf("[残留持仓 %v]", pos["positionAmt"])
			}
		}
	}
	views, err = gt.ListTriggerOrders(symbolBase)
	if err != nil {
		residue += " 读条件单失败: " + err.Error()
	} else if len(views) != 0 {
		residue += fmt.Sprintf("[残留条件单 %d 张]", len(views))
		for _, v := range views {
			fmt.Printf("    残留条件单 id=%s trigger=%v\n", v.OrderID(), v.TriggerPrice)
		}
	}
	orders, err := gt.GetOpenOrders(symbolBase)
	if err != nil {
		residue += " 读挂单失败: " + err.Error()
	} else if len(orders) != 0 {
		residue += fmt.Sprintf("[残留挂单 %d 张]", len(orders))
	}
	step(symbolBase+" 终态无残留", residue == "", residue)

	_ = side
}

// cleanupPosition 异常路径的还原：市价全平测试合约的多头仓位
func cleanupPosition(gt *nofxgate.GateTrader, symbol string) {
	fmt.Printf("[cleanup] 尝试市价全平 %s 多头仓位\n", symbol)
	if _, err := gt.CloseLong(symbol, 0); err != nil {
		fmt.Printf("[cleanup] %s 全平失败：%v（请人工核对账户）\n", symbol, err)
	}
}

func runW2(gt *nofxgate.GateTrader) {
	fmt.Println("===== W2: DOGE_USDT 17 张（≈9.5U）=====")
	runProtectedOpenFlow(gt, "DOGE_USDT", 17, 10)
	if hasResidue(gt, "DOGE_USDT") {
		cleanupPosition(gt, "DOGE_USDT")
	}
}

func runW3(gt *nofxgate.GateTrader) {
	fmt.Println("===== W3: ETH_USDT 0.1 张（最小步长）=====")
	runProtectedOpenFlow(gt, "ETH_USDT", 0.1, 0.01)
	if hasResidue(gt, "ETH_USDT") {
		cleanupPosition(gt, "ETH_USDT")
	}
}

// hasResidue 检查测试合约是否还有多头仓位（异常时还原用）
func hasResidue(gt *nofxgate.GateTrader, symbol string) bool {
	positions, err := gt.GetPositions()
	if err != nil {
		return false
	}
	for _, pos := range positions {
		if pos["symbol"] == symbol && pos["side"] == "long" {
			return true
		}
	}
	return false
}

func trimTrailingZero(s string) string {
	if len(s) > 1 && s[len(s)-1] == '0' {
		for len(s) > 1 && s[len(s)-1] == '0' {
			s = s[:len(s)-1]
		}
		if s[len(s)-1] == '.' {
			s = s[:len(s)-1]
		}
	}
	return s
}

var _ = settleFunding
