package market

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestToGateContractSymbol(t *testing.T) {
	cases := map[string]string{
		"BTCUSDT":   "BTC_USDT",
		"ETHUSDT":   "ETH_USDT",
		"BTC_USDT":  "BTC_USDT",
		"btcusdt":   "BTC_USDT",
		"BTC-USDT":  "BTC_USDT",
		"HYPEUSDT":  "HYPE_USDT",
		"XAUUSDT":   "XAU_USDT",
		"BTC-USDTSWAP": "BTC_USDT",
	}
	for in, want := range cases {
		if got := toGateContractSymbol(in); got != want {
			t.Errorf("toGateContractSymbol(%q) = %q, want %q", in, got, want)
		}
	}
}

// 单测只覆盖解析与聚合逻辑；数据真实性由真实 Gate 接口验证。
func TestGetGateOpenInterestAndFunding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/futures/usdt/contract_stats" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("contract") != "BTC_USDT" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `[
			{"time":100,"open_interest":"1000","open_interest_usd":"3000000000","last_funding_rate":"0.0001"},
			{"time":200,"open_interest":"1010","open_interest_usd":"3300000000","last_funding_rate":"0.0002"},
			{"time":300,"open_interest":"1020","open_interest_usd":"3600000000","last_funding_rate":"0.0003"}
		]`)
	}))
	defer srv.Close()

	orig := gateFuturesBaseURL
	gateFuturesBaseURL = srv.URL
	defer func() { gateFuturesBaseURL = orig }()

	oi, fr, err := getGateOpenInterestAndFunding("BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if oi == nil || fr == nil {
		t.Fatalf("expected non-nil OI and funding rate")
	}
	// 最后一期为最新
	if oi.Latest != 3_600_000_000 {
		t.Errorf("Latest = %v, want 3600000000", oi.Latest)
	}
	// 均值 = (30+33+36)亿 / 3
	if oi.Average != 3_300_000_000 {
		t.Errorf("Average = %v, want 3300000000", oi.Average)
	}
	if *fr != 0.0003 {
		t.Errorf("funding rate = %v, want 0.0003", *fr)
	}
}

func TestGetGateOpenInterestAndFundingEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()

	orig := gateFuturesBaseURL
	gateFuturesBaseURL = srv.URL
	defer func() { gateFuturesBaseURL = orig }()

	oi, fr, err := getGateOpenInterestAndFunding("BTCUSDT")
	if err == nil || oi != nil || fr != nil {
		t.Fatalf("expected error with nil results, got oi=%v fr=%v err=%v", oi, fr, err)
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error should mention empty, got: %v", err)
	}
}

func TestGetGateOpenInterestAndFundingInvalidUSD(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"time":100,"open_interest":"1000","open_interest_usd":"","last_funding_rate":"0.0001"}]`)
	}))
	defer srv.Close()

	orig := gateFuturesBaseURL
	gateFuturesBaseURL = srv.URL
	defer func() { gateFuturesBaseURL = orig }()

	_, _, err := getGateOpenInterestAndFunding("BTCUSDT")
	if err == nil {
		t.Fatal("expected error for empty open_interest_usd")
	}
}
