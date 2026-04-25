import type { NodeInfo } from './WorldMap'

interface Props {
  nodes: NodeInfo[]
  selectedNode: string | null
  onSelectNode: (name: string) => void
  mobile: boolean
}

const NEON_GREEN = '#00ff9f'
const NEON_CYAN  = '#00d4ff'
const NEON_RED   = '#ff4444'
const NEON_AMBER = '#ffb800'

function statusColor(node: NodeInfo): string {
  if (node.role === 'ctrl') return NEON_CYAN
  if (node.status === 'ok') return NEON_GREEN
  if (node.status === 'degraded') return NEON_AMBER
  return NEON_RED
}

function statusLabel(node: NodeInfo): string {
  if (node.role === 'ctrl') return 'CTRL'
  return node.status.toUpperCase()
}

export default function NodeGrid({ nodes, selectedNode, onSelectNode, mobile }: Props) {
  return (
    <div style={{
      display: 'grid',
      gridTemplateColumns: mobile
        ? 'repeat(2, 1fr)'
        : `repeat(${Math.min(nodes.length, 4)}, 1fr)`,
      gap: '0.5rem',
      padding: '0.5rem',
      overflowY: 'auto',
    }}>
      {nodes.map((node) => {
        const color = statusColor(node)
        const selected = selectedNode === node.name
        return (
          <button
            key={node.name}
            onClick={() => onSelectNode(selected ? '' : node.name)}
            style={{
              background: selected ? `${color}12` : '#000d1a',
              border: `1px solid ${selected ? color : '#0d2a3a'}`,
              borderRadius: 4,
              padding: '0.6rem 0.75rem',
              cursor: 'pointer',
              textAlign: 'left',
              transition: 'all 0.15s',
              boxShadow: selected ? `0 0 12px ${color}44` : 'none',
              fontFamily: 'monospace',
            }}
          >
            {/* Node name row */}
            <div style={{ display: 'flex', alignItems: 'center', gap: '0.4rem', marginBottom: '0.25rem' }}>
              {/* Status dot */}
              <div style={{
                width: 7,
                height: 7,
                borderRadius: '50%',
                background: color,
                boxShadow: `0 0 6px ${color}`,
                flexShrink: 0,
              }} />
              <span style={{ color, fontWeight: 700, fontSize: '0.8rem', letterSpacing: '0.08em' }}>
                {node.name.toUpperCase()}
              </span>
              <span style={{
                marginLeft: 'auto',
                fontSize: '0.55rem',
                color,
                border: `1px solid ${color}`,
                borderRadius: 2,
                padding: '0.05rem 0.3rem',
                letterSpacing: '0.1em',
              }}>
                {statusLabel(node)}
              </span>
            </div>

            {/* Location */}
            <div style={{ color: '#4a7a8a', fontSize: '0.6rem', lineHeight: 1.5 }}>
              {node.city}, {node.country}
            </div>
            <div style={{ color: '#2a5a6a', fontSize: '0.58rem' }}>
              {node.provider}
            </div>

            {/* etcd badge for runtime nodes */}
            {node.role === 'runtime' && (
              <div style={{
                marginTop: '0.3rem',
                fontSize: '0.55rem',
                color: node.etcd_healthy ? NEON_GREEN : NEON_RED,
                letterSpacing: '0.05em',
              }}>
                etcd {node.etcd_healthy ? '● healthy' : '○ degraded'}
              </div>
            )}

            {/* Click hint */}
            <div style={{
              marginTop: '0.35rem',
              fontSize: '0.5rem',
              color: '#1a4050',
              letterSpacing: '0.08em',
            }}>
              {selected ? '▲ CLOSE' : '▼ INSPECT'}
            </div>
          </button>
        )
      })}
    </div>
  )
}
