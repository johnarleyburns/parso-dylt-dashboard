import { describe, it, expect } from 'vitest'
import type { HistoryPoint, HistoryResponse } from '../types'

// ---------------------------------------------------------------------------
// These are pure-function extractions of the logic inside PriceHistoryModal.
// They replicate exactly what the component computes from the API response.
// ---------------------------------------------------------------------------

/** Maps API points into Recharts-compatible chart data. */
function mapChartData(points: HistoryPoint[]): { t: number; price: number }[] {
  return points.map((p) => ({ t: new Date(p.scraped_at).getTime(), price: p.price }))
}

/** Replicates the yDomain useMemo from the component. */
function computeYDomain(chartData: { t: number; price: number }[]): [number, number] {
  if (chartData.length === 0) return [0, 1]
  let min = Infinity, max = -Infinity
  for (const d of chartData) { if (d.price < min) min = d.price; if (d.price > max) max = d.price }
  const pad = (max - min) * 0.08 || max * 0.05 || 1
  return [+(min - pad).toFixed(4), +(max + pad).toFixed(4)]
}

/** Builds the same URL the component's fetch uses. */
function buildHistoryURL(sector: string, symbol: string, days: number): string {
  return `/api/v1/history/${sector}/${encodeURIComponent(symbol)}?days=${days}`
}

/** Simulates the full data flow from API JSON → chart-ready data. */
function processHistoryResponse(resp: HistoryResponse): { chartData: { t: number; price: number }[]; yDomain: [number, number] } {
  const points: HistoryPoint[] = resp.points ?? []
  const chartData = mapChartData(points)
  const yDomain = computeYDomain(chartData)
  return { chartData, yDomain }
}

// ---------------------------------------------------------------------------
// helper to build a HistoryPoint
// ---------------------------------------------------------------------------
function hp(price: number, scrapedAt: string): HistoryPoint {
  return { price, scraped_at: scrapedAt }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('PriceHistoryModal data processing', () => {

  // ---- chartData mapping ----

  describe('mapChartData', () => {
    it('converts an empty array to an empty array', () => {
      expect(mapChartData([])).toEqual([])
    })

    it('maps scraped_at strings to numeric timestamps', () => {
      const pts: HistoryPoint[] = [
        hp(75.5, '2025-06-15T12:00:00Z'),
        hp(76.0, '2025-06-16T12:00:00Z'),
      ]
      const result = mapChartData(pts)
      expect(result).toHaveLength(2)
      expect(result[0].t).toBe(new Date('2025-06-15T12:00:00Z').getTime())
      expect(result[0].price).toBe(75.5)
      expect(result[1].t).toBe(new Date('2025-06-16T12:00:00Z').getTime())
      expect(result[1].price).toBe(76.0)
    })

    it('handles a single point', () => {
      const pts: HistoryPoint[] = [hp(100, '2025-01-01T00:00:00Z')]
      const result = mapChartData(pts)
      expect(result).toHaveLength(1)
      expect(result[0].price).toBe(100)
    })

    it('preserves the order of input points', () => {
      const pts: HistoryPoint[] = [
        hp(10, '2025-01-01T00:00:00Z'),
        hp(20, '2025-01-02T00:00:00Z'),
        hp(30, '2025-01-03T00:00:00Z'),
      ]
      const prices = mapChartData(pts).map((d) => d.price)
      expect(prices).toEqual([10, 20, 30])
    })
  })

  // ---- yDomain ----

  describe('computeYDomain', () => {
    it('returns [0, 1] for empty data', () => {
      expect(computeYDomain([])).toEqual([0, 1])
    })

    it('computes padded domain for normal data', () => {
      const data = [{ t: 1, price: 50 }, { t: 2, price: 100 }]
      const [min, max] = computeYDomain(data)
      expect(min).toBe(46)
      expect(max).toBe(104)
    })

    it('handles single-point data (triggers 5% fallback pad)', () => {
      const data = [{ t: 1, price: 75 }]
      const [min, max] = computeYDomain(data)
      const pad = 75 * 0.05
      expect(min).toBeCloseTo(75 - pad, 2)
      expect(max).toBeCloseTo(75 + pad, 2)
    })

    it('handles zero-value data (triggers || 1 fallback)', () => {
      const data = [{ t: 1, price: 0 }, { t: 2, price: 0 }]
      const [min, max] = computeYDomain(data)
      expect(min).toBe(-1)
      expect(max).toBe(1)
    })

    it('handles negative prices', () => {
      const data = [{ t: 1, price: -10 }, { t: 2, price: 10 }]
      const [min, max] = computeYDomain(data)
      expect(min).toBeCloseTo(-11.6, 1)
      expect(max).toBeCloseTo(11.6, 1)
    })

    it('handles all identical non-zero values', () => {
      const data = [
        { t: 1, price: 42 }, { t: 2, price: 42 }, { t: 3, price: 42 },
      ]
      const [min, max] = computeYDomain(data)
      const pad = 42 * 0.05
      expect(min).toBeCloseTo(42 - pad, 1)
      expect(max).toBeCloseTo(42 + pad, 1)
    })
  })

  // ---- URL construction ----

  describe('buildHistoryURL', () => {
    it('builds a correct URL for a simple symbol', () => {
      expect(buildHistoryURL('crude', 'WTI', 90)).toBe('/api/v1/history/crude/WTI?days=90')
    })

    it('encodes symbols with spaces', () => {
      expect(buildHistoryURL('refined', 'RBOB Gasoline', 7)).toBe('/api/v1/history/refined/RBOB%20Gasoline?days=7')
    })

    it('encodes symbols with slashes', () => {
      expect(buildHistoryURL('ngls', 'Ethane/Propane Mix', 30)).toBe('/api/v1/history/ngls/Ethane%2FPropane%20Mix?days=30')
    })
  })

  // ---- full response processing ----

  describe('processHistoryResponse', () => {
    it('processes a multi-point API response end-to-end', () => {
      const resp: HistoryResponse = {
        sector: 'crude',
        symbol: 'WTI',
        days: 90,
        points: [
          hp(72.50, '2025-05-01T00:00:00Z'),
          hp(73.80, '2025-05-15T00:00:00Z'),
          hp(75.10, '2025-06-01T00:00:00Z'),
        ],
      }
      const { chartData, yDomain } = processHistoryResponse(resp)

      expect(chartData).toHaveLength(3)
      expect(chartData[0].price).toBe(72.50)
      expect(chartData[2].price).toBe(75.10)
      expect(chartData[0].t).toBeGreaterThan(0)

      const [min, max] = yDomain
      expect(min).toBeLessThan(72.5)
      expect(max).toBeGreaterThan(75.1)
    })

    it('returns empty chartData and [0,1] domain for empty points', () => {
      const resp: HistoryResponse = { sector: 'crude', symbol: 'WTI', days: 90, points: [] }
      const { chartData, yDomain } = processHistoryResponse(resp)
      expect(chartData).toEqual([])
      expect(yDomain).toEqual([0, 1])
    })

    it('returns empty chartData and [0,1] domain when points is undefined (null safety)', () => {
      const resp = { sector: 'crude', symbol: 'WTI', days: 90 } as HistoryResponse
      const { chartData, yDomain } = processHistoryResponse(resp)
      expect(chartData).toEqual([])
      expect(yDomain).toEqual([0, 1])
    })
  })
})
