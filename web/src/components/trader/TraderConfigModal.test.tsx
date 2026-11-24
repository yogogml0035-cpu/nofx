import { beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { TraderConfigModal } from './TraderConfigModal'
import { LanguageProvider } from '../../contexts/LanguageContext'
import { httpClient } from '../../lib/httpClient'
import type { CreateTraderRequest, TraderConfigData } from '../../types'

// 表单打开时会拉取策略列表；这里不让它发真实请求
vi.mock('../../lib/httpClient', () => ({
  httpClient: {
    get: vi.fn().mockResolvedValue({ success: true, data: { strategies: [] } }),
  },
}))

const existingTrader: TraderConfigData = {
  trader_id: 'trader-1',
  trader_name: 'demo',
  ai_model: 'model-1',
  exchange_id: 'exchange-1',
  strategy_id: 'strategy-1',
  is_cross_margin: true,
  show_in_competition: true,
  scan_interval_minutes: 3,
  initial_balance: 107.5,
  is_running: false,
}

// 这两个 props 必须传稳定的引用：表单里同步表单状态的 effect 把它们列为依赖，
// 不传时组件的默认参数每次渲染都是新数组，会无限重渲染（页面上父组件传的是 state）
const availableModels = [
  { id: 'model-1', name: 'demo', provider: 'openai', enabled: true },
]
const availableExchanges = [
  { id: 'exchange-1', name: 'gate', exchange_type: 'gate', enabled: true },
]

async function renderModal(
  overrides: Partial<React.ComponentProps<typeof TraderConfigModal>> = {}
) {
  const onSave = vi.fn().mockResolvedValue(undefined)
  render(
    <LanguageProvider>
      <TraderConfigModal
        isOpen
        onClose={() => {}}
        isEditMode
        traderData={existingTrader}
        availableModels={availableModels}
        availableExchanges={availableExchanges}
        onSave={onSave}
        {...overrides}
      />
    </LanguageProvider>
  )
  // 等策略列表那次请求落定，状态更新都发生在 act 之内
  await waitFor(() => expect(httpClient.get).toHaveBeenCalledTimes(1))
  await act(async () => {})
  return { onSave }
}

function saveButton() {
  return screen.getByRole('button', { name: 'Save Changes' })
}

describe('TraderConfigModal – initial balance', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('edit mode shows the current initial balance and sends it on save', async () => {
    const { onSave } = await renderModal()

    const input = screen.getByLabelText(
      'Initial Balance ($)'
    ) as HTMLInputElement
    expect(input.value).toBe('107.5')

    fireEvent.change(input, { target: { value: '500.5' } })
    fireEvent.click(saveButton())

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1))
    const saved = onSave.mock.calls[0][0] as CreateTraderRequest
    expect(saved.initial_balance).toBe(500.5)
    expect(saved.name).toBe('demo')
    expect(saved.scan_interval_minutes).toBe(3)
  })

  it('edit mode refuses to save a non-positive initial balance', async () => {
    const { onSave } = await renderModal()

    const input = screen.getByLabelText('Initial Balance ($)')
    fireEvent.change(input, { target: { value: '0' } })

    expect(saveButton()).toBeDisabled()
    expect(screen.getByRole('alert').textContent).toBe(
      'Initial balance must be greater than 0'
    )
    fireEvent.click(saveButton())
    expect(onSave).not.toHaveBeenCalled()

    fireEvent.change(input, { target: { value: '-5' } })
    expect(saveButton()).toBeDisabled()

    fireEvent.change(input, { target: { value: '' } })
    expect(saveButton()).toBeDisabled()

    fireEvent.change(input, { target: { value: '12.34' } })
    expect(saveButton()).not.toBeDisabled()
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('edit mode of a trader stored with 0 blocks saving until a value is entered', async () => {
    await renderModal({ traderData: { ...existingTrader, initial_balance: 0 } })

    expect(
      (screen.getByLabelText('Initial Balance ($)') as HTMLInputElement).value
    ).toBe('0')
    expect(saveButton()).toBeDisabled()
  })

  it('create mode has no initial balance input and does not send the field', async () => {
    const { onSave } = await renderModal({
      isEditMode: false,
      traderData: null,
    })

    expect(screen.getByText(/current principal/i)).toBeInTheDocument()
    expect(screen.queryByLabelText('Initial Balance ($)')).toBeNull()
    expect(screen.queryByText(/account equity/i)).toBeNull()

    // 名称是必填项：填上名称后保存
    fireEvent.change(screen.getByRole('textbox'), {
      target: { value: 'new trader' },
    })
    fireEvent.click(screen.getByRole('button', { name: /Create Trader/i }))

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1))
    const saved = onSave.mock.calls[0][0] as CreateTraderRequest
    expect('initial_balance' in saved).toBe(false)
  })

  it('create mode explains the new rule in Chinese', async () => {
    window.localStorage.setItem('language', 'zh')
    await renderModal({ isEditMode: false, traderData: null })

    expect(
      screen.getByText(/当前本金（不含浮动盈亏）记为初始余额/)
    ).toBeInTheDocument()
    expect(screen.queryByText(/账户净值/)).toBeNull()
  })
})
