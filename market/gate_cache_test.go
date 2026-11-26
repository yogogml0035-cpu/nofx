package market

// Gate 行情缓存与并发合并的单元测试。其余行情测试按「每次调用都打到本地 HTTP 服务」编写，
// 所以本测试包默认关闭缓存，只有缓存用例在用例内打开。

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// gateCacheDefaultOn 记录生产默认值（init 里先读再关），供「生产默认开启」的用例断言。
var gateCacheDefaultOn bool

func init() {
	// 其余行情测试按「每次调用都打到本地 HTTP 服务」编写，所以本测试包默认关闭缓存；
	// 只有缓存用例在用例内打开。生产默认开启这一事实由 TestGateCacheDefaultOn 钉住。
	gateCacheDefaultOn = gateCacheOn.Load()
	gateCacheOn.Store(false)
}

func TestGateCacheDefaultOn(t *testing.T) {
	if !gateCacheDefaultOn {
		t.Fatal("the Gate market cache must be enabled by default in production (init)")
	}
	if gateCacheOn.Load() {
		t.Fatal("this test package must keep the cache off so the other market tests hit their local servers")
	}
}

func withGateCache(t *testing.T) {
	t.Helper()
	restore := SetGateCacheEnabled(true)
	t.Cleanup(restore)
}

// 领先者先进入 fetch 并阻塞，其余请求必须在它返回前到达：它们拿到的只能是同一个进行中的结果，
// 而不是缓存。这样删掉「进行中请求的合并」后用例会失败。
func TestGateCachedDedupesConcurrentFetches(t *testing.T) {
	withGateCache(t)
	var calls atomic.Int32
	entered := make(chan struct{}, 8)
	release := make(chan struct{})
	fetch := func() (any, error) {
		calls.Add(1)
		entered <- struct{}{}
		<-release
		return "v", nil
	}
	leaderDone := make(chan struct{})
	go func() {
		defer close(leaderDone)
		if v, err := gateCached("k", time.Minute, fetch); err != nil || v != "v" {
			t.Errorf("leader: gateCached = %v, %v", v, err)
		}
	}()
	<-entered // 领先者已进入 fetch 并阻塞
	var wg sync.WaitGroup
	for i := 0; i < 7; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if v, err := gateCached("k", time.Minute, fetch); err != nil || v != "v" {
				t.Errorf("follower: gateCached = %v, %v", v, err)
			}
		}()
	}
	time.Sleep(200 * time.Millisecond)
	close(release)
	<-leaderDone
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
		{"10s", 30, 2 * time.Second},  // 下限 2 秒
		{"10s", 500, 2 * time.Second}, // 与 limit 无关
		{"1m", 200, 6 * time.Second},  // 引擎取数的形状：周期 / 10
		{"5m", 1500, 10 * time.Second},
		{"1h", 2000, 10 * time.Second}, // 上限 10 秒
		{"1w", 500, 2 * time.Second},   // 自然周边界不在纪元网格：key 不带时间桶，只合并并发
		{"30d", 500, 2 * time.Second},
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

func TestGateCacheRespectsMaxEntries(t *testing.T) {
	withGateCache(t)
	for i := 0; i < gateMaxEntries+25; i++ {
		if _, err := gateCached(fmt.Sprintf("k%d", i), time.Minute, func() (any, error) { return i, nil }); err != nil {
			t.Fatal(err)
		}
	}
	gateCacheMu.Lock()
	size := len(gateCache)
	gateCacheMu.Unlock()
	if size > gateMaxEntries {
		t.Fatalf("cache holds %d entries, want at most %d", size, gateMaxEntries)
	}
}

func TestClassifyGateErrorParsesResetTimestamp(t *testing.T) {
	reset := time.Unix(1_700_000_077, 0)
	header := http.Header{}
	header.Set("X-Gate-RateLimit-Reset-Timestamp", "1700000077")
	err := classifyGateError(http.StatusTooManyRequests, header, []byte(`{"label":"TOO_MANY_REQUESTS","message":"slow down"}`))
	var limitErr *GateRateLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("classifyGateError = %v, want a *GateRateLimitError", err)
	}
	if !limitErr.ResetAt.Equal(reset) {
		t.Fatalf("ResetAt = %v, want %v", limitErr.ResetAt, reset)
	}
	if !strings.Contains(err.Error(), "status 429") {
		t.Fatalf("error = %q, want the HTTP status in the message", err)
	}
	// 头缺失或不可解析时不 panic，按零值处理
	err = classifyGateError(http.StatusTooManyRequests, http.Header{}, []byte(`{}`))
	if !errors.Is(err, ErrGateRateLimited) {
		t.Fatalf("classifyGateError without header = %v, want ErrGateRateLimited", err)
	}
}

// 三个行情入口都走缓存：ticker、全量 tickers、合约指标（含 contract_stats）两轮只打一次上游。
func TestGateMarketEntriesUseCache(t *testing.T) {
	withGateCache(t)
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/futures/usdt/tickers":
			if r.URL.Query().Get("contract") != "" {
				fmt.Fprint(w, `[{"contract":"DOGE_USDT","last":"0.1","change_percentage":"1","volume_24h_quote":"1000","high_24h":"0.2","low_24h":"0.05"}]`)
				return
			}
			fmt.Fprint(w, `[{"contract":"DOGE_USDT","last":"0.1","change_percentage":"1","volume_24h_quote":"1000","high_24h":"0.2","low_24h":"0.05"}]`)
		case "/futures/usdt/contracts/DOGE_USDT":
			fmt.Fprint(w, `{"mark_price":"0.1","index_price":"0.1","funding_rate":"0.0001","funding_next_apply":1700000000}`)
		case "/futures/usdt/contract_stats":
			fmt.Fprint(w, `[{"time":1700000000,"open_interest_usd":"20000000","last_funding_rate":"0.0001"}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	previous := gateFuturesBaseURL
	gateFuturesBaseURL = server.URL
	t.Cleanup(func() { gateFuturesBaseURL = previous })

	for round := 0; round < 2; round++ {
		if _, err := GetGateTicker("DOGE_USDT"); err != nil {
			t.Fatalf("GetGateTicker: %v", err)
		}
		if _, err := ListGateTickers(); err != nil {
			t.Fatalf("ListGateTickers: %v", err)
		}
		if _, err := GetGateContractMetrics("DOGE_USDT"); err != nil {
			t.Fatalf("GetGateContractMetrics: %v", err)
		}
	}
	// ticker 1 + tickers 1 + contracts 1 + contract_stats 1；第二轮全部命中缓存
	if got := hits.Load(); got != 4 {
		t.Fatalf("upstream hits = %d, want 4 (every entry point cached across two rounds)", got)
	}
}

func TestGateKlineCacheKeyUsesCandleBucket(t *testing.T) {
	now := time.Unix(1_700_000_000, 0) // 1m 桶的边界
	gateNow = func() time.Time { return now }
	t.Cleanup(func() { gateNow = time.Now })
	a := gateKlineCacheKey("DOGE_USDT", "1m", 200)
	now = now.Add(30 * time.Second)
	b := gateKlineCacheKey("DOGE_USDT", "1m", 200)
	now = now.Add(31 * time.Second) // 跨过下一根 K 线
	c := gateKlineCacheKey("DOGE_USDT", "1m", 200)
	if a != b {
		t.Fatalf("same candle must share a key: %q vs %q", a, b)
	}
	if a == c {
		t.Fatalf("a new candle must get a new key (a request after the close must not read the pre-close snapshot): %q", a)
	}
	if week := gateKlineCacheKey("DOGE_USDT", "1w", 200); week != "klines|DOGE_USDT|1w|200" {
		t.Fatalf("1w key = %q, want no bucket (calendar week boundary)", week)
	}
}

// 收线后即使 TTL 还没到也必须重取：缓存 key 带当前 K 线的时间桶
func TestKlineCacheRefetchesWhenCandleRollsOver(t *testing.T) {
	withGateCache(t)
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"t":1699999800,"v":1,"o":"1","h":"2","l":"0.5","c":"1.5","sum":"10"}]`)
	}))
	t.Cleanup(server.Close)
	previousURL, previousNow := gateFuturesBaseURL, gateNow
	gateFuturesBaseURL = server.URL
	// 5m 周期，TTL 10 秒；桶边界在 1700000100。第一次取数在边界前 2 秒，第二次在边界后 2 秒——
	// 相隔只有 4 秒（TTL 未过期），但已经换了 K 线：必须重取，否则引擎会拿到收线前的快照
	now := time.Unix(1700000095, 0)
	gateNow = func() time.Time { return now }
	t.Cleanup(func() { gateFuturesBaseURL, gateNow = previousURL, previousNow })

	if _, err := GetGateKlines("DOGE_USDT", "5m", 500); err != nil {
		t.Fatal(err)
	}
	now = time.Unix(1700000098, 0) // 同一根 K 线内、TTL（首次取数起 10 秒）内 → 命中缓存
	if _, err := GetGateKlines("DOGE_USDT", "5m", 500); err != nil {
		t.Fatal(err)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("hits = %d, want 1 within the same candle and TTL", got)
	}
	now = time.Unix(1700000102, 0) // 跨过 K 线边界（距首次取数才 7 秒，TTL 未到）：必须重取
	if _, err := GetGateKlines("DOGE_USDT", "5m", 500); err != nil {
		t.Fatal(err)
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("hits = %d, want 2 after the candle rolled over", got)
	}
}

// TTL 过期后各入口重新取数（contract_stats 的 TTL 也不能写成常量 1 小时）
func TestGateMarketEntriesExpire(t *testing.T) {
	withGateCache(t)
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/futures/usdt/contracts/DOGE_USDT":
			fmt.Fprint(w, `{"mark_price":"0.1","index_price":"0.1","funding_rate":"0.0001","funding_next_apply":1700000000}`)
		case "/futures/usdt/contract_stats":
			fmt.Fprint(w, `[{"time":1700000000,"open_interest_usd":"20000000","last_funding_rate":"0.0001"}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	previousURL, previousNow := gateFuturesBaseURL, gateNow
	gateFuturesBaseURL = server.URL
	now := time.Unix(1_700_000_000, 0)
	gateNow = func() time.Time { return now }
	t.Cleanup(func() { gateFuturesBaseURL, gateNow = previousURL, previousNow })

	if _, err := GetGateContractMetrics("DOGE_USDT"); err != nil {
		t.Fatal(err)
	}
	before := hits.Load()
	now = now.Add(11 * time.Second) // 超过 10 秒 TTL
	if _, err := GetGateContractMetrics("DOGE_USDT"); err != nil {
		t.Fatal(err)
	}
	if got := hits.Load() - before; got != 2 {
		t.Fatalf("after the TTL the metrics request fetched %d times, want 2 (contract + contract_stats)", got)
	}
}
