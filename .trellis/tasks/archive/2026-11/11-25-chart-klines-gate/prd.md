# 子任务 ⑭ PRD：图表 K 线切换（chart-klines-gate）

父任务 `11-24-gate-trading-foundation`。依据：计划 §8.5、D1/D6/D9/D25、F41；⑫ 的
`market.GetGateKlines`。本文件列需求与验收；设计见 `design.md`，顺序见 `implement.md`。

## Goal

`/api/klines` 的加密货币 K 线改为 Gate 直连，删除 CoinAnk 交易所映射、Binance 回退与秒级/
非保留周期；前端图表去掉 `exchange` 参数与旧成交量口径。股票（Alpaca）、外汇与金属
（TwelveData）路由保留。

## Requirements

1. **后端**：`handleKlines` 的默认/加密货币分支调用 `market.GetGateKlines(symbol, interval, limit)`；
   删除 `getKlinesFromCoinank` 与全部 CoinAnk 交易所/周期映射；`exchange` 对加密货币不再影响
   取数（只走 Gate，D1）；`limit` 限制在 1..2000。
2. **周期**：只接受 D25 保留集合 `{10s,1m,5m,15m,30m,1h,4h,8h,1d,7d,1w,30d}`；旧
   `1s/5s/30s/3m/10m/2h/6h/12h/3d/1M` 一律拒绝（Gate 取数层已校验）。
3. **成交量**：API 返回的 `volume` 与 `quoteVolume` 都是 USDT 成交额（Gate `sum`），图表
   tooltip 只展示 `V(USDT)`，柱状图用该值（D6）。
4. **前端**：`AdvancedChart`/`ChartWithOrders`/`ChartTabs`/调用方去掉 `exchange` 参数、URL 中的
   `exchange=`、默认 `binance` 与交易所徽标；删除无调用方的 `TradingViewChart`（旧交易所列表）。
   图表周期选项保留现有 1m/5m/15m/30m/1h/4h/1d（都在保留集合内）；删除无调用方的
   `TradingViewChart`、`ChartWithOrders`、`ChartWithOrdersSimple`（旧交易所/成交量口径）。
5. **CoinAnk 清理**：删除 `provider/coinank/coinank_api/`（K 线 + 符号归一/基础币）与
   `provider/coinank/kline.go`（K 线能力）；保留 `provider/coinank` 的其他数据能力与
   `coinank_enum`（D10）。
6. **保留**：Alpaca（`exchange=alpaca`）、TwelveData（`exchange=forex|metals`）分支与
   `/api/symbols`（已是 Gate）不变。

## Non-goals

- 不改策略编辑器周期（⑧ 已完成）、`/api/symbols`、助手行情（⑮）。
- 不重构图表渲染逻辑，只改数据来源参数与成交量单位。

## Acceptance Criteria

1. 后端单测（Gate mock）：无参/`binance`/`gate` 都走 Gate 且 URL 正确；`limit` 上限 2000；
   Gate 失败 → 500；非保留周期被拒；Alpaca/TwelveData 路由保留（代码审查）。
2. 前端：`npm test`、`tsc --noEmit`、`vite build` 通过；K 线 URL 不含 `exchange=`；图表无
   交易所徽标；tooltip 只剩 `V(USDT)`；无对 `TradingViewChart` 的引用。
3. `git diff --check`、`make test`、`make build` 通过；独立检查子智能体 PASS。