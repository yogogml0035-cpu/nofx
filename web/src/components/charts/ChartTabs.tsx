import { useState, useEffect } from 'react'
import { EquityChart } from './EquityChart'
import { AdvancedChart } from './AdvancedChart'
import { useLanguage } from '../../contexts/LanguageContext'
import { t } from '../../i18n/translations'
import { strategyApi } from '../../lib/api/strategies'
import { BarChart3, CandlestickChart } from 'lucide-react'
import { motion, AnimatePresence } from 'framer-motion'

interface ChartTabsProps {
  traderId: string
  selectedSymbol?: string // Externally selected symbol
  updateKey?: number // Force update key
  exchangeId?: string // Exchange ID
  strategyId?: string // 当前 trader 绑定策略，用于行情标的下拉选项
}

type ChartTab = 'equity' | 'kline'
type Interval = '1m' | '5m' | '15m' | '30m' | '1h' | '4h' | '1d'

const INTERVALS: { value: Interval; label: string }[] = [
  { value: '1m', label: '1m' },
  { value: '5m', label: '5m' },
  { value: '15m', label: '15m' },
  { value: '30m', label: '30m' },
  { value: '1h', label: '1h' },
  { value: '4h', label: '4h' },
  { value: '1d', label: '1d' },
]

export function ChartTabs({
  traderId,
  selectedSymbol,
  updateKey,
  exchangeId,
  strategyId,
}: ChartTabsProps) {
  const { language } = useLanguage()
  const [activeTab, setActiveTab] = useState<ChartTab>('equity')
  const [chartSymbol, setChartSymbol] = useState<string>('BTCUSDT')
  const [interval, setInterval] = useState<Interval>('5m')
  const [symbolInput, setSymbolInput] = useState('')
  // 策略自选标的（下拉选项数据源；无策略或读取失败时回退空，仅保留当前符号）
  const [strategySymbols, setStrategySymbols] = useState<string[]>([])

  // 拉取策略自选标的：ai_config.coin_source.static_coins（legacy 平铺）∪ 网格交易对
  useEffect(() => {
    let cancelled = false
    if (!strategyId) {
      setStrategySymbols([])
      return
    }
    strategyApi
      .getStrategy(strategyId, true)
      .then((strategy) => {
        if (cancelled) return
        const config = strategy.config ?? {}
        const coins =
          config.ai_config?.coin_source?.static_coins ??
          config.coin_source?.static_coins ??
          []
        // 策略标的统一转紧凑形态（BTC_USDT → BTCUSDT），与图表 kline 请求一致
        const symbols = (
          config.grid_config?.symbol ? [config.grid_config.symbol] : []
        )
          .concat(coins)
          .map((s) => s.trim().toUpperCase().replace(/_/g, ''))
          .filter(Boolean)
        setStrategySymbols(Array.from(new Set(symbols)))
      })
      .catch(() => {
        if (!cancelled) setStrategySymbols([])
      })
    return () => {
      cancelled = true
    }
  }, [strategyId])

  // 下拉选项：当前图表符号恒在首位（受控值必须可选），策略标的去重追加
  const symbolOptions = Array.from(new Set([chartSymbol, ...strategySymbols]))

  // Auto-switch to kline chart when symbol selected externally（统一紧凑形态）
  useEffect(() => {
    if (selectedSymbol) {
      setChartSymbol(selectedSymbol.trim().toUpperCase().replace(/_/g, ''))
      setActiveTab('kline')
    }
  }, [selectedSymbol, updateKey])

  // Handle manual symbol input
  const handleSymbolSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const symbol = symbolInput.trim().toUpperCase()
    if (symbol) {
      // 加密合约统一 USDT 计价，缺后缀自动补全
      setChartSymbol(symbol.endsWith('USDT') ? symbol : symbol + 'USDT')
      setSymbolInput('')
    }
  }

  // 加密行情固定使用 trader 交易所，缺省走 gate
  const currentExchange = exchangeId || 'gate'

  return (
    <div
      className={`nofx-glass rounded-lg border border-white/5 relative z-10 w-full flex flex-col transition-all duration-300 ${
        typeof window !== 'undefined' && window.innerWidth < 768
          ? 'h-[500px]'
          : 'h-[600px]'
      }`}
    >
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
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-[11px] font-medium transition-all ${
              activeTab === 'equity'
                ? 'bg-nofx-gold/10 text-nofx-gold border border-nofx-gold/20 shadow-[0_0_10px_rgba(240,185,11,0.1)]'
                : 'text-nofx-text-muted hover:text-nofx-text-main hover:bg-white/5'
            }`}
          >
            <BarChart3 className="w-3.5 h-3.5" />
            <span className="hidden md:inline">
              {t('accountEquityCurve', language)}
            </span>
            <span className="md:hidden">Eq</span>
          </button>

          <button
            onClick={() => setActiveTab('kline')}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-[11px] font-medium transition-all ${
              activeTab === 'kline'
                ? 'bg-nofx-gold/10 text-nofx-gold border border-nofx-gold/20 shadow-[0_0_10px_rgba(240,185,11,0.1)]'
                : 'text-nofx-text-muted hover:text-nofx-text-main hover:bg-white/5'
            }`}
          >
            <CandlestickChart className="w-3.5 h-3.5" />
            <span className="hidden md:inline">
              {t('marketChart', language)}
            </span>
            <span className="md:hidden">Kline</span>
          </button>
        </div>

        {/* Right: Symbol Select + Interval */}
        {activeTab === 'kline' && (
          <div className="flex items-center gap-2 md:gap-3 w-full md:w-auto min-w-0">
            {/* Symbol Selector: 策略自选标的下拉 */}
            <select
              value={chartSymbol}
              onChange={(e) => setChartSymbol(e.target.value)}
              aria-label={t('symbol', language)}
              className="shrink-0 max-w-[140px] px-2 py-1 bg-black/40 border border-white/10 rounded text-[11px] font-bold text-nofx-text-main font-mono focus:outline-none focus:border-nofx-gold/50 cursor-pointer"
            >
              {symbolOptions.map((s) => (
                <option
                  key={s}
                  value={s}
                  className="bg-[#0B0E11] text-nofx-text-main"
                >
                  {s}
                </option>
              ))}
            </select>

            {/* Interval Selector - Allow scrolling if needed */}
            <div className="flex items-center bg-black/40 rounded border border-white/10 overflow-x-auto no-scrollbar max-w-[200px] md:max-w-none">
              {INTERVALS.map((int) => (
                <button
                  key={int.value}
                  onClick={() => setInterval(int.value)}
                  className={`px-2 py-1 text-[10px] font-medium transition-all ${
                    interval === int.value
                      ? 'bg-nofx-gold/20 text-nofx-gold'
                      : 'text-nofx-text-muted hover:text-white hover:bg-white/5'
                  }`}
                >
                  {int.label}
                </button>
              ))}
            </div>

            {/* Quick Input */}
            <form
              onSubmit={handleSymbolSubmit}
              className="hidden md:flex items-center shrink-0"
            >
              <input
                type="text"
                value={symbolInput}
                onChange={(e) => setSymbolInput(e.target.value)}
                placeholder="Sym"
                className="w-16 px-2 py-1 bg-black/40 border border-white/10 rounded-l text-[10px] text-white placeholder-gray-600 focus:outline-none focus:border-nofx-gold/50 font-mono transition-colors"
              />
              <button
                type="submit"
                className="px-2 py-1 bg-white/5 border border-white/10 border-l-0 rounded-r text-[10px] text-nofx-text-muted hover:text-white hover:bg-white/10 transition-all"
              >
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
