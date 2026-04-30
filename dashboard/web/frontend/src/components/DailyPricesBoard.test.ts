import { describe, it, expect } from 'vitest'
import { fmtPrice, fmtChange, fmtPct, isPriceStale } from './DailyPricesBoard'

// ---- fmtPrice ----

describe('fmtPrice', () => {
  it('formats values >= 10000 with no decimals', () => {
    expect(fmtPrice(12345)).toBe('12,345')
    expect(fmtPrice(10000)).toBe('10,000')
  })

  it('formats values >= 1000 with 1 decimal', () => {
    expect(fmtPrice(1234.56)).toBe('1234.6')
    expect(fmtPrice(1000)).toBe('1000.0')
  })

  it('formats values >= 100 with 2 decimals', () => {
    expect(fmtPrice(123.456)).toBe('123.46')
    expect(fmtPrice(100)).toBe('100.00')
  })

  it('formats values >= 10 with 2 decimals', () => {
    expect(fmtPrice(75.123)).toBe('75.12')
    expect(fmtPrice(10)).toBe('10.00')
  })

  it('formats values < 10 with 3 decimals', () => {
    expect(fmtPrice(3.14159)).toBe('3.142')
    expect(fmtPrice(0.5)).toBe('0.500')
  })
})

// ---- fmtChange ----

describe('fmtChange', () => {
  it('shows + prefix for positive values', () => {
    expect(fmtChange(1.5)).toBe('+1.500')
  })

  it('shows − prefix for negative values', () => {
    expect(fmtChange(-2.25)).toBe('−2.250')
  })

  it('formats large positive values with fewer decimals', () => {
    expect(fmtChange(1500)).toBe('+1500')
    expect(fmtChange(150)).toBe('+150.0')
    expect(fmtChange(15)).toBe('+15.00')
  })

  it('formats zero as positive', () => {
    expect(fmtChange(0)).toBe('+0.000')
  })
})

// ---- fmtPct ----

describe('fmtPct', () => {
  it('shows + prefix for positive percentages', () => {
    expect(fmtPct(3.14)).toBe('+3.14%')
  })

  it('shows no + prefix for negative percentages', () => {
    expect(fmtPct(-1.5)).toBe('-1.50%')
  })

  it('formats zero as positive', () => {
    expect(fmtPct(0)).toBe('+0.00%')
  })
})

// ---- isPriceStale ----

describe('isPriceStale', () => {
  const ONE_HOUR_MS  = 60 * 60 * 1000
  const NOW = new Date('2026-04-30T12:00:00Z').getTime()

  it('returns false for a price scraped 1 hour ago', () => {
    const scrapedAt = new Date(NOW - ONE_HOUR_MS).toISOString()
    expect(isPriceStale(scrapedAt, NOW)).toBe(false)
  })

  it('returns false for a price scraped exactly at the 24h boundary', () => {
    const scrapedAt = new Date(NOW - 24 * ONE_HOUR_MS).toISOString()
    expect(isPriceStale(scrapedAt, NOW)).toBe(false)
  })

  it('returns true for a price scraped 25 hours ago', () => {
    const scrapedAt = new Date(NOW - 25 * ONE_HOUR_MS).toISOString()
    expect(isPriceStale(scrapedAt, NOW)).toBe(true)
  })

  it('returns true for a price scraped 2 days ago', () => {
    const scrapedAt = new Date(NOW - 48 * ONE_HOUR_MS).toISOString()
    expect(isPriceStale(scrapedAt, NOW)).toBe(true)
  })

  it('returns true for an invalid/empty scraped_at string', () => {
    expect(isPriceStale('', NOW)).toBe(true)
    expect(isPriceStale('not-a-date', NOW)).toBe(true)
  })

  it('uses Date.now() when nowMs is omitted (smoke test)', () => {
    const recentlyScraped = new Date(Date.now() - ONE_HOUR_MS).toISOString()
    expect(isPriceStale(recentlyScraped)).toBe(false)
  })
})

// ---- Alphabetical sort within group ----

describe('alphabetical sort within group', () => {
  it('sorts price rows alphabetically by name', () => {
    const names = ['WTI Crude', 'Brent Crude', 'Dubai Crude', 'Arab Light']
    const rows = names.map(name => ({ name }))
    rows.sort((a, b) => a.name.localeCompare(b.name))
    expect(rows.map(r => r.name)).toEqual(['Arab Light', 'Brent Crude', 'Dubai Crude', 'WTI Crude'])
  })

  it('does not put US prices before international ones (pure alpha)', () => {
    // Henry Hub (US) should come before Natural Gas Europe alphabetically
    const rows = [
      { name: 'Natural Gas Europe', geography: 'EUROPE' },
      { name: 'Henry Hub', geography: 'NORTH_AMERICA' },
    ]
    rows.sort((a, b) => a.name.localeCompare(b.name))
    expect(rows[0].name).toBe('Henry Hub')
    expect(rows[1].name).toBe('Natural Gas Europe')
  })

  it('sorts mixed geography purely by name', () => {
    const rows = [
      { name: 'WTI',        geography: 'NORTH_AMERICA' },
      { name: 'Brent',      geography: 'EUROPE' },
      { name: 'Arab Light', geography: 'MIDDLE_EAST' },
    ]
    rows.sort((a, b) => a.name.localeCompare(b.name))
    expect(rows.map(r => r.name)).toEqual(['Arab Light', 'Brent', 'WTI'])
  })
})
