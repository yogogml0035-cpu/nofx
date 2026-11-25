# 子任务 ⑬ PRD：引擎 K 线切换（engine-klines-gate）

父任务 `11-24-gate-trading-foundation`。依据：计划 §8.5、D1/D6/D9/D10/D25、F41，以及 ⑫ 的
`market/gate_market.go`（`GetGateKlines` 等）。本文件列需求与验收；设计见 `design.md`，
顺序见 `implement.md`。

## Goal

把引擎决策 K 线、网格长周期箱体、以及旧内嵌行情路径的取数全部改到 Gate；删除市场层的
CoinAnk K 线代码与无调用方的 CoinAnk 实时/深度代码；修正提示词对周期与成交量的表述。
父分支在每一步都必须可编译（⑭⑮ 尚未完成的删除留给各自子任务）。

## Requirements

1. **决策 K 线**：`market.GetWithTimeframes` 的每个周期改用 `GetGateKlines`（成交量变为
   USDT 成交额，D6）；周期使用 D25 保留集合，`10s/7d/30d` 必须可用（F41：⑧ 已开放这些
   选项但旧取数链路不支持）。
2. **网格箱体**：`market.GetBoxData` 的 1h × 500 根改用 Gate。
3. **旧内嵌路径**：`market.Get`/`GetWithExchange` 的取数改为 Gate（5m 日内序列 + 4h 长周期），
   保持返回结构不变，供助手在 ⑮ 切换前继续使用；函数注释明确“仅助手旧路径，⑮ 删除”。
4. **删除 CoinAnk K 线（市场层）**：删除 `getKlinesFromCoinAnk`、`parseTimeframeToMinutes`
   的旧周期表（改为按 Gate 周期时长计算）、`market/timeframe.go`（无生产调用方）与对应测试。
   `provider/coinank/coinank_api/kline.go` 仍被 ⑭ 的图表接口使用，本子任务不删；其无调用方的
   `kline_ws.go`、`depth_ws.go` 与测试删除。
5. **删除死代码**：`kernel.StrategyEngine.FetchMarketData`（无调用方）。
6. **提示词**：时间周期输出顺序改为 D25 保留集合（`10s..30d`），新周期策略的 K 线必须出现在
   提示词里；K 线表的成交量列标明 USDT 成交额；数据字典/文档中的成交量口径同步。
7. **回退兜底**：`api/strategy.go` 在未配置周期时的 `"3m"` 兜底改为 `"5m"`（D25）。
8. **不做**：不改 ⑭ 的 `api/handler_klines.go` 与前端；不改 ⑮ 的 agent 取数、`market.Format`
   与 `/api/agent/*`；不删除 `provider/coinank/coinank_api/kline.go`。

## Acceptance Criteria

1. 单测：`GetWithTimeframes`/`GetBoxData`/`GetWithExchange` 在 Gate mock 下正确取数（含
   10s/7d/30d、成交量=sum、失败闭合）；提示词周期顺序与成交量列；删除后无残留引用。
2. 真实取数：12 个保留周期逐一经 `GetWithTimeframes` 成功（7d/30d/10s 含在内），箱体数据为
   Gate 1h；无 AI 的提示词渲染显示 Gate 价格与 USDT 成交量。
3. `git diff --check`、`make test`、`make build` 通过；独立检查子智能体 PASS。