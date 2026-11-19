# C4：策略市场环境上下文注入决策链

## Goal

策略可勾选启用市场环境上下文（市场总览 / 恐惧贪婪 / 新闻情绪），决策时把对应数据以紧凑格式注入模型 prompt，为模型提供大盘视角。

## Requirements

1. **配置结构（关键：走 AIStrategyConfig，不走 StrategyConfig 顶层）**：`StrategyConfig` 的 JSON 是自定义 MarshalJSON/UnmarshalJSON（store/strategy.go 输出/解析固定匿名 struct）——**在顶层 struct 加 json tag 会被静默丢弃**。`MarketContext` 定义进 `AIStrategyConfig`（存储键 `ai_config.market_context`），并在 Marshal 输出 struct 与 Unmarshal raw struct 中显式接线：
   ```go
   type MarketContextConfig struct {
       Overview      bool `json:"overview,omitempty"`       // 市场总览
       FearGreed     bool `json:"fear_greed,omitempty"`     // 恐惧贪婪指数
       NewsSentiment bool `json:"news_sentiment,omitempty"` // 新闻+社交情绪
   }
   ```
   零值 = 全关 = 现状行为（新增字段的默认关闭是产品语义，非历史兼容）。仅 ai_trading 策略有效（grid 不暴露）。前端 `types/strategy.ts` 的 AIStrategyConfig 同步。
2. **数据获取**：决策循环组装上下文时按勾选项调 `provider/gatemcp` 取数函数（走 C1 TTL 缓存，缓存命中时零网络调用）；未勾选任何项时完全不触达 gatemcp
3. **注入位置**：`kernel/engine_prompt.go` 的 `BuildUserPrompt` 新增「市场环境上下文 Market Environment Context」一节，语义锚点固定为 **Current Positions 段之后、`## Candidate Coins` 标题之前**（不用行号——C2b 删段后行号会漂移）；中英文按策略 `language`（`GetLanguage()==LangChinese` + `label(en, zh)` 机制）输出
4. **注入格式**：紧凑文本块（非原始 JSON dump），字段集以探针实证为准（无板块数据）——总览：总市值 ±24h 变化、总成交、BTC 价格与 dominance、ETH dominance、山寨季指数；恐惧贪婪：当前值 + 标签 + 24h 派生变化（不足 24h 省略变化）；新闻情绪：BTC 新闻 top 标题 + 整体情绪分（标注 BTC）。整节总量软上限约 600 token，估算口径与 `EstimateTokens` 一致（中文 len/2、英文 len/4 字符换算），超限先减新闻条目再减总览字段，行尾标注 `[truncated]`
5. **失败语义**：gatemcp 返回错误时该节输出 `[市场环境数据暂不可用]` 一行并继续决策（决策链路不得因环境数据缺失而中断——这是"可选上下文"的语义，不是吞错：事件记入决策日志。与 C3 看板的区块级 fast-fail 是有意的两层语义：UI 面向人展示失败，决策链面向交易不阻断）
6. **API/工具/UI 同步**：`api/strategy.go` 策略读写透传（Config 内嵌自动携带，核对 create/update 白名单）；agent `manage_strategy` 工具 schema 增加对应 slots 与描述，**flat-key 链路同步**（strategy_field_catalog.go 三份清单、skill_execution_handlers.go 的 label/apply/keywords switch、central_brain.go 值展示——否则 skill 会话路径设置不到 market_context）；前端策略编辑器新增「市场环境上下文」勾选组（`components/strategy/MarketContextEditor.tsx`，置于 CoinSourceEditor 附近，三勾选 + 简要说明文案；i18n 走 strategy-translations 的 {zh,en,es} 结构）
7. **token 估算**：`EstimateTokens`（store/strategy.go）按 config 估算且是超限硬阻断门（kernel/engine_analysis.go）——**必须把新节计入（约 +600 token 上限）**，否则阻断判断失真

## Constraints

- 不引入 per-trader 全局开关（策略级配置，与用户需求一致）
- 不做历史策略数据迁移（默认全关）
- 决策延迟：勾选全开且多路缓存同时过期时，各源并行刷新、等待不叠加串行（overview 5m / news·sentiment 15m TTL）

## Acceptance Criteria

- [ ] `go build ./...`、`go test ./...` 全绿（Docker golang:1.25-alpine + GOPROXY）：**MarketContext 序列化往返单测（设置→保存→重读→值保持，覆盖 Marshal/Unmarshal 接线）**；注入节格式化函数单测（中英、截断、三项勾选组合、Unavailable）；决策上下文组装单测（勾选→取数调用、全不勾→零调用）
- [ ] web build/test 全绿：勾选组组件渲染与受控更新用例
- [ ] 运行时验证：勾选三项的策略跑一轮决策，决策日志 prompt 含完整市场环境段；未勾选策略的 prompt 不含该段
- [ ] gatemcp 断链（`GATEMCP_ENDPOINT=http://127.0.0.1:9`）时决策照常执行，prompt 含 `[市场环境数据暂不可用]`，日志有事件记录
- [ ] agent `manage_strategy` 经对话创建策略可设置 market_context（工具 schema 生效）；skill flat-key 路径同步
