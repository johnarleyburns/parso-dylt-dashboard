import { useState, useEffect, useCallback } from 'react'
import { RefreshCw, AlertTriangle, Box, LineChart, TableIcon, Shield } from 'lucide-react'
import EnergyCurve3D from './components/EnergyCurve3D'
import PriceChart2D from './components/PriceChart2D'
import PriceTable from './components/PriceTable'
import NewsPanel from './components/NewsPanel'
import NodeHealthGrid from './components/NodeHealthGrid'
import AdminPanel from './components/AdminPanel'
import AdminConsole from './components/AdminConsole'
import type { AllPrices, NewsResponse, AllHealth } from './types'

type ViewMode = '3d' | '2d' | 'table'

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

function useMobile(): boolean {
  const [mobile, setMobile] = useState(() =>
    typeof window !== 'undefined' && window.innerWidth < 768,
  )
  useEffect(() => {
    const mq = window.matchMedia('(max-width: 767px)')
    const handler = (e: MediaQueryListEvent) => setMobile(e.matches)
    mq.addEventListener('change', handler)
    return () => mq.removeEventListener('change', handler)
  }, [])
  return mobile
}

async function apiFetch<T>(path: string): Promise<T> {
  const ctrl = new AbortController()
  const timer = setTimeout(() => ctrl.abort(), 10_000)
  try {
    const resp = await fetch(API_BASE + path, {
      headers: { Accept: 'application/json' },
      signal: ctrl.signal,
    })
    if (!resp.ok) throw new Error(`HTTP ${resp.status} from ${path}`)
    return resp.json() as Promise<T>
  } finally {
    clearTimeout(timer)
  }
}

export default function App() {
  const [prices, setPrices]   = useState<AllPrices>({})
  const [news, setNews]       = useState<NewsResponse>({ eia: [], iea: [] })
  const [health, setHealth]   = useState<AllHealth>({})
  const [lastRefresh, setLastRefresh] = useState<Date | null>(null)
  const [error, setError]     = useState<string | null>(null)
  const [visibleSectors, setVisibleSectors] = useState<Set<string>>(new Set(ALL_SECTORS))
  const [viewMode, setViewMode] = useState<ViewMode>('3d')
  const [showConsole, setShowConsole] = useState(false)

  const mobile = useMobile()

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
        if (next.size > 1) next.delete(sector)
      } else {
        next.add(sector)
      }
      return next
    })
  }

  const viewButtons = (
    [
      { mode: '3d'    as ViewMode, icon: <Box size={12} />,       label: '3D' },
      { mode: '2d'    as ViewMode, icon: <LineChart size={12} />, label: '2D' },
      { mode: 'table' as ViewMode, icon: <TableIcon size={12} />, label: 'Table' },
    ] as const
  ).map(({ mode, icon, label }) => (
    <button
      key={mode}
      onClick={() => setViewMode(mode)}
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: '0.2rem',
        padding: '0.25rem 0.45rem',
        borderRadius: 4,
        border: `1px solid ${viewMode === mode ? '#3b82f6' : '#1e293b'}`,
        background: viewMode === mode ? '#3b82f622' : 'transparent',
        color: viewMode === mode ? '#3b82f6' : '#475569',
        cursor: 'pointer',
        fontSize: '0.65rem',
        fontFamily: 'inherit',
        whiteSpace: 'nowrap',
      }}
    >
      {icon} {label}
    </button>
  ))

  return (
    <div
      style={{
        height: '100dvh',
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
        background: '#0a0e1a',
      }}
    >
      {/* ---- Header ---- */}
      {mobile ? (
        /* Mobile header: two compact rows */
        <header style={{ borderBottom: '1px solid #1e293b', flexShrink: 0 }}>
          {/* Row 1: title + error/time */}
          <div style={{
            padding: '0.5rem 0.75rem',
            display: 'flex',
            alignItems: 'center',
            gap: '0.5rem',
            borderBottom: '1px solid #0f172a',
          }}>
            <h1 style={{ color: '#e2e8f0', fontSize: '0.75rem', fontWeight: 700, letterSpacing: '0.06em', flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              OILFIELD DASHBOARD
            </h1>
            {error && (
              <span style={{ color: '#f59e0b', fontSize: '0.65rem', display: 'flex', alignItems: 'center', gap: '0.2rem', flexShrink: 0 }}>
                <AlertTriangle size={11} />
              </span>
            )}
            {lastRefresh && (
              <span style={{ color: '#475569', fontSize: '0.6rem', display: 'flex', alignItems: 'center', gap: '0.2rem', flexShrink: 0 }}>
                <RefreshCw size={9} />
                {lastRefresh.toLocaleTimeString('en-US', { hour12: false })}
              </span>
            )}
          </div>
          {/* Row 2: node health + view toggles + admin */}
          <div style={{
            padding: '0.4rem 0.75rem',
            display: 'flex',
            alignItems: 'center',
            gap: '0.5rem',
            overflowX: 'auto',
          }}>
            <NodeHealthGrid health={health} />
            <div style={{ marginLeft: 'auto', display: 'flex', gap: '0.35rem', flexShrink: 0 }}>
              {viewButtons}
              <button
                onClick={() => setShowConsole(true)}
                style={{
                  display: 'flex', alignItems: 'center', gap: '0.2rem',
                  padding: '0.25rem 0.45rem', borderRadius: 4,
                  border: '1px solid #00d4ff44', background: '#00d4ff11',
                  color: '#00d4ff', cursor: 'pointer',
                  fontSize: '0.65rem', fontFamily: 'inherit', whiteSpace: 'nowrap',
                }}
              >
                <Shield size={11} /> CTRL
              </button>
              <AdminPanel nodeNames={['n1', 'n2', 'n3']} />
            </div>
          </div>
        </header>
      ) : (
        /* Desktop header: single row */
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
          <h1 style={{ color: '#e2e8f0', fontSize: '0.875rem', fontWeight: 700, letterSpacing: '0.1em', whiteSpace: 'nowrap' }}>
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
            {viewButtons}
            <button
              onClick={() => setShowConsole(true)}
              style={{
                display: 'flex', alignItems: 'center', gap: '0.25rem',
                padding: '0.25rem 0.55rem', borderRadius: 4,
                border: '1px solid #00d4ff44', background: '#00d4ff11',
                color: '#00d4ff', cursor: 'pointer',
                fontSize: '0.7rem', fontFamily: 'inherit', whiteSpace: 'nowrap',
              }}
            >
              <Shield size={12} /> CTRL
            </button>
            <AdminPanel nodeNames={['n1', 'n2', 'n3']} />
          </div>
        </header>
      )}

      {/* ---- Sector toggles ---- */}
      <div
        style={{
          padding: mobile ? '0.3rem 0.75rem' : '0.375rem 1rem',
          borderBottom: '1px solid #1e293b',
          display: 'flex',
          gap: '0.4rem',
          flexShrink: 0,
          overflowX: 'auto',          // scrollable on mobile if sectors overflow
          WebkitOverflowScrolling: 'touch',
        }}
      >
        {ALL_SECTORS.map((s) => {
          const active = visibleSectors.has(s)
          return (
            <button
              key={s}
              onClick={() => toggleSector(s)}
              style={{
                padding: mobile ? '0.15rem 0.45rem' : '0.2rem 0.6rem',
                borderRadius: 4,
                border: `1px solid ${active ? SECTOR_COLORS[s] : '#1e293b'}`,
                background: active ? `${SECTOR_COLORS[s]}22` : 'transparent',
                color: active ? SECTOR_COLORS[s] : '#475569',
                cursor: 'pointer',
                fontSize: mobile ? '0.65rem' : '0.7rem',
                fontFamily: 'inherit',
                fontWeight: active ? 600 : 400,
                transition: 'all 0.15s',
                flexShrink: 0,         // prevent buttons squishing on scroll
                whiteSpace: 'nowrap',
              }}
            >
              {SECTOR_LABELS[s]}
            </button>
          )
        })}
      </div>

      {/* ---- Main content ---- */}
      <div style={{
        flex: 1,
        display: 'flex',
        flexDirection: mobile ? 'column' : 'row',
        overflow: 'hidden',
        minHeight: 0,
      }}>
        {/* Primary chart/table panel */}
        <div style={{
          flex: mobile ? '1 1 0' : '0 0 72%',
          position: 'relative',
          overflow: 'hidden',
          minHeight: 0,
        }}>
          {viewMode === '3d' && (
            <EnergyCurve3D prices={prices} visibleSectors={visibleSectors} />
          )}
          {viewMode === '2d' && (
            <PriceChart2D prices={prices} visibleSectors={visibleSectors} />
          )}
          {viewMode === 'table' && (
            <PriceTable prices={prices} visibleSectors={visibleSectors} />
          )}
        </div>

        {/* News panel — right side on desktop, bottom panel on mobile */}
        <div
          style={{
            flex: mobile ? '0 0 240px' : '0 0 28%',
            borderLeft: mobile ? 'none' : '1px solid #1e293b',
            borderTop: mobile ? '1px solid #1e293b' : 'none',
            display: 'flex',
            flexDirection: 'column',
            overflow: 'hidden',
            minHeight: 0,
          }}
        >
          <div
            style={{
              padding: '0.4rem 0.75rem',
              borderBottom: '1px solid #1e293b',
              color: '#64748b',
              fontSize: '0.65rem',
              fontWeight: 600,
              letterSpacing: '0.08em',
              flexShrink: 0,
            }}
          >
            EIA / IEA NEWS
          </div>
          <div style={{ flex: 1, overflow: 'hidden' }}>
            <NewsPanel news={news} />
          </div>
        </div>
      </div>

      {/* ---- Daylight Control Console overlay ---- */}
      {showConsole && (
        <AdminConsole onClose={() => setShowConsole(false)} mobile={mobile} />
      )}
    </div>
  )
}
