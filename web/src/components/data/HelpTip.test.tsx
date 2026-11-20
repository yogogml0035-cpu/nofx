import { describe, expect, it } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { HelpTip } from './HelpTip'
import { LanguageProvider } from '../../contexts/LanguageContext'

function renderWithLang(ui: React.ReactElement) {
  return render(<LanguageProvider>{ui}</LanguageProvider>)
}

describe('HelpTip', () => {
  it('默认不显示说明，点击问号展开，再点击收起', () => {
    renderWithLang(
      <HelpTip label="榜单说明">
        <p>这是定义。</p>
        <p>这是信号解读。</p>
      </HelpTip>
    )
    // 初始收起
    expect(screen.queryByText('这是定义。')).toBeNull()
    // 点击问号展开
    fireEvent.click(screen.getByRole('button', { name: '榜单说明' }))
    expect(screen.getByText('这是定义。')).toBeDefined()
    expect(screen.getByText('这是信号解读。')).toBeDefined()
    expect(screen.getByRole('tooltip')).toBeDefined()
    // 再次点击收起
    fireEvent.click(screen.getByRole('button', { name: '榜单说明' }))
    expect(screen.queryByText('这是定义。')).toBeNull()
  })

  it('点击外部区域关闭气泡', () => {
    renderWithLang(
      <div>
        <div data-testid="outside">outside</div>
        <HelpTip label="榜单说明">
          <p>内容</p>
        </HelpTip>
      </div>
    )
    fireEvent.click(screen.getByRole('button', { name: '榜单说明' }))
    expect(screen.getByText('内容')).toBeDefined()
    fireEvent.mouseDown(screen.getByTestId('outside'))
    expect(screen.queryByText('内容')).toBeNull()
  })

  it('展开状态反映到 aria-expanded', () => {
    renderWithLang(
      <HelpTip label="榜单说明">
        <p>内容</p>
      </HelpTip>
    )
    const btn = screen.getByRole('button', { name: '榜单说明' })
    expect(btn.getAttribute('aria-expanded')).toBe('false')
    fireEvent.click(btn)
    expect(btn.getAttribute('aria-expanded')).toBe('true')
  })
})
