# 执行计划：⑯ scan-alignment

> 归档后回填（2026-11-26）：下列待办在交付时均已完成，原文件没有回填勾选。完成证据见计划文档 10.5 第 16 行、本任务的 probe/ 与父分支的合并提交。

## 待办

- [x] 1. `trader/scan_schedule.go`：`scanTimeframe` 与 `nextScanTime`（Unix 网格 / 7d / 1w 周一 / 30d 月初，+5s）。
- [x] 2. `trader/auto_trader.go`：Run 循环改用定时器等待下一收线时刻，保留启动立即执行与 stop 通道。
- [x] 3. 单测矩阵 `trader/scan_schedule_test.go`。
- [x] 4. 门禁：`git diff --check`、`make test`（Go 全部包 + 前端 14 文件 / 162 用例）、`make build`。
- [x] 5. 独立检查 **PASS**（无 P0/P1/P2；6 项 P3）。修复 4 项：safe 模式日志改为中性口径（`auto_trader_loop.go:167`）、Run 循环每轮重取主周期（引擎首轮归一化别名后自愈）、回退路径输出实际等待且启动告警用 1 分钟兜底真实值、补 30m/8h 与 1w/30d「恰在触发时刻」用例。留档 2 项：回退路径按「每轮结束后再等 ScanInterval」（design 已定，与旧 ticker 节拍差异可忽略）、`GetStatus.scan_interval` 仍返回配置值（PRD 明确不改周期配置/前端）。
- [x] 6. 合父、归档、推送、清理；更新地图与 10.5 进度。

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
