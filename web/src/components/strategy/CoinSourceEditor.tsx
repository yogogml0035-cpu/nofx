import { useEffect, useMemo, useState } from 'react'
import { Check, Globe2, Search, X } from 'lucide-react'
import type { CoinSourceConfig } from '../../types'

const API_BASE = import.meta.env.VITE_API_BASE || ''

interface CoinSourceEditorProps {
  config: CoinSourceConfig
  onChange: (config: CoinSourceConfig) => void
  disabled?: boolean
  language: string
}

interface MarketSymbol {
  symbol: string
  display?: string
  name?: string
  category?: string
  mark_price?: number
  volume_24h?: number
  change_24h_pct?: number
}

const t = (language: string, zh: string, en: string) => (language === 'zh' ? zh : en)

const SELECTED_MARKET_LIMIT = 10
const CATALOG_DISPLAY_LIMIT = 120

function formatCompactNumber(value?: number) {
  if (!value || Number.isNaN(value)) return '—'
  if (value >= 1_000_000_000) return `$${(value / 1_000_000_000).toFixed(1)}B`
  if (value >= 1_000_000) return `$${(value / 1_000_000).toFixed(1)}M`
  if (value >= 1_000) return `$${(value / 1_000).toFixed(1)}K`
  return `$${value.toFixed(0)}`
}

function displaySymbol(symbol?: MarketSymbol) {
  return symbol?.display || symbol?.symbol || ''
}

export function CoinSourceEditor({ config, onChange, disabled, language }: CoinSourceEditorProps) {
  const [symbols, setSymbols] = useState<MarketSymbol[]>([])
  const [loadingSymbols, setLoadingSymbols] = useState(false)
  const [symbolError, setSymbolError] = useState<string | null>(null)
  const [query, setQuery] = useState('')

  useEffect(() => {
    let cancelled = false
    const loadSymbols = async () => {
      setLoadingSymbols(true)
      setSymbolError(null)
      try {
        const res = await fetch(`${API_BASE}/api/symbols?exchange=gate`)
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        const data = await res.json()
        const rows: MarketSymbol[] = data.symbols || []
        if (!cancelled) setSymbols(rows)
      } catch (err) {
        if (!cancelled) setSymbolError(err instanceof Error ? err.message : 'Failed to load symbols')
      } finally {
        if (!cancelled) setLoadingSymbols(false)
      }
    }
    loadSymbols()
    return () => {
      cancelled = true
    }
  }, [])

  const selectedCoins = config.static_coins || []
  const selectedSet = useMemo(() => new Set(selectedCoins), [selectedCoins])
  const selectedMarketSymbols = useMemo(
    () => selectedCoins.map((coin) => symbols.find((s) => s.symbol === coin) || { symbol: coin, display: coin }),
    [selectedCoins, symbols]
  )

  const filteredSymbols = useMemo(() => {
    const q = query.trim().toLowerCase()
    return symbols
      .filter((symbol) => {
        if (!q) return true
        return [symbol.symbol, symbol.display, symbol.name, symbol.category]
          .filter(Boolean)
          .some((v) => String(v).toLowerCase().includes(q))
      })
      .slice(0, CATALOG_DISPLAY_LIMIT)
  }, [symbols, query])

  const addSymbol = (symbol: MarketSymbol) => {
    if (disabled || selectedSet.has(symbol.symbol) || selectedCoins.length >= SELECTED_MARKET_LIMIT) return
    onChange({
      ...config,
      static_coins: [...selectedCoins, symbol.symbol],
    })
  }

  const removeSymbol = (symbol: string) => {
    if (disabled) return
    onChange({
      ...config,
      static_coins: selectedCoins.filter((coin) => coin !== symbol),
    })
  }

  return (
    <div className="space-y-5">
      <div className="rounded-2xl border border-sky-400/20 bg-gradient-to-br from-sky-500/10 via-nofx-bg-lighter to-nofx-bg p-4">
        <div className="flex items-start justify-between gap-4">
          <div>
            <div className="flex items-center gap-2 text-nofx-text">
              <Globe2 className="w-5 h-5 text-sky-300" />
              <h3 className="font-semibold">
                {t(language, '加密货币标的', 'Crypto markets')}
              </h3>
            </div>
            <p className="mt-1 text-xs text-nofx-text-muted">
              {t(
                language,
                '从 Gate USDT 永续合约清单中选择交易标的；清单按 24h 成交额排序。',
                'Pick trading symbols from the Gate USDT perpetual list, sorted by 24h volume.'
              )}
            </p>
          </div>
          <span className="rounded-full border border-sky-400/30 bg-sky-400/10 px-3 py-1 text-[11px] text-sky-200">
            {symbols.length || '—'} {t(language, '个可视化标的', 'visual markets')}
          </span>
        </div>
      </div>

      <div className="space-y-4">
          <div>
            <div className="mb-2 flex items-center justify-between gap-3 text-sm font-medium text-nofx-text">
              <span>{t(language, '自选标的', 'Selected markets')}</span>
              <span className="text-xs font-normal text-nofx-text-muted">{selectedCoins.length}/{SELECTED_MARKET_LIMIT}</span>
            </div>
            <div className="flex flex-wrap gap-2">
              {selectedMarketSymbols.length > 0 ? selectedMarketSymbols.map((symbol) => (
                <span key={symbol.symbol} className="inline-flex items-center gap-2 rounded-full border border-sky-300/25 bg-sky-400/10 px-3 py-1.5 text-sm text-sky-100">
                  {displaySymbol(symbol)}
                  {!disabled && (
                    <button type="button" onClick={() => removeSymbol(symbol.symbol)} className="text-sky-200 hover:text-white">
                      <X className="w-3.5 h-3.5" />
                    </button>
                  )}
                </span>
              )) : (
                <span className="text-xs text-nofx-text-muted">
                  {t(language, '点击下方标的卡片添加。', 'Click market cards below to add.')}
                </span>
              )}
            </div>
          </div>

          <div className="rounded-2xl border border-white/10 bg-nofx-bg/80 p-3">
            <div className="mb-3 flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
              <div className="relative flex-1">
                <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-nofx-text-muted" />
                <input
                  value={query}
                  disabled={disabled}
                  onChange={(e) => setQuery(e.target.value)}
                  placeholder={t(language, '搜索 BTC / ETH / SOL…', 'Search BTC / ETH / SOL…')}
                  className="w-full rounded-xl border border-white/10 bg-nofx-bg py-2 pl-9 pr-3 text-sm text-nofx-text outline-none focus:border-sky-400/50"
                />
              </div>
            </div>

            {loadingSymbols && <div className="py-8 text-center text-sm text-nofx-text-muted">{t(language, '加载 Gate 标的中…', 'Loading Gate markets…')}</div>}
            {symbolError && <div className="py-6 text-center text-sm text-nofx-danger">{symbolError}</div>}

            {!loadingSymbols && !symbolError && (
              <div className="grid max-h-[420px] gap-2 overflow-y-auto pr-1 sm:grid-cols-2 xl:grid-cols-3">
                {filteredSymbols.map((symbol) => {
                  const selected = selectedSet.has(symbol.symbol)
                  const change = symbol.change_24h_pct
                  return (
                    <button
                      key={symbol.symbol}
                      type="button"
                      disabled={disabled || selected || selectedCoins.length >= SELECTED_MARKET_LIMIT}
                      onClick={() => addSymbol(symbol)}
                      className={`rounded-xl border p-3 text-left transition-all ${
                        selected
                          ? 'border-sky-300/50 bg-sky-400/10'
                          : 'border-white/10 bg-white/[0.02] hover:border-sky-400/40 hover:bg-sky-400/[0.06]'
                      }`}
                    >
                      <div className="flex items-start justify-between gap-2">
                        <div>
                          <div className="text-sm font-semibold text-nofx-text">{displaySymbol(symbol)}</div>
                          <div className="mt-0.5 text-[11px] text-nofx-text-muted">{symbol.name || 'Crypto'}</div>
                        </div>
                        {selected && <Check className="w-4 h-4 text-sky-300" />}
                      </div>
                      <div className="mt-3 flex items-center justify-between text-[11px]">
                        <span className="text-nofx-text-muted">Vol {formatCompactNumber(symbol.volume_24h)}</span>
                        {typeof change === 'number' && (
                          <span className={change >= 0 ? 'text-nofx-success' : 'text-nofx-danger'}>
                            {change >= 0 ? '+' : ''}{change.toFixed(2)}%
                          </span>
                        )}
                      </div>
                    </button>
                  )
                })}
              </div>
            )}
          </div>
        </div>
    </div>
  )
}
