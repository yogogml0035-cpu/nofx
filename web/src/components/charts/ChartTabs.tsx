import { useState, useEffect } from 'react'
import { EquityChart } from './EquityChart'
import { AdvancedChart } from './AdvancedChart'
import { useLanguage } from '../../contexts/LanguageContext'
import { t } from '../../i18n/translations'
import { chartTabs, ts } from '../../i18n/strategy-translations'
import { BarChart3, CandlestickChart } from 'lucide-react'
import { motion, AnimatePresence } from 'framer-motion'

interface ChartTabsProps {
  traderId: string
  selectedSymbol?: string // Externally selected symbol
  updateKey?: number // Force update key
  exchangeId?: string // Exchange ID
}

type ChartTab = 'equity' | 'kline'
type Interval = '1m' | '5m' | '15m' | '30m' | '1h' | '4h' | '1d'
type MarketType = 'crypto' | 'stocks' | 'forex' | 'metals'

interface MarketConfigEntry {
  exchange: string
  defaultSymbol: string
  icon: string
  labelKey: keyof typeof chartTabs
  color: string
}

// Market type configuration
const MARKET_CONFIG: Record<MarketType, MarketConfigEntry> = {
  crypto: { exchange: 'gate', defaultSymbol: 'BTCUSDT', icon: '₿', labelKey: 'crypto' as const, color: 'yellow' },
  stocks: { exchange: 'alpaca', defaultSymbol: 'AAPL', icon: '📈', labelKey: 'stocks' as const, color: 'green' },
  forex: { exchange: 'forex', defaultSymbol: 'EUR/USD', icon: '💱', labelKey: 'forex' as const, color: 'blue' },
  metals: { exchange: 'metals', defaultSymbol: 'XAU/USD', icon: '🥇', labelKey: 'metals' as const, color: 'amber' },
}

const INTERVALS: { value: Interval; label: string }[] = [
  { value: '1m', label: '1m' },
  { value: '5m', label: '5m' },
  { value: '15m', label: '15m' },
  { value: '30m', label: '30m' },
  { value: '1h', label: '1h' },
  { value: '4h', label: '4h' },
  { value: '1d', label: '1d' },
]

// Infer market type from exchange ID
function getMarketTypeFromExchange(exchangeId: string | undefined): MarketType {
  // 无交易所上下文时默认 crypto（数据源 gate，D7）
  if (!exchangeId) return 'crypto'
  // Other exchanges default to crypto type
  return 'crypto'
}

export function ChartTabs({ traderId, selectedSymbol, updateKey, exchangeId }: ChartTabsProps) {
  const { language } = useLanguage()
  const [activeTab, setActiveTab] = useState<ChartTab>('equity')
  const [chartSymbol, setChartSymbol] = useState<string>('BTCUSDT')
  const [interval, setInterval] = useState<Interval>('5m')
  const [symbolInput, setSymbolInput] = useState('')
  const [marketType, setMarketType] = useState<MarketType>(() => getMarketTypeFromExchange(exchangeId))

  // Auto-switch market type when exchange ID changes
  useEffect(() => {
    const newMarketType = getMarketTypeFromExchange(exchangeId)
    setMarketType(newMarketType)
  }, [exchangeId])

  // Determine exchange from market type
  const marketConfig = MARKET_CONFIG[marketType]
  // Prefer passed-in exchangeId (crypto uses the trader's exchange when available)
  const currentExchange = marketType === 'crypto' && exchangeId ? exchangeId : marketConfig.exchange

  // Update default symbol when switching market type
  const handleMarketTypeChange = (type: MarketType) => {
    setMarketType(type)
    setChartSymbol(MARKET_CONFIG[type].defaultSymbol)
  }

  // Auto-switch to kline chart when symbol selected externally
  useEffect(() => {
    if (selectedSymbol) {
      setChartSymbol(selectedSymbol)
      setActiveTab('kline')
    }
  }, [selectedSymbol, updateKey])

  // Handle manual symbol input
  const handleSymbolSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (symbolInput.trim()) {
      let symbol = symbolInput.trim().toUpperCase()
      // Auto-append USDT suffix for crypto
      if (marketType === 'crypto' && !symbol.endsWith('USDT')) {
        symbol = symbol + 'USDT'
      }
      setChartSymbol(symbol)
      setSymbolInput('')
    }
  }

  return (
    <div className={`nofx-glass rounded-lg border border-white/5 relative z-10 w-full flex flex-col transition-all duration-300 ${typeof window !== 'undefined' && window.innerWidth < 768 ? 'h-[500px]' : 'h-[600px]'
      }`}>
      {/*
        Premium Professional Toolbar
        Mobile: Single row, horizontal scroll with gradient mask
        Desktop: Standard flex-wrap/nowrap
      */}
      <div
        className="relative z-20 flex flex-wrap md:flex-nowrap items-center justify-between gap-y-2 px-3 py-2 shrink-0 backdrop-blur-md bg-[#0B0E11]/80 rounded-t-lg"
        style={{ borderBottom: '1px solid rgba(255, 255, 255, 0.05)' }}
      >
        {/* Left: Tab Switcher */}
        <div className="flex flex-wrap items-center gap-1">
          <button
            onClick={() => setActiveTab('equity')}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-[11px] font-medium transition-all ${activeTab === 'equity'
              ? 'bg-nofx-gold/10 text-nofx-gold border border-nofx-gold/20 shadow-[0_0_10px_rgba(240,185,11,0.1)]'
              : 'text-nofx-text-muted hover:text-nofx-text-main hover:bg-white/5'
              }`}
          >
            <BarChart3 className="w-3.5 h-3.5" />
            <span className="hidden md:inline">{t('accountEquityCurve', language)}</span>
            <span className="md:hidden">Eq</span>
          </button>

          <button
            onClick={() => setActiveTab('kline')}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-[11px] font-medium transition-all ${activeTab === 'kline'
              ? 'bg-nofx-gold/10 text-nofx-gold border border-nofx-gold/20 shadow-[0_0_10px_rgba(240,185,11,0.1)]'
              : 'text-nofx-text-muted hover:text-nofx-text-main hover:bg-white/5'
              }`}
          >
            <CandlestickChart className="w-3.5 h-3.5" />
            <span className="hidden md:inline">{t('marketChart', language)}</span>
            <span className="md:hidden">Kline</span>
          </button>

          {/* Market Type Pills - Only when kline active, HIDDEN on mobile to save space */}
          {activeTab === 'kline' && (
            <div className="hidden md:flex items-center gap-1 ml-2 border-l border-white/10 pl-2">
              {(Object.keys(MARKET_CONFIG) as MarketType[]).map((type) => {
                const config = MARKET_CONFIG[type]
                const isActive = marketType === type
                return (
                  <button
                    key={type}
                    onClick={() => handleMarketTypeChange(type)}
                    className={`px-2.5 py-1 text-[10px] font-medium rounded transition-all border ${isActive
                      ? 'bg-white/10 text-white border-white/20'
                      : 'text-nofx-text-muted border-transparent hover:text-nofx-text-main hover:bg-white/5'
                      }`}
                  >
                    <span className="mr-1 opacity-70">{config.icon}</span>
                    {ts(chartTabs[config.labelKey], language)}
                  </button>
                )
              })}
            </div>
          )}
        </div>

        {/* Right: Symbol + Interval */}
        {activeTab === 'kline' && (
          <div className="flex items-center gap-2 md:gap-3 w-full md:w-auto min-w-0">
            {/* Symbol Display */}
            <span className="shrink-0 px-2.5 py-1 bg-black/40 border border-white/10 rounded text-[11px] font-bold text-nofx-text-main font-mono">{chartSymbol}</span>

            {/* Interval Selector - Allow scrolling if needed */}
            <div className="flex items-center bg-black/40 rounded border border-white/10 overflow-x-auto no-scrollbar max-w-[200px] md:max-w-none">
              {INTERVALS.map((int) => (
                <button
                  key={int.value}
                  onClick={() => setInterval(int.value)}
                  className={`px-2 py-1 text-[10px] font-medium transition-all ${interval === int.value
                    ? 'bg-nofx-gold/20 text-nofx-gold'
                    : 'text-nofx-text-muted hover:text-white hover:bg-white/5'
                    }`}
                >
                  {int.label}
                </button>
              ))}
            </div>

            {/* Quick Input */}
            <form onSubmit={handleSymbolSubmit} className="hidden md:flex items-center shrink-0">
              <input
                type="text"
                value={symbolInput}
                onChange={(e) => setSymbolInput(e.target.value)}
                placeholder="Sym"
                className="w-16 px-2 py-1 bg-black/40 border border-white/10 rounded-l text-[10px] text-white placeholder-gray-600 focus:outline-none focus:border-nofx-gold/50 font-mono transition-colors"
              />
              <button type="submit" className="px-2 py-1 bg-white/5 border border-white/10 border-l-0 rounded-r text-[10px] text-nofx-text-muted hover:text-white hover:bg-white/10 transition-all">
                Go
              </button>
            </form>
          </div>
        )}
      </div>

      {/* Tab Content - Chart autosizes to this container */}
      <div className="relative flex-1 bg-[#0B0E11]/50 rounded-b-lg overflow-hidden h-full min-h-0">
        <AnimatePresence mode="wait">
          {activeTab === 'equity' ? (
            <motion.div
              key="equity"
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
              transition={{ duration: 0.2 }}
              className="h-full w-full absolute inset-0"
            >
              <EquityChart traderId={traderId} embedded />
            </motion.div>
          ) : (
            <motion.div
              key={`kline-${chartSymbol}-${interval}-${currentExchange}`}
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
              transition={{ duration: 0.2 }}
              className="h-full w-full absolute inset-0"
            >
              <AdvancedChart
                symbol={chartSymbol}
                interval={interval}
                traderID={traderId}
                // Dynamic auto-sizing via ResizeObserver
                exchange={currentExchange}
                onSymbolChange={setChartSymbol}
              />
            </motion.div>
          )}
        </AnimatePresence>
      </div>
    </div>
  )
}
