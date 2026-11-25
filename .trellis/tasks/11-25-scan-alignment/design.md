# 技术设计：⑯ scan-alignment

## 边界与职责

- 新增 `trader/scan_schedule.go`：周期选取与下一个触发时刻的纯函数。
- `trader/auto_trader.go` 的 `Run` 循环：用定时器等待下一个触发时刻，替换固定 `time.NewTicker`。
- 不触碰决策/风控逻辑、配置结构与前端。

## 契约

```go
// scanCloseDelay 收线后固定延迟：给 Gate 收线与本地时钟留出稳定窗口。
const scanCloseDelay = 5 * time.Second

// scanTimeframe 返回策略配置的主周期；缺失/不合法返回 ""（调用方回退到 ScanInterval）。
func scanTimeframe(config *store.StrategyConfig) string

// nextScanTime 返回 now 之后（含当前边界尚未到点的触发时刻）最近的一次扫描时刻：
// 触发时刻 = 周期边界 + delay，边界按 UTC 墙钟：
//   10s/1m/5m/15m/30m/1h/4h/8h/1d → Unix 纪元网格；
//   7d → Unix 纪元的 7 天网格；1w → 每周一 00:00 UTC；30d → 每月 1 日 00:00 UTC。
func nextScanTime(now time.Time, timeframe string, delay time.Duration) (time.Time, error)
```

## 算法

```go
func nextScanTime(now time.Time, timeframe string, delay time.Duration) (time.Time, error) {
    now = now.UTC()
    switch timeframe {
    case "1w":
        weekday := (int(now.Weekday()) + 6) % 7 // Monday=0
        monday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -weekday)
        trigger := monday.Add(delay)
        if trigger.After(now) { return trigger, nil }
        return monday.AddDate(0, 0, 7).Add(delay), nil
    case "30d":
        first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
        trigger := first.Add(delay)
        if trigger.After(now) { return trigger, nil }
        return first.AddDate(0, 1, 0).Add(delay), nil
    default:
        duration, err := market.GateIntervalDuration(timeframe) // 含 7d 与其余保留周期
        if err != nil { return time.Time{}, err }
        step := int64(duration / time.Second)
        boundary := (now.Unix() / step) * step
        trigger := time.Unix(boundary, 0).UTC().Add(delay)
        if trigger.After(now) { return trigger, nil }
        return time.Unix(boundary+step, 0).UTC().Add(delay), nil
    }
}
```

- `delay` 允许 0（测试用）；负值拒绝。
- `now` 恰在触发时刻（trigger == now）→ 视为已到点，排下一次边界（`After` 为 false）。

## Run 循环

```go
cadence := scanTimeframe(at.config.StrategyConfig)
if cadence == "" {
    at.logWarnf("⚠️ strategy primary timeframe missing/invalid, falling back to ScanInterval %v", at.config.ScanInterval)
} else {
    at.logInfof("⚙️ Scan aligned to %s close + %v", cadence, scanCloseDelay)
}
// 启动立即执行一轮（网格/普通策略）
for {
    // running 检查
    next, err := nextScanTime(time.Now(), cadence, scanCloseDelay)
    var timer *time.Timer
    if err != nil {
        timer = time.NewTimer(at.config.ScanInterval) // 回退：旧固定间隔
    } else {
        at.logInfof("⏰ Next scan at %s (UTC)", next.Format(time.RFC3339))
        wait := time.Until(next)
        if wait < 0 { wait = 0 }
        timer = time.NewTimer(wait)
    }
    select {
    case <-timer.C:
        run cycle
    case <-at.stopMonitorCh:
        timer.Stop()
        return nil
    }
}
```

- 周期内耗时超过一个周期时，下一轮从“当前时刻之后”的边界算起，自动跳过错过的边界。
- `stopMonitorCh` 到达时停止计时器并退出（比旧实现更及时）。
- `ScanInterval <= 0` 的旧配置：回退时若 `<=0`，用 1 分钟保底（防御）。

## 测试

`trader/scan_schedule_test.go`：

- `nextScanTime` 矩阵：
  - `1m`：12:00:30 → 12:01:05；12:00:02 → 12:00:05；12:00:05 → 12:01:05。
  - `5m/15m/4h`：边界对齐；`1d`：UTC 00:00+5s。
  - `7d`：Unix 纪元 7 天网格（周四）。
  - `1w`：周三 → 下周一 00:00:05；周一 00:00:02 → 当日 00:00:05；周一 00:00:10 → 下周一。
  - `30d`：11 月 15 日 → 12 月 1 日 00:00:05；12 月 1 日 00:00:02 → 当日 00:00:05；12 月 1 日 00:00:10 → 1 月 1 日。
  - 非法周期 → error；负 delay → error。
- `scanTimeframe`：nil/空/非法 → ""；合法 D25 周期 → 原值。

## 权衡与已知边界

- **为什么用 UTC 墙钟**：Gate K 线边界是 UTC（1d 为 UTC 00:00、1w 周一、30d 月初）；
  本地时区/DST 不参与。
- **5 秒延迟**：Gate 收线后数据稳定需要一点时间；固定值便于测试与解释。
- **长周期策略**：主周期 30d 时扫描每月一次；仓位风险由独立每分钟的回撤监控兜底（既有设计）。
- **回退**：旧配置没有主周期时按 `ScanInterval`；不删除该字段（存量兼容）。