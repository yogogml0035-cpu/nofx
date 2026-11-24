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
6. [x] 独立检查子智能体检查（候选 `f33d78e3`）：结论通过，没有阻断项，也没有必须修改的缺陷。
       四个换算函数与设计规则逐条推演一致，并与一份独立实现的对照程序在 23.1 万个用例上零差异。
       10 条建议级发现全部处理，修正提交 `62ab50a5`：
       - 浮点容差改为“10⁻⁹ 张与张数 × 10⁻¹² 取较大者”：约 900 万张以上的持仓原先会被误判为不在步长上
       - `floorDiv`、`ceilDiv` 对负数也正确取整（原先只在非负入参下成立）
       - 注释如实标注步长取 `order_size_min` 是推论；市价单上限写作 `0.00` 时归一成 `0`；
         读取失败的报错不再重复合约名
       - 补测试：缓存与换算的并发调用、最小下单金额为 0 且不足一个步长、取整除法的负数、
         超大持仓的容差、`0.00` 归一化
       - 验证脚本的输出带 UTC 时间与代码提交号，凭据改为可选（只调公开接口）；
         按 `62ab50a5` 重跑读数仍是 `contracts=7 failed=0 mismatched=0`
       - 留给后续子任务的事项记入计划文档 F31、F33–F35
7. [x] 门禁（在 `5d41dde5` 上执行一次）：`git diff --check` 通过；`make test` 后端 22 个含测试的包全部通过、
       前端 11 个测试文件 142 个用例通过；`make build` 通过。宿主机没有 Go，make 目标里的 go 命令经一个
       转发脚本在 Docker 里执行；子 worktree 没有前端依赖，软链了主仓已安装的 `web/node_modules`
       （两个 worktree 的 `package-lock.json` 一致）
8. [ ] 合入父分支，归档，推送父分支留档，删除子分支与子 worktree

## 验证命令

```bash
# 包内测试（迭代期）
docker run --rm -v "$PWD":/w -w /w -v nofx-gocache:/go golang:1.25-alpine sh -c \
  "apk add --no-cache gcc musl-dev && CGO_ENABLED=1 GOPROXY=https://goproxy.cn,direct go test -count=1 -race ./trader/gate/"

# 7 个合约元数据真实读数（只读，只调公开接口，不需要凭据）
#   脚本：本任务 probe/livecheck.go，运行时放到子 worktree 的 scratch/livecheck/main.go（scratch 被 gitignore）
docker run --rm -v "$PWD":/w -w /w -v nofx-gocache:/go -e GOPROXY=https://goproxy.cn,direct -e LIVECHECK_COMMIT=<提交号> \
  golang:1.25-alpine sh -c "apk add --no-cache gcc musl-dev && CGO_ENABLED=1 go run ./scratch/livecheck"

# 子任务门禁（宿主机没有 Go：把 go 命令转进 Docker 的转发脚本放在 PATH 最前，再执行 make 目标）
git diff --check
make test
make build
```

## 回滚点

- 提交边界：规划文档一个提交，实现与证据一个提交；出错时按提交边界手动恢复文件，不使用 git 回滚命令
- 本任务是纯新增：未通过门禁时不合入父分支，父分支不受影响
