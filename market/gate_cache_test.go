package market

// Gate 行情缓存与并发合并的单元测试。其余行情测试按「每次调用都打到本地 HTTP 服务」编写，
// 所以本测试包默认关闭缓存，只有缓存用例在用例内打开。

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func init() {
	gateCacheOn.Store(false)
}

func withGateCache(t *testing.T) {
	t.Helper()
	restore := SetGateCacheEnabled(true)
	t.Cleanup(restore)
}

func TestGateCachedDedupesConcurrentFetches(t *testing.T) {
	withGateCache(t)
	var calls atomic.Int32
	release := make(chan struct{})
	started := make(chan struct{}, 8)
	fetch := func() (any, error) {
		if calls.Add(1) == 1 {
			close(release)
		}
		started <- struct{}{}
		<-release
		return "v", nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, err := gateCached("k", time.Minute, fetch)
			if err != nil || value != "v" {
				t.Errorf("gateCached = %v, %v", value, err)
			}
		}()
	}
	wg.Wait()
	if got := calls.Load(); got != 1 {
		t.Fatalf("fetch ran %d times, want 1 (concurrent callers must share one upstream request)", got)
	}
}

func TestGateCachedServesWithinTTLAndRefetchesAfter(t *testing.T) {
	withGateCache(t)
	now := time.Unix(1_700_000_000, 0)
	gateNow = func() time.Time { return now }
	t.Cleanup(func() { gateNow = time.Now })

	var calls int
	fetch := func() (any, error) { calls++; return calls, nil }
	for i := 0; i < 3; i++ {
		if _, err := gateCached("k", 2*time.Second, fetch); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 inside the TTL", calls)
	}
	now = now.Add(3 * time.Second)
	if _, err := gateCached("k", 2*time.Second, fetch); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want a refetch after the TTL", calls)
	}
}

func TestGateCachedDoesNotCacheErrors(t *testing.T) {
	withGateCache(t)
	boom := errors.New("upstream down")
	var calls int
	fetch := func() (any, error) { calls++; return nil, boom }
	for i := 0; i < 2; i++ {
		if _, err := gateCached("k", time.Minute, fetch); !errors.Is(err, boom) {
			t.Fatalf("gateCached error = %v, want the upstream error", err)
		}
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2 (errors must not be cached)", calls)
	}
}

func TestGateKlineCacheTTL(t *testing.T) {
	for _, tc := range []struct {
		interval string
		limit    int
		want     time.Duration
	}{
		{"1m", 30, 2 * time.Second},   // 引擎取数：只合并收线时刻的并发请求
		{"5m", 100, 2 * time.Second},  // 边界
		{"10s", 500, 3 * time.Second}, // 下限 3 秒
		{"5m", 1500, 10 * time.Second},
		{"1m", 1500, 6 * time.Second},
		{"1h", 2000, 10 * time.Second}, // 上限 10 秒
	} {
		if got := gateKlineCacheTTL(tc.interval, tc.limit); got != tc.want {
			t.Errorf("gateKlineCacheTTL(%s, %d) = %v, want %v", tc.interval, tc.limit, got, tc.want)
		}
	}
}

func TestGetGateKlinesUsesCache(t *testing.T) {
	withGateCache(t)
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"t":1700000000,"v":1,"o":"1","h":"2","l":"0.5","c":"1.5","sum":"10"}]`)
	}))
	t.Cleanup(server.Close)
	previous := gateFuturesBaseURL
	gateFuturesBaseURL = server.URL
	t.Cleanup(func() { gateFuturesBaseURL = previous })

	for i := 0; i < 3; i++ {
		bars, err := GetGateKlines("DOGE_USDT", "5m", 500)
		if err != nil || len(bars) != 1 {
			t.Fatalf("GetGateKlines = %v, %v", bars, err)
		}
		if bars[0].QuoteVolume != bars[0].Volume {
			t.Fatalf("QuoteVolume = %v, want the USDT quote volume %v", bars[0].QuoteVolume, bars[0].Volume)
		}
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("upstream hits = %d, want 1 (the 5m/500 request is cached for 10s)", got)
	}
}

func TestGetGateKlinesMapsRateLimitAndUnknownContract(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		body    string
		wantErr error
	}{
		{"rate limited", http.StatusTooManyRequests, `{"label":"TOO_MANY_REQUESTS","message":"too many requests"}`, ErrGateRateLimited},
		{"unknown contract", http.StatusBadRequest, `{"label":"CONTRACT_NOT_FOUND","message":"not found"}`, ErrGateContractNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			t.Cleanup(server.Close)
			previous := gateFuturesBaseURL
			gateFuturesBaseURL = server.URL
			t.Cleanup(func() { gateFuturesBaseURL = previous })

			_, err := GetGateKlines("NOPE_USDT", "5m", 30)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("GetGateKlines error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestGateRateLimitErrorRetryAfter(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	if got := (&GateRateLimitError{}).RetryAfter(now); got != time.Second {
		t.Fatalf("RetryAfter without reset = %v, want 1s", got)
	}
	reset := now.Add(7 * time.Second)
	if got := (&GateRateLimitError{ResetAt: reset}).RetryAfter(now); got != 7*time.Second {
		t.Fatalf("RetryAfter = %v, want 7s", got)
	}
	if got := (&GateRateLimitError{ResetAt: now.Add(-time.Hour)}).RetryAfter(now); got != time.Second {
		t.Fatalf("RetryAfter in the past = %v, want the 1s floor", got)
	}
}
