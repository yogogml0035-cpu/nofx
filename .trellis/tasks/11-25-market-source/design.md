# 技术设计：⑫ market-source

## 边界与职责

- 新增 `market/gate_market.go`：只依赖 Gate 公开行情接口的 K 线与 ticker 取数、解析与校验。
- 复用 `market/gate_stats.go` 的 `gateFuturesBaseURL`、`toGateContractSymbol`（Gate 合约名转换）
  与 `market/api_client.go` 的 HTTP 客户端；不引入认证。
- 不改任何现有调用方；⑬（引擎）、⑭（图表 API）、⑮（助手）在本子任务之后接入。
- 数值字段统一 `json.Number`（Gate 的字段可能是字符串也可能是数字）。

## 契约

```go
// market/gate_market.go

// GateIntervalDuration 返回保留周期的时长；非保留周期返回错误。
// 保留集合固定为 {10s,1m,5m,15m,30m,1h,4h,8h,1d,7d,1w,30d}（D25），周期顺序由内部常量维护。
func GateIntervalDuration(interval string) (time.Duration, error)

// GetGateKlines 取 Gate USDT 永续合约 K 线，按时间升序返回。
// OpenTime/CloseTime 为毫秒；Volume 是成交额（USDT，来自 sum），不是合约张数（D6）。
func GetGateKlines(symbol, interval string, limit int) ([]Kline, error)

// GateTicker 一行合约行情。
type GateTicker struct {
    Contract         string  // Gate 合约名，如 BTC_USDT
    Symbol           string  // 内部符号（Normalize 后），如 BTCUSDT
    Last             float64 // 最新价
    ChangePercent24h float64 // 24h 涨跌幅（百分比数值，3.2 表示 +3.2%）
    QuoteVolume24h   float64 // 24h 成交额（USDT）
}

func GetGateTicker(symbol string) (GateTicker, error) // 单合约；找不到即错误
func ListGateTickers() ([]GateTicker, error)         // 全部合约（跳过无法解析的条目？不跳过，见校验）
```

## 接口与字段（SDK v7.2.184 模型，文档站不可达时的依据）

| 项目 | 内容 |
|---|---|
| K 线 | `GET /futures/usdt/candlesticks?contract=BTC_USDT&interval=1m&limit=200` |
| 返回 | `[{t, v, c, h, l, o, sum}]`：`t` 秒；`v` 张数；`sum` 成交额（计价币）；OHLC 为字符串 |
| 周期枚举 | 文档列出 `10s 1m 5m 15m 30m 1h 4h 8h 1d 7d`，参数说明另有 `1w`、`30d`；实测均可用（计划 §3.3） |
| 周期对齐（2026-11-25 实测） | `1w` 为自然周（周一起算）；`30d` 是自然月（相邻开盘间距 28–31 天），但**只有 limit 覆盖全部历史时首根才是每月 1 日**（limit=200 实测 85 根均为 1 号；limit=3 为 9/10/11 月 2 日），代码不依赖月初对齐 |
| limit | 单次最多 2000；本子任务要求 1..2000 |
| ticker | `GET /futures/usdt/tickers?contract=BTC_USDT` |
| ticker 字段 | `contract`、`last`、`change_percentage`、`volume_24h_quote`（均为字符串；`change_percentage` 为百分比） |

## 解析与校验

K 线：

1. 周期必须在保留集合内；非保留（`3m`、`2h`、`6h`、`12h`、`3d`、`1w` 之外的别名等）返回错误，
   错误信息列出保留集合。
2. `limit` 必须在 `[1,2000]`。
3. HTTP 必须 200；响应体解析失败、空数组 → 错误。
4. 每根：`t` 必须为正、整数且严格递增（按 Gate 实际开盘校验；`1w` 周一、`30d` 月初，
   不强制能被周期整除）；`o/h/l/c` 必须可解析且为正；`sum` 必须可解析（允许 0，但必须存在）；
   `low ≤ open/close ≤ high`（不满足视为脏数据）。
5. `OpenTime = t × 1000`；`CloseTime` 取下一根的开盘时间（`30d` 自然月间距 28–31 天），
   最后一根用周期时长近似（`30d` 即 30 天，31 天月份会早 1 天；调用方禁止用最后一根的
   CloseTime 做精确收线判断）；`Volume = sum`。
6. 不做回退：Gate 返回错误就是错误，不切换到 CoinAnk/Binance，也不把 `v` 当成交量。

ticker：

1. 单合约：响应必须包含匹配 `contract` 的条目；`last` 必须为正；`change_percentage` 可解析
   （负值允许）；`volume_24h_quote` 缺失或不可解析 → 错误（24h 行情接口需要该字段）。
2. 列表：逐条解析；任一条目的必填字段不可解析即整体失败（fail-closed，不静默丢弃）。
3. `Symbol = Normalize(contract)`，保留 `Contract` 供调用方展示 Gate 原生名。

## 数据流

```
调用方（⑬⑭⑮） ── GetGateKlines(symbol, interval, limit) ──▶ Gate candlesticks
                ── GetGateTicker / ListGateTickers ─────────▶ Gate tickers
   返回 market.Kline（Volume=USDT 成交额） / GateTicker
```

## 权衡

- **为什么 Volume 用 sum**：D6 明确成交量口径用成交额（USDT），指标与提示词按 USDT 解释；
  用张数会让不同 `quanto_multiplier` 的合约不可比。缺失即失败避免量纲漂移。
- **为什么严格校验时间递增与 OHLC 关系**：数据源异常或缓存错位时，引擎指标会被污染；
  在入口失败比在决策层产生错误信号更安全。
- **为什么不做回退**：D1 要求行情全部来自 Gate；回退会掩盖 Gate 故障并让实盘依据他源数据。
- **为什么保留 `Contract` 与 `Symbol` 两个字段**：Gate 原生名用于日志/展示，内部符号用于
  交易路径与提示词。

## 兼容与已知边界

- `market.Kline.Volume` 在旧 CoinAnk 路径是合约张数，新 Gate 路径是 USDT 成交额；两条路径在
  本子任务内并存（⑬ 接入后旧路径删除），调用方必须按来源理解，禁止混用。
- `1w` 为自然周（周一起算）、`7d` 与 Unix 初始时间对齐（计划 §3.3）；`30d` 为自然月，相邻开盘
  28–31 天。`30d` 的时长按 30 天用于最后一根的 CloseTime 近似与调用方的周期换算；小 limit 的
  30d 窗口不保证月初对齐（调用方不得依赖）。
- `10s` 只保留 10s 周期；K 线数据量按 200 根请求即可（⑬ 的指标窗口由调用方决定）。