import type {
  SystemStatus,
  AccountInfo,
  Position,
  DecisionRecord,
  Statistics,
  CompetitionData,
  PositionHistoryResponse,
} from '../../types'
import { API_BASE, httpClient } from './helpers'

export interface MarketSymbol {
  symbol: string
  display?: string
  name: string
  category: 'crypto' | 'stock' | 'forex' | 'commodity' | 'index' | string
  exchange?: string
  volume_24h?: number
  mark_price?: number
  funding_rate?: number
  change_24h_pct?: number
  prev_day_price?: number
  maxLeverage?: number
  sz_decimals?: number
}

export interface SymbolListResponse {
  exchange: string
  symbols: MarketSymbol[]
  count: number
}

// ===== Gate 行情看板（C1 六端点）=====

export interface MarketOverview {
  total_market_cap: number
  market_cap_change_24h: number
  total_volume_24h: number
  btc_dominance: number
  eth_dominance: number
  btc_price: number
  fear_greed_index: number
  fear_greed_label: string
  ahr999: number
  altcoin_season_index: number
  active_coins_count: number
  updated_at: string
}

export interface FearGreed {
  value: number
  label: string
  change_24h: number | null
  first_seen_at: string | null
  updated_at: string
}

export interface NewsItem {
  title: string
  url: string
  source: string
  published_at: number
  sentiment?: string
}

export interface Sentiment {
  coin: string
  overall_sentiment: number
  sentiment_label: string
  mention_count: number
  positive_ratio: number
  negative_ratio: number
  neutral_ratio: number
  top_tweets: Array<{
    author: string
    content: string
    url: string
    created_time: string
    likes_num: number
  }>
}

export interface LiquidationHotspot {
  contract: string
  value: number
  count: number
  long_amount: number
  short_amount: number
}

export interface LiquidationsResponse {
  window_seconds: number
  window_start: string
  hotspots: LiquidationHotspot[]
  count: number
  updated_at?: string
}

export const dataApi = {
  async getSymbols(exchange = 'gate'): Promise<SymbolListResponse> {
    const result = await httpClient.get<SymbolListResponse>(
      `${API_BASE}/symbols?exchange=${encodeURIComponent(exchange)}`
    )
    if (!result.success) throw new Error('Failed to fetch symbol list')
    return result.data || { exchange, symbols: [], count: 0 }
  },

  async getMarketOverview(): Promise<MarketOverview> {
    const result = await httpClient.get<MarketOverview>(
      `${API_BASE}/market/overview`
    )
    if (!result.success || !result.data)
      throw new Error('Failed to fetch market overview')
    return result.data
  },

  async getFearGreed(): Promise<FearGreed> {
    const result = await httpClient.get<FearGreed>(`${API_BASE}/market/fear-greed`)
    if (!result.success || !result.data)
      throw new Error('Failed to fetch fear & greed index')
    return result.data
  },

  async getMarketNews(limit = 20): Promise<{ items: NewsItem[]; count: number }> {
    const result = await httpClient.get<{ items: NewsItem[]; count: number }>(
      `${API_BASE}/market/news?limit=${limit}`
    )
    if (!result.success || !result.data)
      throw new Error('Failed to fetch market news')
    return result.data
  },

  async getAnnouncements(
    limit = 10
  ): Promise<{ items: NewsItem[]; count: number }> {
    const result = await httpClient.get<{ items: NewsItem[]; count: number }>(
      `${API_BASE}/market/announcements?limit=${limit}`
    )
    if (!result.success || !result.data)
      throw new Error('Failed to fetch announcements')
    return result.data
  },

  async getSentiment(coin = 'BTC'): Promise<Sentiment> {
    const result = await httpClient.get<Sentiment>(
      `${API_BASE}/market/sentiment?coin=${encodeURIComponent(coin)}`
    )
    if (!result.success || !result.data)
      throw new Error('Failed to fetch social sentiment')
    return result.data
  },

  async getLiquidations(): Promise<LiquidationsResponse> {
    const result = await httpClient.get<LiquidationsResponse>(
      `${API_BASE}/market/liquidations`
    )
    if (!result.success || !result.data)
      throw new Error('Failed to fetch liquidation hotspots')
    return result.data
  },

  async getStatus(traderId?: string, silent?: boolean): Promise<SystemStatus> {
    const url = traderId
      ? `${API_BASE}/status?trader_id=${traderId}`
      : `${API_BASE}/status`
    const result = await httpClient.request<SystemStatus>(url, { silent })
    if (!result.success) throw new Error('Failed to fetch system status')
    return result.data!
  },

  async getAccount(traderId?: string, silent?: boolean): Promise<AccountInfo> {
    const url = traderId
      ? `${API_BASE}/account?trader_id=${traderId}`
      : `${API_BASE}/account`
    const result = await httpClient.request<AccountInfo>(url, { silent })
    if (!result.success) throw new Error('Failed to fetch account info')
    return result.data!
  },

  async getPositions(
    traderId?: string,
    silent?: boolean
  ): Promise<Position[]> {
    const url = traderId
      ? `${API_BASE}/positions?trader_id=${traderId}`
      : `${API_BASE}/positions`
    const result = await httpClient.request<Position[]>(url, { silent })
    if (!result.success) throw new Error('Failed to fetch positions')
    return result.data!
  },

  async getDecisions(traderId?: string): Promise<DecisionRecord[]> {
    const url = traderId
      ? `${API_BASE}/decisions?trader_id=${traderId}`
      : `${API_BASE}/decisions`
    const result = await httpClient.get<DecisionRecord[]>(url)
    if (!result.success) throw new Error('Failed to fetch decision logs')
    return result.data!
  },

  async getLatestDecisions(
    traderId?: string,
    limit: number = 5,
    silent?: boolean
  ): Promise<DecisionRecord[]> {
    const params = new URLSearchParams()
    if (traderId) {
      params.append('trader_id', traderId)
    }
    params.append('limit', limit.toString())

    const result = await httpClient.request<DecisionRecord[]>(
      `${API_BASE}/decisions/latest?${params}`,
      { silent }
    )
    if (!result.success) throw new Error('Failed to fetch latest decisions')
    return result.data!
  },

  async getStatistics(
    traderId?: string,
    silent?: boolean
  ): Promise<Statistics> {
    const url = traderId
      ? `${API_BASE}/statistics?trader_id=${traderId}`
      : `${API_BASE}/statistics`
    const result = await httpClient.request<Statistics>(url, { silent })
    if (!result.success) throw new Error('Failed to fetch statistics')
    return result.data!
  },

  async getEquityHistory(
    traderId?: string,
    silent?: boolean
  ): Promise<any[]> {
    const url = traderId
      ? `${API_BASE}/equity-history?trader_id=${traderId}`
      : `${API_BASE}/equity-history`
    const result = await httpClient.request<any[]>(url, { silent })
    if (!result.success) throw new Error('Failed to fetch equity history')
    return result.data!
  },

  async getEquityHistoryBatch(traderIds: string[], hours?: number): Promise<any> {
    const result = await httpClient.post<any>(
      `${API_BASE}/equity-history-batch`,
      { trader_ids: traderIds, hours: hours || 0 }
    )
    if (!result.success) throw new Error('Failed to fetch batch equity history')
    return result.data!
  },

  async getTopTraders(): Promise<any[]> {
    const result = await httpClient.get<any[]>(`${API_BASE}/top-traders`)
    if (!result.success) throw new Error('Failed to fetch top traders')
    return result.data!
  },

  async getPublicTraderConfig(traderId: string): Promise<any> {
    const result = await httpClient.get<any>(
      `${API_BASE}/traders/${traderId}/public-config`
    )
    if (!result.success) throw new Error('Failed to fetch public trader config')
    return result.data!
  },

  async getCompetition(): Promise<CompetitionData> {
    const result = await httpClient.get<CompetitionData>(
      `${API_BASE}/competition`
    )
    if (!result.success) throw new Error('Failed to fetch competition data')
    return result.data!
  },

  async getPositionHistory(
    traderId: string,
    limit: number = 100,
    silent?: boolean
  ): Promise<PositionHistoryResponse> {
    const result = await httpClient.request<PositionHistoryResponse>(
      `${API_BASE}/positions/history?trader_id=${traderId}&limit=${limit}`,
      { silent }
    )
    if (!result.success) throw new Error('Failed to fetch position history')
    return result.data!
  },
}
