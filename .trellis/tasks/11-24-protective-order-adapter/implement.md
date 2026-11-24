# 执行计划：保护单适配（protective-order-adapter）

## 步骤

1. [x] **实现 `trader/gate/protective_orders.go`**（候选 `381d7f02`）
   - 类型与常量：`ProtectiveKind`、`PriceRuleGE/LE`、`TriggerOrderView`
   - 纯函数：`protectiveTriggerRule`、`BuildPositionProtectiveOrder`、`ApplyAttachedTPSL`、
     `classifyTriggerOrder`（方向 + rule → Kind）、`parseTriggerOrder`（SDK 结构 → 视图）
   - 薄封装：`ListTriggerOrders`、`PlaceProtectiveOrder`、`AmendProtectiveOrderTrigger`、
     `AmendProtectiveOrderSize`、`CancelProtectiveOrder`、`CancelProtectiveOrdersByKind`
2. [x] **单测 `trader/gate/protective_orders_test.go`**
   - 构造矩阵：方向（多/空）× 类型（止损/止盈）→ `rule`、`order_type`、`initial` 逐项断言
   - 拒绝矩阵：非法方向/类型、非正触发价、随单止盈止损顺序颠倒、空合约
   - 判定矩阵：`order_type` + `rule` → Side/Kind；普通条件单（无 order_type）Kind 未知
   - 解析：触发价非法字符串整体报错；`amount` 优先于 `size` 的读数呈现
   - 改单请求体：路径与请求体 ID 一致、`amount` 提交、`size=0`
3. [x] **迭代期定向测试**：docker go 只跑 `./trader/gate/`
4. [x] **真实验证** `probe/livecheck.go`（全程立即还原）：`LIVECHECK_PASS steps=10`，
   终态零残留（独立核查 CLEAN）。实测结论改写了两处研究缺口：仓位级条件单无持仓被拒
   （H.1）、API 创建的触价单当前不能改单（H.2，撤旧建新为修改止损止盈的实际路径）。
   环境注意：Gate key 有 IP 白名单，容器经 VPN TUN 出口被拒，验证脚本须交叉编译到
   宿主机运行（probe/README.md）
5. [x] **门禁**（在 `381d7f02` 上执行）：`git diff --check` 通过；后端 22 个含测试的包全部
   通过、前端 11 个测试文件 142 个用例通过；`make build` 通过。go 转发脚本须带
   CGO_ENABLED=1 与 gcc（go-sqlite3 依赖 cgo）
6. [x] **独立检查**：trellis-check 报告 PASS（无 P0/P1）。已修复：P2 证据日志被全局
   `*.log` ignore 漏掉未入库 → 改名 `livecheck-2026-11-24.jsonl` 入库并更新 README；
   P3 gofmt 测试文件、design.md 三处文案与实现对齐、prd 验收勾选。未采纳：livecheck
   失败路径的尽力撤销兜底（检查建议可延后；脚本是一次性验证工具且本次运行零残留，
   修改已运行过的证据脚本反而削弱留档的可信度）。修复后定向测试与 vet 通过
7. [ ] **收口**：提交子分支 → 父 worktree 合入 → 归档子任务 → push 父分支 → 删除子分支与
   子 worktree → 同步计划文档 10.5 节与父任务 implement.md

## 验证命令

宿主机没有 Go，go 命令统一经 Docker（同父任务 implement.md）：

```bash
# 定向测试（迭代期）
docker run --rm -v "$PWD":/w -w /w -v nofx-gocache:/go -e GOPROXY=https://goproxy.cn,direct \
  golang:1.25-alpine sh -c "apk add --no-cache gcc musl-dev && CGO_ENABLED=1 go test -count=1 ./trader/gate/"

# 子任务门禁
make test     # go test -v ./... 与前端测试
make build    # go build -o nofx
```

真实写操作验证脚本经 docker go 运行（worktree 内 `go run ./probe` 或编译后执行），
API key 从环境变量读取，不落盘。

## 回滚点

- 提交边界即回滚单元：实现与单测一个提交，真实验证证据一个提交；失败时手动恢复文件。
- 子任务失败不外溢：门禁未过的子分支不合入父分支。
