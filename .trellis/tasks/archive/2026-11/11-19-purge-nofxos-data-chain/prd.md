# C2b：nofxos 数据链全删（AI500/OI/NetFlow）与 CoinSource 收敛 static-only

## Goal

上游 `https://nofxos.ai/api/ai500/list` 对内置公共 key 返回 HTTP 402（"public API key has been deprecated"，续用路径全部指向已清除的 vergex/claw402 生态）——AI500/OI 排名/NetFlow/价格排名数据源对本代码库已死。一次性删除全部 nofxos 语义，策略币源收敛为纯静态 Gate 标的清单。Gate 无 AI 注意力等价数据，不迁移、不替换，直接删除。

## Requirements（删除清单）

### 后端

1. `provider/nofxos/` 整包删除（ai500/oi/netflow/price/coin/client/ai500_cache 等全部文件）
2. `api/handler_ai500.go` + `GET /api/ai500` 路由（api/server.go:208）删除；**api/server.go:325-355 策略创建帮助文案同步改写为 static-only 口径**（现宣称默认「ai500 top coins」、`coin_source.source_type` 枚举、内置 key 明文 `cm_568c67eae410d912c54c`、`enable_quant_netflow`/`netflow_ranking: ALWAYS true`——与代码真实默认值矛盾）
3. `agent/ai500.go`（AI500Board）、`get_ai500_list` 工具（tools.go 的 market 域清单与工具定义、skill_domain_context.go）
4. `agent/strategy_field_catalog.go`、`agent/skill_execution_handlers.go`、`agent/llm_flow_extractor.go` 中 ai500/oi/netflow 语义
5. `kernel/engine.go`：`OITopData`/`OIRankingData`/`PriceRankingData` 字段（:101-105/:103）、`FetchOIRankingData`（:638）/`FetchPriceRankingData`（:701）、`GetTopRatedCoins` 调用（:394）、Sources 标签注入；`engine_prompt.go` 的 OI Ranking / Price Ranking 段（:581-593）与 NetFlow Ranking 段（:586-588、:859-902 区域）；`engine_analysis.go` 的 nofxos 分析；`formatter.go` 的 nofxos 格式化（:94-98）
6. **kernel.QuantData 链**：`QuantData` 结构（engine.go:147）、`FetchQuantData`/`FetchQuantDataBatch`（:544/:617）、`ctx.QuantDataMap`/`NetFlowRankingData`/`OITopDataMap` 字段（engine.go:101-105）及其填充逻辑——market/ 包实测无 nofxos 字段，勿在 market/ 找
7. `store/strategy.go`：CoinSourceConfig 收敛——删除 `SourceType`、`UseAI500`、`UseOITop`、`UseOILow`、`AI500Limit` 与 normalize/inferCoinSourceType 的 ai500/oi_top/oi_low 分支；保留 `StaticCoins`、`ExcludedCoins`（币源语义 = 纯静态清单，清单默认取 Gate，沿用上任务 D7）
8. `store.IndicatorConfig`：**删除整个「NofxOS Unified API Configuration」段（store/strategy.go:743-765，共 13 字段）**——`NofxOSAPIKey`、`EnableQuantData`、`EnableQuantOI`、`EnableQuantNetflow`、`EnableOIRanking`/`OIRankingDuration`/`OIRankingLimit`、`EnableNetFlowRanking`/`NetFlowRankingDuration`/`NetFlowRankingLimit`、`EnablePriceRanking`/`PriceRankingDuration`/`PriceRankingLimit` 及其消费逻辑（trader/auto_trader_loop.go:638 的 EnableQuantData 分支等）
9. **trader 级币源链（与策略 CoinSource 是两条独立链）**：`store/trader.go:40-41/122-123` 的 `UseAI500`/`UseOITop`（gorm 列 use_coin_pool/use_oi_top）与输出映射、`api/handler_trader.go:39-40/494-495` DTO、`api/handler_order.go:102-103` 输出、`manager/trader_manager.go:599/634` 的 ai500/oiTop URL 构建逻辑；AutoMigrate 删字段后 traders 表旧列残留无害（本地库已重置）
10. `api/strategy.go:26-30` 的 NofxOS key 校验删除
11. 相关测试同步删除/改写：strategy_source_type_test.go（SourceType 收敛后仅剩 static 语义）、ai500_cache_test.go 随包删除、kernel 相关用例

### 前端

12. `components/agent/AI500Panel.tsx` 删除；`pages/AgentChatPage.tsx` 的 AI500Panel import（:23-24）、右侧面板「AI500 精选」tab（:469）、`analyzeAI500Symbol` 与相关 prompt 文案（:584-589）删除
13. `web/src/lib/api/data.ts`：`AI500Coin`/`AI500ListResponse`/`getAI500List`（:32-57）删除
14. `components/strategy/CoinSourceEditor.tsx`：删除 ai500/oi_top/oi_low 选项，收敛为 Gate 标的静态多选（含排除清单，语义不变）
15. `components/strategy/IndicatorEditor.tsx`：`nofxos_api_key` 输入、`enable_quant_netflow`、`enable_netflow_ranking` 等量化开关 UI（:54-59）删除
16. `components/trader/TraderConfigModal.tsx:339-341` 的 source_type 三分支展示收敛
17. `types/strategy.ts`、`types/trading.ts`、`types/config.ts` 中 AI500/OI/NetFlow 类型与字段
18. `components/landing/brand/AgentTerminal.tsx:86` 的装饰性「Netflow」标签改中性词（如 Flow）——与 nofxos 无关的词表假阳性，按词表归零口径处理
19. i18n：AI500/OI 币源/净流入相关键删除（translations.ts 三语言 en/zh/id 与 strategy-translations.ts）
20. WelcomeScreen.tsx / AI 相关引导文案中的 AI500 提及

### 文档

21. 功能性文档中 nofxos/AI500/OI/NetFlow 段落随代码删除或改写为 static-only 口径：`CONTRIBUTING.md` + `docs/i18n/zh-CN/CONTRIBUTING.md`、`docs/prompt-guide{,.zh-CN}.md`、`docs/token-estimation.zh-CN.md`、`docs/api/API_REFERENCE.md`、`docs/guides/README{,.zh-CN}.md`、`docs/architecture/README{,.zh-CN}.md`、`docs/architecture/STRATEGY_MODULE{,.zh-CN}.md`（:104 GetTopRatedCoins 示例代码）、`docs/roadmap/README{,.zh-CN}.md`、`docs/community/OFFICIAL_ACCOUNTS.md`

### 明确保留（不得误删）

- **Coinank 数据链**（provider/coinank：K 线、OI 图表、清算数据，服务 `/api/klines` 等）——与 nofxos 是不同上游、仍在服务；词表三与 Coinank 符号零冲突（已实证）
- **排行榜**（CompetitionPage + handler_competition.go）——已实证零 nofxos 依赖
- 策略静态币单、排除清单、指标中的 K 线/技术指标（非量化 OI/Netflow）

## Constraints

- CoinSource 收敛后 `source_type` 字段整体消失：策略 config 不再有"币源类型"概念，币源就是静态清单（含排除）——不做"只剩 static 一种取值"的枚举残留
- 存量策略 config 中的 ai500/oi 字段随 JSON 解析自然失效，无迁移（本地库已在 11-18 重置；解析端直接删除字段定义，未知字段忽略是 gorm/JSON 默认行为，非兼容代码）
- fast-fail 语义不变：不造空榜、不留假面板

## Acceptance Criteria

- [ ] `rg -il "nofxos|AI500|ai500|oi_top|oi_low|UseAI500|UseOITop|UseOILow|OITop|OILow|NetFlow|netflow|EnableQuantOI|EnableQuantNetflow|EnableQuantData|OIRanking|PriceRanking|quant_data|oi_ranking|price_ranking|GetTopRatedCoins" --hidden` 排除集（CHANGELOG×2、.trellis、web/node_modules、.nofx-local、scratch、docs/legal、docs/plans、screenshots）外归零；与 Coinank 符号冲突处以 nofxos 独有词校准
- [ ] `go build ./...`、`go test ./...`（Docker golang:1.25-alpine + GOPROXY）、`npm --prefix web run build`、`npm --prefix web run test` 全绿
- [ ] 运行时探针：`GET /api/ai500` 404；Agent 页无 AI500 面板；策略编辑器币源仅静态 Gate 清单；创建 static 策略 → 决策循环正常选币交易（回归）
- [ ] 排行榜回归探针通过（不受本任务影响）
- [ ] `/api/klines` 正常返回（Coinank 保留实证）
