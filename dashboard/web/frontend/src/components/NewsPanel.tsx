import { ExternalLink } from 'lucide-react'
import type { NewsResponse } from '../types'

function timeAgo(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime()
  const h = Math.floor(diff / 3_600_000)
  const d = Math.floor(h / 24)
  if (d > 0) return `${d}d ago`
  if (h > 0) return `${h}h ago`
  const m = Math.floor(diff / 60_000)
  return m > 0 ? `${m}m ago` : 'just now'
}

// Deterministic accent colour per source label
const SOURCE_COLORS: Record<string, { bg: string; text: string; bar: string }> = {
  'EIA':            { bg: '#1e3a5f', text: '#93c5fd', bar: '#3b82f6' },
  'OilPrice':       { bg: '#14532d', text: '#86efac', bar: '#22c55e' },
  'US DOE':         { bg: '#1e3a5f', text: '#bfdbfe', bar: '#60a5fa' },
  'EU Energy':      { bg: '#312e81', text: '#c7d2fe', bar: '#818cf8' },
  'UK DESNZ':       { bg: '#1c1917', text: '#d6d3d1', bar: '#a8a29e' },
  'Canada NRC':     { bg: '#7f1d1d', text: '#fca5a5', bar: '#ef4444' },
  'Rigzone':        { bg: '#1c1917', text: '#fdba74', bar: '#f97316' },
  'Carbon Brief':   { bg: '#064e3b', text: '#6ee7b7', bar: '#10b981' },
  'IEEFA':          { bg: '#4a1d96', text: '#ddd6fe', bar: '#a78bfa' },
  'Energy Monitor': { bg: '#0c4a6e', text: '#7dd3fc', bar: '#38bdf8' },
}

const DEFAULT_COLOR = { bg: '#1e293b', text: '#94a3b8', bar: '#475569' }

interface NewsPanelProps {
  news: NewsResponse
}

export default function NewsPanel({ news }: NewsPanelProps) {
  const items = news.items ?? []

  if (items.length === 0) {
    return (
      <div style={{ padding: '1rem', color: '#64748b' }}>
        No news items — scraper has not run yet.
      </div>
    )
  }

  return (
    <div
      style={{
        overflowY: 'auto',
        height: '100%',
        padding: '0.75rem',
        display: 'flex',
        flexDirection: 'column',
        gap: '0.75rem',
      }}
    >
      {items.map((item, idx) => {
        const c = SOURCE_COLORS[item.source] ?? DEFAULT_COLOR
        return (
          <article
            key={idx}
            style={{
              borderLeft: `3px solid ${c.bar}`,
              paddingLeft: '0.625rem',
            }}
          >
            <div
              style={{
                display: 'flex',
                alignItems: 'flex-start',
                justifyContent: 'space-between',
                gap: '0.5rem',
              }}
            >
              <a
                href={item.url}
                target="_blank"
                rel="noreferrer"
                style={{
                  color: '#e2e8f0',
                  textDecoration: 'none',
                  fontWeight: 600,
                  lineHeight: 1.3,
                  flex: 1,
                }}
              >
                {item.title}
              </a>
              <ExternalLink size={12} color="#64748b" style={{ flexShrink: 0, marginTop: 2 }} />
            </div>

            <div
              style={{
                marginTop: '0.25rem',
                display: 'flex',
                gap: '0.5rem',
                alignItems: 'center',
                color: '#64748b',
                fontSize: '0.7rem',
              }}
            >
              <span
                style={{
                  background: c.bg,
                  color: c.text,
                  padding: '0 0.375rem',
                  borderRadius: 3,
                }}
              >
                {item.source}
              </span>
              <span>{timeAgo(item.published_at)}</span>
            </div>

            {item.summary && (
              <p
                style={{
                  marginTop: '0.25rem',
                  color: '#94a3b8',
                  lineHeight: 1.4,
                  fontSize: '0.7rem',
                }}
              >
                {item.summary}
              </p>
            )}

            {item.tags && item.tags.length > 0 && (
              <div style={{ marginTop: '0.25rem', display: 'flex', flexWrap: 'wrap', gap: '0.25rem' }}>
                {item.tags.map((tag) => (
                  <span key={tag} style={{ color: '#475569', fontSize: '0.65rem' }}>
                    #{tag}
                  </span>
                ))}
              </div>
            )}
          </article>
        )
      })}
    </div>
  )
}
