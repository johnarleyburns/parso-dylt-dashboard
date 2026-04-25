import { useState, useEffect, useRef } from 'react'
import { X, RefreshCw } from 'lucide-react'
import type { NodeInfo } from './WorldMap'

const API_BASE = import.meta.env.VITE_API_BASE ?? 'https://ctrl.oilfield.parso.guru'

interface NodeMetrics {
  load1: number
  load5: number
  load15: number
  mem_total_mb: number
  mem_used_mb: number
  mem_free_mb: number
  mem_used_pct: number
  uptime_seconds: number
}

interface LogsResponse {
  node: string
  lines: string[]
}

interface Props {
  node: NodeInfo | null
  onClose: () => void
  mobile: boolean
}

function Bar({ pct, color }: { pct: number; color: string }) {
  return (
    <div style={{
      height: 6,
      background: '#0d2a3a',
      borderRadius: 3,
      overflow: 'hidden',
    }}>
      <div style={{
        height: '100%',
        width: `${Math.min(100, pct)}%`,
        background: color,
        borderRadius: 3,
        boxShadow: `0 0 4px ${color}`,
        transition: 'width 0.4s',
      }} />
    </div>
  )
}

function formatUptime(sec: number): string {
  if (!sec) return '—'
  const d = Math.floor(sec / 86400)
  const h = Math.floor((sec % 86400) / 3600)
  const m = Math.floor((sec % 3600) / 60)
  return d > 0 ? `${d}d ${h}h ${m}m` : h > 0 ? `${h}h ${m}m` : `${m}m`
}

const NEON_GREEN = '#00ff9f'
const NEON_CYAN  = '#00d4ff'
const NEON_AMBER = '#ffb800'

export default function NodeDetailDrawer({ node, onClose, mobile }: Props) {
  const [metrics, setMetrics] = useState<NodeMetrics | null>(null)
  const [logs, setLogs]       = useState<string[]>([])
  const [metricsErr, setMetricsErr] = useState<string | null>(null)
  const [logsErr, setLogsErr]       = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const logRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!node) return
    setMetrics(null)
    setLogs([])
    setMetricsErr(null)
    setLogsErr(null)
    setLoading(true)

    const ctrl = new AbortController()
    const { signal } = ctrl
    const timer = setTimeout(() => ctrl.abort(), 10_000)

    Promise.all([
      fetch(`${API_BASE}/api/v1/nodes/${node.name}/metrics`, { signal })
        .then(r => r.ok ? r.json() : Promise.reject(`HTTP ${r.status}`))
        .then((m: NodeMetrics) => setMetrics(m))
        .catch((e) => { if ((e as Error).name !== 'AbortError') setMetricsErr(String(e)) }),
      fetch(`${API_BASE}/api/v1/nodes/${node.name}/logs`, { signal })
        .then(r => r.ok ? r.json() : Promise.reject(`HTTP ${r.status}`))
        .then((d: LogsResponse) => setLogs(d.lines ?? []))
        .catch((e) => { if ((e as Error).name !== 'AbortError') setLogsErr(String(e)) }),
    ]).finally(() => { setLoading(false); clearTimeout(timer) })

    return () => { ctrl.abort(); clearTimeout(timer) }
  }, [node])

  // Auto-scroll log to bottom when lines update
  useEffect(() => {
    if (logRef.current) {
      logRef.current.scrollTop = logRef.current.scrollHeight
    }
  }, [logs])

  if (!node) return null

  const drawerStyle: React.CSSProperties = mobile ? {
    position: 'fixed',
    bottom: 0,
    left: 0,
    right: 0,
    height: '70dvh',
    zIndex: 2000,
    display: 'flex',
    flexDirection: 'column',
    background: '#000d1a',
    borderTop: `2px solid ${NEON_CYAN}`,
    boxShadow: `0 -8px 32px ${NEON_CYAN}22`,
  } : {
    position: 'fixed',
    top: 0,
    right: 0,
    bottom: 0,
    width: 380,
    zIndex: 2000,
    display: 'flex',
    flexDirection: 'column',
    background: '#000d1a',
    borderLeft: `2px solid ${NEON_CYAN}`,
    boxShadow: `-8px 0 32px ${NEON_CYAN}22`,
  }

  const loadColor = (metrics?.load1 ?? 0) > 2 ? NEON_AMBER : NEON_GREEN
  const memColor  = (metrics?.mem_used_pct ?? 0) > 80 ? NEON_AMBER : NEON_GREEN

  return (
    <>
      {/* Backdrop */}
      <div
        onClick={onClose}
        style={{ position: 'fixed', inset: 0, zIndex: 1999, background: 'transparent' }}
      />
      <div style={drawerStyle}>
        {/* Drawer header */}
        <div style={{
          padding: '0.6rem 0.75rem',
          borderBottom: `1px solid #0d2a3a`,
          display: 'flex',
          alignItems: 'center',
          gap: '0.5rem',
          flexShrink: 0,
        }}>
          <span style={{ color: NEON_CYAN, fontFamily: 'monospace', fontWeight: 700, fontSize: '0.8rem', letterSpacing: '0.1em' }}>
            // {node.name.toUpperCase()} · {node.city}
          </span>
          {loading && <RefreshCw size={11} style={{ color: NEON_CYAN, animation: 'spin 1s linear infinite' }} />}
          <button
            onClick={onClose}
            style={{ marginLeft: 'auto', background: 'none', border: 'none', cursor: 'pointer', color: '#4a7a8a', padding: 2 }}
          >
            <X size={14} />
          </button>
        </div>

        <div style={{ flex: 1, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
          {/* Metrics section */}
          <div style={{
            padding: '0.6rem 0.75rem',
            borderBottom: '1px solid #0d2a3a',
            flexShrink: 0,
            fontFamily: 'monospace',
          }}>
            <div style={{ color: '#2a6a7a', fontSize: '0.55rem', letterSpacing: '0.12em', marginBottom: '0.4rem' }}>
              // SYSTEM METRICS
            </div>
            {metricsErr ? (
              <div style={{ color: '#ff4444', fontSize: '0.65rem' }}>{metricsErr}</div>
            ) : metrics ? (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '0.45rem' }}>
                {/* Load */}
                <div>
                  <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '0.2rem' }}>
                    <span style={{ color: '#4a7a8a', fontSize: '0.6rem' }}>LOAD AVG</span>
                    <span style={{ color: loadColor, fontSize: '0.6rem' }}>
                      {metrics.load1.toFixed(2)} / {metrics.load5.toFixed(2)} / {metrics.load15.toFixed(2)}
                    </span>
                  </div>
                  <Bar pct={(metrics.load1 / 4) * 100} color={loadColor} />
                </div>
                {/* Memory */}
                <div>
                  <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '0.2rem' }}>
                    <span style={{ color: '#4a7a8a', fontSize: '0.6rem' }}>MEMORY</span>
                    <span style={{ color: memColor, fontSize: '0.6rem' }}>
                      {metrics.mem_used_mb} / {metrics.mem_total_mb} MB ({metrics.mem_used_pct.toFixed(0)}%)
                    </span>
                  </div>
                  <Bar pct={metrics.mem_used_pct} color={memColor} />
                </div>
                {/* Uptime */}
                <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                  <span style={{ color: '#4a7a8a', fontSize: '0.6rem' }}>UPTIME</span>
                  <span style={{ color: NEON_GREEN, fontSize: '0.6rem' }}>
                    {formatUptime(metrics.uptime_seconds)}
                  </span>
                </div>
              </div>
            ) : (
              <div style={{ color: '#1a4050', fontSize: '0.6rem' }}>fetching…</div>
            )}
          </div>

          {/* Log terminal */}
          <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
            <div style={{
              padding: '0.3rem 0.75rem',
              borderBottom: '1px solid #0d2a3a',
              color: '#2a6a7a',
              fontSize: '0.55rem',
              letterSpacing: '0.12em',
              flexShrink: 0,
              fontFamily: 'monospace',
            }}>
              // JOURNAL LOGS (last 100 lines)
            </div>
            <div
              ref={logRef}
              style={{
                flex: 1,
                overflow: 'auto',
                padding: '0.4rem 0.75rem',
                fontFamily: '"Courier New", Courier, monospace',
                fontSize: '0.6rem',
                lineHeight: 1.6,
                color: '#00cc80',
                background: '#00050d',
                WebkitOverflowScrolling: 'touch',
              }}
            >
              {logsErr ? (
                <span style={{ color: '#ff4444' }}>{logsErr}</span>
              ) : logs.length === 0 ? (
                <span style={{ color: '#1a4050' }}>{loading ? 'fetching…' : 'no logs'}</span>
              ) : (
                logs.map((line, i) => (
                  <div key={i} style={{
                    color: line.includes('ERROR') || line.includes('error') ? '#ff6666'
                         : line.includes('WARN')  || line.includes('warn')  ? '#ffb800'
                         : '#00cc80',
                    wordBreak: 'break-all',
                  }}>
                    {line}
                  </div>
                ))
              )}
            </div>
          </div>
        </div>
      </div>
    </>
  )
}
