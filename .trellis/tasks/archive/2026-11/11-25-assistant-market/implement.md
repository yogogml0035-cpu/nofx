# 执行计划：⑮ assistant-market

> 归档后回填（2026-11-26）：下列待办在交付时均已完成，原文件没有回填勾选。完成证据见计划文档 10.5 第 15 行、本任务的 probe/ 与父分支的合并提交。

## 待办（完成即勾选）

- [x] 1. market：GateTicker 扩展 24h 高低/绝对涨跌；新增 `GetGateContractMetrics`（标记/指数/
  资金费率/下次结算毫秒/OI USD）；单测矩阵。
- [x] 2. agent/market_source.go：测试缝 + `buildMarketSnapshot` 纯函数；单测。
- [x] 3. tools.go：snapshot/kline 改 Gate；`validKlineInterval` 与 schema 枚举改 D25；删除
  Binance 取数。
- [x] 4. web.go：删 HandleKlines/HandleTicker/proxyBinance/binanceClient；HandleTickers 改 Gate；
  routes 删两个路由。
- [x] 5. sentinel.go/brain.go/agent.go：ticker 改 Gate；market.Get → GetWithTimeframes；
  无 AI 兜底改本地摘要。
- [x] 6. 删除旧 market 路径与 kernel 旧格式分支及测试；`go build ./...` 无残留引用。
- [x] 7. 真实探针：Gate ticker/快照/K 线工具（含 10s/7d/30d）`PROBE_PASS`。
- [x] 8. 门禁：`git diff --check`、`make test`、`make build`。
- [x] 9. 独立检查子智能体检查；修复并补验。
- [x] 10. 合父、归档、推送父分支、清理子分支/worktree；更新地图与 10.5 进度。

## 验证命令

```bash
# 定向
docker run --rm -v "$PWD":/w -w /w -v nofx-gocache:/go -e GOPROXY=https://goproxy.cn,direct \
  golang:1.25-alpine sh -c "apk add --no-cache gcc musl-dev && CGO_ENABLED=1 go test -count=1 ./market/ ./agent/ ./api/"

# 真实探针（容器内，公开行情可达）
docker run --rm -v "$PWD":/w -w /w -v nofx-gocache:/go -e GOPROXY=https://goproxy.cn,direct \
  golang:1.25-alpine sh -c "apk add --no-cache gcc musl-dev >/dev/null; CGO_ENABLED=1 \
  go build -o /tmp/assistant-probe ./.trellis/tasks/11-25-assistant-market/probe/livecheck.go && \
  cd .trellis/tasks/11-25-assistant-market/probe && /tmp/assistant-probe livecheck-<date>.jsonl > livecheck-<date>.log"

# 门禁
git diff --check && make test && make build
```

## 审查门禁

- 助手行情只走 Gate；无 Binance 取数残留（`grep fapi.binance.com` 为空）。
- 两个旧接口路由删除；tickers 保留且字段兼容前端。
- 快照/K 线工具的成交量=USDT、OI=USD、周期=保留集合。
- 旧 market 路径与结构删除后无引用（构建双向确认）。

## 回滚点

- 回滚单元是本子任务提交边界；未过门禁不合入父分支。

## 证据

- `probe/livecheck-<date>.{log,jsonl}`：Gate ticker/快照/K 线的真实取数。
