import { describe, expect, it } from 'vitest'
import {
  buildTraderUpdateRequest,
  parseInitialBalance,
} from './traderUpdateRequest'

describe('parseInitialBalance', () => {
  it('accepts plain decimal numbers above 0', () => {
    expect(parseInitialBalance('500.5')).toBe(500.5)
    expect(parseInitialBalance(' 107.5 ')).toBe(107.5)
    expect(parseInitialBalance('.5')).toBe(0.5)
    expect(parseInitialBalance('12.')).toBe(12)
    expect(parseInitialBalance('0.01')).toBe(0.01)
  })

  it('rejects empty, zero, negative and anything that is not a plain decimal', () => {
    const rejected = [
      '',
      '   ',
      '0',
      '0.0',
      '-5',
      '50abc', // parseFloat 会放过它
      '0x10', // Number() 会把它当成 16
      '1e3', // Number() 会把它当成 1000
      '1,000',
      'Infinity',
      'NaN',
      '12.3.4',
    ]
    for (const raw of rejected) {
      expect(parseInitialBalance(raw), JSON.stringify(raw)).toBeNull()
    }
  })
})

describe('buildTraderUpdateRequest', () => {
  const data = {
    name: 'demo',
    ai_model_id: 'model-1',
    exchange_id: 'exchange-1',
    strategy_id: 'strategy-1',
    scan_interval_minutes: 5,
    is_cross_margin: true,
    show_in_competition: false,
  }

  it('passes the editable fields through', () => {
    expect(buildTraderUpdateRequest(data)).toEqual(data)
  })

  it('defaults a missing visibility flag to shown', () => {
    // 缺失字段在 JSON.stringify 时会被丢掉，后端只能保持旧值；默认值与后端一致为 true
    const withoutFlag: Partial<typeof data> = { ...data }
    delete withoutFlag.show_in_competition

    expect(
      buildTraderUpdateRequest(withoutFlag as typeof data).show_in_competition
    ).toBe(true)
  })

  it('carries the initial balance only when the form provides one', () => {
    // 后端把「正值」当作明确给出。表单没有给出时请求里不能出现这个键：
    // 否则换绑交易所时旧的初始余额会被当成明确给出的值保留下来
    expect(buildTraderUpdateRequest(data)).not.toHaveProperty('initial_balance')

    const withBalance = buildTraderUpdateRequest({
      ...data,
      initial_balance: 350,
    })
    expect(withBalance).toEqual({ ...data, initial_balance: 350 })
  })
})
