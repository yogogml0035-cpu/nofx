import { useLanguage } from '../../contexts/LanguageContext'
import { t } from '../../i18n/translations'
import type { MarketSymbol } from '../../lib/api/data'
import {
  formatCompactNumber,
  formatFundingRate,
  sortByChangeAsc,
  sortByChangeDesc,
  sortByFundingAsc,
  sortByFundingDesc,
  sortByVolumeDesc,
} from './boardUtils'
import { BoardTable } from './BoardTable'

export type ChangeBoardMode = 'gainers' | 'losers'
export type FundingBoardMode = 'positive' | 'negative'

// ===== 涨跌幅榜（涨/跌切换，按 change_24h_pct 排序，undefined→0）=====
export function ChangeBoard({
  symbols,
  loading,
  error,
  lastSuccessAt,
  mode,
}: {
  symbols?: MarketSymbol[]
  loading: boolean
  error: string | null
  lastSuccessAt: Date | null
  mode: ChangeBoardMode
}) {
  const { language } = useLanguage()

  const sorted = symbols
    ? mode === 'gainers'
      ? sortByChangeDesc(symbols).slice(0, 20)
      : sortByChangeAsc(symbols).slice(0, 20)
    : []

  const rows = sorted.map((s) => ({
    symbol: s.display || s.symbol,
    price: s.mark_price ? s.mark_price.toLocaleString(undefined, { maximumFractionDigits: 6 }) : '—',
    change: s.change_24h_pct === undefined ? '—' : `${s.change_24h_pct >= 0 ? '+' : ''}${s.change_24h_pct.toFixed(2)}%`,
    volume: formatCompactNumber(s.volume_24h),
  }))

  return (
    <BoardTable
      title={
        mode === 'gainers'
          ? t('dataPage.gainersTop20', language)
          : t('dataPage.losersTop20', language)
      }
      loading={loading}
      error={error}
      lastSuccessAt={lastSuccessAt}
      columns={[
        { key: 'symbol', label: t('dataPage.symbol', language) },
        { key: 'price', label: t('dataPage.lastPrice', language), align: 'right' },
        { key: 'change', label: '24h', align: 'right' },
        { key: 'volume', label: t('dataPage.volume', language), align: 'right' },
      ]}
      rows={rows}
      emptyText={t('dataPage.noSymbolData', language)}
    />
  )
}

// ===== 成交额榜 top 20 =====
export function VolumeBoard({
  symbols,
  loading,
  error,
  lastSuccessAt,
}: {
  symbols?: MarketSymbol[]
  loading: boolean
  error: string | null
  lastSuccessAt: Date | null
}) {
  const { language } = useLanguage()

  const sorted = symbols ? sortByVolumeDesc(symbols).slice(0, 20) : []
  const rows = sorted.map((s) => ({
    symbol: s.display || s.symbol,
    price: s.mark_price ? s.mark_price.toLocaleString(undefined, { maximumFractionDigits: 6 }) : '—',
    volume: formatCompactNumber(s.volume_24h),
    change: s.change_24h_pct === undefined ? '—' : `${s.change_24h_pct >= 0 ? '+' : ''}${s.change_24h_pct.toFixed(2)}%`,
  }))

  return (
    <BoardTable
      title={t('dataPage.volumeTop20', language)}
      loading={loading}
      error={error}
      lastSuccessAt={lastSuccessAt}
      columns={[
        { key: 'symbol', label: t('dataPage.symbol', language) },
        { key: 'price', label: t('dataPage.lastPrice', language), align: 'right' },
        { key: 'volume', label: '24h ' + t('dataPage.volume', language), align: 'right' },
        { key: 'change', label: '24h', align: 'right' },
      ]}
      rows={rows}
      emptyText={t('dataPage.noSymbolData', language)}
    />
  )
}

// ===== 资金费率榜 top 20（正/负切换；展示 ×100 保留 4 位小数）=====
export function FundingBoard({
  symbols,
  loading,
  error,
  lastSuccessAt,
  mode,
}: {
  symbols?: MarketSymbol[]
  loading: boolean
  error: string | null
  lastSuccessAt: Date | null
  mode: FundingBoardMode
}) {
  const { language } = useLanguage()

  const filtered = (symbols || []).filter((s) =>
    mode === 'positive' ? (s.funding_rate ?? 0) > 0 : (s.funding_rate ?? 0) < 0
  )
  const sorted =
    mode === 'positive'
      ? sortByFundingDesc(filtered).slice(0, 20)
      : sortByFundingAsc(filtered).slice(0, 20)

  const rows = sorted.map((s) => ({
    symbol: s.display || s.symbol,
    rate: formatFundingRate(s.funding_rate),
    price: s.mark_price ? s.mark_price.toLocaleString(undefined, { maximumFractionDigits: 6 }) : '—',
  }))

  return (
    <BoardTable
      title={
        mode === 'positive'
          ? t('dataPage.fundingPositiveTop20', language)
          : t('dataPage.fundingNegativeTop20', language)
      }
      loading={loading}
      error={error}
      lastSuccessAt={lastSuccessAt}
      columns={[
        { key: 'symbol', label: t('dataPage.symbol', language) },
        { key: 'rate', label: t('dataPage.fundingPer8h', language), align: 'right' },
        { key: 'price', label: t('dataPage.lastPrice', language), align: 'right' },
      ]}
      rows={rows}
      emptyText={t('dataPage.noRateInDirection', language)}
    />
  )
}
