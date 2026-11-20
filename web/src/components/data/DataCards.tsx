import type {
  MarketOverview,
  FearGreed,
  NewsItem,
  Sentiment,
  LiquidationsResponse,
} from '../../lib/api/data'
import { formatCompactNumber, formatIsoTime } from './boardUtils'
import { useLanguage } from '../../contexts/LanguageContext'
import { t } from '../../i18n/translations'

// ===== 市场总览卡 =====
export function MarketOverviewCard({
  data,
  loading,
  error,
  lastSuccessAt,
}: {
  data?: MarketOverview
  loading: boolean
  error: string | null
  lastSuccessAt: Date | null
}) {
  const { language } = useLanguage()

  return (
    <div className="rounded-2xl border border-white/10 bg-nofx-bg-lighter p-4">
      <div className="mb-3 flex items-center justify-between">
        <h3 className="text-sm font-mono tracking-widest uppercase text-nofx-text">
          {t('dataPage.marketOverview', language)}
        </h3>
        {error && (
          <span className="text-[10px] font-mono text-nofx-danger">
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
        <div className="py-6 text-center text-xs font-mono text-nofx-text-muted">
          {t('dataPage.loading', language)}
        </div>
      ) : error ? (
        <div className="py-6 text-center text-xs font-mono text-nofx-danger">
          {error}
        </div>
      ) : data ? (
        <div className="grid grid-cols-2 gap-x-4 gap-y-3 text-xs sm:grid-cols-4 xl:grid-cols-8">
          <StatCell
            label={t('dataPage.totalMarketCap', language)}
            value={formatCompactNumber(data.total_market_cap)}
            sub={`${data.market_cap_change_24h >= 0 ? '+' : ''}${data.market_cap_change_24h.toFixed(1)}%`}
            subColor={
              data.market_cap_change_24h >= 0
                ? 'text-nofx-success'
                : 'text-nofx-danger'
            }
          />
          <StatCell
            label={t('dataPage.totalVolume', language)}
            value={formatCompactNumber(data.total_volume_24h)}
          />
          <StatCell
            label="BTC Price"
            value={`$${data.btc_price.toLocaleString(undefined, {
              maximumFractionDigits: 0,
            })}`}
          />
          <StatCell
            label={t('dataPage.activeCoins', language)}
            value={String(data.active_coins_count)}
          />
          <StatCell
            label="BTC Dominance"
            value={`${data.btc_dominance.toFixed(1)}%`}
          />
          <StatCell
            label="ETH Dominance"
            value={`${data.eth_dominance.toFixed(1)}%`}
          />
          <StatCell label="ahr999" value={data.ahr999.toFixed(4)} />
          <StatCell
            label={t('dataPage.altcoinSeason', language)}
            value={String(data.altcoin_season_index)}
          />
        </div>
      ) : null}
    </div>
  )
}

function StatCell({
  label,
  value,
  sub,
  subColor,
}: {
  label: string
  value: string
  sub?: string
  subColor?: string
}) {
  return (
    <div>
      <div className="text-nofx-text-muted">{label}</div>
      <div className="mt-0.5 text-base font-semibold font-mono text-nofx-text">
        {value}
        {sub && (
          <span
            className={`ml-2 text-[11px] font-mono font-normal ${subColor || 'text-nofx-text-muted'}`}
          >
            {sub}
          </span>
        )}
      </div>
    </div>
  )
}

// ===== 恐惧贪婪卡 =====
export function FearGreedCard({
  data,
  loading,
  error,
  lastSuccessAt,
}: {
  data?: FearGreed
  loading: boolean
  error: string | null
  lastSuccessAt: Date | null
}) {
  const { language } = useLanguage()

  return (
    <div className="rounded-2xl border border-white/10 bg-nofx-bg-lighter p-4">
      <div className="mb-3 flex items-center justify-between">
        <h3 className="text-sm font-mono tracking-widest uppercase text-nofx-text">
          {t('dataPage.fearGreed', language)}
        </h3>
        <div className="flex items-center gap-2">
          {error && (
            <span className="text-[10px] font-mono text-nofx-danger">
              {t('dataPage.fetchFailed', language)}
              {lastSuccessAt && (
                <span className="ml-1 text-nofx-text-muted">
                  ({t('dataPage.lastOk', language)}{' '}
                  {lastSuccessAt.toLocaleTimeString()})
                </span>
              )}
            </span>
          )}
          {data && !error && (
            <span className="text-[10px] font-mono text-nofx-text-muted">
              {t('dataPage.updatedAt', language)}{' '}
              {formatIsoTime(data.updated_at)}
            </span>
          )}
        </div>
      </div>

      {loading ? (
        <div className="py-6 text-center text-xs font-mono text-nofx-text-muted">
          {t('dataPage.loading', language)}
        </div>
      ) : error ? (
        <div className="py-6 text-center text-xs font-mono text-nofx-danger">
          {error}
        </div>
      ) : data ? (
        <div>
          <div className="flex h-[72px] items-center gap-4">
            <span className="text-5xl font-bold font-mono text-nofx-gold">
              {Math.round(data.value)}
            </span>
            <div className="space-y-1">
              <div className="text-base text-nofx-text">{data.label}</div>
              <div className="text-xs font-mono text-nofx-text-muted">
                {data.change_24h === null
                  ? t('dataPage.change24hAccumulating', language)
                  : `24h: ${data.change_24h >= 0 ? '+' : ''}${data.change_24h.toFixed(0)}`}
              </div>
            </div>
          </div>
          {/* 指针条：0-100，指示点相对轨道双轴居中 */}
          <div className="relative h-2.5 w-full rounded-full bg-gradient-to-r from-nofx-danger via-nofx-gold to-nofx-success">
            <div
              className="absolute top-1/2 h-5 w-5 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-white bg-nofx-bg"
              style={{ left: `${Math.min(100, Math.max(0, data.value))}%` }}
            />
          </div>
          <div className="mt-2 flex justify-between text-[10px] font-mono text-nofx-text-muted">
            <span>0</span>
            <span>25</span>
            <span>50</span>
            <span>75</span>
            <span>100</span>
          </div>
        </div>
      ) : null}
    </div>
  )
}

// ===== 新闻卡（含公告）=====
export function NewsCard({
  news,
  announcements,
  loading,
  error,
  lastSuccessAt,
}: {
  news?: Array<NewsItem>
  announcements?: Array<NewsItem>
  loading: boolean
  error: string | null
  lastSuccessAt: Date | null
}) {
  const { language } = useLanguage()

  return (
    <div className="rounded-2xl border border-white/10 bg-nofx-bg-lighter p-4">
      <div className="mb-3 flex items-center justify-between">
        <h3 className="text-sm font-mono tracking-widest uppercase text-nofx-text">
          {t('dataPage.newsAnnouncements', language)}
        </h3>
        {error && (
          <span className="text-[10px] font-mono text-nofx-danger">
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
        <div className="py-6 text-center text-xs font-mono text-nofx-text-muted">
          {t('dataPage.loading', language)}
        </div>
      ) : error ? (
        <div className="py-6 text-center text-xs font-mono text-nofx-danger">
          {error}
        </div>
      ) : (
        <div className="grid gap-4 lg:grid-cols-2">
          <NewsList
            heading={t('dataPage.btcNews', language)}
            items={news || []}
          />
          <NewsList
            heading={t('dataPage.marketAnnouncements', language)}
            items={announcements || []}
          />
        </div>
      )}
    </div>
  )
}

function NewsList({
  heading,
  items,
}: {
  heading: string
  items: Array<NewsItem>
}) {
  const { language } = useLanguage()

  if (items.length === 0) {
    return (
      <div>
        <div className="mb-1.5 text-xs font-medium text-nofx-text-muted">
          {heading}
        </div>
        <div className="py-2 text-sm font-mono text-nofx-text-muted">
          {t('dataPage.noItems', language)}
        </div>
      </div>
    )
  }

  return (
    <div>
      <div className="mb-1.5 text-xs font-medium text-nofx-text-muted">
        {heading}
      </div>
      <ul className="space-y-2">
        {items.map((item, i) => (
          <li key={i} className="text-sm leading-6">
            <a
              href={item.url}
              target="_blank"
              rel="noopener noreferrer"
              className="text-nofx-text hover:text-nofx-gold"
            >
              {item.title}
            </a>
            <span className="ml-2 text-[11px] font-mono text-nofx-text-muted">
              {item.source} · {formatUnixTimeSafe(item.published_at)}
              {item.sentiment && (
                <span
                  className={`ml-1 ${
                    item.sentiment === 'pos'
                      ? 'text-nofx-success'
                      : item.sentiment === 'neg'
                        ? 'text-nofx-danger'
                        : 'text-nofx-text-muted'
                  }`}
                >
                  [{item.sentiment}]
                </span>
              )}
            </span>
          </li>
        ))}
      </ul>
    </div>
  )
}

function formatUnixTimeSafe(seconds: number): string {
  if (!seconds) return ''
  return new Date(seconds * 1000).toLocaleString()
}

// ===== 社交情绪卡 =====
export function SentimentCard({
  data,
  loading,
  error,
  lastSuccessAt,
}: {
  data?: Sentiment
  loading: boolean
  error: string | null
  lastSuccessAt: Date | null
}) {
  const { language } = useLanguage()

  return (
    <div className="rounded-2xl border border-white/10 bg-nofx-bg-lighter p-4">
      <div className="mb-3 flex items-center justify-between">
        <h3 className="flex items-center text-sm font-mono tracking-widest uppercase text-nofx-text">
          {t('dataPage.socialSentiment', language)}
          <span className="ml-2 text-[10px] font-mono text-nofx-text-muted">
            {data?.coin || 'BTC'}
          </span>
        </h3>
        {error && (
          <span className="text-[10px] font-mono text-nofx-danger">
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
        <div className="py-6 text-center text-xs font-mono text-nofx-text-muted">
          {t('dataPage.loading', language)}
        </div>
      ) : error ? (
        <div className="py-6 text-center text-xs font-mono text-nofx-danger">
          {error}
        </div>
      ) : data ? (
        <div>
          <div className="flex h-[72px] items-center gap-3 text-xs">
            <span className="text-2xl font-bold font-mono text-nofx-text">
              {data.overall_sentiment.toFixed(2)}
            </span>
            <span className="text-nofx-text-muted">{data.sentiment_label}</span>
            <span className="ml-auto font-mono text-nofx-text-muted">
              {t('dataPage.mentions', language)} {data.mention_count}
            </span>
          </div>
          {/* 正负比例条 */}
          <div className="flex h-2.5 w-full overflow-hidden rounded-full">
            <div
              className="bg-nofx-success"
              style={{ width: `${data.positive_ratio * 100}%` }}
            />
            <div
              className="bg-nofx-text-muted/40"
              style={{ width: `${data.neutral_ratio * 100}%` }}
            />
            <div
              className="bg-nofx-danger"
              style={{ width: `${data.negative_ratio * 100}%` }}
            />
          </div>
          <div className="mt-2 flex justify-between text-[10px] font-mono text-nofx-text-muted">
            <span className="text-nofx-success">
              +{(data.positive_ratio * 100).toFixed(0)}%
            </span>
            <span>{(data.neutral_ratio * 100).toFixed(0)}%</span>
            <span className="text-nofx-danger">
              -{(data.negative_ratio * 100).toFixed(0)}%
            </span>
          </div>
        </div>
      ) : null}
    </div>
  )
}

// ===== 清算热点卡 =====
export function LiquidationCard({
  data,
  loading,
  error,
  lastSuccessAt,
}: {
  data?: LiquidationsResponse
  loading: boolean
  error: string | null
  lastSuccessAt: Date | null
}) {
  const { language } = useLanguage()

  return (
    <div className="rounded-2xl border border-white/10 bg-nofx-bg-lighter p-4">
      <div className="mb-3 flex items-center justify-between">
        <h3 className="text-sm font-mono tracking-widest uppercase text-nofx-text">
          {t('dataPage.liquidationHotspots', language)}
        </h3>
        {error && (
          <span className="text-[10px] font-mono text-nofx-danger">
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
        <div className="py-6 text-center text-xs font-mono text-nofx-text-muted">
          {t('dataPage.loading', language)}
        </div>
      ) : error ? (
        <div className="py-6 text-center text-xs font-mono text-nofx-danger">
          {error}
        </div>
      ) : data ? (
        <div className="space-y-2">
          {data.updated_at && (
            <div className="text-right text-[10px] font-mono text-nofx-text-muted">
              {t('dataPage.updatedAt', language)}{' '}
              {formatIsoTime(data.updated_at)}
            </div>
          )}
          {data.hotspots.length === 0 ? (
            <div className="py-4 text-center text-xs font-mono text-nofx-text-muted">
              {t('dataPage.noLiquidationsInWindow', language)}
            </div>
          ) : (
            <div className="max-h-[420px] space-y-2 overflow-y-auto pr-1">
              {data.hotspots.map((h) => {
                const longPct =
                  h.value > 0 ? (h.long_amount / h.value) * 100 : 0
                return (
                  <div key={h.contract} className="text-xs">
                    <div className="flex items-center justify-between">
                      <span className="font-mono font-medium text-nofx-text">
                        {h.contract}
                      </span>
                      <span className="font-mono text-nofx-text-muted">
                        {formatCompactNumber(h.value)} · {h.count}
                        {t('dataPage.ordersSuffix', language)}
                      </span>
                    </div>
                    <div className="mt-1 flex h-1.5 w-full overflow-hidden rounded-full">
                      <div
                        className="bg-nofx-danger"
                        style={{ width: `${longPct}%` }}
                      />
                      <div
                        className="bg-nofx-success"
                        style={{ width: `${100 - longPct}%` }}
                      />
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </div>
      ) : null}
    </div>
  )
}
