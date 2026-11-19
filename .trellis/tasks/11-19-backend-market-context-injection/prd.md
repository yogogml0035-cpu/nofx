# C4：策略市场环境上下文注入决策链

## Goal

策略可勾选启用市场环境上下文（市场总览 / 恐惧贪婪 / 新闻情绪），决策时把对应数据以紧凑格式注入模型 prompt，为模型提供大盘视角（现状 prompt 仅有标的 K 线与 AI500/OI 标签，缺市场环境）。

## Requirements

1. **配置结构**：`store.StrategyConfig` 新增 `MarketContext MarketContextConfig json:"market_context,omitempty"`：
   ```go
   type MarketContextConfig struct {
       Overview     bool `json:"overview,omitempty"`      // 市场总览
       FearGreed    bool `json:"fear_greed,omitempty"`    // 恐惧贪婪指数
       NewsSentiment bool `json:"news_sentiment,omitempty"` // 新闻+社交情绪
   }
   ```
   零值 = 全关 = 现状行为（新增字段的默认关闭是产品语义，非历史兼容）。Config JSON 内嵌存储，无表结构变更。
2. **数据获取**：决策循环组装上下文时按勾选项调 `provider/gatemcp` 取数函数（走 C1 TTL 缓存，缓存命中时零网络调用）；未勾选任何项时完全不触达 gatemcp
3. **注入位置**：`kernel/engine_prompt.go` 的 `BuildUserPrompt` 新增「市场环境上下文 Market Environment Context」一节，排在标的市场数据之前；中英文按策略 `language` 输出（对齐现有 writeModeVariant 等的中英模式）
4. **注入格式**：紧凑文本块（非原始 JSON dump）——总览：top 5 板块涨跌 + BTC/ETH 24h 摘要；恐惧贪婪：当前值 + 24h 变化 + 简档（恐惧/贪婪）；新闻情绪：top 10 标题 + 整体情绪分。整节总量软上限约 600 token，超限截断并标注 `[truncated]`
5. **失败语义**：gatemcp 返回错误时该节输出 `[市场环境数据暂不可用]` 一行并继续决策（决策链路不得因环境数据缺失而中断——这是"可选上下文"的语义，不是吞错：事件记入决策日志）
6. **API/工具/UI 同步**：`api/strategy.go` 策略读写 DTO 透传 market_context；agent `manage_strategy` 工具 schema 增加对应 slots 与描述；前端策略编辑器新增「市场环境上下文」勾选组（`components/strategy/MarketContextEditor.tsx`，置于 CoinSourceEditor 附近，三勾选 + 简要说明文案）
7. **token 估算**：`EstimateTokens` 若按 config 计算需把新节计入（实施时核对现有估算逻辑）

## Constraints

- 不引入 per-trader 全局开关（策略级配置，与用户需求一致）
- 不做历史策略数据迁移（默认全关）
- 决策延迟：勾选全开且缓存过期时最多新增一次同步刷新等待（5m/15m TTL 下可忽略）

## Acceptance Criteria

- [ ] `go build ./...`、`go test ./...` 全绿：注入节格式化函数单测（中英、截断、三项勾选组合）；kernel Context 组装单测（勾选→取数调用、全不勾→零调用）
- [ ] web build/test 全绿：勾选组组件渲染与受控更新用例
- [ ] 运行时验证：勾选三项的策略跑一轮决策，决策日志 prompt 含完整市场环境段；未勾选策略的 prompt 不含该段
- [ ] gatemcp 停止（断网点）时决策照常执行，prompt 含 `[市场环境数据暂不可用]`，日志有事件记录
- [ ] agent `manage_strategy` 经对话创建策略可设置 market_context（工具 schema 生效）
