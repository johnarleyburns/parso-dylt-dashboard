import { useState, useEffect } from 'react'
import { X, RefreshCw, Shield, Database, Server, Globe } from 'lucide-react'
import WorldMap, { type NodeInfo } from './WorldMap'
import NodeGrid from './NodeGrid'
import NodeDetailDrawer from './NodeDetailDrawer'
import MapErrorBoundary from './MapErrorBoundary'

const API_BASE = import.meta.env.VITE_API_BASE ?? 'https://ctrl.oilfield.parso.guru'
const REFRESH_MS = 15_000

interface Props {
  onClose: () => void
  mobile: boolean
}

const NEON_CYAN  = '#00d4ff'
const NEON_GREEN = '#00ff9f'
const NEON_PINK  = '#ff0080'
const BG         = '#000d1a'
const BORDER     = '#0d2a3a'

// ---- data types ----

interface EtcdKVEntry {
  key: string
  size_b: number
  version: number
}

interface ServiceStatus {
  unit: string
  load_state: string
  active_state: string
  sub_state: string
  description: string
}

interface NodeServicesData {
  node: string
  services: ServiceStatus[]
}

interface DNSRecord {
  hostname: string
  type: string
  values: string[]
  error?: string
}

type ConsoleTab = 'nodes' | 'etcd' | 'services' | 'dns'

// ---- helpers ----

function fmtBytes(b: number): string {
  if (b < 1024) return `${b} B`
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(1)} KB`
  return `${(b / 1024 / 1024).toFixed(2)} MB`
}

function activeColor(state: string): string {
  if (state === 'active')   return NEON_GREEN
  if (state === 'inactive') return '#475569'
  if (state === 'failed')   return NEON_PINK
  return '#f59e0b'
}

// ---- shared panel chrome ----

function PanelLoading({ label }: { label: string }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', flex: 1, color: BORDER, fontFamily: 'monospace', fontSize: '0.65rem', letterSpacing: '0.1em' }}>
      // LOADING {label} …
    </div>
  )
}

function PanelError({ msg }: { msg: string }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', flex: 1, color: NEON_PINK, fontFamily: 'monospace', fontSize: '0.65rem' }}>
      ⚠ {msg}
    </div>
  )
}

const TH: React.CSSProperties = {
  padding: '0.3rem 0.6rem',
  fontFamily: 'monospace',
  fontSize: '0.6rem',
  fontWeight: 700,
  letterSpacing: '0.1em',
  color: '#1a4050',
  textAlign: 'left',
  borderBottom: `1px solid ${BORDER}`,
  whiteSpace: 'nowrap',
}

const TD: React.CSSProperties = {
  padding: '0.25rem 0.6rem',
  fontFamily: 'monospace',
  fontSize: '0.62rem',
  color: '#94a3b8',
  borderBottom: `1px solid #050f18`,
  whiteSpace: 'nowrap',
}

// ---- ETCD panel ----

function EtcdPanel({ mobile }: { mobile: boolean }) {
  const [entries, setEntries] = useState<EtcdKVEntry[] | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError]     = useState<string | null>(null)

  async function load() {
    setLoading(true)
    const ctrl = new AbortController()
    const t = setTimeout(() => ctrl.abort(), 12_000)
    try {
      const r = await fetch(`${API_BASE}/api/v1/cluster/etcd`, { signal: ctrl.signal })
      if (!r.ok) throw new Error(`HTTP ${r.status}`)
      setEntries(await r.json())
      setError(null)
    } catch (e) {
      setError(String(e))
    } finally {
      clearTimeout(t)
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [])

  if (loading) return <PanelLoading label="ETCD KV" />
  if (error)   return <PanelError msg={error} />
  if (!entries || entries.length === 0) return <PanelError msg="no keys found" />

  const totalSize = entries.reduce((s, e) => s + e.size_b, 0)

  return (
    <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
      <div style={{ padding: '0.35rem 0.75rem', borderBottom: `1px solid ${BORDER}`, display: 'flex', gap: '1rem', alignItems: 'center', flexShrink: 0 }}>
        <span style={{ fontFamily: 'monospace', fontSize: '0.6rem', color: NEON_CYAN }}>
          // /oilfield/ PREFIX — {entries.length} KEYS — {fmtBytes(totalSize)} TOTAL
        </span>
        <button onClick={load} style={{ marginLeft: 'auto', background: 'none', border: `1px solid ${BORDER}`, borderRadius: 3, color: '#2a6a7a', cursor: 'pointer', padding: '0.15rem 0.35rem', fontFamily: 'monospace', fontSize: '0.55rem', display: 'flex', alignItems: 'center', gap: '0.2rem' }}>
          <RefreshCw size={9} /> REFRESH
        </button>
      </div>
      <div style={{ flex: 1, overflow: 'auto' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse' }}>
          <thead>
            <tr>
              <th style={TH}>KEY PATH</th>
              <th style={{ ...TH, textAlign: 'right' }}>SIZE</th>
              <th style={{ ...TH, textAlign: 'right' }}>VERSION</th>
            </tr>
          </thead>
          <tbody>
            {entries.map((e) => (
              <tr key={e.key} style={{ background: 'transparent' }}>
                <td style={{ ...TD, color: NEON_CYAN, opacity: 0.85, maxWidth: mobile ? 200 : 500, overflow: 'hidden', textOverflow: 'ellipsis' }}>
                  {e.key}
                </td>
                <td style={{ ...TD, textAlign: 'right', color: e.size_b > 10000 ? '#f59e0b' : '#94a3b8' }}>
                  {fmtBytes(e.size_b)}
                </td>
                <td style={{ ...TD, textAlign: 'right', color: '#475569' }}>
                  {e.version}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

// ---- SERVICES panel ----

function ServicesPanel({ mobile }: { mobile: boolean }) {
  const [data, setData]   = useState<Record<string, NodeServicesData> | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError]   = useState<string | null>(null)

  async function load() {
    setLoading(true)
    try {
      const results = await Promise.all(
        ['n1', 'n2', 'n3'].map(async (name) => {
          const ctrl = new AbortController()
          const t = setTimeout(() => ctrl.abort(), 18_000)
          try {
            const r = await fetch(`${API_BASE}/api/v1/nodes/${name}/services`, { signal: ctrl.signal })
            if (!r.ok) throw new Error(`${name}: HTTP ${r.status}`)
            return (await r.json()) as NodeServicesData
          } finally {
            clearTimeout(t)
          }
        })
      )
      const byName: Record<string, NodeServicesData> = {}
      for (const r of results) byName[r.node] = r
      setData(byName)
      setError(null)
    } catch (e) {
      setError(String(e))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [])

  if (loading) return <PanelLoading label="SERVICES" />
  if (error)   return <PanelError msg={error} />
  if (!data)   return <PanelError msg="no data" />

  const nodes = ['n1', 'n2', 'n3']

  return (
    <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
      <div style={{ padding: '0.35rem 0.75rem', borderBottom: `1px solid ${BORDER}`, display: 'flex', alignItems: 'center', flexShrink: 0 }}>
        <span style={{ fontFamily: 'monospace', fontSize: '0.6rem', color: NEON_CYAN }}>
          // SYSTEMD SERVICES — N1 · N2 · N3
        </span>
        <button onClick={load} style={{ marginLeft: 'auto', background: 'none', border: `1px solid ${BORDER}`, borderRadius: 3, color: '#2a6a7a', cursor: 'pointer', padding: '0.15rem 0.35rem', fontFamily: 'monospace', fontSize: '0.55rem', display: 'flex', alignItems: 'center', gap: '0.2rem' }}>
          <RefreshCw size={9} /> REFRESH
        </button>
      </div>
      <div style={{ flex: 1, overflow: 'auto' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse' }}>
          <thead>
            <tr>
              <th style={TH}>NODE</th>
              <th style={TH}>UNIT</th>
              <th style={TH}>ACTIVE</th>
              <th style={TH}>STATE</th>
              {!mobile && <th style={TH}>DESCRIPTION</th>}
            </tr>
          </thead>
          <tbody>
            {nodes.flatMap((name) => {
              const nd = data[name]
              if (!nd) return []
              return nd.services.map((svc) => (
                <tr key={`${name}-${svc.unit}`}>
                  <td style={{ ...TD, color: NEON_CYAN, opacity: 0.7 }}>{name}</td>
                  <td style={{ ...TD, color: '#e2e8f0' }}>{svc.unit}</td>
                  <td style={{ ...TD, color: activeColor(svc.active_state), fontWeight: 600 }}>
                    {svc.active_state}
                  </td>
                  <td style={{ ...TD, color: '#64748b' }}>{svc.sub_state}</td>
                  {!mobile && <td style={{ ...TD, color: '#475569', maxWidth: 300, overflow: 'hidden', textOverflow: 'ellipsis' }}>{svc.description}</td>}
                </tr>
              ))
            })}
          </tbody>
        </table>
      </div>
    </div>
  )
}

// ---- DNS panel ----

function DNSPanel() {
  const [records, setRecords] = useState<DNSRecord[] | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError]     = useState<string | null>(null)

  async function load() {
    setLoading(true)
    const ctrl = new AbortController()
    const t = setTimeout(() => ctrl.abort(), 18_000)
    try {
      const r = await fetch(`${API_BASE}/api/v1/cluster/dns`, { signal: ctrl.signal })
      if (!r.ok) throw new Error(`HTTP ${r.status}`)
      setRecords(await r.json())
      setError(null)
    } catch (e) {
      setError(String(e))
    } finally {
      clearTimeout(t)
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [])

  if (loading) return <PanelLoading label="DNS" />
  if (error)   return <PanelError msg={error} />
  if (!records) return <PanelError msg="no data" />

  return (
    <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
      <div style={{ padding: '0.35rem 0.75rem', borderBottom: `1px solid ${BORDER}`, display: 'flex', alignItems: 'center', flexShrink: 0 }}>
        <span style={{ fontFamily: 'monospace', fontSize: '0.6rem', color: NEON_CYAN }}>
          // CLUSTER DNS — LIVE RESOLUTION VIA 1.1.1.1
        </span>
        <button onClick={load} style={{ marginLeft: 'auto', background: 'none', border: `1px solid ${BORDER}`, borderRadius: 3, color: '#2a6a7a', cursor: 'pointer', padding: '0.15rem 0.35rem', fontFamily: 'monospace', fontSize: '0.55rem', display: 'flex', alignItems: 'center', gap: '0.2rem' }}>
          <RefreshCw size={9} /> REFRESH
        </button>
      </div>
      <div style={{ flex: 1, overflow: 'auto' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse' }}>
          <thead>
            <tr>
              <th style={TH}>HOSTNAME</th>
              <th style={TH}>TYPE</th>
              <th style={TH}>VALUE(S)</th>
            </tr>
          </thead>
          <tbody>
            {records.map((rec) => (
              <tr key={rec.hostname}>
                <td style={{ ...TD, color: NEON_CYAN, opacity: 0.85 }}>{rec.hostname}</td>
                <td style={{ ...TD, color: '#64748b' }}>{rec.type}</td>
                <td style={{ ...TD }}>
                  {rec.error
                    ? <span style={{ color: NEON_PINK }}>{rec.error}</span>
                    : rec.values.map((v, i) => (
                        <span key={i} style={{ display: 'block', color: '#e2e8f0' }}>{v}</span>
                      ))
                  }
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

// ---- main component ----

export default function AdminConsole({ onClose, mobile }: Props) {
  const [nodes, setNodes]           = useState<NodeInfo[]>([])
  const [selectedNode, setSelectedNode] = useState<string | null>(null)
  const [lastRefresh, setLastRefresh] = useState<Date | null>(null)
  const [error, setError]           = useState<string | null>(null)
  const [tab, setTab]               = useState<ConsoleTab>('nodes')

  async function fetchNodes() {
    const ctrl = new AbortController()
    const timer = setTimeout(() => ctrl.abort(), 10_000)
    try {
      const resp = await fetch(`${API_BASE}/api/v1/nodes`, {
        headers: { Accept: 'application/json' },
        signal: ctrl.signal,
      })
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
      const data: NodeInfo[] = await resp.json()
      setNodes(data)
      setLastRefresh(new Date())
      setError(null)
    } catch (e) {
      setError(String(e))
    } finally {
      clearTimeout(timer)
    }
  }

  useEffect(() => {
    fetchNodes()
    const timer = setInterval(fetchNodes, REFRESH_MS)
    return () => clearInterval(timer)
  }, [])

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        if (selectedNode) setSelectedNode(null)
        else onClose()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [selectedNode, onClose])

  const selectedNodeObj = nodes.find(n => n.name === selectedNode) ?? null

  // ---- tab bar config ----
  const tabs: { id: ConsoleTab; label: string; icon: React.ReactNode }[] = [
    { id: 'nodes',    label: 'NODES',    icon: <Globe size={10} /> },
    { id: 'etcd',     label: 'ETCD',     icon: <Database size={10} /> },
    { id: 'services', label: 'SERVICES', icon: <Server size={10} /> },
    { id: 'dns',      label: 'DNS',      icon: <Globe size={10} /> },
  ]

  return (
    <div style={{
      position: 'fixed',
      inset: 0,
      zIndex: 1000,
      display: 'flex',
      flexDirection: 'column',
      background: BG,
      backgroundImage: `
        repeating-linear-gradient(
          0deg,
          transparent,
          transparent 2px,
          rgba(0, 212, 255, 0.012) 2px,
          rgba(0, 212, 255, 0.012) 4px
        )
      `,
    }}>
      {/* ---- Console header ---- */}
      <header style={{
        padding: mobile ? '0.5rem 0.75rem' : '0.6rem 1rem',
        borderBottom: `1px solid ${BORDER}`,
        display: 'flex',
        alignItems: 'center',
        gap: '0.6rem',
        flexShrink: 0,
        background: '#00050d',
      }}>
        <Shield size={mobile ? 13 : 15} style={{ color: NEON_CYAN }} />
        <span style={{
          color: NEON_CYAN,
          fontFamily: 'monospace',
          fontWeight: 700,
          fontSize: mobile ? '0.7rem' : '0.8rem',
          letterSpacing: '0.12em',
        }}>
          DAYLIGHT CONTROL CONSOLE
        </span>

        <div style={{ display: 'flex', alignItems: 'center', gap: '0.3rem', marginLeft: mobile ? 'auto' : '0.5rem' }}>
          <div style={{
            width: 6,
            height: 6,
            borderRadius: '50%',
            background: error ? '#ff4444' : NEON_CYAN,
            boxShadow: `0 0 6px ${error ? '#ff4444' : NEON_CYAN}`,
          }} />
          <span style={{ color: '#2a6a7a', fontSize: '0.55rem', fontFamily: 'monospace' }}>
            {error ? 'ERR' : 'LIVE'}
          </span>
        </div>

        {lastRefresh && !mobile && (
          <span style={{ color: '#1a4050', fontSize: '0.55rem', fontFamily: 'monospace', display: 'flex', alignItems: 'center', gap: '0.2rem' }}>
            <RefreshCw size={9} />
            {lastRefresh.toLocaleTimeString('en-US', { hour12: false })}
          </span>
        )}

        <div style={{ marginLeft: mobile ? '0' : 'auto', display: 'flex', gap: '0.4rem', alignItems: 'center' }}>
          <button
            onClick={fetchNodes}
            style={{ background: 'none', border: `1px solid ${BORDER}`, borderRadius: 3, color: '#2a6a7a', cursor: 'pointer', padding: '0.2rem 0.4rem', display: 'flex', alignItems: 'center', gap: '0.2rem', fontFamily: 'monospace', fontSize: '0.55rem' }}
          >
            <RefreshCw size={10} /> REFRESH
          </button>
          <button
            onClick={onClose}
            style={{ background: 'none', border: `1px solid ${BORDER}`, borderRadius: 3, color: '#4a7a8a', cursor: 'pointer', padding: '0.2rem 0.35rem', display: 'flex', alignItems: 'center' }}
          >
            <X size={12} />
          </button>
        </div>
      </header>

      {/* ---- Tab bar ---- */}
      <div style={{
        display: 'flex',
        gap: 0,
        borderBottom: `1px solid ${BORDER}`,
        background: '#00050d',
        flexShrink: 0,
        overflowX: 'auto',
      }}>
        {tabs.map(({ id, label, icon }) => {
          const active = tab === id
          return (
            <button
              key={id}
              onClick={() => setTab(id)}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: '0.3rem',
                padding: mobile ? '0.45rem 0.75rem' : '0.5rem 1.1rem',
                fontFamily: 'monospace',
                fontSize: mobile ? '0.6rem' : '0.65rem',
                letterSpacing: '0.08em',
                fontWeight: active ? 700 : 400,
                color: active ? NEON_CYAN : '#2a5060',
                background: active ? `${NEON_CYAN}08` : 'none',
                border: 'none',
                borderBottom: active ? `2px solid ${NEON_CYAN}` : '2px solid transparent',
                cursor: 'pointer',
                whiteSpace: 'nowrap',
                flexShrink: 0,
              }}
            >
              {icon} {label}
            </button>
          )
        })}
      </div>

      {/* ---- Tab content ---- */}
      {tab === 'nodes' && (
        <>
          {/* World map */}
          <div style={{
            flex: mobile ? '0 0 42%' : '0 0 54%',
            position: 'relative',
            borderBottom: `1px solid ${BORDER}`,
            overflow: 'hidden',
          }}>
            {nodes.length === 0 ? (
              <div style={{ position: 'absolute', inset: 0, display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#1a4050', fontFamily: 'monospace', fontSize: '0.65rem', letterSpacing: '0.1em' }}>
                {error ? `⚠ ${error}` : '// SCANNING CLUSTER …'}
              </div>
            ) : (
              <MapErrorBoundary>
                <WorldMap
                  nodes={nodes}
                  selectedNode={selectedNode}
                  onSelectNode={(name) => setSelectedNode(prev => prev === name ? null : name)}
                />
              </MapErrorBoundary>
            )}
            <div style={{ position: 'absolute', top: 4, left: 4, width: 10, height: 10, borderTop: `1px solid ${NEON_CYAN}`, borderLeft: `1px solid ${NEON_CYAN}`, opacity: 0.4, pointerEvents: 'none' }} />
            <div style={{ position: 'absolute', top: 4, right: 4, width: 10, height: 10, borderTop: `1px solid ${NEON_CYAN}`, borderRight: `1px solid ${NEON_CYAN}`, opacity: 0.4, pointerEvents: 'none' }} />
            <div style={{ position: 'absolute', bottom: 4, left: 4, width: 10, height: 10, borderBottom: `1px solid ${NEON_CYAN}`, borderLeft: `1px solid ${NEON_CYAN}`, opacity: 0.4, pointerEvents: 'none' }} />
            <div style={{ position: 'absolute', bottom: 4, right: 4, width: 10, height: 10, borderBottom: `1px solid ${NEON_CYAN}`, borderRight: `1px solid ${NEON_CYAN}`, opacity: 0.4, pointerEvents: 'none' }} />
          </div>

          {/* Node grid */}
          <div style={{ flex: 1, overflow: 'auto' }}>
            <div style={{ padding: '0.3rem 0.75rem 0.2rem', color: '#1a4a5a', fontSize: '0.55rem', fontFamily: 'monospace', letterSpacing: '0.12em', flexShrink: 0 }}>
              // NODE CLUSTER — {nodes.length} NODES — CLICK TO INSPECT
            </div>
            {nodes.length > 0 && (
              <NodeGrid
                nodes={nodes}
                selectedNode={selectedNode}
                onSelectNode={(name) => setSelectedNode(prev => prev === name ? null : name)}
                mobile={mobile}
              />
            )}
          </div>

          {/* Node detail drawer */}
          <NodeDetailDrawer
            node={selectedNodeObj}
            onClose={() => setSelectedNode(null)}
            mobile={mobile}
          />
        </>
      )}

      {tab === 'etcd' && (
        <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
          <EtcdPanel mobile={mobile} />
        </div>
      )}

      {tab === 'services' && (
        <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
          <ServicesPanel mobile={mobile} />
        </div>
      )}

      {tab === 'dns' && (
        <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
          <DNSPanel />
        </div>
      )}
    </div>
  )
}
