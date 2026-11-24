package gate

import (
	"net/http"
	"strings"
	"sync"
	"testing"

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
	log := &requestLog{}
	gt := newLocalGateTrader(t, func(w http.ResponseWriter, r *http.Request) {
		log.record(r)
		switch r.URL.Path {
		case "/api/v4/wallet/total_balance":
			writeJSON(w, walletTotalBody("107.49769421", "0", "-0.3125", "USDT"))
		case "/api/v4/futures/usdt/accounts":
			writeJSON(w, futuresAccountBody("0.000000005", "-0.3125", "107.18519"))
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
		t.Errorf("UnrealizedPnL = %v, want -0.3125", balance.UnrealizedPnL)
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
			if !strings.Contains(err.Error(), tt.label) {
				t.Errorf("exchange error %q is not in the error chain: %q", tt.label, err.Error())
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
