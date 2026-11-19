import { describe, expect, it } from 'vitest'
import {
  formatCompactNumber,
  formatFundingRate,
  sortByChangeAsc,
  sortByChangeDesc,
  sortByFundingAsc,
  sortByFundingDesc,
  sortByVolumeDesc,
  type SymbolLike,
} from './boardUtils'

function sym(partial: Partial<SymbolLike>): SymbolLike {
  return { symbol: 'X', ...partial }
}

// change_24h_pct 排序：undefined 按 0 处理（omitempty 字段缺失语义）
describe('change sorting', () => {
  const input = [
    sym({ symbol: 'A', change_24h_pct: 5 }),
    sym({ symbol: 'B', change_24h_pct: undefined }),
    sym({ symbol: 'C', change_24h_pct: -3 }),
    sym({ symbol: 'D', change_24h_pct: 1.5 }),
    sym({ symbol: 'E', change_24h_pct: 0 }),
  ]

  it('desc puts highest first, undefined treated as 0', () => {
    const out = sortByChangeDesc(input)
    expect(out.map((s) => s.symbol)).toEqual(['A', 'D', 'B', 'E', 'C'])
  })

  it('asc puts lowest first, undefined treated as 0', () => {
    const out = sortByChangeAsc(input)
    expect(out.map((s) => s.symbol)).toEqual(['C', 'B', 'E', 'D', 'A'])
  })

  it('does not mutate input', () => {
    const copy = [...input]
    sortByChangeDesc(input)
    expect(input.map((s) => s.symbol)).toEqual(copy.map((s) => s.symbol))
  })
})

// 成交额排序
describe('volume sorting', () => {
  it('desc puts highest volume first', () => {
    const out = sortByVolumeDesc([
      sym({ symbol: 'A', volume_24h: 100 }),
      sym({ symbol: 'B', volume_24h: 5_000_000 }),
      sym({ symbol: 'C', volume_24h: undefined }),
    ])
    expect(out.map((s) => s.symbol)).toEqual(['B', 'A', 'C'])
  })
})

// 资金费率排序：正榜降序（多头付费最深）、负榜升序；原值排序
describe('funding sorting', () => {
  const input = [
    sym({ symbol: 'A', funding_rate: 0.0001 }),
    sym({ symbol: 'B', funding_rate: -0.0002 }),
    sym({ symbol: 'C', funding_rate: 0.0005 }),
    sym({ symbol: 'D', funding_rate: -0.00005 }),
    sym({ symbol: 'E', funding_rate: 0 }),
  ]

  it('desc puts deepest positive first', () => {
    const out = sortByFundingDesc(input)
    expect(out.map((s) => s.symbol)).toEqual(['C', 'A', 'E', 'D', 'B'])
  })

  it('asc puts deepest negative first', () => {
    const out = sortByFundingAsc(input)
    expect(out.map((s) => s.symbol)).toEqual(['B', 'D', 'E', 'A', 'C'])
  })
})

// 费率换算：Gate 原始小数 ×100 保留 4 位小数
describe('funding rate formatting', () => {
  it('converts raw decimal to percent with 4 decimals', () => {
    expect(formatFundingRate(0.000389)).toBe('0.0389%')
  })

  it('handles zero rate', () => {
    expect(formatFundingRate(0)).toBe('0.0000%')
  })

  it('handles negative rate', () => {
    expect(formatFundingRate(-0.00012)).toBe('-0.0120%')
  })

  it('returns dash for undefined/NaN', () => {
    expect(formatFundingRate(undefined)).toBe('—')
    expect(formatFundingRate(Number.NaN)).toBe('—')
  })
})

// 紧凑数字
describe('compact number formatting', () => {
  it('formats billions/millions/thousands', () => {
    expect(formatCompactNumber(3_600_000_000)).toBe('$3.6B')
    expect(formatCompactNumber(266_800_000)).toBe('$266.8M')
    expect(formatCompactNumber(84_800)).toBe('$84.8K')
    expect(formatCompactNumber(500)).toBe('$500')
  })

  it('returns dash for undefined', () => {
    expect(formatCompactNumber(undefined)).toBe('—')
  })
})
