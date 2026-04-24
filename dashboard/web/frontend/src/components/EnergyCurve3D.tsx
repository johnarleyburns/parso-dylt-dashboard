/**
 * EnergyCurve3D — 3-D forward-curve chart for all energy sectors.
 *
 * IMPORTANT — Agent Failure Case #3 prevention:
 *   The <Canvas> mounts ONCE and is never torn down between data refreshes.
 *   Geometry is updated imperatively via a bufferGeometry ref (useEffect +
 *   setAttribute). React state changes in App.tsx do NOT remount the scene.
 *   An earlier agent version re-created the entire scene on every 30-second
 *   price poll, pegging one CPU core at 100%.
 */

import { useRef, useEffect, useMemo } from 'react'
import { Canvas } from '@react-three/fiber'
import { OrbitControls, Text } from '@react-three/drei'
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

// Months short-label from "YYYY-MM-01"
function monthLabel(deliveryMonth: string): string {
  const d = new Date(deliveryMonth)
  return isNaN(d.getTime()) ? '' : d.toLocaleString('en-US', { month: 'short', year: '2-digit' })
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

// ---- ProductCurve — ref-update pattern (never remounts) ----
interface ProductCurveProps {
  points: [number, number, number][]
  color: string
}

function ProductCurve({ points, color }: ProductCurveProps) {
  const lineRef = useRef<THREE.Line>(null)

  // Allocate line geometry ONCE.
  const lineObject = useMemo(() => {
    const geo = new THREE.BufferGeometry()
    // Pre-allocate for up to 60 months; actual draw range set in effect.
    geo.setAttribute(
      'position',
      new THREE.BufferAttribute(new Float32Array(60 * 3), 3),
    )
    const mat = new THREE.LineBasicMaterial({ color, linewidth: 2 })
    return new THREE.Line(geo, mat)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [color]) // color changes (sector toggle) get a new material — that's intentional

  // Update geometry imperatively when price data changes.
  // This is the key pattern: no React re-render of the scene graph.
  useEffect(() => {
    const line = lineRef.current
    if (!line || points.length === 0) return

    const flat = new Float32Array(points.length * 3)
    points.forEach(([x, y, z], i) => {
      flat[i * 3]     = x
      flat[i * 3 + 1] = y
      flat[i * 3 + 2] = z
    })

    const attr = line.geometry.getAttribute('position') as THREE.BufferAttribute
    attr.set(flat)
    attr.needsUpdate = true
    line.geometry.setDrawRange(0, points.length)
    line.geometry.computeBoundingSphere()
  }, [points])

  return <primitive ref={lineRef} object={lineObject} />
}

// ---- Axis labels ----
function AxisLabel({
  position,
  text,
  fontSize = 0.4,
}: {
  position: [number, number, number]
  text: string
  fontSize?: number
}) {
  return (
    <Text
      position={position}
      fontSize={fontSize}
      color="#94a3b8"
      anchorX="center"
      anchorY="middle"
    >
      {text}
    </Text>
  )
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

      // Group by symbol — each symbol gets its own curve.
      const bySymbol = new Map<string, PricePoint[]>()
      pts.forEach((p) => {
        const existing = bySymbol.get(p.symbol) ?? []
        existing.push(p)
        bySymbol.set(p.symbol, existing)
      })

      for (const [symbol, symbolPts] of bySymbol) {
        curves.push({
          key: `${sector}-${symbol}`,
          points: buildPoints(symbolPts, zIndex, baseMonth),
          color: SECTOR_COLORS[sector] ?? '#94a3b8',
          label: symbol,
        })
        zIndex++
      }
    }
    return result
  }, [prices, visibleSectors, baseMonth])

  // Month tick labels for X axis (at z=0 level)
  const monthLabels = useMemo(() => {
    const labels: Array<{ x: number; text: string }> = []
    for (let i = 0; i <= 12; i++) {
      const d = new Date(baseMonth)
      d.setMonth(d.getMonth() + i)
      labels.push({ x: i, text: monthLabel(d.toISOString()) })
    }
    return labels
  }, [baseMonth])

  return (
    // The Canvas mounts once. Price data changes update geometry via refs — no remount.
    <Canvas
      camera={{ position: [12, 60, 30], fov: 50 }}
      style={{ background: '#0a0e1a' }}
    >
      <ambientLight intensity={0.6} />

      {/* X-axis month labels */}
      {monthLabels.map(({ x, text }) => (
        <AxisLabel key={x} position={[x, -5, -2]} text={text} fontSize={0.35} />
      ))}

      {/* Y-axis label */}
      <AxisLabel position={[-2, 40, 0]} text="Price (USD)" />

      {/* One curve per product symbol */}
      {curves.map(({ key, points, color }) => (
        <ProductCurve key={key} points={points} color={color} />
      ))}

      {/* Grid helpers for orientation */}
      <gridHelper args={[40, 20, '#1e293b', '#1e293b']} position={[18, 0, 20]} />

      <OrbitControls
        enablePan
        enableZoom
        enableRotate
        target={[10, 30, 10]}
        minDistance={5}
        maxDistance={150}
      />
    </Canvas>
  )
}
