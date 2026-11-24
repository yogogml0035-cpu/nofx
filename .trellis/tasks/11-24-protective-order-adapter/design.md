# 技术设计：保护单适配（protective-order-adapter）

## 边界

纯新增，不接入交易路径，合入后运行中的 trader 行为不变。

- `trader/gate/protective_orders.go`：全部新代码（类型、纯构造函数、随单止盈止损、
  查询/挂单/改单/撤销的薄封装）
- `trader/gate/protective_orders_test.go`：单测矩阵与解析测试
- 不动 `SetStopLoss`、`SetTakeProfit`、`CancelStopLossOrders`、`CancelTakeProfitOrders`、
  `cancelTriggerOrders`、`CancelAllOrders`、`CancelStopOrders`；旧的错误语义与「撤光」行为
  由子任务 4 在接入时删除
- 复用子任务 2 的 `decimal.go`（`parseDecimal`、`decimalFromFloat`、`formatDecimal`）做
  价格的精确解析与格式化；不新增第三方依赖

## 文档依据

原文摘录见 `research/gate-price-order-docs.md`（文档站直连不可达，内容引用父任务子任务 2
留档的 ZH-FUT 原文 + 官方 Go SDK v7.2.184 源码，两源一致）。与本设计直接相关的结论：

| 设计项 | 依据 | 性质 |
|---|---|---|
| `rule` 1 = 价格 ≥ 触发价且触发价须高于最新价；2 = 价格 ≤ 触发价且须低于最新价 | SDK `FuturesPriceTrigger.Rule` 注释原文 | 原文 |
| `close-long-position` / `close-short-position` 可写入；随单类型 `close-*-order` 只读 | SDK `FuturesPriceTriggeredOrder.OrderType` 注释原文 | 原文 |
| 仓位级全平：`size=0` + `close=true`；单向持仓模式不需 `auto_size` | ZH-FUT 创建条件单参数表 | 原文 |
| 小数张走字符串 `amount`，与 `size` 同时给出以 `amount` 为准 | changelog v4.106.52 + SDK `FuturesInitialOrder.Amount` | 原文 |
| 改单 `PUT price_orders/amend/{order_id}`，路径与请求体都带 `order_id` 且一致 | changelog v4.106.160 + SDK 方法签名 | 原文 |
| 改单不能改 `rule`（请求体无该字段） | SDK `FuturesUpdatePriceTriggeredOrder` 模型 | 原文 |
| 随单止盈止损字段 `tpsl_tp_trigger_price` / `tpsl_sl_trigger_price` | SDK `FuturesOrder` 模型 | 原文 |
| 无持仓时创建仓位级止盈止损 → 交易所拒绝 `AUTO_USER_NO_LONG_POSITION` | 2026-11-24 实测（research H.1） | 实测 |
| API 创建的触价单当前不能改单：新路径 404 未部署、旧路径拒绝 `APIOrderNotSupportUpdateTouchOrder` | 2026-11-24 实测（research H.2） | 实测 |

## 触发规则矩阵（核心修正，F6）

| 持仓方向 | 保护类型 | 价格运动 | `rule` | `order_type` |
|---|---|---|---|---|
| 多头 | 止损 | 跌到止损价 | 2 | `close-long-position` |
| 多头 | 止盈 | 涨到止盈价 | 1 | `close-long-position` |
| 空头 | 止损 | 涨到止损价 | 1 | `close-short-position` |
| 空头 | 止盈 | 跌到止盈价 | 2 | `close-short-position` |

查询侧的反向判定（同一矩阵倒查）：多头条件下 `rule` 2 = 止损、1 = 止盈；空头条件下
`rule` 1 = 止损、2 = 止盈。`order_type` 缺失（普通条件单）时方向未知，类型不判定。

## 数据结构与函数

```go
// ProtectiveKind 保护单类型
type ProtectiveKind int
const (
    ProtectiveStopLoss ProtectiveKind = iota + 1 // 止损
    ProtectiveTakeProfit                          // 止盈
)
func (k ProtectiveKind) String() string // "stop_loss" / "take_profit"

// 触发规则常量（Gate 语义，非 NOFX 方向语义）
const (
    PriceRuleGE int32 = 1 // 价格 >= 触发价触发，触发价须高于最新价
    PriceRuleLE int32 = 2 // 价格 <= 触发价触发，触发价须低于最新价
)

// protectiveTriggerRule 方向 × 类型 → rule；方向或类型非法时报错
func protectiveTriggerRule(side string, kind ProtectiveKind) (int32, error)

// BuildPositionProtectiveOrder 构造仓位级止盈止损条件单（平掉整个仓位）。
// side 取 "LONG"/"SHORT"（大小写不敏感）；kind 为止损或止盈；triggerPrice > 0，
// 按最短十进制表示输出。initial：size=0、close=true、price="0"（市价）、tif="ioc"。
// 触发固定 strategy_type=0（价格触发）、price_type=0（最新成交价），不设 expiration。
func BuildPositionProtectiveOrder(contract, side string, kind ProtectiveKind, triggerPrice float64) (gateapi.FuturesPriceTriggeredOrder, error)

// ApplyAttachedTPSL 把随单止盈止损触发价写入开仓委托。
// 校验：两价均为正；多头要求 SL < TP，空头要求 TP < SL（两价必然分居入场价两侧）。
// 只设置 TpslSlTriggerPrice / TpslTpTriggerPrice，触发方向由交易所按委托方向推导。
func ApplyAttachedTPSL(order *gateapi.FuturesOrder, side string, stopLossPrice, takeProfitPrice float64) error

// TriggerOrderView 未触发条件单的解析视图；解析失败即报错，不回落猜测值
type TriggerOrderView struct {
    ID          int64
    IDString    string
    Contract    string
    OrderType   string          // 交易所返回的原始值；普通条件单为空
    Side        string          // "LONG"/"SHORT"，由 order_type 判定；未知为 ""
    Kind        ProtectiveKind  // 由方向 + rule 判定；无法判定时为 0（零值）
    KindKnown   bool
    TriggerRule int32
    TriggerPrice float64        // parseDecimal 解析；失败即整体报错
    InitialSize  int64
    InitialAmount string
    Status      string
}

// ListTriggerOrders 列出合约上 status=open 的条件单（ListPriceTriggeredOrders），
// 逐条解析为 TriggerOrderView；任何一条解析失败即返回错误
func (t *GateTrader) ListTriggerOrders(symbol string) ([]TriggerOrderView, error)

// PlaceProtectiveOrder 提交构造好的条件单，返回订单 ID（优先 IdString）
func (t *GateTrader) PlaceProtectiveOrder(order gateapi.FuturesPriceTriggeredOrder) (string, error)

// AmendProtectiveOrderTrigger 改触发价：构造 FuturesUpdatePriceTriggeredOrder
// {OrderId: id, TriggerPrice: 新价}，路径与请求体携带同一订单 ID；不触碰 size/amount/close
func (t *GateTrader) AmendProtectiveOrderTrigger(orderID string, newTriggerPrice float64) (string, error)

// AmendProtectiveOrderSize 改委托张数：十进制张数字符串经 amount 提交（F32），
// 同时置 size=0 避免两字段歧义；正负号按方向约定：平多仓为负、平空仓为正
func (t *GateTrader) AmendProtectiveOrderSize(orderID, contracts string) (string, error)

// CancelProtectiveOrder 按订单 ID 撤销单个条件单
func (t *GateTrader) CancelProtectiveOrder(orderID string) error

// CancelProtectiveOrdersByKind 撤销合约上指定类型的未触发保护单（F10 的语义修正）：
// 先 ListTriggerOrders，逐条判定 Kind，只撤匹配者；无法判定类型的条件单不撤（fail-closed），
// 返回撤销的张数列表；单个撤销失败即中止并返回错误
func (t *GateTrader) CancelProtectiveOrdersByKind(symbol string, kind ProtectiveKind) ([]string, error)
```

### 设计要点

1. **构造与提交分离**：`BuildPositionProtectiveOrder` 是纯函数，单测矩阵直接覆盖；
   `PlaceProtectiveOrder` 是薄封装，只提交与回传 ID。
2. **方向用 `side string`**（"LONG"/"SHORT"，大小写不敏感），与现有 Trader 接口的
   `positionSide` 形参一致；严格校验，其他取值报错。
3. **价格处理**：`float64` 入参经 `decimalFromFloat` 取最短十进制表示（`0.0775` 就是
   `"0.0775"`），非正数报错；不按 tick size 取整——官方资料未见触发价精度的校验行为，
   由交易所校验并回传错误（research G 节）。调用方给 tick 对齐价格。
4. **改单是尽力路径，撤旧建新是实际路径**：改单请求体没有 `rule` 字段，方向在创建时固定；
   且实测当前 API 创建的触价单一律被拒（新路径 404 未部署、旧路径 code 1077，research H.2）。
   改单封装保留（SDK 规范路径，W2 有仓位时补测），修改止损止盈的实际路径由撤旧建新
   （撤销 + 构造 + 提交）承担，子任务 11 在其上做亏损上限校验（D34）。
5. **`AmendProtectiveOrderSize` 只发 `amount`**：两字段同给以 `amount` 为准，`size` 因
   omitempty 不随请求发送；正负号按「平多仓为负、平空仓为正」的方向约定，与创建接口文档一致。
6. **改单响应**：SDK 返回 `TriggerOrderResponse`（Id/IdString），本设计回传 ID 字符串供日志。
7. **查询不做单条 Get**：已完结条件单 10 分钟后不能按 ID 查询（F25），列表查询是唯一
   稳定入口；`GetPriceTriggeredOrder` 不封装，避免误用。

## 真实写操作验证（D35 范围内）

对象：DOGE_USDT（用户无手动持仓；1 张 ≈ 0.56 USDT）。步骤（每步打印读数，全程立即还原）：

1. 读最新价，取远离现价的触发价（下方 ≈ −50%）
2. **挂**：先试仓位级止损单（`close-long-position`）——实测被拒 `AUTO_USER_NO_LONG_POSITION`
   （research H.1，交易所校验持仓存在）；改用普通条件单（不带 `order_type`、`close=false`、
   `size=+1`）完成链路
3. **查**：`ListTriggerOrders` 读回，核对触发价与规则
4. **改**：`AmendProtectiveOrderTrigger` 尝试改触发价，交易所拒绝（research H.2）→
   记录拒绝；随后验证实际路径 **撤旧建新**：撤销原单、按新触发价重新构造并提交、查询核对
5. **撤**：`CancelProtectiveOrder` 撤销，再查确认列表为空
6. 按类型撤销：仓位级单挂不上，普通单方向未知，跳过（判定逻辑由单测覆盖）；W2 补测
7. 随单止盈止损（`ApplyAttachedTPSL`）：在远离现价处挂一张带 tpsl 的 gtc 限价单
   （名义 1 张 ≈ 0.56U，不会成交），从下单返回确认交易所接受 tpsl 字段，随后撤单还原

脚本放 `probe/livecheck.go`（独立 main，不进构建产物），证据输出 JSONL 留档。

## 已知边界

- 触发价必须与最新价保持规则要求的一侧关系，否则交易所拒绝（`rule` 语义的另一半）；
  本子任务不校验最新价（适配层无行情职责），调用方保证，交易所兜底
- **改单当前不可用**（research H.2）：API 创建的触价单一律被拒；修改止损止盈走撤旧建新。
  改单封装保留并在代码注释标注实测状态，W2 有仓位时补测仓位级条件单是否可改
- **仓位级条件单必须有持仓才能挂**（research H.1）：挂、查、改、撤的仓位级真实验证全部
  并入 W2（子任务 4），本子任务的仓位级正确性由单测矩阵覆盖
- `TriggerOrderView.TriggerPrice` 是 float64，仅用于展示与日志；需要精确值时用原始字符串
  （SDK 触发价字段为 string，解析用 `parseDecimal`，失败即报错）
- 撤销后查询列表有秒级延迟的可能；真实验证以撤销接口的返回与再次查询共同佐证
- `ListTriggerOrders` 只看 `status=open`；`finished` 历史与本子任务无关
