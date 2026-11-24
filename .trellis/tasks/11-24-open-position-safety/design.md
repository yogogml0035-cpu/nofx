# 子任务 4 技术设计：开仓路径加固

接口事实与文档依据见计划文档第 3 节、子任务 2/3 的 research 留档；跨子任务约定见父任务 design.md。
本文件记录本子任务的边界、接口形状与取舍。

## 分层

```
trader/types     新增 ProtectedOpener 能力接口（请求/结果结构）
trader/gate      实现：随单保护开仓、保护核对、开平仓原语重写、getContract 移除
trader           executeOpenLong/ShortWithRecord 改走 ProtectedOpener；成交后核对；缺失即平仓
```

交易判断（仓位上限、可用保证金、最小下单金额、平仓时机）留在 trader 层；适配层只做
取价、换算、提交与核对（父任务 design.md 分层职责）。

## ProtectedOpener 接口形状（trader/types）

```go
type ProtectedOpenRequest struct {
    Symbol      string
    NotionalUSD float64 // 开仓名义价值（USDT）
    Leverage    int
    StopLoss    float64 // 止损触发价
    TakeProfit  float64 // 止盈触发价
    MinNotional float64 // 最小下单金额（USDT）；0 表示不设下限
    SlipRatio   float64 // 市价滑点上限（0.01 = 1%）；0 表示用交易所默认
}

type ProtectedOpenResult struct {
    OrderID   string
    Contracts string  // 下单张数（十进制字符串）
    Notional  float64 // 张数 × 价格 的实际名义价值
    FillPrice float64
    Quantity  float64 // 币数量（张数 × quanto_multiplier，展示与记账）
}

type ProtectedOpener interface {
    OpenLongProtected(req ProtectedOpenRequest) (ProtectedOpenResult, error)
    OpenShortProtected(req ProtectedOpenRequest) (ProtectedOpenResult, error)
    VerifyOrderProtection(symbol, orderID string) error
}
```

- 采用能力接口而非改 `types.Trader`：接口被网格（`GridTraderAdapter`）、助手（`agent/trade.go`）
  与 9 个交易所适配器共用，改签名外溢整个交易层（违反本子任务边界）。
- AI 开仓路径断言 `types.ProtectedOpener`，未实现则报错拒绝开仓（fail-closed）；
  不提供"无保护回退路径"。
- `VerifyOrderProtection` 单独成方法：核对是"成交后"的动作，调用方决定缺失后的平仓
  （平仓是交易判断）。

## Gate 适配层

### 随单保护开仓（新文件 protected_open.go）

1. `GetContractSpec(symbol)` 取元数据（缓存 10 分钟）。
2. `GetMarketPrice` 内部取最新价（开仓取价的唯一来源；调用方不再取价传入）。
3. `spec.SizeForNotional(price, notional, minNotional)` 换算；`!Orderable()` 时报错并带
   `SkipReason` 中文说明（不足一步 / 低于最小下单金额 / 超市价单最大张数）。
4. 构造 `FuturesOrder`：`Size` = 张数字符串（多正空负）、`Price="0"`、`Tif="ioc"`、
   `MarketOrderSlipRatio` = 调用方滑点（校验 (0, 0.5]，文档上限 50%）、
   `Text="t-nofx"`；`ApplyAttachedTPSL` 写入随单止损止盈（校验两价合法且分居两侧）。
5. best-effort `SetLeverage`（沿用现状：失败只警告；子任务 5 重写杠杆与保证金模式）。
6. `CreateFuturesOrder` 提交（`action_mode` 默认 FULL，返回值即终态，F25：不依赖 GetOrderStatus 确认成交）。
7. 返回 `ProtectedOpenResult`（含张数、实际名义价值、成交价、币数量）。

### 保护核对 VerifyOrderProtection(symbol, orderID)

两条路径，任一命中即受保护：

1. `GetFuturesOrder(orderID)` 回读：`TpslSlTriggerPrice` 与 `TpslTpTriggerPrice` 均非空
   （随单 tpsl 已被交易所接受并保持在委托上）。
2. `ListTriggerOrders(symbol)`（子任务 3 原语）：该合约的未触发条件单同时包含止损与止盈
   （`KindKnown`，方向匹配）。

两条都不满足 → 报错（列出已有条件单情况）；调用方立即市价全平。
路径 2 兜底"随单 tpsl 在委托完结后转为仓位级条件单"的可能形态，W2 实测确认实际行为。

### 开平仓原语重写（trader_orders.go）

- `OpenLong`/`OpenShort`：保留为普通市价开仓原语（助手手动交易、网格使用，不带保护）。
  币数量换算改为精确十进制并**向下取整到最小步长**（`ContractsFromQuantityFloor`，新增
  `ContractSpec` 方法）；不足一个最小步长报错，删除"强制 1 张"；删除开仓前的
  `CancelAllOrders`（IOC 市价单不挂单，无需预撤；避免误撤已有仓位的保护单）。
- `CloseLong`/`CloseShort`：`quantity=0`（全平）→ 读取 `ListPositions` 确认仓位存在与方向，
  按 C.1 节文档语义提交 `close=true` + `size="0"`（不再换算张数）；
  `quantity>0`（部分平，网格/助手）→ `ContractsFromQuantityFloor` + `reduce_only`，
  不足一步报错。
- `SetStopLoss`/`SetTakeProfit`：删除旧实现（四个方向触发规则相反、`close:true` 带非零
  `size`、无 `order_type`）。方法因 `types.Trader` 接口被网格路径占用必须保留，改为明确
  返回不支持错误——语义错误的静默实现替换为显式错误（fail-closed；Gate 网格挂单修复
  不在 17 个子任务范围）。
- `CancelStopLossOrders`/`CancelTakeProfitOrders`：改为 `CancelProtectiveOrdersByKind`
  薄封装（F10 语义修正：按类型撤，不再带动另一类）。
- `CancelAllOrders`：保留（撤限价单 + 撤全部条件单是其文档语义），条件单部分改用
  `ListTriggerOrders` + 逐条 `CancelProtectiveOrder`；`cancelTriggerOrders` 删除。
- `getContract` 删除（F34）：`GetPositions`、`GetOrderStatus`、`GetOpenOrders`、
  `order_sync.go` 中所有 `quanto_multiplier` 读取改经 `GetContractSpec` 精确解析
  （`QuantityFromContracts`），解析失败报错，不再回落 multiplier=1（禁止猜测值）。
  `contractsCache` 字段随之删除。

## trader 层开仓路径（auto_trader_orders.go)

`executeOpenLongWithRecord` / `executeOpenShortWithRecord` 新流程：

1. 持仓重复检查、`enforceMaxPositions`（不变）。
2. `GetBalance` → `enforcePositionValueRatio`、可用保证金上限收缩、`enforceMinPositionSize`（不变）。
3. `ProtectedOpener` 断言失败 → 报错（fail-closed）。
4. `OpenLongProtected/OpenShortProtected`（NotionalUSD=收缩后的仓位、MinNotional=风险控制的
   最小下单金额、SlipRatio=0.01（D8 常量））。
5. 记录：`actionRecord.Quantity/Price/OrderID` 用返回值；`recordAndConfirmOrder` 保留
   （gate 走 OrderSync）；`positionFirstSeenTime` 保留。
6. `VerifyOrderProtection` → 失败：`CloseLong/CloseShort(symbol, 0)` 立即全平 + 报错。
7. 删除：`market.GetWithExchange`、`quantity = size/price` 浮点换算、开仓后
   `SetStopLoss`/`SetTakeProfit` 补挂。

平仓路径：`market.GetWithExchange` → `at.trader.GetMarketPrice`（其余不变；
分批平仓与张数化全平在子任务 6、10）。

## 单元测试（httptest，真实响应格式）

- 随单保护开仓：模拟 ticker/contract/order 端点，断言请求 payload 的 `size`、`tpsl_*`、
  `market_order_slip_ratio`、`tif=ioc`、`price=0`、方向符号；多头与空头。
- 不可下单：三种 `SkipReason` 透出为错误信息。
- `VerifyOrderProtection` 三情形：tpsl 回读有、条件单列表补足、两者皆无。
- 全平 payload：`close=true`、`size="0"`；无仓位时报错。
- `ContractsFromQuantityFloor`：向下取整、浮点噪声吸收、不足一步报错。
- `SetStopLoss`/`SetTakeProfit` 返回不支持错误。
- trader 层：非 `ProtectedOpener` 适配器开仓被拒（最小 mock）。

## 真实验证（probe/livecheck.go，宿主机运行）

D35 授权范围，凭据走环境变量、每步打印、立即还原：

- **W2**（DOGE_USDT）：随单保护开多 17 张（≈9.5U）→ 读仓位确认方向与张数 →
  `VerifyOrderProtection` → 查条件单列表确认止损止盈在列 → 市价全平 → 确认无持仓、
  无未触发条件单、无挂单。
- **W3**（ETH_USDT）：0.1 张（最小步长，≈1.7U）开多并全平，验证小数张随单 tpsl 与步长。
- 容器出口走 VPN 不在 key 白名单：Docker 内 `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64`
  交叉编译，宿主机运行。
