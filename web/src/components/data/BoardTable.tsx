import { useLanguage } from '../../contexts/LanguageContext'
import { t } from '../../i18n/translations'

interface BoardTableProps {
  title: string
  loading: boolean
  error: string | null
  lastSuccessAt: Date | null
  columns: Array<{ key: string; label: string; align?: 'left' | 'right' }>
  rows: Array<Record<string, string | number>>
  emptyText?: string
}

// 通用榜单表：列配置驱动 + 加载/错误/空态
export function BoardTable({
  title,
  loading,
  error,
  lastSuccessAt,
  columns,
  rows,
  emptyText,
}: BoardTableProps) {
  const { language } = useLanguage()

  return (
    <div className="rounded-2xl border border-white/10 bg-nofx-bg-lighter p-4">
      <div className="mb-3 flex items-center justify-between">
        <h3 className="text-sm font-semibold text-nofx-text">{title}</h3>
        {error && (
          <span className="text-[10px] text-nofx-danger">
            {t('dataPage.fetchFailed', language)}
            {lastSuccessAt && (
              <span className="ml-1 text-nofx-text-muted">
                ({t('dataPage.lastOk', language)}{' '}
                {lastSuccessAt.toLocaleTimeString()})
              </span>
            )}
          </span>
        )}
      </div>

      {loading ? (
        <div className="py-6 text-center text-xs text-nofx-text-muted">
          {t('dataPage.loading', language)}
        </div>
      ) : error ? (
        <div className="py-6 text-center text-xs text-nofx-danger">{error}</div>
      ) : rows.length === 0 ? (
        <div className="py-6 text-center text-xs text-nofx-text-muted">
          {emptyText || t('dataPage.noData', language)}
        </div>
      ) : (
        <table className="w-full text-xs">
          <thead>
            <tr className="text-nofx-text-muted">
              {columns.map((col) => (
                <th
                  key={col.key}
                  className={`pb-2 font-normal ${
                    col.align === 'right' ? 'text-right' : 'text-left'
                  }`}
                >
                  {col.label}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row, i) => (
              <tr
                key={i}
                className="border-t border-white/5 text-nofx-text"
              >
                {columns.map((col) => (
                  <td
                    key={col.key}
                    className={`py-1.5 ${
                      col.align === 'right' ? 'text-right' : 'text-left'
                    }`}
                  >
                    {row[col.key]}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}
