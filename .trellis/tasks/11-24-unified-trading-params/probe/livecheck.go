package main

// 真实写操作验证 W4（子任务 unified-trading-params，D35 授权范围内）：
// 在一个未持仓的合约上设置全仓、逐仓与杠杆，从设置接口的返回值与持仓条目读回，
// 结束后还原原设置。设置操作全部调用交付的适配器代码（与线上同一路径）；
// 只读用途（账户、持仓条目、挂单）与第 2 步对「重复设置」的原始响应观察使用单独的 SDK 客户端。
//
//	预检  账户的持仓模式与保证金模式；目标合约无持仓、无挂单、无条件单；
//	      记录三个合约的仓位设置条目作为基线；从未设置过的合约读不到条目
//	1     全仓 10x            2  再设一次全仓 10x（重复设置）
//	3     全仓 5x             4  逐仓 5x           5  逐仓 3x
//	6     非法值：超过合约上限（100 > 75）、杠杆 0 —— 预期被拒且读回不变
//	7     SetLeverage 10x（保持逐仓）   8  SetMarginMode 全仓（保持杠杆）
//	9     还原为基线           10 终态：三个合约的条目与基线一致，无持仓与挂单
//
// 还原不依赖前面步骤的结果（用预检记录的基线），并注册为兜底：中途失败也会执行。
// 凭据从 GATE_KEY / GATE_SECRET 环境变量读取，不落盘、不打印凭据。
// 容器出口走 VPN 不在 key 白名单：在 Docker 内 CGO_ENABLED=0 GOOS=darwin GOARCH=arm64
// 交叉编译，宿主机运行。证据为 JSONL：默认写到 stdout，用 -out 指定文件时适配器的日志
// （stdout）不会混进证据。

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/antihax/optional"
	"github.com/gate/gateapi-go/v7"

	nofxgate "nofx/trader/gate"
)

const target = "DOGE_USDT"

// watched 基线与终态核对的合约：目标合约 + 两个不得被改动的合约
var watched = []string{"BTC_USDT", "DOGE_USDT", "ETH_USDT"}

var failures int

// out 证据输出；适配器的日志走 stdout
var out io.Writer = os.Stdout

// entry 仓位条目里与保证金设置有关的字段（原样记录，线上格式）
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

// step 记录一步的结果；失败只计数，不中断（后续的还原仍会执行）
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

// leverageOf 按模式取条目的设置杠杆：全仓 cross_leverage_limit、逐仓 leverage
func leverageOf(e entry) (int, error) {
	raw := e.Leverage
	if e.PosMarginMode == "cross" {
		raw = e.CrossLeverageLimit
	}
	return strconv.Atoi(raw)
}

func describe(e entry) string {
	leverage, _ := leverageOf(e)
	return fmt.Sprintf("%s %dx", e.PosMarginMode, leverage)
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

	// 被验证的对象：交付的适配器
	gt := nofxgate.NewGateTrader(key, secret)
	// 只读用途的原始客户端
	rawConfig := gateapi.NewConfiguration()
	rawClient := gateapi.NewAPIClient(rawConfig)
	ctx := context.WithValue(context.Background(), gateapi.ContextGateAPIV4,
		gateapi.GateAPIV4{Key: key, Secret: secret})

	// ===== 预检（只读）=====
	account, _, err := rawClient.FuturesApi.ListFuturesAccounts(ctx, "usdt")
	if err != nil {
		step("preflight account", false, map[string]interface{}{"error": errText(err)})
		return 1
	}
	step("preflight account", true, map[string]interface{}{
		"position_mode": account.PositionMode, "margin_mode": account.MarginMode,
		"in_dual_mode": account.InDualMode, "available": account.Available,
	})

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
			"error": fmt.Sprintf("%s has a position of size %s; W4 runs on an idle contract only", target, baseline[target].Size)})
		return 1
	}
	orders, _, err := rawClient.FuturesApi.ListFuturesOrders(ctx, "usdt", "open", nil)
	if err != nil {
		step("preflight orders", false, map[string]interface{}{"error": errText(err)})
		return 1
	}
	triggers, _, err := rawClient.FuturesApi.ListPriceTriggeredOrders(ctx, "usdt", "open",
		&gateapi.ListPriceTriggeredOrdersOpts{Contract: optional.NewString(target)})
	if err != nil {
		step("preflight price orders", false, map[string]interface{}{"error": errText(err)})
		return 1
	}
	idle := len(orders) == 0 && len(triggers) == 0
	step("preflight idle", idle, map[string]interface{}{"open_orders": len(orders), "open_price_orders": len(triggers)})
	if !idle {
		return 1
	}

	// 从未设置过的合约：没有条目 → ErrNoPositionSettings
	_, err = gt.PositionMarginSettings("SOL_USDT")
	step("preflight no-entry contract", errors.Is(err, nofxgate.ErrNoPositionSettings),
		map[string]interface{}{"error": errText(err)})

	// ===== 还原：只执行一次，用基线；中途失败也会执行 =====
	restored := false
	restore := func(name string) {
		if restored {
			return
		}
		restored = true
		want := baseline[target]
		leverage, err := leverageOf(want)
		if err != nil {
			step(name, false, map[string]interface{}{"error": "baseline leverage unreadable: " + err.Error()})
			return
		}
		cross := want.PosMarginMode == "cross"
		if err := gt.ApplyMarginSettings(target, cross, leverage); err != nil {
			step(name, false, map[string]interface{}{"error": errText(err), "want": describe(want)})
			return
		}
		after, err := readEntry(rawClient, ctx, target)
		step(name, err == nil && after == want, map[string]interface{}{"want": want, "readback": after, "error": errText(err)})
	}
	defer restore("restore (deferred)")

	// expectSettings 读回目标合约的条目：模式、该模式下的杠杆字段、lever 都与目标一致，且没有产生持仓
	expectSettings := func(name, mode string, leverage int) {
		after, err := readEntry(rawClient, ctx, target)
		if err != nil {
			step(name, false, map[string]interface{}{"error": errText(err)})
			return
		}
		want := strconv.Itoa(leverage)
		modeField := after.Leverage
		if mode == "cross" {
			modeField = after.CrossLeverageLimit
		}
		ok := after.PosMarginMode == mode && modeField == want && after.Lever == want && after.Size == "0"
		step(name, ok, map[string]interface{}{"want": mode + " " + want, "readback": after})
	}
	apply := func(name string, cross bool, leverage int) {
		err := gt.ApplyMarginSettings(target, cross, leverage)
		step(name, err == nil, map[string]interface{}{"error": errText(err)})
	}

	// ===== 1 全仓 10x =====
	apply("1 set cross 10x", true, 10)
	expectSettings("1 read back", "cross", 10)

	// ===== 2 再设一次全仓 10x（重复设置）=====
	apply("2 repeat cross 10x via adapter", true, 10)
	// 原始调用：观察交易所对「设置成与当前相同的值」的响应
	repeatPosition, _, repeatErr := rawClient.FuturesApi.UpdateContractPositionLeverage(ctx, "usdt", target, "10", "cross", nil)
	step("2 repeat cross 10x raw observe", true, map[string]interface{}{
		"error": errText(repeatErr), "response": toEntry(repeatPosition)})
	expectSettings("2 read back", "cross", 10)

	// ===== 3 全仓 5x =====
	apply("3 set cross 5x", true, 5)
	expectSettings("3 read back", "cross", 5)

	// ===== 4 逐仓 5x =====
	apply("4 set isolated 5x", false, 5)
	expectSettings("4 read back", "isolated", 5)

	// ===== 5 逐仓 3x =====
	apply("5 set isolated 3x", false, 3)
	expectSettings("5 read back", "isolated", 3)

	// ===== 6 非法值：预期被拒，读回不变 =====
	err = gt.ApplyMarginSettings(target, true, 100) // 超过 DOGE_USDT 的杠杆上限 75
	step("6 above max rejected", err != nil, map[string]interface{}{"error": errText(err)})
	err = gt.ApplyMarginSettings(target, true, 0)
	step("6 zero leverage rejected", err != nil, map[string]interface{}{"error": errText(err)})
	expectSettings("6 read back unchanged", "isolated", 3)

	// ===== 7 SetLeverage：保持逐仓，只改杠杆 =====
	err = gt.SetLeverage(target, 10)
	step("7 SetLeverage 10x", err == nil, map[string]interface{}{"error": errText(err)})
	expectSettings("7 read back keeps isolated", "isolated", 10)

	// ===== 8 SetMarginMode：保持杠杆，只改模式 =====
	err = gt.SetMarginMode(target, true)
	step("8 SetMarginMode cross", err == nil, map[string]interface{}{"error": errText(err)})
	expectSettings("8 read back keeps 10x", "cross", 10)

	// ===== 9 还原 =====
	restore("9 restore to baseline")

	// ===== 10 终态 =====
	unchanged := true
	final := map[string]interface{}{}
	for _, contract := range watched {
		after, err := readEntry(rawClient, ctx, contract)
		if err != nil || after != baseline[contract] {
			unchanged = false
		}
		final[contract] = after
	}
	orders, _, err = rawClient.FuturesApi.ListFuturesOrders(ctx, "usdt", "open", nil)
	if err != nil {
		unchanged = false
	}
	triggers, _, err = rawClient.FuturesApi.ListPriceTriggeredOrders(ctx, "usdt", "open",
		&gateapi.ListPriceTriggeredOrdersOpts{Contract: optional.NewString(target)})
	if err != nil {
		unchanged = false
	}
	clean := unchanged && len(orders) == 0 && len(triggers) == 0
	step("10 final state", clean, map[string]interface{}{
		"entries": final, "open_orders": len(orders), "open_price_orders": len(triggers)})

	if failures > 0 {
		fmt.Fprintf(out, "LIVECHECK_FAIL failures=%d\n", failures)
		fmt.Fprintf(os.Stderr, "LIVECHECK_FAIL failures=%d\n", failures)
		return 1
	}
	fmt.Fprintln(out, "LIVECHECK_PASS")
	fmt.Fprintln(os.Stderr, "LIVECHECK_PASS")
	return 0
}
