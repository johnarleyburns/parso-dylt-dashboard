import { useState, useMemo } from 'react'
import * as XLSX from 'xlsx'
import { Download } from 'lucide-react'
import type { AllPrices, PricePoint } from '../types'

interface PriceTableProps {
  prices: AllPrices
  visibleSectors: Set<string>
}

type SortKey = keyof PricePoint
type SortDir = 'asc' | 'desc'

export default function PriceTable({ prices, visibleSectors }: PriceTableProps) {
  const [sortKey, setSortKey] = useState<SortKey>('name')
  const [sortDir, setSortDir] = useState<SortDir>('asc')
  const [filterText, setFilterText] = useState('')

  const rows = useMemo<PricePoint[]>(() => {
    const result: PricePoint[] = []
    for (const [sector, pts] of Object.entries(prices)) {
      if (!visibleSectors.has(sector)) continue
      result.push(...pts)
    }
    return result
  }, [prices, visibleSectors])

  const filtered = useMemo(() => {
    const q = filterText.toLowerCase()
    return rows.filter(
      (p) =>
        !q ||
        p.symbol.toLowerCase().includes(q) ||
        p.name.toLowerCase().includes(q) ||
        p.sector.toLowerCase().includes(q) ||
        p.geography.toLowerCase().includes(q),
    )
  }, [rows, filterText])

  const sorted = useMemo(() => {
    return [...filtered].sort((a, b) => {
      const av = a[sortKey] ?? ''
      const bv = b[sortKey] ?? ''
      const cmp = String(av).localeCompare(String(bv), undefined, { numeric: true })
      return sortDir === 'asc' ? cmp : -cmp
    })
  }, [filtered, sortKey, sortDir])

  function handleSort(key: SortKey) {
    if (key === sortKey) {
      setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'))
    } else {
      setSortKey(key)
      setSortDir('asc')
    }
  }

  function exportExcel() {
    const data = sorted.map((p) => ({
      Symbol: p.symbol,
      Name: p.name,
      Sector: p.sector,
      Exchange: p.exchange,
      Geography: p.geography,
      'Delivery Month': p.delivery_month,
      Price: p.price,
      Unit: p.unit,
      Source: p.source,
      'Scraped At': p.scraped_at,
    }))
    const ws = XLSX.utils.json_to_sheet(data)
    const wb = XLSX.utils.book_new()
    XLSX.utils.book_append_sheet(wb, ws, 'Prices')
    XLSX.writeFile(wb, `oilfield-prices-${new Date().toISOString().slice(0, 10)}.xlsx`)
  }

  const cols: { key: SortKey; label: string; align?: 'right' }[] = [
    { key: 'symbol',         label: 'Symbol' },
    { key: 'name',           label: 'Name' },
    { key: 'sector',         label: 'Sector' },
    { key: 'exchange',       label: 'Exchange' },
    { key: 'geography',      label: 'Geography' },
    { key: 'delivery_month', label: 'Delivery' },
    { key: 'price',          label: 'Price', align: 'right' },
    { key: 'unit',           label: 'Unit' },
    { key: 'source',         label: 'Source' },
  ]

  const thStyle: React.CSSProperties = {
    padding: '0.4rem 0.6rem',
    textAlign: 'left',
    color: '#64748b',
    fontSize: '0.65rem',
    fontWeight: 600,
    letterSpacing: '0.06em',
    borderBottom: '1px solid #1e293b',
    cursor: 'pointer',
    userSelect: 'none',
    whiteSpace: 'nowrap',
    position: 'sticky',
    top: 0,
    background: '#0a0e1a',
  }

  const tdStyle: React.CSSProperties = {
    padding: '0.3rem 0.6rem',
    fontSize: '0.7rem',
    borderBottom: '1px solid #0f172a',
    whiteSpace: 'nowrap',
    color: '#cbd5e1',
  }

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column', background: '#0a0e1a' }}>
      {/* Toolbar */}
      <div
        style={{
          padding: '0.5rem 1rem',
          borderBottom: '1px solid #1e293b',
          display: 'flex',
          alignItems: 'center',
          gap: '0.75rem',
          flexShrink: 0,
        }}
      >
        <input
          type="text"
          placeholder="Filter symbol, name, sector..."
          value={filterText}
          onChange={(e) => setFilterText(e.target.value)}
          style={{
            flex: 1,
            background: '#0f172a',
            border: '1px solid #1e293b',
            borderRadius: 4,
            padding: '0.3rem 0.6rem',
            color: '#e2e8f0',
            fontSize: '0.7rem',
            outline: 'none',
            fontFamily: 'inherit',
          }}
        />
        <span style={{ color: '#475569', fontSize: '0.65rem' }}>
          {sorted.length} row{sorted.length !== 1 ? 's' : ''}
        </span>
        <button
          onClick={exportExcel}
          title="Export to Excel"
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: '0.3rem',
            padding: '0.3rem 0.6rem',
            background: '#14532d',
            border: '1px solid #166534',
            borderRadius: 4,
            color: '#4ade80',
            cursor: 'pointer',
            fontSize: '0.65rem',
            fontFamily: 'inherit',
          }}
        >
          <Download size={11} /> Export XLSX
        </button>
      </div>

      {/* Table */}
      <div style={{ flex: 1, overflow: 'auto' }}>
        {sorted.length === 0 ? (
          <div style={{ padding: '2rem', textAlign: 'center', color: '#475569', fontSize: '0.8rem' }}>
            No data
          </div>
        ) : (
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr>
                {cols.map((c) => (
                  <th
                    key={c.key}
                    style={{ ...thStyle, textAlign: c.align ?? 'left' }}
                    onClick={() => handleSort(c.key)}
                  >
                    {c.label}
                    {sortKey === c.key && (
                      <span style={{ marginLeft: '0.25rem' }}>{sortDir === 'asc' ? '↑' : '↓'}</span>
                    )}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {sorted.map((p, i) => (
                <tr
                  key={i}
                  style={{ background: i % 2 === 0 ? 'transparent' : '#0f172a22' }}
                >
                  <td style={{ ...tdStyle, color: '#e2e8f0', fontWeight: 600 }}>{p.symbol}</td>
                  <td style={tdStyle}>{p.name}</td>
                  <td style={tdStyle}>{p.sector}</td>
                  <td style={tdStyle}>{p.exchange}</td>
                  <td style={tdStyle}>{p.geography}</td>
                  <td style={tdStyle}>{p.delivery_month?.slice(0, 7) ?? '—'}</td>
                  <td style={{ ...tdStyle, textAlign: 'right', fontVariantNumeric: 'tabular-nums' }}>
                    {p.price > 0 ? p.price.toFixed(4) : '—'}
                  </td>
                  <td style={{ ...tdStyle, color: '#64748b' }}>{p.unit}</td>
                  <td style={{ ...tdStyle, color: '#64748b' }}>{p.source}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}
