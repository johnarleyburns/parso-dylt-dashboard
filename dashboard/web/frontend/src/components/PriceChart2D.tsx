import { useState, useMemo } from 'react'
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
} from 'recharts'
import type { AllPrices, PricePoint } from '../types'

const SECTOR_COLORS: Record<string, string> = {
  crude:       '#3b82f6',
  natgas:      '#f97316',
  lng:         '#f59e0b',
  lpg:         '#22c55e',
  ngls:        '#84cc16',
  electricity: '#a855f7',
  refined:     '#ef4444',
}

function shortMonth(deliveryMonth: string): string {
  const d = new Date(deliveryMonth)
  return isNaN(d.getTime()) ? deliveryMonth : d.toLocaleString('en-US', { month: 'short', year: '2-digit' })
}

interface SeriesSpec {
  key: string        // "sector:symbol"
  label: string      // e.g. "CL (crude)"
  color: string
  unit: string
}

interface PriceChart2DProps {
  prices: AllPrices
  visibleSectors: Set<string>
}

export default function PriceChart2D({ prices, visibleSectors }: PriceChart2DProps) {
  // Build list of all available series
  const allSeries = useMemo<SeriesSpec[]>(() => {
    const result: SeriesSpec[] = []
    for (const [sector, pts] of Object.entries(prices)) {
      if (!visibleSectors.has(sector) || pts.length === 0) continue
      const bySymbol = new Map<string, PricePoint>()
      pts.forEach((p) => bySymbol.set(p.symbol, p))
      for (const [symbol, p] of bySymbol) {
        result.push({
          key: `${sector}:${symbol}`,
          label: `${symbol} (${sector})`,
          color: SECTOR_COLORS[sector] ?? '#94a3b8',
          unit: p.unit,
        })
      }
    }
    return result
  }, [prices, visibleSectors])

  const [selectedKeys, setSelectedKeys] = useState<Set<string>>(new Set())

  // Auto-select all series when they first appear
  const effectiveSelected = selectedKeys.size === 0 ? new Set(allSeries.map((s) => s.key)) : selectedKeys

  function toggleSeries(key: string) {
    setSelectedKeys((prev) => {
      const next = new Set(prev.size === 0 ? allSeries.map((s) => s.key) : prev)
      if (next.has(key)) {
        if (next.size > 1) next.delete(key)
      } else {
        next.add(key)
      }
      return next
    })
  }

  // Build chart data: array of { month, "sector:symbol": price, ... }
  const chartData = useMemo(() => {
    const monthMap = new Map<string, Record<string, number>>()

    for (const [sector, pts] of Object.entries(prices)) {
      if (!visibleSectors.has(sector)) continue
      for (const p of pts) {
        if (!p.delivery_month) continue
        const seriesKey = `${sector}:${p.symbol}`
        if (!effectiveSelected.has(seriesKey)) continue
        const month = shortMonth(p.delivery_month)
        const row = monthMap.get(month) ?? {}
        // If multiple points for same symbol+month (e.g. EIA + YF), take the most recent
        if (!(seriesKey in row) || p.price > 0) {
          row[seriesKey] = p.price
        }
        monthMap.set(month, row)
      }
    }

    // Sort by original date order
    const sorted = Array.from(monthMap.entries())
      .map(([month, vals]) => ({ month, ...vals }))
      .sort((a, b) => {
        const da = new Date(a.month), db = new Date(b.month)
        return da.getTime() - db.getTime()
      })
    return sorted
  }, [prices, visibleSectors, effectiveSelected])

  const selectedSeries = allSeries.filter((s) => effectiveSelected.has(s.key))

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column', background: '#0a0e1a', color: '#e2e8f0' }}>
      {/* Series selector */}
      <div
        style={{
          padding: '0.5rem 1rem',
          borderBottom: '1px solid #1e293b',
          display: 'flex',
          flexWrap: 'wrap',
          gap: '0.4rem',
          flexShrink: 0,
        }}
      >
        <span style={{ color: '#64748b', fontSize: '0.65rem', fontWeight: 600, alignSelf: 'center', marginRight: '0.25rem' }}>
          SERIES:
        </span>
        {allSeries.map((s) => {
          const active = effectiveSelected.has(s.key)
          return (
            <button
              key={s.key}
              onClick={() => toggleSeries(s.key)}
              style={{
                padding: '0.15rem 0.5rem',
                borderRadius: 3,
                border: `1px solid ${active ? s.color : '#1e293b'}`,
                background: active ? `${s.color}22` : 'transparent',
                color: active ? s.color : '#475569',
                cursor: 'pointer',
                fontSize: '0.65rem',
                fontFamily: 'inherit',
                fontWeight: active ? 600 : 400,
              }}
            >
              {s.label}
            </button>
          )
        })}
      </div>

      {/* Chart */}
      <div style={{ flex: 1, padding: '0.75rem 0.5rem 0.5rem' }}>
        {chartData.length === 0 ? (
          <div style={{ height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#475569', fontSize: '0.8rem' }}>
            No price data available
          </div>
        ) : (
          <ResponsiveContainer width="100%" height="100%">
            <LineChart data={chartData} margin={{ top: 4, right: 16, left: 0, bottom: 4 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="#1e293b" />
              <XAxis
                dataKey="month"
                tick={{ fill: '#64748b', fontSize: 10 }}
                tickLine={false}
                interval="preserveStartEnd"
              />
              <YAxis
                tick={{ fill: '#64748b', fontSize: 10 }}
                tickLine={false}
                axisLine={false}
                width={55}
              />
              <Tooltip
                contentStyle={{ background: '#0f172a', border: '1px solid #1e293b', borderRadius: 4, fontSize: 11 }}
                labelStyle={{ color: '#94a3b8' }}
                itemStyle={{ color: '#e2e8f0' }}
              />
              <Legend
                wrapperStyle={{ fontSize: 10, color: '#64748b', paddingTop: 4 }}
              />
              {selectedSeries.map((s) => (
                <Line
                  key={s.key}
                  type="monotone"
                  dataKey={s.key}
                  name={s.label}
                  stroke={s.color}
                  dot={false}
                  strokeWidth={1.5}
                  connectNulls
                />
              ))}
            </LineChart>
          </ResponsiveContainer>
        )}
      </div>
    </div>
  )
}
