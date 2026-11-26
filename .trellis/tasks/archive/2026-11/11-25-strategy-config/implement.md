# 执行计划：⑧ strategy-config

> 归档后回填（2026-11-26）：下列待办在交付时均已完成，原文件没有回填勾选。完成证据见计划文档 10.5 第 8 行、本任务的 probe/ 与父分支的合并提交。

迭代期只跑定向测试；收敛后跑一次完整门禁。检查由独立子智能体完成后，主会话修复并补验。

## 清单

- [x] 1. store 层：常量/clamp/结构体/默认值/diff/normalizeTimeframe（+store 单测 `strategy_position_test.go`，clamp 7 例 + 周期矩阵 + legacy JSON + diff 警告）
- [x] 2. kernel：`engine_position.go` 删 ratio 参数与上限块；`engine_analysis.go` 删透传、兜底 3m→5m；`engine_prompt.go` 口径切百分比（12/60 硬编码与盈亏比校验按计划保留给 ⑨）
- [x] 3. trader：删 `enforcePositionValueRatio`、`isMajorAsset`（及检查后死代码 `isBTCETH`）、调用点、兜底 12→10
- [x] 4. agent：`tools.go` 锁定名单与文案；`skill_management_handlers.go` removeLocked+摘要（中英）；`skill_execution_handlers.go` 标签/关键词/编辑分支+不开单诊断（中英）+planner 指令段；`strategy_field_catalog.go` editable 名单；skills 两个 JSON；`agent/trade.go` 上限块删除+兜底 10
- [x] 5. api：`server.go` schema 说明行（含 D25 周期枚举）
- [x] 6. 前端：types/strategy.ts、RiskControlEditor.tsx（百分比输入 1–20 步长 0.1 默认 10；最小下单输入默认 10）、IndicatorEditor.tsx（12 周期）、strategy-translations.ts（zh/en/es）；`tsc --noEmit` 0 错、vitest 158/158、vite build 成功
- [x] 7. 文档：STRATEGY_MODULE.md/.zh-CN.md 默认值口径
- [x] 8. 全仓旧语义残留检索：ratio 族 0 命中（除计划文档留档与 legacy 用例）；12 兜底文案清零（kernel 12/60 硬编码属 ⑨，prompt-guide 与代码一致）
- [x] 9. 完整门禁：`git diff --check` 干净、`make test` 全绿（22 Go 包 + 前端 158 用例）、`make build` 成功
- [x] 10. 独立检查子智能体：**PASS**（P0/P1 无；1 P2 + 4 P3）。主会话修复：P2 断言收紧为 `"\"error\""` 约定；P3-1 删 `isBTCETH` 死代码；P3-4 en 标签改 "position size percent"。P3-2（10s/7d/30d 引擎取数在 ⑫⑬ 落地）、P3-3（网格 RiskInfo.position_percent 同名异义）留档进计划文档。修复后 `go build ./...` + `go test ./trader/ ./agent/` 全绿
- [x] 11. 合父（--no-ff 合并提交）、父 worktree 归档、jsonl 路径修正、计划文档 10.5/文件表/F 发现、父任务 prd/implement 地图同步、journal、push 父分支
- [x] 12. 清子 worktree 被忽略文件后移除，`merge-base --is-ancestor` 确认后删子分支

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
