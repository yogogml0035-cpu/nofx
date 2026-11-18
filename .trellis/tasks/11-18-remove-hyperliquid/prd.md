# 父任务：彻底移除 Hyperliquid 交易所与 claw402 钱包体系

## Goal

一次性删除两套体系的全部语义，不做兼容分支、不兼容历史变量和历史操作：

1. **Hyperliquid 交易所**：后端、前端、文档、营销层的全部逻辑与痕迹
2. **claw402 钱包体系**：claw402 / blockrun 钱包付费模型、x402 支付轨道、钱包创建/二维码绑定/余额查询、新手 onboarding API
3. **新手/老手模式**：模式选择器、新手页、模式分支全部删除，登录后统一进入标准（原老手）界面

终态：剩余 9 个交易所平权（binance / bybit / okx / bitget / gate / kucoin / aster / lighter / indodax，统一 API Key 凭证模式，只做合约）；模型层收敛为纯 API Key provider（DeepSeek / OpenAI / Claude / Gemini / Qwen / Kimi / Grok / MiniMax 等）；本地数据重置仅保留登录账号；营销推荐位仅保留 Gate；以 Gate 子账户完成端到端验收。

## 用户决策记录（2026-11-18 问答定稿）

| # | 决策点 | 用户选择 |
|---|--------|----------|
| D1 | Gate 定位 | **纯删除**：Gate 不做增强、不接 Gate MCP、不补现货；剩余 9 所平权，产品只做合约 |
| D2 | hyperliquid-xyz 标的板 | **随 Hyperliquid 整体删除**，标的目录回到纯 crypto；多市场能力后续另立任务重建 |
| D3 | 存量数据 | **本地数据整体重置，仅保留登录账号**（users 表），其余配置/策略/trader/模型全部清空重建 |
| D4 | 文档营销边界 | 清除全部 Hyperliquid 痕迹，**仅 CHANGELOG 保留历史记录**；**其他所有交易所 referral 链接全部删除（含前端落地页/Footer 推荐位），推荐位只保留 Gate** |
| D5 | 验证深度 | **端到端验证**：用户已提供 Gate 子账户 API Key/Secret（仅在 UI 配置与联调时使用，禁止写入任何文件/文档/commit，展示时脱敏） |
| D6 | claw402 与模式体系 | **全部删除**：claw402 钱包相关体系的 Agent 调用、新手模式弹窗二维码绑定 claw402 钱包的逻辑、新手和老手模式体系本身；**默认即老手（标准）模式** |
| D7 | 默认数据源（2026-11-19 追加） | **Gate 为默认交易所与默认行情数据源**：`/api/symbols` 与 `/api/klines` 无参默认 exchange=gate、前端无交易所上下文时默认市场 gate、策略 static 币源清单默认取 Gate 标的；9 所功能平权不变（仅默认值与推荐位偏向 Gate）；实现走 Gate API v4 REST 公开行情接口（免鉴权、单接口 200r/10s/IP），不接 Gate MCP |

## 对抗式审查结论（已按 main@3b6831eb 代码实证校准）

1. **gate-for-ai 文档无实施对象**：D1=A 下不接 MCP、不增强 Gate，本任务实质是两套体系的完全清除，Gate 文档仅为背景上下文。
2. **代码平权与营销仅留 Gate 并存**：代码层 9 所无特殊分支；README/落地页/Footer 推荐位只剩 Gate。两层决定，非矛盾。D4 的前端半边（FooterSection 推荐位、FeaturesSection、登录页文案）由 C3 执行。
3. **README 产品叙事收缩**：xyz 板删除后执行层只剩 crypto 永续（research 侧 search_stock / AI500 数据仍在），tagline 收缩为 crypto 永续核心表述。
4. **AI500 存续**：`provider/nofxos/client_resolve.go:19-21` 无钱包 key 时直连 `nofxos.ai`（`client.go:19` DefaultAuthKey 硬编码），已验证不依赖 claw402 任何代码。删除 claw402 路由后直连成为唯一路径，AI500 面板不受影响。
5. **blockrun-base / blockrun-sol 随 D6 删除**：二者仅存在于目录/枚举/校验（`agent/model_provider_catalog.go`、`config_validation.go`、`handler_ai_model.go`），无独立运行时轨道；D6 语义是钱包付费 provider 体系整体消失，保留任何一个都是残留。
6. **AICharge 定位**：`store/ai_charge.go` 的 ai_charges 表与通用成本统计（全模型适用）**保留**；同文件的 `IsClaw402Config`（:152）与 `EstimateRunway`（:157，USDC runway）是 claw402 专属函数，**随 C2 删除**（调用点：`trader/auto_trader.go:650,675`、`trader/auto_trader_loop.go:34,745`）。
7. **mcp / wallet 包的定位**：`mcp/` 是全项目通用 LLM client 库（38 个消费文件，含全部 API Key provider 的调用路径），**整包保留**，仅删 `mcp/payment/` 子包与 claw402 残留；`wallet/`（usdc.go + balance_cache.go）7 个消费方全部在 C2 清单内，**整包删除**。
8. **聊天 setup 流保留**：`agent/onboard.go` 实证无 beginner/wallet/claw402 分支，是通用首次配置路径（选交易所→选模型→建 trader）；仅删其中的 hyperliquid 分支（C1）。
9. **AIModel 无钱包列**：`store/ai_model.go` 的 AIModel 结构与 ai_models 表从未有 WalletAddress/BalanceUSDC 列（余额是运行时查询）；这两个字段是 DTO（`agent/tools.go:993`、`api/handler_ai_model.go:37-38`）。store 层删除目标是 `ResolveClaw402WalletKey` / `FindOrphanClaw402` / `AdoptModel` 与"recover wallets after account reset"机制。
10. **数据重置代价与工具**：用户现有全部配置清空，验证阶段以 D5 凭据重建 Gate 配置（用户已接受）。**禁止用 `nofx reset-account` 代替手工 drop**：它会连 users 一起删（违反 D3）、漏清 12 张表、且不清 exchanges 的 hyperliquid 旧列（`cli.go:169-178`）。
11. **凭据安全**：Gate 子账户 key/secret 不得出现在任何代码、文档、日志、commit 中；建议用户验收后在 Gate 后台轮换。
12. **SourceType 枚举收窄（2026-11-19 定稿）**：策略 CoinSource 的 `hyper_all / hyper_main / hyper_rank` 随 Hyperliquid 删除；`normalize` / `inferCoinSourceType` 的 default 分支重定向到 `static`；`GetDefaultStrategyConfig` 默认 SourceType 改为 `static`，RoleDefinition 换通用 crypto 文案。static 是"用户手工从标的清单选币"的币源类型（其余保留的 ai500 / oi_top / oi_low 为程序化币源），其标的清单默认从 Gate 拉取（D7）。
13. **预置美股策略删除（2026-11-19 用户确认）**：`api/handler_user.go` `createDefaultStrategies` 为新用户创建的 3 个美股 USDC 预置策略（hyper_rank 来源）是 xyz 板产品功能，随 D2 删除；新用户初始无预置策略，建策略时走 `GetDefaultStrategyConfig`（static 默认）。
14. **/api/symbols 以 Gate 重接（2026-11-19 定稿，D7）**：现状该端点仅实现 hyperliquid / hyperliquid-xyz / xyz 三个 case（`handler_klines.go:472-532`，default 400），删除后即为空壳——新增 `case "gate"`：新建 `provider/gate/` 以 gateapi-go 公开客户端（无需 API key）拉取 `GET /futures/usdt/contracts` + `GET /futures/usdt/tickers`，映射为 SymbolInfo（contract→symbol、last/mark_price→MarkPrice、volume_24h_settle→Volume24h、24h 涨跌幅→Change24hPct、leverage_max→MaxLeverage、order_size_min→SzDecimals；过滤 in_delisting；字段名以 SDK FuturesContract/FuturesTicker 为准）；`DefaultQuery` 默认值 :453 改 `gate`。默认值全局统一：`/api/klines` 默认（:33，现 binance）、前端 ChartTabs :49（现 hyperliquid）与 TraderDashboardPage :60（现 binance）一并收敛为 `gate`。

## 子任务地图

| 子任务 | 交付物 | 依赖 | 闭环验证 |
|--------|--------|------|----------|
| `11-18-backend-remove-hyperliquid` | 后端 Hyperliquid 全删 + /api/symbols 以 Gate 重接（D7） | 无（首个执行） | `go build ./...` + `go test ./...` 全绿；后端 `rg -i "hyperliquid\|hyper_all\|hyper_main\|hyper_rank"` 归零；`/api/symbols?exchange=gate` 返回非空清单 |
| `11-18-backend-remove-claw402-mode` | 后端 claw402/blockrun/新手模式全删 | C1（共享 agent/api 文件，顺序编辑） | build/test 全绿；`rg -i "claw402\|blockrun"` Go 代码归零；模型目录仅 API Key |
| `11-18-frontend-purge` | 前端两套体系与模式体系全删 | C1+C2（运行时验证需新后端） | web 构建+vitest 全绿；前端 grep 归零；浏览器探针截图 |
| `11-18-docs-marketing-cleanup` | 文档营销清理（推荐位仅 Gate） | C3（弱依赖，可与 C3 并行，须在 C5 前完成） | docs grep 归零（CHANGELOG 除外）；README 渲染检查 |
| `11-18-integration-reset-e2e` | 数据重置 + 镜像重建 + Gate e2e + 截图总验收 | C1–C4 全部 | 本 PRD 跨子任务验收标准 |

依赖关系已写入各子任务 prd.md；父任务只做最终集成评审，不承担直接实现。C1/C2 的闭环为静态级（build/test/grep），系统整体可运行性由 C3（go run 临时后端）与 C5（终态镜像）验证。

## 跨子任务验收标准（父任务集成评审）

全部 grep 词表统一为 `hyperliquid|claw402|blockrun|x402|hyper_all|hyper_main|hyper_rank`；排除集统一为 CHANGELOG 双文件、`.trellis/`、`web/node_modules/`、`.nofx-local/`、`scratch/`（rg 默认尊重 .gitignore/.git/info/exclude，排除项作冗余保险）；`.github/` 是隐藏目录，须以 `--hidden` 显式纳入。

- [ ] `rg -il "hyperliquid|claw402|blockrun|x402|hyper_all|hyper_main|hyper_rank" --hidden --glob '!CHANGELOG.md' --glob '!CHANGELOG.zh-CN.md' --glob '!.trellis/**' --glob '!.git/**' --glob '!web/node_modules/**' --glob '!.nofx-local/**' --glob '!scratch/**' .` 归零
- [ ] `go build ./...`、`go test ./...`、`npm --prefix web run build`、`npm --prefix web run test` 全绿
- [ ] UI 交易所列表 9 所、统一 API Key 模式、无钱包连接入口；落地页/Footer 推荐位仅 Gate
- [ ] 模型配置仅 API Key provider，无钱包创建/二维码充值/余额展示
- [ ] 登录后直接进入 `/traders`，无模式选择器与 `/welcome` 新手页
- [ ] AI500 面板（nofxos 直连）正常渲染
- [ ] `.nofx-local` 重置后旧账密可登录，其余数据为空态；重建表无 hyperliquid 三列
- [ ] 终态镜像运行时探针：`GET /api/symbols`（无参）默认返回 Gate USDT 永续标的清单（非空、无 hyperliquid）、`GET /hyperliquid/account` 返回 404、前端页面无 Hyperliquid 字样（证明部署物即终态代码）
- [ ] Gate e2e：余额（以 available 为基准）、持仓正确，最小下单→成交→平仓链路走通，前后 available 变动与手续费对账一致
- [ ] 用户 Gate 凭据零落盘（`-F -uuu` 固定串全仓搜索无命中）

## Non-Goals

- 不接入 Gate MCP（api.gatemcp.ai），不增强 Gate，不新增现货
- 不重建多市场标的板（美股/大宗/FX/Pre-IPO），后续另立任务
- 不删除 AI500（nofxos 直连数据源）、search_stock（alpaca/twelvedata/vergex）、ai_charges 表与通用成本统计、聊天 setup 流、`mcp/` 通用 LLM client 库
- 不做任何兼容迁移或历史变量兼容
- CHANGELOG 历史记录不修改
