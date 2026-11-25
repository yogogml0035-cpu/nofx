# 执行计划：⑯ scan-alignment

## 待办

- [x] 1. `trader/scan_schedule.go`：`scanTimeframe` 与 `nextScanTime`（Unix 网格 / 7d / 1w 周一 / 30d 月初，+5s）。
- [x] 2. `trader/auto_trader.go`：Run 循环改用定时器等待下一收线时刻，保留启动立即执行与 stop 通道。
- [x] 3. 单测矩阵 `trader/scan_schedule_test.go`。
- [x] 4. 门禁：`git diff --check`、`make test`、`make build`。
- [ ] 5. 独立检查子智能体检查；修复并补验。
- [ ] 6. 合父、归档、推送、清理；更新地图与 10.5 进度。

## 验证命令

```bash
docker run --rm -v "$PWD":/w -w /w -v nofx-gocache:/go -e GOPROXY=https://goproxy.cn,direct \
  golang:1.25-alpine sh -c "apk add --no-cache gcc musl-dev && CGO_ENABLED=1 go test -count=1 ./trader/"
git diff --check && make test && make build
```

## 审查门禁

- 边界只由 UTC 墙钟与主周期决定；1w/30d 的日历语义正确；无补跑风暴。
- 非法/缺失周期回退 ScanInterval；stop 通道及时退出。
- 不改变决策/风控逻辑与配置结构。
