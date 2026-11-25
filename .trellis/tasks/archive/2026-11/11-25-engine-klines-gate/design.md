# 技术设计：⑬ engine-klines-gate

## 边界与职责

- `market`：把 K 线取数切到 ⑫ 的 `GetGateKlines`；删除市场层 CoinAnk K 线代码与无调用的
  CoinAnk 实时/深度代码；保留 `Get`/`GetWithExchange` 的结构供 ⑮ 删除。
- `kernel`：提示词的周期顺序与成交量口径；删除无调用方的 `FetchMarketData`。
- `api`：仅改 test-run 的 `"3m"` 兜底（⑭ 的图表接口不动）。
- `provider/coinank/coinank_api`：删除 `kline_ws.go`、`depth_ws.go` 与测试；`kline.go` 保留给 ⑭。
- 不触碰 `agent` 的取数；`market.Format` 只同步文案口径（结构留给 ⑮ 删除）。

## 契约与改动点

### market 取数

```go
// GetWithTimeframes：每个周期一行
klines, err = GetGateKlines(symbol, tf, 200)   // 原 getKlinesFromCoinAnk(symbol, tf, "binance", 200)

// GetBoxData
klines, err := GetGateKlines(symbol, "1h", LongBoxPeriod)

// GetWithExchange（旧内嵌路径，仅供助手到 ⑮）
klines5m, err := GetGateKlines(symbol, "5m", 100)
klines4h, err := GetGateKlines(symbol, "4h", 100)
// 1h 涨跌：calculatePriceChangeByBars(klines5m, "5m", 60)
```

- `Get`/`GetWithExchange` 的 `exchange` 参数保留但不再使用（D1：只走 Gate），注释说明。
- `calculatePriceChangeByBars` 改用 `GateIntervalDuration` 计算 barsBack（秒级口径），
  不再使用 `parseTimeframeToMinutes` 的旧周期表；`10s` 等周期也能得到合理的回看根数
  （不足时按可用窗口）。
- 数据陈旧检测（`isStaleData`）保留，但判定窗口按 K 线间隔换算为至少 15 分钟（最少 5 根）：
  10s 需要 90 根、1m 需要 15 根，避免短周期下 50 秒无成交就误判数据源异常；30d 等长周期仍以
  5 根为下限（15 分钟 / 30 天 = 0）。
- `market.Format`（旧内嵌路径的展示）文案同步为 5m / USDT 成交额，避免助手在 ⑮ 删除前误标；
  结构本身仍由 ⑮ 删除。

### 提示词

- `formatMarketData` 的 `timeframeOrder` 改为 `{10s,1m,5m,15m,30m,1h,4h,8h,1d,7d,1w,30d}`。
- K 线表头 `Volume` → `Volume(USDT)`；旧内嵌分支的 `Volume` 文案加 `(USDT)`。
- 系统提示词的指标说明 `Volume data` → `Volume data (USDT quote volume)`（中英）。

### 删除清单

| 位置 | 处理 |
|---|---|
| `market/data_klines.go` `getKlinesFromCoinAnk` 与其 CoinAnk 映射 | 删除；`data_klines.go` 不再 import coinank |
| `market/data_klines.go` `parseTimeframeToMinutes` | 删除（改用 Gate 周期时长） |
| `market/timeframe.go` + `timeframe_test.go` | 无生产调用方，删除 |
| `provider/coinank/coinank_api/kline_ws.go` + `kline_ws_test.go` | 无调用方，删除 |
| `provider/coinank/coinank_api/depth_ws.go` + `depth_ws_test.go` | 无调用方，删除 |
| `kernel/engine.go` `FetchMarketData` | 无调用方，删除 |
| `api/strategy.go` `"3m"` 兜底 | 改 `"5m"` |

保留：`market.Get`/`GetWithExchange`、`IntradaySeries`/`LongerTermContext`、
`calculateIntradaySeries`/`calculateLongerTermData`、`market.Format`（⑮ 删除）；
`provider/coinank/coinank_api/kline.go`（⑭ 切换后删除）。

## 数据流

```
kernel engine ── GetWithTimeframes ──▶ GetGateKlines × N 周期 ──▶ TimeframeData ──▶ 提示词
grid regime  ── GetBoxData ──────────▶ GetGateKlines 1h × 500 ──▶ BoxData
assistant legacy (到 ⑮) ── Get/GetWithExchange ──▶ GetGateKlines 5m/4h ──▶ Intraday/LongerTerm
```

## 权衡

- **为什么不删 Get/GetWithExchange**：`agent/agent.go` 仍在调用；在 ⑬ 删除会让父分支无法编译。
  改为 Gate 供数，⑮ 切换助手后删除结构（计划 §8.5 “以切换后无调用方为准”）。
- **为什么 kline.go 留给 ⑭**：`api/handler_klines.go` 仍直接调用 `coinank_api.Kline`；⑭ 切换
  后删除。⑬ 只删无调用方的 ws/depth 代码。
- **成交量语义变化**：`KlineBar.Volume` 从合约张数变为 USDT 成交额（D6）；提示词表头显式标注，
  避免模型把张数当成交额。指标计算只用价格，不受影响。
- **周期顺序**：提示词必须包含 10s/7d/30d，否则 ⑧ 开放的周期在提示词里不可见（F41）。

## 已知边界

- `isStaleData` 的窗口是 max(5 根, 15 分钟)：10s 需要 90 根、1m 15 根、30d 仍 5 根；短周期不再
  因为 50 秒无成交被误判。
- `GetWithExchange` 的 `exchange` 参数被忽略；其他交易所的 trader 仍用 Gate 分析行情（D26）。
- 30d 的最后一根 CloseTime 是近似值（⑫ 已注明）；引擎只用 OpenTime 排序与展示，不做收线判断。
- 主周期与 1h/4h 目标窗口不匹配时，`calculatePriceChangeByBars` 退回可用窗口：10s 主周期最多约
  33 分钟（200 根），7d/30d 主周期退化为“1 根前”。提示词字段名沿用 1h/4h，读数是可用窗口内的
  近似值而非真实 1h/4h；这是本子任务的已知边界（父计划 F46 留档）。