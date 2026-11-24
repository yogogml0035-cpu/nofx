# Gate 条件单（price_orders）与随单止盈止损文档留档

> 用途：保护单适配子任务（protective-order-adapter）的文档依据。
> 文档站 www.gate.com 在本机直连被重置（实测 HTTP 000，与父任务研究记录一致），本文的
> 文档站内容引用父任务子任务 2 留档的原文摘录（当时经网页阅读器抓取）；SDK 内容取自
> 本机 Docker go 模块缓存 `gateapi-go/v7@v7.2.184`（子任务 2 研究时已用 `cmp` 与
> GitHub 上游 tag v7.2.184 逐字节比对一致）。两源交叉核对，未发现冲突。

## A. 触发规则 `rule`（正确语义，F6 的修正依据）

SDK `model_futures_price_trigger.go`（v7.2.184）原文：

```
// Price Condition Type  - 1: Trigger when the price calculated based on `strategy_type`
// and `price_type` is greater than or equal to `Trigger.Price`, while Trigger.Price
// must > last_price - 2: Trigger when the price calculated based on `strategy_type`
// and `price_type` is less than or equal to `Trigger.Price`, and Trigger.Price
// must < last_price
Rule int32 `json:"rule"`
```

- **1 = 价格 ≥ 触发价触发，触发价必须高于最新价**
- **2 = 价格 ≤ 触发价触发，触发价必须低于最新价**
- 其余字段：`strategy_type`（0 = 价格触发）、`price_type`（0 = 最新成交价、1 = 标记价格、
  2 = 指数价格）、`price`（必填字符串）、`expiration`（可选，超时撤单秒数）

NOFX 现状（`trader/gate/trader_orders.go`）与之相反：多头止损、空头止盈用 1；空头止损、
多头止盈用 2——四个方向全部填反（计划文档 F6）。

方向 × 类型 → 规则的正确矩阵：

| 持仓方向 | 保护类型 | 价格运动 | 规则 |
|---|---|---|---|
| 多头 | 止损 | 跌到止损价 | 2 |
| 多头 | 止盈 | 涨到止盈价 | 1 |
| 空头 | 止损 | 涨到止损价 | 1 |
| 空头 | 止盈 | 跌到止盈价 | 2 |

## B. 仓位级止盈止损的 `order_type`

SDK `model_futures_price_triggered_order.go`（v7.2.184）原文：

```
// Types of take-profit and stop-loss orders, including:
//  - `close-long-order`: Order take-profit/stop-loss, close long position
//  - `close-short-order`: Order take-profit/stop-loss, close short position
//  - `close-long-position`: Position take-profit/stop-loss, used to close all long positions
//  - `close-short-position`: Position take-profit/stop-loss, used to close all short positions
//  - `plan-close-long-position`: Position plan take-profit/stop-loss, used to close
//    all or partial long positions
//  - `plan-close-short-position`: Position plan take-profit/stop-loss, used to close
//    all or partial short positions
// The two types of order take-profit/stop-loss are read-only and cannot be passed in requests
OrderType string `json:"order_type,omitempty"`
```

- `close-long-order` / `close-short-order`（随单止盈止损，挂在普通委托上）**只读**，不能在
  创建条件单时传入
- `close-long-position` / `close-short-position`（仓位级，平掉整个仓位）**可写入**
- `plan-close-long-position` / `plan-close-short-position`（仓位计划止盈止损，可全平或部分平）
  可写入
- 状态枚举：`open`（未触发）、`finished`；`inactive` 与 `invalid` 只出现在随单止盈止损上

## C. 创建条件单的 `initial`（ZH-FUT「创建价格触发订单」参数表，父任务留档原文）

| 字段 | 类型 | 说明（原文摘要） |
|---|---|---|
| `contract` | string | 合约标识（必填） |
| `size` | integer(int64) | 代表需要平仓的合约张数，全平仓:size=0 部分平仓:plan-close-short-position size>0 部分平仓:plan-close-long-position size<0 |
| `amount` | string | 同size参数，用于兼容小数,同时存在时以amount为准 |
| `price` | string | 交易价，当价格为 0 时，表示通过市价方式来下单（必填） |
| `close` | boolean | 单仓模式全部平仓时,必须设置为true执行平仓操作 单仓模式部分平仓时/双仓模式下，可以不设置close，或close=false（只写） |
| `tif` | string | 默认 gtc，市价单当前只支持 ioc |
| `reduce_only` | boolean | 设为 true 可确保订单不会开新仓，只用于平仓或减仓 |
| `auto_size` | string | 单仓模式不需设置；双仓模式全部平仓（size=0）时必须设置，close_long 平多头，close_short 平空头 |

小数张结论（两源一致）：条件单的 `size` 是 `int64`，小数张走字符串 `amount`，两者同时给出
以 `amount` 为准。SDK `FuturesInitialOrder` 同时具备 `Size int64` 与 `Amount string`。

NOFX 账户的持仓模式是 `single`（单向，2026-11-24 复核），仓位级全平用 `size=0` + `close=true`，
不需 `auto_size`。

## D. 查询、撤销与改单

| 操作 | 接口 | SDK 方法（v7.2.184） |
|---|---|---|
| 查询（按状态） | `GET /futures/{settle}/price_orders?status=open` | `ListPriceTriggeredOrders(ctx, settle, status, *ListPriceTriggeredOrdersOpts{Contract, Limit, Offset})` |
| 查单个 | `GET /futures/{settle}/price_orders/{order_id}` | `GetPriceTriggeredOrder(ctx, settle, orderId int64)` |
| 撤单个 | `DELETE /futures/{settle}/price_orders/{order_id}` | `CancelPriceTriggeredOrder(ctx, settle, orderId int64)` |
| 撤全部 | `DELETE /futures/{settle}/price_orders` | `CancelPriceTriggeredOrderList(ctx, settle, *CancelPriceTriggeredOrderListOpts)` |
| 改单 | `PUT /futures/{settle}/price_orders/amend/{order_id}` | `UpdatePriceTriggeredOrder(ctx, settle, orderId string, body FuturesUpdatePriceTriggeredOrder) → TriggerOrderResponse` |

改单请求体 `FuturesUpdatePriceTriggeredOrder`（SDK 原文要点）：

- `order_id`（int64，必填）与路径参数 `order_id`（字符串）必须一致
- `size`（int64）/ `amount`（string）：修改后的合约张数，全平：0；部分平仓：正负代表方向；
  同时给出以 `amount` 为准
- `price`（string）：修改后的交易价，0 表示市价；**不传表示不修改**
- `trigger_price`（string）：修改后的触发价
- `price_type`、`auto_size`、`close` 同创建语义
- **没有 `rule` 字段：触发方向在创建时固定，改单不能改变触发方向**。止损从现价一侧移到另一侧时
  只能撤旧建新（由子任务 11 的管理路径处理）

changelog v4.106.160（2026-10-16，父任务留档原文）：

> 不兼容变更：合约条件单改单接口由 `PUT /futures/{settle}/price_orders/amend` 改为
> `PUT /futures/{settle}/price_orders/amend/{order_id}`。新增必填的字符串路径参数 `order_id`，
> JSON 请求体中的 `order_id` 仍需提供。调用方需在路径和请求体中传入相同的订单 ID。

已完结的条件单在完结 10 分钟后不能按 ID 查询（计划文档 F25，下单接口文档说明）。

## E. 随单附带止盈止损（普通委托字段）

SDK `model_futures_order.go`（v7.2.184）：

```
TpslTpTriggerPrice string `json:"tpsl_tp_trigger_price,omitempty"`
TpslSlTriggerPrice string `json:"tpsl_sl_trigger_price,omitempty"`
TpslTpBboType      string `json:"tpsl_tp_bbo_type,omitempty"`
TpslSlBboType      string `json:"tpsl_sl_bbo_type,omitempty"`
```

计划文档 3.4 节结论：下单接口可选 `tpsl_tp_trigger_price`、`tpsl_sl_trigger_price`；随单止盈止损
的触发方向由交易所按委托方向推导（委托平仓方向固定，与仓位级 `rule` 显式填写不同）。挂到未成交
委托上的随单止盈止损在条件单列表中以 `inactive`/`invalid` 状态出现（B 节状态枚举）。

## F. 响应模型

`CreatePriceTriggeredOrder` 与 `UpdatePriceTriggeredOrder` 的返回都是 `TriggerOrderResponse`：
只有 `Id int64` 与 `IdString string`（同一订单 ID 的十进制字符串形式，文档建议展示用它）。
查询返回的 `FuturesPriceTriggeredOrder` 另含 `Status`、`FinishAs`、`Reason`、`OrderType`、
`MeOrderId`、`PosMarginMode` 等。

## G. 未找到项

- 官方资料没有说明「仓位级止盈止损在无持仓时创建」的行为 → **已实测回答（见 H.1）**：拒绝
- 官方资料没有说明随单止盈止损对触发价精度（tick size）的校验行为；实现按调用方给的价格
  原样提交，由交易所校验并回传错误

## H. 实测结论（2026-11-24，DOGE_USDT，写操作即时还原）

### H.1 仓位级止盈止损在无持仓时被拒绝

`POST /futures/usdt/price_orders`，`order_type=close-long-position`、`size=0`、`close=true`：

```
label: AUTO_USER_NO_LONG_POSITION
message: user not have long position
```

交易所在下单时就校验持仓存在。仓位级保护单的真实挂、查、改、撤只能在有仓位的 W2
（子任务 4）上验证（计划文档 F27 同一结论）。

### H.2 条件单改单当前不可用（API 创建的触价单被拒绝）

| 路径 | 结果 |
|---|---|
| 新 `PUT /futures/usdt/price_orders/amend/{order_id}`（changelog v4.106.160、SDK v7.2.184 所用） | **404 Not Found**：线上环境未部署该路径 |
| 旧 `PUT /futures/usdt/price_orders/amend` + 请求体 `order_id`（uint64） | **400，code 1077** `APIOrderNotSupportUpdateTouchOrder`：「Updating (including editing, adding and deleting) TP/SL conditions for API orders is not supported.」 |

补充实测细节：

- 请求体缺 `order_id` → 400 code 1003 `AUTO_INVALID_PARAM_ORDER_ID`；`order_id` 为 JSON
  字符串 → 400 code 1101 `AUTO_INVALID_REQUEST_BODY`（必须为数值）。说明旧路径在协议层
  仍在服务、按请求体读订单 ID
- 拒绝与下单时的 `text` 无关：不带 text（默认 api）与带 `t-nofx` 都被拒
- 被测订单是普通条件单（无 `order_type`）。仓位级/计划级条件单是否同样被拒，待 W2 有仓位时实测

**结论：修改止损止盈的实际路径是撤旧建新**（撤销 + 重新构造 + 提交，适配层已具备全部
原语）。SDK 的 `UpdatePriceTriggeredOrder` 按文档/changelog 是规范路径，实现保留并在
注释中标注实测状态；改单的真实验证在 W2（有仓位）上补做。
