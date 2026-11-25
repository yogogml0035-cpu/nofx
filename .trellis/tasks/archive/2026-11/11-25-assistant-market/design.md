# 技术设计：⑮ assistant-market

## 边界与职责

- `market`：扩展 Gate ticker（24h 高低/绝对涨跌），新增 `GetGateContractMetrics`（标记价/
  指数价/资金费率/下次结算/持仓量 USD）；删除旧内嵌行情路径与其结构。
- `agent`：所有行情取数经过新增的 `agent/market_source.go` 汇聚到 Gate（包级变量作测试缝）；
  工具/简报/告警/实时摘要改写；删除两个路由与 Binance 取数。
- `kernel`：删除提示词中的旧内嵌格式分支。

## 市场层契约

```go
// market/gate_market.go 扩展
type GateTicker struct {
    Contract string; Symbol string
    Last, ChangePercent24h, Change24h float64
    High24h, Low24h, QuoteVolume24h float64
}

// 标记价/指数价/资金费率/下次结算时间（毫秒）与持仓量（USD）
type GateContractMetrics struct {
    MarkPrice, IndexPrice, FundingRate float64
    NextFundingTime int64 // ms；Gate funding_next_apply 为秒
    OpenInterestUSD float64
}
func GetGateContractMetrics(symbol string) (GateContractMetrics, error)
```

- ticker 的 `high_24h/low_24h/change_price` 为可选字段（新合约可能缺省 → 0，不让整个列表失败）；
  `last`、`change_percentage`、`volume_24h_quote` 仍严格必填。
- **30d 小 limit 的 Gate 行为（2026-11-25 实测）**：`interval=30d` 且 limit 较小（如 50）时，
  Gate 返回非月初窗口并夹杂无成交占位根（价格 0.1268、v=0、sum=0）；limit=200/2000 时是
  月初对齐的真实月线。`GetGateKlines` 对 30d 小 limit 先按上限 2000 拉全历史，再截取最后
  limit 根（1w/7d 小 limit 实测正常，不做特殊处理）。
- `GetGateContractMetrics`：`GET /futures/usdt/contracts/{contract}` 取标记/指数/资金费率/
  下次结算；`contract_stats` 取最新 `open_interest_usd`；任一失败即错误（快照 fail-closed）。

## agent 汇聚层（`agent/market_source.go`）

```go
var (
    assistantGetTicker    = market.GetGateTicker
    assistantListTickers  = market.ListGateTickers
    assistantGetKlines    = market.GetGateKlines
    assistantGetMetrics   = market.GetGateContractMetrics
)
```

- 包级变量是测试缝：agent 包测试必须保持非并行（与 ⑭ 的 handler 缝同理）。
- `buildMarketSnapshot(symbol, interval string, ticker, metrics, klines) map[string]any`：
  纯函数，保留原 JSON 结构：
  - `ticker_24h`: `price_change`（`change_price`）、`price_change_percent`、`high_price`、
    `low_price`、`volume`/`quote_volume` 都是 USDT 成交额（D6），不再输出不可得的 `trade_count`。
  - `perp_metrics`: `mark_price`、`index_price`、`funding_rate`、`next_funding_time`（毫秒）、
    `open_interest`（USD）。
  - `kline_snapshot`: `open_time`/`close_time` 毫秒、`volume` USDT；其余字段不变。

## 工具与接口

| 位置 | 处理 |
|---|---|
| `agent/tools.go` `toolGetMarketSnapshot` | 用 seam 取 ticker/metrics/klines → 纯函数组装 |
| `agent/tools.go` `toolGetKline` | `assistantGetKlines`；`validKlineInterval` 改为 D25 集合；limit ≤300 |
| `agent/tools.go` tool schema 周期枚举 | 改为 D25 集合（含 10s/7d/1w/30d） |
| `agent/tools.go` `binanceFuturesGET`/`binanceFuturesAPIBaseURL` | 删除（无其他调用方） |
| `agent/web.go` `HandleKlines`/`HandleTicker`/`proxyBinance`/`binanceClient` | 删除 |
| `agent/web.go` `HandleTickers` | 一次 `assistantListTickers` + 过滤；输出前端现有字段 |
| `api/agent_routes.go` | 删除 `/api/agent/klines`、`/api/agent/ticker`；保留 tickers |
| `agent/sentinel.go` | 用 `assistantGetTicker`；volume=USDT 成交额、涨跌=ChangePercent24h |
| `agent/brain.go` | BTC/ETH 简报用 `assistantGetTicker` |
| `agent/agent.go` | `market.Get` → `GetWithTimeframes(sym, {"5m","4h"}, "5m", 20)`；无 AI 兜底改本地摘要 |

## 旧 market 路径删除

| 位置 | 处理 |
|---|---|
| `market.Get`、`GetWithExchange`、`Format`、`BuildDataFromKlines` | 删除（调用方全部切换后） |
| `market.IntradayData`/`LongerTermData`、`Data.IntradaySeries`/`LongerTermContext` | 删除 |
| `market/data_klines.go` `calculateIntradaySeries`/`calculateLongerTermData` | 删除 |
| `kernel/engine_prompt.go` 旧格式分支（Intraday/LongerTerm 渲染） | 删除 |
| 只为上述结构服务的测试 | 删除；保留 TimeframeSeries/isStaleData/Box 测试 |

## 权衡与边界

- **USDT 成交量**：Gate 的 `volume_24h` 是合约张数，跨合约不可比；ticker 的 `volume` 字段与
  `quote_volume` 都取 USDT 成交额（D6），模型与前端看到的是同一量纲。
- **`trade_count` 不可得**：Gate ticker 没有成交笔数，直接省略而不是填 0。
- **OI 缺失即失败**：快照工具旧实现对 OI 取数失败返回错误；保持 fail-closed，不用 0 冒充。
- **ticker 列表跳过异常合约**：Gate 的 1042 个合约里若个别新上架合约字段缺失（如 last=0），
  `ListGateTickers` 跳过该条并记录数量，只有全部条目都不可解析时才报错；避免一个异常合约
  让助手 24h 行情整体 502（2026-11-25 实测全部合约正常）。
- **助手币种判定**：`get_market_snapshot`/`get_kline` 不再按 `isStockSymbol` 的启发式提前拒绝；
  先按 Gate 取数，失败且符号形似股票（1–5 位纯字母）时才返回“仅支持加密币种”提示，避免
  HYPE/GOAT/BERA 等不在硬编码清单里的 Gate 合约被误判为股票。
- **无 AI 兜底价格精度**：按价格量级动态选择小数位（旧 `market.Format` 的精度口径），避免
  极小价格显示为 0。
- **网格 EMA50**：旧值来自 4h `LongerTermContext`；改为优先读 4h `TimeframeData` 的
  EMA50，缺失时退回 5m，保持原口径。
- **`GetWithTimeframes` 仅 200 根**：助手实时摘要只需价格/指标；K 线工具单独按请求 limit 取数。
- **股票/外汇**：不在本子任务；Alpaca/TwelveData 工具与路由不动。