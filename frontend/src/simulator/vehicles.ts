import type { Layout, LayoutVehicle, VehicleType } from '@/api/types'
import { seedHue } from '@/lib/utils'
import { entrySlot, type Geometry } from './geometry'
import type { SimTicket, SimVehicle } from './types'

// Creating simulated vehicles, and keeping the drawing in step with the server.

let lastId = 0
function newVehicleId() {
  lastId += 1
  return `v${lastId}`
}

const FACING_UP = -Math.PI / 2

/** A vehicle that appears below the IN gate and joins the back of the queue. */
export function arrivingVehicle(plate: string, vehicleType: VehicleType, colorSeed: string, geometry: Geometry, placeInQueue: number): SimVehicle {
  const slot = entrySlot(geometry, placeInQueue)
  return {
    id: newVehicleId(),
    plate,
    vehicleType,
    hue: seedHue(colorSeed),
    state: 'TO_ENTRY', // driving to its place in the queue (place 0 = the gate itself)
    x: slot.x,
    y: Math.max(geometry.spawnY, slot.y + 80), // start just off the bottom of the drawing
    heading: FACING_UP,
    path: [slot],
    nextPoint: 0,
    idempotencyKeys: {},
  }
}

function ticketOf(v: LayoutVehicle): SimTicket {
  return { id: v.ticketId, ticketNumber: v.ticketNumber, entryTime: v.entryTime, rate: v.rate, isSimulated: v.isSimulated }
}

/**
 * Makes the drawing match the server's layout (polled every 2 s, FR-49):
 *  - a spot that is occupied on the server but has no vehicle drawn gets a parked vehicle
 *    (it came in through a real gate or an admin check-in);
 *  - a vehicle drawn as parked whose ticket was closed elsewhere is removed.
 * Vehicles that are driving are left alone; the animation owns them until they stop.
 */
export function syncWithServer(vehicles: SimVehicle[], layout: Layout, floor: number, geometry: Geometry): SimVehicle[] {
  // Which plate is parked in which spot on this floor, according to the server.
  const parkedOnServer = new Map<string, { spotCode: string; vehicle: LayoutVehicle }>()
  for (const row of layout.rows) {
    if (row.floor !== floor) continue
    for (const spot of row.spots) {
      if (spot.vehicle) parkedOnServer.set(spot.vehicle.vehicleNumber, { spotCode: spot.code, vehicle: spot.vehicle })
    }
  }

  const result: SimVehicle[] = []
  for (const v of vehicles) {
    if (v.state !== 'PARKED') {
      result.push(v)
      continue
    }
    const onServer = parkedOnServer.get(v.plate)
    if (!onServer) {
      // Keep vehicles that parked in the last few seconds: this answer may be older than their entry.
      if (Date.now() - (v.parkedAt ?? 0) < 6000) result.push(v)
      continue
    }
    const bay = geometry.spots.get(onServer.spotCode)
    result.push(bay ? { ...v, spotCode: onServer.spotCode, x: bay.cx, y: bay.cy, heading: bay.heading, ticket: ticketOf(onServer.vehicle) } : v)
  }

  const drawn = new Set(result.map((v) => v.plate))
  for (const [plate, { spotCode, vehicle }] of parkedOnServer) {
    const bay = geometry.spots.get(spotCode)
    if (drawn.has(plate) || !bay) continue
    result.push({
      id: newVehicleId(),
      plate,
      vehicleType: vehicle.vehicleType,
      hue: seedHue(vehicle.colorSeed),
      state: 'PARKED',
      x: bay.cx,
      y: bay.cy,
      heading: bay.heading,
      path: [],
      nextPoint: 0,
      ticket: ticketOf(vehicle),
      spotCode,
      spotFloor: floor,
      parkedAt: Date.now(),
      idempotencyKeys: {},
    })
  }
  return result
}
