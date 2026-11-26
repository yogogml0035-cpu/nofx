package okx

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"nofx/trader/types"
)

// accountBalanceTransport 只回应账户余额接口，返回 OKX 线上格式（totalEq 是含未实现盈亏的权益）
type accountBalanceTransport struct{}

func (accountBalanceTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Path != okxAccountPath {
		return nil, fmt.Errorf("unexpected request %s %s", req.Method, req.URL.Path)
	}
	body := `{"code":"0","msg":"","data":[{"totalEq":"1500","details":[{"ccy":"USDT","eq":"1500","cashBal":"1400","availBal":"1300","upl":"100"}]}]}`
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}

func TestGetBalanceReportsEquitySeparately(t *testing.T) {
	// 适配器把 totalEq（权益）放在 totalWalletBalance 里是既有行为；权益要单独报告，
	// 否则统一的权益口径会把它算成 totalEq + 未实现盈亏（多算一次）
	trader := newTestOKXTrader(accountBalanceTransport{}, false)

	raw, err := trader.GetBalance()
	if err != nil {
		t.Fatalf("GetBalance unexpected error: %v", err)
	}

	balance, err := types.ParseAccountBalance(raw)
	if err != nil {
		t.Fatalf("GetBalance result is not parseable: %v", err)
	}
	if balance.Equity != 1500 {
		t.Errorf("Equity = %v, want 1500 (the reported totalEq, not totalEq + unrealized pnl)", balance.Equity)
	}
	if balance.UnrealizedPnL != 100 {
		t.Errorf("UnrealizedPnL = %v, want 100", balance.UnrealizedPnL)
	}
	if balance.Available != 1300 {
		t.Errorf("Available = %v, want 1300", balance.Available)
	}
}
