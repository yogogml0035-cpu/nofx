package main

// 真实读数验证（子任务 account-equity）：不带仓与带仓两组读数，全部经交付的适配器代码读取
// （types.ReadAccountBalance、trader.NewAutoTrader 的加载路径），与两个原始接口的返回值互相印证。
//
// 带仓读数需要账户里有仓位：沿用 W3（ETH_USDT 0.1 张，约 1.7 USDT，随单保护）开一个测试仓位，
// 请求带上该合约现有的保证金模式与杠杆（不改动账户的仓位设置），读数后立即全平。
// 在 D35 的授权范围内：单笔名义价值不超过 10 USDT，每一步完成后立即还原。
//
//	预检  全账户无持仓、无未完结委托、无未触发条件单；记录三个合约的仓位设置条目作为基线，
//	      ETH_USDT 必须已有设置条目（没有条目时开仓会新建一条，无法还原）
//	1     不带仓读数：ReadAccountBalance 与两个原始接口
//	2     不带仓的初始资金重建：trader.NewAutoTrader，配置里的初始资金给 0，不接数据库
//	3     W3：ETH_USDT 以 0.1 张随单保护开多，核对张数与保护单
//	4     带仓读数：约 75 秒内采样 7 次（超过钱包总额接口 1 分钟的缓存窗口），核对口径，
//	      并对比钱包总额、合约账户、持仓条目三处的未实现盈亏是否同步
//	5     全平；确认无持仓、无未完结委托、无未触发条件单
//	6     终态：三个合约的设置条目与基线一致；平仓后约 5 秒与 70 秒各读一次不带仓读数
//	7     提示词预览：用第 1 步的读数渲染系统提示词与用户提示词
//
// 还原注册为兜底：中途失败也会全平。凭据从 GATE_KEY / GATE_SECRET 环境变量读取，不落盘、不打印。
// 容器出口走 VPN 不在 key 白名单：在 Docker 内 CGO_ENABLED=0 GOOS=darwin GOARCH=arm64
// 交叉编译，宿主机运行。证据为 JSONL：默认写到 stdout，用 -out 指定文件时适配器的日志（stdout）
// 不会混进证据；提示词预览用 -prompt-out 指定文件。带 -dry-run 时只跑只读步骤（预检、1、2、7），到开仓前为止。

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/antihax/optional"
	"github.com/gate/gateapi-go/v7"

	"nofx/kernel"
	"nofx/store"
	"nofx/trader"
	nofxgate "nofx/trader/gate"
	"nofx/trader/types"
)

const (
	target = "ETH_USDT"
	// targetContracts W3 的目标张数：ETH_USDT 的最小步长
	targetContracts = "0.1"
)

// takerFee 对账时代入的 taker 费率 0.05%（经验值，精确十进制，只用于观察记录）
var takerFee = big.NewRat(5, 10000)

// watched 基线与终态核对的合约：目标合约 + 两个不得被改动的合约
var watched = []string{"BTC_USDT", "DOGE_USDT", "ETH_USDT"}

var (
	failures int
	out      io.Writer = os.Stdout
)

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

// observe 只记录、不计入成败：没有可断言的内容、但要留档的读数
func observe(name string, detail map[string]interface{}) {
	if detail == nil {
		detail = map[string]interface{}{}
	}
	detail["step"] = name
	detail["result"] = "OBSERVED"
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

// rat 把交易所返回的十进制字符串精确解析为有理数；空串按 0
func rat(s string) (*big.Rat, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return big.NewRat(0, 1), nil
	}
	v, ok := new(big.Rat).SetString(s)
	if !ok {
		return nil, fmt.Errorf("not a decimal string: %q", s)
	}
	return v, nil
}

// ratFloat64 float64 → 有理数（按最短十进制表示，不带二进制噪声）
func ratFloat64(v float64) *big.Rat {
	r, _ := rat(strconv.FormatFloat(v, 'f', -1, 64))
	return r
}

func f(r *big.Rat) float64 {
	v, _ := r.Float64()
	return v
}

// within |a − b| ≤ tol
func within(a, b *big.Rat, tol string) bool {
	limit := big.NewRat(0, 1)
	if tol != "" {
		limit, _ = rat(tol)
	}
	diff := new(big.Rat).Abs(new(big.Rat).Sub(a, b))
	return diff.Cmp(limit) <= 0
}

// rawAccount 两个原始接口的读数（线上格式，字符串）
type rawAccount struct {
	WalletAmount         string `json:"wallet_total_amount"`
	WalletBorrowed       string `json:"wallet_total_borrowed"`
	WalletUnrealised     string `json:"wallet_total_unrealised_pnl"`
	WalletCurrency       string `json:"wallet_total_currency"`
	FuturesTotal         string `json:"futures_account_total"`
	FuturesUnrealised    string `json:"futures_account_unrealised_pnl"`
	FuturesAvailable     string `json:"futures_account_available"`
	FuturesInitialMargin string `json:"futures_account_position_initial_margin"`
	FuturesOrderMargin   string `json:"futures_account_order_margin"`
	FuturesMaintenance   string `json:"futures_account_maintenance_margin"`
}

func readRaw(client *gateapi.APIClient, ctx context.Context) (rawAccount, error) {
	wallet, _, err := client.WalletApi.GetTotalBalance(ctx, &gateapi.GetTotalBalanceOpts{Currency: optional.NewString("USDT")})
	if err != nil {
		return rawAccount{}, fmt.Errorf("wallet total balance: %w", err)
	}
	account, _, err := client.FuturesApi.ListFuturesAccounts(ctx, "usdt")
	if err != nil {
		return rawAccount{}, fmt.Errorf("futures account: %w", err)
	}
	return rawAccount{
		WalletAmount: wallet.Total.Amount, WalletBorrowed: wallet.Total.Borrowed,
		WalletUnrealised: wallet.Total.UnrealisedPnl, WalletCurrency: wallet.Total.Currency,
		FuturesTotal: account.Total, FuturesUnrealised: account.UnrealisedPnl,
		FuturesAvailable: account.Available, FuturesInitialMargin: account.PositionInitialMargin,
		FuturesOrderMargin: account.OrderMargin, FuturesMaintenance: account.MaintenanceMargin,
	}, nil
}

// settingsEntry 仓位设置条目；Has 为 false 表示合约没有条目
type settingsEntry struct {
	Contract string `json:"contract"`
	Has      bool   `json:"has_entry"`
	Cross    bool   `json:"cross"`
	Leverage int    `json:"leverage"`
}

func readSettings(gt *nofxgate.GateTrader, contract string) (settingsEntry, error) {
	settings, err := gt.PositionMarginSettings(contract)
	if errors.Is(err, nofxgate.ErrNoPositionSettings) {
		return settingsEntry{Contract: contract}, nil
	}
	if err != nil {
		return settingsEntry{}, err
	}
	return settingsEntry{Contract: contract, Has: true, Cross: settings.Cross, Leverage: settings.Leverage}, nil
}

func readAllSettings(gt *nofxgate.GateTrader) (map[string]settingsEntry, error) {
	entries := map[string]settingsEntry{}
	for _, contract := range watched {
		entry, err := readSettings(gt, contract)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", contract, err)
		}
		entries[contract] = entry
	}
	return entries, nil
}

func sameSettings(a, b map[string]settingsEntry) bool {
	for _, contract := range watched {
		if a[contract] != b[contract] {
			return false
		}
	}
	return true
}

func settingsView(entries map[string]settingsEntry) map[string]interface{} {
	view := map[string]interface{}{}
	for _, contract := range watched {
		e := entries[contract]
		if !e.Has {
			view[contract] = "no entry"
			continue
		}
		mode := "isolated"
		if e.Cross {
			mode = "cross"
		}
		view[contract] = fmt.Sprintf("%s %dx", mode, e.Leverage)
	}
	return view
}

// positionView 目标合约的持仓条目；没有持仓时 Has 为 false
type positionView struct {
	Has        bool   `json:"has_position"`
	Size       string `json:"size"`
	EntryPrice string `json:"entry_price"`
	MarkPrice  string `json:"mark_price"`
	Unrealised string `json:"unrealised_pnl"`
	Leverage   string `json:"leverage"`
}

func readPosition(client *gateapi.APIClient, ctx context.Context) (positionView, error) {
	positions, _, err := client.FuturesApi.ListPositions(ctx, "usdt", nil)
	if err != nil {
		return positionView{}, err
	}
	for _, p := range positions {
		if p.Contract != target {
			continue
		}
		size, err := rat(p.Size)
		if err != nil {
			return positionView{}, err
		}
		if size.Sign() == 0 {
			continue
		}
		return positionView{Has: true, Size: p.Size, EntryPrice: p.EntryPrice, MarkPrice: p.MarkPrice,
			Unrealised: p.UnrealisedPnl, Leverage: p.Leverage}, nil
	}
	return positionView{}, nil
}

// residue 账户里非零持仓数、未完结委托数、未触发条件单数
func residue(client *gateapi.APIClient, ctx context.Context) (positions, orders, priceOrders int, err error) {
	list, _, err := client.FuturesApi.ListPositions(ctx, "usdt", nil)
	if err != nil {
		return 0, 0, 0, err
	}
	for _, p := range list {
		size, err := rat(p.Size)
		if err != nil {
			return 0, 0, 0, err
		}
		if size.Sign() != 0 {
			positions++
		}
	}
	// SDK 里 status 是位置参数（open = 未完结 / 未触发）
	openOrders, _, err := client.FuturesApi.ListFuturesOrders(ctx, "usdt", "open", nil)
	if err != nil {
		return 0, 0, 0, err
	}
	orders = len(openOrders)
	triggers, _, err := client.FuturesApi.ListPriceTriggeredOrders(ctx, "usdt", "open", nil)
	if err != nil {
		return 0, 0, 0, err
	}
	priceOrders = len(triggers)
	return positions, orders, priceOrders, nil
}

// sample 一次带仓读数：适配器（新建实例，绕开 15 秒缓存）、原始接口、持仓条目
type sample struct {
	ElapsedMs int64              `json:"elapsed_ms"`
	Balance   map[string]float64 `json:"adapter_balance"`
	Raw       rawAccount         `json:"raw"`
	Position  positionView       `json:"position"`

	principal, unrealized, available, equity *big.Rat
	walletUpnl, futuresUpnl, positionUpnl    *big.Rat
}

func takeSample(key, secret string, client *gateapi.APIClient, ctx context.Context, started time.Time) (sample, error) {
	var s sample
	s.ElapsedMs = time.Since(started).Milliseconds()

	fresh := nofxgate.NewGateTrader(key, secret) // 新实例：不复用上一个采样的缓存
	balance, err := types.ReadAccountBalance(fresh)
	if err != nil {
		return s, fmt.Errorf("adapter balance: %w", err)
	}
	s.principal, s.unrealized, s.available, s.equity = ratFloat64(balance.Principal), ratFloat64(balance.UnrealizedPnL),
		ratFloat64(balance.Available), ratFloat64(balance.Equity)
	s.Balance = map[string]float64{"principal": balance.Principal, "unrealized_pnl": balance.UnrealizedPnL,
		"available": balance.Available, "equity": balance.Equity}

	if s.Raw, err = readRaw(client, ctx); err != nil {
		return s, err
	}
	if s.walletUpnl, err = rat(s.Raw.WalletUnrealised); err != nil {
		return s, err
	}
	if s.futuresUpnl, err = rat(s.Raw.FuturesUnrealised); err != nil {
		return s, err
	}
	if s.Position, err = readPosition(client, ctx); err != nil {
		return s, err
	}
	if s.positionUpnl, err = rat(s.Position.Unrealised); err != nil {
		return s, err
	}
	return s, nil
}

func absDiff(a, b *big.Rat) *big.Rat {
	return new(big.Rat).Abs(new(big.Rat).Sub(a, b))
}

func maxRat(values ...*big.Rat) *big.Rat {
	var m *big.Rat
	for _, v := range values {
		if m == nil || v.Cmp(m) > 0 {
			m = v
		}
	}
	return m
}

func minRat(values ...*big.Rat) *big.Rat {
	var m *big.Rat
	for _, v := range values {
		if m == nil || v.Cmp(m) < 0 {
			m = v
		}
	}
	return m
}

// sampleUpnls 各次采样时合约账户的未实现盈亏
func sampleUpnls(samples []sample) []*big.Rat {
	values := make([]*big.Rat, 0, len(samples))
	for _, s := range samples {
		values = append(values, s.futuresUpnl)
	}
	return values
}

func buildPromptPreview(equity, available, unrealized, initialBalance float64) (string, string) {
	engine := kernel.NewStrategyEngine(&store.StrategyConfig{
		Language:    "zh",
		CoinSource:  store.CoinSourceConfig{StaticCoins: []string{"ETHUSDT"}},
		RiskControl: store.RiskControlConfig{},
	})
	ctx := &kernel.Context{
		CurrentTime:    time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
		RuntimeMinutes: 0,
		CallCount:      1,
		Account: kernel.AccountInfo{
			TotalEquity: equity, AvailableBalance: available, UnrealizedPnL: unrealized,
			TotalPnL: equity - initialBalance,
		},
		Positions:      []kernel.PositionInfo{},
		CandidateCoins: []kernel.CandidateCoin{{Symbol: "ETHUSDT", Sources: []string{"static"}}},
	}
	if initialBalance > 0 {
		ctx.Account.TotalPnLPct = (equity - initialBalance) / initialBalance * 100
	}
	return engine.BuildSystemPrompt(equity, "balanced"), engine.BuildUserPrompt(ctx)
}

func main() {
	outPath := flag.String("out", "", "把 JSONL 证据写到文件（默认 stdout）")
	promptOut := flag.String("prompt-out", "", "把提示词预览写到文件")
	dryRun := flag.Bool("dry-run", false, "只跑只读步骤（预检、1、2、7），到开仓前为止")
	flag.Parse()

	if *outPath != "" {
		file, err := os.Create(*outPath)
		if err != nil {
			fmt.Println("无法创建证据文件:", err)
			os.Exit(2)
		}
		defer file.Close()
		out = file
	}

	key := os.Getenv("GATE_KEY")
	secret := os.Getenv("GATE_SECRET")
	if key == "" || secret == "" {
		emit(map[string]interface{}{"step": "credentials", "result": "FAIL", "detail": "GATE_KEY / GATE_SECRET 未设置"})
		os.Exit(2)
	}

	ctx := context.WithValue(context.Background(), gateapi.ContextGateAPIV4, gateapi.GateAPIV4{Key: key, Secret: secret})
	config := gateapi.NewConfiguration()
	config.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	client := gateapi.NewAPIClient(config)
	gt := nofxgate.NewGateTrader(key, secret)

	// ===== 预检：只读；不满足就不开仓 =====
	baselineSettings, err := readAllSettings(gt)
	if err != nil {
		emit(map[string]interface{}{"step": "preflight", "result": "FAIL", "detail": errText(err)})
		os.Exit(2)
	}
	positions, orders, priceOrders, err := residue(client, ctx)
	if err != nil {
		emit(map[string]interface{}{"step": "preflight", "result": "FAIL", "detail": errText(err)})
		os.Exit(2)
	}
	observe("preflight", map[string]interface{}{
		"settings": settingsView(baselineSettings), "positions": positions, "open_orders": orders, "open_price_orders": priceOrders,
	})
	if positions != 0 || orders != 0 || priceOrders != 0 {
		emit(map[string]interface{}{"step": "preflight", "result": "FAIL",
			"detail": fmt.Sprintf("account is not flat: positions=%d open_orders=%d open_price_orders=%d", positions, orders, priceOrders)})
		os.Exit(2)
	}
	ethSettings, hasETH := baselineSettings[target]
	if !hasETH || !ethSettings.Has {
		emit(map[string]interface{}{"step": "preflight", "result": "FAIL",
			"detail": target + " has no position settings entry: opening would create one that cannot be restored"})
		os.Exit(2)
	}

	// ===== 兜底还原：退出前若目标合约仍有仓位，市价全平 =====
	opened := false
	defer func() {
		if !opened {
			return
		}
		position, err := readPosition(client, ctx)
		if err == nil && position.Has {
			fmt.Println("[cleanup] 目标合约仍有仓位，市价全平")
			if _, err := gt.CloseLong(target, 0); err != nil {
				fmt.Println("[cleanup] 全平失败，请人工核对账户:", err)
			}
		}
	}()

	// ===== 1 不带仓读数 =====
	flat, err := types.ReadAccountBalance(gt)
	if err != nil {
		step("1 flat reading", false, map[string]interface{}{"error": errText(err)})
		finish()
		return
	}
	flatRaw, err := readRaw(client, ctx)
	if err != nil {
		step("1 flat reading", false, map[string]interface{}{"error": errText(err)})
		finish()
		return
	}
	amount, _ := rat(flatRaw.WalletAmount)
	borrowed, _ := rat(flatRaw.WalletBorrowed)
	walletUpnl, _ := rat(flatRaw.WalletUnrealised)
	futuresUpnl, _ := rat(flatRaw.FuturesUnrealised)
	futuresAvailable, _ := rat(flatRaw.FuturesAvailable)
	principal := new(big.Rat).Sub(amount, borrowed)

	checks := map[string]bool{
		"principal equals total.amount − total.borrowed": within(ratFloat64(flat.Principal), principal, "0.0000000001"),
		"unrealized pnl equals total.unrealised_pnl":     within(ratFloat64(flat.UnrealizedPnL), walletUpnl, "0.0000000001"),
		"available equals futures account available":     within(ratFloat64(flat.Available), futuresAvailable, "0.0000000001"),
		"equity equals principal + unrealized pnl":       within(ratFloat64(flat.Equity), new(big.Rat).Add(principal, walletUpnl), "0.0000000001"),
		"currency is USDT":                               flatRaw.WalletCurrency == "USDT",
		"equity is positive":                             flat.Equity > 0,
	}
	step("1 flat reading", allTrue(checks), map[string]interface{}{
		"adapter_balance": map[string]float64{"principal": flat.Principal, "unrealized_pnl": flat.UnrealizedPnL,
			"available": flat.Available, "equity": flat.Equity},
		"raw": flatRaw, "checks": checks,
	})
	// 合约账户的 total 在统一账户下恒为 0：这正是旧口径把权益读成 0 的原因
	observe("1 futures account total (not the principal)", map[string]interface{}{
		"futures_account_total": flatRaw.FuturesTotal, "principal": f(principal),
		"flat_abs(wallet unrealised − futures unrealised)": f(absDiff(walletUpnl, futuresUpnl)),
	})

	// ===== 2 初始资金重建（加载路径，不接数据库） =====
	at, err := trader.NewAutoTrader(trader.AutoTraderConfig{
		ID: "account-equity-probe", Name: "account-equity-probe", Exchange: "gate",
		GateAPIKey: key, GateSecretKey: secret, AIModel: "custom",
		InitialBalance: 0, // 无效值：加载时以当前本金重建
		StrategyConfig: &store.StrategyConfig{},
	}, nil, "probe")
	if err != nil {
		step("2 initial balance rebuilt on load", false, map[string]interface{}{"error": errText(err)})
		finish()
		return
	}
	rebuilt, _ := at.GetStatus()["initial_balance"].(float64)
	withinFlat := within(ratFloat64(rebuilt), ratFloat64(flat.Principal), "0.0000001")
	step("2 initial balance rebuilt on load", withinFlat, map[string]interface{}{
		"rebuilt_initial_balance": rebuilt, "flat_principal": flat.Principal,
	})
	initialBalance := rebuilt

	if *dryRun {
		observe("dry run", map[string]interface{}{"detail": "stopped before opening a position"})
		previewPrompt(flat.Equity, flat.Available, flat.UnrealizedPnL, initialBalance, *promptOut)
		finish()
		return
	}

	// ===== 3 W3：ETH_USDT 0.1 张随单保护开多 =====
	price, err := gt.GetMarketPrice(target)
	if err != nil {
		step("3 open 0.1 contracts", false, map[string]interface{}{"error": errText(err)})
		finish()
		return
	}
	spec, err := gt.GetContractSpec(target)
	if err != nil {
		step("3 open 0.1 contracts", false, map[string]interface{}{"error": errText(err)})
		finish()
		return
	}
	multiplier, _ := rat(spec.QuantoMultiplier)
	contracts, _ := rat(targetContracts)
	// 名义价值 = 目标张数（0.1 张）× 每张币数量 × 现价，再加 5% 裕度：取价与换算之间价格会动，
	// 压在一步的边界上会被拒（2026-11-24 子任务 4 首轮实测的教训）
	targetValue := new(big.Rat).Mul(new(big.Rat).Mul(ratFloat64(price), contracts), multiplier)
	notional := f(new(big.Rat).Mul(targetValue, big.NewRat(105, 100)))
	// 授权上限（D35）：单笔测试名义价值不超过 10 USDT，超出即不下单
	if notional > 10 {
		step("3 open 0.1 contracts", false, map[string]interface{}{
			"error": fmt.Sprintf("notional %.4f USDT exceeds the authorized cap of 10 USDT", notional), "price": price,
		})
		finish()
		return
	}
	// 下单前先用同一个换算确认得到 0.1 张
	quote, err := spec.SizeForNotional(price, notional, 0)
	if err != nil || quote.Contracts != targetContracts || !quote.Orderable() {
		detail := map[string]interface{}{"price": price, "notional": notional, "contracts": quote.Contracts}
		if err != nil {
			detail["error"] = errText(err)
		}
		step("3 open 0.1 contracts", false, detail)
		finish()
		return
	}
	opened = true // 从这里起若失败，兜底还原会全平
	result, err := gt.OpenLongProtected(types.ProtectedOpenRequest{
		Symbol: target, NotionalUSD: notional,
		Leverage: ethSettings.Leverage, IsCrossMargin: ethSettings.Cross, // 该合约现有的设置：不改动账户
		StopLoss: price * 0.9, TakeProfit: price * 1.1,
		MinNotional: 0, SlipRatio: 0.01,
	})
	if err != nil {
		step("3 open 0.1 contracts", false, map[string]interface{}{"error": errText(err), "price": price, "notional": notional})
		finish()
		return
	}
	step("3 open 0.1 contracts", result.Contracts == targetContracts, map[string]interface{}{
		"order_id": result.OrderID, "contracts": result.Contracts, "notional": result.Notional,
		"fill_price": result.FillPrice, "margin_settings_sent": fmt.Sprintf("cross=%v leverage=%d", ethSettings.Cross, ethSettings.Leverage),
	})
	if err := gt.VerifyOrderProtection(target, result.OrderID); err != nil {
		step("3 protection verified", false, map[string]interface{}{"error": errText(err)})
	} else {
		step("3 protection verified", true, map[string]interface{}{"order_id": result.OrderID})
	}

	// ===== 4 带仓读数：约 75 秒内 7 次 =====
	started := time.Now()
	samples := []sample{}
	readOK := true
	var readErr string
	for i := 0; i < 7; i++ {
		wait := 12 * time.Second
		if i == 0 {
			wait = 3 * time.Second
		}
		time.Sleep(wait)
		s, err := takeSample(key, secret, client, ctx, started)
		if err != nil {
			readOK, readErr = false, errText(err)
			break
		}
		samples = append(samples, s)
	}
	if readOK {
		checks := map[string]bool{"7 samples read": true}
		maxPrincipalSpread := big.NewRat(0, 1) // 持仓期间各次采样的本金之差
		maxEquityGap := big.NewRat(0, 1)
		maxAvailableGap := big.NewRat(0, 1)
		availableGaps := []map[string]interface{}{}
		for i, s := range samples {
			checks[fmt.Sprintf("sample %d: position is the 0.1-contract long", i+1)] = s.Position.Has && s.Position.Size == targetContracts
			maxPrincipalSpread = maxRat(maxPrincipalSpread, absDiff(s.principal, samples[0].principal))
			maxEquityGap = maxRat(maxEquityGap, absDiff(s.equity, new(big.Rat).Add(s.principal, s.unrealized)))
			// 可用额度的旁证：本金 + 未实现盈亏 − 头寸占用保证金。交易所内部怎样计算可用额度没有文档，
			// 残差只记录、不断言相等；量级校验用 0.01 USDT（约为本次名义价值的 0.6%）
			formula := new(big.Rat).Sub(new(big.Rat).Add(s.principal, s.futuresUpnl), ratOrZero(s.Raw.FuturesInitialMargin))
			gap := new(big.Rat).Sub(s.available, formula)
			maxAvailableGap = maxRat(maxAvailableGap, new(big.Rat).Abs(gap))
			availableGaps = append(availableGaps, map[string]interface{}{"sample": i + 1, "gap": f(gap),
				"order_margin": s.Raw.FuturesOrderMargin, "maintenance_margin": s.Raw.FuturesMaintenance})
		}
		upnlSpread := new(big.Rat).Sub(maxRat(sampleUpnls(samples)...), minRat(sampleUpnls(samples)...))
		// 本金不含浮动盈亏：持仓期间未实现盈亏来回变动，本金保持不变（各次采样之差为 0）。
		// 开仓那一刻本金下降了开仓手续费，所以不与开仓前的基线比
		checks["unrealized pnl varies across the samples (the position is live)"] = upnlSpread.Sign() > 0
		checks["principal is constant while the position is open (≤ 0.000000001)"] = maxPrincipalSpread.Cmp(big.NewRat(1, 1000000000)) <= 0
		checks["equity = principal + unrealized pnl in every sample"] = maxEquityGap.Cmp(big.NewRat(1, 1000000000)) <= 0
		checks["available is close to principal + unrealized pnl − position margin (≤ 0.01)"] = maxAvailableGap.Cmp(big.NewRat(1, 100)) <= 0
		views := make([]map[string]interface{}, 0, len(samples))
		for _, s := range samples {
			views = append(views, map[string]interface{}{
				"elapsed_ms": s.ElapsedMs, "adapter_balance": s.Balance, "raw": s.Raw,
				"position_unrealised_pnl": s.Position.Unrealised, "position_mark_price": s.Position.MarkPrice,
			})
		}
		step("4 readings with position", allTrue(checks), map[string]interface{}{
			"checks": checks, "max_principal_spread": f(maxPrincipalSpread), "unrealized_pnl_spread": f(upnlSpread),
			"max_available_formula_gap": f(maxAvailableGap), "available_formula_gaps": availableGaps, "samples": views,
		})

		// 开仓那一刻本金的降幅：手续费即时从本金里扣除（钱包总额接口官方说明的 1 分钟缓存没有体现在这里）
		stepAtOpen := new(big.Rat).Sub(samples[0].principal, ratFloat64(flat.Principal))
		openFee := new(big.Rat).Mul(ratFloat64(result.Notional), takerFee)
		observe("4 principal step at open", map[string]interface{}{
			"principal_before": flat.Principal, "principal_with_position": f(samples[0].principal),
			"change": f(stepAtOpen), "taker_fee_0.05pct_of_notional": f(openFee),
			"explained_by_open_fee":               within(stepAtOpen, new(big.Rat).Neg(openFee), "0.00001"),
			"futures_account_total_with_position": samples[0].Raw.FuturesTotal,
		})

		// 钱包总额接口官方说明数据最长缓存 1 分钟：对比三处未实现盈亏是否同步（观察，不计成败）
		maxWalletFutures := big.NewRat(0, 1)
		maxWalletPosition := big.NewRat(0, 1)
		maxFuturesMove := big.NewRat(0, 1)
		for i, s := range samples {
			maxWalletFutures = maxRat(maxWalletFutures, absDiff(s.walletUpnl, s.futuresUpnl))
			maxWalletPosition = maxRat(maxWalletPosition, absDiff(s.walletUpnl, s.positionUpnl))
			if i > 0 {
				maxFuturesMove = maxRat(maxFuturesMove, absDiff(s.futuresUpnl, samples[i-1].futuresUpnl))
			}
		}
		lags := []map[string]interface{}{}
		for i, s := range samples {
			if within(s.walletUpnl, s.futuresUpnl, "0.00000001") {
				continue
			}
			for j := 0; j < i; j++ {
				if within(s.walletUpnl, samples[j].futuresUpnl, "0.00000001") {
					lags = append(lags, map[string]interface{}{"sample": i + 1, "matches_futures_sample": j + 1,
						"lag_ms": samples[i].ElapsedMs - samples[j].ElapsedMs})
				}
			}
		}
		observe("4 unrealized pnl sync across sources", map[string]interface{}{
			"max_abs(wallet − futures)": f(maxWalletFutures), "max_abs(wallet − position)": f(maxWalletPosition),
			"max_futures_change_between_samples": f(maxFuturesMove), "wallet_value_matching_an_earlier_futures_sample": lags,
			"last_sample": map[string]interface{}{"wallet": samples[len(samples)-1].Raw.WalletUnrealised,
				"futures": samples[len(samples)-1].Raw.FuturesUnrealised, "position": samples[len(samples)-1].Position.Unrealised},
		})
	} else {
		step("4 readings with position", false, map[string]interface{}{"error": readErr, "samples_read": len(samples)})
	}

	// ===== 5 全平 =====
	closeFill := 0.0
	if closeResult, err := gt.CloseLong(target, 0); err != nil {
		step("5 close all", false, map[string]interface{}{"error": errText(err)})
	} else {
		closeFill, _ = closeResult["avgPrice"].(float64)
		step("5 close all", true, map[string]interface{}{"fill_price": closeFill})
	}
	time.Sleep(3 * time.Second)
	position, err := readPosition(client, ctx)
	leftPositions, leftOrders, leftTriggers, resErr := residue(client, ctx)
	clean := err == nil && resErr == nil && !position.Has && leftPositions == 0 && leftOrders == 0 && leftTriggers == 0
	detail := map[string]interface{}{"positions": leftPositions, "open_orders": leftOrders, "open_price_orders": leftTriggers}
	if err != nil || resErr != nil {
		detail["error"] = errText(err) + " " + errText(resErr)
	}
	step("5 no residue after close", clean, detail)
	if clean {
		opened = false
	}

	// ===== 6 终态 =====
	finalSettings, err := readAllSettings(gt)
	if err != nil {
		step("6 settings unchanged", false, map[string]interface{}{"error": errText(err)})
	} else {
		step("6 settings unchanged", sameSettings(baselineSettings, finalSettings),
			map[string]interface{}{"baseline": settingsView(baselineSettings), "final": settingsView(finalSettings)})
	}

	// 平仓后约 5 秒（已过 3 秒等待）读一次，再等到约 70 秒读一次：钱包总额接口官方说明数据最长缓存 1 分钟
	early, err := types.ReadAccountBalance(nofxgate.NewGateTrader(key, secret))
	if err != nil {
		step("6 flat reading after close", false, map[string]interface{}{"error": errText(err)})
	} else {
		time.Sleep(65 * time.Second)
		late, err := types.ReadAccountBalance(nofxgate.NewGateTrader(key, secret))
		if err != nil {
			step("6 flat reading after close", false, map[string]interface{}{"error": errText(err)})
		} else {
			change := late.Principal - flat.Principal
			checks := map[string]bool{
				"unrealized pnl back to 0":   late.UnrealizedPnL == 0,
				"available equals principal": within(ratFloat64(late.Available), ratFloat64(late.Principal), "0.0001"),
				"principal change is small (< 0.05 USDT: fees and realized pnl of one round trip)": change < 0.05 && change > -0.05,
			}
			step("6 flat reading after close", allTrue(checks), map[string]interface{}{
				"checks":           checks,
				"principal_before": flat.Principal, "principal_5s_after": early.Principal, "principal_70s_after": late.Principal,
				"principal_change": change, "available_70s_after": late.Available,
			})
			observe("6 principal settled between 5s and 70s", map[string]interface{}{
				"changed": early.Principal != late.Principal, "difference": late.Principal - early.Principal,
			})
			// 对账（观察）：本金的变化 = 已实现盈亏 − 开仓手续费 − 平仓手续费（taker 0.05%）。
			// 手续费率是按经验值代入的，只记录不断言
			if closeFill > 0 {
				quantity := new(big.Rat).Mul(contracts, multiplier)
				realized := new(big.Rat).Mul(quantity, new(big.Rat).Sub(ratFloat64(closeFill), ratFloat64(result.FillPrice)))
				openFee := new(big.Rat).Mul(ratFloat64(result.Notional), takerFee)
				closeFee := new(big.Rat).Mul(new(big.Rat).Mul(quantity, ratFloat64(closeFill)), takerFee)
				expected := new(big.Rat).Sub(realized, new(big.Rat).Add(openFee, closeFee))
				observe("6 principal change explained", map[string]interface{}{
					"observed_change": change, "realized_pnl": f(realized), "open_fee": f(openFee), "close_fee": f(closeFee),
					"expected_change": f(expected), "matches_within_0.00001": within(ratFloat64(change), expected, "0.00001"),
				})
			}
		}
	}

	// ===== 7 提示词预览 =====
	previewPrompt(flat.Equity, flat.Available, flat.UnrealizedPnL, initialBalance, *promptOut)

	finish()
}

// previewPrompt 第 7 步：用不带仓读数渲染提示词，核对账户段
func previewPrompt(equity, available, unrealized, initialBalance float64, promptOut string) {
	// 生产路径（kernel.GetFullDecisionWithStrategy）用这两个函数渲染提示词；用户提示词的账户行是英文
	systemPrompt, userPrompt := buildPromptPreview(equity, available, unrealized, initialBalance)
	if promptOut != "" {
		if err := os.WriteFile(promptOut, []byte("# System Prompt\n\n"+systemPrompt+"\n\n# User Prompt\n\n"+userPrompt), 0o644); err != nil {
			step("7 prompt preview", false, map[string]interface{}{"error": err.Error()})
			return
		}
	}
	accountLine := fmt.Sprintf("Account: Equity %.2f | Balance %.2f", equity, available)
	checks := map[string]bool{
		"user prompt shows the real equity and available": strings.Contains(userPrompt, accountLine),
		"user prompt shows no NaN":                        !strings.Contains(userPrompt, "NaN"),
		"system prompt sizes limits on equity":            strings.Contains(systemPrompt, fmt.Sprintf("权益 %.0f ×", equity)),
		"equity in the prompt is not zero":                equity > 0,
	}
	step("7 prompt preview", allTrue(checks), map[string]interface{}{
		"checks": checks, "account_line": accountLine,
	})
}

func ratOrZero(s string) *big.Rat {
	v, err := rat(s)
	if err != nil {
		return big.NewRat(0, 1)
	}
	return v
}

func allTrue(checks map[string]bool) bool {
	for _, ok := range checks {
		if !ok {
			return false
		}
	}
	return true
}

func finish() {
	if failures > 0 {
		emit(map[string]interface{}{"step": "summary", "result": "LIVECHECK_FAIL", "failures": failures})
		os.Exit(1)
	}
	emit(map[string]interface{}{"step": "summary", "result": "LIVECHECK_PASS", "failures": 0})
}
