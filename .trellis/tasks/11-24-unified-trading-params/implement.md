# 执行计划：杠杆与保证金模式（unified-trading-params）

## 步骤

1. [ ] **规划**：`prd.md`、`design.md`、本文件、`implement.jsonl` / `check.jsonl`，连同研究留档一并提交
2. [ ] **实现适配层**
   - `trader/types/interface.go`：能力接口 `MarginSettingsApplier`；`ProtectedOpenRequest.IsCrossMargin`
   - `trader/gate/margin_settings.go`：`MarginSettings`、`ErrNoPositionSettings`、
     `PositionMarginSettings`、`ApplyMarginSettings`，以及重写的 `SetLeverage`、`SetMarginMode`
   - `trader/gate/protected_open.go`：提交订单前一次性生效，失败不开仓
   - `trader/gate/trader_orders.go`：删除旧的 `SetLeverage` / `SetMarginMode` 与 `RISK_LIMIT_EXCEEDED` 分支；
     `openMarket` 的杠杆设置改为失败即不开仓
3. [ ] **接入交易路径**
   - `trader/auto_trader_orders.go`：删除独立的 `SetMarginMode` 调用，开仓请求带上 `IsCrossMargin`
   - `trader/auto_trader_grid.go`：网格初始化按 trader 的设置一次性生效
4. [ ] **单测**
   - `trader/gate/margin_settings_test.go`：请求路径与参数矩阵、返回值读回、读回不一致报错、
     重复设置、交易所拒绝、杠杆非正不发请求、两个旧方法的保持语义、没有条目的两种处理
   - `trader/gate/protected_open_test.go`：设置生效先于下单；生效失败不提交订单；请求带 trader 的模式
   - `trader/gate/size_test.go` 等既有测试的桩改为新接口
   - `trader/auto_trader_orders_test.go`：trader 的全仓 / 逐仓设置传入开仓请求；交易路径不再调用 `SetMarginMode`
   - `trader/auto_trader_grid*_test.go`：网格初始化的接入
5. [ ] **迭代期定向测试**：docker go 只跑 `./trader/gate/` 与 `./trader/`
6. [ ] **W4 真实验证** `probe/livecheck.go`（design.md 的十步 + 只读探测），还原并核对基线，证据留档。
   实测结论回写 design.md 与 research
7. [ ] **门禁**：`git diff --check`、`make test`、`make build`
8. [ ] **独立检查**：trellis-check 子智能体检查，问题由主会话修复并补验
9. [ ] **收口**：规格更新 → 提交子分支 → 父 worktree 合入 → 归档子任务 → 推送父分支 →
   删除子分支与子 worktree → 同步计划文档 10.5 节与父任务的 `prd.md`、`implement.md`

## 验证命令

宿主机没有 Go，go 命令统一经 Docker（同父任务 implement.md）。`make` 里的 go 命令经 PATH 最前的转发脚本
`/Users/hsikey/Trading/nofx-wt/.bin/go` 转进 Docker（脚本带 CGO_ENABLED=1 与 gcc 安装，go-sqlite3 需要 cgo）；
子 worktree 软链主仓已安装的 `web/node_modules`。

```bash
# 定向测试（迭代期）
docker run --rm -v "$PWD":/w -w /w -v nofx-gocache:/go -e GOPROXY=https://goproxy.cn,direct \
  golang:1.25-alpine sh -c "apk add --no-cache gcc musl-dev && CGO_ENABLED=1 go test -count=1 ./trader/gate/ ./trader/"

# 子任务门禁
make test     # go test -v ./... 与前端测试
make build    # go build -o nofx
```

W4 脚本在 Docker 内交叉编译（`CGO_ENABLED=0 GOOS=darwin GOARCH=arm64`）后在宿主机运行，
API key 经环境变量传入，不落盘。

## 回滚点

- 提交边界即回滚单元：规划、实现与单测、W4 证据各自成提交；失败时手动恢复文件，不使用 git 回滚命令。
- W4 中途失败：脚本先记录基线，还原步骤不依赖前面步骤的结果；还原后读回不一致时立即报告，
  人工用同一接口改回基线。
- 子任务失败不外溢：门禁未过的子分支不合入父分支。
