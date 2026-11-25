# 执行计划：⑬ engine-klines-gate

## 待办（完成即勾选）

- [x] 1. `market` 切 Gate：`GetWithTimeframes`、`GetBoxData`、`GetWithExchange`；`calculatePriceChangeByBars`
  改用 Gate 周期时长；删除 `getKlinesFromCoinAnk`/`parseTimeframeToMinutes`。
- [x] 2. 删除无调用方代码：`market/timeframe.go`（+测试）、`coinank_api/kline_ws.go`、`depth_ws.go`
  （+测试）、`kernel.FetchMarketData`；`api/strategy.go` 的 `"3m"` 兜底改 `"5m"`。
- [x] 3. 提示词：周期顺序 `{10s..30d}`、成交量列/文案标 USDT（中英）。
- [x] 4. 单测：Gate mock 下 GetWithTimeframes（含 10s/7d/30d）、GetBoxData（1h/500 请求形状）、
  GetWithExchange、主/副周期失败语义；提示词周期与成交量；stale 判定 15 分钟窗口；
  删除后无残留引用（`go build ./...`）。
- [x] 5. 真实探针：12 个保留周期经 GetWithTimeframes、箱体、无 AI 提示词渲染。
- [x] 6. 门禁：`git diff --check`、`make test`、`make build`。
- [ ] 7. 独立检查子智能体检查；修复并补验。
- [ ] 8. 合父、归档、推送父分支、清理子分支/worktree；更新地图与 10.5 进度（F41 收口）。

## 验证命令

```bash
# 定向
docker run --rm -v "$PWD":/w -w /w -v nofx-gocache:/go -e GOPROXY=https://goproxy.cn,direct \
  golang:1.25-alpine sh -c "apk add --no-cache gcc musl-dev && CGO_ENABLED=1 go test -count=1 ./market/ ./kernel/ ./api/"

# 真实探针（宿主机）
docker run --rm -v "$PWD":/w -v /tmp:/hosttmp -w /w -v nofx-gocache:/go \
  -e GOPROXY=https://goproxy.cn,direct golang:1.25-alpine sh -c \
  'CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o /hosttmp/engine-klines-livecheck \
   ./.trellis/tasks/11-25-engine-klines-gate/probe/livecheck.go'
/hosttmp/engine-klines-livecheck probe/livecheck-<date>.jsonl > probe/livecheck-<date>.log

# 门禁
git diff --check && make test && make build
```

## 审查门禁

- 父分支每步可编译；没有“先删后接”的中间状态。
- 引擎/网格/旧路径全部走 Gate；无 CoinAnk/Binance 回退。
- 新周期（10s/7d/30d）在提示词中可见；成交量明确为 USDT 成交额。
- 删除的文件没有任何引用（`grep` 与构建双向确认）。

## 回滚点

- 回滚单元是本子任务提交边界；未过门禁不合入父分支。
- 真实探针只读，不需要还原。

## 证据

- `probe/livecheck-<date>.{log,jsonl}`：12 周期真实取数、箱体、提示词渲染（无 AI）。