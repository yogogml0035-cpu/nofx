import { useLanguage } from '../contexts/LanguageContext'
import { t } from '../i18n/translations'

// 「数据」页占位：旧行情 iframe 已删除，Gate 行情看板由后续任务重建
export function DataPage() {
  const { language } = useLanguage()

  return (
    <div className="w-full min-h-[calc(100vh-64px)] flex items-center justify-center">
      <div className="text-center space-y-2">
        <h1 className="text-2xl font-semibold text-white">
          {t('dataCenter', language)}
        </h1>
        <p className="text-sm text-zinc-400">
          {language === 'zh' ? '数据看板升级中，敬请期待' : 'Dashboard upgrade in progress, stay tuned'}
        </p>
      </div>
    </div>
  )
}
