# PRD：策略配置收敛（⑧ strategy-config）

父任务：`11-24-gate-trading-foundation`；子任务地图第 8 步；开工前置：无。
需求权威来源：`docs/plans/2026-11-24-gate-trading-foundation.zh-CN.md`（下称计划文档）§5、§8.3、§10.2 第 8 行、D18/D19/D22/D23/D24/D25；周期枚举以计划文档 §3.3/§3.7 的 Gate 文档留档为准（本任务不新增 Gate 接口调用）。

## 背景

开仓金额从「AI 报金额 + 权益×倍数的上限封顶」改为「本金 × 用户在策略里选的百分比」（§5）。倍数字段是旧语义，必须在一次性替换中整体消失：不止 store 结构体，还包括所有读它的封顶逻辑、提示词口径、助手字段目录/诊断口径、前端展示与文档。

## 需求

1. **删除两个仓位价值倍数字段** `btc_eth_max_position_value_ratio`、`altcoin_max_position_value_ratio`（`btceth_` 驼峰别名一并消失）：
   - `store/strategy.go`：结构体字段、clamp（MinPositionRatio/MaxPositionRatio 常量）、调整提示 diff 行；
   - 消费点封顶逻辑整体删除：`kernel/engine_analysis.go`（透传参数）、`kernel/engine_position.go`（validateDecision 权益×倍数上限块）、`kernel/engine_prompt.go`（风控硬约束「单仓最大价值」与仓位大小指引的倍数例子、输出示例里的「权益×倍数」金额）、`trader/auto_trader_risk.go`（`enforcePositionValueRatio` 及其调用）、`agent/trade.go`（助手下单的 ratio 上限拒绝）；
   - 助手层：锁定字段名单与文案（`agent/tools.go`）、`removeLockedAIRiskFields` 与策略摘要（`agent/skill_management_handlers.go`）、字段标签/关键词/编辑分支（`agent/skill_execution_handlers.go`）、不开单诊断里「仓位上限 ≈ 权益×倍数」口径（同上 2810–2860 段）、`agent/skills/strategy_diagnosis.json` 与 `trader_diagnosis.json` 的 System enforced 名单、`api/server.go` 策略创建 schema 说明、`docs/architecture/STRATEGY_MODULE.md(.zh-CN.md)` 的默认值口径。
2. **新增开仓金额百分比**（D23）：`position_percent`，1–20，步长 0.1，默认 10；旧配置缺失（0 值）时按默认 10 补齐；clamp 到 [1,20]。策略摘要/提示词里以「开仓名义价值 ≈ 权益 × percent%」的中间口径表达（本金口径与代码强制金额由 ⑨ 落地）。
3. **最小下单金额**（D19/§5）：默认 12→10；所有代码兜底 12→10（`store` 默认、`auto_trader_risk.go`、`auto_trader_orders.go` 的 `minOrderNotional`、`agent/trade.go`）；从锁定字段名单移除——前端策略编辑器提供输入控件，助手可编辑（字段标签/关键词/编辑分支补齐），诊断文案「不能手动修改」口径同步纠正。
4. **周期收敛（D25）**：策略配置允许周期 = Gate 文档枚举表（`10s 1m 5m 15m 30m 1h 4h 8h 1d 7d`，计划文档 §3.3 留档）加 `1w`、`30d`；`store/strategy.go` 的 `normalizeTimeframe` 与中文别名按此收敛（丢弃 `3m/2h/6h/12h/3d`；`7d`/`30d` 为文档口径新增）；`kernel/engine_analysis.go` 旧配置兜底 `"3m"` 改为 `"5m"`；前端 `IndicatorEditor.tsx` 周期选项数组对齐；助手对 `primary_timeframe`/`selected_timeframes` 的写值随 store normalize 收敛。
5. **前端策略编辑器**：`RiskControlEditor.tsx` 倍数展示块替换为「开仓金额百分比」输入（1–20，步长 0.1，默认 10）；最小下单从 System enforced 只读展示改为输入（默认 10）；`web/src/types/strategy.ts` 同步；`web/src/i18n/strategy-translations.ts` 增删对应文案（按文件既有语言）。

## 明确不做（边界）

- kernel 硬编码最小下单（12/60）与盈亏比校验删除、`position_size_usd`/`risk_usd` 输出契约移除、引擎按本金算金额——全部是 ⑨ `sizing-engine`；⑧ 只保证倍数字段与其封顶逻辑消失、百分比配置与口径就位。
- 助手行情工具输入 `validKlineInterval` 与 `market/timeframe.go` 的周期表——⑮/⑫ 的边界；本任务只收敛**策略配置**周期。
- `docs/prompt-guide.zh-CN.md` 整篇重写——⑨ 的提示词预览门禁时统一；⑧ 不动它（kernel 12/60 兜底仍在，426/1427 行尚与实际一致）。
- 分批平仓、止损止盈管理、扫描对齐——⑩⑪⑯。

## 验收标准

- [ ] 全仓（除 `.trellis` 归档与研究留档）检索 `max_position_value_ratio|MaxPositionValueRatio|MinPositionRatio|MaxPositionRatio` 为 0 命中。
- [ ] `store` 单测：percent clamp（0→10、0.5→1、25→20、12.5 保留）、默认值（percent 10、min 10）、diff 提示含 position_percent；normalizeTimeframe 矩阵（新集合全收、旧弃值归空、别名映射）。
- [ ] kernel 决策校验不再因「权益×倍数」拒绝；`validateDecisions`/`parseFullDecisionResponse` 参数收敛后原有用例适配。
- [ ] trader/agent 单测适配：`enforcePositionValueRatio` 删除后下单路径不封顶；min 兜底 10；助手对 `min_position_size`、`position_percent` 可编辑，ratio 的锁定/标签/关键词分支消失；`config_visibility_test` 断言更新。
- [ ] 前端：编辑器无倍数展示；百分比与最小下单可输入并回写；周期选项 = D25 集合；类型检查与前端测试过。
- [ ] 门禁：`git diff --check`、`make test`、`make build`；独立检查子智能体 PASS。
- [ ] 存量策略兼容：JSON 里旧 ratio 键在反序列化时被忽略、下次保存自然消失；被弃周期归一为空后引擎走既有回落链（`timeframes[0]`→`5m`）。
