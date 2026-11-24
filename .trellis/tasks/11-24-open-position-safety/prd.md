# 开仓路径加固：随单保护、张数换算、成交后核对保护单、缺失即平仓

> 父任务：`11-24-gate-trading-foundation`（需求与决策以 `docs/plans/2026-11-24-gate-trading-foundation.zh-CN.md` 为准，本文件只做子任务范围内的需求固化）。
> 对应计划文档 10.2 节子任务 4，授权写操作见 10.3 节（W2 开仓与全平、W3）。

## Goal

AI 自动交易的开仓路径接入子任务 2（contract-math）与子任务 3（protective-order-adapter）的成果，
使 Gate 上开出的仓位从存在那一刻起就带有止损与止盈（计划文档第 5 节保护不变量）：
保护单随开仓单一并提交，成交后核对保护单存在，不存在则立即平仓并报错。

## Requirements

1. **随单保护**：AI 开仓（open_long/open_short）的下单请求随单附带止盈止损触发价
   （`tpsl_tp_trigger_price`、`tpsl_sl_trigger_price`）；不再在开仓后另行补挂。
2. **张数换算**：开仓名义价值（USDT）按最新价换算为合约张数（十进制字符串、最小步长向下取整，
   `SizeForNotional`）；不足一个最小步长、取整后低于最小下单金额、超过市价单最大张数则不下单并报错（D30）。
3. **取价**：开仓与平仓路径的取价改用交易适配器的 `GetMarketPrice`（Gate 最新价，D8/D26）；
   开仓路径不再调用 `market.GetWithExchange`（CoinAnk 通道）。
4. **滑点上限**：市价开仓单设置 `market_order_slip_ratio`（D8：1%），由调用方传入、适配层校验范围。
5. **成交后核对**：开仓成交后核对保护单存在（随单 tpsl 回读或仓位级条件单列表）；
   缺失则立即市价全平该仓位并报错（保护不变量）。
6. **删除旧语义**：删除开仓后补挂（`SetStopLoss`/`SetTakeProfit` 调用与失败只记日志的路径）、
   开仓路径的 `market.GetWithExchange` 取价、Gate 的旧 `getContract` 及各处按浮点读取
   `quanto_multiplier` 的代码（F34，统一到 `GetContractSpec`）、张数换算的浮点截断与强制 1 张（F3/F23）。
7. **fail-closed**：交易适配器不实现随单保护开仓能力时，AI 开仓直接报错，不允许无保护开仓
   （父任务安全性不变量：父分支不得出现"能开仓但没有有效止损"的状态）。
8. **接口保留**：`types.Trader` 接口签名不变（网格与助手路径继续使用现有方法）；
   Gate 的 `SetStopLoss`/`SetTakeProfit` 因接口占用必须保留方法，但删除旧的错误实现
   （规则相反、`close:true` 带非零 `size`、无 `order_type`），改为明确的不支持错误——
   AI 开仓路径不再经过它们。

## 范围边界

- 不改 kernel 决策契约、金额计算与亏损上限校验（子任务 9）、分批平仓与改单执行（子任务 10、11）、
  杠杆与保证金模式重写（子任务 5）、回撤监控（子任务 6）。
- 不修复网格策略在 Gate 上的挂单能力（17 个子任务无此范围；网格经 `SetStopLoss`/`SetTakeProfit`
  挂单在 Gate 上收到明确的不支持错误，替代旧实现静默的错误规则）。
- 助手手动交易（agent/trade.go）与网格继续使用平仓/开仓原语，本任务只保证原语的张数换算正确。

## Acceptance Criteria

- [ ] 单元测试：随单保护请求构造（方向 × 止损止盈）、名义价值换算调用、不可下单原因透出、
      保护核对三种情形（随单 tpsl 回读有 / 条件单列表补足 / 缺失）、全平 payload（`close=true`+`size=0`）、
      币数量换算向下取整与不足一步报错、`SetStopLoss`/`SetTakeProfit` 不支持错误。
- [ ] AI 开仓路径不再调用 `market.GetWithExchange`；开仓后补挂代码删除。
- [ ] Gate 适配层无 `getContract`、无按浮点解析 `quanto_multiplier` 的代码。
- [ ] 门禁：`git diff --check`、`make test`、`make build` 通过。
- [ ] 真实验证（W2，D35 授权，名义价值 ≤10 USDT、立即还原）：DOGE_USDT 随单保护开多约 17 张 →
      查询确认仓位与止损止盈存在 → 市价全平 → 确认无残留委托与持仓。
- [ ] 真实验证（W3）：ETH_USDT 以 0.1 张（最小步长）开仓并平仓，验证小数张与随单止盈止损。
- [ ] 独立检查子智能体检查通过，发现的问题已修复并补验。
