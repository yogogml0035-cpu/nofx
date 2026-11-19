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
| D10 | 「数据」页访问性 | 维持现状免登录（requiresAuth: false），数据端点全部公开（助手默认设计，评审未否决） |
| D11 | 排行榜（2026-11-19 二轮追加） | **保留**：实证独立功能——handler_competition.go 与 CompetitionPage.tsx 对 nofxos/vergex/hyperliquid 零引用，数据源为本地 decision/equity/trader 统计，跨子任务回归探针保障不受影响 |
| D12 | AI500/nofxos（2026-11-19 二轮追加） | **全链删除**：实证 nofxos.ai 返回 HTTP 402「公共 key 已废弃」，上游仅提供升级 nofx release / vergex.trade / claw402.ai 三条路——全部通向已清除生态；Gate 无 AI 注意力等价数据；CoinSource 收敛 static-only |
| D13 | MCP 探针前置（2026-11-19 二轮追加） | 探针已在规划阶段完成并留档（非 C1 第一步）：/mcp/info、/mcp/news 可直连、上海网络直连 200、go-sdk v1.8.0 可经 goproxy.cn 获取，宿主机无 Go（门禁走 Docker golang:1.25-alpine） |

## 对抗式审查结论（已按 main@10453714 代码实证校准；三轮多智能体复审后修订）

1. **策略市场语义远大于 iframe**（用户答 D4 时未显式覆盖，经实证扩大范围）：`store/strategy.go` 的 `IsPublic`（注释即 "whether visible in strategy market"）与 `ConfigVisible`、`PublishStrategyConfig`；`api/strategy.go` 的 `attachPublishConfig`、`handlePublicStrategies`（api/server.go 注册 `GET /strategies/public`，前端 web/src 零调用——纯服务 vergex explore 拉取）；前端 `PublishSettingsEditor.tsx` 与 StrategyStudioPage 的发布 UI；agent 层 7 个 .go 文件的 publish/is_public 语义 + `skills/strategy_management.json` + `telegram/agent/apicall.go` 白名单。以上全部随 D4 删除（完整清单见 C2 PRD）。
2. **排行榜边界**：`handler_competition.go` 的 public 端点服务内部 CompetitionPage（排行榜导航项），与策略市场无关，**保留**；词表二（IsPublic）在 Go 域对排行榜/visibility/AI model 零假阳性（实证）。
3. **D3 的 MCP 依赖成立**：Gate REST v4 无恐惧贪婪/市场总览/新闻情绪公开端点，此三项只能走 MCP；MCP 为 Streamable HTTP + SSE 协议，后端引入官方 `github.com/modelcontextprotocol/go-sdk`（锁定 v1.8.0 稳定版）。
4. **清算热点语义**：`GET /futures/usdt/liq_orders`（gateapi-go `ListLiquidatedOrders`，公开免鉴权）支持 from/to 时间窗（≤3600s）与 limit 分页，但**无唯一单号 ID、历史深度有限**——热点榜由后端 60s 轮询（From=上次水位−60s 重叠）+ 30 分钟滚动窗口聚合，语义为热点相对排名（极端行情下有界漏计），不是清算总量审计值。
5. **D6 成本/延迟约束（默认设计）**：决策循环不得每次实时打 MCP——总览/恐惧贪婪缓存 5 分钟、新闻/情绪 15 分钟，决策时读缓存；注入段软上限 600 token（口径与 EstimateTokens 一致：中文 len/2、英文 len/4），超限截断标注。命名用「市场环境上下文 market context」。
6. **DB schema**：strategies 表 `is_public`/`config_visible` 列与 traders 表 `use_coin_pool`/`use_oi_top` 列随删除从 gorm 定义移除；本地 `.nofx-local` 数据已在 11-18 任务重置过，无存量兼容负担，旧列残留无害。新配置 `market_context` 随 `ai_config` JSON 内嵌存储（**必须接进 StrategyConfig 自定义 Marshal/Unmarshal，否则静默丢失**，无表结构变更，见 C4）。
7. **MCP 信息面（三轮探针定稿，留档 C1 research/ 四份）**：采用 4 个工具——`info_marketsnapshot_get_market_overview`（一个调用含总览+恐惧贪婪+dominance+ahr999+山寨季；**字段全集即探针实证，无板块数据、无逐币 24h 涨跌**）、`news_feed_search_news`（**仅传 coin 不传 query**：query 任何取值全矩阵 total:0，coin-only 实测 5/5 真实条目）、`news_feed_get_exchange_announcements`（市场级标题补充流，实测 5/5）、`news_feed_get_social_sentiment`（coin 必传，默认 BTC）。/mcp/news 共 17 工具，events/ugc 实测空、web_search/search_x 为 LLM 合成类不采用。
8. **恐惧贪婪 24h 变化为后端派生**：上游无变化字段；overview 缓存每次真实刷新追加样本进进程内历史环（保留 25h），change_24h 与 now−24h 最近样本作差；冷启动不足 24h 或重启后为 null，不编造。
9. **nofxos 断供实证（D12 依据）**：`GET https://nofxos.ai/api/ai500/list`（带内置 key）返回 HTTP 402 + "The public API key has been deprecated"，续用路径全部指向已清除的生态。牵连面（完整清单见 C2b PRD）：provider/nofxos 整包、策略 CoinSource 三类型 + **trader 级 UseAI500/UseOITop 独立链**、kernel OI/Price/NetFlow Ranking 注入段与 QuantData 链、IndicatorConfig 整个 NofxOS 配置段（13 字段）、agent get_ai500_list、`/api/ai500` 与 server 帮助文案（含内置 key 明文）、前端 AI500Panel/CoinSourceEditor/IndicatorEditor/TraderConfigModal、功能性文档面。Coinank（K 线/OI 图表数据）与 nofxos 是不同上游，不受影响。
10. **排行榜独立性核验（D11 依据）**：handler_competition.go 与 CompetitionPage.tsx 对 nofxos/vergex/hyperliquid 零引用，数据全部来自本地 store。保留，C5 回归探针。

## 子任务地图

| 顺序 | 子任务 | 交付物 | 顺序依据 | 闭环验证 |
|---|--------|--------|------|----------|
| C1 | `11-19-backend-gate-market-data` | MCP provider（TTL 缓存+恐惧贪婪历史环）+ funding_rate + symbols 缓存 + 清算热点聚合 + `/api/market/*` 公开端点 | 无依赖（首个执行） | build/test 全绿；curl 真实数据探针 + 断网点 502 |
| C2 | `11-19-purge-strategy-market-vergex` | 策略市场全链删除 + vergex 品牌全清 + DataPage 置空态占位 + Landing 跳转改向 | 无功能依赖（与 C1 仅 api/server.go 相邻区域，串行避免冲突） | grep 归零；全门禁；浏览器探针 |
| C2b | `11-19-purge-nofxos-data-chain` | nofxos 全链删除（AI500/OI/NetFlow/Ranking/量化段）+ CoinSource 收敛 static-only | 无功能依赖（串行） | grep 归零；全门禁；排行榜/策略/klines 回归 |
| C3 | `11-19-frontend-data-dashboard` | 「数据」页重建（七模块：四类 REST 榜单 + 三类 MCP 环境模块） | C1（端点与 funding_rate）、C2（占位壳） | web build/vitest 全绿；浏览器探针截图（桌面+移动+断链分组） |
| C4 | `11-19-backend-market-context-injection` | ai_config.market_context + kernel prompt 注入 + 策略编辑 UI 勾选组 | C1（取数函数）、C2（发布语义已清）、C2b（engine_prompt.go 先删后加） | 单测（含序列化往返）；决策日志含注入段实证 |
| C5 | `11-19-integration-e2e-probe` | 全门禁 + 三条词表 + 浏览器全页探针 + 收口 | C1–C4 全部 | 本 PRD 跨子任务验收标准 |

执行顺序：C1（先建数据底座）→ C2/C2b（清除旧语义）→ C3/C4（在干净基础上重建）→ C5（集成收口）。每个子任务在自己的 worktree 通过完整门禁后合入父分支，再删除子分支与子 worktree；C5 阶段发现的缺陷直接在父分支修复（见 C5 PRD）。执行顺序的单一权威记录在父任务 implement.md。

## 跨子任务验收标准（父任务集成评审）

grep 词表（排除集：CHANGELOG 双文件、`.trellis/`、`web/node_modules/`、`.nofx-local/`、`scratch/`、`docs/legal/`、`docs/plans/`、`screenshots/`；`.github/` 以 `--hidden` 纳入）：词表一 `vergex|strategy-market|strategy_market|strategyMarket|PublishConfig|publish_config|publishToMarket|strategies/public|handlePublicStrategies|ListPublic|ConfigVisible|config_visible`；词表二（Go 语义）`IsPublic|"is_public"`；词表三（nofxos 数据链）`nofxos|AI500|ai500|oi_top|oi_low|UseAI500|UseOITop|UseOILow|OITop|OILow|NetFlow|netflow|EnableQuantOI|EnableQuantNetflow|EnableQuantData|OIRanking|PriceRanking|quant_data|oi_ranking|price_ranking|GetTopRatedCoins`。各子任务 PRD 词表为准，本表为超集复跑。

- [x] 三条词表在排除集外归零（词表三与 Coinank 符号零冲突已实证）
- [x] `go build ./...`、`go test ./...`（Docker golang:1.25-alpine + GOPROXY=goproxy.cn）、`npm --prefix web run build`、`npm --prefix web run test` 全绿
- [x] `GET /api/symbols`（无参默认 gate）响应含 `funding_rate` 且抽查 BTC_USDT 等 5 个合约非空（Gate 原始小数语义）
- [x] 六个 `/api/market/overview|fear-greed|news|announcements|sentiment|liquidations` 返回真实 Gate/MCP 数据（curl 探针；news/announcements 允许真实空列表；fear-greed 的 change_24h 冷启动可为 null）
- [x] MCP 不可达（`GATEMCP_ENDPOINT=http://127.0.0.1:9`）时 `/api/market/*` 返回 502 + 明确错误类别（fast-fail 实证）
- [x] 导航无「策略市场」；Landing Initialize（已登录）跳 `/traders`；`/strategy-market` 路由 404（无重定向兼容）
- [x] 「数据」页免登录可看：涨跌幅/成交额/资金费率/清算热点/市场总览/恐惧贪婪/新闻情绪七模块渲染，浏览器探针截图留档
- [x] 策略编辑含市场环境上下文勾选组；勾选后决策日志 prompt 含「市场环境上下文」段落，未勾选不含；market_context 序列化往返单测通过
- [x] 策略币源编辑器无 AI500/OI 选项，仅静态 Gate 标的清单；Agent 页无 AI500 面板；`GET /api/ai500` 404
- [x] 排行榜页面功能不受影响（回归探针：公开榜渲染、参赛开关、equity 曲线）
- [x] `/api/klines` 正常返回（Coinank 保留实证）

## Non-Goals

- 不接 `/mcp/exchange`（OAuth2 交易）、`/mcp/dex`，不做 Gate 账户/交易功能
- 不引入 MCP 的技术分析、链上数据、合规检查、LLM 合成搜索类工具（web_search/search_x）
- 不动排行榜（CompetitionPage 与其 public 端点）
- 不动 Coinank 数据链（K 线/OI 图表数据源，与 nofxos 是不同上游）
- 不做 `/strategy-market` 重定向或任何历史路由兼容
- 不重建跨用户策略分享/市场机制
- CHANGELOG 双文件、docs/legal/（对外维权报告）、docs/plans/（历史规划快照）历史记录不修改
