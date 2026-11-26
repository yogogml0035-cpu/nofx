# 执行计划：⑭ chart-klines-gate

> 归档后回填（2026-11-26）：下列待办在交付时均已完成，原文件没有回填勾选。完成证据见计划文档 10.5 第 14 行、本任务的 probe/ 与父分支的合并提交。

## 待办（完成即勾选）

- [x] 1. `api/handler_klines.go`：加密货币分支改 `market.GetGateKlines`（包级 `cryptoKlines`
  测试缝）；删 `getKlinesFromCoinank`/CoinAnk 映射/回退；limit 1..2000；`QuoteVolume=Volume`。
- [x] 2. 删除 `provider/coinank/coinank_api/`（kline/base_coin/symbol + tests）与
  `provider/coinank/kline.go`；保留其他 CoinAnk 能力与 enum。
- [x] 3. 前端：AdvancedChart/ChartWithOrders/ChartTabs/TraderDashboardPage 去 `exchange`；
  tooltip 只留 `V(USDT)`；柱状图用 `quoteVolume`；删无调用方的 `TradingViewChart`/`ChartWithOrders`/
  `ChartWithOrdersSimple`。
- [x] 4. 后端单测：Gate mock/注入的取数形状、exchange 忽略、limit 边界、错误 500、周期拒绝。
- [x] 5. 前端门禁：`npm test`、`tsc --noEmit`、`vite build`。
- [x] 6. 门禁：`git diff --check`、`make test`、`make build`。
- [x] 7. 独立检查子智能体检查；修复并补验。
- [x] 8. 合父、归档、推送父分支、清理子分支/worktree；更新地图与 10.5 进度。

## 验证命令

```bash
# 后端定向
docker run --rm -v "$PWD":/w -w /w -v nofx-gocache:/go -e GOPROXY=https://goproxy.cn,direct \
  golang:1.25-alpine sh -c "apk add --no-cache gcc musl-dev && CGO_ENABLED=1 go test -count=1 ./api/ ./market/"

# 前端
cd web && npm test && npx tsc --noEmit && npm run build

# 门禁
git diff --check && make test && make build
```

## 审查门禁

- 加密货币 K 线只走 Gate；`exchange` 不再改变取数；无 CoinAnk/Binance 回退。
- 非保留周期被拒；limit 上界 2000。
- 前端 K 线 URL 不含 `exchange=`；成交量只展示 USDT。
- 删除的 provider 与组件无残留引用（构建双向确认）。

## 回滚点

- 回滚单元是本子任务提交边界；未过门禁不合入父分支。

## 证据

- `api/handler_klines_test.go`、前端用例与构建输出；父任务 dev-offline E2E 覆盖真实 `/api/klines`。
