# C4 技术设计：市场环境上下文注入

## 数据流

```
策略编辑器勾选 ──> PATCH 策略 config.ai_config.market_context ──> strategies.Config JSON（经自定义 Marshal/Unmarshal 接线）
决策循环（auto_trader）──> kernel.Context 附带 marketEnv（按勾选拉 gatemcp 缓存）
kernel/engine_prompt.go BuildUserPrompt ──> writeMarketEnv(sb, env, zh)
决策 LLM ──> prompt 含「市场环境上下文」节
```

## 关键点

### 序列化接线（成败关键）

`StrategyConfig` 的 MarshalJSON/UnmarshalJSON 输出/解析固定匿名 struct（store/strategy.go:556-645）——`MarketContext` 必须定义在 `AIStrategyConfig` 且两个 marshal 函数显式接线，否则静默丢失。单测先写序列化往返（set→save→reload→值保持），再做其余。api 层内嵌 `store.StrategyConfig` 透传（api/strategy.go:50/499/548），DTO 无需单独字段，仅核对 create/update 白名单。

### kernel.Context 扩展

`kernel.Context` 新增字段 `MarketEnv *MarketEnvSnapshot`（nil = 未启用）。直接引用 `provider/gatemcp` 视图类型（kernel 已有 import provider 的先例，gatemcp 为纯数据包无循环依赖）：

```go
type MarketEnvSnapshot struct {
    Overview    *gatemcp.MarketOverview
    FearGreed   *gatemcp.FearGreed
    News        []gatemcp.NewsItem
    Sentiment   *gatemcp.Sentiment
    Unavailable bool // 上游失败标记
}
```

### 组装点

`trader/auto_trader_loop.go` buildTradingContext 流程读当前策略 `ai_config.market_context`：任一勾选为真则调 `gatemcp.Shared()` 对应函数填充快照（各函数独立 try，失败置该子项 nil + Unavailable；各源并行刷新不叠加串行）；全不勾则不构造快照。注入节格式（字段集为探针实证，无板块数据）：

```
=== Market Environment Context (市场环境上下文) ===
[Overview] MktCap $1.28T (+1.3% 24h) | Vol $220.7B | BTC $63,749 dom 58.9% | ETH dom 10.2% | Altseason 58
[Fear & Greed] 12 (Extreme Fear), 24h ago 15
[News & Sentiment] BTC sentiment -0.40 negative — top: "ETF inflow hits record"; ...
(或 [市场环境数据暂不可用])
```

`writeMarketEnv(sb, snap, zh)` 纯函数，单测覆盖：三项全开/单开/Unavailable/截断（>600 token 先减新闻条数再减总览字段，口径与 EstimateTokens 一致：中文 len/2、英文 len/4）。锚点：Current Positions 段之后、`## Candidate Coins` 之前。

### store / api / agent / UI

- `store/strategy.go`：MarketContextConfig 定义进 AIStrategyConfig + Marshal/Unmarshal 接线；`EstimateTokens` 计入新节（约 +600 上限，硬阻断门）
- `agent/strategy_field_catalog.go`（三份清单）+ `manage_strategy` schema + `skill_execution_handlers.go` flat-key switch + `central_brain.go` 值展示：新增 market_context 三键（overview/fear_greed/news_sentiment）
- 前端 `MarketContextEditor.tsx`：三 Checkbox + 说明，风格对齐 CoinSourceEditor；`types/strategy.ts` 的 AIStrategyConfig 补类型；i18n 走 strategy-translations {zh,en,es}

## 测试

- kernel：writeMarketEnv 中英/组合/截断/Unavailable
- trader：上下文组装（stub gatemcp 取数函数——通过函数注入或包级 var 替换点，只替换上游、不 mock 业务）；全不勾零调用
- store：MarketContext 序列化往返（Marshal/Unmarshal 接线回归）
- agent：工具 schema 序列化含新 slots；flat-key 链路
- web：勾选组件受控 + config 序列化

## 风险

- 决策日志体积：`trader/auto_trader_loop.go` 现状即整段记录 UserPrompt 入库、无截断机制——市场环境节（≤600 token）随之入库，不为此新增截断（属独立优化）
- gatemcp 快照类型若含 time.Time 等非序列化稳定字段，注入前统一转字符串
