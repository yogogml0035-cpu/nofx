# C1 技术设计：Gate 市场数据底座

## 边界

- 新增 `provider/gatemcp/`（MCP 客户端 + 缓存 + 对外稳定取数函数）
- 修改 `provider/gate/symbols.go`（funding_rate）
- 新增 `api/handler_market.go`（五个端点）+ `api/server.go` 公开路由注册
- 不动 kernel/trader/agent（C4 消费方自行接入）

## 数据流

```
前端/C4 决策循环 ──> api/handler_market.go ──> provider/gatemcp 取数函数（TTL 缓存）
                                                ├─ /info  server: overview / snapshot(fear&greed)
                                                └─ /news  server: news / social sentiment
后端 goroutine（60s tick）──> provider/gate ListLiquidationOrders ──> 滚动窗口聚合器（内存）
前端 ──> GET /api/symbols（现有）──> provider/gate ListSymbols（现加 funding_rate）
```

## provider/gatemcp 结构

```go
// Client 封装 /info 与 /news 两个 MCP server 连接（懒初始化，进程内单例）
type Client struct { ... }
func Shared() *Client

// 取数函数（全部走 TTL 缓存，返回原始 JSON 的裁剪视图）
func (c *Client) MarketOverview(ctx) (*MarketOverview, error)   // TTL 5m
func (c *Client) FearGreed(ctx) (*FearGreed, error)             // TTL 5m，取自 snapshot
func (c *Client) News(ctx, limit int) ([]NewsItem, error)       // TTL 15m
func (c *Client) SocialSentiment(ctx) (*Sentiment, error)       // TTL 15m
```

- MCP 客户端：`github.com/modelcontextprotocol/go-sdk/mcp` 的 `mcp.NewStreamableClientTransport(endpoint)` + `mcp.NewClient(...)`；调用 `CallTool(ctx, "info_marketsnapshot_get_market_overview", args)`
- 连接生命周期：Streamable HTTP 无长连握手负担，每次调用走 SDK 内部会话；实现按 SDK 文档推荐方式（首用时 Initialize，出错重建）——以 SDK 实际 API 为准，编码前先跑最小 main 验证调用形态
- 缓存：每取数函数一个 `cacheEntry{mu, expiresAt, value, refreshing chan}`；过期后首个请求发起刷新，期间到达的请求等待同一 chan 结果；刷新失败对等待者返回错误，`value` 保留旧值仅供错误信息标注"上次成功时间"，**不作为成功返回**
- 裁剪视图：overview 保留板块排行 top 10 + BTC/ETH 摘要；news 保留 title/url/source/published_at/情绪标签 top 20；sentiment 保留整体分值 + top 标签；快照只取恐惧贪婪字段。字段名以真实返回为准，编码时先 curl 一次对齐

## 清算热点聚合器

```go
// provider/gate/liquidations.go
type Hotspot struct {
    Contract     string  `json:"contract"`
    Value        float64 `json:"value"`         // 名义清算额（USDT 近似）
    Count        int     `json:"count"`
    LongAmount   float64 `json:"long_amount"`   // 多头被清算名义额
    ShortAmount  float64 `json:"short_amount"`  // 空头被清算名义额
}
func StartLiquidationAggregator(ctx) // server 启动时拉起，60s tick
func LiquidationHotspots(window time.Duration, topN int) []Hotspot
```

- 每次轮询拉取接口当前返回的全部清算单（`ListLiquidationOrders` 默认页），按 `create_time` 去重入窗口
- 窗口 30 分钟环形/切片裁剪；名义额 = size × mark_price（单内字段，缺失时跳过该单）
- 轮询失败：记日志，保留已有窗口继续服务（这是聚合器的容错语义，不是数据造假——窗口内是真实拉取过的数据）

## API 端点

`api/handler_market.go` 薄封装，直接调 provider 取数函数；错误用现有 `SafeInternalError` 风格返回 500 + message。五个端点全部进 `api/server.go` 公开路由区（与 `/api/symbols` 相邻）。

## 测试

- gatemcp：缓存 TTL 过期/命中/并发合并（用 httptest 假 MCP server 或注入 fake caller——只 fake 上游返回结构，不 fake 业务逻辑）；裁剪函数纯函数单测
- gate：`SymbolInfoFromContract` 增加 funding_rate 映射断言；清算聚合窗口滚动/去重/聚合单测（构造时间戳可控的单据）
- handler：路由注册存在性 + 错误传递（上游错误 → 500）

## 风险与回滚点

- go-sdk API 变动（pre-1.0）：第一步先写探针 main 实测四工具调用与返回结构，再定封装；版本锁死后升级需显式改 go.mod
- api.gatemcp.ai 国内可达性：探针 main 首先验证；不可达则上报用户决策（不私接代理）
- 回滚点：本任务全部为新增 + 两处小改（symbols.go、server.go），单 commit 粒度可 revert（不用 git 回滚，见用户规则——此处指实现按 commit 分步，出错手动恢复文件状态）
