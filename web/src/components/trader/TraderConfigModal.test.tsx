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

// 页面轮询时父组件会重渲染并产生新的选项数组；测试里也模拟这一点，检查表单不会被重置。
const availableModels = [
  { id: 'model-1', name: 'demo', provider: 'openai', enabled: true },
]
const availableExchanges = [
  { id: 'exchange-1', name: 'gate', exchange_type: 'gate', enabled: true },
]

function modalElement(
  onSave: ReturnType<typeof vi.fn>,
  overrides: Partial<React.ComponentProps<typeof TraderConfigModal>> = {}
) {
  return (
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
}

async function renderModal(
  overrides: Partial<React.ComponentProps<typeof TraderConfigModal>> = {}
) {
  const onSave = vi.fn().mockResolvedValue(undefined)
  const view = render(modalElement(onSave, overrides))
  // 等策略列表那次请求落定，状态更新都发生在 act 之内
  await waitFor(() => expect(httpClient.get).toHaveBeenCalledTimes(1))
  await act(async () => {})
  return { onSave, view }
}

function saveButton() {
  return screen.getByRole('button', { name: 'Save Changes' })
}

// 每个用例都从干净队列开始：clearAllMocks 不会清掉未消费的 mockResolvedValueOnce，
// 上一个用例提前失败时会把策略列表泄漏到下一个用例。
beforeEach(() => {
  vi.resetAllMocks()
  vi.mocked(httpClient.get).mockResolvedValue({
    success: true,
    data: { strategies: [] },
  } as never)
})

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
    // 线上真实形状：AI 策略的 indicators/risk_control 在 ai_config 下（GET /api/strategies 返回）
    config: {
      strategy_type: 'ai_trading',
      ai_config: {
        coin_source: { static_coins: ['BTCUSDT'] },
        indicators: {
          klines: {
            primary_timeframe: timeframe,
            primary_count: 30,
            longer_timeframe: '',
            longer_count: 0,
            enable_multi_timeframe: true,
            selected_timeframes: ['5m', '4h', '15m', '1h'],
          },
        },
        risk_control: { max_margin_usage: 0.9 },
      },
    },
  })

  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('uses the scan interval, not the strategy primary timeframe', async () => {
    // 短线策略：primary=4h 是背景周期，实际触发在 5m。节奏必须跟着扫描间隔走。
    vi.mocked(httpClient.get).mockResolvedValueOnce({
      success: true,
      data: { strategies: [strategyWithTimeframe('4h')] },
    } as never)

    await renderModal({
      traderData: { ...existingTrader, scan_interval_minutes: 5 },
    })

    const cadence = screen.getByTestId('trader-scan-cadence')
    expect(cadence.textContent).toContain('5m')
    expect(cadence.textContent).toContain('5s after each')
    expect(cadence.textContent).not.toContain('4h')
  })

  it('updates the effective cadence when the interval changes', async () => {
    await renderModal({
      traderData: { ...existingTrader, scan_interval_minutes: 5 },
    })

    const input = screen.getByLabelText('Scan interval (minutes)')
    fireEvent.change(input, { target: { value: '60' } })
    expect(screen.getByTestId('trader-scan-cadence').textContent).toContain('1h')

    fireEvent.change(input, { target: { value: '7' } })
    expect(screen.getByTestId('trader-scan-cadence').textContent).toContain(
      '7 minutes after each round'
    )
  })

  it('shows the 5-minute cap for grid strategies', async () => {
    vi.mocked(httpClient.get).mockResolvedValueOnce({
      success: true,
      data: {
        strategies: [
          {
            id: 'strategy-1',
            name: 'grid',
            description: '',
            is_active: true,
            is_default: false,
            created_at: '',
            updated_at: '',
            config: {
              strategy_type: 'grid_trading',
              grid_config: { symbol: 'BTCUSDT', grid_count: 10 },
            },
          },
        ],
      },
    } as never)

    await renderModal({
      traderData: { ...existingTrader, scan_interval_minutes: 60 },
    })

    // 网格循环含突破/回撤/日亏检查，实际节奏被后端封顶在 5 分钟
    expect(screen.getByTestId('trader-scan-cadence').textContent).toContain(
      '5m'
    )
    expect(screen.getByText(/capped at 5 minutes/)).toBeInTheDocument()
  })

  it('does not claim candle alignment for an interval without a matching period', async () => {
    await renderModal({
      traderData: { ...existingTrader, scan_interval_minutes: 7 },
    })

    const cadence = screen.getByTestId('trader-scan-cadence')
    expect(cadence.textContent).toContain('7 minutes after each round')
    expect(cadence.textContent).toContain('not aligned')
    expect(screen.getByText(/Scan interval \(minutes\)/)).toBeTruthy()
  })
})

describe('TraderConfigModal – competition visibility', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('keeps the user choice when the parent re-renders with new option arrays', async () => {
    const { onSave, view } = await renderModal()

    expect(screen.getByRole('button', { name: 'Show' })).toHaveAttribute(
      'aria-pressed',
      'true'
    )
    fireEvent.click(screen.getByRole('button', { name: 'Hide' }))
    expect(screen.getByRole('button', { name: 'Hide' })).toHaveAttribute(
      'aria-pressed',
      'true'
    )

    // 模拟页面 5 秒轮询：traderData 内容不变但对象/选项数组都是新引用
    view.rerender(
      modalElement(onSave, {
        traderData: { ...existingTrader },
        availableModels: [...availableModels],
        availableExchanges: [...availableExchanges],
      })
    )

    expect(screen.getByRole('button', { name: 'Hide' })).toHaveAttribute(
      'aria-pressed',
      'true'
    )
    expect(screen.getByRole('button', { name: 'Show' })).toHaveAttribute(
      'aria-pressed',
      'false'
    )
  })

  it('sends the chosen visibility on save', async () => {
    const { onSave } = await renderModal()

    fireEvent.click(screen.getByRole('button', { name: 'Hide' }))
    fireEvent.click(saveButton())

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1))
    const saved = onSave.mock.calls[0][0] as CreateTraderRequest
    expect(saved.show_in_competition).toBe(false)
  })

  it('defaults a legacy config without the flag to shown', async () => {
    const legacy: TraderConfigData = { ...existingTrader }
    delete (legacy as Partial<TraderConfigData>).show_in_competition

    await renderModal({ traderData: legacy })

    expect(screen.getByRole('button', { name: 'Show' })).toHaveAttribute(
      'aria-pressed',
      'true'
    )
  })

  it('reloads the form when the modal switches to another trader', async () => {
    const { onSave, view } = await renderModal()

    fireEvent.click(screen.getByRole('button', { name: 'Hide' }))
    view.rerender(
      modalElement(onSave, {
        traderData: {
          ...existingTrader,
          trader_id: 'trader-2',
          show_in_competition: true,
        },
      })
    )

    expect(screen.getByRole('button', { name: 'Show' })).toHaveAttribute(
      'aria-pressed',
      'true'
    )
  })
})

describe('TraderConfigModal – form resilience', () => {
  it('keeps all other unsaved edits across a poll-style rerender', async () => {
    const { onSave, view } = await renderModal()

    fireEvent.change(screen.getByRole('textbox'), {
      target: { value: 'renamed' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Isolated Margin' }))
    fireEvent.change(screen.getByLabelText('Scan interval (minutes)'), {
      target: { value: '15' },
    })

    view.rerender(
      modalElement(onSave, {
        traderData: { ...existingTrader },
        availableModels: [...availableModels],
        availableExchanges: [...availableExchanges],
      })
    )
    fireEvent.click(saveButton())

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1))
    const saved = onSave.mock.calls[0][0] as CreateTraderRequest
    expect(saved.name).toBe('renamed')
    expect(saved.is_cross_margin).toBe(false)
    expect(saved.scan_interval_minutes).toBe(15)
  })

  it('rounds a decimal interval and normalizes a legacy value on load', async () => {
    await renderModal({
      traderData: { ...existingTrader, scan_interval_minutes: 1 },
    })

    const input = screen.getByLabelText(
      'Scan interval (minutes)'
    ) as HTMLInputElement
    expect(input.value).toBe('3')

    fireEvent.change(input, { target: { value: '7.5' } })
    expect(input.value).toBe('8')
    expect(screen.getByTestId('trader-scan-cadence').textContent).toContain(
      '8 minutes after each round'
    )
  })
})

describe('TraderConfigModal – create-mode strategy race', () => {
  it('keeps an explicit manual choice when the strategy list arrives late', async () => {
    let resolveStrategies: ((value: unknown) => void) | undefined
    const pending = new Promise((resolve) => {
      resolveStrategies = resolve
    })
    vi.mocked(httpClient.get).mockReturnValueOnce(pending as never)

    const { onSave } = await renderModal({
      isEditMode: false,
      traderData: null,
    })

    // 打开策略下拉并明确选择“手动配置”（空值）
    fireEvent.click(
      screen.getByText('-- No Strategy (Manual Configuration) --')
    )
    const options = screen.getAllByText(
      '-- No Strategy (Manual Configuration) --'
    )
    fireEvent.click(options[options.length - 1])

    // 请求此刻才返回，列表里有一个激活策略
    await act(async () => {
      resolveStrategies?.({
        success: true,
        data: {
          strategies: [
            {
              id: 'strategy-active',
              name: 'Active',
              description: '',
              is_active: true,
              is_default: false,
              created_at: '',
              updated_at: '',
              config: { strategy_type: 'ai_trading', ai_config: {} },
            },
          ],
        },
      })
    })

    fireEvent.change(screen.getByRole('textbox'), {
      target: { value: 'manual trader' },
    })
    fireEvent.click(screen.getByRole('button', { name: /Create Trader/i }))

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1))
    const saved = onSave.mock.calls[0][0] as CreateTraderRequest
    expect(saved.strategy_id).toBe('')
  })
})
