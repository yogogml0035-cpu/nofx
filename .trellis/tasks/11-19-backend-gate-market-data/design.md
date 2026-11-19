# C1 技术设计：Gate 市场数据底座

## 边界

- 新增 `provider/gatemcp/`（MCP 客户端 + 缓存 + 对外稳定取数函数）
- 修改 `provider/gate/symbols.go`（funding_rate）
- 新增 `api/handler_market.go`（五个端点）+ `api/server.go` 公开路由注册
- 不动 kernel/trader/agent（C4 消费方自行接入）

## 探针结论（已完成，留档 research/mcp_probe_output.txt）

- 端点：`https://api.gatemcp.ai/mcp/info`、`https://api.gatemcp.ai/mcp/news`（**带 /mcp 前缀**；根路径 /info /news 返回 404）
- initialize → notifications/initialized → tools/call 全 200，Mcp-Session-Id 会话正常，上海网络直连可用
- `info_marketsnapshot_get_market_overview` 单调用即含：总市值/24h 变化/总量、BTC/ETH dominance、BTC 价格、**恐惧贪婪指数+标签**、ahr999、山寨季指数 → FearGreed 取数不再调 snapshot，直接从 overview 缓存切片
- `news_feed_search_news` 通道通但 query="bitcoin" 返回 total:0——实施时调参（keyword/similarity/sort），空态返回真实空列表
- `news_feed_get_social_sentiment` 正常（overall_sentiment/正负比例/top_tweets）
- 依赖：`github.com/modelcontextprotocol/go-sdk` 锁 **v1.8.0**（goproxy.cn 可取）；宿主机无 Go，门禁与调试走 `docker run golang:1.25-alpine`（GOPROXY=goproxy.cn）

## 数据流

```
前端/C4 决策循环 ──> api/handler_market.go ──> provider/gatemcp 取数函数（TTL 缓存）
                                                ├─ /mcp/info  server: market_overview（含恐惧贪婪）
                                                └─ /mcp/news  server: news / social sentiment
后端 goroutine（60s tick）──> provider/gate ListLiquidationOrders ──> 滚动窗口聚合器（内存）
前端 ──> GET /api/symbols（现有）──> provider/gate ListSymbols（现加 funding_rate）
```

## provider/gatemcp 结构

```go
// Client 封装 /info 与 /news 两个 MCP server 连接（懒初始化，进程内单例）
type Client struct { ... }
func Shared() *Client

// 取数函数（全部走 TTL 缓存，返回原始 JSON 的裁剪视图）
func (c *Client) MarketOverview(ctx) (*MarketOverview, error)   // TTL 5m，含恐惧贪婪字段
func (c *Client) FearGreed(ctx) (*FearGreed, error)             // TTL 5m，从 overview 缓存切片（零额外上游调用）
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

- go-sdk v1.8.0 为锁定版本：SDK API 以该版本文档为准（探针已验证协议层可用，SDK 封装层以 v1.8.0 API 编码）；升级需显式改 go.mod
- api.gatemcp.ai 可达性已实证（直连 200）；生产环境（Docker 同宿主网络）复测一次即可
- news_search 空结果：调参后仍可能稀疏——空列表是真实空态，不造假
- 回滚点：本任务全部为新增 + 两处小改（symbols.go、server.go），单 commit 粒度（不用 git 回滚，见用户规则——此处指实现按 commit 分步，出错手动恢复文件状态）
