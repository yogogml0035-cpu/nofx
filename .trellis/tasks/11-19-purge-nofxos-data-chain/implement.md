# C2b 执行计划

纯删除+收敛任务。顺序：store schema → kernel/market/trader → agent/api → 前端 → 文档痕迹复核。

## 步骤

1. [ ] `store/strategy.go`：CoinSourceConfig 收敛（删 SourceType/UseAI500/UseOITop/UseOILow/AI500Limit 与 normalize 分支）；`IndicatorConfig` 删 EnableQuantOI/EnableQuantNetflow；strategy_source_type_test 等同步
2. [ ] `kernel/`：engine.go Sources 标签/OITopData、engine_prompt.go NetFlow 段（:586-588、:859-902）、engine_analysis.go、formatter.go 清理；相关测试同步
3. [ ] `market/`：Data.Netflow/OITop 字段与填充；`trader/`：auto_trader* 的 ai500/oi 币源分支
4. [ ] `agent/`：ai500.go、get_ai500_list 工具、strategy_field_catalog、skill_execution_handlers、llm_flow_extractor、skill_domain_context
5. [ ] `api/`：handler_ai500.go + 路由；`provider/nofxos/` 整包删除
6. [ ] Docker 门禁：build + test 全绿 → commit
7. [ ] 前端：AI500Panel、CoinSourceEditor 收敛、types、i18n、WelcomeScreen、Agent 页 tab
8. [ ] web build/test 全绿 → commit
9. [ ] 全量 grep（PRD 词表）归零复核；浏览器探针（策略编辑器/Agent 页/排行榜回归/klines）截图留档 `probe/`

## 验证命令

```bash
docker run --rm -v "$PWD":/w -w /w -e GOPROXY=https://goproxy.cn,direct golang:1.25-alpine go build ./...
docker run --rm -v "$PWD":/w -w /w -e GOPROXY=https://goproxy.cn,direct golang:1.25-alpine go test ./...
npm --prefix web run build && npm --prefix web run test
```

## 回滚点

store/kernel、agent/api、前端三个 commit 边界；探针不过不合父。
