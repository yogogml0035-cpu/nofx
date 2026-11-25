# W2 真实分批平仓证据（2026-11-25）

- `livecheck.go` — 探针脚本（独立 main，不进构建产物）：预检 → 随单保护开多 17 张 →
  可分批状态 → 50% 分批 → 仓位与保护核对 → 100%/1% 拒绝 → 市价全平 → 终态零残留。
- `livecheck-2026-11-25.log` — 通过运行的完整输出：`LIVECHECK_PASS`（全步骤通过）。
- `livecheck-2026-11-25.jsonl` — 同一运行的逐步 JSONL 证据。

## 运行方式

宿主机无 Go 且容器出口 IP 不在 Gate key 白名单（VPN TUN 出口）：Docker 内交叉编译
`CGO_ENABLED=0 GOOS=darwin GOARCH=arm64`，宿主机运行。凭据经环境变量 `GATE_KEY` /
`GATE_SECRET` 传入，不落盘、不打印。

```bash
docker run --rm -v "$PWD":/w -v /tmp:/hosttmp -w /w -v nofx-gocache:/go \
  -e GOPROXY=https://goproxy.cn,direct golang:1.25-alpine sh -c \
  'CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o /hosttmp/partial-close-livecheck \
  ./.trellis/tasks/11-25-partial-close/probe/livecheck.go'
GATE_KEY=… GATE_SECRET=… /tmp/partial-close-livecheck -out livecheck-2026-11-25.jsonl \
  > livecheck-2026-11-25.log
```

## 步骤与结果（2026-11-25 16:43 本地时间；检查修复后重跑）

预检：`DOGE_USDT` 无持仓、无未触发条件单、无挂单；仓位设置 `cross 75x`；可用 107.47078 USDT。

| 步 | 操作 | 读数 |
|---|---|---|
| 1 | 随单保护开多 | 17 张 @ 0.05457，名义 9.2769 USDT（含 5% 裕度）；orderId=56013526826388858；SL 0.049104 / TP 0.060016 |
| 2 | 仓位与保护核对 | positionAmt=170（17 张 × 10 币/张）、leverage=75；委托回读 tpsl 存在 |
| 3 | 可分批状态 | `supported=true contracts=17 step=1` |
| 4 | 50% 分批平仓 | 8 张、剩 9 张；orderId=56013526826389199；fillPrice=0.05456；名义 4.3648 USDT |
| 5 | 分批后核对 | positionAmt=90（9 张）；`supported=true contracts=9`；开仓委托的 tpsl 仍可读回；条件单列表 status=open 2 条（观察） |
| 6 | 拒绝用例 | 100% → “full close”错误；1%（9 张 × 1% 向下取整 0 张）→ “below one size step”；未缓存读数 contracts=9，仓位未变 |
| 7 | 市价全平 | `close=true + size=0`，orderId=56013526826389504 |
| 8 | 终态 | 无持仓、无未触发条件单、无挂单；仓位设置仍为 `cross 75x`；可用 107.45891（差 −0.01187，开平仓 taker 手续费） |

## 说明

- 随单止盈止损是委托级保护；分批平仓不撤不改保护单。分批后 `VerifyOrderProtection` 以开仓
  委托回读为准仍然通过。条件单列表在分批后出现 2 条（同一交易对），列表形态只记录观察，
  不作断言；子任务 11 做改单时需再核对两类合约的列表稳定性。
- 官方文档站在本次执行时不可达（`ERR_CONNECTION_RESET`，备用域名超时）；字段语义以
  Gate 官方 Go SDK v7.2.184（`model_futures_order.go`：`size` 买入为正/卖出为负、`close=true`
  配 `size=0`、`reduce_only`、`tpsl_tp_trigger_price` / `tpsl_sl_trigger_price`）与仓库已留档的
  官方文档摘录（子任务 3 research）为准。
- 单笔名义 ≤ 10 USDT；无残留仓位、委托或条件单。