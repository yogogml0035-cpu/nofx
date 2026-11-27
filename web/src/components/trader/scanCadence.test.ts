import { describe, expect, it } from 'vitest'
import {
  normalizeScanIntervalMinutes,
  scanTimeframeForMinutes,
} from './scanCadence'

describe('scanTimeframeForMinutes', () => {
  it('maps intervals that exactly match a K-line period', () => {
    expect(scanTimeframeForMinutes(5)).toBe('5m')
    expect(scanTimeframeForMinutes(15)).toBe('15m')
    expect(scanTimeframeForMinutes(30)).toBe('30m')
    expect(scanTimeframeForMinutes(60)).toBe('1h')
  })

  it('returns null for intervals without an exact period', () => {
    expect(scanTimeframeForMinutes(1)).toBeNull()
    expect(scanTimeframeForMinutes(3)).toBeNull()
    expect(scanTimeframeForMinutes(7)).toBeNull()
    expect(scanTimeframeForMinutes(90)).toBeNull()
    expect(scanTimeframeForMinutes(240)).toBeNull()
    // 7d 与 1w 时长相同但收线边界不同，不能从间隔唯一确定
    expect(scanTimeframeForMinutes(7 * 24 * 60)).toBeNull()
  })

  it('rejects non-positive and non-finite values', () => {
    expect(scanTimeframeForMinutes(0)).toBeNull()
    expect(scanTimeframeForMinutes(-5)).toBeNull()
    expect(scanTimeframeForMinutes(Number.NaN)).toBeNull()
    expect(scanTimeframeForMinutes(Number.POSITIVE_INFINITY)).toBeNull()
  })
})

describe('normalizeScanIntervalMinutes', () => {
  it('rounds to an integer and clamps to 3–60', () => {
    expect(normalizeScanIntervalMinutes(5)).toBe(5)
    expect(normalizeScanIntervalMinutes(7.5)).toBe(8)
    expect(normalizeScanIntervalMinutes(0)).toBe(3)
    expect(normalizeScanIntervalMinutes(1)).toBe(3)
    expect(normalizeScanIntervalMinutes(-5)).toBe(3)
    expect(normalizeScanIntervalMinutes(90)).toBe(60)
    expect(normalizeScanIntervalMinutes(1440)).toBe(60)
  })

  it('falls back to the minimum for missing or non-finite values', () => {
    expect(normalizeScanIntervalMinutes(undefined)).toBe(3)
    expect(normalizeScanIntervalMinutes(null)).toBe(3)
    expect(normalizeScanIntervalMinutes(Number.NaN)).toBe(3)
  })
})