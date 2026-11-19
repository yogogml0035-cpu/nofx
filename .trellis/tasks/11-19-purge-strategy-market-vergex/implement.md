# C2 执行计划

纯删除任务，PRD-only + 本执行清单（无新技术设计）。顺序：后端 → 前端 → 文档，每步编译/测试校验。go 命令统一经 Docker（宿主机无 Go）。

## 步骤

1. [ ] 后端 store：`store/strategy.go` 删 IsPublic/ConfigVisible/PublishStrategyConfig/ListPublic；相关测试同步
2. [ ] 后端 api：`api/strategy.go` 删 handlePublicStrategies/attachPublishConfig/DTO 字段；`api/server.go` 删路由
3. [ ] 后端 agent + telegram：grep `IsPublic|is_public|publish` 逐文件清语义（清单见 PRD 第 13 条，含 `skills/strategy_management.json` 与 `telegram/agent/apicall.go` 白名单条目），工具 schema 描述同步
4. [ ] Docker 门禁 `go build ./... && go test ./...` 全绿 → commit
5. [ ] 前端路由导航：paths.ts / paths.test.ts / AppRoutes / HeaderBar 两处 / StrategyMarketPage 删除
6. [ ] AgentGrid 跳转改 `/traders`；DataPage 删 iframe 置空态占位；PublishSettingsEditor 删除；StrategyStudioPage / types/strategy.ts / i18n 键清理（三语言）
7. [ ] `npm --prefix web run build && npm --prefix web run test` 全绿 → commit
8. [ ] 文档营销：README、六语言 i18n README、CLA.md、branding.ts、CommunitySection
9. [ ] 全量 grep 复跑（PRD 两条词表，排除集含 docs/legal、docs/plans、screenshots）归零 → commit
10. [ ] 浏览器探针（临时后端 + 前端 dev）：导航/404/跳转/占位页/策略编辑器截图留档任务目录 `probe/`

## 验证命令

```bash
docker run --rm -v "$PWD":/w -w /w -e GOPROXY=https://goproxy.cn,direct golang:1.25-alpine go build ./...
docker run --rm -v "$PWD":/w -w /w -e GOPROXY=https://goproxy.cn,direct golang:1.25-alpine go test ./...
rg -il "vergex|strategy-market|strategy_market|strategyMarket|publishToMarket|handlePublicStrategies|ListPublic|ConfigVisible|config_visible|PublishConfig|publish_config|strategies/public" --hidden --glob '!CHANGELOG.md' --glob '!CHANGELOG.zh-CN.md' --glob '!.trellis/**' --glob '!.git/**' --glob '!web/node_modules/**' --glob '!.nofx-local/**' --glob '!scratch/**' --glob '!docs/legal/**' --glob '!docs/plans/**' --glob '!screenshots/**' .
rg -n "IsPublic|\"is_public\"" --type go
```

## 回滚点

每步一 commit；发现问题按 commit 边界手动恢复文件状态（禁止 git 回滚命令）。
