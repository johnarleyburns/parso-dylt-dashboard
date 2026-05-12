import type { AllHealth } from '../types'

const STATUS_COLOR: Record<string, string> = {
  ok:       '#22c55e',
  degraded: '#f59e0b',
  offline:  '#ef4444',
}

const PROVIDER_LABEL: Record<string, string> = {
  hetzner:  'Hetzner / US-East',
  kamatera: 'Kamatera / US-West',
  ionos:    'Ionos / EU-Berlin',
  upcloud:  'UpCloud / US-Central',
}

interface NodeHealthGridProps {
  health: AllHealth
}

export default function NodeHealthGrid({ health }: NodeHealthGridProps) {
  const nodes = ['n1', 'n2', 'n3']

  return (
    <div style={{ display: 'flex', gap: '0.75rem' }}>
      {nodes.map((name) => {
        const node = health[name]
        const status = node?.status ?? 'offline'
        const color = STATUS_COLOR[status] ?? STATUS_COLOR.offline
        const provider = node?.provider ? PROVIDER_LABEL[node.provider] ?? node.provider : '—'

        return (
          <div
            key={name}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '0.5rem',
              background: '#111827',
              border: `1px solid ${color}33`,
              borderRadius: 6,
              padding: '0.4rem 0.75rem',
            }}
          >
            {/* Status indicator */}
            <span
              style={{
                display: 'inline-block',
                width: 10,
                height: 10,
                borderRadius: '50%',
                background: color,
                boxShadow: `0 0 6px ${color}`,
              }}
            />
            <span style={{ color: '#e2e8f0', fontWeight: 600 }}>{name.toUpperCase()}</span>
            <span style={{ color: '#64748b', fontSize: '0.7rem' }}>{provider}</span>
            <span
              style={{
                color,
                fontSize: '0.7rem',
                fontWeight: 500,
                textTransform: 'uppercase',
              }}
            >
              {status}
            </span>
            {node?.etcd_healthy === false && (
              <span style={{ color: '#f59e0b', fontSize: '0.65rem' }}>etcd!</span>
            )}
          </div>
        )
      })}
    </div>
  )
}
