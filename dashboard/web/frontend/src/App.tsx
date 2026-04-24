import { useState, useEffect, useCallback } from 'react'
import { RefreshCw, AlertTriangle } from 'lucide-react'
import EnergyCurve3D from './components/EnergyCurve3D'
import NewsPanel from './components/NewsPanel'
import NodeHealthGrid from './components/NodeHealthGrid'
import AdminPanel from './components/AdminPanel'
import type { AllPrices, NewsResponse, AllHealth } from './types'

const API_BASE = import.meta.env.VITE_API_BASE ?? 'https://ctrl.oilfield.parso.guru'
const PRICE_INTERVAL_MS  = 30_000
const NEWS_INTERVAL_MS   = 300_000
const HEALTH_INTERVAL_MS = 15_000

const ALL_SECTORS = ['crude', 'natgas', 'lng', 'lpg', 'ngls', 'electricity', 'refined']

const SECTOR_LABELS: Record<string, string> = {
  crude:       'Crude',
  natgas:      'Nat Gas',
  lng:         'LNG',
  lpg:         'LPG',
  ngls:        'NGLs',
  electricity: 'Electricity',
  refined:     'Refined',
}

const SECTOR_COLORS: Record<string, string> = {
  crude:       '#3b82f6',
  natgas:      '#f97316',
  lng:         '#f59e0b',
  lpg:         '#22c55e',
  ngls:        '#84cc16',
  electricity: '#a855f7',
  refined:     '#ef4444',
}

async function apiFetch<T>(path: string): Promise<T> {
  const resp = await fetch(API_BASE + path, {
    headers: { Accept: 'application/json' },
  })
  if (!resp.ok) throw new Error(`HTTP ${resp.status} from ${path}`)
  return resp.json() as Promise<T>
}

export default function App() {
  const [prices, setPrices]   = useState<AllPrices>({})
  const [news, setNews]       = useState<NewsResponse>({ eia: [], iea: [] })
  const [health, setHealth]   = useState<AllHealth>({})
  const [lastRefresh, setLastRefresh] = useState<Date | null>(null)
  const [error, setError]     = useState<string | null>(null)
  const [visibleSectors, setVisibleSectors] = useState<Set<string>>(new Set(ALL_SECTORS))

  const fetchPrices = useCallback(async () => {
    try {
      const data = await apiFetch<AllPrices>('/api/v1/prices/all')
      setPrices(data)
      setLastRefresh(new Date())
      setError(null)
    } catch (e) {
      setError(String(e))
    }
  }, [])

  const fetchNews = useCallback(async () => {
    try {
      const data = await apiFetch<NewsResponse>('/api/v1/news')
      setNews(data)
    } catch {
      // news failure is non-fatal; keep existing data
    }
  }, [])

  const fetchHealth = useCallback(async () => {
    try {
      const data = await apiFetch<AllHealth>('/api/v1/health/all')
      setHealth(data)
    } catch {
      // health failure is non-fatal
    }
  }, [])

  useEffect(() => {
    fetchPrices()
    fetchNews()
    fetchHealth()

    const priceTimer  = setInterval(fetchPrices,  PRICE_INTERVAL_MS)
    const newsTimer   = setInterval(fetchNews,    NEWS_INTERVAL_MS)
    const healthTimer = setInterval(fetchHealth,  HEALTH_INTERVAL_MS)

    return () => {
      clearInterval(priceTimer)
      clearInterval(newsTimer)
      clearInterval(healthTimer)
    }
  }, [fetchPrices, fetchNews, fetchHealth])

  function toggleSector(sector: string) {
    setVisibleSectors((prev) => {
      const next = new Set(prev)
      if (next.has(sector)) {
        if (next.size > 1) next.delete(sector)  // always show at least one
      } else {
        next.add(sector)
      }
      return next
    })
  }

  return (
    <div
      style={{
        height: '100vh',
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
        background: '#0a0e1a',
      }}
    >
      {/* ---- Header ---- */}
      <header
        style={{
          padding: '0.625rem 1rem',
          borderBottom: '1px solid #1e293b',
          display: 'flex',
          alignItems: 'center',
          gap: '1rem',
          flexShrink: 0,
        }}
      >
        <h1 style={{ color: '#e2e8f0', fontSize: '0.875rem', fontWeight: 700, letterSpacing: '0.1em' }}>
          OILFIELD — ENERGY MARKET DASHBOARD
        </h1>

        <NodeHealthGrid health={health} />

        <div style={{ marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
          {error && (
            <span style={{ color: '#f59e0b', fontSize: '0.7rem', display: 'flex', alignItems: 'center', gap: '0.25rem' }}>
              <AlertTriangle size={12} /> {error}
            </span>
          )}
          {lastRefresh && (
            <span style={{ color: '#475569', fontSize: '0.7rem', display: 'flex', alignItems: 'center', gap: '0.25rem' }}>
              <RefreshCw size={10} />
              {lastRefresh.toLocaleTimeString('en-US', { hour12: false })} UTC
            </span>
          )}
          <AdminPanel nodeNames={['n1', 'n2', 'n3']} />
        </div>
      </header>

      {/* ---- Sector toggles ---- */}
      <div
        style={{
          padding: '0.375rem 1rem',
          borderBottom: '1px solid #1e293b',
          display: 'flex',
          gap: '0.5rem',
          flexShrink: 0,
        }}
      >
        {ALL_SECTORS.map((s) => {
          const active = visibleSectors.has(s)
          return (
            <button
              key={s}
              onClick={() => toggleSector(s)}
              style={{
                padding: '0.2rem 0.6rem',
                borderRadius: 4,
                border: `1px solid ${active ? SECTOR_COLORS[s] : '#1e293b'}`,
                background: active ? `${SECTOR_COLORS[s]}22` : 'transparent',
                color: active ? SECTOR_COLORS[s] : '#475569',
                cursor: 'pointer',
                fontSize: '0.7rem',
                fontFamily: 'inherit',
                fontWeight: active ? 600 : 400,
                transition: 'all 0.15s',
              }}
            >
              {SECTOR_LABELS[s]}
            </button>
          )
        })}
      </div>

      {/* ---- Main content ---- */}
      <div style={{ flex: 1, display: 'flex', overflow: 'hidden' }}>
        {/* 3D chart — takes 72% of width */}
        <div style={{ flex: '0 0 72%', position: 'relative' }}>
          <EnergyCurve3D prices={prices} visibleSectors={visibleSectors} />
        </div>

        {/* News panel — takes remaining 28% */}
        <div
          style={{
            flex: '0 0 28%',
            borderLeft: '1px solid #1e293b',
            display: 'flex',
            flexDirection: 'column',
            overflow: 'hidden',
          }}
        >
          <div
            style={{
              padding: '0.5rem 0.75rem',
              borderBottom: '1px solid #1e293b',
              color: '#64748b',
              fontSize: '0.7rem',
              fontWeight: 600,
              letterSpacing: '0.08em',
            }}
          >
            EIA / IEA NEWS
          </div>
          <div style={{ flex: 1, overflow: 'hidden' }}>
            <NewsPanel news={news} />
          </div>
        </div>
      </div>
    </div>
  )
}
