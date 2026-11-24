# 技术设计：张数换算（contract-math）

## 边界

纯新增，不接入交易路径，合入后运行中的 trader 行为不变。

- `trader/gate/decimal.go`：十进制基础函数（精确解析、按最短十进制取 float、定点输出、取整除法）
- `trader/gate/contract.go`：`ContractSpec` 与构造校验、`SizeForNotional`、`SizeForCloseRatio`、
  `ContractsFromQuantity`、`QuantityFromContracts`、`GetContractSpec`
- `trader/gate/trader.go`：只加一个缓存字段
- 不动 `OpenLong`、`OpenShort`、`CloseLong`、`CloseShort`、`SetStopLoss`、`SetTakeProfit`、
  `FormatQuantity` 与 `getContract`。旧的截断与强制 1 张逻辑由子任务 4 在接入时删除

## 文档依据

原文摘录见 `research/gate-contract-size-docs.md`（API v4 文档站的根页与 Futures 分页、Agent 文档页、
官方 Go SDK 文档、两篇 Gate 公告）。与本设计直接相关的结论：

| 设计项 | 依据 | 性质 |
|---|---|---|
| 下单的 `size` 是合约张数，不是币数量 | 文档站「合约交易下单」、SDK `CreateFuturesOrder` 说明 | 原文 |
| 一张合约对应 `quanto_multiplier` 个标的币 | Contract 模型的 `quanto_multiplier`「合约乘数」 | 原文 |
| `enable_decimal` 为 true 的合约可以提交小数 `size` | 文档站 changelog v4.106.19、公告 | 原文 |
| 读元数据要带 `X-Gate-Size-Decimal: 1`，否则 ETH 的 `order_size_min` 返回 0 | SDK `GetFuturesContract` 说明、研究时的实测 | 英文原文与实测；中文站只在 changelog 提到该请求头 |
| 市价单最大张数取 `market_order_size_max`，为 0 时取 `order_size_max` | 文档站 changelog v4.106.7、v4.106.8 | 原文 |
| 最小步长等于 `order_size_min` | 公告把「下单张数精度」的调整表述为「最小下单张数」由 1 调为 0.1 | **推论，官方资料没有明文** |
| 交易所层面没有最小下单金额 | 全部来源未见 | 未找到 |
| Contract 模型里没有 `order_size_min` 之外的张数步长或精度字段 | 文档站模型表 48 个字段、SDK 模型、实测响应 | 已确认不存在 |

需要后续验证或留意的三点：

1. 「步长等于 `order_size_min`」是计划文档 D30 的决策。官方资料没有一句话写明“张数必须是它的整数倍”，
   也没有说明不在步长上时交易所是拒绝还是截断。实现从每个合约的元数据读步长，不写死 0.1；
   由子任务 4 的 W3（ETH 以 0.1 张真实开平仓）实测确认。
2. 公告写明 `X-Gate-Size-Decimal` 计划废弃：默认将改为返回字符串，请求头随后失效。本实现只按字符串解析，
   两种情况都兼容；`enable_decimal` 为 true 而步长读成 0 时报错，不回落到 1 张。
3. 持仓的 `size` 同样受这个请求头影响。分批平仓读持仓张数时必须用带请求头读到的值（SDK 默认带）。

## 数据结构

```go
// ContractSpec 下单数量换算所需的合约元数据；构造后不可变，方法可并发调用
type ContractSpec struct {
	Name               string // 合约名，如 ETH_USDT
	QuantoMultiplier   string // 一张合约对应的标的币数量
	SizeStep           string // 最小下单张数，也是张数的最小步长
	SizeMax            string // 单笔订单的最大张数（order_size_max）
	MarketOrderSizeMax string // 市价单的最大张数；交易所未设置为 "0"，此时按 SizeMax 计
}

func NewContractSpec(name, quantoMultiplier, orderSizeMin, orderSizeMax, marketOrderSizeMax string, enableDecimal bool) (ContractSpec, error)
```

导出字段保留接口返回的原始字符串，供展示与日志使用；解析后的有理数放在未导出字段里，
只能经 `NewContractSpec` 写入。构造时逐项校验，任何一项不合法都返回错误：

- 名称非空；`quanto_multiplier`、`order_size_min`、`order_size_max` 必须是正的十进制数
- `enable_decimal` 为 false 时步长必须是整数
- `order_size_max` 不小于步长；`market_order_size_max` 留空或 0 表示未设置，否则不小于步长
- 市价单可用的最大张数取 `order_size_max` 与 `market_order_size_max` 中较小者

张数以十进制字符串输出（`"3"`、`"0.6"`），小数位数等于步长的小数位数，可以直接作为下单接口的 `size`。

## 换算规则

### 名义价值换算为张数

设参考价为 p，每张的币数量为 m，步长为 s，名义价值为 N，最小下单金额为 A：

- 一个最小步长的名义价值：v = p × m × s
- 最少步数：k = max(1, ⌈A ÷ v⌉)；最小可交易额 = k × v
- 步数：n = ⌊N ÷ v⌋

| 条件 | 结果 |
|---|---|
| n = 0 | 不下单：不足一个最小步长 |
| n < k | 不下单：取整后低于最小下单金额 |
| n × s 超过市价单最大张数 | 不下单：超过最大张数 |
| 其余 | 张数 = n × s，取整后的名义价值 = n × v |

两种“不下单”的先后：先判断步长，再判断最小下单金额。名义价值连一个步长都不够时，即使最小下单金额为 0
也下不了单，所以它是更根本的原因。不能下单时 `Contracts` 留空，调用方不会误用。

### 分批平仓比例换算为张数

持仓张数 P（取绝对值）必须是步长的正整数倍，比例 r 取 (0, 100]：

- 平仓步数 = ⌊P ÷ s × r ÷ 100⌋
- 平仓步数为 0：报 `ErrCloseBelowStep`（换算后不足一个最小步长）
- 平仓步数等于持仓步数：返回 `Full = true`，这是全平而不是分批
- 其余：平仓张数 = 步数 × s，剩余 = 持仓 − 平仓

### 币数量与张数互转

- 张数 → 币数量：张数必须在步长上，乘以 m
- 币数量 → 张数：除以 m，结果必须落在步长上，否则报错。不截断，也不把不足 1 张改成 1 张

## 精确十进制

- 换算全程用标准库 `math/big.Rat`，不引入新的直接依赖。解析十进制字符串得到的是精确值（`"0.0001"`
  即 1/10000），乘除与取整都是精确的
- `float64` 入参按最短十进制表示取值：`0.3` 就是 3/10，`8.6 ÷ 0.1` 得 86。直接用浮点除法会得
  `85.99999999999999`，截断后少 1 张；旧实现 `int64(quantity / quantoMultiplier)` 在
  `0.0003 ÷ 0.0001` 上得 2 而不是 3（F23）
- 币数量常由“张数 × 每张币数量”的浮点乘法得来，带着乘法误差（`3 × 0.0001` 是
  `0.00030000000000000003`）。`ContractsFromQuantity` 吸收 10⁻⁹ 张以内的误差，超出即报错；
  这个容差远小于任何最小步长，也远大于浮点误差

## 元数据读取与缓存

`GetContractSpec(symbol)` 经 SDK 读取 `GET /futures/usdt/contracts/{contract}`（公开接口），缓存 10 分钟：

- 有效期内不重复请求；读取失败与元数据不合法都不缓存
- 交易所调整过合约的张数精度，所以不永久缓存
- 没有复用现有的 `getContract`：它永久缓存，且被现有下单路径依赖。本任务不改它的行为，
  子任务 4 接入时再统一

## 验证

- 单测矩阵：元数据校验、名义价值换算、浮点掉格的对照用例、分批比例、币数量互转、缓存行为
- 计划文档第 6.1 节两张表（下单位置表、当前本金下各合约所需百分比）用同一组价格逐格复算
- 7 个合约元数据的真实只读读数：解析结果与 SDK 原始字段逐项对照，输出留档 `probe/`

## 已知边界

- 价格是参考价。市价单的实际成交价会有滑点，实际名义价值随之变化
- 比例小于 100% 时平仓量向下取整后必然小于持仓，剩余至少一个最小步长。因此 D31 说的“剩余部分不足一个
  最小步长”只出现在比例为 100% 的情况，此时返回 `Full`。子任务 10 据此决定是拒绝分批动作还是改走全平
- `Orderable()` 是唯一的可用性判断。`Notional`、`MinOrderValue` 是 `float64`，只用于展示与日志
