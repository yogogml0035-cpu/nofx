# 技术设计：⑭ chart-klines-gate

## 边界与职责

- `api/handler_klines.go`：加密货币分支切 Gate；删除 CoinAnk 映射与回退；保留 Alpaca/TwelveData。
- `provider/coinank`：删除 K 线能力（`coinank_api/` 与顶层 `kline.go`）；保留其他数据能力与 enum。
- `web`：图表组件去掉 `exchange` 与旧成交量单位；删除无调用方的 `TradingViewChart`。

## 后端

```go
// 测试可替换的取数入口（默认 Gate 直连）
var cryptoKlines = market.GetGateKlines

func (s *Server) handleKlines(c *gin.Context) {
    symbol := c.Query("symbol"); interval := c.DefaultQuery("interval", "5m")
    limit := clamp(c.DefaultQuery("limit","1000"), 1, 2000)
    switch strings.ToLower(c.DefaultQuery("exchange", defaultExchange)) {
    case "alpaca": ...            // 股票（保留）
    case "forex", "metals": ...   // 外汇/金属（保留）
    default:                      // 加密货币：只走 Gate（D1）
        klines, err = cryptoKlines(symbol, interval, limit)
        for i := range klines { klines[i].QuoteVolume = klines[i].Volume } // 两者都是 USDT 成交额
    }
    c.JSON(200, klines)
}
```

- 删除 `getKlinesFromCoinank` 与 `coinank_api`/`coinank_enum` import。
- `exchange` 对加密货币不再参与路由（旧映射全删）；`defaultExchange` 保留给 `/api/symbols` 与默认值。
- `limit`：<=0 → 1000；>2000 → 2000（Gate 单次上限）。
- 周期交给 `GetGateKlines` 校验（保留集合）；错误经 `SafeInternalError` → 500。

## CoinAnk 清理

| 位置 | 处理 |
|---|---|
| `provider/coinank/coinank_api/kline.go` + `kline_test.go` | 删除 |
| `provider/coinank/coinank_api/base_coin.go` + `symbol.go`（+ tests） | 仅被 K 线用于符号归一，随 K 线删除 |
| `provider/coinank/kline.go`（`CoinankClient.Kline` + `KlineResult`） | 无调用方，删除 |
| 其余 `provider/coinank/*.go`、`coinank_enum/` | 保留（D10：CoinAnk 其他数据能力） |

## 前端

- `AdvancedChart`：删 `exchange` prop/默认 `binance`/URL 参数/交易所徽标/依赖项；
  `getQuoteUnit` 恒为 `USDT`，删 `getBaseUnit`；tooltip 只显示 `V(USDT)`（用 `quoteVolume`）；
  柱状图数据用 `quoteVolume`（USDT）。
- `ChartWithOrders`：删 `exchange` prop/URL 参数；成交量按 `volume`（Gate 已是 USDT）展示，
  注释改 USDT。
- `ChartTabs`：删 `exchangeId` prop 与 `currentExchange`/key 段/`<AdvancedChart exchange>`。
- `TraderDashboardPage`：不再传 `exchangeId`。
- 删除无调用方的旧图表组件：`TradingViewChart.tsx`（旧交易所列表/外部 widget）、
  `ChartWithOrders.tsx`、`ChartWithOrdersSimple.tsx`（内置 `exchange` 默认 binance、无任何引用）。
- 周期选项保持 1m/5m/15m/30m/1h/4h/1d（均在保留集合）。

## 验证

- 后端：`api/handler_klines_test.go` 用注入的 `cryptoKlines` 断言 symbol/interval/limit、无参与
  `exchange=binance`/`gate` 同一路径、limit 边界、错误 500、非保留周期（注入真实
  `market.GetGateKlines` + 无效 interval 直接报错）。
- 前端：`npm test`、`tsc --noEmit`、`vite build`；URL 与徽标检查。
- 真实链路：父任务 dev-offline E2E 用 `/api/klines` 对 Gate 取数（本子任务不单独起栈）。

## 权衡与边界

- **为什么用包级函数变量**：`market` 的 Gate baseURL 不导出；handler 测试注入取数函数是最小
  测试缝，不改变生产路径。
- **Volume/QuoteVolume 双写**：图表同时使用两个字段（柱状图/ tooltip），Gate 只有 `sum`，
  双写避免出现 0 或口径混用。
- **TradingViewChart 删除**：无任何调用方且内置 Binance/Bybit/OKX/MEXC 列表，属旧语义。