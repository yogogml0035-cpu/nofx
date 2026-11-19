# C4 执行计划

前置：C1（gatemcp 取数函数）、C2（发布语义已清）、**C2b（engine_prompt.go 先删后加，避免段落/行号冲突）**合入父分支。

## 步骤

1. [ ] `store/strategy.go`：MarketContextConfig 定义进 AIStrategyConfig + Marshal/Unmarshal 显式接线 + 序列化往返单测；EstimateTokens 计入新节
2. [ ] kernel：`MarketEnvSnapshot` 类型 + Context 字段 + `engine_prompt.go` 的 writeMarketEnv（中英/截断/Unavailable，锚点 Current Positions 后、Candidate Coins 前）+ 单测
3. [ ] 决策循环组装（trader/auto_trader_loop.go buildTradingContext）：按勾选拉 gatemcp（独立 try/失败标记，各源并行）；全不勾零调用 + 单测
4. [ ] api/agent：DTO 透传核对、`strategy_field_catalog.go`/`manage_strategy` schema/flat-key 链路增 slots
5. [ ] Docker 门禁 `go build ./... && go test ./...` 全绿 → commit
6. [ ] 前端：`types/strategy.ts` AIStrategyConfig + `MarketContextEditor.tsx` + StrategyStudioPage 挂载 + i18n 键（{zh,en,es}）
7. [ ] `npm --prefix web run build && npm --prefix web run test` 全绿 → commit
8. [ ] 运行时验证：勾选策略跑决策，日志 prompt 含注入段；未勾选不含；断链（`GATEMCP_ENDPOINT=http://127.0.0.1:9`）验 `[市场环境数据暂不可用]`；截图/日志摘录留档 `probe/`

## 验证命令

```bash
docker run --rm -v "$PWD":/w -w /w -e GOPROXY=https://goproxy.cn,direct golang:1.25-alpine go build ./...
docker run --rm -v "$PWD":/w -w /w -e GOPROXY=https://goproxy.cn,direct golang:1.25-alpine go test ./...
npm --prefix web run build && npm --prefix web run test
```

## 回滚点

后端/前端各一 commit 边界；探针不过不合父。
