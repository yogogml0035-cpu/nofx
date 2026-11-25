# PRD：金额计算（⑨ sizing-engine）

父任务：`11-24-gate-trading-foundation`；子任务地图第 9 步；开工前置：⑥ `drawdown-monitor`、⑦ `account-equity` 已合父（均已归档）。
需求权威来源：`docs/plans/2026-11-24-gate-trading-foundation.zh-CN.md`（下称计划文档）§5、§8.3、§10.2 第 9 行、D12/D18/D19/D22/D23/D24/D30/D34、F33。

## 背景

⑧ 已把配置改成「开仓金额百分比 `position_percent`（1%–20%，步长 0.1，默认 10）」与「最小下单金额默认 10」。但金额仍由模型在决策 JSON 里自报（`position_size_usd`），kernel 还保留旧语义的三处硬编码：两个最小下单门槛（12 / 60 USDT）、盈亏比 ≥3:1 的代码校验，以及提示词「仓位大小指引」要求模型填金额。D12 已确认盈亏比只靠提示词；D22/D24 确认名义价值由代码计算。

## 需求

1. **金额由代码计算**：开仓名义价值 = 本金 × `position_percent`。本金取 `types.ReadAccountBalance` 的 `Principal`（⑦ 口径），经 `kernel.AccountInfo.Principal` 进入决策上下文；换算按 F33 的算法（本金 × 百分比的十分位整数 ÷ 1000），不把 `percent/100` 的浮点噪声带进结果。计算结果由代码写入决策并交给执行路径。
2. **决策契约去掉金额字段**：模型输出的开仓决策不再包含 `position_size_usd`、`risk_usd`；`kernel.Decision` 删除这两个 JSON 字段，新增代码填充的 `NotionalUSD`（`json:"-"`，模型给了也忽略）。开仓必填字段降为 leverage、stop_loss、take_profit、confidence。
3. **开仓校验（kernel）**：删除硬编码最小下单 12/60 与盈亏比 ≥3:1 代码校验（D12）；保留杠杆上限回落；新增
   - 本金与百分比有效（本金不大于 0 时开仓决策拒绝，fail-closed）；
   - 止损与止盈必须分居当前价两侧（多：SL < 现价 < TP；空：TP < 现价 < SL），否则随单条件单方向非法；
   - 价格到止损价时的亏损 = 名义价值 × |现价 − 止损价| ÷ 现价 ≤ 本金的 2%（D34），超过即拒绝；等于 2% 允许。
4. **提示词改为代码金额口径**：硬约束写明「开仓名义价值 = 本金 × 百分比，由代码计算并强制」与「到止损价的亏损 ≤ 本金 2%，代码强制」；删除输出示例与字段说明里的金额字段；「仓位大小指引」改写为模型只决定方向/杠杆/止损/止盈/置信度，不输出、也不要自行缩放金额。中英两版同步。
5. **本金进入提示词与试运行**：`BuildSystemPrompt` 的金额参数改按本金；`/strategies/preview-prompt` 的请求字段与 `/strategies/test-run` 的模拟账户改用本金口径（无持仓时本金 = 权益）。
6. **助手诊断口径**：删除「position_size_usd 是 AI 决策输出」相关表述与证据解析；不开单诊断里的预期开仓金额改为本金 × 百分比；保留「调高百分比 / 调整最小开仓金额 / 增加资金」等真实可改项。
7. **旧语义一次性删除**：删除只有测试调用的旧提示词构建器 `kernel/prompt_builder.go` 与其专用的 `kernel/formatter.go`（仍携带 `position_size_usd` 契约与 HOLD/OPEN_NEW 旧动作集合）；同步 `store` 注释、架构文档、提示词指南与排障文档（中英）。

## 明确不做（边界）

- 百分比字段、clamp、最小下单默认值、周期枚举：⑧ 已交付，不改。
- 分批平仓、修改止损止盈的执行与契约：⑩⑪；本任务只提供可复用的止损亏损上限校验原语与开仓校验。
- 行情取数（K 线 / ticker）与周期：⑫⑬；本任务只读现有 `MarketDataMap` 的现价做校验。
- 网格交易路径（limit 单按币数量）与助手手动下单（`agent/trade.go` 自己的金额路径）：不改。
- trader 的 `enforceMinPositionSize` 早退检查与 Gate 适配器的取整后最小下单金额校验：保留（⑧ 已把默认值改为 10）。
- 真实写操作：本任务无下单需求，不涉及 W2–W5。

## 验收标准

- [ ] kernel 单测：金额换算（本金 × 百分比，F33 例 `10000 × 1.13% = 113` 不带浮点噪声）、亏损上限边界（2% 内通过、恰好 2% 通过、超出拒绝）、止损止盈方向（多空两侧的正反例）、本金非正拒绝、杠杆回落保留、删除的最小下单/盈亏比旧校验不再拒绝。
- [ ] 决策解析单测：模型 JSON 即使带 `position_size_usd`/`risk_usd` 也被忽略，`NotionalUSD` 恒等于代码计算结果。
- [ ] 提示词单测：中英系统提示词不含 `position_size_usd`/`risk_usd`，含「本金 × 百分比」与「2%」亏损上限；输出示例为合法 JSON 数组且不含金额字段。
- [ ] `/strategies/preview-prompt` 与 `/strategies/test-run`（`run_real_ai=false`）经真实 handler 返回 200，提示词按本金显示代码计算的名义价值。
- [ ] 全仓（除 `.trellis` 归档、计划文档留档与历史数据字段）检索 `position_size_usd|risk_usd|PositionSizeUSD|RiskUSD` 为 0 命中。
- [ ] 门禁：`git diff --check`、`make test`、`make build`；独立检查子智能体 PASS。
- [ ] 不启动 trader、不激活策略；无真实写操作。