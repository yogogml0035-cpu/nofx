# 子任务 ⑫ PRD：行情数据源（market-source）

父任务 `11-24-gate-trading-foundation`。依据：计划文档 §3.3、§8.5、D1/D6/D9/D10/D25、F41；
本文件只列需求与验收，技术设计见 `design.md`，顺序见 `implement.md`。

## Goal

纯新增一个只依赖 Gate 的行情取数层：USDT 永续合约 K 线（保留周期，成交量取成交额）
与 ticker（最新价、24h 涨跌幅、24h 成交额），供给后续 ⑬ 引擎、⑭ 图表、⑮ 助手使用。
本子任务不接入任何调用方，也不删除旧路径（接入与删除在 ⑬⑭⑮ 完成）。

## Requirements

1. **合约 K 线**：`GET /futures/usdt/candlesticks`，内部符号（`BTCUSDT`/`BTC_USDT`/
   `BTC-USDTSWAP`）统一转为 Gate 合约名；周期只接受 D25 的保留集合
   `{10s,1m,5m,15m,30m,1h,4h,8h,1d,7d,1w,30d}`，其余（含旧 `3m/2h/6h/12h/3d`）明确报错。
2. **成交量口径**：用成交额 `sum`（计价币 USDT），不使用合约张数 `v`（D6）。`sum` 缺失或
   不可解析即失败，不静默回落 `v`（会改变量纲）。
3. **时间**：返回的 `Kline.OpenTime` 用毫秒；`CloseTime` 按周期时长计算；严格校验时间戳
   为正且严格递增，乱序或重复即失败。
4. **严格校验**：HTTP 非 200、响应为空、JSON 解析失败、OHLC 缺失/非正、`t` 缺失即失败；
   Gate 数值字段可能是字符串或数字，统一按 `json.Number` 解析。
5. **ticker**：单合约最新价、24h 涨跌幅（百分比数值）、24h 成交额（USDT 计价）；
   另提供全合约列表（供 ⑮ 的 24h 行情接口）。
6. **只走 Gate**：复用 `gateFuturesBaseURL` 与公开行情客户端，没有 CoinAnk/Binance 回退。
7. **不认证**：公开行情接口无需 API key；请求头与超时沿用现有 `NewAPIClient`。

## Non-goals

- 不修改 `market.Get`/`GetWithExchange`/`GetWithTimeframes` 与 `GetBoxData`（⑬ 接入时替换）。
- 不删除 CoinAnk K 线与 `provider/coinank/coinank_api`（⑬⑭⑮ 按各自范围删除）。
- 不改 `api/handler_klines.go`、`agent` 取数、前端（属 ⑭⑮）。
- 不修改 `market/timeframe.go`（无生产调用方，属 ⑬⑮ 清理边界）。

## Acceptance Criteria

1. 单测矩阵：周期映射（12 个保留周期 + 拒绝列表）、`sum` 成交量、时间递增/闭合时间、
   HTTP/JSON/字段错误的 fail-closed、ticker 单条/列表/缺字段。
2. 真实取数：7 个标的（BTC、ETH、SOL、ZEC、HYPE、DOGE、XAU）× 12 个保留周期全部成功；
   每个周期时间间距等于周期时长、最后一根收盘价为正、成交量等于原始 `sum`；ticker 的
   last 与最新 K 线收盘价在容差内一致；不支持的周期被拒。
3. `git diff --check`、`make test`、`make build` 通过；独立检查子智能体 PASS。