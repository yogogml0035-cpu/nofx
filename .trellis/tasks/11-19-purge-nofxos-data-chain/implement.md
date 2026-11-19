# C2b 执行计划

纯删除+收敛任务。顺序：store schema → kernel → trader/manager/api/agent → 前端 → 文档痕迹复核。

## 步骤

1. [ ] `store/strategy.go`：CoinSourceConfig 收敛（删 SourceType/UseAI500/UseOITop/UseOILow/AI500Limit 与 normalize 分支）；`IndicatorConfig` 删整个 NofxOS 段（13 字段）；`store/trader.go` 删 UseAI500/UseOITop；strategy_source_type_test 等同步
2. [ ] `kernel/`：engine.go 的 Ranking 字段/Fetch 链/Sources 标签/QuantData 链（:101-105/:147/:394/:544-668）、engine_prompt.go Ranking 段（:581-593、:859-902）、engine_analysis.go、formatter.go 清理；相关测试同步
3. [ ] `trader/`：auto_trader* 的 ai500/oi 币源分支与 EnableQuantData 消费；`manager/trader_manager.go` URL 构建；`api/handler_trader.go`/`handler_order.go` DTO
4. [ ] `agent/`：ai500.go、get_ai500_list 工具、strategy_field_catalog、skill_execution_handlers、llm_flow_extractor、skill_domain_context
5. [ ] `api/`：handler_ai500.go + 路由 + server.go 帮助文案改写 + strategy.go NofxOS key 校验；`provider/nofxos/` 整包删除
6. [ ] Docker 门禁：build + test 全绿 → commit
7. [ ] 前端：AI500Panel、AgentChatPage tab、data.ts AI500 client、CoinSourceEditor 收敛、IndicatorEditor、TraderConfigModal、types、i18n、WelcomeScreen、AgentTerminal「Netflow」标签改词
8. [ ] web build/test 全绿 → commit
9. [ ] 文档面（PRD 第 21 条清单）；全量 grep（PRD 词表）归零复核；浏览器探针（策略编辑器/Agent 页/排行榜回归/klines）截图留档 `probe/`

## 验证命令

```bash
docker run --rm -v "$PWD":/w -w /w -e GOPROXY=https://goproxy.cn,direct golang:1.25-alpine go build ./...
docker run --rm -v "$PWD":/w -w /w -e GOPROXY=https://goproxy.cn,direct golang:1.25-alpine go test ./...
npm --prefix web run build && npm --prefix web run test
```

## 回滚点

store/kernel、trader/agent/api、前端三个 commit 边界；探针不过不合父。
