import { useEffect } from 'react'
import { MapContainer, TileLayer, Marker, Tooltip, Polyline } from 'react-leaflet'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'

export interface NodeInfo {
  name: string
  lat: number
  lon: number
  city: string
  country: string
  provider: string
  role: string
  status: string
  etcd_healthy: boolean
}

interface Props {
  nodes: NodeInfo[]
  selectedNode: string | null
  onSelectNode: (name: string) => void
}

const NEON_GREEN = '#00ff9f'
const NEON_CYAN  = '#00d4ff'
const NEON_RED   = '#ff4444'

function statusColor(node: NodeInfo): string {
  if (node.role === 'ctrl') return NEON_CYAN
  if (node.status === 'ok') return NEON_GREEN
  return NEON_RED
}

function makePulseIcon(node: NodeInfo, selected: boolean): L.DivIcon {
  const color = statusColor(node)
  const size = selected ? 18 : 12
  const ring = selected ? 32 : 24
  return L.divIcon({
    className: '',
    html: `
      <div style="position:relative;width:${ring}px;height:${ring}px;display:flex;align-items:center;justify-content:center;">
        <div class="pulse-ring" style="
          position:absolute;width:${ring}px;height:${ring}px;border-radius:50%;
          border:1.5px solid ${color};opacity:0.6;
        "></div>
        <div style="
          width:${size}px;height:${size}px;border-radius:50%;
          background:${color};box-shadow:0 0 ${selected ? 12 : 8}px ${color};
        "></div>
      </div>
    `,
    iconSize: [ring, ring],
    iconAnchor: [ring / 2, ring / 2],
  })
}

// All node-pair connection edges
function nodeEdges(nodes: NodeInfo[]): [NodeInfo, NodeInfo][] {
  const edges: [NodeInfo, NodeInfo][] = []
  for (let i = 0; i < nodes.length; i++) {
    for (let j = i + 1; j < nodes.length; j++) {
      edges.push([nodes[i], nodes[j]])
    }
  }
  return edges
}

export default function WorldMap({ nodes, selectedNode, onSelectNode }: Props) {
  // Inject CSS keyframe animation once
  useEffect(() => {
    if (document.getElementById('daylight-pulse-css')) return
    const style = document.createElement('style')
    style.id = 'daylight-pulse-css'
    style.textContent = `
      @keyframes pulse-ring {
        0%   { transform: scale(0.6); opacity: 0.8; }
        100% { transform: scale(1.8); opacity: 0; }
      }
      .pulse-ring { animation: pulse-ring 2s ease-out infinite; }
    `
    document.head.appendChild(style)
  }, [])

  const edges = nodeEdges(nodes)

  return (
    <MapContainer
      center={[48, -10]}
      zoom={2}
      minZoom={1}
      maxZoom={6}
      style={{ height: '100%', width: '100%', background: '#000d1a' }}
      attributionControl={false}
      zoomControl={false}
    >
      <TileLayer
        url="https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png"
        subdomains="abcd"
      />

      {/* Connection lines between nodes */}
      {edges.map(([a, b]) => (
        <Polyline
          key={`${a.name}-${b.name}`}
          positions={[[a.lat, a.lon], [b.lat, b.lon]]}
          pathOptions={{ color: NEON_CYAN, weight: 1, opacity: 0.35, dashArray: '4 6' }}
        />
      ))}

      {/* Node markers */}
      {nodes.map((node) => (
        <Marker
          key={node.name}
          position={[node.lat, node.lon]}
          icon={makePulseIcon(node, selectedNode === node.name)}
          eventHandlers={{ click: () => onSelectNode(node.name) }}
        >
          <Tooltip
            permanent={false}
            direction="top"
            offset={[0, -8]}
            className=""
            opacity={1}
          >
            <div style={{
              background: '#000d1a',
              border: `1px solid ${statusColor(node)}`,
              color: statusColor(node),
              fontFamily: 'monospace',
              fontSize: '0.65rem',
              padding: '0.2rem 0.4rem',
              lineHeight: 1.4,
              whiteSpace: 'nowrap',
              borderRadius: 2,
            }}>
              <strong>{node.name.toUpperCase()}</strong> · {node.city}, {node.country}<br />
              {node.provider} · {node.status.toUpperCase()}
            </div>
          </Tooltip>
        </Marker>
      ))}
    </MapContainer>
  )
}
