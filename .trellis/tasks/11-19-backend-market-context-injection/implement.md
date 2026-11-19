# C4 执行计划

前置：C1（gatemcp 取数函数）与 C2（发布语义已清）合入父分支。

## 步骤

1. [ ] `store/strategy.go`：MarketContextConfig + StrategyConfig.MarketContext；核对 EstimateTokens 是否需计入
2. [ ] kernel：`MarketEnvSnapshot` 类型 + Context 字段 + `engine_prompt.go` 的 writeMarketEnv（中英/截断/Unavailable）+ 单测
3. [ ] 决策循环组装：按勾选拉 gatemcp（独立 try/失败标记）；全不勾零调用 + 单测
4. [ ] api/agent：DTO 透传核对、`strategy_field_catalog.go`/`manage_strategy` schema 增 slots
5. [ ] `go build ./... && go test ./...` 全绿 → commit
6. [ ] 前端：`types/strategy.ts` + `MarketContextEditor.tsx` + StrategyStudioPage 挂载 + i18n 键
7. [ ] `npm --prefix web run build && npm --prefix web run test` 全绿 → commit
8. [ ] 运行时验证：勾选策略跑决策，日志 prompt 含注入段；未勾选不含；断网点验 `[市场环境数据暂不可用]`；截图/日志摘录留档 `probe/`

## 验证命令

```bash
~/sdk/go1.26.2/bin/go build ./... && ~/sdk/go1.26.2/bin/go test ./...
npm --prefix web run build && npm --prefix web run test
```

## 回滚点

后端/前端各一 commit 边界；探针不过不合父。
