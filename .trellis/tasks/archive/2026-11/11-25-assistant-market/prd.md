# 子任务 ⑮ PRD：助手行情切换（assistant-market）

父任务 `11-24-gate-trading-foundation`。依据：计划 §8.5、D1/D6/D11/D25、F41；⑫ 的
`market.GetGateKlines`/`GateTicker`。本文件列需求与验收；设计见 `design.md`，顺序见
`implement.md`。

## Goal

助手（agent）的全部行情取数改为 Gate：24h ticker、市场快照、K 线工具、定时简报、价格告警；
删除 `/api/agent/klines`、`/api/agent/ticker` 与所有 Binance 行情取数；删除只服务于助手旧路径
的 `market.Get`/`GetWithExchange`/`Format` 与旧内嵌序列结构。

## Requirements

1. **24h 行情**：`/api/agent/tickers` 改 Gate（一次 `ListGateTickers` + 过滤请求的 symbol），
   返回前端现有字段（symbol/lastPrice/priceChangePercent/highPrice/lowPrice/volume），
   成交量为 USDT 成交额（D6）。
2. **市场快照 `get_market_snapshot`**：价格、24h 高低/涨跌/成交额、标记价/指数价/资金费率/
   下次结算、持仓量（USD）与近期 K 线全部来自 Gate；输出字段保持现有 JSON 结构（成交量口径
   标 USDT、持仓量为 USD、`trade_count` 不可得则省略）。
3. **K 线工具 `get_kline`**：改 Gate 合约 K 线；周期只接受 D25 保留集合
   `{10s,1m,5m,15m,30m,1h,4h,8h,1d,7d,1w,30d}`；`open_time/close_time` 为毫秒；成交量=USDT
   成交额；工具 schema 的周期枚举同步。
4. **简报与价格告警**：`Brain` 的 24h 简报（BTC/ETH 价格与涨跌）与 `Sentinel` 的价格/成交额/
   涨跌采样改 Gate ticker。
5. **助手实时行情摘要与无 AI 兜底**：`agent/agent.go` 的两处 `market.Get` 改用
   `GetWithTimeframes`（Gate）；无 AI 兜底的 `market.Format` 改为本地简洁摘要。
6. **删除**：
   - `/api/agent/klines`、`/api/agent/ticker` 路由与 `HandleKlines`/`HandleTicker`/`proxyBinance`/
     `binanceClient`（无其他调用方时）。
   - `agent` 内所有 `fapi.binance.com` 取数与 `binanceFuturesGET`/`binanceFuturesAPIBaseURL`。
   - `market.Get`、`GetWithExchange`、`Format`、`IntradayData`、`LongerTermData`、
     `BuildDataFromKlines`、`calculateIntradaySeries`、`calculateLongerTermData`、只为它们服务的
     测试；`kernel/engine_prompt.go` 的旧格式渲染分支。
7. **保留**：股票（Alpaca）价格工具 `search_stock`、交易/账户工具；`/api/agent/tickers` 路由保留。

## Non-goals

- 不改助手对话流程、planner、skill 文案（除行情工具描述与周期枚举）。
- 不改交易路径/引擎（⑬ 已完成）与图表 API（⑭ 已完成）。

## Acceptance Criteria

1. 单测：ticker 过滤与字段映射、快照组装（含 null OI 语义）、K 线周期拒绝、简报/告警用 Gate、
   handler 删路由后不受影响；`go build ./...` 证明无 Binance/旧结构残留引用。
2. 真实调用：Gate ticker/24h/快照/K 线工具的真实取数探针 `PROBE_PASS`（含 10s/7d/30d）。
3. `git diff --check`、`make test`、`make build` 通过；独立检查子智能体 PASS。