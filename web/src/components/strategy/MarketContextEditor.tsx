import { t } from '../../i18n/translations'
import type { MarketContextConfig } from '../../types'

interface MarketContextEditorProps {
  config: MarketContextConfig
  onChange: (config: MarketContextConfig) => void
  disabled?: boolean
  language: 'en' | 'zh' | 'id'
}

// 市场环境上下文勾选组：三勾选，零值全关（决策 prompt 注入对应 Gate 数据段）
export function MarketContextEditor({
  config,
  onChange,
  disabled,
  language,
}: MarketContextEditorProps) {
  const toggles: Array<{
    key: keyof MarketContextConfig
    zh: string
    en: string
    zhDesc: string
    enDesc: string
  }> = [
    {
      key: 'overview',
      zh: '市场总览',
      en: 'Market Overview',
      zhDesc: '总市值/成交/BTC 价格与占比/山寨季指数',
      enDesc: 'Total cap, volume, BTC price & dominance, altcoin season',
    },
    {
      key: 'fear_greed',
      zh: '恐惧贪婪指数',
      en: 'Fear & Greed Index',
      zhDesc: '当前值 + 标签 + 24h 派生变化（数据积累期显示 —）',
      enDesc: 'Current value + label + server-derived 24h change',
    },
    {
      key: 'news_sentiment',
      zh: '新闻与社交情绪',
      en: 'News & Social Sentiment',
      zhDesc: 'BTC 新闻标题与整体情绪分',
      enDesc: 'BTC news headlines and overall sentiment score',
    },
  ]

  return (
    <div className="space-y-4">
      <div className="rounded-2xl border border-sky-400/20 bg-sky-500/5 p-4">
        <div className="flex items-start justify-between gap-3">
          <div>
            <h3 className="text-sm font-semibold text-nofx-text">
              {t('marketContext.title', language as 'en' | 'zh' | 'id')}
            </h3>
            <p className="mt-1 text-xs leading-5 text-nofx-text-muted">
              {t('marketContext.desc', language as 'en' | 'zh' | 'id')}
            </p>
          </div>
        </div>

        <div className="mt-3 space-y-2">
          {toggles.map(({ key, zh, en, zhDesc, enDesc }) => {
            const checked = !!config[key]
            return (
              <label
                key={key}
                className={`flex cursor-pointer items-start gap-3 rounded-xl border p-3 transition ${
                  checked
                    ? 'border-sky-400/50 bg-sky-400/[0.08]'
                    : 'border-white/10 bg-white/[0.02] hover:border-sky-400/30'
                } ${disabled ? 'cursor-not-allowed opacity-60' : ''}`}
              >
                <input
                  type="checkbox"
                  checked={checked}
                  disabled={disabled}
                  onChange={(e) => onChange({ ...config, [key]: e.target.checked })}
                  className="mt-0.5 h-4 w-4 accent-sky-400"
                />
                <span>
                  <span className="block text-xs font-medium text-nofx-text">
                    {language === 'zh' ? zh : en}
                  </span>
                  <span className="block text-[11px] text-nofx-text-muted">
                    {language === 'zh' ? zhDesc : enDesc}
                  </span>
                </span>
              </label>
            )
          })}
        </div>
      </div>
    </div>
  )
}
