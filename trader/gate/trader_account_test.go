package gate

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/gate/gateapi-go/v7"

	"nofx/trader/types"
)

// 本文件的测试服务按 Gate 线上格式回复（2026-11-24 实测的响应形态，见任务 research G 节）。

// requestLog 记录测试服务收到的请求
type requestLog struct {
	mu    sync.Mutex
	paths []string
}

func (l *requestLog) record(r *http.Request) {
	l.mu.Lock()
	l.paths = append(l.paths, r.URL.RequestURI())
	l.mu.Unlock()
}

func (l *requestLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.paths...)
}

// walletTotalBody 钱包总额接口 GET /wallet/total_balance?currency=USDT 的线上格式
func walletTotalBody(amount, borrowed, unrealisedPnl, currency string) map[string]interface{} {
	total := map[string]interface{}{}
	if amount != "" {
		total["amount"] = amount
	}
	if borrowed != "" {
		total["borrowed"] = borrowed
	}
	if unrealisedPnl != "" {
		total["unrealised_pnl"] = unrealisedPnl
	}
	if currency != "" {
		total["currency"] = currency
	}
	return map[string]interface{}{
		"total": total,
		"details": map[string]interface{}{
			"spot":    map[string]interface{}{"currency": "USDT", "amount": amount, "borrowed": borrowed},
			"futures": map[string]interface{}{"currency": "USDT", "amount": "0", "unrealised_pnl": unrealisedPnl},
		},
	}
}

// futuresAccountBody 合约账户接口 GET /futures/usdt/accounts 的线上格式（节选）。
// 统一账户下 total ≈ 0，不能被当作钱包余额
func futuresAccountBody(total, unrealisedPnl, available string) map[string]interface{} {
	return map[string]interface{}{
		"user":                    533333,
		"currency":                "USDT",
		"total":                   total,
		"unrealised_pnl":          unrealisedPnl,
		"available":               available,
		"enable_credit":           true,
		"margin_mode":             1,
		"position_mode":           "single",
		"position_initial_margin": "0.3124",
	}
}

func TestGetBalanceReadsUnifiedAccount(t *testing.T) {
	// 两个接口的未实现盈亏故意取不同的值：钉住「未实现盈亏取自钱包总额」这个来源
	log := &requestLog{}
	gt := newLocalGateTrader(t, func(w http.ResponseWriter, r *http.Request) {
		log.record(r)
		switch r.URL.Path {
		case "/api/v4/wallet/total_balance":
			writeJSON(w, walletTotalBody("107.49769421", "0", "-0.3125", "USDT"))
		case "/api/v4/futures/usdt/accounts":
			writeJSON(w, futuresAccountBody("0.000000005", "-0.2", "107.18519"))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.RequestURI())
			w.WriteHeader(http.StatusNotFound)
		}
	})

	raw, err := gt.GetBalance()
	if err != nil {
		t.Fatalf("GetBalance unexpected error: %v", err)
	}

	balance, err := types.ParseAccountBalance(raw)
	if err != nil {
		t.Fatalf("GetBalance result is not parseable: %v", err)
	}
	if balance.Principal != 107.49769421 {
		t.Errorf("Principal = %v, want 107.49769421 (wallet total.amount, not the futures account total)", balance.Principal)
	}
	if balance.UnrealizedPnL != -0.3125 {
		t.Errorf("UnrealizedPnL = %v, want -0.3125 (wallet total.unrealised_pnl, not the futures account unrealised_pnl)", balance.UnrealizedPnL)
	}
	if balance.Available != 107.18519 {
		t.Errorf("Available = %v, want 107.18519 (futures account available)", balance.Available)
	}
	if want := balance.Principal + balance.UnrealizedPnL; balance.Equity != want {
		t.Errorf("Equity = %v, want %v (the adapter does not report equity itself)", balance.Equity, want)
	}
	if len(raw) != 3 {
		t.Errorf("GetBalance returned %d keys (%v), want exactly the three canonical keys", len(raw), raw)
	}
	wantRequests := []string{
		"/api/v4/wallet/total_balance?currency=USDT",
		"/api/v4/futures/usdt/accounts",
	}
	if got := log.snapshot(); strings.Join(got, ",") != strings.Join(wantRequests, ",") {
		t.Errorf("requests = %v, want %v", got, wantRequests)
	}
}

func TestGetBalancePrincipalIsExactDecimal(t *testing.T) {
	// 1000.3 − 0.1：float64 直接相减得 1000.1999999999999，精确十进制相减才是 1000.2
	gt := newLocalGateTrader(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/wallet/total_balance":
			writeJSON(w, walletTotalBody("1000.3", "0.1", "", "USDT"))
		case "/api/v4/futures/usdt/accounts":
			writeJSON(w, futuresAccountBody("0", "", "880.2"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	raw, err := gt.GetBalance()
	if err != nil {
		t.Fatalf("GetBalance unexpected error: %v", err)
	}
	if got := raw[types.BalanceKeyPrincipal]; got != 1000.2 {
		t.Errorf("principal = %v, want 1000.2 (amount − borrowed as exact decimals)", got)
	}
	if got := raw[types.BalanceKeyUnrealizedPnL]; got != 0.0 {
		t.Errorf("unrealized pnl = %v, want 0 when the field is omitted", got)
	}
}

func TestGetBalanceWithoutOptionalFields(t *testing.T) {
	// 没有借款、没有浮盈亏的账户：borrowed 与 unrealised_pnl 可能不返回，按 0 计
	gt := newLocalGateTrader(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/wallet/total_balance":
			writeJSON(w, walletTotalBody("107.49769421", "", "", ""))
		case "/api/v4/futures/usdt/accounts":
			writeJSON(w, futuresAccountBody("0", "0", "107.49769"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	raw, err := gt.GetBalance()
	if err != nil {
		t.Fatalf("GetBalance unexpected error: %v", err)
	}
	balance, err := types.ParseAccountBalance(raw)
	if err != nil {
		t.Fatalf("GetBalance result is not parseable: %v", err)
	}
	if balance.Principal != 107.49769421 || balance.UnrealizedPnL != 0 || balance.Equity != 107.49769421 {
		t.Errorf("balance = %+v, want principal 107.49769421, no unrealized pnl, equity = principal", balance)
	}
}

func TestGetBalanceRejectsUnreadableAccountData(t *testing.T) {
	tests := []struct {
		name   string
		wallet map[string]interface{}
		// account 为 nil 时用默认的合约账户返回
		account func() map[string]interface{}
		wantErr string
	}{
		{name: "total.amount missing", wallet: walletTotalBody("", "", "", "USDT"), wantErr: "total.amount"},
		{name: "total.amount malformed", wallet: walletTotalBody("abc", "", "", "USDT"), wantErr: "total.amount"},
		{name: "total.amount in scientific notation", wallet: walletTotalBody("1.07e2", "", "", "USDT"), wantErr: "total.amount"},
		{name: "total.borrowed malformed", wallet: walletTotalBody("107.5", "lots", "", "USDT"), wantErr: "total.borrowed"},
		{name: "total.unrealised_pnl malformed", wallet: walletTotalBody("107.5", "0", "-", "USDT"), wantErr: "total.unrealised_pnl"},
		{name: "currency is not USDT", wallet: walletTotalBody("107.5", "0", "", "USD"), wantErr: `currency is "USD"`},
		{name: "wallet total missing entirely", wallet: map[string]interface{}{"details": map[string]interface{}{}}, wantErr: "total.amount"},
		{
			name:    "available malformed",
			wallet:  walletTotalBody("107.5", "0", "", "USDT"),
			account: func() map[string]interface{} { return futuresAccountBody("0", "0", "n/a") },
			wantErr: "available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gt := newLocalGateTrader(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v4/wallet/total_balance":
					writeJSON(w, tt.wallet)
				case "/api/v4/futures/usdt/accounts":
					account := futuresAccountBody("0", "0", "107.49769")
					if tt.account != nil {
						account = tt.account()
					}
					writeJSON(w, account)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			})

			raw, err := gt.GetBalance()
			if err == nil {
				t.Fatalf("GetBalance expected an error mentioning %q, got result %v", tt.wantErr, raw)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q should mention %q", err.Error(), tt.wantErr)
			}
			if raw != nil {
				t.Errorf("GetBalance returned a result (%v) together with the error", raw)
			}
			// 失败的读数不得进入缓存
			gt.balanceCacheMutex.RLock()
			cached := gt.cachedBalance
			gt.balanceCacheMutex.RUnlock()
			if cached != nil {
				t.Errorf("a failed reading was cached: %v", cached)
			}
		})
	}
}

func TestGetBalancePropagatesRequestFailures(t *testing.T) {
	// Gate 的错误响应是 JSON；内容类型要先于状态码写入，SDK 才会把 label 解析成 GateAPIError
	writeFailure := func(w http.ResponseWriter, label string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"label":"` + label + `"}`))
	}

	tests := []struct {
		name        string
		failPath    string
		label       string
		wantInChain string
	}{
		{name: "wallet total balance fails", failPath: "/api/v4/wallet/total_balance", label: "INTERNAL", wantInChain: "wallet total balance"},
		{name: "futures account fails", failPath: "/api/v4/futures/usdt/accounts", label: "INVALID_PARAM_VALUE", wantInChain: "futures account"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gt := newLocalGateTrader(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == tt.failPath {
					writeFailure(w, tt.label)
					return
				}
				switch r.URL.Path {
				case "/api/v4/wallet/total_balance":
					writeJSON(w, walletTotalBody("107.5", "0", "", "USDT"))
				case "/api/v4/futures/usdt/accounts":
					writeJSON(w, futuresAccountBody("0", "0", "107.49769"))
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			})

			raw, err := gt.GetBalance()
			if err == nil {
				t.Fatalf("GetBalance expected an error, got result %v", raw)
			}
			if !strings.Contains(err.Error(), tt.wantInChain) {
				t.Errorf("error %q should mention %q", err.Error(), tt.wantInChain)
			}
			// 交易所的带 label 的错误必须留在错误链里：上层要能按类型取出来
			var gateErr gateapi.GateAPIError
			if !errors.As(err, &gateErr) {
				t.Fatalf("the GateAPIError must stay in the error chain, got %q", err.Error())
			}
			if gateErr.Label != tt.label {
				t.Errorf("exchange error label = %q, want %q", gateErr.Label, tt.label)
			}
		})
	}
}

func TestGetBalanceCachesWithinTTL(t *testing.T) {
	log := &requestLog{}
	gt := newLocalGateTrader(t, func(w http.ResponseWriter, r *http.Request) {
		log.record(r)
		switch r.URL.Path {
		case "/api/v4/wallet/total_balance":
			writeJSON(w, walletTotalBody("107.49769421", "0", "", "USDT"))
		case "/api/v4/futures/usdt/accounts":
			writeJSON(w, futuresAccountBody("0", "0", "107.49769"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	first, err := gt.GetBalance()
	if err != nil {
		t.Fatalf("first GetBalance unexpected error: %v", err)
	}
	second, err := gt.GetBalance()
	if err != nil {
		t.Fatalf("second GetBalance unexpected error: %v", err)
	}
	if got := len(log.snapshot()); got != 2 {
		t.Errorf("requests after two calls = %d, want 2 (the second call is served from the cache)", got)
	}
	if first[types.BalanceKeyPrincipal] != second[types.BalanceKeyPrincipal] {
		t.Errorf("cached principal %v differs from the first reading %v", second[types.BalanceKeyPrincipal], first[types.BalanceKeyPrincipal])
	}

	gt.clearCache()
	if _, err := gt.GetBalance(); err != nil {
		t.Fatalf("GetBalance after clearCache unexpected error: %v", err)
	}
	if got := len(log.snapshot()); got != 4 {
		t.Errorf("requests after clearCache = %d, want 4", got)
	}
}

// TestGetPositionsLeverageReadsActualValue 子任务 6：持仓列表返回杠杆的实际值（F12 修复）。
// lever 是文档钦定的权威字段（逐仓和全仓都适用）；缺失时按同模式旧字段兜底；都读不出按 0
// （不可用）计，不让整条查询失败。map 里的类型是 float64（trader 包的消费方都按 float64 断言）。
func TestGetPositionsLeverageReadsActualValue(t *testing.T) {
	cases := []struct {
		name        string
		lever       string
		legacyCross string
		legacyIso   string
		posMode     string
		want        float64
	}{
		{name: "lever 优先（全仓，与 cross_leverage_limit 一致）", lever: "25", legacyCross: "25", legacyIso: "0", posMode: "cross", want: 25},
		{name: "lever 与旧字段不一致时取 lever", lever: "30", legacyCross: "75", legacyIso: "0", posMode: "cross", want: 30},
		{name: "全仓旧字段兜底（无 lever）", legacyCross: "75", legacyIso: "0", posMode: "cross", want: 75},
		{name: "逐仓旧字段兜底（无 lever）", legacyCross: "0", legacyIso: "10", posMode: "isolated", want: 10},
		{name: "lever 异常值回落同模式旧字段", lever: "abc", legacyCross: "0", legacyIso: "10", posMode: "isolated", want: 10},
		{name: "字段全为 0 按 0（不可用）计", lever: "", legacyCross: "0", legacyIso: "0", posMode: "cross", want: 0},
		{name: "模式与杠杆字段全缺按 0（不可用）计", lever: "", legacyCross: "", legacyIso: "", posMode: "", want: 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			entry := map[string]interface{}{
				"contract":        "DOGE_USDT",
				"size":            "17",
				"mode":            "single",
				"entry_price":     "0.1",
				"mark_price":      "0.2",
				"liq_price":       "0.01",
				"unrealised_pnl":  "0.5",
				"pos_margin_mode": c.posMode,
			}
			if c.lever != "" {
				entry["lever"] = c.lever
			}
			if c.legacyCross != "" {
				entry["cross_leverage_limit"] = c.legacyCross
			}
			if c.legacyIso != "" {
				entry["leverage"] = c.legacyIso
			}
			mock := newGateMock()
			mock.positions = []map[string]interface{}{entry}
			gt := newLocalGateTrader(t, mock.handler)

			positions, err := gt.GetPositions()
			if err != nil {
				t.Fatalf("GetPositions unexpected error: %v", err)
			}
			if len(positions) != 1 {
				t.Fatalf("positions = %d, want 1", len(positions))
			}
			got, ok := positions[0]["leverage"].(float64)
			if !ok {
				t.Fatalf("leverage is %T, want float64", positions[0]["leverage"])
			}
			if got != c.want {
				t.Errorf("leverage = %v, want %v", got, c.want)
			}
		})
	}
}
