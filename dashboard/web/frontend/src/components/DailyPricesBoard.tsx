import { useMemo } from 'react'
import type { AllPrices, PricePoint } from '../types'

const SECTOR_ORDER = ['crude', 'natgas', 'lng', 'lpg', 'ngls', 'electricity', 'refined', 'coal', 'carbon']

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
}

interface SectorSection {
  sector: string
  rows: RowData[]
  latestScrape: string
}

function fmtPrice(v: number): string {
  if (v >= 10000) return v.toLocaleString('en-US', { maximumFractionDigits: 0 })
  if (v >= 1000)  return v.toFixed(1)
  if (v >= 100)   return v.toFixed(2)
  if (v >= 10)    return v.toFixed(2)
  return v.toFixed(3)
}

function fmtChange(v: number): string {
  const abs = Math.abs(v)
  const s = abs >= 1000 ? abs.toFixed(0) : abs >= 100 ? abs.toFixed(1) : abs >= 10 ? abs.toFixed(2) : abs.toFixed(3)
  return (v >= 0 ? '+' : '−') + s
}

function fmtPct(v: number): string {
  return (v >= 0 ? '+' : '') + v.toFixed(2) + '%'
}

function monthLabel(iso: string): string {
  const d = new Date(iso)
  if (isNaN(d.getTime())) return iso
  return d.toLocaleString('en-US', { month: 'short', year: '2-digit' })
}

function geoFlag(geo: string): string {
  const map: Record<string, string> = {
    'NORTH_AMERICA': '🌎', 'EUROPE': '🌍', 'ASIA': '🌏',
    'OCEANIA': '🌏', 'GLOBAL': '🌐', 'MIDDLE_EAST': '🌍',
  }
  return map[geo] ?? ''
}

interface DailyPricesBoardProps {
  prices: AllPrices
  visibleSectors: Set<string>
}

export default function DailyPricesBoard({ prices, visibleSectors }: DailyPricesBoardProps) {
  const sections = useMemo<SectorSection[]>(() => {
    return SECTOR_ORDER.flatMap((sector) => {
      if (!visibleSectors.has(sector)) return []
      const pts: PricePoint[] = prices[sector] ?? []
      if (pts.length === 0) return []

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
        })
      }

      // Sort by geography then name for consistency
      rows.sort((a, b) => a.geography.localeCompare(b.geography) || a.name.localeCompare(b.name))

      return [{ sector, rows, latestScrape }]
    })
  }, [prices, visibleSectors])

  // Latest scraped_at across all visible sectors for the header
  const asOf = useMemo(() => {
    let latest = ''
    for (const s of sections) {
      if (s.latestScrape > latest) latest = s.latestScrape
    }
    if (!latest) return null
    const d = new Date(latest)
    return isNaN(d.getTime()) ? null : d
  }, [sections])

  if (sections.length === 0) {
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
        padding: '0.45rem 1rem',
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
            <span style={{ color: '#334155', fontSize: '0.6rem' }}>·</span>
            <span style={{ color: '#475569', fontSize: '0.6rem' }}>
              as of {asOf.toLocaleString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', timeZone: 'UTC', hour12: false })} UTC
            </span>
          </>
        )}
        <span style={{ marginLeft: 'auto', color: '#334155', fontSize: '0.55rem', fontStyle: 'italic' }}>
          Chg vs prev scraped period
        </span>
      </div>

      {/* Scrollable grid of sector cards */}
      <div style={{
        flex: 1,
        overflow: 'auto',
        padding: '0.75rem',
        display: 'grid',
        gridTemplateColumns: 'repeat(auto-fill, minmax(340px, 1fr))',
        gap: '0.75rem',
        alignContent: 'start',
      }}>
        {sections.map(({ sector, rows, latestScrape }) => {
          const color = SECTOR_COLORS[sector] ?? '#94a3b8'
          const label = SECTOR_LABELS[sector] ?? sector
          const scrapeTime = latestScrape ? new Date(latestScrape) : null
          const scrapeLabel = scrapeTime && !isNaN(scrapeTime.getTime())
            ? scrapeTime.toLocaleString('en-US', { hour: '2-digit', minute: '2-digit', timeZone: 'UTC', hour12: false }) + ' UTC'
            : ''

          return (
            <div
              key={sector}
              style={{
                background: '#0d1525',
                border: '1px solid #1a2332',
                borderTop: `2px solid ${color}`,
                borderRadius: 6,
                overflow: 'hidden',
              }}
            >
              {/* Card header */}
              <div style={{
                padding: '0.45rem 0.75rem',
                background: `${color}12`,
                display: 'flex',
                alignItems: 'center',
                gap: '0.5rem',
                borderBottom: '1px solid #1a2332',
              }}>
                <span style={{ color, fontSize: '0.7rem', fontWeight: 700, letterSpacing: '0.08em' }}>
                  {label.toUpperCase()}
                </span>
                <span style={{ color: '#334155', fontSize: '0.6rem' }}>{rows.length} series</span>
                {scrapeLabel && (
                  <span style={{ marginLeft: 'auto', color: '#334155', fontSize: '0.55rem' }}>{scrapeLabel}</span>
                )}
              </div>

              {/* Column header */}
              <div style={{
                display: 'grid',
                gridTemplateColumns: '1fr 7rem 5rem',
                padding: '0.25rem 0.75rem',
                borderBottom: '1px solid #111827',
              }}>
                <span style={{ color: '#334155', fontSize: '0.55rem', fontWeight: 600, letterSpacing: '0.06em' }}>NAME</span>
                <span style={{ color: '#334155', fontSize: '0.55rem', fontWeight: 600, textAlign: 'right' }}>PRICE</span>
                <span style={{ color: '#334155', fontSize: '0.55rem', fontWeight: 600, textAlign: 'right' }}>CHG</span>
              </div>

              {/* Price rows */}
              {rows.map((row, i) => {
                const up   = row.change !== null && row.change > 0
                const down = row.change !== null && row.change < 0
                const chgColor = up ? '#22c55e' : down ? '#ef4444' : '#475569'
                const chgTip = row.prevMonth
                  ? `vs ${monthLabel(row.prevMonth)} (${monthLabel(row.deliveryMonth)})`
                  : 'No previous period available'

                return (
                  <div
                    key={row.symbol}
                    style={{
                      display: 'grid',
                      gridTemplateColumns: '1fr 7rem 5rem',
                      padding: '0.4rem 0.75rem',
                      borderBottom: i < rows.length - 1 ? '1px solid #0f1623' : 'none',
                      alignItems: 'center',
                      transition: 'background 0.1s',
                    }}
                    onMouseEnter={(e) => (e.currentTarget.style.background = '#ffffff07')}
                    onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
                  >
                    {/* Name + meta */}
                    <div style={{ minWidth: 0, paddingRight: '0.5rem' }}>
                      <div style={{
                        color: '#c0cfe0',
                        fontSize: '0.7rem',
                        fontWeight: 500,
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        whiteSpace: 'nowrap',
                        lineHeight: 1.3,
                      }}
                        title={row.name}
                      >
                        {geoFlag(row.geography)} {row.name}
                      </div>
                      <div style={{ color: '#334155', fontSize: '0.55rem', marginTop: '0.1rem' }}>
                        {row.symbol} · {row.source}
                      </div>
                    </div>

                    {/* Price + unit */}
                    <div style={{ textAlign: 'right' }}>
                      <span style={{
                        color: '#f1f5f9',
                        fontSize: '0.85rem',
                        fontWeight: 700,
                        fontFamily: 'ui-monospace, monospace',
                        fontVariantNumeric: 'tabular-nums',
                        letterSpacing: '-0.01em',
                      }}>
                        {row.price > 0 ? fmtPrice(row.price) : '—'}
                      </span>
                      <div style={{ color: '#475569', fontSize: '0.55rem', marginTop: '0.05rem' }}>
                        {row.unit}
                      </div>
                    </div>

                    {/* Change */}
                    <div style={{ textAlign: 'right' }} title={chgTip}>
                      {row.change !== null ? (
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
                        <span style={{ color: '#1e293b', fontSize: '0.65rem' }}>—</span>
                      )}
                    </div>
                  </div>
                )
              })}
            </div>
          )
        })}
      </div>
    </div>
  )
}
