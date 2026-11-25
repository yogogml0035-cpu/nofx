# 技术设计：⑨ sizing-engine

需求与决策以 PRD 与计划文档为准。本文只写实现契约、数据流与取舍。

## 分层与数据流

```
trader.buildTradingContext          读取账户读数 → kernel.AccountInfo.Principal
  └→ kernel.GetFullDecisionWithStrategy
       1. BuildSystemPrompt(principal)         提示词只展示「本金 × 百分比」的代码金额口径
       2. AI 调用                               模型输出：action/leverage/SL/TP/confidence
       3. parseFullDecisionResponse(sizing)    金额由代码计算、校验、写入 Decision.NotionalUSD
trader.prepareOpen                  读 Decision.NotionalUSD；余额可负担性检查与最小下单早退检查
  └→ gate 适配器                    按最小步长取整；取整后低于最小下单金额 / 超过最大张数则不下单
```

金额的唯一定义在 kernel：`principal × position_percent`。适配器只做「USDT 名义价值 → 张数」的精确换算；交易路径不再信任模型给出的任何金额。

## 契约

### kernel

```go
// AccountInfo 新增本金（不含未实现盈亏，已实现盈亏计入）
type AccountInfo struct {
    Principal float64 `json:"principal"` // 新增
    ...
}

// Decision：删除 PositionSizeUSD / RiskUSD；NotionalUSD 由代码填充，json:"-" 防止模型注入
type Decision struct {
    ...
    NotionalUSD float64 `json:"-"` // 开仓名义价值 = 本金 × position_percent
}

// DecisionSizing 一次决策校验所需的金额与价格输入
type DecisionSizing struct {
    Principal       float64
    PositionPercent float64
    BTCETHLeverage  int
    AltcoinLeverage int
    Prices          map[string]float64 // symbol → 当前价（校验止损方向与亏损上限）
}
```

- `parseFullDecisionResponse(aiResponse string, sizing DecisionSizing)` 与
  `validateDecision(d *Decision, sizing DecisionSizing)` 替换现有的
  `(accountEquity, btcEthLeverage, altcoinLeverage)` 参数组。
- `PositionPercent` 在 `GetFullDecisionWithStrategy` 取 `RiskControl.PositionPercent`；
  `ClampLimits` 已把缺失值与越界值归一，kernel 内再按 `<=0 → store.DefaultPositionPercent` 兜底一次。
- `Prices` 由 `ctx.MarketDataMap[symbol].CurrentPrice` 构建（模型看到的同一价格）。

### 金额换算（F33）

```go
func openNotional(principal, percent float64) (float64, error) {
    if principal <= 0 { return 0, 本金错误 }
    tenths := math.Round(percent * 10)      // 百分比十分位整数；percent 由 store clamp 到 0.1 步长
    if tenths <= 0 { return 0, 百分比错误 }
    return principal * float64(tenths) / 1000, nil
}
```

先乘整数再除 1000：`percent/100` 的二进制噪声不会进入结果（`10000 × 0.0113` 得 `112.99999999999999`，`10000 × 113 ÷ 1000` 得 `113`）。适配器按最短十进制表示解析，不会少取一格。

### 校验顺序（`validateDecision`，开仓动作）

1. 动作合法、杠杆 > 0（保留原自动回落到上限的行为与日志）。
2. `openNotional` 成功（本金 > 0、百分比 > 0），写 `d.NotionalUSD`。
3. 止损、止盈均 > 0；多头 SL < TP，空头 SL > TP（保留原校验）。
4. 现价可得（`Prices[symbol]` 缺失即拒绝）。
5. 方向：多 `SL < price < TP`；空 `TP < price < SL`。
6. 亏损上限：`d.NotionalUSD × |price − SL| ÷ price ≤ principal × 2%`（等于允许）。
7. `d.NotionalUSD` 只在此处赋值；解析层不读模型任何金额字段。

`MaxLossPercentOfPrincipal = 2.0` 与 `LossAtStop(notional, entry, stop)`、`ValidateStopLossLoss(notional, entry, stop, principal)` 导出给 ⑪ 复用（修改止损时同一上限）。跌停/异常价格（现价 ≤ 0、非有限数）按校验失败处理。

### 提示词（`BuildSystemPrompt(principal float64, variant string)`）

- 硬约束（代码强制）新增：
  - 开仓名义价值 = 本金 × 百分比，由代码计算并强制（不再显示「权益 × 百分比」）；
  - 到止损价的亏损 ≤ 本金 2%，代码强制。
- 输出示例与字段说明删除 `position_size_usd`、`risk_usd`；开仓必填 = leverage / stop_loss / take_profit / confidence。
- 「仓位大小指引」改为：金额由系统计算，模型只决定方向、杠杆、止损、止盈与置信度；不要输出金额，也不要自行缩放。
- 代码强制金额为 0 或本金无效时，提示词仍按本金 0 渲染，但开仓决策会在校验层被拒绝（fail-closed）。

### API 与前端

- `POST /strategies/preview-prompt`：请求字段 `account_equity` → `account_principal`（默认 1000），
  传给 `BuildSystemPrompt` 的数值即本金；前端 `StrategyStudioPage` 同步字段名。
- `POST /strategies/test-run`：模拟账户 `Principal: 1000`、`TotalEquity: 1000`、`UnrealizedPnL: 0`；
  `BuildSystemPrompt(1000.0, …)`。`run_real_ai=false` 时只返回提示词（不使用 AI）。
- `api/server.go` schema 文案：`position_percent` 改为「本金 × 百分比，代码计算并强制」；
  `min_risk_reward_ratio` 保留为提示词指引（代码不校验，D12）。

### 助手

- `agent/skills/{strategy,trader}_diagnosis.json`：把「不要建议修改 AI 决策里的 position_size_usd」改为
  「开仓金额由系统按本金 × 百分比计算，不要建议修改金额字段」。
- `agent/skill_execution_handlers.go`：
  - 不开单诊断的预期金额 `totalEquity × percent` → `wallet_balance（本金）× percent`；
  - 证据解析删除 `"position_size_usd"` 正则，改为解析 Gate 的 `notional ([\d.]+) USDT` 与
    trader 的 `below minimum \(([\d.]+) USDT\)`；
  - 「不要建议修改 position_size_usd」的 planner 指令改为「不要建议修改系统计算的开仓金额」。
- `agent/trader_scope_test.go` 的历史错误样例（`must be ≥60.00 USDT`）改为新语义的错误样例
  （Gate 取整后低于最小下单金额），断言同步。

## 删除清单（⑨ 范围）

| 位置 | 内容 |
|---|---|
| `kernel/engine_position.go` | 12/60 硬编码最小下单；盈亏比 ≥3:1 校验与 `SL + (TP−SL)×0.2` 的伪入场价 |
| `kernel/engine_prompt.go` | `position_size_usd`/`risk_usd` 输出契约与示例；权益口径的金额展示；要求模型填金额的仓位指引 |
| `kernel/engine.go` | `Decision.PositionSizeUSD`、`Decision.RiskUSD` |
| `kernel/prompt_builder.go`、`kernel/formatter.go` 及 `prompt_builder_test.go` | 无生产调用方的旧提示词/格式化路径（仍带旧金额契约与 HOLD/OPEN_NEW 动作集合） |
| `trader/auto_trader_orders.go` | 读取决策金额的旧注释与赋值路径（改为 `NotionalUSD`） |
| 文档 | `STRATEGY_MODULE(.zh-CN).md`、`prompt-guide(.zh-CN).md`、`TROUBLESHOOTING(.zh-CN).md` 的金额契约与 12/60 口径；`store/strategy.go` 的「step 9」遗留注释 |

## 兼容与回滚

- 不做兼容分支：模型输出多余字段由 `json.Unmarshal` 忽略；历史决策记录里的旧键只作为展示数据保留。
- 回滚点是本子任务的提交边界，失败不合父；不启动 trader、不激活策略。
- 无外部写操作，无 Gate 接口调用；不改变行情取数链路。

## 测试与验证

- `kernel/sizing_test.go`：金额换算（含 F33 例）、方向矩阵、亏损上限边界、本金/百分比异常、杠杆回落、
  旧校验不再拒绝（12 → 10 名义价值、低盈亏比可通过）。
- `kernel/engine_prompt_test.go`：中英提示词契约（无金额字段、含本金口径与 2% 上限、示例可解析）。
- `kernel/engine_analysis_test.go`：模型 JSON 带旧金额字段时被忽略；`NotionalUSD` = 代码计算值。
- `api/strategy_test.go`：`preview-prompt` 与 `test-run`（无币种、无 AI）返回 200 且提示词含代码金额。
- `probe/prompt_preview.go`（`livecheck`/`probe` 构建标签，不入单元测试）：渲染中英系统提示词留档为证据。
- 门禁：`git diff --check`、`make test`、`make build`。