import type { VehicleType } from '@/api/types'
import { angleDiff, angleTo } from './geometry'
import type { SimVehicle } from './types'

const PIXELS_PER_SECOND = 170 // driving speed at 1×
const TURN_PER_SECOND = 7 // how fast a vehicle can turn, in radians per second at 1×

function speedOf(type: VehicleType) {
  if (type === 'MOTORCYCLE') return PIXELS_PER_SECOND * 1.15
  if (type === 'TRUCK') return PIXELS_PER_SECOND * 0.82
  return PIXELS_PER_SECOND
}

/**
 * Moves a vehicle along its path for `seconds` of (already speed-adjusted) time.
 * Returns the moved vehicle, and `arrived: true` if it reached the last point just now.
 * A pure function: it never changes the vehicle you pass in.
 */
export function moveAlongPath(vehicle: SimVehicle, seconds: number): { vehicle: SimVehicle; arrived: boolean } {
  if (vehicle.nextPoint >= vehicle.path.length) return { vehicle, arrived: false } // standing still

  let { x, y, nextPoint } = vehicle
  let distanceLeft = speedOf(vehicle.vehicleType) * seconds
  let wantedHeading = vehicle.heading

  // Drive towards the next point; if we reach it, carry on to the one after.
  while (distanceLeft > 0 && nextPoint < vehicle.path.length) {
    const target = vehicle.path[nextPoint]
    const distance = Math.hypot(target.x - x, target.y - y)
    if (distance > 0.01) {
      // Face the target. When reversing out of a bay, the vehicle faces the other way.
      wantedHeading = angleTo({ x, y }, target) + (target.reverse ? Math.PI : 0)
    }
    if (distance <= distanceLeft) {
      x = target.x
      y = target.y
      distanceLeft -= distance
      nextPoint++
    } else {
      x += ((target.x - x) / distance) * distanceLeft
      y += ((target.y - y) / distance) * distanceLeft
      distanceLeft = 0
    }
  }

  // Turn gradually towards the wanted heading, so corners look smooth.
  const turnNeeded = angleDiff(vehicle.heading, wantedHeading)
  const maxTurn = TURN_PER_SECOND * seconds
  const heading = vehicle.heading + Math.max(-maxTurn, Math.min(maxTurn, turnNeeded))

  return {
    vehicle: { ...vehicle, x, y, heading, nextPoint },
    arrived: nextPoint >= vehicle.path.length,
  }
}
