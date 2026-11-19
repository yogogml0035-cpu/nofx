# 后端 Gate 市场数据底座：MCP provider、清算聚合与公开端点

## Goal

为「数据」看板（C3）与市场环境上下文注入（C4）提供统一后端底座：Gate MCP 免认证只读 provider（带 TTL 缓存）、Gate 行情补资金费率字段、清算热点滚动聚合、`/api/market/*` 公开端点。本任务不涉及任何前端改动。

## Requirements

1. **provider/gatemcp 新包**：连接 `https://api.gatemcp.ai/mcp/info` 与 `https://api.gatemcp.ai/mcp/news`（Streamable HTTP + SSE，免认证；探针已实证直连可用，见 research/），只封装 4 个工具：
   - `info_marketsnapshot_get_market_overview` → 市场总览（**含恐惧贪婪指数与标签**，一个调用双数据；字段集见 design「探针结论」——无板块数据、无逐币 24h 涨跌）
   - `news_feed_search_news` → **仅传 coin 不传 query**（定稿 `{"coin":"BTC","limit":20}`；query 任何取值实测全空，见 research/news_matrix_output.txt）
   - `news_feed_get_exchange_announcements` → 市场级公告/标题流（不传 exchange，实测真实条目）
   - `news_feed_get_social_sentiment` → 社交情绪（coin 必传，默认 BTC）
2. **TTL 缓存**：总览(含恐惧贪婪) 5 分钟，新闻/公告/情绪 15 分钟；进程内缓存，过期后首次请求同步刷新，并发请求经 `golang.org/x/sync/singleflight`（go.sum 已有 v0.20.0）合并为一次上游调用；上游失败时返回错误（fast-fail），不返回过期缓存冒充新数据——API 错误响应含可公开的错误类别摘要，完整错误进服务日志
3. **provider/gate 增强**：`SymbolInfo` 新增 `funding_rate float64`（源 `ticker.FundingRate`，**Gate 原始小数语义、不 ×100、不加 omitempty**——费率恰为 0 的合约不能字段缺失），`/api/symbols` 响应随之携带；同时为 ListSymbols 增加 30–60s 进程内 TTL 缓存（该端点免鉴权且将被 30s 轮询，现状每请求打两次 Gate 上游）。仅改 provider/gate/symbols.go 的 SymbolInfo——market 包有同名 Binance 语义结构，勿动
4. **清算热点聚合**：后端每 60s 轮询 `client.FuturesApi.ListLiquidatedOrders(ctx, "usdt", opts)`（`GET /futures/usdt/liq_orders` 公开免鉴权；opts：`From` = 上次成功水位 − 60s 重叠、`Limit` = 1000），维护最近 30 分钟滚动窗口，按合约聚合名义清算额（`|Size| × FillPrice`，缺失回退 OrderPrice）并按 Size 符号拆多/空，进程内状态；进程重启后窗口重新积累（可接受，不做持久化）。语义：热点相对排名，极端行情下有界漏计（上游无唯一单号，去重键 Time+Contract+Size+FillPrice）
5. **公开 API 端点**（注册方式对齐 `/api/symbols`，`api/server.go` 公开路由表；上游失败一律 502 + 可公开错误类别摘要，不用 SafeInternalError 的 generic 隐藏语义）：
   - `GET /api/market/overview` — 市场总览（探针实证字段集：总市值/24h 变化/总量/dominance/BTC 价格/ahr999/山寨季/活跃币数/updated_at）
   - `GET /api/market/fear-greed` — `{value, label, change_24h, first_seen_at, updated_at}`；**上游无变化字段，change_24h 由后端进程内 24h 历史环派生**（overview 每次真实刷新追加样本，保留 25h；冷启动不足 24h 或重启后为 null，不编造）
   - `GET /api/market/news?limit=` — BTC 新闻 top N（默认 20 上限 50；coin-only 调用）
   - `GET /api/market/announcements?limit=` — 市场级公告/标题 top N（默认 10 上限 50）
   - `GET /api/market/sentiment?coin=` — 社交情绪（coin 默认 BTC）
   - `GET /api/market/liquidations` — 清算热点 top 20（合约、清算额、单数、窗口起点）
6. **网络与依赖**：引入官方 `github.com/modelcontextprotocol/go-sdk@v1.8.0`（锁版本写死 go.mod）；GOPROXY 走 goproxy.cn；宿主机无 Go，go 命令统一经 `docker run golang:1.25-alpine` 执行；provider 支持 `GATEMCP_ENDPOINT` 环境变量覆盖（默认 `https://api.gatemcp.ai`，断链验收用）

## Constraints

- MCP 仅限免认证只读端点；不接 `/mcp/exchange`、`/mcp/dex`；MCP 工具仅上述 4 个（/mcp/news 共 17 工具，events/ugc 实测空、web_search/search_x 为 LLM 合成类、hot_topics 与 sentiment 重复——均不采用）
- fast-fail：不造数据、不吞错误、不做降级假值
- 公开端点与 `/api/symbols` 同姿态（免鉴权只读）：上游调用被 TTL + singleflight 封顶、清算聚合内存有界；不做应用层频控，超该姿态的滥用由部署层网关处理
- 缓存并发合并用 `golang.org/x/sync/singleflight`，不手写 chan 合并（CI 带 -race）

## Acceptance Criteria

- [ ] `go build ./...`、`go test ./...` 全绿（新增包有单测：缓存 TTL/singleflight、恐惧贪婪历史环、清算聚合窗口/去重/方向拆分、字段映射）
- [ ] 本地起临时后端（Docker 交叉编译 darwin/arm64 二进制后宿主运行，命令见 implement.md），curl 六个 `/api/market/*` 端点均返回真实 Gate/MCP 数据（探针输出留档到任务目录 `probe/`）
- [ ] `GET /api/symbols` 响应含 `funding_rate`，抽查 BTC_USDT 等 5 个合约值非空（Gate 原始小数语义）
- [ ] 断网点复测（`GATEMCP_ENDPOINT=http://127.0.0.1:9` 起后端）：六个 `/api/market/*` 返回 502 + 明确错误类别；`/api/symbols` 与清算端点不受 MCP 断链影响
- [ ] go.mod 中 go-sdk 版本为锁定版本 v1.8.0
