import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { IndicatorEditor } from './IndicatorEditor'
import type { IndicatorConfig } from '../../types'

const makeConfig = (primaryCount: number): IndicatorConfig => ({
  klines: {
    primary_timeframe: '5m',
    primary_count: primaryCount,
    enable_multi_timeframe: false,
    selected_timeframes: ['5m'],
  },
  enable_raw_klines: true,
  enable_ema: false,
  enable_macd: false,
  enable_rsi: false,
  enable_atr: false,
  enable_boll: false,
  enable_volume: false,
  enable_oi: false,
  enable_funding_rate: false,
})

function Harness({
  initial,
  onConfigChange,
}: {
  initial: IndicatorConfig
  onConfigChange: (config: IndicatorConfig) => void
}) {
  const [config, setConfig] = useState(initial)
  return (
    <IndicatorEditor
      config={config}
      onChange={(next) => {
        setConfig(next)
        onConfigChange(next)
      }}
      language="zh"
    />
  )
}

const lastPrimaryCount = (onChange: ReturnType<typeof vi.fn>) => {
  const calls = onChange.mock.calls
  return (calls[calls.length - 1][0] as IndicatorConfig).klines.primary_count
}

const getInput = () => screen.getByRole('spinbutton') as HTMLInputElement

describe('IndicatorEditor kline count', () => {
  it('uses the same 10-2000 bounds as the backend', () => {
    render(<Harness initial={makeConfig(30)} onConfigChange={() => {}} />)
    const input = getInput()
    expect(input.min).toBe('10')
    expect(input.max).toBe('2000')
    expect(input.step).toBe('1')
  })

  it('commits 2000 on blur instead of clamping it back to an older limit', () => {
    const onChange = vi.fn()
    render(<Harness initial={makeConfig(30)} onConfigChange={onChange} />)

    fireEvent.change(getInput(), { target: { value: '2000' } })
    fireEvent.blur(getInput())

    expect(lastPrimaryCount(onChange)).toBe(2000)
    expect(getInput().value).toBe('2000')
  })

  it('keeps an empty draft while typing, so a following 1500 is not merged into 20', () => {
    const onChange = vi.fn()
    render(<Harness initial={makeConfig(30)} onConfigChange={onChange} />)

    fireEvent.change(getInput(), { target: { value: '' } })
    expect(getInput().value).toBe('')
    fireEvent.change(getInput(), { target: { value: '1500' } })
    expect(getInput().value).toBe('1500')

    fireEvent.blur(getInput())
    expect(lastPrimaryCount(onChange)).toBe(1500)
    expect(getInput().value).toBe('1500')
  })

  it('restores the saved value when an empty draft is blurred', () => {
    const onChange = vi.fn()
    render(<Harness initial={makeConfig(30)} onConfigChange={onChange} />)

    fireEvent.change(getInput(), { target: { value: '' } })
    fireEvent.blur(getInput())

    expect(onChange).not.toHaveBeenCalled()
    expect(getInput().value).toBe('30')
  })

  it('clamps above the ceiling on blur so saved value matches the display', () => {
    const onChange = vi.fn()
    render(<Harness initial={makeConfig(30)} onConfigChange={onChange} />)

    fireEvent.change(getInput(), { target: { value: '5000' } })
    expect(onChange).not.toHaveBeenCalled() // not committed yet

    fireEvent.blur(getInput())
    expect(lastPrimaryCount(onChange)).toBe(2000)
    expect(getInput().value).toBe('2000')
  })

  it('clamps below the floor on blur', () => {
    const onChange = vi.fn()
    render(<Harness initial={makeConfig(30)} onConfigChange={onChange} />)

    fireEvent.change(getInput(), { target: { value: '5' } })
    fireEvent.blur(getInput())
    expect(lastPrimaryCount(onChange)).toBe(10)
  })

  it('clamps negative and zero values to the floor instead of the default', () => {
    const onChange = vi.fn()
    render(<Harness initial={makeConfig(30)} onConfigChange={onChange} />)

    fireEvent.change(getInput(), { target: { value: '-50' } })
    fireEvent.blur(getInput())
    expect(lastPrimaryCount(onChange)).toBe(10)

    fireEvent.change(getInput(), { target: { value: '0' } })
    fireEvent.blur(getInput())
    expect(lastPrimaryCount(onChange)).toBe(10)
  })
})