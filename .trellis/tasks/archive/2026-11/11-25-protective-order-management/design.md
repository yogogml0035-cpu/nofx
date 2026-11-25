# 技术设计：⑪ protective-order-management

## 边界与职责

- `kernel`：决策契约（两个修改动作、价格字段）、校验（持仓存在/保护可读/方向/2% 上限/
  字段匹配）、提示词（动作说明与示例、持仓行的当前止损止盈）、`PositionInfo` 字段。
- `trader/types`：保护单管理能力接口与值对象；通用交易路径只依赖这里。
- `trader`（交易路径）：动作分发、能力断言、执行、决策日志、本金读取。
- `trader/gate`：条件单列表归类、触发价对齐、保护读取、改单（先挂新、核对、撤旧、核对）、
  随单保护核对收紧。
- `web`：决策卡片动作映射与新价格展示。
- 文档：架构文档中英、规范新增场景、父任务计划文档（F44/F45 与进度）。

Gate 的 `order_type`/`rule`/`order_price_round` 不出现在 `kernel` 与 `trader`（通用层）。

## 契约

### 决策（kernel/engine.go）

```go
type Decision struct {
    // 已有：open_* / close_* / close_*_partial / hold / wait
    Action string `json:"action"` // 新增 update_stop_loss | update_take_profit
    StopLoss   float64 `json:"stop_loss,omitempty"`   // update_stop_loss 读取
    TakeProfit float64 `json:"take_profit,omitempty"` // update_take_profit 读取
}

type PositionInfo struct {
    // 已有字段...
    StopLoss   float64 `json:"stop_loss"`   // 0 = 未知
    TakeProfit float64 `json:"take_profit"` // 0 = 未知
    // ProtectionUnavailableReason 非空表示当前保护不可读（模型不得修改）
    ProtectionUnavailableReason string `json:"protection_unavailable_reason,omitempty"`
}
```

### 校验（kernel/engine_position.go）

`DecisionSizing` 增加 `Positions []PositionInfo`（来自本轮上下文）。新增动作：

- 标的方向：按 `Symbol` 匹配 `Positions`（大小写/后缀变体沿用现有 `currentPrice` 的匹配口径，
  由调用方把决策 symbol 规范成持仓 symbol 后再校验；匹配不到即拒绝）。
- 目标持仓必须存在，且 `StopLoss > 0 && TakeProfit > 0`（保护可读）——否则拒绝：
  “protection for <symbol> is not readable; refusing to modify”。
- 价格：`update_stop_loss` 读 `StopLoss`、`update_take_profit` 读 `TakeProfit`；有限正数。
- 方向（现价 P，多）：`newSL < P`、`newTP > P`；空相反；等于现价拒绝。
- 止损上限（D34）：`loss = |quantity| × |entryPrice − newSL|`；上限
  `principal × MaxLossPercentOfPrincipal / 100`；超过（带 1e-12 相对容差）拒绝。
  数量与入场价取持仓快照；本金取 `sizing.Principal`。
- 动作与字段不交叉：修改止损不读取 `take_profit`，反之亦然。

### 适配器能力（trader/types/interface.go）

```go
type ProtectionKind string

const (
    ProtectionStopLoss   ProtectionKind = "stop_loss"
    ProtectionTakeProfit ProtectionKind = "take_profit"
)

// PositionProtection 当前持仓的止损止盈读取结果。
// 多个同类条件单并存时取距现价最近（最先触发）的一条作为有效值，计数给出实际条数。
type PositionProtection struct {
    StopLoss         float64 // 有效止损触发价；0 = 无
    TakeProfit       float64 // 有效止盈触发价；0 = 无
    StopLossOrders   int
    TakeProfitOrders int
    Known            bool   // 止损与止盈都可读
    Reason           string // Known=false 的原因（英文，给日志/模型）
}

type ProtectionUpdateRequest struct {
    Symbol         string
    Side           string // "long"/"short"
    Kind           ProtectionKind
    Price          float64
    Principal      float64 // 本金（D4 口径）
    MaxLossPercent float64 // 止损上限（百分点，开仓与修改共用 2.0）
}

type ProtectionUpdateResult struct {
    OrderID         string  // 新保护单 ID
    Kind            ProtectionKind
    OldPrice        float64 // 替换前的触发价；0 = 无
    NewPrice        float64 // 对齐后的实际触发价
    ReplacedOrderIDs []string
}

type ProtectiveOrderManager interface {
    PositionProtection(symbol, side string) (PositionProtection, error)
    UpdateProtection(req ProtectionUpdateRequest) (ProtectionUpdateResult, error)
}
```

### Gate 实现

#### 触发价对齐（`ContractSpec`）

- 读合约 `order_price_round` 作为价格步长（字段缺失或非正 → 构造失败，fail-closed）。
- `AlignTriggerPrice(price float64, direction int) (string, *big.Rat, error)`：
  - 用 `big.Rat` 精确计算 `price / tick`，按方向 `floor`（-1）或 `ceil`（+1）取整数步，
    再乘 tick，输出与 tick 同精度的十进制字符串（`0.04884` 而不是 `0.048840000000000003`）。
  - 入参必须是有限正数。
- 方向约定（保护不变量）：
  - 止损：向现价收紧——多向上、空向下；对齐后必须仍在现价正确一侧，否则拒绝。
  - 止盈：远离现价——多向上、空向下；对齐后必须仍在现价正确一侧，否则拒绝。
  - 这样对齐只会减小止损时的亏损（2% 上限仍成立），不会降低止盈目标。

#### 开仓路径（protected_open.go）

1. 取价、换算张数（不变）。
2. 取合约步长，对齐 SL/TP；对齐失败或不在现价正确一侧 → 拒绝开仓（不发任何请求）。
3. `ApplyAttachedTPSL` 用对齐后的价格；提交。
4. 成交后 `VerifyOrderProtection`：
   - 读取该合约未触发条件单，按 `order_type` + `rule` 归类；
   - 同侧（由开仓委托张数符号或列表判定）止损与止盈各至少 1 条才通过；
   - 开仓委托的 `tpsl_*` 字段只记日志，不再单独作为通过依据；
   - 列表查询最多重试 3 次（间隔 300ms）吸收创建延迟；
   - 该方向持仓已不存在（被触发平掉）→ 视为无需保护，返回 nil。
5. 对齐后的 SL/TP 写入 `ProtectedOpenResult`；交易路径用实际价格覆盖决策记录的止损止盈。

#### 保护读取（protective_orders.go 扩展）

`PositionProtection(symbol, side)`：

1. `ListTriggerOrders`（未触发，该合约）；按 `classifyTriggerOrder` 得方向与类型；
2. 只保留同侧且类型已知的条目；同类多条时取 `|trigger − 现价|` 最小者（最先触发）；
3. `Known = 同侧止损 ≥1 && 同侧止盈 ≥1`；否则 `Reason` 写明缺失项；
4. 现价取 `GetMarketPrice`；读取失败返回错误（由调用方标为不可读，不猜测）。

#### 改单（protective_orders.go 新增 `UpdateProtection`）

1. 参数校验：side/kind、价格有限正数、本金正数、止损上限正数。
2. 读取该方向持仓（`findPosition`：直接查交易所，绕过缓存）；不存在 → 拒绝。
3. 止损：`loss = quantity × |entry − price|`，`quantity = |contracts| × quanto_multiplier`；
   超过 `principal × maxLossPercent / 100`（1e-12 容差）→ 拒绝。
4. 现价校验后对齐（止损收紧、止盈远离）；对齐后仍在正确一侧，否则拒绝。
5. 读取当前保护；止损或止盈任一不可读 → 拒绝修改（无法保证旧保护被替换）。
6. 挂新仓位级保护单（`BuildPositionProtectiveOrder` + `PlaceProtectiveOrder`），
   列表核对新 ID 与新触发价存在。
7. 撤销同类旧单（按 ID，排除新单）；撤销失败 → 撤销新单、核对旧保护仍在、返回错误。
8. 列表核对同类只剩新单；否则撤销新单并返回错误。
9. 返回 `OldPrice`、`NewPrice`、`OrderID`、`ReplacedOrderIDs`。

先挂新后撤旧保证任何时刻仓位都有同类型保护单；失败路径恢复到调用前状态。

#### 交易路径（trader/auto_trader_orders.go）

- 新动作分发到 `executeUpdateProtectionWithRecord(kind)`：
  1. 能力断言（`ProtectiveOrderManager`），未实现 → 拒绝；
  2. `GetPositions` 找到该标的持仓与方向，不存在 → 拒绝；
  3. `ReadAccountBalance` 取本金；
  4. `UpdateProtection`，失败即返回（无请求 = 无副作用）；
  5. 记录 `actionRecord.Price`（新价）、对应止损/止盈字段、订单 ID；
  6. 日志：旧价 → 新价、新单 ID、被替换的旧单 ID。
- `executeProtectedOpen` 用 `ProtectedOpenResult.StopLoss/TakeProfit` 覆盖 `actionRecord`。

#### 交易循环（buildTradingContext）

- 对每个持仓调用能力接口 `PositionProtection(symbol, side)`：
  - 成功：写 `StopLoss`/`TakeProfit`；
  - 失败/适配器不支持：`StopLoss=TakesProfit=0`，`ProtectionUnavailableReason` 写明原因；
- 每轮读取，不落库（F9 不新增字段；来源是交易所状态）。

#### 提示词（kernel/engine_prompt.go）

- 持仓行追加：`| SL <price> | TP <price>`；未知时 `| SL unknown (原因) | TP unknown`。
  价格用 `strconv.FormatFloat(v, 'f', -1, 64)`（保留有效小数，避免丢 tick）。
- 动作清单加入 `update_stop_loss | update_take_profit`；
- 字段说明（中英）：只对当前保护可读的持仓使用；`stop_loss`/`take_profit` 填新价格；止损与
  止盈必须位于现价正确一侧；到止损价的亏损不得超过本金 2%，否则被拒绝；系统按合约价格
  步长对齐新价格（止损只会更紧、止盈不会更近）。
- 单独格式示例（与分批平仓同风格）：`{"symbol": "...", "action": "update_stop_loss",
  "stop_loss": 0.05}`，标注仅格式示例。
- “Current Positions” 后的说明段追加修改止损止盈的规则段（中英）。

#### 前端（DecisionCard）

- 动作映射：`update_stop_loss`（🛡️，UPDATE SL）、`update_take_profit`（🎯，UPDATE TP）；
- 非开仓动作展示新价格（沿用持仓详情网格？开仓才展示 SL/TP；修改动作用一行小字
  `SL 0.0488` / `TP 0.0597`，来自 `stop_loss`/`take_profit`）；
- 用例：动作识别、价格展示、不落入 wait。

#### 文档

- `docs/architecture/STRATEGY_MODULE.md` 与 `.zh-CN.md`：动作集合、Decision 字段、优先级、
  新增“修改止损止盈”小节与执行顺序。
- `.trellis/spec/...`（受影响层规范）：新增 “Update protective order (cancel-and-replace)”
  场景与触发价对齐要求（按现有规范文件结构落位）。
- 父任务计划文档：F44（未对齐价格静默不建保护单）、F45（随单保护 reduce-only、可撤销、
  分批后张数不变）、10.5 进度第 11 行。

## 数据流

```
Gate 未触发条件单 ──ListTriggerOrders──▶ PositionProtection ──▶ kernel.PositionInfo ──▶ 提示词
        ▲                                                                                 │
        └── BuildPositionProtectiveOrder / PlaceProtectiveOrder ◀── UpdateProtection ◀── 决策
             (对齐后的仓位级保护单)                                   ▲
                                                       AutoTrader 执行路径（本金/持仓复核）
```

## 权衡与兼容

- **为什么撤旧建新**：交易所改单接口不可用（404 / code 1077）；保留 SDK 改单封装但不依赖。
- **为什么先挂新后撤旧**：避免“已撤旧、未挂新”的无保护窗口；失败路径可完整恢复。
- **为什么止损对齐取“收紧”**：对齐不允许放大亏损（D34），且对齐后必须在现价正确一侧。
- **为什么保护未知时拒绝修改**：无法枚举旧单就无法保证替换；只读展示“未知”仍允许模型
  选择不动，但不允许它制造“看起来改了、实际旧单仍生效”的状态。
- **为什么开仓核对改看列表**：探针证明 `tpsl_*` 字段被接受不等于条件单已创建；以列表为准
  才能在价格未对齐时 fail-closed（本次同时修掉价格未对齐这一根因）。
- **兼容性**：`ContractSpec` 构造函数新增 `orderPriceRound` 参数；全部调用点与测试同步更新。
  `ProtectedOpenResult` 增加两个字段；旧调用方零值语义不变。`PositionInfo` 增加字段，
  JSON 消费方（决策记录/前端）向后兼容。

## 已知边界

- 开仓委托在完结 10 分钟后不能按 ID 查询（F25）；当前止损止盈以条件单列表为唯一稳定来源。
- 条件单列表只含 `status=open`；被触发的保护单不再出现在列表，持仓也随之关闭。
- 分批平仓后随单保护单张数仍为开仓时张数（reduce-only），本子任务不迁移为仓位级；
  ⑩ 的“分批不撤不改保护单”保持不变。
- `order_price_round` 是所有永续合约的必填字段；缺失时该合约不进入保护/交易路径（fail-closed）。