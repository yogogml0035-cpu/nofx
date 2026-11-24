# 张数换算：USDT 名义价值换算合约张数、分批比例换算张数、合约元数据读取

父任务：`.trellis/tasks/11-24-gate-trading-foundation/`（计划文档第 10.2 节子任务 2）。本任务只做纯新增的
纯函数与只读适配，不接入交易路径；合入后运行中的 trader 行为不变。

## Goal

为开仓路径加固（子任务 4）、回撤监控（6）、金额计算（9）与分批平仓（10）提供 Gate 合约张数的换算底座：

- 把以 USDT 表达的下单金额换算成可以直接提交给 Gate 的合约张数
- 把分批平仓的比例换算成张数
- 提供下单换算所需的合约元数据

## Requirements

1. **合约元数据读取**：从 `GET /futures/usdt/contracts/{contract}` 读取 `quanto_multiplier`、`order_size_min`、
   `order_size_max`、`market_order_size_max`、`enable_decimal`，带 `X-Gate-Size-Decimal: 1` 请求头。
   最小步长取 `order_size_min`（整数张合约为 1 张，小数张合约按接口返回的值，如 0.1 张）。
   官方资料没有写明张数必须是它的整数倍，这是 D30 的推论，由子任务 4 的 W3 实测确认。
   读取结果带缓存，过期后重新读取；读取失败不缓存。
2. **USDT 名义价值换算为下单张数**（D24、D30）：
   - 张数 = 名义价值 ÷（价格 × `quanto_multiplier`），按该合约的最小步长向下取整
   - 取整后不足一个最小步长则不下单
   - 取整后的名义价值低于最小下单金额则不下单（最小下单金额由调用方传入，策略配置默认 10 USDT，D19）
   - 超过市价单的单笔最大张数则不下单（`market_order_size_max`，未设置时取 `order_size_max`）
   - 结果张数以十进制字符串给出，可直接作为下单接口的 `size`；同时给出取整后的名义价值
   - 给出该合约当前的最小可交易额：同时满足最小步长与最小下单金额的最小下单名义价值
3. **分批平仓比例换算为张数**（D31）：持仓张数 × 比例（0–100%），按最小步长向下取整。
   - 比例 100% 为全平
   - 换算后不足一个最小步长则拒绝
   - 剩余部分不足一个最小步长时不算分批：向下取整只会让平仓量变小，所以这种情况只在比例为 100% 时出现，
     此时给出全平标志而不是分批结果，由调用方决定拒绝分批动作还是改走全平
4. **标的币数量与张数互转**（F23）：数量 ÷ `quanto_multiplier` 用精确十进制计算，
   `0.0003 ÷ 0.0001` 得 3 张。结果必须落在最小步长上，否则报错，不截断、不强制成 1 张。
5. **精确十进制**（F3、F23）：换算过程不使用浮点除法截断。以 `float64` 传入的数值按其最短十进制表示
   参与计算，避免 `0.0003 / 0.0001` 得 2 这类误差。
6. **解析失败即报错**：元数据缺字段、不是合法十进制数、自相矛盾（例如合约支持小数张而最小步长读成 0）
   一律返回错误并说明原因，不回落到猜测值。

## Constraints

- 纯新增：不改 `OpenLong`、`OpenShort`、`CloseLong`、`CloseShort`、`SetStopLoss`、`SetTakeProfit`、
  `getContract` 等现有函数的行为。旧的截断与强制 1 张逻辑由子任务 4 在接入时删除。
- 不发任何真实写请求；真实接口验证只用公开的合约查询接口。
- 不引入新的直接依赖。
- 验证方式：单元测试矩阵、7 个合约（BTC、ETH、SOL、ZEC、HYPE、DOGE、XAU）元数据的真实只读读数。

## Acceptance Criteria

- [x] 元数据解析矩阵：整数张合约、小数张合约、缺少小数张请求头导致 `order_size_min` 为 0、字段为空或非数字、
      `enable_decimal` 与步长不一致、最大张数小于步长、市价单上限未设置、小于或大于单笔上限
      ——`TestNewContractSpec`
- [x] 名义价值换算矩阵：整数张向下取整、小数张向下取整、不足一个最小步长、取整后低于最小下单金额、
      恰好等于最小下单金额、超过市价单最大张数、最小可交易额 ——`TestContractSpec_SizeForNotional`
- [x] 浮点会算错的边界按十进制语义给出正确结果：商恰为整数时不掉一格，`0.0003 ÷ 0.0001` 得 3
      ——`TestContractSpec_SizeForNotionalAvoidsFloatTruncation`、`TestContractSpec_ContractsFromQuantity`
- [x] 计划文档第 6.1 节两张表（下单位置表、当前本金下各合约所需百分比）用同一组价格复算一致
      ——`TestContractSpec_SizeForNotionalMatchesPlanTable`、`TestContractSpec_MinOrderablePercentMatchesPlanTable`
- [x] 分批比例矩阵：全平、可分批、换算后不足一个最小步长、持仓张数不在步长上、比例越界；
      比例小于 100% 时剩余至少一个最小步长 ——`TestContractSpec_SizeForCloseRatio` 及其性质测试
- [x] 元数据缓存：有效期内不重复请求，过期后重新请求，请求失败与元数据不合法都不缓存
      ——`TestGateTrader_GetContractSpec*`（含 64 个 goroutine 的并发调用）
- [x] 7 个合约元数据真实读数：解析结果与接口原始字段逐项一致，每个合约给出当前价格下的最小可交易额；
      输出留档到本任务 `probe/`（代码提交 `62ab50a5`，`contracts=7 failed=0 mismatched=0`）
- [ ] `git diff --check`、`make test`、`make build` 通过，现有测试不做修改

## Non-Goals

- 把换算接入开仓、平仓、保护单、回撤监控的调用路径（子任务 4、6、10、11）
- 策略配置的百分比与最小下单金额字段（子任务 8）、金额计算与亏损上限校验（子任务 9）
- 市价单滑点、下单取价来源（子任务 4）
