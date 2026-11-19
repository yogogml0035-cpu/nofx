# C4 技术设计：市场环境上下文注入

## 数据流

```
策略编辑器勾选 ──> PATCH 策略 config.market_context ──> strategies.Config JSON
决策循环（auto_trader）──> kernel.Context 附带 marketEnv（按勾选拉 gatemcp 缓存）
kernel/engine_prompt.go BuildUserPrompt ──> writeMarketEnv(sb, env, zh)
决策 LLM ──> prompt 含「市场环境上下文」节
```

## 关键点

### kernel.Context 扩展

`kernel.Context` 新增字段 `MarketEnv *MarketEnvSnapshot`（nil = 未启用）。`MarketEnvSnapshot` 定义在 kernel 包（避免 kernel 依赖 provider——快照结构独立于获取方式）：

```go
type MarketEnvSnapshot struct {
    Overview   *gatemcp.MarketOverview // 或复制为 kernel 本地类型，二选一：实施时若 gatemcp 类型够纯（无连接语义）直接引用，否则复制字段
    FearGreed  *gatemcp.FearGreed
    News       []gatemcp.NewsItem
    Sentiment  *gatemcp.Sentiment
    Unavailable bool // 上游失败标记
}
```

依赖方向：`provider/gatemcp` 是纯数据包（无循环依赖风险）；kernel 引用其视图类型可接受，与 kernel 引用 store 类型的现有模式一致。

### 组装点

决策循环在进入 kernel 前，读当前策略 config.MarketContext：任一勾选为真则调 `gatemcp.Shared()` 对应函数填充快照（各函数独立 try，失败置该子项 nil + Unavailable）；全不勾则不构造快照。注入节格式：

```
=== Market Environment Context (市场环境上下文) ===
[Overview] Sectors 24h: AI +4.2%, DeFi +1.1%, ... | BTC +2.1% ($67,234) | ETH +1.8%
[Fear & Greed] 72 (Greed), yesterday 68
[News & Sentiment] score 0.62 bullish — top: "ETF inflow hits record"; ...
(或 [市场环境数据暂不可用])
```

`writeMarketEnv(sb, snap, zh)` 纯函数，单测覆盖：三项全开/单开/Unavailable/截断（>600 token 时砍新闻条数再砍板块行）。

### store / api / agent / UI

- `store/strategy.go`：MarketContextConfig 定义 + StrategyConfig 字段；`strategy_schema_test.go` 无需列断言（JSON 内嵌）
- `api/strategy.go`：DTO 已是 Config 透传则自动携带；仅当显式白名单字段时补透传（实施时核对 create/update handler）
- `agent/strategy_field_catalog.go` 与 `manage_strategy` schema：新增 market_context 说明（overview/fear_greed/news_sentiment 三个 bool slots）
- 前端 `MarketContextEditor.tsx`：三 Checkbox + 说明，风格对齐 CoinSourceEditor；`types/strategy.ts` 补类型

## 测试

- kernel：writeMarketEnv 中英/组合/截断/Unavailable；Context 组装（stub gatemcp 取数函数——通过函数注入或包级 var 替换点，只替换上游、不 mock 业务）
- agent：工具 schema 序列化含新 slots
- web：勾选组件受控 + config 序列化

## 风险

- 决策日志体积：注入节 ≤600 token，日志已有截断机制则对齐
- gatemcp 快照类型若含 time.Time 等非序列化稳定字段，注入前统一转字符串
