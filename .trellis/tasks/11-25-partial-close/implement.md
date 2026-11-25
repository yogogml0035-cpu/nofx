# 执行计划：⑩ partial-close

迭代期只跑定向测试；收敛后跑一次完整门禁。实施由主会话完成，独立检查子智能体检查后由主会话
修复并补验。

## 清单

- [ ] 1. 子分支与 worktree：从父分支最新提交 `bd9562d5` 建
      `feat/2026_1123_gate-trading-foundation--partial-close`，worktree 位于
      `nofx-wt/feat-2026-1123-gate-trading-foundation--partial-close`；在子 worktree 激活任务。
- [ ] 2. `trader/types`：新增 `PartialCloseStatus`、`PartialCloseRequest`、`PartialCloseResult`、
      `PartialCloser`；注释写明比例范围、剩余张数与不可分批语义。
- [ ] 3. `trader/gate`：新增 `partial_close.go`（状态与执行）、`ContractSpec` 市价单最大张数校验；
      执行用 `findPositionSize` + `SizeForCloseRatio` + reduce-only 市价单；单测覆盖状态/换算/下单矩阵。
- [ ] 4. `kernel` 契约：`Decision.ClosePercent`、两个新动作、`validateDecision` 比例校验
      （(0,100)，100 指向全平）；单测。
- [ ] 5. `kernel` 提示词：动作清单、`close_percent` 说明、可用条件、中英示例；`PositionInfo`
      两个字段与持仓行标记；提示词单测。
- [ ] 6. `trader` 执行：`partialCloser()`、`executePartialCloseWithRecord`、动作分发、排序优先级；
      单测（路由、能力缺失、参数与记录）。
- [ ] 7. `trader` 持仓上下文：`buildTradingContext` 调用 `PartialCloseStatus`，错误/未实现
      标记不可分批；单测。
- [ ] 8. 前端：`DecisionCard` 动作映射与 `close_percent` 展示；类型；前端测试/构建。
- [ ] 9. 文档：`STRATEGY_MODULE.md`/`.zh-CN.md` 动作清单与优先级同步。
- [ ] 10. 定向测试：`go test ./kernel/ ./trader/ ./trader/gate/ ./trader/types/`，前端相关测试。
- [ ] 11. W2 真实验证：`probe/livecheck.go`（Docker 交叉编译 darwin/arm64，宿主机运行）：
      预检 → 随单保护开 DOGE 17 张 → 保护核对 → 50% 分批（8 张，剩 9）→ 仓位/保护核对 →
      1% 与 100% 被拒（无副作用）→ 全平 → 零残留；保存日志与 README。
- [ ] 12. 完整门禁：`git diff --check`、`make test`、`make build`。
- [ ] 13. 独立检查子智能体：范围/PRD/设计/规范/候选/验收规则；主会话修复 P0–P3 并补验。
- [ ] 14. 合父（`--no-ff`）、父 worktree 归档、计划文档 10.5 勾选、父任务 prd/implement 地图同步、
      journal、push 父分支。
- [ ] 15. 确认父分支包含子提交、子 worktree 干净、证据已保存后删除子分支与 worktree。

## 验证命令

宿主机无 Go，Go 命令经 Docker（带 `nofx-gocache` 与 `GOPROXY`）：

```bash
# 定向
docker run --rm -v "$PWD":/w -w /w -v nofx-gocache:/go -e GOPROXY=https://goproxy.cn,direct \
  -e GOFLAGS=-buildvcs=false golang:1.25-alpine sh -c \
  'apk add --no-cache gcc musl-dev >/dev/null 2>&1; CGO_ENABLED=1 go test -count=1 ./kernel/ ./trader/ ./trader/gate/ ./trader/types/'

# 完整门禁
make test
make build
```

W2 探针交叉编译与运行（凭据只经环境变量传入）：

```bash
docker run --rm -v "$PWD":/w -v /tmp:/hosttmp -w /w -v nofx-gocache:/go \
  -e GOPROXY=https://goproxy.cn,direct golang:1.25-alpine sh -c \
  'CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o /hosttmp/partial-close-livecheck \
  ./.trellis/tasks/11-25-partial-close/probe/livecheck.go'
GATE_KEY=... GATE_SECRET=... /tmp/partial-close-livecheck -out ... > ...adapter-log.txt
```

## 回滚点

- 子任务提交边界即回滚单元；任一门禁失败不合入父分支。
- W2 探针每一步异常时尽力还原（残留仓位市价全平）；不修改保护单。
- 若实测发现分批后剩余仓位失去保护，停止合父并回报（属于需要在计划层决定的新问题）。

## 边界

- 不启动 trader、不激活策略。
- 其他交易所适配器不实现分批能力，保持全平路径不变。
- 读取/修改当前止损止盈属子任务 11，本子任务只验证分批后保护仍可读回。