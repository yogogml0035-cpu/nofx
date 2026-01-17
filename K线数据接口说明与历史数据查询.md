# K线数据接口说明与历史数据查询

## 当前使用的API接口

系统支持多个K线数据源，可以通过环境变量切换：

### 1. CoinAnk API（默认）

**文件位置**: `market/data.go` → `getKlinesFromCoinAnk()`

**API端点**: `https://api.coinank.com/api/kline/list/open`

**调用方式**:
```go
func Kline(
    ctx context.Context, 
    symbol string,                    // 交易对，如 "BTCUSDT"
    exchange coinank_enum.Exchange,   // 交易所，如 coinank_enum.Binance
    ts int64,                         // 时间戳（毫秒）
    side coinank_enum.Side,           // 方向：To（向前查询）或 From（向后查询）
    size int,                         // 数据条数
    interval coinank_enum.Interval    // K线周期
) ([]coinank.KlineResult, error)
```

**当前使用方式**:
```go
// 从当前时间向前查询最新的 limit 条K线
ts := time.Now().UnixMilli()
coinankKlines, err := coinank_api.Kline(
    ctx, 
    symbol,                  // "BTCUSDT"
    coinank_enum.Binance,    // 交易所
    ts,                      // 当前时间戳
    coinank_enum.To,         // 向前查询（获取历史数据）
    limit,                   // 200
    coinankInterval          // 如 coinank_enum.Minute30
)
```

**支持的时间周期**:
- 1m, 3m, 5m, 15m, 30m
- 1h, 2h, 4h, 6h, 8h, 12h
- 1d, 3d, 1w

**时间参数说明**:
- `ts`: 时间戳（毫秒），作为查询的起点或终点
- `side`: 
  - `To`: 从 ts 向前查询（获取 ts 之前的数据）
  - `From`: 从 ts 向后查询（获取 ts 之后的数据）
- `size`: 返回的K线数量

**是否支持历史数据**: ✅ **支持**
- 可以通过修改 `ts` 参数查询任意时间段的数据
- 例如查询去年的数据：`ts = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()`

---

### 2. OKX API（可选）

**文件位置**: `provider/okx/okx_market.go` → `GetKlines()`

**API端点**: 
- 最新数据: `https://www.okx.com/api/v5/market/candles`
- 历史数据: `https://www.okx.com/api/v5/market/history-candles`

**调用方式**:
```go
func (c *OKXMarketClient) GetKlines(
    ctx context.Context, 
    symbol string,      // 交易对，如 "BTCUSDT"
    interval string,    // K线周期，如 "30m", "4H"
    limit int           // 数据条数
) ([]KlineData, error)
```

**当前使用方式**:
```go
// 获取最新的 limit 条K线
okxKlines, err := client.GetKlines(ctx, symbol, interval, limit)
```

**支持的时间周期**:
- 1m, 3m, 5m, 15m, 30m
- 1H, 2H, 4H, 6H, 8H, 12H
- 1D, 1W

**分批获取机制**:
- OKX API 单次最多返回 100 条数据
- 系统会自动分批获取：
  - 第一批：使用 `/candles` 获取最新 100 条
  - 后续批次：使用 `/history-candles` + `before` 参数获取更早的数据
  - 自动合并所有批次，返回完整数据

**是否支持历史数据**: ✅ **支持**
- 通过 `before` 参数可以查询历史数据
- 但当前实现只获取最新数据，需要修改代码支持指定时间范围

---

### 3. Hyperliquid API（用于 xyz dex 资产）

**文件位置**: `market/data.go` → `getKlinesFromHyperliquid()`

**用途**: 专门用于获取 xyz dex 资产（如 `xyz:SILVER`, `xyz:GOLD`）的K线数据

**是否支持历史数据**: ✅ **支持**（取决于 Hyperliquid API）

---

## 历史数据查询功能

### 已实现的历史数据查询函数

**文件位置**: `market/historical.go` → `GetKlinesRange()`

```go
// GetKlinesRange 获取指定时间范围内的K线数据
func GetKlinesRange(
    symbol string,      // 交易对，如 "BTCUSDT"
    timeframe string,   // K线周期，如 "30m", "4h"
    start time.Time,    // 开始时间
    end time.Time       // 结束时间
) ([]Kline, error)
```

**实现方式**:
1. 使用 OKX API 的 `/api/v5/market/history-candles` 端点
2. 计算时间范围需要的K线数量
3. 自动分批获取数据
4. 返回按时间升序排列的K线数据

**使用示例**:
```go
// 查询去年1月的数据
start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
end := time.Date(2025, 1, 31, 23, 59, 59, 0, time.UTC)
klines, err := market.GetKlinesRange("BTCUSDT", "30m", start, end)
```

**限制**:
- 单次最多获取 2000 条K线（OKX API 限制）
- 如果时间范围过大，需要多次调用

---

## 如何修改代码支持历史数据查询

### 方案 1: 使用现有的 GetKlinesRange 函数

**适用场景**: 回测、历史数据分析

**修改位置**: `kernel/engine.go` → `fetchMarketDataWithStrategy()`

**修改方案**:
```go
// 在 Context 中添加时间范围参数
type Context struct {
    // ... 现有字段
    StartTime *time.Time `json:"start_time,omitempty"` // 可选：查询开始时间
    EndTime   *time.Time `json:"end_time,omitempty"`   // 可选：查询结束时间
}

// 修改 fetchMarketDataWithStrategy 函数
func fetchMarketDataWithStrategy(ctx *Context, engine *StrategyEngine) error {
    // ... 现有代码
    
    for _, coin := range ctx.CandidateCoins {
        var data *market.Data
        var err error
        
        // 如果指定了时间范围，使用历史数据查询
        if ctx.StartTime != nil && ctx.EndTime != nil {
            data, err = market.GetWithTimeframesRange(
                coin.Symbol, 
                timeframes, 
                primaryTimeframe, 
                klineCount,
                *ctx.StartTime,
                *ctx.EndTime,
            )
        } else {
            // 否则使用默认的最新数据查询
            data, err = market.GetWithTimeframes(
                coin.Symbol, 
                timeframes, 
                primaryTimeframe, 
                klineCount,
            )
        }
        
        if err != nil {
            logger.Infof("⚠️  Failed to fetch market data for %s: %v", coin.Symbol, err)
            continue
        }
        
        ctx.MarketDataMap[coin.Symbol] = data
    }
    
    return nil
}
```

---

### 方案 2: 修改 CoinAnk API 调用支持历史时间

**适用场景**: 使用 CoinAnk API 查询历史数据

**修改位置**: `market/data.go` → `getKlinesFromCoinAnk()`

**修改方案**:
```go
// 添加可选的时间参数
func getKlinesFromCoinAnk(symbol, interval string, limit int) ([]Kline, error) {
    return getKlinesFromCoinAnkWithTime(symbol, interval, limit, time.Now())
}

// 新增支持时间参数的函数
func getKlinesFromCoinAnkWithTime(symbol, interval string, limit int, endTime time.Time) ([]Kline, error) {
    // ... 现有的 interval 映射代码
    
    ctx := context.Background()
    ts := endTime.UnixMilli()  // 使用指定的结束时间
    
    // 使用 To 方向从指定时间向前查询
    coinankKlines, err := coinank_api.Kline(
        ctx, 
        symbol, 
        coinank_enum.Binance, 
        ts,                    // 指定的时间戳
        coinank_enum.To,       // 向前查询
        limit, 
        coinankInterval,
    )
    
    // ... 现有的转换代码
}
```

**使用示例**:
```go
// 查询去年1月1日之前的100根30分钟K线
endTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
klines, err := getKlinesFromCoinAnkWithTime("BTCUSDT", "30m", 100, endTime)
```

---

### 方案 3: 修改 OKX API 调用支持历史时间

**适用场景**: 使用 OKX API 查询历史数据

**修改位置**: `provider/okx/okx_market.go` → `GetKlines()`

**修改方案**:
```go
// 添加支持时间范围的新方法
func (c *OKXMarketClient) GetKlinesWithTime(
    ctx context.Context, 
    symbol string, 
    interval string, 
    limit int,
    endTime *time.Time,  // 可选：结束时间
) ([]KlineData, error) {
    instId := convertSymbolToOKX(symbol)
    bar := convertIntervalToOKX(interval)
    
    const maxBatchSize = 100
    
    // 如果指定了结束时间，使用 history-candles 端点
    if endTime != nil {
        endpoint := "/api/v5/market/history-candles"
        params := map[string]string{
            "instId": instId,
            "bar":    bar,
            "limit":  strconv.Itoa(limit),
            "after":  strconv.FormatInt(endTime.UnixMilli(), 10), // 获取此时间之前的数据
        }
        
        needAuth := c.apiKey != ""
        data, err := c.doRequest(ctx, "GET", endpoint, params, needAuth)
        if err != nil {
            return nil, fmt.Errorf("failed to fetch klines: %w", err)
        }
        
        return c.parseKlineResponse(data)
    }
    
    // 否则使用默认的最新数据查询
    return c.GetKlines(ctx, symbol, interval, limit)
}
```

**使用示例**:
```go
// 查询去年1月1日之前的100根K线
endTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
klines, err := client.GetKlinesWithTime(ctx, "BTCUSDT", "30m", 100, &endTime)
```

---

## 实现完整的历史数据查询功能

### 步骤 1: 创建新的市场数据获取函数

**文件**: `market/data.go`

```go
// GetWithTimeframesRange 获取指定时间范围的市场数据（支持多时间周期）
func GetWithTimeframesRange(
    symbol string, 
    timeframes []string, 
    primaryTimeframe string, 
    count int,
    startTime time.Time,
    endTime time.Time,
) (*Data, error) {
    symbol = Normalize(symbol)
    
    if len(timeframes) == 0 {
        return nil, fmt.Errorf("at least one timeframe is required")
    }
    
    if primaryTimeframe == "" {
        primaryTimeframe = timeframes[0]
    }
    
    // 存储所有时间周期的数据
    timeframeData := make(map[string]*TimeframeSeriesData)
    var primaryKlines []Kline
    
    // 为每个时间周期获取历史K线数据
    for _, tf := range timeframes {
        // 使用 GetKlinesRange 获取指定时间范围的数据
        klines, err := GetKlinesRange(symbol, tf, startTime, endTime)
        if err != nil {
            logger.Infof("⚠️ Failed to get %s %s historical K-line: %v", symbol, tf, err)
            continue
        }
        
        if len(klines) == 0 {
            logger.Infof("⚠️ %s %s historical K-line data is empty", symbol, tf)
            continue
        }
        
        // 保存主时间周期的K线数据
        if tf == primaryTimeframe {
            primaryKlines = klines
        }
        
        // 计算该时间周期的序列数据（取最新 count 条）
        seriesData := calculateTimeframeSeries(klines, tf, count)
        timeframeData[tf] = seriesData
    }
    
    // 如果主时间周期数据为空，返回错误
    if len(primaryKlines) == 0 {
        return nil, fmt.Errorf("Primary timeframe %s historical K-line data is empty", primaryTimeframe)
    }
    
    // 计算当前指标（基于主时间周期的最新数据）
    currentPrice := primaryKlines[len(primaryKlines)-1].Close
    currentEMA20 := calculateEMA(primaryKlines, 20)
    currentMACD := calculateMACD(primaryKlines)
    currentRSI7 := calculateRSI(primaryKlines, 7)
    
    // 计算价格变化
    priceChange1h := calculatePriceChangeByBars(primaryKlines, primaryTimeframe, 60)
    priceChange4h := calculatePriceChangeByBars(primaryKlines, primaryTimeframe, 240)
    
    return &Data{
        Symbol:        symbol,
        CurrentPrice:  currentPrice,
        PriceChange1h: priceChange1h,
        PriceChange4h: priceChange4h,
        CurrentEMA20:  currentEMA20,
        CurrentMACD:   currentMACD,
        CurrentRSI7:   currentRSI7,
        TimeframeData: timeframeData,
    }, nil
}
```

---

### 步骤 2: 修改策略引擎支持历史数据

**文件**: `kernel/engine.go`

```go
// Context 添加时间范围参数
type Context struct {
    // ... 现有字段
    
    // 历史数据查询参数（可选）
    HistoricalMode bool       `json:"historical_mode,omitempty"` // 是否使用历史数据模式
    StartTime      *time.Time `json:"start_time,omitempty"`      // 查询开始时间
    EndTime        *time.Time `json:"end_time,omitempty"`        // 查询结束时间
}

// fetchMarketDataWithStrategy 修改支持历史数据
func fetchMarketDataWithStrategy(ctx *Context, engine *StrategyEngine) error {
    config := engine.GetConfig()
    ctx.MarketDataMap = make(map[string]*market.Data)
    
    timeframes := config.Indicators.Klines.SelectedTimeframes
    primaryTimeframe := config.Indicators.Klines.PrimaryTimeframe
    klineCount := config.Indicators.Klines.PrimaryCount
    
    // ... 兼容性代码
    
    logger.Infof("📊 Strategy timeframes: %v, Primary: %s, Kline count: %d", timeframes, primaryTimeframe, klineCount)
    
    // 如果启用历史数据模式
    if ctx.HistoricalMode && ctx.StartTime != nil && ctx.EndTime != nil {
        logger.Infof("📊 Historical mode enabled: %s to %s", 
            ctx.StartTime.Format("2006-01-02 15:04"), 
            ctx.EndTime.Format("2006-01-02 15:04"))
    }
    
    // 1. 获取持仓币种数据
    for _, pos := range ctx.Positions {
        var data *market.Data
        var err error
        
        if ctx.HistoricalMode && ctx.StartTime != nil && ctx.EndTime != nil {
            // 历史数据模式
            data, err = market.GetWithTimeframesRange(
                pos.Symbol, timeframes, primaryTimeframe, klineCount,
                *ctx.StartTime, *ctx.EndTime,
            )
        } else {
            // 实时数据模式
            data, err = market.GetWithTimeframes(
                pos.Symbol, timeframes, primaryTimeframe, klineCount,
            )
        }
        
        if err != nil {
            logger.Infof("⚠️  Failed to fetch market data for position %s: %v", pos.Symbol, err)
            continue
        }
        ctx.MarketDataMap[pos.Symbol] = data
    }
    
    // 2. 获取候选币种数据
    for _, coin := range ctx.CandidateCoins {
        if _, exists := ctx.MarketDataMap[coin.Symbol]; exists {
            continue
        }
        
        var data *market.Data
        var err error
        
        if ctx.HistoricalMode && ctx.StartTime != nil && ctx.EndTime != nil {
            // 历史数据模式
            data, err = market.GetWithTimeframesRange(
                coin.Symbol, timeframes, primaryTimeframe, klineCount,
                *ctx.StartTime, *ctx.EndTime,
            )
        } else {
            // 实时数据模式
            data, err = market.GetWithTimeframes(
                coin.Symbol, timeframes, primaryTimeframe, klineCount,
            )
        }
        
        if err != nil {
            logger.Infof("⚠️  Failed to fetch market data for %s: %v", coin.Symbol, err)
            continue
        }
        
        ctx.MarketDataMap[coin.Symbol] = data
    }
    
    logger.Infof("📊 Successfully fetched market data for %d coins", len(ctx.MarketDataMap))
    return nil
}
```

---

### 步骤 3: 使用示例

```go
// 实时数据查询（当前行为）
ctx := &kernel.Context{
    CurrentTime:    time.Now().Format("2006-01-02 15:04:05"),
    CallCount:      1,
    RuntimeMinutes: 0,
    Account:        accountInfo,
    Positions:      positions,
    CandidateCoins: candidateCoins,
}

// 历史数据查询（新功能）
startTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
endTime := time.Date(2025, 1, 31, 23, 59, 59, 0, time.UTC)

ctx := &kernel.Context{
    CurrentTime:    endTime.Format("2006-01-02 15:04:05"),
    CallCount:      1,
    RuntimeMinutes: 0,
    Account:        accountInfo,
    Positions:      positions,
    CandidateCoins: candidateCoins,
    HistoricalMode: true,      // 启用历史数据模式
    StartTime:      &startTime, // 查询开始时间
    EndTime:        &endTime,   // 查询结束时间
}

// 获取市场数据（会自动使用历史数据）
err := fetchMarketDataWithStrategy(ctx, engine)
```

---

## 总结

### 当前状态

1. **CoinAnk API**: ✅ 支持通过 `ts` 参数查询历史数据
2. **OKX API**: ✅ 支持通过 `before`/`after` 参数查询历史数据
3. **历史数据函数**: ✅ 已实现 `GetKlinesRange()` 函数

### 需要的修改

1. **添加时间范围参数**: 在 `Context` 中添加 `StartTime` 和 `EndTime` 字段
2. **创建历史数据获取函数**: 实现 `GetWithTimeframesRange()` 函数
3. **修改策略引擎**: 在 `fetchMarketDataWithStrategy()` 中支持历史数据模式
4. **前端支持**: 添加时间范围选择器（如果需要从前端触发）

### 优势

- **灵活性**: 可以查询任意时间段的数据
- **回测支持**: 可以用于策略回测
- **向后兼容**: 不影响现有的实时数据查询功能
- **多数据源**: 支持 CoinAnk、OKX、Hyperliquid 多个数据源

### 使用场景

1. **策略回测**: 使用历史数据测试策略效果
2. **历史分析**: 分析过去某个时间段的市场行为
3. **数据研究**: 获取大量历史数据进行研究
4. **模拟交易**: 在历史数据上模拟交易决策
