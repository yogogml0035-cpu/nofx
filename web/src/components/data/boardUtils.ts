// Gate 行情看板通用工具：排序切片、费率换算、紧凑数字格式化

export interface SymbolLike {
  symbol: string
  display?: string
  name?: string
  volume_24h?: number
  mark_price?: number
  change_24h_pct?: number
  funding_rate?: number
}

// undefined 的 change_24h_pct 按 0 处理（omitempty 字段缺失语义）
export function sortByChangeDesc(symbols: SymbolLike[]): SymbolLike[] {
  return [...symbols].sort(
    (a, b) => (b.change_24h_pct ?? 0) - (a.change_24h_pct ?? 0)
  )
}

export function sortByChangeAsc(symbols: SymbolLike[]): SymbolLike[] {
  return [...symbols].sort(
    (a, b) => (a.change_24h_pct ?? 0) - (b.change_24h_pct ?? 0)
  )
}

export function sortByVolumeDesc(symbols: SymbolLike[]): SymbolLike[] {
  return [...symbols].sort((a, b) => (b.volume_24h ?? 0) - (a.volume_24h ?? 0))
}

// 正榜 = 费率降序（多头付费最深）；负榜 = 升序；排序在 Gate 原始小数上进行
export function sortByFundingDesc(symbols: SymbolLike[]): SymbolLike[] {
  return [...symbols].sort((a, b) => (b.funding_rate ?? 0) - (a.funding_rate ?? 0))
}

export function sortByFundingAsc(symbols: SymbolLike[]): SymbolLike[] {
  return [...symbols].sort((a, b) => (a.funding_rate ?? 0) - (b.funding_rate ?? 0))
}

// Gate 原始小数 → 百分比字符串（0.000389 → "0.0389%"），保留 4 位小数
export function formatFundingRate(raw?: number): string {
  if (raw === undefined || raw === null || Number.isNaN(raw)) return '—'
  return `${(raw * 100).toFixed(4)}%`
}

// 紧凑数字格式化（复用 CoinSourceEditor 风格）
export function formatCompactNumber(value?: number): string {
  if (value === undefined || value === null || Number.isNaN(value)) return '—'
  if (value >= 1_000_000_000) return `$${(value / 1_000_000_000).toFixed(1)}B`
  if (value >= 1_000_000) return `$${(value / 1_000_000).toFixed(1)}M`
  if (value >= 1_000) return `$${(value / 1_000).toFixed(1)}K`
  return `$${value.toFixed(0)}`
}

// Unix 秒 → 本地时区时间字符串
export function formatUnixTime(seconds?: number): string {
  if (!seconds) return '—'
  return new Date(seconds * 1000).toLocaleString()
}

// ISO 字符串 → 本地时区时间字符串
export function formatIsoTime(iso?: string | null): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  return d.toLocaleString()
}
