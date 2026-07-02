import { useState, useEffect, useMemo } from 'react'
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from 'recharts'
import { X } from 'lucide-react'
import type { HistoryResponse, HistoryPoint } from '../types'

const SECTOR_COLORS: Record<string, string> = {
  crude: '#3b82f6', natgas: '#f97316', lng: '#f59e0b', lpg: '#22c55e',
  ngls: '#84cc16', electricity: '#a855f7', refined: '#ef4444', coal: '#78716c', carbon: '#14b8a6',
}

const RANGES: { label: string; days: number }[] = [
  { label: '7D', days: 7 },
  { label: '30D', days: 30 },
  { label: '90D', days: 90 },
  { label: '1Y', days: 365 },
]

interface Props {
  sector: string
  symbol: string
  name?: string
  unit?: string
  onClose: () => void
}

export default function PriceHistoryModal({ sector, symbol, name, unit, onClose }: Props) {
  const [days, setDays] = useState(90)
  const [data, setData] = useState<HistoryResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const color = SECTOR_COLORS[sector] ?? '#3b82f6'

  useEffect(() => {
    const ac = new AbortController()
    setLoading(true)
    setError(null)
    fetch(`/api/v1/history/${sector}/${encodeURIComponent(symbol)}?days=${days}`, {
      headers: { Accept: 'application/json' },
      signal: ac.signal,
    })
      .then((r) => { if (!r.ok) throw new Error(`HTTP ${r.status}`); return r.json() as Promise<HistoryResponse> })
      .then((d) => { setData(d); setLoading(false) })
      .catch((e) => { if (e.name !== 'AbortError') { setError(String(e)); setLoading(false) } })
    return () => ac.abort()
  }, [sector, symbol, days])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  const points: HistoryPoint[] = data?.points ?? []

  const chartData = useMemo(() =>
    points.map((p) => ({ t: new Date(p.scraped_at).getTime(), price: p.price })),
    [points])

  const yDomain = useMemo<[number, number]>(() => {
    if (chartData.length === 0) return [0, 1]
    let min = Infinity, max = -Infinity
    for (const d of chartData) { if (d.price < min) min = d.price; if (d.price > max) max = d.price }
    const pad = (max - min) * 0.08 || max * 0.05 || 1
    return [+(min - pad).toFixed(4), +(max + pad).toFixed(4)]
  }, [chartData])

  const stats = useMemo(() => {
    if (points.length === 0) return null
    const first = points[0].price
    const last = points[points.length - 1].price
    const chg = last - first
    const pct = first > 0 ? (chg / first) * 100 : 0
    return { first, last, chg, pct }
  }, [points])

  const fmtDate = (t: number) => {
    const d = new Date(t)
    return days <= 30
      ? d.toLocaleString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', hour12: false })
      : d.toLocaleString('en-US', { month: 'short', day: 'numeric', year: '2-digit' })
  }

  return (
    <div
      onClick={onClose}
      style={{
        position: 'fixed', inset: 0, background: '#000000cc', zIndex: 1000,
        display: 'flex', alignItems: 'center', justifyContent: 'center', padding: '1rem',
      }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        style={{
          background: '#0d1525', border: `1px solid ${color}44`, borderTop: `2px solid ${color}`,
          borderRadius: 8, width: 'min(760px, 100%)', maxHeight: '90vh',
          display: 'flex', flexDirection: 'column', boxShadow: '0 10px 40px #000a',
        }}
      >
        {/* Header */}
        <div style={{ padding: '0.75rem 1rem', borderBottom: '1px solid #1a2332', display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
          <div style={{ minWidth: 0 }}>
            <div style={{ color: '#f1f5f9', fontSize: '0.9rem', fontWeight: 700, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {name || symbol}
            </div>
            <div style={{ color: '#64748b', fontSize: '0.65rem', marginTop: 2 }}>
              {symbol} · {sector}{unit ? ` · ${unit}` : ''}
            </div>
          </div>
          {stats && (
            <div style={{ marginLeft: 'auto', textAlign: 'right' }}>
              <div style={{ color: '#f1f5f9', fontSize: '1rem', fontWeight: 700, fontFamily: 'ui-monospace, monospace' }}>
                {stats.last.toLocaleString('en-US', { maximumFractionDigits: 4 })}
              </div>
              <div style={{ color: stats.chg >= 0 ? '#22c55e' : '#ef4444', fontSize: '0.7rem', fontWeight: 600 }}>
                {stats.chg >= 0 ? '▲' : '▼'} {Math.abs(stats.chg).toFixed(3)} ({stats.pct >= 0 ? '+' : ''}{stats.pct.toFixed(2)}%)
              </div>
            </div>
          )}
          <button
            onClick={onClose}
            aria-label="Close"
            style={{ background: 'transparent', border: 'none', color: '#64748b', cursor: 'pointer', padding: 4, flexShrink: 0 }}
          >
            <X size={18} />
          </button>
        </div>

        {/* Range selector */}
        <div style={{ padding: '0.5rem 1rem', display: 'flex', gap: '0.4rem', borderBottom: '1px solid #1a2332' }}>
          {RANGES.map((r) => (
            <button
              key={r.days}
              onClick={() => setDays(r.days)}
              style={{
                padding: '0.2rem 0.6rem', borderRadius: 4,
                border: `1px solid ${days === r.days ? color : '#1e293b'}`,
                background: days === r.days ? `${color}22` : 'transparent',
                color: days === r.days ? color : '#64748b',
                cursor: 'pointer', fontSize: '0.65rem', fontFamily: 'inherit', fontWeight: days === r.days ? 600 : 400,
              }}
            >
              {r.label}
            </button>
          ))}
          <span style={{ marginLeft: 'auto', alignSelf: 'center', color: '#475569', fontSize: '0.6rem' }}>
            {points.length} point{points.length !== 1 ? 's' : ''}
          </span>
        </div>

        {/* Chart */}
        <div style={{ flex: 1, minHeight: 320, padding: '0.75rem 0.5rem 0.5rem', display: 'flex', flexDirection: 'column' }}>
          {loading ? (
            <Centered>Loading history…</Centered>
          ) : error ? (
            <Centered color="#f59e0b">Failed to load history</Centered>
          ) : chartData.length === 0 ? (
            <Centered>No history yet — data accumulates as the scraper runs.</Centered>
          ) : (
            <div style={{ flex: 1, minHeight: 0 }}>
              <ResponsiveContainer width="100%" height="100%">
                <LineChart data={chartData} margin={{ top: 8, right: 20, left: 0, bottom: 4 }}>
                  <CartesianGrid strokeDasharray="3 3" stroke="#1e293b" />
                  <XAxis
                    dataKey="t"
                    type="number"
                    scale="time"
                    domain={['dataMin', 'dataMax']}
                    tickFormatter={fmtDate}
                    tick={{ fill: '#64748b', fontSize: 10 }}
                    tickLine={false}
                    minTickGap={40}
                  />
                  <YAxis
                    domain={yDomain}
                    tickFormatter={(v: number) => (v >= 100 ? v.toFixed(0) : v.toFixed(2))}
                    tick={{ fill: '#64748b', fontSize: 10 }}
                    tickLine={false}
                    axisLine={false}
                    width={58}
                  />
                  <Tooltip
                    contentStyle={{ background: '#0f172a', border: '1px solid #1e293b', borderRadius: 4, fontSize: 11 }}
                    labelStyle={{ color: '#94a3b8' }}
                    itemStyle={{ color: '#e2e8f0' }}
                    labelFormatter={(t) => new Date(t as number).toLocaleString('en-US', { timeZone: 'UTC', hour12: false })}
                    formatter={(v) => [Number(v).toLocaleString('en-US', { maximumFractionDigits: 4 }) + (unit ? ` ${unit}` : ''), 'Price']}
                  />
                  <Line type="monotone" dataKey="price" stroke={color} dot={false} strokeWidth={1.75} connectNulls />
                </LineChart>
              </ResponsiveContainer>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

function Centered({ children, color = '#475569' }: { children: React.ReactNode; color?: string }) {
  return (
    <div style={{ height: '100%', minHeight: 300, display: 'flex', alignItems: 'center', justifyContent: 'center', color, fontSize: '0.8rem', textAlign: 'center', padding: '0 1rem' }}>
      {children}
    </div>
  )
}
