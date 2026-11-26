# 实施计划：子任务 6 回撤监控（drawdown-monitor）

## 顺序清单

1. [ ] 适配层：`trader/gate/trader_account.go` 新增 `positionLeverage`（lever → 同模式旧字段 → 0），
   `GetPositions` 的 `leverage` 改 float64。
2. [ ] 监控：`trader/auto_trader_risk.go` 新增 `isOwnPosition`（网格子句 + DB OPEN 记录子句，
   其余不属于）；`checkPositionDrawdown` 开头过滤归属、杠杆读取改 float64 实际值（≤0 回落 10 倍）。
3. [ ] 单测 `trader/gate/trader_account_test.go`（追加）：`lever` 优先、逐仓旧字段兜底、全仓旧字段兜底、
   全缺 → 0、异常值 → 0（不失败）、map 里类型为 float64。
4. [ ] 单测 `trader/auto_trader_risk_test.go`（新增）：
   - 杠杆取值：float64 实际值参与收益率；0/缺失回落 10 倍；
   - 阈值判定：>5% 且回撤 ≥40% 触发全平（long / short 各一）、峰值记录与更新、未达阈值不平仓；
   - 仓位归属：DB OPEN 记录命中 → 处理并全平；无记录 / 无 store / DB 出错 → 跳过；
     网格品种命中 → 处理；非网格品种 → 跳过；不可归属仓位不写峰值缓存。
5. [ ] 迭代验证：`go test`（docker 转发，定向 `./trader/...` `./trader/gate/...`）。
6. [ ] 独立检查子智能体检查，修复发现的问题并补验。
7. [ ] 带仓只读真实读数（probe，宿主机运行）：真实开一笔 ≤10U 仓位（W2 授权范围，DOGE_USDT）→
   `GetPositions` 读数（核对 leverage 与 lever 一致、float64）→ 归属判定探针 → 全平还原 → 零残留核对。
8. [ ] 门禁：`git diff --check`、`make test`、`make build`。
9. [ ] 同步计划文档 10.5 进度、父任务 implement.md，提交并合入父分支。

## 验证命令

```bash
# 定向 go test（宿主机无 Go，docker 转发，CGO 因 sqlite 需要）
docker run --rm -v "$PWD":/w -w /w -v nofx-gocache:/go -e GOPROXY=https://goproxy.cn,direct \
  golang:1.25-alpine sh -c "apk add --no-cache gcc musl-dev >/dev/null && CGO_ENABLED=1 go test -count=1 ./trader/... ./trader/gate/..."

# 子任务门禁
make test && make build
```

真实读数探针在宿主机运行（容器出口不在 Gate key IP 白名单）：交叉编译 darwin/arm64 + CGO_ENABLED=1，
密钥经环境变量注入，不落盘。

## 回滚点

单提交边界：手动恢复 `trader/gate/trader_account.go`、`trader/auto_trader_risk.go` 与两个测试文件。
未过门禁不合入父分支。
