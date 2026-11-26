import { useEffect, useState } from 'react'
import { Activity, BarChart2, Clock, Info, Lock, TrendingUp } from 'lucide-react'
import type { IndicatorConfig } from '../../types'

interface IndicatorEditorProps {
  config: IndicatorConfig
  onChange: (config: IndicatorConfig) => void
  disabled?: boolean
  language: string
}

const t = (language: string, zh: string, en: string) => (language === 'zh' ? zh : en)

// Mirror the backend kline limits (store.MinKlineCount / store.MaxKlineCount).
// MaxKlineCount is the Gate candlesticks per-query ceiling: 2000 points.
const PRIMARY_COUNT_MIN = 10
const PRIMARY_COUNT_MAX = 2000
const DEFAULT_PRIMARY_COUNT = 20

const normalizePrimaryCount = (raw: number): number => {
  if (!Number.isFinite(raw)) return DEFAULT_PRIMARY_COUNT
  return Math.min(PRIMARY_COUNT_MAX, Math.max(PRIMARY_COUNT_MIN, Math.round(raw)))
}

const timeframes = [
  { value: '10s', label: '10s', group: 'scalp' },
  { value: '1m', label: '1m', group: 'scalp' },
  { value: '5m', label: '5m', group: 'scalp' },
  { value: '15m', label: '15m', group: 'intraday' },
  { value: '30m', label: '30m', group: 'intraday' },
  { value: '1h', label: '1h', group: 'intraday' },
  { value: '4h', label: '4h', group: 'swing' },
  { value: '8h', label: '8h', group: 'swing' },
  { value: '1d', label: '1D', group: 'position' },
  { value: '7d', label: '7D', group: 'position' },
  { value: '1w', label: '1W', group: 'position' },
  { value: '30d', label: '30D', group: 'position' },
]

const groupLabels: Record<string, string> = {
  scalp: 'Scalp',
  intraday: 'Intraday',
  swing: 'Swing',
  position: 'Position',
}

const indicatorCards = [
  { key: 'enable_ema', label: 'EMA', hint: '20/50', color: '#F0B90B' },
  { key: 'enable_macd', label: 'MACD', hint: 'trend momentum', color: '#a855f7' },
  { key: 'enable_rsi', label: 'RSI', hint: 'overbought/oversold', color: '#F6465D' },
  { key: 'enable_atr', label: 'ATR', hint: 'volatility risk', color: '#60a5fa' },
  { key: 'enable_boll', label: 'BOLL', hint: 'range / breakout', color: '#ec4899' },
] as const

const marketContextCards = [
  { key: 'enable_volume', label: 'Volume', hint: 'from exchange candle volume', color: '#c084fc' },
  { key: 'enable_oi', label: 'Open Interest', hint: 'native exchange context when available', color: '#34d399' },
  { key: 'enable_funding_rate', label: 'Funding', hint: 'perp funding context', color: '#fbbf24' },
] as const

export function IndicatorEditor({ config, onChange, disabled, language }: IndicatorEditorProps) {
  const selectedTimeframes = config.klines.selected_timeframes || [config.klines.primary_timeframe || '5m']
  // 输入中的草稿保留原样（允许空串/多位输入），失焦时才归一，避免清空后继续输入被拼成 20xxxx。
  const [primaryCountDraft, setPrimaryCountDraft] = useState<string | null>(null)

  useEffect(() => {
    setPrimaryCountDraft(null)
  }, [config.klines.primary_count])

  const update = (patch: Partial<IndicatorConfig>) => {
    if (disabled) return
    onChange({
      ...config,
      // Simplified strategy editor works on exchange-native data only.
      ...patch,
      enable_raw_klines: true,
    })
  }

  const commitPrimaryCount = () => {
    const raw = primaryCountDraft
    setPrimaryCountDraft(null)
    if (raw === null) return
    const trimmed = raw.trim()
    // 清空后失焦恢复已保存值，不把空输入静默改成默认值。
    if (trimmed === '') return
    const parsed = Number(trimmed)
    if (!Number.isFinite(parsed)) return
    const next = normalizePrimaryCount(parsed)
    if (next !== config.klines.primary_count) {
      update({ klines: { ...config.klines, primary_count: next } })
    }
  }

  const toggleTimeframe = (tf: string) => {
    if (disabled) return
    const current = [...selectedTimeframes]
    const exists = current.includes(tf)
    if (exists && current.length === 1) return
    const next = exists ? current.filter((item) => item !== tf) : [...current, tf].slice(0, 4)
    const primary = next.includes(config.klines.primary_timeframe) ? config.klines.primary_timeframe : next[0]
    update({
      klines: {
        ...config.klines,
        selected_timeframes: next,
        primary_timeframe: primary,
        enable_multi_timeframe: next.length > 1,
      },
    })
  }

  const setPrimary = (tf: string) => {
    if (disabled || !selectedTimeframes.includes(tf)) return
    update({ klines: { ...config.klines, primary_timeframe: tf } })
  }

  const toggleBool = (key: keyof IndicatorConfig) => {
    update({ [key]: !config[key] } as Partial<IndicatorConfig>)
  }

  return (
    <div className="space-y-5">
      <div className="rounded-2xl border border-sky-400/20 bg-sky-500/5 p-4">
        <div className="flex items-center gap-2 text-nofx-text">
          <BarChart2 className="h-5 w-5 text-sky-300" />
          <h3 className="font-semibold">{t(language, '真实行情输入', 'Real market inputs')}</h3>
        </div>
        <p className="mt-1 text-xs leading-5 text-nofx-text-muted">
          {t(
            language,
            'AI 只喂交易所原生 K 线、成交量、资金费率/持仓等交易所可用数据；不混入外部聚合数据。',
            'AI uses native exchange candles, volume, funding/OI when available. External aggregate datasets are not mixed in.'
          )}
        </p>
      </div>

      <div className="rounded-2xl border border-white/10 bg-nofx-bg-lighter p-4">
        <div className="mb-4 flex items-start justify-between gap-3">
          <div>
            <div className="flex items-center gap-2 text-sm font-semibold text-nofx-text">
              <TrendingUp className="h-4 w-4 text-nofx-gold" />
              {t(language, 'K 线数据', 'Candles')}
              <span className="inline-flex items-center gap-1 rounded-full bg-nofx-gold/15 px-2 py-0.5 text-[10px] text-nofx-gold">
                <Lock className="h-3 w-3" />
                {t(language, '必需', 'Required')}
              </span>
            </div>
            <p className="mt-1 text-xs text-nofx-text-muted">
              {t(language, '来自交易所 K 线数据。最多选择 4 个时间周期。', 'From exchange candle data. Select up to 4 timeframes.')}
            </p>
          </div>
          <div className="flex items-center gap-2 text-xs text-nofx-text-muted">
            {t(language, '根数', 'Bars')}
            <input
              type="number"
              min={PRIMARY_COUNT_MIN}
              max={PRIMARY_COUNT_MAX}
              step={1}
              inputMode="numeric"
              aria-label={t(language, 'K 线根数', 'K-line count')}
              title={t(
                language,
                '10–2000；2000 是 Gate 单次取数上限，实际可用根数受模型上下文限制',
                '10-2000; 2000 is the Gate per-query ceiling; usable bars depend on the model context limit'
              )}
              value={primaryCountDraft ?? String(config.klines.primary_count || DEFAULT_PRIMARY_COUNT)}
              disabled={disabled}
              onChange={(e) => setPrimaryCountDraft(e.target.value)}
              onBlur={commitPrimaryCount}
              className="w-20 rounded-lg border border-white/10 bg-nofx-bg px-2 py-1 text-center text-nofx-text"
            />
          </div>
        </div>

        <div className="space-y-3">
          {['scalp', 'intraday', 'swing', 'position'].map((group) => (
            <div key={group} className="flex items-center gap-3">
              <span className="w-16 text-[11px] text-nofx-text-muted">{groupLabels[group]}</span>
              <div className="flex flex-wrap gap-2">
                {timeframes.filter((tf) => tf.group === group).map((tf) => {
                  const selected = selectedTimeframes.includes(tf.value)
                  const primary = config.klines.primary_timeframe === tf.value
                  return (
                    <button
                      key={tf.value}
                      type="button"
                      disabled={disabled}
                      onClick={() => toggleTimeframe(tf.value)}
                      onDoubleClick={() => setPrimary(tf.value)}
                      className={`rounded-lg border px-3 py-1.5 text-xs transition-all ${
                        selected
                          ? 'border-nofx-gold bg-nofx-gold/10 text-nofx-gold'
                          : 'border-white/10 bg-white/[0.02] text-nofx-text-muted hover:text-white'
                      }`}
                      title={primary ? 'Primary timeframe' : 'Double click selected item to make primary'}
                    >
                      {tf.label}{primary && ' ★'}
                    </button>
                  )
                })}
              </div>
            </div>
          ))}
        </div>
      </div>

      <div className="rounded-2xl border border-white/10 bg-nofx-bg-lighter p-4">
        <div className="mb-3 flex items-center gap-2 text-sm font-semibold text-nofx-text">
          <Activity className="h-4 w-4 text-nofx-success" />
          {t(language, '可选技术指标', 'Optional technical indicators')}
        </div>
        <div className="mb-3 flex items-start gap-2 rounded-xl bg-nofx-success/5 p-3 text-xs text-nofx-text-muted">
          <Info className="mt-0.5 h-3.5 w-3.5 flex-shrink-0 text-nofx-success" />
          {t(language, '默认只给原始 K 线，AI 可以自己计算。需要固定指标时再开启。', 'Raw candles are enough by default; enable fixed indicators only when needed.')}
        </div>
        <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-3">
          {indicatorCards.map(({ key, label, hint, color }) => {
            const enabled = Boolean(config[key])
            return (
              <button
                key={key}
                type="button"
                disabled={disabled}
                onClick={() => toggleBool(key)}
                className={`rounded-xl border p-3 text-left transition-all ${enabled ? 'bg-white/[0.04]' : 'bg-transparent hover:bg-white/[0.03]'}`}
                style={{ borderColor: enabled ? `${color}66` : 'rgba(255,255,255,0.1)' }}
              >
                <div className="flex items-center justify-between text-sm font-medium text-nofx-text">
                  <span>{label}</span>
                  <span className="h-2 w-2 rounded-full" style={{ background: enabled ? color : '#5E6673' }} />
                </div>
                <div className="mt-1 text-[11px] text-nofx-text-muted">{hint}</div>
              </button>
            )
          })}
        </div>
      </div>

      <div className="rounded-2xl border border-white/10 bg-nofx-bg-lighter p-4">
        <div className="mb-3 flex items-center gap-2 text-sm font-semibold text-nofx-text">
          <Clock className="h-4 w-4 text-amber-300" />
          {t(language, '交易所上下文', 'Exchange context')}
        </div>
        <div className="grid gap-2 sm:grid-cols-3">
          {marketContextCards.map(({ key, label, hint, color }) => {
            const enabled = Boolean(config[key])
            return (
              <button
                key={key}
                type="button"
                disabled={disabled}
                onClick={() => toggleBool(key)}
                className={`rounded-xl border p-3 text-left transition-all ${enabled ? 'bg-white/[0.04]' : 'bg-transparent hover:bg-white/[0.03]'}`}
                style={{ borderColor: enabled ? `${color}66` : 'rgba(255,255,255,0.1)' }}
              >
                <div className="text-sm font-medium text-nofx-text">{label}</div>
                <div className="mt-1 text-[11px] text-nofx-text-muted">{hint}</div>
              </button>
            )
          })}
        </div>
      </div>
    </div>
  )
}
