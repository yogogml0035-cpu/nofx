# 设计：⑧ strategy-config

## 语义基线（计划文档 §5）

- 开仓名义价值 = 本金 × `position_percent`（1%–20%，默认 10%）。⑧ 落地**配置与口径**；⑨ 落地**代码计算与强制**。
- 最小下单金额 = 策略配置项，默认 10 USDT，用户可改（D19）。
- 周期选项 = Gate 文档枚举（§3.3 留档：`10s 1m 5m 15m 30m 1h 4h 8h 1d 7d`）+ `1w` + `30d`（D25）。
- 倍数字段整体消失：字段、clamp、diff、所有封顶消费、所有文案口径。

## 字段与常量（store/strategy.go）

```go
MinPositionPercent = 1.0
MaxPositionPercent = 20.0
DefaultPositionPercent = 10.0
```

`RiskControlConfig` 删除 `BTCETHMaxPositionValueRatio`/`AltcoinMaxPositionValueRatio`，新增：

```go
// 开仓名义价值 = 本金 × 该百分比（1–20，默认 10；D23，引擎金额计算在 ⑨ 落地）
PositionPercent float64 `json:"position_percent"`
```

ClampLimits：删 ratio 段；`PositionPercent <= 0 → 10`（兼容存量 JSON 无此键），再夹 [1,20]。默认配置 `MinPositionSize: 10`、`PositionPercent: 10`。diff 提示：删两行 ratio，加 `appendFloat("开仓金额百分比","position_percent",…)`。删除常量 `MinPositionRatio`/`MaxPositionRatio`（全仓唯一消费者是本文件 clamp）。

normalizeTimeframe：allowed = {10s,1m,5m,15m,30m,1h,4h,8h,1d,7d,1w,30d}；别名表删「3分钟/2小时/6小时/12小时/3天」，加「10秒→10s、7天→7d、30天/1个月→30d」（「1周」保持→1w）。弃值行为不变：返回 `""`，由消费侧既有回落处理。

## 消费点（旧语义拔除清单）

| 文件 | 处理 |
|---|---|
| `kernel/engine_position.go` | `validateDecisions`/`validateDecision` 删 `btcEthPosRatio/altcoinPosRatio` 参数与「权益×倍数」上限块（含 tolerance）；保留杠杆回落、硬编码 12/60 下限与盈亏比校验（⑨ 删）|
| `kernel/engine_analysis.go` | `parseFullDecisionResponse` 同步删参；调用处删两行透传；旧配置兜底 `"3m"`→`"5m"` |
| `kernel/engine_prompt.go` | 删 ratio 取值与默认 5/1 兜底；`writeHardConstraints` 删参，「单仓最大价值」两分支合并为一条 `- 开仓名义价值: 权益 %.0f × %.1f%% ≈ %.0f USDT`（本金口径 ⑨ 换）；「仓位大小指引」改为围绕该金额：置信度不足则减仓或等待、示例行用 percent；`writeOutputFormat` 示例 `size`/`examplePositionSize` = `accountEquity × PositionPercent/100` |
| `trader/auto_trader_risk.go` | 删 `enforcePositionValueRatio` 与 `isMajorAsset`（唯一消费者即它）；`enforceMinPositionSize` 兜底 12→10 |
| `trader/auto_trader_orders.go` | 删 110 行调用（`positionSize` 直接用 `decision.PositionSizeUSD`，`wasCapped` 相关日志一并清）；`minOrderNotional` 两处兜底 12→10 |
| `agent/trade.go` | 删 ratio 上限拒绝块与 `maxPositionValue`；`isMajorTradeSymbol` 若无其他消费者则删；min 兜底 12→10 |
| `agent/tools.go` | `strategyLockedFieldError` 删两 ratio case 与 `min_position_size` case；`strategyConfigContainsLockedField` 名单缩为 `max_positions`、`max_margin_usage`（顶层 `min_position_size` 键检查同删）|
| `agent/skill_management_handlers.go` | `removeLockedAIRiskFields` 删三行（两 ratio 别名 + min_position_size）；策略摘要（中英两处）：ratio 两行替换为「开仓金额百分比：X%」，「最小开仓金额（System enforced）」去掉 System enforced 标注 |
| `agent/skill_execution_handlers.go` | 标签函数删两 ratio case、加 `position_percent`（开仓金额百分比/position size percent）；编辑分支：ratio 两个 locked 删除，`min_position_size` 改为解析 float64 写入（>0），加 `position_percent`（解析 float64，clamp 交给 ClampLimits）；关键词表（两处）删 ratio、加 position_percent（仓位百分比/开仓百分比/单次开仓比例/position percent）与 min_position_size（最小开仓金额/最小下单金额/min position size）；不开单诊断：`maxBTCETHPositionValue` 改 `totalEquity × risk["position_percent"]/100`，文案「按策略风控算出来的单笔仓位上限」→「按策略开仓金额百分比算出来的单笔名义价值约 X USDT」，「最小开仓金额是系统限制，不能手动修改」→「最小开仓金额可在策略里调整（默认 10 USDT）」（中英文两处分支）|
| `agent/strategy_field_catalog.go` | 两处（全量 + ai_trading）editable 名单加 `position_percent`、`min_position_size` |
| `agent/skills/strategy_diagnosis.json`、`trader_diagnosis.json` | System enforced 名单删 ratio 两字段与 `min_position_size`（剩 max_positions、max_margin_usage）；「不要建议修改 min_position_size」规则改为「最小开仓金额是策略可编辑项（默认 10 USDT）；资金不足时优先建议增加权益、换小资金友好标的或调整百分比」 |
| `api/server.go` | 创建策略 schema 说明：ratio 两行删除，加 `risk_control.position_percent: 名义价值占本金百分比（1-20，默认 10）`；`min_position_size (default 12)`→`(default 10, user configurable)` |
| `docs/architecture/STRATEGY_MODULE.md(.zh-CN.md)` | 风控默认值/代码示例行：删 ratio，min 12→10，补 position_percent |
| `web/src/types/strategy.ts` | 删两 ratio 键；加 `position_percent: number`（1–20，默认 10）；min_position_size 注释改「用户可配置，默认 10」 |
| `web/src/components/strategy/RiskControlEditor.tsx` | Position Value Ratio 展示块整块替换为「开仓金额百分比」输入（min 1、max 20、step 0.1、默认 10，写入 `position_percent`）；最小下单 System enforced 展示改为输入（默认 10）|
| `web/src/components/strategy/IndicatorEditor.tsx` | timeframes 数组 = {10s,1m,5m,15m,30m,1h,4h,8h,1d,7d,1w,30d}（分组 scalp/intraday/swing/position 合理归类）|
| `web/src/i18n/strategy-translations.ts` | 删 positionValueRatio 系列 key（按文件内实际语言列）；新增 positionPercent（+Desc）；minPositionSizeDesc 改可配置口径 |

## 兼容与迁移

- 存量策略 JSON 的 ratio 键：Go `json.Unmarshal` 忽略未知键；前端合并保留原键但 store 序列化时丢弃——下次任意保存即净；不写迁移脚本（旧语义一次性替换）。
- 存量周期弃值（3m/2h/6h/12h/3d）：`NormalizeProductSchema` 归空；引擎回落链已有（`timeframes[0]` → 兜底 5m/4h 默认）。提示词里显示空字符串的路径检查 PrimaryTimeframe 消费行（engine_prompt 359/366 用配置值拼句子；归空后句子含空串——接受：⑨ 重写该段；不在 ⑧ 扩散）。
- `EnableMultiTimeframe=true` 但 selected 归空：与旧「用户清空选择」同路径，不加特殊逻辑。

## 测试设计

- `store/strategy_test.go`（新增或扩展）：clamp 矩阵、默认值、diff 提示、normalizeTimeframe 矩阵（新集合逐一通过、弃值出空、别名、大小写/全角容错沿用）。
- `kernel`：`validateDecision` 用例改签名；补「大额仓位不再被 ratio 拒绝」用例。
- `trader`：`auto_trader_account_test.go` 删 ratio 字面量；orders/risk 相关 min=10 用例；`enforcePositionValueRatio` 删除后调用方测试适配。
- `agent`：`config_visibility_test` 对 min_position_size 的断言从「锁定拒绝」改「可编辑生效」；补 `position_percent` 编辑用例（标签+写入）。
- 前端：`npx tsc --noEmit` + vitest 全量（现有 158 用例）；RiskControlEditor 无组件测试先例则不强造，浏览器探针在 ⑨/父级界面验收统一做（⑧ 门禁按地图「前端测试与构建」）。

## 风险

- ⑧→⑨ 中间态提示词含「权益×百分比」而非最终「本金×百分比」：⑨ 的提示词预览门禁统一校正，风险受控（父分支不单独送测）。
- 助手在 ⑧ 后仍可能收到旧 ratio 键（模型幻觉/旧客户端）：merge 进结构体时自然丢弃，不再报 locked 错误——行为可接受。
