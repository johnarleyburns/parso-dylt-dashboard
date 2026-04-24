export interface PricePoint {
  symbol: string
  name: string
  sector: string
  exchange: string
  geography: string
  delivery_month: string
  price: number
  unit: string
  scraped_at: string
  source: string
}

export type AllPrices = Record<string, PricePoint[]>

export interface NewsItem {
  title: string
  url: string
  published_at: string
  source: string
  summary: string
  tags: string[]
}

export interface NewsResponse {
  eia: NewsItem[]
  iea: NewsItem[]
}

export interface NodeHealth {
  node: string
  provider: string
  status: 'ok' | 'degraded' | 'offline'
  etcd_healthy: boolean
}

export type AllHealth = Record<string, NodeHealth>
