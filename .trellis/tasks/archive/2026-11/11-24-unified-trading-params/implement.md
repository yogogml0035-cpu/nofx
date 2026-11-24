# 执行计划：杠杆与保证金模式（unified-trading-params）

## 步骤

1. [x] **规划**：`prd.md`、`design.md`、本文件、`implement.jsonl` / `check.jsonl`，连同研究留档一并提交
   ——`0d926e1d`（研究留档由研究子智能体定向抓取文档站三个页面后落档，主会话逐条核对）
2. [x] **实现适配层**
   - `trader/types/interface.go`：能力接口 `MarginSettingsApplier`；`ProtectedOpenRequest.IsCrossMargin`
   - `trader/gate/margin_settings.go`：`MarginSettings`、`ErrNoPositionSettings`、
     `PositionMarginSettings`、`ApplyMarginSettings`，以及重写的 `SetLeverage`、`SetMarginMode`
   - `trader/gate/protected_open.go`：提交订单前一次性生效，失败不开仓
   - `trader/gate/trader_orders.go`：删除旧的 `SetLeverage` / `SetMarginMode` 与 `RISK_LIMIT_EXCEEDED` 分支；
     `openMarket` 的杠杆设置改为失败即不开仓
   - 读回解析以 `lever` 优先（文档写明它逐步取代旧字段；逐仓时 `cross_leverage_limit` 残留旧值），
     返回体的 `mode` 带 `dual_*` 时报错
3. [x] **接入交易路径**
   - `trader/auto_trader_orders.go`：删除独立的 `SetMarginMode` 调用，开仓请求带上 `IsCrossMargin`
   - `trader/auto_trader_grid.go`：网格初始化按 trader 的设置一次性生效（`applyMarginSettings`），
     生效后才标记 `IsInitialized`；其余适配器沿用两次调用、行为不变
4. [x] **单测**
   - `trader/gate/margin_settings_test.go`：请求路径与参数矩阵、回退到持仓条目的全部分支
     （接口报错 / 返回值读不出 / 返回值不一致 × 条目等于或不等于目标）、交易所错误在错误链里、
     只有 `POSITION_NOT_FOUND` 代表没有条目、杠杆非正不发请求、两个旧方法的保持语义、
     解析矩阵（`lever` 与同模式旧字段矛盾即报错、旧字段缺失或读不出时取 `lever`、逐仓残留旧全仓杠杆、
     双向持仓模式拒绝）、`SetMarginMode` 保持杠杆
   - `trader/gate/protected_open_test.go`：设置生效先于下单；生效失败不提交订单；请求带 trader 的模式；
     `gateMock` 按线上格式模拟仓位设置条目（无条目 → HTTP 400 + `POSITION_NOT_FOUND`）
   - `trader/gate/size_test.go` 的桩改为新接口；`trader/gate/trader_test.go` 里不可达的 `set_leverage` 桩已删除
   - `trader/auto_trader_orders_test.go`：trader 的全仓 / 逐仓设置传入开仓请求；交易路径不再调用 `SetMarginMode`
   - `trader/auto_trader_grid_margin_test.go`：网格初始化的接入（失败不标记已初始化；其他适配器行为不变）
5. [x] **迭代期定向测试**：docker go 只跑 `./trader/gate/` 与 `./trader/`——通过；`go vet` 通过；
   本次改动的行 `gofmt` 无差异（`protected_open.go`、`protected_open_test.go`、`types/interface.go`
   基线里就有的偏差没有重排）
6. [x] **W4 真实验证** `probe/livecheck.go`（design.md 的十步 + 只读探测），还原并核对基线，证据留档。
   ——三轮均 `LIVECHECK_PASS`，入库的是第三轮：在最终候选 `d758d3ec` 的干净检出上运行（23 步 + 1 条观察记录），
   证据 `probe/livecheck-2026-11-24.jsonl` 与适配器日志。DOGE_USDT 还原为全仓 75x，BTC_USDT、ETH_USDT
   条目不变，可用余额前后一致，无持仓、无挂单、无条件单。
   实测结论回写 design.md 与 research G 节：`set_leverage` 线上已部署（不带 `dual_side`）；
   逐仓条目的 `cross_leverage_limit` 残留旧值；重复设置返回 200；超过上限的 label 是 `LEVERAGE_EXCEEDED`
7. [x] **门禁**：`git diff --check`、`make test`、`make build`
   ——最终候选 `e1b6fdc3` 上：`git diff --check d135abc3..HEAD` 无输出；`make test`（`MAKE_TEST_EXIT=0`）
   后端 22 个含测试的包全部 ok、18 个包没有测试文件，前端 11 个文件 142 个用例通过；
   `make build`（`MAKE_BUILD_EXIT=0`）通过。宿主机没有 Go，make 里的 go 命令经转发脚本在 Docker 里执行。
   全量门禁共跑过两次：第一次在独立检查修复之前的代码上（同样通过），修复期间只跑定向测试，
   收敛后跑了这一次
8. [x] **独立检查**：trellis-check 子智能体检查，问题由主会话修复并补验
   ——首轮 NEEDS FIX：无 P0/P1，4 项 P2（`lever` 优先级的注释与实现不符、规格错误矩阵与实现不符、W4 证据的
   覆盖缺口与表述偏强、4 个存活的变异）与 9 项 P3；变异实验 31 个变体杀死 26 个。修复见 `d758d3ec`
   （读回矛盾即报错、补齐回退分支与 label 区分的测试、`%w` 包装交易所错误等）与 `eb84f2e4`
   （在修复后的代码上重跑 W4、修正文档表述）。复核 PASS：上次各项判定已解决，变异实验扩到 53 个可编译变体、
   杀死 50 个，新存活 3 个（次要分支的错误链）。这 3 个与复核列出的其余 P3 在 `e1b6fdc3` 修复，
   主会话在临时副本上重做这 3 个变异，全部被杀死。复核提出的计划文档与父任务文档的同步放在父分支的收口提交里做
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
