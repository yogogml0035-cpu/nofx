# 技术设计：⑩ partial-close

## 边界与职责

- `kernel`：决策契约（动作名、`close_percent`）、解析与校验、提示词、`PositionInfo` 的可分批标记。
- `trader`（交易路径）：动作分发、适配器能力断言、执行、决策日志。
- `trader/types`：分批平仓能力接口与值对象，交易路径只依赖这里。
- `trader/gate`：读取持仓张数与合约步长、精确换算、构造并提交 reduce-only 市价单。
- `web`：决策卡片动作映射与比例展示。

不把 Gate 的合约概念泄漏到 `kernel` 或通用交易路径：能力与数值由适配器以值对象返回。

## 契约

### 决策

```go
type Decision struct {
    // ...
    ClosePercent float64 `json:"close_percent,omitempty"`
}

// 新动作
close_long_partial
close_short_partial
```

校验（`kernel/engine_position.go`）：

- `validActions` 加入两个动作。
- 分批动作：`close_percent` 必须是有限正数且 `< 100`。
  - `= 100` 返回错误，信息说明“100% 是全平，请使用 close_long/close_short”。
  - `<= 0`、NaN/Inf 返回错误。
- 全平动作不要求也不读取 `close_percent`；旧行为不变。

说明：D31 的比例范围写作 0–100；F31 指出“剩余不足一个最小步长”只出现在 100%。
本设计把 100% 归入全平动作，分批动作接受 `(0,100)`，这样“拒绝分批”有明确、可测试的边界，
且不会让一个“全平”决策依赖分批路径。

### 适配器能力（`trader/types/interface.go`）

```go
type PartialCloseStatus struct {
    Supported bool   // 能否分批
    Reason    string // 不能分批的原因（英文，给模型/日志）
    Contracts string // 当前持仓张数（不带符号）
    SizeStep  string // 最小步长（张）
}

type PartialCloseRequest struct {
    Symbol  string
    Side    string  // "long" / "short"
    Percent float64 // (0, 100)
}

type PartialCloseResult struct {
    OrderID   string
    Contracts string  // 本次平掉的张数（不带符号）
    Remaining string  // 平后的剩余张数；"0" 只可能出现在全平，分批路径拒绝
    FillPrice float64
    Quantity  float64 // 平掉的基础币数量（展示与记账）
    Notional  float64 // Quantity × FillPrice
}

type PartialCloser interface {
    PartialCloseStatus(symbol, side string) (PartialCloseStatus, error)
    ClosePartial(req PartialCloseRequest) (PartialCloseResult, error)
}
```

`PartialCloseStatus` 的 `Supported` 语义：持仓张数严格大于一个最小步长。只有一步的仓位任何
`<100%` 比例都向下取整为 0 步，无法分批；`100%` 属于全平。

### Gate 实现（`trader/gate/partial_close.go`）

- `PartialCloseStatus`：
  1. `convertSymbol` 归一；
  2. `findPositionSize(symbol, side)` 读取当前持仓张数（直接查交易所，绕过 15 秒缓存，与全平
     原语同源）；
  3. `GetContractSpec` 读取最小步长（10 分钟缓存、精确解析）；
  4. 持仓绝对值 ÷ 步长必须是正整数；步数 `>1` 时可分批，否则 `Supported=false`，原因
     `position is a single size step`。
- `ClosePartial`：
  1. 校验 `side`、`percent`（有限、`(0,100)`）；
  2. `findPositionSize`；
  3. `SizeForCloseRatio(contracts, percent)`：
     - `Full=true`（比例 100）→ 拒绝并指向全平；
     - `ErrCloseBelowStep` → 拒绝（含比例与持仓）；
     - 其它错误原样返回；
  4. 新增 `ContractSpec` 的市价单最大张数校验（复用已有的 `sizeMax`，即
     `order_size_max` 与 `market_order_size_max` 的较小值）；
  5. 构造 `FuturesOrder{Price:"0", Tif:"ioc", ReduceOnly:true, Text:"t-nofx-partial-close"}`，
     平多 `size = -contracts`、平空 `size = +contracts`；
  6. 成交价必须可解析且为正；`QuantityFromContracts` 得到基础币数量；
     `Notional = Quantity × FillPrice`；
  7. `clearCache()`。
- 不修改保护单：随单止盈止损（`close-*-order`）是委托级保护；分批平仓不撤销、不改单。
  W2 核对分批后保护仍可读回，行为记录到任务证据。

### 交易路径（`trader/auto_trader_orders.go`、`trader/auto_trader_loop.go`）

- `executeDecisionWithRecord` 增加两个动作，调用
  `executePartialCloseWithRecord(decision, actionRecord, "LONG"/"SHORT")`。
- `partialCloser()` 与 `protectedOpener()` 同构：适配器未实现 `types.PartialCloser` 时返回
  错误，发单前拒绝。
- 执行顺序：`sortDecisionsByPriority` 把两个分批动作放在与全平相同的最高优先级（1），
  先平仓再开仓。
- `actionRecord.Action` 保留决策的原始动作名；`Quantity` 填平掉的基础币数量，
  `Price` 填成交价，`OrderID` 填分批单 ID。
- `recordAndConfirmOrder` 按 `close_<side>` 记账；Gate 仍由 OrderSync → PositionBuilder 按实际
  成交数量缩减仓位（`handleClose` 以数量判断部分/全平），不会误记全平。

### 持仓标记（`kernel/engine.go`、`trader/auto_trader_loop.go`、`kernel/engine_prompt.go`）

```go
type PositionInfo struct {
    // ...
    CanPartialClose               bool   `json:"can_partial_close"`
    PartialCloseUnavailableReason string `json:"partial_close_unavailable_reason,omitempty"`
}
```

- `buildTradingContext` 构造每个持仓时：
  - 适配器实现 `PartialCloser` → 调用 `PartialCloseStatus(symbol, side)`；
  - 返回错误 → 记录日志，标记不可分批，原因 `status unavailable`；
  - 未实现 → 标记不可分批，原因 `adapter does not support partial close`。
- `formatPositionInfo` 追加一段：
  - 可分批：`| Partial Close: available`
  - 不可分批：`| Partial Close: unavailable (<reason>)`
- 只在持仓行显示，不影响候选币格式。

### 提示词（`kernel/engine_prompt.go`）

- 输出格式动作清单：`open_long | open_short | close_long | close_short | close_long_partial |
  close_short_partial | hold | wait`。
- 字段说明新增：
  - `close_percent`：分批平仓比例，`(0,100)`；
  - 系统按合约最小步长向下取整换算张数，换算后不足一个最小步长会被拒绝；
  - `100` 表示全平，使用 `close_long`/`close_short`；
  - 仅当持仓行标注 `Partial Close: available` 时可发起。
- 示例 JSON 增加一条分批平仓（单标的与多标的示例都覆盖）。
- 中英双语同步。

### 前端（`web/src/components/trader/DecisionCard.tsx`、`web/src/types/trading.ts`）

- `ACTION_CONFIG` 增加 `close_long_partial`/`close_short_partial`：CLOSE 色系、剪刀/部分平仓
  图标、`PARTIAL` 标签。
- `DecisionAction` 增加可选 `close_percent?: number`；卡片在非开仓且字段存在时显示
  `50%` 比例徽标。

### 文档

- `docs/architecture/STRATEGY_MODULE.md`/`.zh-CN.md`：`validActions`、`Decision` 注释、
  `getActionPriority` 清单加入两个动作。

## 数据流

```
AI JSON
  → extractDecisions（Decision.ClosePercent）
  → validateDecision（动作/比例，(0,100)）
  → sortDecisionsByPriority（close 优先）
  → executePartialCloseWithRecord
  → types.PartialCloser.ClosePartial
  → gate: findPositionSize → SizeForCloseRatio → reduce-only 市价单
  → 成交返回 → DecisionAction（action/quantity/price/orderID）
  → OrderSync → PositionBuilder.handleClose（按成交数量部分缩减）
```

## 错误矩阵

| 条件 | 结果 |
|---|---|
| 动作非法/未知 | 解析阶段拒绝 |
| `close_percent <= 0`、NaN/Inf、缺失 | 解析阶段拒绝 |
| `close_percent = 100` | 解析阶段拒绝，提示使用全平动作 |
| 适配器未实现 `PartialCloser` | 执行前拒绝，不触碰交易所 |
| 找不到该方向持仓 | 适配器返回错误，不下单 |
| 持仓张数不是步长正整数倍 | 适配器返回错误，不下单 |
| 持仓只有一步 | `PartialCloseStatus` 标不可分批；执行换算得 0 步，拒绝 |
| 比例换算后不足一步 | 拒绝（`ErrCloseBelowStep`） |
| 平仓张数超过市价单最大张数 | 拒绝 |
| 下单被交易所拒绝 | 原样返回错误，不改走全平 |
| 成交价不可解析或 ≤0 | 报错（订单已成交，由调用方记录/后续对账处理） |

## 测试

- `kernel/validate_test.go`：新动作解析与比例边界。
- `kernel/engine_prompt_test.go`：中英动作清单、字段说明、持仓标记。
- `trader/gate/contract_test.go` 或 `partial_close_test.go`：市价单最大张数校验。
- `trader/gate/partial_close_test.go`：httptest 模拟 `ListPositions`、
  `GET contracts/{contract}`、`POST orders`，覆盖状态矩阵、换算矩阵、下单矩阵。
- `trader/auto_trader_orders_test.go`：执行路由、能力缺失拒绝、请求参数与记录字段。
- `trader/auto_trader_loop` 相关测试：持仓标记（能力实现/未实现/读取失败）与排序优先级。
- 前端：若已有 DecisionCard 测试则补动作映射；否则以 `tsc --noEmit`、`vite build` 和现有前端
  测试覆盖。

## 兼容与回滚

- 没有旧语义需要兼容：新增动作与字段，全平行为不变。
- 适配器未实现能力时不暴露可分批标记，执行拒绝，不存在“无精确换算的部分平仓”回退。
- 回滚单元是本子任务的提交边界；未通过门禁不合入父分支。

## 已知边界

- 随单止盈止损是委托级保护（子任务 4 实测）；分批平仓不管理保护单，改单/读取当前止损止盈
  属子任务 11。
- 其他交易所适配器不实现 `PartialCloser`，其持仓在提示词中标为不可分批；不改变其全平路径。
- `PartialCloseStatus` 每次调用会查一次交易所持仓列表（Gate `cacheDuration` 为 15 秒但
  `findPositionSize` 有意绕过缓存）；每轮每个持仓一次，数量为当前持仓数。