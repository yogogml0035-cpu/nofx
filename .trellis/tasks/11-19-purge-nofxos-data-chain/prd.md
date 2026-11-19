# C2b：nofxos 数据链全删（AI500/OI/NetFlow）与 CoinSource 收敛 static-only

## Goal

上游 `https://nofxos.ai/api/ai500/list` 对内置公共 key 返回 HTTP 402（"public API key has been deprecated"，续用路径全部指向已清除的 vergex/claw402 生态）——AI500/OI 排名/NetFlow/价格排名数据源对本代码库已死。一次性删除全部 nofxos 语义，策略币源收敛为纯静态 Gate 标的清单。Gate 无 AI 注意力等价数据，不迁移、不替换，直接删除。

## Requirements（删除清单）

### 后端

1. `provider/nofxos/` 整包删除（ai500/oi/netflow/price/coin/client/ai500_cache 等全部文件）
2. `api/handler_ai500.go` + `GET /api/ai500` 路由删除
3. `agent/ai500.go`（AI500Board）、`get_ai500_list` 工具（tools.go 的 market 域清单与工具定义、skill_domain_context.go）
4. `agent/strategy_field_catalog.go`、`agent/skill_execution_handlers.go`、`agent/llm_flow_extractor.go` 中 ai500/oi/netflow 语义
5. `kernel/`：engine.go 的 OITopData/coin Sources 标签注入、engine_prompt.go 的 NetFlow Ranking 段与 Netflow 指标格式化（:586-588、:859-902 区域）、engine_analysis.go 的 nofxos 分析、formatter.go 的 nofxos 格式化（:94-98）
6. `store/strategy.go`：CoinSourceConfig 收敛——删除 `SourceType`、`UseAI500`、`UseOITop`、`UseOILow`、`AI500Limit` 与 normalize/inferCoinSourceType 的 ai500/oi_top/oi_low 分支；保留 `StaticCoins`、`ExcludedCoins`（币源语义 = 纯静态清单，清单默认取 Gate，沿用上任务 D7）
7. `store.IndicatorConfig`：删除 `EnableQuantOI`、`EnableQuantNetflow` 及其消费逻辑
8. `market/` 包：market.Data 的 Netflow/OITop 字段与填充逻辑（grep netflow/OITop 定位）
9. `trader/`：决策循环中 AI500/OI 币源解析调用点（auto_trader*.go 中 UseAI500/UseOITop/UseOILow 分支）
10. 相关测试同步删除/改写：strategy_source_type_test.go（SourceType 枚举收敛后仅剩 static 语义）、ai500_cache_test.go 随包删除、kernel 相关用例

### 前端

11. `components/agent/AI500Panel.tsx` 删除；Agent 页右侧面板「AI500 精选」tab 及其加载失败态删除
12. `components/strategy/CoinSourceEditor.tsx`：删除 ai500/oi_top/oi_low 选项，收敛为 Gate 标的静态多选（含排除清单，语义不变）
13. `types/strategy.ts`、`types/trading.ts`、`types/config.ts` 中 AI500/OI/NetFlow 类型与字段
14. i18n：AI500/OI 币源/净流入相关键删除（含 translations.ts 与 strategy-translations.ts）
15. WelcomeScreen.tsx / AI 相关引导文案中的 AI500 提及

### 明确保留（不得误删）

- **Coinank 数据链**（provider/coinank：K 线、OI 图表、清算数据，服务 `/api/klines` 等）——与 nofxos 是不同上游、仍在服务
- **排行榜**（CompetitionPage + handler_competition.go）——已实证零 nofxos 依赖
- 策略静态币单、排除清单、指标中的 K 线/技术指标（非量化 OI/Netflow）

## Constraints

- CoinSource 收敛后 `source_type` 字段整体消失：策略 config 不再有"币源类型"概念，币源就是静态清单（含排除）——不做"只剩 static 一种取值"的枚举残留
- 存量策略 config 中的 ai500/oi 字段随 JSON 解析自然失效，无迁移（本地库已在 11-18 重置；解析端直接删除字段定义，未知字段忽略是 gorm/JSON 默认行为，非兼容代码）
- fast-fail 语义不变：不造空榜、不留假面板

## Acceptance Criteria

- [ ] `rg -il "nofxos|AI500|ai500|oi_top|oi_low|UseAI500|UseOITop|UseOILow|OITop|OILow|NetFlow|netflow|EnableQuantOI|EnableQuantNetflow" --hidden` 排除集（CHANGELOG×2、.trellis、web/node_modules、.nofx-local、scratch）外归零；与 Coinank 符号冲突处以 nofxos 独有词校准
- [ ] `go build ./...`、`go test ./...`、`npm --prefix web run build`、`npm --prefix web run test` 全绿（Docker golang:1.25-alpine + GOPROXY）
- [ ] 运行时探针：`GET /api/ai500` 404；Agent 页无 AI500 面板；策略编辑器币源仅静态 Gate 清单；创建 static 策略 → 决策循环正常选币交易（回归）
- [ ] 排行榜回归探针通过（不受本任务影响）
- [ ] `/api/klines` 正常返回（Coinank 保留实证）
