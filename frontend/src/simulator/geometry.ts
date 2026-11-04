import type { LayoutRow, LayoutSpot } from '@/api/types'

// Lot geometry (SRS 11.1, 11.5). Rows pair up into "bands" facing a shared middle lane:
// row A above the lane (bays open downward), row B below it (bays open upward), and so on.
// A one-way loop runs: in at bottom-left, up the left lane, right along a band lane, down the
// right lane to the PAY kiosk and out. Every spot maps to a route from fixed lane waypoints.

export type Point = { x: number; y: number; reverse?: boolean }

export const SPOT_W = 66
export const SPOT_D = 104
export const LANE_H = 92
export const SIDE_LANE_W = 84
export const BAND_GAP = 56
export const QUEUE_GAP = 64

export type SpotGeo = {
  spot: LayoutSpot
  rowLabel: string
  band: number
  top: boolean // true: bay opens downward to the lane below it
  x: number // left
  y: number // top
  cx: number
  cy: number // parked centre
  heading: number // parked heading (radians)
}

export type Band = { index: number; y: number; laneY: number; rows: string[] }

export type Geometry = {
  width: number
  height: number
  leftX: number
  rightX: number
  spotsX0: number
  spotsX1: number
  lotTop: number
  lotBottom: number
  gateStopY: number // vehicle centre when standing at the IN kiosk
  barrierInY: number
  payStopY: number
  barrierOutY: number
  spawnY: number
  exitY: number
  bands: Band[]
  spots: Map<string, SpotGeo> // by spot code
}

const UP = -Math.PI / 2
const DOWN = Math.PI / 2

export function buildGeometry(rows: LayoutRow[]): Geometry {
  const sorted = [...rows].sort((a, b) => a.rowLabel.localeCompare(b.rowLabel))
  const maxPos = Math.max(6, ...sorted.flatMap((r) => r.spots.map((s) => s.position)))
  const margin = 70
  const leftX = margin + SIDE_LANE_W / 2
  const spotsX0 = margin + SIDE_LANE_W + 8
  const spotsX1 = spotsX0 + maxPos * SPOT_W
  const rightX = spotsX1 + 8 + SIDE_LANE_W / 2
  const width = rightX + SIDE_LANE_W / 2 + margin
  const lotTop = 120

  const bands: Band[] = []
  const spots = new Map<string, SpotGeo>()
  const bandCount = Math.max(1, Math.ceil(sorted.length / 2))
  for (let b = 0; b < bandCount; b++) {
    const y = lotTop + b * (2 * SPOT_D + LANE_H + BAND_GAP)
    const laneY = y + SPOT_D + LANE_H / 2
    const pair = sorted.slice(b * 2, b * 2 + 2)
    bands.push({ index: b, y, laneY, rows: pair.map((r) => r.rowLabel) })
    pair.forEach((row, i) => {
      const top = i === 0
      const rowY = top ? y : y + SPOT_D + LANE_H
      for (const s of row.spots) {
        const x = spotsX0 + (s.position - 1) * SPOT_W
        spots.set(s.code, {
          spot: s,
          rowLabel: row.rowLabel,
          band: b,
          top,
          x,
          y: rowY,
          cx: x + SPOT_W / 2,
          cy: rowY + SPOT_D / 2 + (top ? -4 : 4),
          heading: top ? UP : DOWN,
        })
      }
    })
  }
  const last = bands[bands.length - 1]
  const lotBottom = last.y + 2 * SPOT_D + LANE_H + 16
  const barrierInY = lotBottom + 26
  const gateStopY = barrierInY + 44
  const height = gateStopY + 150
  return {
    width,
    height,
    leftX,
    rightX,
    spotsX0,
    spotsX1,
    lotTop,
    lotBottom,
    gateStopY,
    barrierInY,
    payStopY: gateStopY - 6,
    barrierOutY: gateStopY + 40,
    spawnY: height + 70,
    exitY: height + 90,
    bands,
    spots,
  }
}

/** Queue slot below the IN gate (slot 0 = at the ticket kiosk). */
export function entrySlot(g: Geometry, i: number): Point {
  return { x: g.leftX, y: g.gateStopY + i * QUEUE_GAP }
}

/** Queue slot above the PAY kiosk (slot 0 = at the kiosk). */
export function paySlot(g: Geometry, i: number): Point {
  return { x: g.rightX, y: g.payStopY - i * QUEUE_GAP }
}

/** IN gate → up the left lane → along the band lane → into the bay. */
export function pathToSpot(g: Geometry, code: string): Point[] {
  const s = g.spots.get(code)
  if (!s) return pathToRamp(g)
  const laneY = g.bands[s.band].laneY
  return smooth([
    { x: g.leftX, y: g.gateStopY },
    { x: g.leftX, y: laneY },
    { x: s.cx, y: laneY },
    { x: s.cx, y: s.cy },
  ]).slice(1)
}

/** For spots on another floor: drive to the top-right ramp and leave the drawing. */
export function pathToRamp(g: Geometry): Point[] {
  const laneY = g.bands[0].laneY
  return smooth([
    { x: g.leftX, y: g.gateStopY },
    { x: g.leftX, y: laneY },
    { x: g.rightX, y: laneY },
    { x: g.rightX, y: g.lotTop - 90 },
  ]).slice(1)
}

/** Bay → reverse out → along the lane → down the right lane to a PAY queue slot. */
export function pathToPay(g: Geometry, code: string | undefined, slot: Point): Point[] {
  const s = code ? g.spots.get(code) : undefined
  if (!s) {
    // Not on this floor (or unknown): appear from the ramp at the top of the right lane.
    return [{ x: g.rightX, y: g.lotTop - 60 }, slot]
  }
  const laneY = g.bands[s.band].laneY
  const out: Point[] = [{ x: s.cx, y: laneY, reverse: true }]
  const fwd = smooth([
    { x: s.cx, y: laneY },
    { x: g.rightX, y: laneY },
    { x: g.rightX, y: slot.y },
  ]).slice(1)
  return [...out, ...fwd]
}

export function pathToExit(g: Geometry): Point[] {
  return [{ x: g.rightX, y: g.exitY }]
}

/** Rejected at IN: back up a little, then drive off to the left. */
export function pathTurnBack(g: Geometry): Point[] {
  const y = g.gateStopY + 46
  return [{ x: g.leftX, y, reverse: true }, ...smooth([{ x: g.leftX, y }, { x: g.leftX - 40, y: y + 20 }, { x: -120, y: y + 24 }]).slice(1)]
}

/** Rounds right-angle corners with a sampled quadratic curve so turns look natural. */
export function smooth(points: Point[], radius = 26, samples = 6): Point[] {
  if (points.length < 3) return points
  const out: Point[] = [points[0]]
  for (let i = 1; i < points.length - 1; i++) {
    const p0 = points[i - 1]
    const p = points[i]
    const p1 = points[i + 1]
    const d0 = Math.hypot(p.x - p0.x, p.y - p0.y)
    const d1 = Math.hypot(p1.x - p.x, p1.y - p.y)
    const r = Math.min(radius, d0 / 2, d1 / 2)
    if (r < 2 || p.reverse || p1.reverse) {
      out.push(p)
      continue
    }
    const a = { x: p.x - ((p.x - p0.x) / d0) * r, y: p.y - ((p.y - p0.y) / d0) * r }
    const b = { x: p.x + ((p1.x - p.x) / d1) * r, y: p.y + ((p1.y - p.y) / d1) * r }
    for (let k = 0; k <= samples; k++) {
      const t = k / samples
      const u = 1 - t
      out.push({ x: u * u * a.x + 2 * u * t * p.x + t * t * b.x, y: u * u * a.y + 2 * u * t * p.y + t * t * b.y })
    }
  }
  out.push(points[points.length - 1])
  return out
}

export function angleTo(from: { x: number; y: number }, to: { x: number; y: number }): number {
  return Math.atan2(to.y - from.y, to.x - from.x)
}

/** Shortest signed difference between two angles. */
export function angleDiff(a: number, b: number): number {
  let d = b - a
  while (d > Math.PI) d -= 2 * Math.PI
  while (d < -Math.PI) d += 2 * Math.PI
  return d
}
