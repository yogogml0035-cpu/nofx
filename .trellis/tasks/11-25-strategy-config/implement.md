# 执行计划：⑧ strategy-config

迭代期只跑定向测试；收敛后跑一次完整门禁。检查由独立子智能体完成后，主会话修复并补验。

## 清单

- [ ] 1. store 层：常量/clamp/结构体/默认值/diff/normalizeTimeframe（+store 单测）
- [ ] 2. kernel：`engine_position.go` 删 ratio 参数与上限块；`engine_analysis.go` 删透传、兜底 3m→5m；`engine_prompt.go` 口径切百分比（+定向 `go test ./kernel/`）
- [ ] 3. trader：删 `enforcePositionValueRatio`/`isMajorAsset`、调用点、兜底 12→10（+`go test ./trader/`）
- [ ] 4. agent：`tools.go` 锁定名单与文案；`skill_management_handlers.go` removeLocked+摘要（中英）；`skill_execution_handlers.go` 标签/关键词/编辑分支+不开单诊断（中英）；`strategy_field_catalog.go` editable 名单；skills 两个 JSON；`agent/trade.go` 上限块删除+兜底 10（+`go test ./agent/`）
- [ ] 5. api：`server.go` schema 说明行
- [ ] 6. 前端：types/strategy.ts、RiskControlEditor.tsx、IndicatorEditor.tsx、strategy-translations.ts（+`tsc --noEmit`、vitest 定向→全量）
- [ ] 7. 文档：STRATEGY_MODULE.md/.zh-CN.md 默认值口径
- [ ] 8. 全仓旧语义残留检索（PRD 验收第 1 条的 grep）
- [ ] 9. 完整门禁：`git diff --check`、`make test`、`make build`（Docker go 转发 + web/node_modules 软链）
- [ ] 10. 独立检查子智能体 → 主会话修复 → 补验
- [ ] 11. 合父（--no-ff 合并提交）、父 worktree 归档、jsonl 路径修正、计划文档 10.5/文件表/F 发现、父任务 prd/implement 地图同步、journal、push 父分支
- [ ] 12. 清子 worktree 被忽略文件后移除，`merge-base --is-ancestor` 确认后删子分支

## 验证命令

```bash
# 定向（宿主机无 Go，走 Docker；须带 nofx-gocache 与 GOPROXY）
docker run --rm -v "$PWD":/w -w /w -v nofx-gocache:/go -e GOPROXY=https://goproxy.cn,direct \
  -e GOFLAGS=-buildvcs=false golang:1.25-alpine sh -c \
  'apk add --no-cache gcc musl-dev >/dev/null 2>&1; CGO_ENABLED=1 go test -count=1 ./store/ ./kernel/ ./trader/ ./agent/'

# 前端（先软链主仓 web/node_modules，确认 package-lock 一致）
cd web && npx tsc --noEmit && npx vitest run

# 完整门禁
make test && make build
```

## 回滚点

按清单步为单元手动回滚（不加 git revert）；子分支未过门禁不合父。
