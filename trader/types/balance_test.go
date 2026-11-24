package types

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func TestParseAccountBalance(t *testing.T) {
	base := func() map[string]interface{} {
		return map[string]interface{}{
			BalanceKeyPrincipal:     107.5,
			BalanceKeyUnrealizedPnL: 2.25,
			BalanceKeyAvailable:     80.0,
		}
	}

	tests := []struct {
		name       string
		mutate     func(m map[string]interface{})
		wantEquity float64
		wantErr    string // 为空表示不应报错；否则是错误信息里应包含的键名
	}{
		{name: "adapter without equity keys: equity is principal plus unrealized", mutate: func(map[string]interface{}) {},
			wantEquity: 109.75},
		{name: "totalEquity reported: used as is", mutate: func(m map[string]interface{}) { m["totalEquity"] = 111.0 },
			wantEquity: 111.0},
		{name: "total_equity reported: used as is", mutate: func(m map[string]interface{}) { m["total_equity"] = 112.0 },
			wantEquity: 112.0},
		{name: "both equity keys reported: totalEquity wins", mutate: func(m map[string]interface{}) {
			m["totalEquity"] = 111.0
			m["total_equity"] = 112.0
		}, wantEquity: 111.0},
		{name: "reported equity of zero falls back to the formula", mutate: func(m map[string]interface{}) { m["total_equity"] = 0.0 },
			wantEquity: 109.75},
		{name: "reported equity that is not a number is ignored", mutate: func(m map[string]interface{}) { m["totalEquity"] = "111" },
			wantEquity: 109.75},
		{name: "negative unrealized pnl", mutate: func(m map[string]interface{}) {
			m[BalanceKeyUnrealizedPnL] = -7.5
		}, wantEquity: 100.0},

		{name: "principal missing", mutate: func(m map[string]interface{}) { delete(m, BalanceKeyPrincipal) },
			wantErr: BalanceKeyPrincipal},
		{name: "unrealized pnl missing", mutate: func(m map[string]interface{}) { delete(m, BalanceKeyUnrealizedPnL) },
			wantErr: BalanceKeyUnrealizedPnL},
		{name: "available missing", mutate: func(m map[string]interface{}) { delete(m, BalanceKeyAvailable) },
			wantErr: BalanceKeyAvailable},
		{name: "principal is a string", mutate: func(m map[string]interface{}) { m[BalanceKeyPrincipal] = "107.5" },
			wantErr: BalanceKeyPrincipal},
		{name: "available is an int", mutate: func(m map[string]interface{}) { m[BalanceKeyAvailable] = 80 },
			wantErr: BalanceKeyAvailable},
		{name: "principal is NaN", mutate: func(m map[string]interface{}) {
			m[BalanceKeyPrincipal] = math.NaN()
		}, wantErr: BalanceKeyPrincipal},
		{name: "available is infinite", mutate: func(m map[string]interface{}) {
			m[BalanceKeyAvailable] = math.Inf(1)
		}, wantErr: BalanceKeyAvailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := base()
			tt.mutate(m)
			got, err := ParseAccountBalance(m)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("ParseAccountBalance expected an error naming %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error %q should name the field %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseAccountBalance unexpected error: %v", err)
			}
			if got.Equity != tt.wantEquity {
				t.Errorf("Equity = %v, want %v", got.Equity, tt.wantEquity)
			}
			if got.Available != 80.0 {
				t.Errorf("Available = %v, want 80", got.Available)
			}
		})
	}
}

func TestParseAccountBalanceNilMap(t *testing.T) {
	if _, err := ParseAccountBalance(nil); err == nil {
		t.Fatal("ParseAccountBalance(nil) expected an error")
	}
}

// balanceTraderStub 只实现 GetBalance 的适配器桩；其余方法不应被调用
type balanceTraderStub struct {
	Trader
	balance map[string]interface{}
	err     error
}

func (s *balanceTraderStub) GetBalance() (map[string]interface{}, error) {
	return s.balance, s.err
}

func TestReadAccountBalance(t *testing.T) {
	errExchange := errors.New("exchange down")
	tests := []struct {
		name    string
		stub    *balanceTraderStub
		wantErr string // 为空表示不应报错
	}{
		{name: "complete reading", stub: &balanceTraderStub{balance: map[string]interface{}{
			BalanceKeyPrincipal: 107.5, BalanceKeyUnrealizedPnL: 1.5, BalanceKeyAvailable: 90.0,
		}}},
		{name: "exchange request fails", stub: &balanceTraderStub{err: errExchange}, wantErr: "failed to get account balance"},
		{name: "reading is unreadable", stub: &balanceTraderStub{balance: map[string]interface{}{
			BalanceKeyAvailable: 90,
		}}, wantErr: "unreadable account balance"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			balance, err := ReadAccountBalance(tt.stub)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("ReadAccountBalance expected an error containing %q, got %+v", tt.wantErr, balance)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error %q should contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ReadAccountBalance unexpected error: %v", err)
			}
			if balance.Principal != 107.5 || balance.Equity != 109.0 || balance.Available != 90 {
				t.Errorf("balance = %+v, want principal 107.5, equity 109, available 90", balance)
			}
		})
	}
}

func TestReadAccountBalanceKeepsExchangeErrorInChain(t *testing.T) {
	exchangeErr := errors.New("exchange down")

	_, err := ReadAccountBalance(&balanceTraderStub{err: exchangeErr})

	if !errors.Is(err, exchangeErr) {
		t.Fatalf("the exchange error must stay in the error chain, got %v", err)
	}
}

func TestReadInitialBalanceIsPrincipal(t *testing.T) {
	// 桩报告的权益是 205：初始资金取本金 200
	principal, err := ReadInitialBalance(&balanceTraderStub{balance: map[string]interface{}{
		BalanceKeyPrincipal: 200.0, BalanceKeyUnrealizedPnL: 5.0, BalanceKeyAvailable: 150.0, "totalEquity": 205.0,
	}})
	if err != nil {
		t.Fatalf("ReadInitialBalance unexpected error: %v", err)
	}
	if principal != 200 {
		t.Errorf("ReadInitialBalance = %v, want the principal 200", principal)
	}

	if _, err := ReadInitialBalance(&balanceTraderStub{err: errors.New("exchange down")}); err == nil {
		t.Error("ReadInitialBalance expected an error when the exchange request fails")
	}
}

func TestAccountBalanceFieldsAndInitialBalance(t *testing.T) {
	balance, err := ParseAccountBalance(map[string]interface{}{
		BalanceKeyPrincipal:     107.49769421,
		BalanceKeyUnrealizedPnL: -0.31,
		BalanceKeyAvailable:     107.49769,
		"totalEquity":           999.0, // 适配器报告的权益不改变本金
	})
	if err != nil {
		t.Fatalf("ParseAccountBalance unexpected error: %v", err)
	}
	if balance.Principal != 107.49769421 {
		t.Errorf("Principal = %v, want 107.49769421", balance.Principal)
	}
	if balance.UnrealizedPnL != -0.31 {
		t.Errorf("UnrealizedPnL = %v, want -0.31", balance.UnrealizedPnL)
	}
	if balance.Equity != 999.0 {
		t.Errorf("Equity = %v, want 999", balance.Equity)
	}
	// 初始资金是本金，不是权益
	if got := balance.InitialBalance(); got != 107.49769421 {
		t.Errorf("InitialBalance() = %v, want the principal 107.49769421", got)
	}
}
