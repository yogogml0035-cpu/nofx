# 子任务 4 执行计划

## 顺序

1. [ ] `trader/types/interface.go`：新增 `ProtectedOpener` 能力接口与请求/结果结构
2. [ ] `trader/gate/contract.go`：新增 `ContractsFromQuantityFloor`
3. [ ] `trader/gate/protected_open.go`：随单保护开仓 + `VerifyOrderProtection`
4. [ ] `trader/gate/trader_orders.go`：开平仓原语重写、`SetStopLoss`/`SetTakeProfit` 不支持、
       按类型撤销薄封装、`CancelAllOrders` 条件单部分改新原语
5. [ ] `trader/gate/trader_account.go`、`order_sync.go`：`getContract` 移除，改 `GetContractSpec`
6. [ ] `trader/gate/trader.go`：删除 `getContract`、`contractsCache`
7. [ ] `trader/auto_trader_orders.go`：开仓路径改 `ProtectedOpener` + 成交后核对 + 缺失即平仓；
       平仓取价改 `GetMarketPrice`
8. [ ] 单元测试（gate 层 + trader 层 fail-closed）
9. [ ] 门禁：`git diff --check`、`make test`、`make build`
10. [ ] probe/livecheck.go：W2（DOGE 17 张开+全平）、W3（ETH 0.1 张开+平），宿主机运行
11. [ ] 独立检查子智能体检查，修复发现的问题
12. [ ] 父 worktree 合入、归档、推送、清理

## 验证命令

```bash
# 定向测试（迭代期；worktree 根目录执行）
docker run --rm -v "$PWD":/w -w /w -v nofx-gocache:/go -e GOPROXY=https://goproxy.cn,direct \
  golang:1.25-alpine sh -c "apk add --no-cache gcc musl-dev && CGO_ENABLED=1 go test -count=1 ./trader/gate/ ./trader/"

# 子任务门禁
make test && make build && git diff --check
```

## 回滚点

单一提交边界：子分支上的实施提交；未过门禁不合入父分支。
