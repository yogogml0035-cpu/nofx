import { describe, expect, it } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { BoardTable } from './BoardTable'
import { LanguageProvider } from '../../contexts/LanguageContext'

function renderWithLang(ui: React.ReactElement) {
  return render(<LanguageProvider>{ui}</LanguageProvider>)
}

describe('BoardTable', () => {
  const columns = [
    { key: 'name', label: 'Name' },
    { key: 'value', label: 'Value', align: 'right' as const },
  ]

  it('renders rows when data is present', () => {
    renderWithLang(
      <BoardTable
        title="Test Board"
        loading={false}
        error={null}
        lastSuccessAt={null}
        columns={columns}
        rows={[
          { name: 'BTC_USDT', value: 123 },
          { name: 'ETH_USDT', value: 456 },
        ]}
      />
    )
    expect(screen.getByText('BTC_USDT')).toBeDefined()
    expect(screen.getByText('ETH_USDT')).toBeDefined()
    expect(screen.getByText('123')).toBeDefined()
  })

  it('renders loading placeholder', () => {
    renderWithLang(
      <BoardTable
        title="Test Board"
        loading={true}
        error={null}
        lastSuccessAt={null}
        columns={columns}
        rows={[]}
      />
    )
    // LanguageProvider 默认语言为 en
    expect(screen.getByText('Loading…')).toBeDefined()
  })

  it('renders error placeholder with last success time', () => {
    const at = new Date('2026-11-19T10:00:00')
    renderWithLang(
      <BoardTable
        title="Test Board"
        loading={false}
        error="fetch failed"
        lastSuccessAt={at}
        columns={columns}
        rows={[]}
      />
    )
    expect(screen.getByText('fetch failed')).toBeDefined()
    expect(screen.getByText(/Data fetch failed/)).toBeDefined()
    expect(screen.getByText(/last ok/)).toBeDefined()
  })

  it('renders empty placeholder when no rows', () => {
    renderWithLang(
      <BoardTable
        title="Test Board"
        loading={false}
        error={null}
        lastSuccessAt={null}
        columns={columns}
        rows={[]}
        emptyText="Nothing here"
      />
    )
    expect(screen.getByText('Nothing here')).toBeDefined()
  })

  it('renders actions slot in card header', () => {
    renderWithLang(
      <BoardTable
        title="Test Board"
        loading={false}
        error={null}
        lastSuccessAt={null}
        columns={columns}
        rows={[]}
        actions={<button type="button">toggle</button>}
      />
    )
    const header = screen.getByText('Test Board').closest('div.mb-3')
    expect(header).not.toBeNull()
    expect(header!.querySelector('button')!.textContent).toBe('toggle')
  })

  it('renders help tooltip in card header when help provided', () => {
    renderWithLang(
      <BoardTable
        title="Test Board"
        loading={false}
        error={null}
        lastSuccessAt={null}
        columns={columns}
        rows={[]}
        help={{ desc: '板块定义', signal: '信号解读' }}
      />
    )
    // 默认收起，点击问号展开两段说明
    expect(screen.queryByText('板块定义')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Test Board' }))
    expect(screen.getByText('板块定义')).toBeDefined()
    expect(screen.getByText('信号解读')).toBeDefined()
  })

  it('renders no help tooltip when help absent', () => {
    renderWithLang(
      <BoardTable
        title="Test Board"
        loading={false}
        error={null}
        lastSuccessAt={null}
        columns={columns}
        rows={[]}
      />
    )
    expect(screen.queryByTestId('help-tip')).toBeNull()
  })
})
