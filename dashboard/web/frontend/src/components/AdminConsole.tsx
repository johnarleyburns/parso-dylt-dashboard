import { useState, useEffect } from 'react'
import { X, RefreshCw, Shield } from 'lucide-react'
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

const NEON_CYAN = '#00d4ff'

export default function AdminConsole({ onClose, mobile }: Props) {
  const [nodes, setNodes]           = useState<NodeInfo[]>([])
  const [selectedNode, setSelectedNode] = useState<string | null>(null)
  const [lastRefresh, setLastRefresh] = useState<Date | null>(null)
  const [error, setError]           = useState<string | null>(null)

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

  // Close drawer when ESC is pressed
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

  return (
    <div style={{
      position: 'fixed',
      inset: 0,
      zIndex: 1000,
      display: 'flex',
      flexDirection: 'column',
      background: '#000d1a',
      // Scan-line cyberpunk overlay
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
        borderBottom: `1px solid #0d2a3a`,
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

        {/* Live indicator */}
        <div style={{
          display: 'flex',
          alignItems: 'center',
          gap: '0.3rem',
          marginLeft: mobile ? 'auto' : '0.5rem',
        }}>
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
            style={{
              background: 'none',
              border: `1px solid #0d2a3a`,
              borderRadius: 3,
              color: '#2a6a7a',
              cursor: 'pointer',
              padding: '0.2rem 0.4rem',
              display: 'flex',
              alignItems: 'center',
              gap: '0.2rem',
              fontFamily: 'monospace',
              fontSize: '0.55rem',
            }}
          >
            <RefreshCw size={10} /> REFRESH
          </button>
          <button
            onClick={onClose}
            style={{
              background: 'none',
              border: `1px solid #0d2a3a`,
              borderRadius: 3,
              color: '#4a7a8a',
              cursor: 'pointer',
              padding: '0.2rem 0.35rem',
              display: 'flex',
              alignItems: 'center',
            }}
          >
            <X size={12} />
          </button>
        </div>
      </header>

      {/* ---- World map ---- */}
      <div style={{
        flex: mobile ? '0 0 42%' : '0 0 54%',
        position: 'relative',
        borderBottom: `1px solid #0d2a3a`,
        overflow: 'hidden',
      }}>
        {nodes.length === 0 ? (
          <div style={{
            position: 'absolute',
            inset: 0,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            color: '#1a4050',
            fontFamily: 'monospace',
            fontSize: '0.65rem',
            letterSpacing: '0.1em',
          }}>
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
        {/* Corner decorations */}
        <div style={{ position: 'absolute', top: 4, left: 4, width: 10, height: 10, borderTop: `1px solid ${NEON_CYAN}`, borderLeft: `1px solid ${NEON_CYAN}`, opacity: 0.4, pointerEvents: 'none' }} />
        <div style={{ position: 'absolute', top: 4, right: 4, width: 10, height: 10, borderTop: `1px solid ${NEON_CYAN}`, borderRight: `1px solid ${NEON_CYAN}`, opacity: 0.4, pointerEvents: 'none' }} />
        <div style={{ position: 'absolute', bottom: 4, left: 4, width: 10, height: 10, borderBottom: `1px solid ${NEON_CYAN}`, borderLeft: `1px solid ${NEON_CYAN}`, opacity: 0.4, pointerEvents: 'none' }} />
        <div style={{ position: 'absolute', bottom: 4, right: 4, width: 10, height: 10, borderBottom: `1px solid ${NEON_CYAN}`, borderRight: `1px solid ${NEON_CYAN}`, opacity: 0.4, pointerEvents: 'none' }} />
      </div>

      {/* ---- Node grid ---- */}
      <div style={{ flex: 1, overflow: 'auto' }}>
        <div style={{
          padding: '0.3rem 0.75rem 0.2rem',
          color: '#1a4a5a',
          fontSize: '0.55rem',
          fontFamily: 'monospace',
          letterSpacing: '0.12em',
          flexShrink: 0,
        }}>
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

      {/* ---- Node detail drawer (slides in when a node is selected) ---- */}
      <NodeDetailDrawer
        node={selectedNodeObj}
        onClose={() => setSelectedNode(null)}
        mobile={mobile}
      />
    </div>
  )
}
