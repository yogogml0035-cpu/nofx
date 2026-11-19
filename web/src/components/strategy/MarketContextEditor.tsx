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
  const toggles: Array<{ key: keyof MarketContextConfig }> = [
    { key: 'overview' },
    { key: 'fear_greed' },
    { key: 'news_sentiment' },
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
          {toggles.map(({ key }) => {
            const checked = !!config[key]
            const labelKey = key === 'fear_greed' ? 'fearGreed' : key === 'news_sentiment' ? 'newsSentiment' : 'overview'
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
                    {t(`marketContext.${labelKey}Label`, language as 'en' | 'zh' | 'id')}
                  </span>
                  <span className="block text-[11px] text-nofx-text-muted">
                    {t(`marketContext.${labelKey}Desc`, language as 'en' | 'zh' | 'id')}
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
