/**
 * EnergyCurve3D — 3-D forward-curve chart for all energy sectors.
 *
 * IMPORTANT — Agent Failure Case #3 prevention:
 *   The <Canvas> mounts ONCE and is never torn down between data refreshes.
 *   Geometry is updated imperatively via a bufferGeometry ref (useEffect +
 *   setAttribute). React state changes in App.tsx do NOT remount the scene.
 *   An earlier agent version re-created the entire scene on every 30-second
 *   price poll, pegging one CPU core at 100%.
 *
 * IMPORTANT — Agent Failure Case #4 prevention:
 *   Do NOT use @react-three/drei Html for any labels in this component.
 *   Html portals to gl.domElement.parentNode and in some drei versions the
 *   portal divs escape CSS containment (display:none on the parent is ignored),
 *   leaving phantom floating labels visible on other views. Use plain DOM
 *   elements rendered outside the Canvas instead (see the legend div below).
 */

import { useRef, useEffect, useMemo } from 'react'
import { Canvas } from '@react-three/fiber'
import { OrbitControls } from '@react-three/drei'
import * as THREE from 'three'
import type { AllPrices, PricePoint } from '../types'

// ---- colour palette (one per sector) ----
const SECTOR_COLORS: Record<string, string> = {
  crude:       '#3b82f6', // blue
  natgas:      '#f97316', // orange
  lng:         '#f59e0b', // amber
  lpg:         '#22c55e', // green
  ngls:        '#84cc16', // lime
  electricity: '#a855f7', // purple
  refined:     '#ef4444', // red
}

// Build a list of 3-D points for one product across its delivery months.
// X = month index relative to the earliest month in the dataset (0, 1, 2 …)
// Y = price (actual USD value — label shows unit)
// Z = product index within its sector (separates curves visually on Z axis)
function buildPoints(
  pts: PricePoint[],
  zIndex: number,
  baseMonth: Date,
): [number, number, number][] {
  return pts
    .map((p) => {
      const d = new Date(p.delivery_month || new Date().toISOString())
      const monthsDiff =
        (d.getFullYear() - baseMonth.getFullYear()) * 12 +
        (d.getMonth() - baseMonth.getMonth())
      return [monthsDiff, p.price, zIndex * 2] as [number, number, number]
    })
    .sort((a, b) => a[0] - b[0])
}

// ---- ProductSurface — tube-mesh ref-update pattern (never remounts) ----
// WebGL ignores linewidth > 1 on most GPUs; TubeGeometry is a real surface
// with controllable radius and shading.
interface ProductCurveProps {
  points: [number, number, number][]
  color: string
}

const TUBE_RADIUS = 0.25
const TUBE_RADIAL_SEGMENTS = 6

function ProductCurve({ points, color }: ProductCurveProps) {
  const meshRef = useRef<THREE.Mesh>(null)

  // Allocate the mesh ONCE with a placeholder geometry and stable material.
  const meshObject = useMemo(() => {
    const mat = new THREE.MeshStandardMaterial({
      color,
      roughness: 0.35,
      metalness: 0.15,
      side: THREE.DoubleSide,
    })
    return new THREE.Mesh(new THREE.BufferGeometry(), mat)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [color])

  // Replace the geometry imperatively when points change — mesh never remounts.
  useEffect(() => {
    const mesh = meshRef.current
    if (!mesh || points.length < 2) return

    mesh.geometry.dispose()
    const v3 = points.map(([x, y, z]) => new THREE.Vector3(x, y, z))
    const curve = new THREE.CatmullRomCurve3(v3)
    mesh.geometry = new THREE.TubeGeometry(
      curve,
      Math.max(points.length * 3, 12),
      TUBE_RADIUS,
      TUBE_RADIAL_SEGMENTS,
      false,
    )
  }, [points])

  return <primitive ref={meshRef} object={meshObject} />
}

// ---- main component ----
interface EnergyCurve3DProps {
  prices: AllPrices
  visibleSectors: Set<string>
}

export default function EnergyCurve3D({ prices, visibleSectors }: EnergyCurve3DProps) {
  // Compute the earliest delivery month across all displayed data.
  const baseMonth = useMemo(() => {
    const dates: Date[] = []
    for (const [sector, pts] of Object.entries(prices)) {
      if (!visibleSectors.has(sector)) continue
      pts.forEach((p) => {
        if (p.delivery_month) dates.push(new Date(p.delivery_month))
      })
    }
    return dates.length > 0 ? new Date(Math.min(...dates.map((d) => d.getTime()))) : new Date()
  }, [prices, visibleSectors])

  // Gather all curves: one per unique symbol across visible sectors.
  // Prices are normalized per-sector (0–100 scale) so crude ($94) and gas ($2.68)
  // are both visible on the same Y axis without one dwarfing the other.
  const curves = useMemo(() => {
    const result: Array<{
      key: string
      points: [number, number, number][]
      color: string
      label: string
    }> = []

    let zIndex = 0
    for (const [sector, pts] of Object.entries(prices)) {
      if (!visibleSectors.has(sector) || pts.length === 0) continue

      // Compute sector-wide min/max for normalization.
      const allPrices = pts.map((p) => p.price).filter((v) => v > 0)
      const sectorMin = Math.min(...allPrices)
      const sectorMax = Math.max(...allPrices)
      const sectorRange = sectorMax - sectorMin || 1

      // Group by symbol — each symbol gets its own curve.
      const bySymbol = new Map<string, PricePoint[]>()
      pts.forEach((p) => {
        const existing = bySymbol.get(p.symbol) ?? []
        existing.push(p)
        bySymbol.set(p.symbol, existing)
      })

      for (const [symbol, symbolPts] of bySymbol) {
        // Normalize each price to 0–100 within this sector's range.
        const normalized = symbolPts.map((p) => ({
          ...p,
          price: ((p.price - sectorMin) / sectorRange) * 80 + 10, // 10–90 band
        }))
        result.push({
          key: `${sector}-${symbol}`,
          points: buildPoints(normalized, zIndex, baseMonth),
          color: SECTOR_COLORS[sector] ?? '#94a3b8',
          label: symbol,
        })
        zIndex++
      }
    }
    return result
  }, [prices, visibleSectors, baseMonth])

  return (
    // Outer div fills the panel; inner div constrains the canvas to ~half height.
    <div style={{ position: 'relative', width: '100%', height: '100%', display: 'flex', alignItems: 'flex-start' }}>
      <div style={{ position: 'relative', width: '100%', height: '42vh' }}>
        {/* The Canvas mounts once. Price data changes update geometry via refs — no remount. */}
        <Canvas
          camera={{ position: [30, 55, 90], fov: 75 }}
          style={{ background: '#0a0e1a', width: '100%', height: '100%' }}
        >
          <ambientLight intensity={0.5} />
          <directionalLight position={[20, 60, 40]} intensity={0.8} />
          <directionalLight position={[-20, 30, -20]} intensity={0.3} />

          {/* One surface tube per product symbol */}
          {curves.map(({ key, points, color }) => (
            <ProductCurve key={key} points={points} color={color} />
          ))}

          {/* Grid helpers for orientation */}
          <gridHelper args={[80, 32, '#1e293b', '#1e293b']} position={[14, 0, 20]} />

          <OrbitControls
            enablePan
            enableZoom
            enableRotate
            target={[14, 45, 10]}
            minDistance={10}
            maxDistance={400}
          />
        </Canvas>

        {/* Static legend — plain DOM, no drei Html, no portal leaks */}
        <div
          style={{
            position: 'absolute',
            bottom: 8,
            left: 10,
            fontSize: '0.6rem',
            color: '#475569',
            pointerEvents: 'none',
            fontFamily: 'ui-monospace, monospace',
            lineHeight: 1.6,
          }}
        >
          <div>X: Forward months &nbsp;·&nbsp; Y: Normalized price (0–100) &nbsp;·&nbsp; Z: Product offset</div>
          <div style={{ color: '#374151', marginTop: 2 }}>Drag to rotate &nbsp;·&nbsp; Scroll to zoom &nbsp;·&nbsp; Shift+drag to pan</div>
        </div>
      </div>
    </div>
  )
}
