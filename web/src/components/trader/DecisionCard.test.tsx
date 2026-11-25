import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { DecisionCard } from './DecisionCard'
import type { DecisionRecord } from '../../types'

const baseDecision: DecisionRecord = {
  timestamp: '2026-11-25T08:00:00Z',
  cycle_number: 7,
  system_prompt: '',
  input_prompt: '',
  cot_trace: '',
  decision_json: '',
  account_state: {
    total_balance: 100,
    available_balance: 90,
    total_unrealized_profit: 0,
    position_count: 1,
    margin_used_pct: 10,
  },
  positions: [],
  candidate_coins: [],
  decisions: [],
  execution_log: [],
  success: true,
}

describe('DecisionCard partial close', () => {
  it('识别分批平仓动作并显示比例，不落入 wait', () => {
    render(
      <DecisionCard
        language="en"
        decision={{
          ...baseDecision,
          decisions: [
            {
              action: 'close_long_partial',
              symbol: 'DOGEUSDT',
              quantity: 80,
              leverage: 0,
              price: 0.05438,
              close_percent: 50,
              order_id: 56013526826240751,
              timestamp: '2026-11-25T08:00:00Z',
              success: true,
            },
          ],
        }}
      />
    )

    expect(screen.getByText('PARTIAL')).toBeDefined()
    expect(screen.getByText('50%')).toBeDefined()
    expect(screen.queryByText('WAIT')).toBeNull()
  })

  it('全平动作不显示比例', () => {
    render(
      <DecisionCard
        language="en"
        decision={{
          ...baseDecision,
          decisions: [
            {
              action: 'close_short',
              symbol: 'DOGEUSDT',
              quantity: 90,
              leverage: 0,
              price: 0.0544,
              order_id: 56013526826240880,
              timestamp: '2026-11-25T08:00:00Z',
              success: true,
            },
          ],
        }}
      />
    )

    expect(screen.getByText('CLOSE')).toBeDefined()
    expect(screen.queryByText('50%')).toBeNull()
  })
})