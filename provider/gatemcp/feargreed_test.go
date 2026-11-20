package gatemcp

import (
	"testing"
	"time"
)

// 冷启动：样本跨度不足 24h 时 change24h 不派生（ok=false）
func TestFearGreedColdStart(t *testing.T) {
	h := newFearGreedHistory()
	now := time.Now()
	h.add(now, 30)
	if _, ok := h.change24h(now, 40); ok {
		t.Fatal("cold start must not derive change_24h")
	}
	if _, ok := h.first(); !ok {
		t.Fatal("first_seen_at should exist after first sample")
	}
}

// 24h 变化：当前值 − 距 now-24h 最近的过去样本
func TestFearGreedChange24h(t *testing.T) {
	h := newFearGreedHistory()
	now := time.Now()
	// 25h 前样本（应被 25h 窗口裁剪掉，不参与）
	h.add(now.Add(-25*time.Hour), 90)
	// 24h 前 + 5min：距 target=now-24h 最近的过去样本
	h.add(now.Add(-24*time.Hour).Add(-5*time.Minute), 50)
	// 23h 前（晚于 target，不参与）
	h.add(now.Add(-23*time.Hour), 60)

	if got, ok := h.change24h(now, 42); !ok {
		t.Fatal("history >= 24h should derive change")
	} else if diff := got - (42 - 50); diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("change = %v, want %v", got, 42-50)
	}
}

// 无任何 ≤ now-24h 的样本时不派生
func TestFearGreedChange24hInsufficientSpan(t *testing.T) {
	h := newFearGreedHistory()
	now := time.Now()
	h.add(now.Add(-12*time.Hour), 50)
	if _, ok := h.change24h(now, 40); ok {
		t.Fatal("span < 24h must not derive")
	}
}

// 窗口裁剪：超过 25h 的样本被丢弃；maxN 兜底
func TestFearGreedPrune(t *testing.T) {
	h := newFearGreedHistory()
	now := time.Now()
	h.add(now.Add(-30*time.Hour), 10) // 超窗
	h.add(now.Add(-1*time.Hour), 20)
	first, ok := h.first()
	if !ok || !first.Equal(now.Add(-1*time.Hour)) {
		t.Fatalf("first = %v, want pruned window start", first)
	}
	if got, ok := h.change24h(now, 20); ok {
		t.Fatalf("after prune span < 24h, change must be null; got %v", got)
	}

	// maxN 兜底：高频样本不撑爆
	for i := 0; i < 500; i++ {
		h.add(now.Add(time.Duration(i)*time.Second), float64(i))
	}
	h.mu.Lock()
	n := len(h.samples)
	h.mu.Unlock()
	if n > h.maxN {
		t.Fatalf("samples = %d exceed cap %d", n, h.maxN)
	}
}
