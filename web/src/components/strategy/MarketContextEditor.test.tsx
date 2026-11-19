import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { MarketContextEditor } from './MarketContextEditor'

describe('MarketContextEditor', () => {
  it('renders three toggles reflecting config state', () => {
    render(
      <MarketContextEditor
        config={{ overview: true }}
        onChange={() => {}}
        language="en"
      />
    )
    const overview = screen.getByLabelText(/Market Overview/) as HTMLInputElement
    const fearGreed = screen.getByLabelText(/Fear & Greed/) as HTMLInputElement
    const news = screen.getByLabelText(/News & Social Sentiment/) as HTMLInputElement
    expect(overview.checked).toBe(true)
    expect(fearGreed.checked).toBe(false)
    expect(news.checked).toBe(false)
  })

  it('emits controlled updates on toggle', async () => {
    const onChange = vi.fn()
    render(
      <MarketContextEditor
        config={{}}
        onChange={onChange}
        language="en"
      />
    )
    fireEvent.click(screen.getByLabelText(/fear/i))
    expect(onChange).toHaveBeenCalledTimes(1)
    expect(onChange).toHaveBeenCalledWith({ fear_greed: true })
  })

  it('preserves other keys when toggling a different one', () => {
    const onChange = vi.fn()
    render(
      <MarketContextEditor
        config={{ overview: true, fear_greed: true }}
        onChange={onChange}
        language="en"
      />
    )
    fireEvent.click(screen.getByLabelText(/News & Social Sentiment/))
    expect(onChange).toHaveBeenCalledWith({
      overview: true,
      fear_greed: true,
      news_sentiment: true,
    })
  })

  it('disables inputs when disabled prop set', () => {
    render(
      <MarketContextEditor
        config={{}}
        onChange={() => {}}
        disabled={true}
        language="en"
      />
    )
    const boxes = screen.getAllByRole('checkbox') as HTMLInputElement[]
    for (const box of boxes) {
      expect(box.disabled).toBe(true)
    }
  })
})
