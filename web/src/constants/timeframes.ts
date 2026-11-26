// 策略可选的 K 线周期（D25）：与后端 store.RetainedTimeframes、Gate 合约 K 线枚举一致。
// 决策按主周期每根 K 线收线后 5 秒触发。
export const RETAINED_TIMEFRAMES = [
  '10s', '1m', '5m', '15m', '30m', '1h', '4h', '8h', '1d', '7d', '1w', '30d',
] as const

export type RetainedTimeframe = (typeof RETAINED_TIMEFRAMES)[number]

export function isRetainedTimeframe(value: string | undefined | null): value is RetainedTimeframe {
  return !!value && (RETAINED_TIMEFRAMES as readonly string[]).includes(value)
}
