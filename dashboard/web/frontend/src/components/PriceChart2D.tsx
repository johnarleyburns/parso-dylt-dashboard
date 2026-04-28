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
  coal:        '#78716c',
  carbon:      '#14b8a6',
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

  // Default: one representative series per sector (first in list). User can toggle more.
  const defaultSelected = useMemo<Set<string>>(() => {
    const seen = new Set<string>()
    const keys = new Set<string>()
    for (const s of allSeries) {
      const sector = s.key.split(':')[0]
      if (!seen.has(sector)) {
        seen.add(sector)
        keys.add(s.key)
      }
    }
    return keys
  }, [allSeries])

  const effectiveSelected = selectedKeys.size === 0 ? defaultSelected : selectedKeys

  function toggleSeries(key: string) {
    setSelectedKeys((prev) => {
      const next = new Set(prev.size === 0 ? defaultSelected : prev)
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
    // Key by ISO delivery_month ("2024-02-01") so we can sort chronologically.
    // Convert to display label only after sorting — shortMonth("Jan 24") is not parseable.
    const monthMap = new Map<string, Record<string, number>>()

    for (const [sector, pts] of Object.entries(prices)) {
      if (!visibleSectors.has(sector)) continue
      for (const p of pts) {
        if (!p.delivery_month || p.price <= 0) continue
        const seriesKey = `${sector}:${p.symbol}`
        if (!effectiveSelected.has(seriesKey)) continue
        const isoKey = p.delivery_month // "YYYY-MM-01"
        const row = monthMap.get(isoKey) ?? {}
        if (!(seriesKey in row)) row[seriesKey] = p.price
        monthMap.set(isoKey, row)
      }
    }

    // ISO date strings sort lexicographically = chronologically.
    return Array.from(monthMap.entries())
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([isoKey, vals]) => ({ month: shortMonth(isoKey), ...vals }))
  }, [prices, visibleSectors, effectiveSelected])

  // Y axis domain: fit to actual selected data with 5 % padding — never start at 0
  const yDomain = useMemo<[number, number]>(() => {
    let min = Infinity, max = -Infinity
    for (const row of chartData) {
      for (const [k, v] of Object.entries(row)) {
        if (k === 'month' || typeof v !== 'number' || v <= 0) continue
        if (v < min) min = v
        if (v > max) max = v
      }
    }
    if (!isFinite(min)) return [0, 1]
    const pad = (max - min) * 0.07 || max * 0.05
    return [Math.max(0, +(min - pad).toFixed(4)), +(max + pad).toFixed(4)]
  }, [chartData])

  const selectedSeries = allSeries.filter((s) => effectiveSelected.has(s.key))

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column', background: '#0a0e1a', color: '#e2e8f0' }}>
      {/* Series selector */}
      <div
        style={{
          padding: '0.5rem 1rem',
          borderBottom: '1px solid #1e293b',
          display: 'flex',
          flexWrap: 'nowrap',
          overflowX: 'auto',
          WebkitOverflowScrolling: 'touch',
          gap: '0.4rem',
          flexShrink: 0,
        }}
      >
        <span style={{ color: '#64748b', fontSize: '0.65rem', fontWeight: 600, alignSelf: 'center', marginRight: '0.25rem', flexShrink: 0 }}>
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
                flexShrink: 0,
                whiteSpace: 'nowrap',
              }}
            >
              {s.label}
            </button>
          )
        })}
      </div>

      {/* Chart */}
      <div style={{ flex: 1, padding: '0.75rem 0.5rem 0.5rem', display: 'flex', flexDirection: 'column' }}>
        {chartData.length === 0 ? (
          <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#475569', fontSize: '0.8rem' }}>
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
                domain={yDomain}
                tickFormatter={(v: number) => v >= 100 ? v.toFixed(0) : v.toFixed(2)}
                tick={{ fill: '#64748b', fontSize: 10 }}
                tickLine={false}
                axisLine={false}
                width={62}
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
