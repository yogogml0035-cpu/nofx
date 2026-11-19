# 父任务：「数据」Gate 化重建、「策略市场」全链清除与市场环境上下文注入

## Goal

一次性完成三件事，不做兼容分支、不兼容历史变量和历史操作：

1. **「数据」板块内部重建**：删除 vergex.trade/trending iframe，以 Gate 公开行情（REST v4）+ Gate MCP 免认证端点（/mcp/info、/mcp/news）重建为行情数据看板
2. **「策略市场」全链删除**：不止删 iframe 页——发布语义链（IsPublic / ConfigVisible / PublishConfig、PublishSettingsEditor、`/api/strategies/public`、agent 工具 publish 语义）与 vergex 品牌痕迹一并清除
3. **策略市场环境上下文注入**：策略配置新增市场环境勾选（市场总览 / 恐惧贪婪 / 新闻情绪），勾选后作为上下文注入决策 prompt
4. **nofxos 数据链全删**：上游 nofxos.ai 已废弃内置公共 key（HTTP 402，指路 vergex.trade / claw402.ai——均已清除的生态），AI500/OI 排名/NetFlow/价格排名数据源对本代码库已死，Gate 无 AI 注意力等价物；策略 CoinSource 收敛为纯静态币单

终态：导航为 Agent | 数据(Gate 看板) | 配置 | 看板 | 策略 | 排行榜 | FAQ（排行榜保留）；「数据」免登录可看真实 Gate 数据；策略编辑含市场环境上下文勾选组、币源为 Gate 标的静态清单；代码库无 vergex、无策略市场发布语义、无 nofxos/AI500/OI 排名/NetFlow 语义。

## 用户决策记录（2026-11-19 问答定稿）

| # | 决策点 | 用户选择 |
|---|--------|----------|
| D1 | vergex 判定 | HL 时代产物且不再维护，两个 iframe 为语义死页 |
| D2 | 「数据」终态 | 内部重建为 Gate 数据看板（REST v4 公开行情为主） |
| D3 | 看板范围 | 涨跌幅榜 + 成交额榜 + 资金费率榜 + 清算热点（REST）+ 市场总览/恐惧贪婪/新闻情绪（MCP） |
| D4 | 「策略市场」终态 | 全链彻底删除 |
| D5 | Gate MCP 边界 | **放开上任务"不接 MCP"边界，仅限 api.gatemcp.ai 的 /mcp/info、/mcp/news 免认证只读端点**；/mcp/exchange（OAuth2 交易）、/mcp/dex 仍禁止 |
| D6 | 新增需求 | 策略级市场环境上下文勾选，注入决策模型上下文 |
| D7 | Landing Initialize 跳转 | 改 `/traders`（未登录 → `/login` 不变） |
| D8 | vergex 品牌痕迹 | 全清（README 头部与链接表、六语言 i18n README、CLA.md、branding.ts、CommunitySection） |
| D9 | 交付方式 | Trellis 父子任务 |
| D10 | 「数据」页访问性 | 维持现状免登录（requiresAuth: false），数据端点全部公开 |
| D11 | 排行榜（2026-11-19 二轮追加） | **保留**：实证独立功能——handler_competition.go 与 CompetitionPage.tsx 对 nofxos/vergex/hyperliquid 零引用，数据源为本地 decision/equity/trader 统计，跨子任务回归探针保障不受影响 |
| D12 | AI500/nofxos（2026-11-19 二轮追加） | **全链删除**：实证 nofxos.ai 返回 HTTP 402「公共 key 已废弃」，上游仅提供升级 nofx release / vergex.trade / claw402.ai 三条路——全部通向已清除生态；Gate 无 AI 注意力等价数据；CoinSource 收敛 static-only |
| D13 | MCP 探针前置（2026-11-19 二轮追加） | 探针已在规划阶段完成并留档（非 C1 第一步）：/mcp/info、/mcp/news 可直连，四工具 200，go-sdk v1.8.0 可经 goproxy.cn 获取，宿主机无 Go（门禁走 Docker golang:1.25-alpine） |

## 对抗式审查结论（已按 main@10453714 代码实证校准）

1. **策略市场语义远大于 iframe**（用户答 D4 时未显式覆盖，经实证扩大范围）：`store/strategy.go:507-508` 的 `IsPublic`（注释即 "whether visible in strategy market"）与 `ConfigVisible`、`PublishStrategyConfig`；`api/strategy.go` 的 `attachPublishConfig`、`handlePublicStrategies`（`api/server.go:172` 注册 `GET /strategies/public`，注释 "for strategy market"，前端 web/src 零调用——纯服务 vergex explore 拉取）；前端 `PublishSettingsEditor.tsx` 与 StrategyStudioPage 的发布 UI；agent 层 `skill_management_handlers.go`、`central_brain.go`、`strategy_field_catalog.go`、`tools.go`、`llm_flow_extractor.go`、`skill_domain_context.go`、`skill_execution_handlers.go` 的 publish/is_public 语义。以上全部随 D4 删除。
2. **排行榜边界**：`handler_competition.go` 的 public trader list / public competition / public-config 端点服务内部 CompetitionPage（排行榜导航项），与策略市场无关，**保留**。IsPublic 命中文件清单经核对不含 competition 文件，边界清晰。
3. **D3 的 MCP 依赖成立**：Gate REST v4 无恐惧贪婪/市场总览/新闻情绪公开端点，此三项只能走 MCP；MCP 为 Streamable HTTP + SSE 协议，后端需引入官方 `github.com/modelcontextprotocol/go-sdk`（pre-1.0，锁版本），首步实测 api.gatemcp.ai 可达性。fast-fail：MCP 不可达时端点报错、前端区块级错误占位，不造假数据。
4. **清算热点语义修正**：`/futures/usdt/liquidation_orders` 公开接口只返回最近清算单快照、无时间范围参数；"热点榜"需后端定时轮询 + 滚动窗口聚合（60s 拉取、最近 30 分钟按合约名义清算额聚合，进程内维护）。
5. **D6 成本/延迟约束（默认设计）**：决策循环不得每次实时打 MCP——总览/恐惧贪婪缓存 TTL 5 分钟、新闻情绪 15 分钟，决策时读缓存；新闻注入前截断为紧凑 JSON（top 10 标题 + 情绪分）。命名用「市场环境上下文 market context」，不叫"市场行情"（与 quotes/K 线混淆）。
6. **DB schema**：strategies 表 `is_public`/`config_visible` 列随 D4 从 gorm 定义删除；本地 `.nofx-local` 数据已在 11-18 任务重置过，无存量兼容负担。新字段 `market_context` 随 StrategyConfig JSON 存储（Config 列内嵌，无表结构变更）。
7. **MCP 信息面**：本任务只用 3 个工具——`info_marketsnapshot_get_market_overview`（实测一个调用即含总览+恐惧贪婪+BTC/ETH dominance+ahr999+山寨季指数，砍掉原设计的 snapshot 单独调用）、`news_feed_search_news`、`news_feed_get_social_sentiment`。技术分析/链上/合规类工具不引入。
8. **MCP 探针已前置完成（D13）**：端点为 `https://api.gatemcp.ai/mcp/info`、`/mcp/news`（带 /mcp 前缀，非根路径）；initialize/tools/call 全 200、session 正常、上海网络直连可用；`news_feed_search_news` 通道通但 query="bitcoin" 返回空列表（total:0，参数需在实施时调优，属数据质量点非通道点，空态返回真实空列表不造假）；官方 go-sdk v1.8.0 稳定版可经 goproxy.cn 获取；**宿主机已无 Go**（~/sdk/go1.26.2 不存在、brew 无 go），后端门禁以 `docker run golang:1.25-alpine` 执行（GOPROXY=goproxy.cn）。探针脚本与原始输出留档 C1 任务 `research/`。
9. **nofxos 断供实证（D12 依据）**：`GET https://nofxos.ai/api/ai500/list`（带内置 key）返回 HTTP 402 + "The public API key has been deprecated"，上游给出的三条续用路径全部指向已清除的生态（NoFx 上游 release / vergex.trade / claw402.ai）。AI500 报错（部署环境 "AI500 榜单加载失败：Network error"）即此断供的表现。牵连面：策略币源 ai500/oi_top/oi_low、kernel OITop/NetFlow 上下文与量化指标开关（EnableQuantOI/EnableQuantNetflow）、agent get_ai500_list 工具、`/api/ai500`、前端 AI500Panel、provider/nofxos 整包。Coinank（K 线/OI 图表数据）与 nofxos 是不同上游，不受影响。
10. **排行榜独立性核验（D11 依据）**：handler_competition.go（482 行）与 CompetitionPage.tsx（486 行）对 nofxos/vergex/hyperliquid 零引用，数据全部来自本地 store（Decision 记录、Equity 快照、trader 统计）。保留，并在 C5 增加回归探针。

## 子任务地图

| 子任务 | 交付物 | 依赖 | 闭环验证 |
|--------|--------|------|----------|
| `11-19-backend-gate-market-data` | MCP provider（TTL 缓存）+ funding_rate 字段 + 清算热点聚合 + `/api/market/*` 公开端点 | 无（首个执行） | build/test 全绿；curl 真实数据探针 |
| `11-19-purge-strategy-market-vergex` | 策略市场全链删除 + vergex 品牌全清 + Landing 跳转改向 | C1 | grep 归零；build/test 全绿 |
| `11-19-purge-nofxos-data-chain` | nofxos 全链删除（AI500/OI/NetFlow/量化指标）+ CoinSource 收敛 static-only | C1 | grep 归零；build/test 全绿；排行榜/策略回归 |
| `11-19-frontend-data-dashboard` | 「数据」页重建（五类看板模块），替换 iframe | C1、C2、C2b | web build/vitest 全绿；浏览器探针截图 |
| `11-19-backend-market-context-injection` | StrategyConfig.MarketContext + kernel prompt 注入 + 策略编辑 UI 勾选组 | C1、C2、C2b | 单测；决策日志含注入段落 |
| `11-19-integration-e2e-probe` | 全门禁 + 浏览器全页探针 + 收口 | C1–C4 全部 | 本 PRD 跨子任务验收标准 |

执行顺序：C1 → C2 → C2b → C3 → C4 → C5（先删后建，C3/C4 在更干净的基础上实施）。每个子任务在自己的 worktree 通过完整门禁后合入父分支，再删除子分支与子 worktree。

## 跨子任务验收标准（父任务集成评审）

grep 词表：`vergex|strategy-market|strategy_market|strategyMarket|PublishConfig|publish_config|publishToMarket|strategies/public|handlePublicStrategies|ListPublic|ConfigVisible|config_visible`；词表二（Go 语义）：`IsPublic|"is_public"`；词表三（nofxos 数据链）：`nofxos|AI500|ai500|oi_top|oi_low|UseAI500|UseOITop|UseOILow|OITop|OILow|NetFlow|netflow|EnableQuantOI|EnableQuantNetflow|GetTopRatedCoins`。排除集：CHANGELOG 双文件、`.trellis/`、`web/node_modules/`、`.nofx-local/`、`scratch/`，`.github/` 以 `--hidden` 纳入。

- [ ] 上述 grep 在排除集外归零（词表三注意与 Coinank 的 OI 图表数据无冲突——Coinank 保留，词以 nofxos 独有符号为准，实施时校准）
- [ ] `go build ./...`、`go test ./...`、`npm --prefix web run build`、`npm --prefix web run test` 全绿（宿主机无 Go，后端门禁走 `docker run golang:1.25-alpine` + GOPROXY=goproxy.cn）
- [ ] `GET /api/symbols`（无参默认 gate）响应含 `funding_rate` 字段且非空值占比正常
- [ ] `GET /api/market/overview|fear-greed|news|sentiment|liquidations` 返回真实 Gate/MCP 数据（curl 探针；news 允许真实空列表）
- [ ] MCP 不可达时 `/api/market/*` 返回明确错误态（fast-fail 实证）
- [ ] 导航无「策略市场」；Landing Initialize（已登录）跳 `/traders`；`/strategy-market` 路由 404（无重定向兼容）
- [ ] 「数据」页免登录可看：涨跌幅/成交额/资金费率/清算热点/市场总览/恐惧贪婪/新闻情绪模块渲染，浏览器探针截图留档
- [ ] 策略编辑含市场环境上下文勾选组；勾选后决策日志 prompt 含「市场环境上下文」段落，未勾选不含
- [ ] 策略币源编辑器无 AI500/OI 选项，仅静态 Gate 标的清单；Agent 页无 AI500 面板；`GET /api/ai500` 404
- [ ] 排行榜页面功能不受影响（回归探针：公开榜渲染、参赛开关、equity 曲线）

## Non-Goals

- 不接 `/mcp/exchange`（OAuth2 交易）、`/mcp/dex`，不做 Gate 账户/交易功能
- 不引入 MCP 的技术分析、链上数据、合规检查、交易所公告工具
- 不动排行榜（CompetitionPage 与其 public 端点）
- 不动 Coinank 数据链（K 线/OI 图表数据源，与 nofxos 是不同上游）
- 不做 `/strategy-market` 重定向或任何历史路由兼容
- 不重建跨用户策略分享/市场机制
- CHANGELOG 历史记录不修改
