package gatemcp

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// cacheEntry 单值 TTL 缓存：过期后首个请求发起同步刷新，并发请求经
// singleflight 合并为一次上游调用（CI 带 -race，不手写 chan 合并）。
// 刷新失败对等待者返回错误；旧值仅用于错误信息标注上次成功时间，不作为成功返回。
type cacheEntry[T any] struct {
	name  string
	ttl   time.Duration
	fetch func(ctx context.Context) (T, error)

	mu    sync.Mutex
	val   T
	at    time.Time
	valid bool
	group singleflight.Group
}

type cacheResult[T any] struct {
	val T
	at  time.Time
}

func (e *cacheEntry[T]) get(ctx context.Context) (T, time.Time, error) {
	e.mu.Lock()
	if e.valid && time.Since(e.at) < e.ttl {
		v, at := e.val, e.at
		e.mu.Unlock()
		return v, at, nil
	}
	e.mu.Unlock()

	res, err, _ := e.group.Do("refresh", func() (any, error) {
		// 等待 singleflight 期间可能已被并发请求刷新
		e.mu.Lock()
		if e.valid && time.Since(e.at) < e.ttl {
			r := cacheResult[T]{val: e.val, at: e.at}
			e.mu.Unlock()
			return r, nil
		}
		e.mu.Unlock()

		v, ferr := e.fetch(ctx)
		if ferr != nil {
			return nil, e.annotate(ferr)
		}
		now := time.Now()
		e.mu.Lock()
		e.val, e.at, e.valid = v, now, true
		e.mu.Unlock()
		return cacheResult[T]{val: v, at: now}, nil
	})
	if err != nil {
		var zero T
		return zero, time.Time{}, err
	}
	r := res.(cacheResult[T])
	return r.val, r.at, nil
}

// annotate 附注上次成功刷新时间（%w 保持 UpstreamError 类别可被 errors.As 识别）
func (e *cacheEntry[T]) annotate(err error) error {
	e.mu.Lock()
	valid, at := e.valid, e.at
	e.mu.Unlock()
	if valid {
		return fmt.Errorf("%w（缓存中已有过期数据：上次成功刷新 %s）", err, at.Format(time.RFC3339))
	}
	return err
}

// sentimentCache 按 coin 分键的情绪缓存；条目数有上限防内存无界
type sentimentCache struct {
	mu       sync.Mutex
	byCoin   map[string]*cacheEntry[Sentiment]
	maxN     int
	newEntry func(coin string) *cacheEntry[Sentiment]
}

func newSentimentCache(newEntry func(coin string) *cacheEntry[Sentiment]) *sentimentCache {
	return &sentimentCache{
		byCoin:   make(map[string]*cacheEntry[Sentiment]),
		maxN:     64,
		newEntry: newEntry,
	}
}

// entry 返回 coin 对应的缓存条目；已达上限时返回一次性条目（fetch 但不驻留）
func (sc *sentimentCache) entry(coin string) *cacheEntry[Sentiment] {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if e, ok := sc.byCoin[coin]; ok {
		return e
	}
	e := sc.newEntry(coin)
	if len(sc.byCoin) >= sc.maxN {
		return e
	}
	sc.byCoin[coin] = e
	return e
}
