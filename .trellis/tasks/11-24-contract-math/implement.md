# 执行计划：张数换算（contract-math）

前置：子 worktree `nofx-wt/feat-2026-1123-gate-trading-foundation--contract-math`，基于父分支 `daa8369a`。
宿主机没有 Go，go 命令统一走 Docker（`golang:1.25-alpine`，`GOPROXY=https://goproxy.cn,direct`；
该包依赖含 sqlite，需要 `apk add gcc musl-dev` 与 `CGO_ENABLED=1`）。

## 步骤

1. [x] 查官方文档并落盘原文：API v4 文档站（根页与 Futures 分页）、Agent 文档页经网页阅读器抓取
       （本机直连被重置），SDK 文档取自官方仓库。见 `research/gate-contract-size-docs.md`。
       两项与预期不同：官方资料没有写明“步长等于 `order_size_min`”（推论），也没有交易所层面的最小下单金额；
       另发现市价单有独立的最大张数 `market_order_size_max`
2. [x] `trader/gate/decimal.go`：精确解析、按最短十进制取 float、定点输出、向下与向上取整的除法
3. [x] `trader/gate/contract.go`：`ContractSpec` 与构造校验、`SizeForNotional`（最小可交易额、市价单上限）、
       `SizeForCloseRatio`、`ContractsFromQuantity`、`QuantityFromContracts`、`GetContractSpec`（缓存 10 分钟）；
       `trader/gate/trader.go` 加缓存字段
4. [x] 单测矩阵：`decimal_test.go`、`contract_test.go`（含计划文档 6.1 节两张表的逐格复算、
       浮点掉格的对照用例），包内全量 `-race` 通过
5. [x] 7 个合约元数据真实只读读数：`contracts=7 failed=0 mismatched=0`，脚本与输出留档 `probe/`
6. [ ] 独立检查子智能体检查；发现的问题由主会话修复并补验
7. [ ] 门禁：`git diff --check`、`make test`、`make build`
8. [ ] 合入父分支，归档，推送父分支留档，删除子分支与子 worktree

## 验证命令

```bash
# 包内测试（迭代期）
docker run --rm -v "$PWD":/w -w /w -v nofx-gocache:/go golang:1.25-alpine sh -c \
  "apk add --no-cache gcc musl-dev && CGO_ENABLED=1 GOPROXY=https://goproxy.cn,direct go test -count=1 -race ./trader/gate/"

# 7 个合约元数据真实读数（只读；凭据经环境变量传入，不落盘）
#   脚本：本任务 probe/livecheck.go，运行时放到子 worktree 的 scratch/livecheck/main.go（scratch 被 gitignore）
docker run --rm -v "$PWD":/w -w /w -v nofx-gocache:/go -e GATE_KEY -e GATE_SECRET golang:1.25-alpine sh -c \
  "apk add --no-cache gcc musl-dev && CGO_ENABLED=1 GOPROXY=https://goproxy.cn,direct go run ./scratch/livecheck"

# 子任务门禁（make 目标里的 go 命令经 Docker 执行，见第 7 步的实际命令）
git diff --check
make test
make build
```

## 回滚点

- 提交边界：规划文档一个提交，实现与证据一个提交；出错时按提交边界手动恢复文件，不使用 git 回滚命令
- 本任务是纯新增：未通过门禁时不合入父分支，父分支不受影响
