// 扫描间隔与 K 线周期的对齐关系。后端 trader/scan_schedule.go 的 scanTimeframeForInterval 使用
// 同一组周期（手动面板范围 3–60 分钟）：间隔正好是 5/15/30/60 分钟时，决策在该周期收线后 5 秒
// 触发；其余间隔按固定计时器运行。7d 与 1w 时长相同但收线边界不同，无法从间隔唯一确定。
export const MIN_SCAN_INTERVAL_MINUTES = 3
export const MAX_SCAN_INTERVAL_MINUTES = 60

const ALIGNED_SCAN_TIMEFRAMES: ReadonlyArray<{
  minutes: number
  timeframe: string
}> = [
  { minutes: 5, timeframe: '5m' },
  { minutes: 15, timeframe: '15m' },
  { minutes: 30, timeframe: '30m' },
  { minutes: 60, timeframe: '1h' },
]

// normalizeScanIntervalMinutes 把表单/存量配置收敛到 3–60 的整数分钟。
// 后端接收的是 int：小数会在 JSON 绑定阶段变成 400；存量 0/1/90 等值也会在这里归一。
export function normalizeScanIntervalMinutes(
  value: number | null | undefined
): number {
  if (value === null || value === undefined || !Number.isFinite(value)) {
    return MIN_SCAN_INTERVAL_MINUTES
  }
  return Math.min(
    MAX_SCAN_INTERVAL_MINUTES,
    Math.max(MIN_SCAN_INTERVAL_MINUTES, Math.round(value))
  )
}

// scanTimeframeForMinutes 返回与扫描间隔等长的 K 线周期；没有完全相等的周期时返回 null。
export function scanTimeframeForMinutes(minutes: number): string | null {
  if (!Number.isFinite(minutes) || minutes <= 0) {
    return null
  }
  return (
    ALIGNED_SCAN_TIMEFRAMES.find((entry) => entry.minutes === minutes)
      ?.timeframe ?? null
  )
}