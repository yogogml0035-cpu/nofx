import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
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
})
