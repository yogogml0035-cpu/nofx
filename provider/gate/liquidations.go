package gate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"time"

	"nofx/logger"
)

// 清算热点聚合器：60s 轮询 GET /futures/usdt/liq_orders（公开免鉴权），
// From 取上次成功水位 −60s 重叠去重；维护 30 分钟滚动窗口，按合约聚合
// 名义清算额（|Size|×FillPrice，缺失回退 OrderPrice，皆缺跳过该单）并按 Size 符号拆多/空。
// 语义边界：上游无唯一单号，去重键 (Time,Contract,Size,FillPrice)，同键独立单会被合并
// （误差有界）；榜单为轮询样本的热点相对排名，极端行情下有界漏计，不是清算总量审计值。
// 进程内状态，重启后窗口重新积累（可接受，不做持久化）。

const (
	// LiquidationWindow 默认滚动窗口
	LiquidationWindow = 30 * time.Minute
	// LiquidationPollInterval 轮询间隔
	LiquidationPollInterval = 60 * time.Second
	// LiquidationTopN 默认榜单条数
	LiquidationTopN = 20

	liqPullLimit  = int32(1000)
	liqRetryLimit = int32(100)
	liqOverlap    = 60 * time.Second
	liqMaxSpan    = 3600 * time.Second // 接口约束 from ≤3600s

	liqEndpoint = "https://api.gateio.ws/api/v4/futures/usdt/liq_orders"
)

// Hotspot 清算热点聚合条目（名义额 USDT 近似）
type Hotspot struct {
	Contract    string  `json:"contract"`
	Value       float64 `json:"value"`
	Count       int     `json:"count"`
	LongAmount  float64 `json:"long_amount"`  // 多头被清算名义额
	ShortAmount float64 `json:"short_amount"` // 空头被清算名义额
}

// liqKey 去重键（上游无唯一单号；size 保留上游原始字符串形态）
type liqKey struct {
	at       int64
	contract string
	size     string
	price    string
}

type liqEvent struct {
	at       time.Time
	contract string
	notional float64
	long     bool
	key      liqKey
}

// rawLiqOrder /futures/usdt/liq_orders 真实返回结构（2026-11-19 运行时探针实证）：
// left/size/order_size 均为字符串且 size 可带小数（如 "-1.4"），与 gateapi-go v6.104.3
// 的 int64 模型不兼容（整批反序列化失败），故该接口走直接 HTTP 解析。
// 方向语义（实证）：size 为被清算仓位符号，>0 多头被清算、<0 空头被清算；
// order_size 为强平执行方向（与之相反）。
type rawLiqOrder struct {
	Contract   string `json:"contract"`
	Size       string `json:"size"`
	FillPrice  string `json:"fill_price"`
	OrderPrice string `json:"order_price"`
	Time       int64  `json:"time"`
}

// liqPullFunc 单次拉取（from 为窗口起点；limit 为服务端单页上限）；可注入替换供测试
type liqPullFunc func(ctx context.Context, from time.Time, limit int32) ([]rawLiqOrder, error)

// LiquidationAggregator 清算事件滚动窗口聚合器
type LiquidationAggregator struct {
	mu            sync.Mutex
	events        []liqEvent
	seen          map[liqKey]struct{}
	window        time.Duration
	lastWatermark time.Time // 上次成功拉取的覆盖终点
	lastPull      time.Time
	pull          liqPullFunc
}

var (
	defaultAggOnce sync.Once
	defaultAgg     *LiquidationAggregator
)

// DefaultLiquidationAggregator 进程内单例（handler 读、main 起循环共用）
func DefaultLiquidationAggregator() *LiquidationAggregator {
	defaultAggOnce.Do(func() {
		defaultAgg = &LiquidationAggregator{
			window: LiquidationWindow,
			pull:   pullLiquidatedOrders,
			seen:   make(map[liqKey]struct{}),
		}
	})
	return defaultAgg
}

// StartLiquidationAggregator 拉起默认聚合循环：启动立即拉一次再进 60s tick；
// ctx 接入 Shutdown，取消即退出。挂载在 main.go（不在 setupRoutes——go test 拉起
// 真实网络轮询会污染测试）。
func StartLiquidationAggregator(ctx context.Context) {
	go DefaultLiquidationAggregator().run(ctx)
}

func (a *LiquidationAggregator) run(ctx context.Context) {
	a.pullOnce(ctx)
	ticker := time.NewTicker(LiquidationPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.pullOnce(ctx)
		}
	}
}

func (a *LiquidationAggregator) pullOnce(ctx context.Context) {
	now := time.Now()
	a.mu.Lock()
	from := now.Add(-liqOverlap)
	if !a.lastWatermark.IsZero() {
		from = a.lastWatermark.Add(-liqOverlap)
	}
	a.mu.Unlock()
	// 接口约束窗口 ≤3600s（长时间失败后水位过期，收缩到安全下界）
	if minFrom := now.Add(-liqMaxSpan + liqOverlap); from.Before(minFrom) {
		from = minFrom
	}

	orders, err := a.pull(ctx, from, liqPullLimit)
	if err != nil {
		// 服务端拒绝大单页时降档重试一次
		logger.Warnf("liquidation pull limit=%d failed, retry limit=%d: %v", liqPullLimit, liqRetryLimit, err)
		orders, err = a.pull(ctx, from, liqRetryLimit)
	}
	if err != nil {
		// 轮询失败记日志、保留已有窗口继续服务（窗口内是真实拉取过的数据）
		logger.Errorf("liquidation pull failed: %v", err)
		return
	}
	a.absorb(now, orders)
}

// absorb 合并新事件、刷新水位并裁剪窗口
func (a *LiquidationAggregator) absorb(now time.Time, orders []rawLiqOrder) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, o := range orders {
		price := parseFloat(o.FillPrice)
		if price == 0 {
			price = parseFloat(o.OrderPrice)
		}
		size := parseFloat(o.Size)
		if price == 0 || size == 0 || o.Contract == "" || o.Time <= 0 {
			continue // 缺价/缺仓位/缺合约/缺时间跳过，不造名义额
		}
		key := liqKey{at: o.Time, contract: o.Contract, size: o.Size, price: o.FillPrice}
		if _, dup := a.seen[key]; dup {
			continue
		}
		a.seen[key] = struct{}{}
		a.events = append(a.events, liqEvent{
			at:       time.Unix(o.Time, 0),
			contract: o.Contract,
			notional: math.Abs(size) * price,
			long:     size > 0, // Gate size 为被清算仓位符号：>0 多头被清算
			key:      key,
		})
	}
	a.lastWatermark = now
	a.lastPull = now
	a.pruneLocked(now)
}

// pruneLocked 裁剪窗口外事件并重建去重集（防 seen 无界增长；
// 窗口外的键不会被重叠拉取再次命中——重叠仅覆盖最近 ~2 分钟）
func (a *LiquidationAggregator) pruneLocked(now time.Time) {
	cutoff := now.Add(-a.window)
	kept := a.events[:0]
	for _, ev := range a.events {
		if ev.at.After(cutoff) {
			kept = append(kept, ev)
		}
	}
	a.events = kept
	a.seen = make(map[liqKey]struct{}, len(kept))
	for _, ev := range kept {
		a.seen[ev.key] = struct{}{}
	}
}

// Hotspots 返回窗口内按名义清算额降序的 topN 热点
func (a *LiquidationAggregator) Hotspots(window time.Duration, topN int) []Hotspot {
	a.mu.Lock()
	defer a.mu.Unlock()
	cutoff := time.Now().Add(-window)
	agg := make(map[string]*Hotspot)
	for _, ev := range a.events {
		if ev.at.Before(cutoff) {
			continue
		}
		h := agg[ev.contract]
		if h == nil {
			h = &Hotspot{Contract: ev.contract}
			agg[ev.contract] = h
		}
		h.Value += ev.notional
		h.Count++
		if ev.long {
			h.LongAmount += ev.notional
		} else {
			h.ShortAmount += ev.notional
		}
	}
	out := make([]Hotspot, 0, len(agg))
	for _, h := range agg {
		out = append(out, *h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Value > out[j].Value })
	if topN >= 0 && len(out) > topN {
		out = out[:topN]
	}
	return out
}

// LastPull 返回最近一次成功拉取时间（无成功拉取 ok=false）
func (a *LiquidationAggregator) LastPull() (time.Time, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.lastPull.IsZero() {
		return time.Time{}, false
	}
	return a.lastPull, true
}

// LiquidationHotspots 默认聚合器的热点查询（handler 用）
func LiquidationHotspots(window time.Duration, topN int) []Hotspot {
	return DefaultLiquidationAggregator().Hotspots(window, topN)
}

// LastLiquidationPull 默认聚合器的最近成功拉取时间
func LastLiquidationPull() (time.Time, bool) {
	return DefaultLiquidationAggregator().LastPull()
}

// pullLiquidatedOrders 直连 Gate 上游（公开免鉴权，无需 APIv4 凭证）
func pullLiquidatedOrders(ctx context.Context, from time.Time, limit int32) ([]rawLiqOrder, error) {
	q := url.Values{}
	q.Set("from", strconv.FormatInt(from.Unix(), 10))
	q.Set("limit", strconv.FormatInt(int64(limit), 10))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, liqEndpoint+"?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("build liquidation request: %w", err)
	}
	req.Header.Set("X-Gate-Channel-Id", "nofx")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gate liquidation request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("gate liquidation HTTP %d: %s", resp.StatusCode, string(body))
	}
	var orders []rawLiqOrder
	if err := json.NewDecoder(resp.Body).Decode(&orders); err != nil {
		return nil, fmt.Errorf("decode gate liquidation response: %w", err)
	}
	return orders, nil
}
