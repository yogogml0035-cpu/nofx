package gate

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gate/gateapi-go/v7"
)

// funding_rate 映射：Gate 原始小数语义（不 ×100）
func TestSymbolInfoFundingRate(t *testing.T) {
	contract := gateapi.Contract{Name: "BTC_USDT", LeverageMax: "100", OrderSizeMin: "1"}
	ticker := gateapi.FuturesTicker{Contract: "BTC_USDT", Last: "65000", FundingRate: "0.000057"}

	info, ok := SymbolInfoFromContract(contract, ticker, true)
	if !ok {
		t.Fatal("contract should be included")
	}
	if info.FundingRate != 0.000057 {
		t.Fatalf("funding_rate = %v, want 0.000057 (raw decimal)", info.FundingRate)
	}

	// 费率恰为 0 的合约：字段仍存在（不能 omitempty）
	info, _ = SymbolInfoFromContract(contract, gateapi.FuturesTicker{Contract: "BTC_USDT", FundingRate: "0"}, true)
	b, _ := json.Marshal(info)
	if !strings.Contains(string(b), `"funding_rate":0`) {
		t.Fatalf("funding_rate=0 must stay in JSON: %s", b)
	}

	// 无 ticker 时为零值且字段存在
	info, _ = SymbolInfoFromContract(contract, gateapi.FuturesTicker{}, false)
	b, _ = json.Marshal(info)
	if !strings.Contains(string(b), `"funding_rate":0`) {
		t.Fatalf("funding_rate field must always be present: %s", b)
	}
}

// ListSymbols TTL 缓存：TTL 内命中不拉上游；过期后重新拉取
func TestListSymbolsCache(t *testing.T) {
	calls := 0
	orig := listSymbolsUpstream
	defer func() { listSymbolsUpstream = orig }()
	listSymbolsUpstream = func(ctx context.Context) ([]SymbolInfo, error) {
		calls++
		return []SymbolInfo{{Symbol: "BTC_USDT", FundingRate: 0.0001}}, nil
	}

	// 重置缓存状态（进程级变量，测试间隔离）
	symbolsMu.Lock()
	symbolsCache, symbolsCacheAt, symbolsCacheOK = nil, time.Time{}, false
	symbolsMu.Unlock()

	s1, err := ListSymbols(context.Background())
	if err != nil || len(s1) != 1 {
		t.Fatalf("first: %v err=%v", s1, err)
	}
	s2, err := ListSymbols(context.Background())
	if err != nil || len(s2) != 1 {
		t.Fatalf("second: %v err=%v", s2, err)
	}
	if calls != 1 {
		t.Fatalf("upstream calls = %d, want 1 (cache hit)", calls)
	}

	// 拨到过期 → 重新拉取
	symbolsMu.Lock()
	symbolsCacheAt = symbolsCacheAt.Add(-symbolsCacheTTL - time.Second)
	symbolsMu.Unlock()
	if _, err := ListSymbols(context.Background()); err != nil {
		t.Fatalf("after expiry: %v", err)
	}
	if calls != 2 {
		t.Fatalf("upstream calls = %d, want 2 (expired refetch)", calls)
	}
}
