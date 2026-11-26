# 带仓只读真实读数证据（2026-11-25，W2 授权范围内：DOGE_USDT ≤10U 开平、立即还原）

- `livecheck.go` — 探针脚本（独立 main，不进构建产物）。依赖 `trader/drawdown_monitor_probe.go`
  （`//go:build livecheck` 探针桥，暴露未导出的单轮监控 / 归属判定 / 峰值缓存；正常构建与测试不编译它）
- `livecheck-2026-11-25.jsonl` — 通过运行的完整证据：`LIVECHECK_PASS`（0 失败）
- `livecheck-2026-11-25.adapter-log.txt` — 同一次运行的适配器日志（stdout）

## 运行方式

宿主机无 Go 且容器出口 IP 不在 Gate key 白名单：Docker 内交叉编译
`CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -tags livecheck`（须带 `-tags livecheck`
把 trader 包的探针桥编进构建），宿主机运行。凭据经环境变量 `GATE_KEY` / `GATE_SECRET` 传入，
不落盘、不打印。

```bash
docker run --rm -v "$PWD":/w -v /tmp:/hosttmp -w /w -v nofx-gocache:/go \
  -e GOPROXY=https://goproxy.cn,direct golang:1.25-alpine sh -c \
  'CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -tags livecheck \
  -o /hosttmp/dm-livecheck ./.trellis/tasks/11-25-drawdown-monitor/probe/livecheck.go'
GATE_KEY=… GATE_SECRET=… /tmp/dm-livecheck -out livecheck-2026-11-25.jsonl \
  > livecheck-2026-11-25.adapter-log.txt
```

## 步骤与结果（2026-11-25 12:21 本地时间）

预检：`position_mode=single`，账户无持仓、无未完结委托、无未触发条件单，可用 107.49163 USDT；
三个 watched 合约的设置条目记为基线（DOGE_USDT 全仓 75x）。

| 步 | 操作 | 读数 |
|---|---|---|
| 1 | 随单保护开仓 DOGE_USDT | 17 张 @ 0.05472，名义 9.3024 USDT（≤10 含 5% 裕度）；SL/TP 现价 ±10%；杠杆 75 全仓 = 基线原值幂等应用 |
| 2 | 适配层杠杆 vs 原始条目 `lever` | `lever="75"` == 适配层返回 `75`（float64）；positionAmt=170 DOGE、entry 0.05472、mark 0.05473 |
| 3 | 归属判定（D40） | 网格品种（DOGEUSDT）持仓判为本 trader 的 = true；非网格品种（ETH_USDT）= false |
| 4 | 回撤监控一轮（首次见仓不触发） | 峰值缓存 1.370614035088139% == 按实际杠杆 75 手算收益率（差 3.5e-17）；监控后仓位仍在 |
| 5 | 全平还原 + 终态 | `CloseLong("DOGEUSDT", 0)` 成功；全账户无持仓（3 条 size=0 条目）、无委托、无条件单；设置条目与基线一致；可用 107.48063，差 −0.011（双边 taker 手续费） |

说明：归属判定的 DB 子句（本地 trader_positions OPEN 记录）由单元测试覆盖
（`trader/auto_trader_risk_test.go`，临时 sqlite 真实查询）；探针不构造 store
（CGO sqlite 无法随探针以 CGO_ENABLED=0 交叉编译），桥的归属走网格子句。
