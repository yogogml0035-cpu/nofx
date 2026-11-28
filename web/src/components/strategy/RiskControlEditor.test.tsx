import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { RiskControlEditor } from './RiskControlEditor'

// 风控编辑器的旧数据兜底与服务端一致：position_percent 0/缺失显示默认 10。
describe('RiskControlEditor', () => {
  const renderEditor = (config: Parameters<typeof RiskControlEditor>[0]['config']) => {
    render(<RiskControlEditor config={config} onChange={vi.fn()} language="zh" />)
    const inputs = document.querySelectorAll('input[type="number"]')
    return Array.from(inputs) as HTMLInputElement[]
  }

  it('shows the default 10 for a legacy zero position_percent', () => {
    const inputs = renderEditor({ position_percent: 0 } as never)
    const percent = inputs.find((input) => input.max === '20')
    expect(percent?.value).toBe('10')
  })

  it('keeps a positive position_percent as saved', () => {
    const inputs = renderEditor({ position_percent: 8 } as never)
    const percent = inputs.find((input) => input.max === '20')
    expect(percent?.value).toBe('8')
  })

  it('has no minimum order amount input: the only size floor is the exchange contract size', () => {
    renderEditor({ position_percent: 10 } as never)
    expect(screen.queryByText(/最小开仓金额/)).toBeNull()
  })
})
