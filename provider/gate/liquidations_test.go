package gate

import (
	"context"
	"testing"
	"time"
)

// newTestAggregator 构造注入 fake puller 的聚合器（不依赖 tick 与真实网络）
func newTestAggregator(pull liqPullFunc, window time.Duration) *LiquidationAggregator {
	return &LiquidationAggregator{
		window: window,
		pull:   pull,
		seen:   make(map[liqKey]struct{}),
	}
}

// 名义额聚合、方向拆分、FillPrice 缺失回退 OrderPrice、缺价/小数 size 处理
func TestLiquidationAbsorbAndHotspots(t *testing.T) {
	a := newTestAggregator(nil, 30*time.Minute)
	now := time.Now()
	ts := func(offset time.Duration) int64 { return now.Add(offset).Unix() }

	a.absorb(now, []rawLiqOrder{
		// 多头被清算：|1000| × 2.0 = 2000（上游实证：size 正值、字符串形态）
		{Time: ts(-1 * time.Minute), Contract: "BTC_USDT", Size: "1000", FillPrice: "2.0"},
		// 空头被清算：|500| × 2.0 = 1000
		{Time: ts(-2 * time.Minute), Contract: "BTC_USDT", Size: "-500", FillPrice: "2.0"},
		// FillPrice 缺失回退 OrderPrice：|10| × 5.0 = 50
		{Time: ts(-3 * time.Minute), Contract: "ETH_USDT", Size: "-10", OrderPrice: "5.0"},
		// 两者皆缺：跳过
		{Time: ts(-4 * time.Minute), Contract: "XRP_USDT", Size: "-100"},
		// 带小数的 size（探针实证形态，如 "-1.4"）：|1.4| × 1800 = 2520
		{Time: ts(-5 * time.Minute), Contract: "SOL_USDT", Size: "-1.4", FillPrice: "1800"},
	})

	hs := a.Hotspots(30*time.Minute, 20)
	if len(hs) != 3 {
		t.Fatalf("hotspots = %d, want 3 (priceless skipped)", len(hs))
	}
	var btc, sol *Hotspot
	for i := range hs {
		switch hs[i].Contract {
		case "BTC_USDT":
			btc = &hs[i]
		case "SOL_USDT":
			sol = &hs[i]
		}
	}
	if btc == nil || btc.Value != 3000 || btc.Count != 2 || btc.LongAmount != 2000 || btc.ShortAmount != 1000 {
		t.Fatalf("BTC hotspot mismatch: %+v", btc)
	}
	if sol == nil || sol.Value != 2520 || sol.LongAmount != 0 || sol.ShortAmount != 2520 {
		t.Fatalf("SOL fractional-size hotspot mismatch: %+v", sol)
	}
}

// 去重键 (Time,Contract,Size,FillPrice)：重叠拉取的同键事件只记一次
func TestLiquidationDedup(t *testing.T) {
	a := newTestAggregator(nil, 30*time.Minute)
	now := time.Now()
	batch := []rawLiqOrder{
		{Time: now.Add(-time.Minute).Unix(), Contract: "BTC_USDT", Size: "10", FillPrice: "2.0"},
	}
	a.absorb(now, batch)
	a.absorb(now.Add(time.Second), batch) // 重叠窗口再次命中同键

	hs := a.Hotspots(30*time.Minute, 20)
	if len(hs) != 1 || hs[0].Count != 1 || hs[0].Value != 20 {
		t.Fatalf("dedup failed: %+v", hs)
	}
}

// 滚动窗口：窗口外事件不参与聚合
func TestLiquidationWindowRolling(t *testing.T) {
	const win = 5 * time.Minute
	a := newTestAggregator(nil, win)
	now := time.Now()

	a.absorb(now, []rawLiqOrder{
		{Time: now.Add(-10 * time.Minute).Unix(), Contract: "OLD_USDT", Size: "100", FillPrice: "1.0"},
		{Time: now.Add(-1 * time.Minute).Unix(), Contract: "NEW_USDT", Size: "100", FillPrice: "1.0"},
	})

	hs := a.Hotspots(win, 20)
	if len(hs) != 1 || hs[0].Contract != "NEW_USDT" {
		t.Fatalf("window trim failed: %+v", hs)
	}
}

// 水位重叠：From = 上次成功水位 − 60s；首轮 From = now − 60s
func TestLiquidationWatermarkOverlap(t *testing.T) {
	var froms []time.Time
	pull := func(ctx context.Context, from time.Time, limit int32) ([]rawLiqOrder, error) {
		froms = append(froms, from)
		return []rawLiqOrder{{Time: time.Now().Unix(), Contract: "BTC_USDT", Size: "1", FillPrice: "1"}}, nil
	}
	a := newTestAggregator(pull, 30*time.Minute)

	ctx := context.Background()
	a.pullOnce(ctx)
	first := froms[0]
	if d := time.Since(first); d > 61*time.Second || d < 59*time.Second {
		t.Fatalf("first from ≈ now-60s, got %v ago", time.Since(first))
	}

	time.Sleep(1100 * time.Millisecond) // 保证水位前进可测
	a.pullOnce(ctx)
	overlap := froms[1].Sub(froms[0])
	if overlap < 0 || overlap > time.Second {
		t.Fatalf("second from should be lastWatermark-60s (+~1s clock), got overlap %v", overlap)
	}
}

// 长时间失败后水位过期：From 收缩到接口 ≤3600s 窗口约束内
func TestLiquidationFromClampedToMaxSpan(t *testing.T) {
	var got time.Time
	pull := func(ctx context.Context, from time.Time, limit int32) ([]rawLiqOrder, error) {
		got = from
		return nil, nil
	}
	a := newTestAggregator(pull, 30*time.Minute)
	a.mu.Lock()
	// 伪造 2 小时前的水位（超出接口 3600s 约束）
	a.lastWatermark = time.Now().Add(-2 * time.Hour)
	a.mu.Unlock()

	a.pullOnce(context.Background())
	if d := time.Since(got); d > 3600*time.Second {
		t.Fatalf("from must be within 3600s, got %v ago", time.Since(got))
	}
}

// 拉取失败：保留已有窗口继续服务、lastPull 不被失败拉取推进；Limit=1000 被拒时降 100 重试
func TestLiquidationPullFailureKeepsWindow(t *testing.T) {
	now := time.Now()
	a := newTestAggregator(nil, 30*time.Minute)
	a.absorb(now, []rawLiqOrder{
		{Time: now.Add(-time.Minute).Unix(), Contract: "BTC_USDT", Size: "10", FillPrice: "2.0"},
	})
	pullBefore, _ := a.LastPull()

	calls := 0
	a.pull = func(ctx context.Context, from time.Time, limit int32) ([]rawLiqOrder, error) {
		calls++
		return nil, context.DeadlineExceeded
	}
	a.pullOnce(context.Background())

	if calls != 2 {
		t.Fatalf("pull calls = %d, want 2 (1000 then retry 100)", calls)
	}
	hs := a.Hotspots(30*time.Minute, 20)
	if len(hs) != 1 || hs[0].Value != 20 {
		t.Fatalf("window must survive pull failure: %+v", hs)
	}
	if pullAfter, _ := a.LastPull(); !pullAfter.Equal(pullBefore) {
		t.Fatal("failed pull must not advance lastPull")
	}
}

// 降档重试成功：水位正常推进
func TestLiquidationRetrySuccess(t *testing.T) {
	calls := 0
	a := newTestAggregator(func(ctx context.Context, from time.Time, limit int32) ([]rawLiqOrder, error) {
		calls++
		if limit == 1000 {
			return nil, context.Canceled
		}
		return []rawLiqOrder{{Time: time.Now().Unix(), Contract: "BTC_USDT", Size: "5", FillPrice: "1.0"}}, nil
	}, 30*time.Minute)

	a.pullOnce(context.Background())
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	if _, ok := a.LastPull(); !ok {
		t.Fatal("successful retry must update lastPull")
	}
}
