import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { RiskControlEditor } from './RiskControlEditor'

// 风控编辑器的旧数据兜底与服务端一致：position_percent 0/缺失显示默认 10，最小开仓下限 10。
describe('RiskControlEditor', () => {
  const renderEditor = (config: Parameters<typeof RiskControlEditor>[0]['config']) => {
    render(<RiskControlEditor config={config} onChange={vi.fn()} language="zh" />)
    const inputs = document.querySelectorAll('input[type="number"]')
    return Array.from(inputs) as HTMLInputElement[]
  }

  it('shows the default 10 for a legacy zero position_percent', () => {
    const inputs = renderEditor({ position_percent: 0, min_position_size: 12 } as never)
    const percent = inputs.find((input) => input.max === '20')
    expect(percent?.value).toBe('10')
  })

  it('keeps a positive position_percent as saved', () => {
    const inputs = renderEditor({ position_percent: 8, min_position_size: 12 } as never)
    const percent = inputs.find((input) => input.max === '20')
    expect(percent?.value).toBe('8')
  })

  it('limits the minimum position size input to the server floor', () => {
    const inputs = renderEditor({ position_percent: 10, min_position_size: 12 } as never)
    const minSize = inputs.find((input) => input.min === '10')
    expect(minSize?.value).toBe('12')
    expect(screen.getByText(/默认 10，可调整/)).toBeTruthy()
  })
})
