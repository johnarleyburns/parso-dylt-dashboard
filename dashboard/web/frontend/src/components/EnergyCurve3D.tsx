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
 *   elements rendered outside the Canvas instead.
 */

import { useRef, useEffect, useMemo } from 'react'
import { Canvas } from '@react-three/fiber'
import { OrbitControls } from '@react-three/drei'
import * as THREE from 'three'
import type { AllPrices, PricePoint } from '../types'

// ---- colour palette (one per sector) ----
const SECTOR_COLORS: Record<string, string> = {
  crude:       '#3b82f6',
  natgas:      '#f97316',
  lng:         '#f59e0b',
  lpg:         '#22c55e',
  ngls:        '#84cc16',
  electricity: '#a855f7',
  refined:     '#ef4444',
}

// X = month index * X_SCALE (spread out forward months)
// Y = normalized price (0–100)
// Z = product offset (separates ribbons in depth)
const X_SCALE = 4
const MAX_PTS = 60
const RIBBON_W = 4.0  // visible width of each ribbon in scene units
const Z_SPACING = 5   // distance between ribbon centres along Z

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
      return [monthsDiff * X_SCALE, p.price, zIndex * Z_SPACING] as [number, number, number]
    })
    .sort((a, b) => a[0] - b[0])
}

// ---- ProductRibbon — flat ribbon mesh, ref-update pattern (never remounts) ----
// Each curve is a quad-strip ribbon lying in the XY plane with width RIBBON_W
// along Z. MeshBasicMaterial means no lighting dependency — colour stays crisp.
// Index buffer is pre-filled once; only positions and draw range are updated.
interface ProductCurveProps {
  points: [number, number, number][]
  color: string
}

function ProductCurve({ points, color }: ProductCurveProps) {
  const meshRef = useRef<THREE.Mesh>(null)

  const meshObject = useMemo(() => {
    const geo = new THREE.BufferGeometry()

    // 2 vertices per point (z ± RIBBON_W/2), pre-allocated for MAX_PTS
    geo.setAttribute(
      'position',
      new THREE.BufferAttribute(new Float32Array(MAX_PTS * 2 * 3), 3),
    )

    // Index buffer: 2 triangles per quad, (MAX_PTS-1) quads — filled once, never changes
    const idx = new Uint16Array((MAX_PTS - 1) * 6)
    for (let i = 0; i < MAX_PTS - 1; i++) {
      const b = i * 6, v = i * 2
      idx[b]     = v;     idx[b + 1] = v + 1; idx[b + 2] = v + 2
      idx[b + 3] = v + 1; idx[b + 4] = v + 3; idx[b + 5] = v + 2
    }
    geo.setIndex(new THREE.BufferAttribute(idx, 1))
    geo.setDrawRange(0, 0)

    const mat = new THREE.MeshBasicMaterial({
      color,
      side: THREE.DoubleSide,
      transparent: true,
      opacity: 0.88,
    })
    return new THREE.Mesh(geo, mat)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [color])

  useEffect(() => {
    const mesh = meshRef.current
    if (!mesh || points.length < 2) return

    const pos = mesh.geometry.getAttribute('position') as THREE.BufferAttribute
    const arr = pos.array as Float32Array
    const half = RIBBON_W / 2

    points.forEach(([x, y, z], i) => {
      const lo = (i * 2) * 3
      const hi = (i * 2 + 1) * 3
      arr[lo]     = x; arr[lo + 1] = y; arr[lo + 2] = z - half
      arr[hi]     = x; arr[hi + 1] = y; arr[hi + 2] = z + half
    })

    pos.needsUpdate = true
    mesh.geometry.setDrawRange(0, (points.length - 1) * 6)
    mesh.geometry.computeBoundingSphere()
  }, [points])

  return <primitive ref={meshRef} object={meshObject} />
}

// ---- main component ----
interface EnergyCurve3DProps {
  prices: AllPrices
  visibleSectors: Set<string>
}

export default function EnergyCurve3D({ prices, visibleSectors }: EnergyCurve3DProps) {
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

      const allPrices = pts.map((p) => p.price).filter((v) => v > 0)
      const sectorMin = Math.min(...allPrices)
      const sectorMax = Math.max(...allPrices)
      const sectorRange = sectorMax - sectorMin || 1

      const bySymbol = new Map<string, PricePoint[]>()
      pts.forEach((p) => {
        const existing = bySymbol.get(p.symbol) ?? []
        existing.push(p)
        bySymbol.set(p.symbol, existing)
      })

      for (const [symbol, symbolPts] of bySymbol) {
        const normalized = symbolPts.map((p) => ({
          ...p,
          price: ((p.price - sectorMin) / sectorRange) * 80 + 10,
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
    <div style={{ position: 'relative', width: '100%', height: '100%' }}>
      <Canvas
        camera={{ position: [30, 70, 200], fov: 65 }}
        style={{ background: '#0a0e1a' }}
      >
        {curves.map(({ key, points, color }) => (
          <ProductCurve key={key} points={points} color={color} />
        ))}

        <gridHelper args={[200, 40, '#1e293b', '#1e293b']} position={[30, 0, 35]} />

        <OrbitControls
          enablePan
          enableZoom
          enableRotate
          target={[24, 35, 35]}
          minDistance={10}
          maxDistance={600}
        />
      </Canvas>
    </div>
  )
}
