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

interface NewsPanelProps {
  news: NewsResponse
}

export default function NewsPanel({ news }: NewsPanelProps) {
  const items = [...(news.eia ?? []), ...(news.iea ?? [])].sort(
    (a, b) => new Date(b.published_at).getTime() - new Date(a.published_at).getTime(),
  )

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
      {items.map((item, idx) => (
        <article
          key={idx}
          style={{
            borderLeft: `3px solid ${item.source === 'EIA' ? '#3b82f6' : '#22c55e'}`,
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
                background: item.source === 'EIA' ? '#1e3a5f' : '#14532d',
                color: item.source === 'EIA' ? '#93c5fd' : '#86efac',
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
      ))}
    </div>
  )
}
