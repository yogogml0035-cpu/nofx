package main

// 真实写操作验证 W5（父任务收口，D41 授权范围内）：
// 在一个从没有设置过仓位设置的合约上调用 set_leverage（全仓、逐仓与杠杆），从设置接口的返回值
// 与持仓条目读回。设置操作全部调用交付的适配器代码（与线上同一路径）；只读用途使用单独的 SDK 客户端。
//
//	预检  账户的持仓模式、保证金模式与可用余额；全账户无未完结委托与未触发条件单；
//	      目标合约没有仓位设置条目（ErrNoPositionSettings）也没有持仓条目（POSITION_NOT_FOUND）；
//	      记录三个不得被改动的合约的仓位设置条目作为基线
//	1     全仓 10x（首次建条目）      2  逐仓 5x
//	3     SetLeverage 10x（保持逐仓）  4  SetMarginMode 全仓（保持 10x）
//	5     终态：目标条目留在全仓 10x（授权内例外，无法还原为「没有条目」），
//	      三个基线条目不变，无未完结委托与未触发条件单，可用余额与预检一致
//
// W5 不下单，不产生资金变动。凭据从 GATE_KEY / GATE_SECRET 环境变量读取，不落盘、不打印凭据。
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

	"github.com/gate/gateapi-go/v7"

	nofxgate "nofx/trader/gate"
)

// candidates 依次尝试的「从未设置过」合约；取第一个既无设置条目也无持仓条目的
var candidates = []string{
	"SOL_USDT", "XRP_USDT", "LTC_USDT", "LINK_USDT", "AVAX_USDT", "BCH_USDT",
	"UNI_USDT", "ADA_USDT", "DOT_USDT", "NEAR_USDT", "APT_USDT", "OP_USDT",
}

// watched 基线与终态核对的合约：不得被 W5 改动
var watched = []string{"BTC_USDT", "DOGE_USDT", "ETH_USDT"}

var failures int

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
	availableBefore := account.Available
	step("preflight account", true, map[string]interface{}{
		"position_mode": account.PositionMode, "margin_mode": account.MarginMode,
		"in_dual_mode": account.InDualMode, "available": availableBefore,
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

	// 选目标：没有设置条目，也没有持仓条目
	target := ""
	scanned := map[string]string{}
	for _, contract := range candidates {
		_, settingsErr := gt.PositionMarginSettings(contract)
		if !errors.Is(settingsErr, nofxgate.ErrNoPositionSettings) {
			scanned[contract] = "has settings: " + errText(settingsErr)
			continue
		}
		_, _, positionErr := rawClient.FuturesApi.GetPosition(ctx, "usdt", contract)
		var gateErr gateapi.GateAPIError
		if !errors.As(positionErr, &gateErr) || gateErr.Label != "POSITION_NOT_FOUND" {
			scanned[contract] = "has position entry: " + errText(positionErr)
			continue
		}
		target = contract
		break
	}
	step("preflight fresh contract", target != "", map[string]interface{}{
		"target": target, "scanned": scanned})
	if target == "" {
		return 1
	}

	// expectSettings 读回目标合约：适配器读回与原始条目都与目标一致，且没有产生持仓
	expectSettings := func(name, mode string, leverage int) {
		want := strconv.Itoa(leverage)
		settings, err := gt.PositionMarginSettings(target)
		adapterOK := err == nil && settings.Cross == (mode == "cross") && settings.Leverage == leverage
		after, readErr := readEntry(rawClient, ctx, target)
		modeField := after.Leverage
		if mode == "cross" {
			modeField = after.CrossLeverageLimit
		}
		entryOK := readErr == nil && after.PosMarginMode == mode && modeField == want &&
			after.Lever == want && after.Size == "0"
		step(name, adapterOK && entryOK, map[string]interface{}{
			"want": mode + " " + want, "adapter": fmt.Sprint(settings), "adapter_error": errText(err),
			"readback": after, "readback_error": errText(readErr)})
	}
	apply := func(name string, cross bool, leverage int) {
		err := gt.ApplyMarginSettings(target, cross, leverage)
		step(name, err == nil, map[string]interface{}{"error": errText(err)})
	}

	// ===== 1 全仓 10x：首次建条目 =====
	apply("1 set cross 10x on fresh contract", true, 10)
	expectSettings("1 read back", "cross", 10)

	// ===== 2 逐仓 5x =====
	apply("2 set isolated 5x", false, 5)
	expectSettings("2 read back", "isolated", 5)

	// ===== 3 SetLeverage：保持逐仓，只改杠杆 =====
	err = gt.SetLeverage(target, 10)
	step("3 SetLeverage 10x", err == nil, map[string]interface{}{"error": errText(err)})
	expectSettings("3 read back keeps isolated", "isolated", 10)

	// ===== 4 SetMarginMode：保持杠杆，只改模式 =====
	err = gt.SetMarginMode(target, true)
	step("4 SetMarginMode cross", err == nil, map[string]interface{}{"error": errText(err)})
	expectSettings("4 read back keeps 10x", "cross", 10)

	// ===== 5 终态 =====
	unchanged := true
	final := map[string]interface{}{}
	for _, contract := range watched {
		after, err := readEntry(rawClient, ctx, contract)
		if err != nil || after != baseline[contract] {
			unchanged = false
		}
		final[contract] = after
	}
	targetFinal, targetErr := readEntry(rawClient, ctx, target)
	final[target] = targetFinal
	orders, _, err = rawClient.FuturesApi.ListFuturesOrders(ctx, "usdt", "open", nil)
	if err != nil {
		unchanged = false
	}
	triggers, _, err = rawClient.FuturesApi.ListPriceTriggeredOrders(ctx, "usdt", "open", nil)
	if err != nil {
		unchanged = false
	}
	finalAccount, _, err := rawClient.FuturesApi.ListFuturesAccounts(ctx, "usdt")
	if err != nil {
		unchanged = false
	}
	availableAfter := ""
	if err == nil {
		availableAfter = finalAccount.Available
	}
	targetLeft := targetErr == nil && targetFinal.PosMarginMode == "cross" &&
		targetFinal.CrossLeverageLimit == "10" && targetFinal.Lever == "10" && targetFinal.Size == "0"
	clean := unchanged && targetLeft && len(orders) == 0 && len(triggers) == 0 && availableAfter == availableBefore
	step("5 final state", clean, map[string]interface{}{
		"entries": final, "open_orders": len(orders), "open_price_orders": len(triggers),
		"available_before": availableBefore, "available_after": availableAfter,
		"target_left_at_cross_10x": targetLeft, "account_error": errText(err)})

	if failures > 0 {
		fmt.Fprintf(out, "LIVECHECK_FAIL failures=%d\n", failures)
		fmt.Fprintf(os.Stderr, "LIVECHECK_FAIL failures=%d\n", failures)
		return 1
	}
	fmt.Fprintln(out, "LIVECHECK_PASS")
	fmt.Fprintln(os.Stderr, "LIVECHECK_PASS")
	return 0
}
