package gatemcp

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// 缓存命中：TTL 内不触发上游
func TestCacheEntryHitWithinTTL(t *testing.T) {
	calls := 0
	e := &cacheEntry[int]{
		name: "t",
		ttl:  time.Minute,
		fetch: func(ctx context.Context) (int, error) {
			calls++
			return 42, nil
		},
	}
	for i := 0; i < 3; i++ {
		v, _, err := e.get(context.Background())
		if err != nil || v != 42 {
			t.Fatalf("get %d: v=%d err=%v", i, v, err)
		}
	}
	if calls != 1 {
		t.Fatalf("upstream calls = %d, want 1", calls)
	}
}

// 缓存过期后重新拉取
func TestCacheEntryExpiry(t *testing.T) {
	calls := 0
	e := &cacheEntry[int]{
		name: "t",
		ttl:  time.Minute,
		fetch: func(ctx context.Context) (int, error) {
			calls++
			return calls, nil
		},
	}
	if v, _, _ := e.get(context.Background()); v != 1 {
		t.Fatalf("first v=%d", v)
	}
	// 手动把时间戳拨到过期
	e.mu.Lock()
	e.at = e.at.Add(-2 * time.Minute)
	e.mu.Unlock()
	if v, _, _ := e.get(context.Background()); v != 2 {
		t.Fatalf("second v=%d, want refetch=2", v)
	}
}

// 并发过期请求经 singleflight 合并为一次上游调用
func TestCacheEntrySingleflightMerge(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	e := &cacheEntry[int]{
		name: "t",
		ttl:  time.Minute,
		fetch: func(ctx context.Context) (int, error) {
			if calls.Add(1) == 1 {
				<-release // 拖住首次刷新，让并发请求排队
			}
			return 7, nil
		},
	}
	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, errs[i] = e.get(context.Background())
		}(i)
	}
	time.Sleep(100 * time.Millisecond) // 等并发请求到达
	close(release)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("upstream calls = %d, want 1 (singleflight merge)", got)
	}
}

// 刷新失败返回错误并标注上次成功时间；旧值不作为成功返回
func TestCacheEntryRefreshFailureAnnotates(t *testing.T) {
	first := true
	e := &cacheEntry[int]{
		name: "t",
		ttl:  time.Minute,
		fetch: func(ctx context.Context) (int, error) {
			if first {
				first = false
				return 1, nil
			}
			return 0, &UpstreamError{Category: ErrUpstream, Err: errors.New("boom")}
		},
	}
	if _, _, err := e.get(context.Background()); err != nil {
		t.Fatalf("first get: %v", err)
	}
	e.mu.Lock()
	e.at = e.at.Add(-2 * time.Minute)
	e.mu.Unlock()

	v, at, err := e.get(context.Background())
	if err == nil {
		t.Fatal("want error on failed refresh")
	}
	var ue *UpstreamError
	if !errors.As(err, &ue) {
		t.Fatalf("error should keep UpstreamError chain: %v", err)
	}
	if !strings.Contains(err.Error(), "上次成功刷新") {
		t.Fatalf("error should annotate last success time: %v", err)
	}
	if v != 0 || !at.IsZero() {
		t.Fatalf("stale value must not be returned as success: v=%d at=%v", v, at)
	}
}

// 情绪缓存按 coin 分键；同一 coin 共享条目
func TestSentimentCachePerCoin(t *testing.T) {
	calls := 0
	sc := newSentimentCache(func(coin string) *cacheEntry[Sentiment] {
		return &cacheEntry[Sentiment]{
			name: "s:" + coin,
			ttl:  time.Minute,
			fetch: func(ctx context.Context) (Sentiment, error) {
				calls++
				return Sentiment{Coin: coin}, nil
			},
		}
	})
	e1 := sc.entry("BTC")
	e2 := sc.entry("BTC")
	if e1 != e2 {
		t.Fatal("same coin should reuse entry")
	}
	e3 := sc.entry("ETH")
	if e1 == e3 {
		t.Fatal("different coin should have own entry")
	}
	_, _, err := e1.get(context.Background())
	if err != nil {
		t.Fatalf("get BTC: %v", err)
	}
	_, _, err = e2.get(context.Background())
	if err != nil {
		t.Fatalf("get BTC again: %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}
