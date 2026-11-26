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

  it('edit mode does not send the initial balance when it is untouched', async () => {
    // 后端把「正值」当作明确给出。没改过时不发送这个字段，
    // 换绑交易所时旧的初始余额才会被重置、加载时以新账户的本金重建
    const { onSave } = await renderModal()

    fireEvent.click(saveButton())

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1))
    const saved = onSave.mock.calls[0][0] as CreateTraderRequest
    expect('initial_balance' in saved).toBe(false)
    expect(saved.name).toBe('demo')
  })

  it('typing the same number again is not a change', async () => {
    const { onSave } = await renderModal()

    const input = screen.getByLabelText('Initial Balance ($)')
    fireEvent.change(input, { target: { value: '107.50' } })
    fireEvent.click(saveButton())

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1))
    const saved = onSave.mock.calls[0][0] as CreateTraderRequest
    expect('initial_balance' in saved).toBe(false)
  })

  it('marks the input as invalid and points it at the error while the value is not usable', async () => {
    await renderModal({ traderData: { ...existingTrader, initial_balance: 0 } })

    const input = screen.getByLabelText(
      'Initial Balance ($)'
    ) as HTMLInputElement
    expect(input.getAttribute('aria-invalid')).toBe('true')
    expect(input.getAttribute('aria-describedby')).toBe(
      'trader-initial-balance-hint trader-initial-balance-error'
    )

    fireEvent.change(input, { target: { value: '25' } })

    expect(input.getAttribute('aria-invalid')).toBe('false')
    expect(input.getAttribute('aria-describedby')).toBe(
      'trader-initial-balance-hint'
    )
    expect(screen.queryByRole('alert')).toBeNull()
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

  it.each([
    ['zh', '初始余额必须大于 0'],
    ['id', 'Saldo awal harus lebih besar dari 0'],
  ])(
    'the invalid-value alert is translated in %s',
    async (language, alertText) => {
      window.localStorage.setItem('language', language)
      await renderModal({
        traderData: { ...existingTrader, initial_balance: 0 },
      })

      expect(screen.getByRole('alert').textContent).toBe(alertText)
    }
  )

  it.each([
    ['zh', '当前本金（不含浮动盈亏）记为初始余额'],
    ['id', 'Prinsipal akun exchange'],
  ])('the create hint is translated in %s', async (language, createHint) => {
    window.localStorage.setItem('language', language)
    await renderModal({ isEditMode: false, traderData: null })

    expect(screen.getByText(new RegExp(createHint))).toBeInTheDocument()
  })
})

describe('TraderConfigModal – scan cadence', () => {
  const strategyWithTimeframe = (timeframe: string) => ({
    id: 'strategy-1',
    name: 'cadence strategy',
    description: '',
    is_active: true,
    is_default: false,
    created_at: '',
    updated_at: '',
    config: {
      strategy_type: 'ai_trading',
      coin_source: { static_coins: ['BTCUSDT'] },
      indicators: {
        klines: {
          primary_timeframe: timeframe,
          primary_count: 30,
          longer_timeframe: '',
          longer_count: 0,
          enable_multi_timeframe: false,
        },
      },
      risk_control: { max_margin_usage: 0.9 },
    },
  })

  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows the effective cadence from the selected strategy primary timeframe', async () => {
    vi.mocked(httpClient.get).mockResolvedValueOnce({
      success: true,
      data: { strategies: [strategyWithTimeframe('1h')] },
    } as never)

    await renderModal()

    // 决策按策略主周期收线后 5 秒触发；扫描间隔只是回退
    const cadence = screen.getByTestId('trader-scan-cadence')
    expect(cadence.textContent).toContain('1h')
    expect(cadence.textContent).toContain('5s after each')
    expect(screen.queryByText(/calls the model about once a minute/)).toBeNull()
  })

  it('warns about model call frequency for a 1m primary timeframe', async () => {
    vi.mocked(httpClient.get).mockResolvedValueOnce({
      success: true,
      data: { strategies: [strategyWithTimeframe('1m')] },
    } as never)

    await renderModal()

    expect(screen.getByTestId('trader-scan-cadence').textContent).toContain('1m')
    expect(screen.getByText(/calls the model about once a minute/)).toBeTruthy()
  })
})

describe('TraderConfigModal – scan cadence validation', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('does not show an effective cadence for a legacy timeframe outside the retained set', async () => {
    vi.mocked(httpClient.get).mockResolvedValueOnce({
      success: true,
      data: {
        strategies: [
          {
            id: 'strategy-1', name: 'legacy', description: '', is_active: true, is_default: false,
            created_at: '', updated_at: '',
            config: {
              strategy_type: 'ai_trading',
              indicators: {
                klines: {
                  primary_timeframe: '3m', primary_count: 30, longer_timeframe: '', longer_count: 0,
                  enable_multi_timeframe: false,
                },
              },
            },
          },
        ],
      },
    } as never)

    await renderModal()

    expect(screen.queryByTestId('trader-scan-cadence')).toBeNull()
    expect(screen.getByText(/Fallback scan interval \(minutes\)/)).toBeTruthy()
  })
})
