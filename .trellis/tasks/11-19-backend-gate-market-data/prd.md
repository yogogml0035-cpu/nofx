# 后端 Gate 市场数据底座：MCP provider、清算聚合与公开端点

## Goal

为「数据」看板（C3）与市场环境上下文注入（C4）提供统一后端底座：Gate MCP 免认证只读 provider（带 TTL 缓存）、Gate 行情补资金费率字段、清算热点滚动聚合、`/api/market/*` 公开端点。本任务不涉及任何前端改动。

## Requirements

1. **provider/gatemcp 新包**：连接 `https://api.gatemcp.ai/info` 与 `https://api.gatemcp.ai/news`（Streamable HTTP + SSE，免认证），只封装 4 个工具：
   - `info_marketsnapshot_get_market_overview` → 市场总览
   - `info_marketsnapshot_get_market_snapshot` → 市场快照（恐惧贪婪指数取自其中）
   - `news_feed_search_news` → 加密新闻搜索（默认近 24h、英文/中文不限）
   - `news_feed_get_social_sentiment` → 社交情绪
2. **TTL 缓存**：总览/快照(恐惧贪婪) 5 分钟，新闻/情绪 15 分钟；进程内缓存，过期后首次请求同步刷新，并发请求合并为一次上游调用（singleflight 语义）；上游失败时返回错误（fast-fail），不返回过期缓存冒充新数据——错误信息含上游错误。
3. **provider/gate 增强**：`SymbolInfo` 新增 `funding_rate` 字段（源 `ticker.FundingRate`），`/api/symbols` 响应随之携带。
4. **清算热点聚合**：后端每 60s 轮询 `GET /futures/usdt/liquidation_orders`（gateapi-go 公开接口），维护最近 30 分钟滚动窗口，按合约聚合计清算名义额（张数 × mark price 近似），进程内状态；进程重启后窗口重新积累（可接受，不做持久化）。
5. **公开 API 端点**（注册方式对齐 `/api/symbols`，`api/server.go` 公开路由表）：
   - `GET /api/market/overview` — 市场总览
   - `GET /api/market/fear-greed` — 恐惧贪婪指数（值 + 变化 + 快照时间）
   - `GET /api/market/news` — 新闻列表（top N，含情绪标签若有）
   - `GET /api/market/sentiment` — 社交情绪
   - `GET /api/market/liquidations` — 清算热点 top 20（合约、清算额、单数、窗口起点）
6. **网络与依赖**：引入官方 `github.com/modelcontextprotocol/go-sdk`（锁定具体版本写死 go.mod，不用模糊最新）；GOPROXY 走 goproxy.cn 构建。

## Constraints

- MCP 仅限免认证只读端点；不接 `/mcp/exchange`、`/mcp/dex`
- 不引入 MCP 其他工具（技术分析/链上/合规/公告）
- fast-fail：不造数据、不吞错误、不做降级假值
- 缓存实现不引入新依赖时优先手写（mutex + 过期时间戳 + singleflight 通道合并）；`golang.org/x/sync/singleflight` 若 go.sum 已有可直接用

## Acceptance Criteria

- [ ] `go build ./...`、`go test ./...` 全绿（新增包有单测：缓存 TTL/合并、清算聚合窗口、字段映射）
- [ ] 本地起临时后端，curl 五个 `/api/market/*` 端点均返回真实 Gate/MCP 数据（探针输出留档到任务目录）
- [ ] `GET /api/symbols` 响应含 `funding_rate`，抽查 BTC_USDT 等 5 个合约值非空
- [ ] 断网点（改 hosts 屏蔽 api.gatemcp.ai）复测：`/api/market/overview|fear-greed|news|sentiment` 返回 5xx + 明确错误信息，`/api/market/liquidations` 与 `/api/symbols` 不受影响（不同上游）
- [ ] go.mod 中 go-sdk 版本为锁定版本
