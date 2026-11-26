# 执行计划：⑫ market-source

> 归档后回填（2026-11-26）：下列待办在交付时均已完成，原文件没有回填勾选。完成证据见计划文档 10.5 第 12 行、本任务的 probe/ 与父分支的合并提交。

## 待办（完成即勾选）

- [x] 1. `market/gate_market.go`：保留周期与时长、`GetGateKlines`（sum 成交量、毫秒时间、
  严格校验）、`GateTicker`/`GetGateTicker`/`ListGateTickers`。
- [x] 2. 单测 `market/gate_market_test.go`：周期矩阵、解析矩阵（字符串/数字、缺失、非正、
  非递增、低高关系）、limit 边界、HTTP/JSON 错误、ticker 单条/列表/失败注入。
- [x] 3. 真实探针 `probe/livecheck.go`：7 标的 × 12 周期取数与校验、ticker 一致性、
  拒绝非保留周期；证据落 `probe/livecheck-<date>.{log,jsonl}`。
- [x] 4. 门禁：`git diff --check`、`make test`、`make build`。
- [x] 5. 独立检查子智能体检查；修复并补验。
- [x] 6. 合父、归档、推送父分支、清理子分支/worktree；更新地图与 10.5 进度。

## 验证命令

```bash
# 定向
docker run --rm -v "$PWD":/w -w /w -v nofx-gocache:/go -e GOPROXY=https://goproxy.cn,direct \
  golang:1.25-alpine sh -c "apk add --no-cache gcc musl-dev && CGO_ENABLED=1 go test -count=1 ./market/"

# 真实探针（宿主机，公开行情无需 key；容器出口不在 Gate 白名单只影响私有接口）
docker run --rm -v "$PWD":/w -v /tmp:/hosttmp -w /w -v nofx-gocache:/go \
  -e GOPROXY=https://goproxy.cn,direct golang:1.25-alpine sh -c \
  'CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o /hosttmp/market-livecheck \
   ./.trellis/tasks/11-25-market-source/probe/livecheck.go'
/hosttmp/market-livecheck probe/livecheck-<date>.jsonl > probe/livecheck-<date>.log

# 门禁
git diff --check && make test && make build
```

## 审查门禁

- 只用 Gate；没有回退分支。
- 成交量只用 `sum`；缺失即失败，不回落 `v`。
- 非保留周期一律拒绝；`limit` 越界拒绝；时间戳乱序/重复拒绝。
- 数值解析失败不回落猜测值；错误信息带合约名与周期。

## 回滚点

- 回滚单元是本子任务提交边界；未过门禁不合入父分支。
- 真实探针只读，不需要还原。

## 证据

- `probe/livecheck-<date>.{log,jsonl}`：7 标的 × 12 周期的真实取数结果与 ticker 一致性。