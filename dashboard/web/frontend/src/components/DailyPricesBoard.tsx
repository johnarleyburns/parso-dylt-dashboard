import { useMemo } from 'react'
import type { AllPrices, PricePoint } from '../types'

const DESKTOP_COLUMNS: string[][] = [
  ['crude', 'natgas', 'lng', 'ngls'],
  ['lpg', 'refined', 'coal', 'carbon'],
  ['electricity'],
]

const MOBILE_COLUMNS: string[][] = [
  ['crude', 'natgas', 'lng', 'ngls', 'lpg', 'refined', 'coal', 'carbon', 'electricity'],
]

const SECTOR_LABELS: Record<string, string> = {
  crude:       'Crude Oil',
  natgas:      'Natural Gas',
  lng:         'LNG',
  lpg:         'LPG',
  ngls:        'NGLs',
  electricity: 'Electricity',
  refined:     'Refined Products',
  coal:        'Coal',
  carbon:      'Carbon',
}

const SECTOR_COLORS: Record<string, string> = {
  crude:       '#3b82f6',
  natgas:      '#f97316',
  lng:         '#f59e0b',
  lpg:         '#22c55e',
  ngls:        '#84cc16',
  electricity: '#a855f7',
  refined:     '#ef4444',
  coal:        '#78716c',
  carbon:      '#14b8a6',
}

const STALE_MS = 24 * 60 * 60 * 1000

interface RowData {
  symbol: string
  name: string
  price: number
  unit: string
  source: string
  geography: string
  scrapedAt: string
  deliveryMonth: string
  change: number | null
  changePct: number | null
  prevMonth: string | null
  stale: boolean
}

interface SectorSection {
  sector: string
  rows: RowData[]
  latestScrape: string
}

export function fmtPrice(v: number): string {
  if (v >= 10000) return v.toLocaleString('en-US', { maximumFractionDigits: 0 })
  if (v >= 1000)  return v.toFixed(1)
  if (v >= 100)   return v.toFixed(2)
  if (v >= 10)    return v.toFixed(2)
  return v.toFixed(3)
}

export function fmtChange(v: number): string {
  const abs = Math.abs(v)
  const s = abs >= 1000 ? abs.toFixed(0) : abs >= 100 ? abs.toFixed(1) : abs >= 10 ? abs.toFixed(2) : abs.toFixed(3)
  return (v >= 0 ? '+' : '−') + s
}

export function fmtPct(v: number): string {
  return (v >= 0 ? '+' : '') + v.toFixed(2) + '%'
}

export function isPriceStale(scrapedAt: string, nowMs: number = Date.now()): boolean {
  const d = new Date(scrapedAt)
  if (isNaN(d.getTime())) return true
  return nowMs - d.getTime() > STALE_MS
}

function monthLabel(iso: string): string {
  const d = new Date(iso)
  if (isNaN(d.getTime())) return iso
  return d.toLocaleString('en-US', { month: 'short', year: '2-digit' })
}

interface DailyPricesBoardProps {
  prices: AllPrices
  visibleSectors: Set<string>
  mobile?: boolean
  onRowClick?: (sector: string, symbol: string) => void
}

function SectorCard({ section, onRowClick }: {
  section: SectorSection
  onRowClick?: (sector: string, symbol: string) => void
}) {
  const { sector, rows, latestScrape } = section
  const color = SECTOR_COLORS[sector] ?? '#94a3b8'
  const label = SECTOR_LABELS[sector] ?? sector

  const scrapeLabel = useMemo(() => {
    if (!latestScrape) return ''
    const d = new Date(latestScrape)
    return isNaN(d.getTime()) ? '' :
      d.toLocaleString('en-US', { hour: '2-digit', minute: '2-digit', timeZone: 'UTC', hour12: false }) + ' UTC'
  }, [latestScrape])

  return (
    <div style={{
      background: '#0d1525',
      border: '1px solid #1a2332',
      borderTop: `2px solid ${color}`,
      borderRadius: 6,
      overflow: 'hidden',
    }}>
      {/* Card header */}
      <div style={{
        padding: '0.4rem 0.75rem',
        background: `${color}12`,
        display: 'flex',
        alignItems: 'center',
        gap: '0.5rem',
        borderBottom: '1px solid #1a2332',
      }}>
        <span style={{ color, fontSize: '0.7rem', fontWeight: 700, letterSpacing: '0.08em' }}>
          {label.toUpperCase()}
        </span>
        <span style={{ color: '#475569', fontSize: '0.6rem' }}>{rows.length} series</span>
        {scrapeLabel && (
          <span style={{ marginLeft: 'auto', color: '#475569', fontSize: '0.55rem' }}>{scrapeLabel}</span>
        )}
      </div>

      {/* Column labels */}
      <div style={{
        display: 'grid',
        gridTemplateColumns: '1fr 7rem 5rem',
        padding: '0.2rem 0.75rem',
        borderBottom: '1px solid #111827',
      }}>
        <span style={{ color: '#475569', fontSize: '0.55rem', fontWeight: 600, letterSpacing: '0.06em' }}>NAME</span>
        <span style={{ color: '#475569', fontSize: '0.55rem', fontWeight: 600, textAlign: 'right' }}>PRICE</span>
        <span style={{ color: '#475569', fontSize: '0.55rem', fontWeight: 600, textAlign: 'right' }}>CHG</span>
      </div>

      {/* Price rows */}
      {rows.map((row, i) => {
        const up    = row.change !== null && row.change > 0
        const down  = row.change !== null && row.change < 0
        const chgColor = up ? '#22c55e' : down ? '#ef4444' : '#475569'
        const chgTip   = row.prevMonth
          ? `vs ${monthLabel(row.prevMonth)} (${monthLabel(row.deliveryMonth)})`
          : 'No previous period available'

        const isGlobal = row.geography !== 'NORTH_AMERICA'

        return (
          <div
            key={row.symbol}
            onClick={onRowClick ? () => onRowClick(section.sector, row.symbol) : undefined}
            style={{
              display: 'grid',
              gridTemplateColumns: '1fr 7rem 5rem',
              padding: '0.38rem 0.75rem',
              borderBottom: i < rows.length - 1 ? '1px solid #0f1623' : 'none',
              alignItems: 'center',
              transition: 'background 0.1s',
              cursor: onRowClick ? 'pointer' : 'default',
            }}
            onMouseEnter={(e) => (e.currentTarget.style.background = '#ffffff0a')}
            onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
          >
            {/* Name + meta */}
            <div style={{ minWidth: 0, paddingRight: '0.5rem' }}>
              <div
                style={{ color: '#c0cfe0', fontSize: '0.7rem', fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', lineHeight: 1.3 }}
                title={row.name}
              >
                {isGlobal && <span style={{ marginRight: '0.25rem', opacity: 0.55, fontSize: '0.65rem' }}>🌐</span>}
                {row.name}
              </div>
              <div style={{ color: '#64748b', fontSize: '0.55rem', marginTop: '0.08rem' }}>
                {row.symbol} · {row.source}
              </div>
            </div>

            {/* Price + unit */}
            <div style={{ textAlign: 'right' }}>
              <span style={{
                color: row.stale ? '#475569' : '#f1f5f9',
                fontSize: '0.85rem',
                fontWeight: 700,
                fontFamily: 'ui-monospace, monospace',
                fontVariantNumeric: 'tabular-nums',
                letterSpacing: '-0.01em',
              }}>
                {row.stale ? 'N/A' : row.price > 0 ? fmtPrice(row.price) : '—'}
              </span>
              <div style={{ color: '#64748b', fontSize: '0.55rem', marginTop: '0.05rem' }}>{row.unit}</div>
            </div>

            {/* Change */}
            <div style={{ textAlign: 'right' }} title={chgTip}>
              {!row.stale && row.change !== null ? (
                <>
                  <div style={{
                    color: chgColor,
                    fontSize: '0.7rem',
                    fontFamily: 'ui-monospace, monospace',
                    fontVariantNumeric: 'tabular-nums',
                    fontWeight: 600,
                  }}>
                    {up ? '▲' : down ? '▼' : '—'} {fmtChange(row.change)}
                  </div>
                  {row.changePct !== null && (
                    <div style={{ color: chgColor, fontSize: '0.6rem', opacity: 0.75 }}>
                      {fmtPct(row.changePct)}
                    </div>
                  )}
                </>
              ) : (
                <span style={{ color: '#334155', fontSize: '0.65rem' }}>—</span>
              )}
            </div>
          </div>
        )
      })}
    </div>
  )
}

export default function DailyPricesBoard({ prices, visibleSectors, mobile = false, onRowClick }: DailyPricesBoardProps) {
  const COLUMNS = mobile ? MOBILE_COLUMNS : DESKTOP_COLUMNS

  const sectionsMap = useMemo<Map<string, SectorSection>>(() => {
    const nowMs = Date.now()
    const map = new Map<string, SectorSection>()
    for (const sector of COLUMNS.flat()) {
      if (!visibleSectors.has(sector)) continue
      const pts: PricePoint[] = prices[sector] ?? []
      if (pts.length === 0) continue

      const bySymbol = new Map<string, PricePoint[]>()
      for (const p of pts) {
        const arr = bySymbol.get(p.symbol) ?? []
        arr.push(p)
        bySymbol.set(p.symbol, arr)
      }

      let latestScrape = ''
      const rows: RowData[] = []

      for (const [symbol, symPts] of bySymbol) {
        const sorted = [...symPts].sort((a, b) => b.delivery_month.localeCompare(a.delivery_month))
        const latest = sorted[0]
        const prev   = sorted[1] ?? null
        if (latest.scraped_at > latestScrape) latestScrape = latest.scraped_at
        rows.push({
          symbol,
          name:          latest.name,
          price:         latest.price,
          unit:          latest.unit,
          source:        latest.source,
          geography:     latest.geography,
          scrapedAt:     latest.scraped_at,
          deliveryMonth: latest.delivery_month,
          change:        prev ? +(latest.price - prev.price).toPrecision(6) : null,
          changePct:     prev && prev.price > 0 ? ((latest.price - prev.price) / prev.price) * 100 : null,
          prevMonth:     prev?.delivery_month ?? null,
          stale:         isPriceStale(latest.scraped_at, nowMs),
        })
      }

      // Sort alphabetically by description within each group.
      rows.sort((a, b) => a.name.localeCompare(b.name))

      map.set(sector, { sector, rows, latestScrape })
    }
    return map
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [prices, visibleSectors, mobile])

  const asOf = useMemo(() => {
    let latest = ''
    for (const s of sectionsMap.values()) {
      if (s.latestScrape > latest) latest = s.latestScrape
    }
    if (!latest) return null
    const d = new Date(latest)
    return isNaN(d.getTime()) ? null : d
  }, [sectionsMap])

  if (sectionsMap.size === 0) {
    return (
      <div style={{ height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center', background: '#0a0e1a', color: '#475569', fontSize: '0.8rem' }}>
        No price data available
      </div>
    )
  }

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column', background: '#0a0e1a', color: '#e2e8f0', overflow: 'hidden' }}>

      {/* Board header */}
      <div style={{
        padding: '0.4rem 1rem',
        borderBottom: '1px solid #1e293b',
        display: 'flex',
        alignItems: 'baseline',
        gap: '0.75rem',
        flexShrink: 0,
        background: '#080c17',
      }}>
        <span style={{ color: '#94a3b8', fontSize: '0.65rem', fontWeight: 700, letterSpacing: '0.1em' }}>
          ENERGY PRICES
        </span>
        {asOf && (
          <>
            <span style={{ color: '#1e293b', fontSize: '0.6rem' }}>·</span>
            <span style={{ color: '#475569', fontSize: '0.6rem' }}>
              as of {asOf.toLocaleString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', timeZone: 'UTC', hour12: false })} UTC
            </span>
          </>
        )}
        <span style={{ marginLeft: 'auto', color: '#334155', fontSize: '0.55rem', fontStyle: 'italic' }}>
          Chg vs prev scraped period
        </span>
      </div>

      {/* Column grid: 3 cols desktop, 1 col mobile (full-width, vertically scrollable) */}
      <div style={{ flex: 1, overflow: 'auto', padding: '0.65rem', display: 'flex', gap: '0.65rem', alignItems: 'flex-start' }}>
        {COLUMNS.map((colSectors, colIdx) => (
          <div key={colIdx} style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: '0.65rem', minWidth: 0 }}>
            {colSectors.map((sector) => {
              const section = sectionsMap.get(sector)
              return section ? <SectorCard key={sector} section={section} onRowClick={onRowClick} /> : null
            })}
          </div>
        ))}
      </div>
    </div>
  )
}
