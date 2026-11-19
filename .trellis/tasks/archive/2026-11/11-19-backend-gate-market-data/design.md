# C1 技术设计：Gate 市场数据底座

## 边界

- 新增 `provider/gatemcp/`（MCP 客户端 + 缓存 + 对外稳定取数函数）
- 修改 `provider/gate/symbols.go`（funding_rate + ListSymbols TTL 缓存）
- 新增 `provider/gate/liquidations.go`（清算热点聚合器）
- 新增 `api/handler_market.go`（六个端点）+ `api/server.go` 公开路由注册；`main.go` 拉起聚合 goroutine
- 不动 kernel/trader/agent（C4 消费方自行接入）
- 仅改 provider/gate/symbols.go 的 SymbolInfo——market 包有同名 Binance 语义结构，勿动

## 探针结论（已完成，留档 research/：mcp_probe、news_matrix、news_tools_list、news_alternatives 四份）

- 端点：`https://api.gatemcp.ai/mcp/info`、`https://api.gatemcp.ai/mcp/news`（**带 /mcp 前缀**；根路径 /info /news 返回 404）
- initialize → notifications/initialized → tools/call 全 200，Mcp-Session-Id 会话正常，上海网络直连可用
- `info_marketsnapshot_get_market_overview` 单调用即含：总市值/24h 变化/总量、BTC/ETH dominance、BTC 价格、**恐惧贪婪指数+标签**、ahr999、山寨季指数、活跃币数、updated_at → FearGreed 取数不调 snapshot，直接从 overview 缓存切片。**字段全集即上述（structuredContent 实证）——无板块(sector)数据、无逐币 24h 涨跌，总览展示与 C4 注入格式以此字段集为准**
- `news_feed_search_news` 参数形态已定稿（news_matrix 全矩阵 + news_alternatives 补充探针）：**query 任何取值（含与 coin 并用、含 time_range/lang/platform 变体）一律 total:0；仅传 `coin` 不传 query 时走 OpenSearch 返回真实新闻**（coin="BTC" 实测 5/5）。定稿调用：`{"coin":"BTC","limit":20}`
- 市场级标题补充：`news_feed_get_exchange_announcements`（不传 exchange 返回市场级新闻/公告混合流，实测 5/5 真实条目）
- 不采用：`news_events_get_latest_events`（实测 0 条）、`news_feed_search_ugc`（0 条）、`news_feed_web_search`/`search_x`（LLM 合成类，慢且带 disclaimer）、`hot_topics`（单币社交主题，与 sentiment 重复）
- `news_feed_get_social_sentiment` 正常——**coin 必传**（探针 {"coin":"BTC"}），返回 overall_sentiment/正负比例/top_tweets
- /mcp/news 服务器共 17 个工具（news_tools_list_output.txt 全量清单），本任务只用上述 4 个
- 依赖：`github.com/modelcontextprotocol/go-sdk` 锁 **v1.8.0**（goproxy.cn 可取）；`golang.org/x/sync` v0.20.0 已在 go.sum（singleflight 直接用）；宿主机无 Go，门禁与调试走 `docker run golang:1.25-alpine`（GOPROXY=goproxy.cn）

## 数据流

```
前端/C4 决策循环 ──> api/handler_market.go ──> provider/gatemcp 取数函数（TTL 缓存）
                                                ├─ /mcp/info  server: market_overview（含恐惧贪婪）
                                                └─ /mcp/news  server: search_news(coin) / announcements / social sentiment
后端 goroutine（60s tick，main.go 拉起）──> provider/gate ListLiquidatedOrders ──> 滚动窗口聚合器（内存）
前端 ──> GET /api/symbols（现有，加 30–60s TTL 缓存）──> provider/gate ListSymbols（现加 funding_rate）
```

## provider/gatemcp 结构

```go
// Client 封装 /info 与 /news 两个 MCP server 连接（懒初始化，进程内单例；GATEMCP_ENDPOINT 环境变量可覆盖默认 https://api.gatemcp.ai）
type Client struct { ... }
func Shared() *Client

// 取数函数（全部走 TTL 缓存，返回原始 JSON 的裁剪视图）
func (c *Client) MarketOverview(ctx) (*MarketOverview, error)        // TTL 5m，含恐惧贪婪字段
func (c *Client) FearGreed(ctx) (*FearGreed, error)                  // TTL 5m，从 overview 缓存切片（零额外上游调用）+ 24h 历史派生变化
func (c *Client) News(ctx, limit int) ([]NewsItem, error)            // TTL 15m，search_news {"coin":"BTC"}（不传 query）
func (c *Client) Announcements(ctx, limit int) ([]NewsItem, error)   // TTL 15m，exchange_announcements 市场级流
func (c *Client) SocialSentiment(ctx, coin string) (*Sentiment, error) // TTL 15m，coin 必传
```

- MCP 客户端（v1.8.0 真实 API；探针脚本为裸 JSON-RPC，SDK 封装形态以步骤 2 冒烟为准）：`transport := &mcp.StreamableClientTransport{Endpoint: endpoint}` → `client := mcp.NewClient(...)` → `session, err := client.Connect(ctx, transport)` → `session.CallTool(ctx, &mcp.CallToolParams{Name: "...", Arguments: ...})`（v1.8.0 无 NewStreamableClientTransport 工厂；CallTool 挂在 ClientSession 上、参数为结构体）
- 连接生命周期：SDK 默认 initialize 后建立 standalone SSE GET 长连接并自动重连——Gate 服务端对 GET SSE 的行为未实测，冒烟必验；若 405/重连风暴则设 `DisableStandaloneSSE: true`。出错重建前必须 `Close()` 旧 ClientSession（否则连接泄漏）；两个 server 连接独立懒初始化
- 缓存：每取数函数一个 `cacheEntry`；过期后首个请求发起刷新，并发请求经 `golang.org/x/sync/singleflight` 合并为一次上游调用（CI 带 -race，不手写 chan 合并）；刷新失败对等待者返回错误，`value` 保留旧值仅供错误信息标注"上次成功时间"，**不作为成功返回**
- 恐惧贪婪 24h 变化派生：上游无变化字段（探针实证仅 index+label+updated_at）。overview 缓存每次真实刷新（非命中）时追加样本 `(fetchTime, index)` 进进程内环形缓冲（保留 25h，约 300 样本）；`change_24h` = 当前值 − 与 `now-24h` 最近的历史样本；冷启动不足 24h 或重启后为 `null`（响应附 `first_seen_at`），不编造
- 裁剪视图：overview 保留探针实证字段集（total_market_cap/market_cap_change_24h/total_volume_24h/btc_dominance/eth_dominance/btc_price/fear_greed_index/fear_greed_label/ahr999/altcoin_season_index/active_coins_count/updated_at）——**无板块数据，不做"板块排行"**；news 保留 title/url/source/published_at/情绪标签 top 20；announcements 保留 title/url/published_at top 10；sentiment 保留整体分值 + 正负比例 + top 标签。字段名以真实返回为准，编码时先 curl 一次对齐

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
func StartLiquidationAggregator(ctx) // main.go 在 server.Start() 前拉起，ctx 接入 Shutdown；启动立即拉取一次再进 60s tick
func LiquidationHotspots(window time.Duration, topN int) []Hotspot
```

- 接口为 `client.FuturesApi.ListLiquidatedOrders(ctx, "usdt", opts)`（`GET /futures/usdt/liq_orders`，公开免鉴权；gateapi-go v6.104.3 实证方法名）
- opts：`From = 上次成功水位 − 60s`（重叠去重防漏）、`Limit = 1000`（服务端拒绝则降 100 并记日志）；接口支持 from/to（间隔 ≤3600s）与分页，**但无唯一单号 ID、历史深度有限**
- 名义额 = `|Size| × parseFloat(FillPrice)`（FillPrice 缺失回退 OrderPrice，两者皆缺跳过该单）——FuturesLiqOrder 无 mark_price 字段
- 方向：`Size > 0` 计多头被清算、`< 0` 计空头（Gate Size 带方向符号）
- 去重键 `(Time, Contract, Size, FillPrice)`；同键的独立清算单会被合并为一笔（上游无单号），误差有界可接受
- 窗口 30 分钟滚动裁剪；轮询失败记日志、保留已有窗口继续服务（窗口内是真实拉取过的数据，非造假）
- 语义边界：榜单为轮询近似样本的**热点相对排名**，极端行情分钟级洪峰下存在漏计，不是清算总量审计值

## API 端点

`api/handler_market.go` 薄封装，直接调 provider 取数函数。上游（MCP/Gate）失败返回 **502**（区别于内部错误 500），message 含可公开的错误类别摘要（"Gate MCP upstream unreachable" / "upstream error: timeout" 类），完整错误进服务日志——**不用 SafeInternalError**（它对客户端只回 generic message 且把网络类错误判敏感隐藏，与「明确错误态」验收冲突）。六个端点全部进 `api/server.go` 公开路由区（与 `/api/symbols` 相邻，注册形态照搬）：

- `GET /api/market/overview` — 市场总览（探针实证字段集）
- `GET /api/market/fear-greed` — `{value, label, change_24h, first_seen_at, updated_at}`（change_24h 为进程内 24h 历史派生，冷启动为 null）
- `GET /api/market/news?limit=` — BTC 新闻 top N（默认 20 上限 50）
- `GET /api/market/announcements?limit=` — 市场级公告/标题 top N（默认 10 上限 50）
- `GET /api/market/sentiment?coin=` — 社交情绪（coin 默认 BTC）
- `GET /api/market/liquidations` — 清算热点 top 20（合约、清算额、单数、窗口起点）

另：`/api/symbols`（现有）加 funding_rate 时为 ListSymbols 增加 30–60s 进程内 TTL 缓存——该端点免鉴权且 C3 将 30s 轮询，现状每请求打两次 Gate 上游（contracts + tickers）。

## 测试

- gatemcp：缓存 TTL 过期/命中/singleflight 合并（httptest 假 MCP server——只 fake 上游返回结构，不 fake 业务逻辑）；裁剪函数纯函数单测；恐惧贪婪历史环（样本追加、24h 差值、冷启动 null）
- gate：`SymbolInfoFromContract` funding_rate 映射断言（Gate 原始小数语义、不 ×100）；ListSymbols 缓存命中/过期；清算聚合窗口滚动/去重/聚合/方向拆分单测（**注入 fake puller** 提供可控时间戳单据，不依赖 tick 与真实网络）
- handler：路由注册存在性 + 错误传递（上游错误 → 502 且 message 含可见类别）
- 聚合器挂载在 main.go、不在 setupRoutes/NewServer——`api/server_test.go` 直接调 setupRoutes，挂那里会让 `go test` 拉起真实网络轮询 goroutine

## 风险与回滚点

- go-sdk v1.8.0 为锁定版本：SDK API 以该版本文档为准；升级需显式改 go.mod
- api.gatemcp.ai 可达性已实证（直连 200）；生产环境（Docker 同宿主网络）复测一次即可
- 断网点验证方式：`GATEMCP_ENDPOINT` 环境变量覆盖（见上），容器/宿主/生产三种形态都可控——改 hosts 在容器网络下不可靠
- news 数据面：coin-only 调用已实证 5/5 真实条目；若上线后上游清空，空列表是真实空态，不造假
- 回滚点：本任务为新增 + 三处小改（symbols.go、server.go、main.go），单 commit 粒度（不用 git 回滚，见用户规则——实现按 commit 分步，出错手动恢复文件状态）
