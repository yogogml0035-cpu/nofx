import { useRef, useState } from 'react'
import useSWR from 'swr'
import { RefreshCw } from 'lucide-react'
import { dataApi } from '../lib/api/data'
import { useLanguage } from '../contexts/LanguageContext'
import { t } from '../i18n/translations'
import {
  ChangeBoard,
  FundingBoard,
  VolumeBoard,
} from '../components/data/Boards'
import {
  FearGreedCard,
  LiquidationCard,
  MarketOverviewCard,
  NewsCard,
  SentimentCard,
} from '../components/data/DataCards'
import type {
  ChangeBoardMode,
  FundingBoardMode,
} from '../components/data/Boards'

const REFRESH_INTERVAL = 30000

// 各模块最近一次成功响应的本地时刻（fetcher 成功回调写入）
interface LastSuccessTimes {
  overview: Date | null
  fearGreed: Date | null
  news: Date | null
  announcements: Date | null
  sentiment: Date | null
  liquidations: Date | null
  symbols: Date | null
}

export function DataPage() {
  const { language } = useLanguage()
  const [changeMode, setChangeMode] = useState<ChangeBoardMode>('gainers')
  const [fundingMode, setFundingMode] = useState<FundingBoardMode>('positive')
  const [refreshKey, setRefreshKey] = useState(0)

  // 上次成功时刻：fetcher 成功返回时写入（SWR 2.2.5 类型未暴露 dataUpdatedAt）
  const lastTimesRef = useRef<LastSuccessTimes>({
    overview: null,
    fearGreed: null,
    news: null,
    announcements: null,
    sentiment: null,
    liquidations: null,
    symbols: null,
  })

  const swrOpts = {
    refreshInterval: REFRESH_INTERVAL,
    shouldRetryOnError: false,
    revalidateOnFocus: false,
    keepPreviousData: true,
  }

  // 包装 fetcher：成功后记录本地时刻并触发一次轻量重渲染以刷新"上次成功"展示
  const [lastTimesTick, setLastTimesTick] = useState(0)
  const trackedFetcher =
    <T,>(key: keyof LastSuccessTimes, fetch: () => Promise<T>) =>
    async () => {
      const result = await fetch()
      lastTimesRef.current[key] = new Date()
      setLastTimesTick((v) => v + 1)
      return result
    }

  const overview = useSWR(
    ['market-overview', refreshKey],
    trackedFetcher('overview', () => dataApi.getMarketOverview()),
    swrOpts
  )
  const fearGreed = useSWR(
    ['fear-greed', refreshKey],
    trackedFetcher('fearGreed', () => dataApi.getFearGreed()),
    swrOpts
  )
  const news = useSWR(
    ['market-news', refreshKey],
    trackedFetcher('news', () => dataApi.getMarketNews(20)),
    swrOpts
  )
  const announcements = useSWR(
    ['announcements', refreshKey],
    trackedFetcher('announcements', () => dataApi.getAnnouncements(10)),
    swrOpts
  )
  const sentiment = useSWR(
    ['sentiment', refreshKey],
    trackedFetcher('sentiment', () => dataApi.getSentiment('BTC')),
    swrOpts
  )
  const liquidations = useSWR(
    ['liquidations', refreshKey],
    trackedFetcher('liquidations', () => dataApi.getLiquidations()),
    swrOpts
  )
  const symbols = useSWR(
    ['symbols', refreshKey],
    trackedFetcher('symbols', () => dataApi.getSymbols('gate')),
    swrOpts
  )

  // lastTimesTick 参与读取以跟踪 ref 变化（fetcher 成功时已触发重渲染）
  void lastTimesTick
  const lastTimes: LastSuccessTimes = lastTimesRef.current

  const manualRefresh = () => setRefreshKey((k) => k + 1)

  const symbolsBoardProps = {
    symbols: symbols.data?.symbols,
    loading: !symbols.data && symbols.isLoading,
    error: symbols.error ? String(symbols.error) : null,
    lastSuccessAt: lastTimes.symbols,
  }

  return (
    <div className="min-h-[calc(100vh-64px)] w-full bg-nofx-bg">
      <div className="mx-auto max-w-7xl px-4 py-6">
        {/* 页头 */}
        <div className="mb-5 flex items-center justify-between">
          <h1 className="text-xl font-bold text-white">
            {t('dataCenter', language)}
          </h1>
          <button
            type="button"
            onClick={manualRefresh}
            className="inline-flex items-center gap-1.5 rounded-lg border border-white/10 bg-white/[0.02] px-3 py-1.5 text-xs font-mono text-nofx-text-muted transition hover:border-nofx-gold/40 hover:text-nofx-gold"
          >
            <RefreshCw className="h-3.5 w-3.5" />
            {t('dataPage.refresh', language)}
          </button>
        </div>

        {/* 市场总览：全宽紧凑指标条 */}
        <div className="mb-4">
          <MarketOverviewCard
            data={overview.data}
            loading={!overview.data && overview.isLoading}
            error={overview.error ? String(overview.error) : null}
            lastSuccessAt={lastTimes.overview}
          />
        </div>

        {/* 双栏：左榜单 / 右市场环境 */}
        <div className="grid gap-4 lg:grid-cols-2">
          {/* 左栏：榜单类（模式切换收进各卡头） */}
          <div className="space-y-4">
            <ChangeBoard
              {...symbolsBoardProps}
              mode={changeMode}
              actions={
                <>
                  <ModeButton
                    active={changeMode === 'gainers'}
                    onClick={() => setChangeMode('gainers')}
                    label={t('dataPage.gainers', language)}
                  />
                  <ModeButton
                    active={changeMode === 'losers'}
                    onClick={() => setChangeMode('losers')}
                    label={t('dataPage.losers', language)}
                  />
                </>
              }
            />

            <VolumeBoard {...symbolsBoardProps} />

            <FundingBoard
              {...symbolsBoardProps}
              mode={fundingMode}
              actions={
                <>
                  <ModeButton
                    active={fundingMode === 'positive'}
                    onClick={() => setFundingMode('positive')}
                    label={t('dataPage.positive', language)}
                  />
                  <ModeButton
                    active={fundingMode === 'negative'}
                    onClick={() => setFundingMode('negative')}
                    label={t('dataPage.negative', language)}
                  />
                </>
              }
            />

            <LiquidationCard
              data={liquidations.data}
              loading={!liquidations.data && liquidations.isLoading}
              error={liquidations.error ? String(liquidations.error) : null}
              lastSuccessAt={lastTimes.liquidations}
            />
          </div>

          {/* 右栏：恐惧贪婪 / 社交情绪 / 新闻公告 */}
          <div className="space-y-4">
            <FearGreedCard
              data={fearGreed.data}
              loading={!fearGreed.data && fearGreed.isLoading}
              error={fearGreed.error ? String(fearGreed.error) : null}
              lastSuccessAt={lastTimes.fearGreed}
            />
            <SentimentCard
              data={sentiment.data}
              loading={!sentiment.data && sentiment.isLoading}
              error={sentiment.error ? String(sentiment.error) : null}
              lastSuccessAt={lastTimes.sentiment}
            />
            <NewsCard
              news={news.data?.items}
              announcements={announcements.data?.items}
              loading={
                (!news.data && news.isLoading) ||
                (!announcements.data && announcements.isLoading)
              }
              error={
                news.error
                  ? String(news.error)
                  : announcements.error
                    ? String(announcements.error)
                    : null
              }
              lastSuccessAt={lastTimes.news}
            />
          </div>
        </div>
      </div>
    </div>
  )
}

// 卡头内的紧凑模式切换按钮
function ModeButton({
  active,
  onClick,
  label,
}: {
  active: boolean
  onClick: () => void
  label: string
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`rounded-md border px-2.5 py-0.5 text-[11px] font-mono transition ${
        active
          ? 'border-nofx-gold/50 bg-nofx-gold/10 text-nofx-gold'
          : 'border-white/10 bg-white/[0.02] text-nofx-text-muted hover:text-nofx-text'
      }`}
    >
      {label}
    </button>
  )
}
