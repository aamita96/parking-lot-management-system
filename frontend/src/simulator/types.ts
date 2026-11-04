import type { EntryResponse, ExitResponse, Rate, VehicleType } from '@/api/types'
import type { Point } from './geometry'

/**
 * Where a simulated vehicle is in its journey (SRS 11.5):
 *
 *   QUEUED → TO_ENTRY → AT_ENTRY → TO_SPOT → PARKED → TO_PAY → AT_PAY → EXITING → (removed)
 *                           └──→ TURNING_BACK → (removed)        when the gate refuses it
 */
export type VehicleState = 'QUEUED' | 'TO_ENTRY' | 'AT_ENTRY' | 'TO_SPOT' | 'PARKED' | 'TO_PAY' | 'AT_PAY' | 'EXITING' | 'TURNING_BACK'

export type SimTicket = {
  id: string
  ticketNumber: string
  entryTime: string
  rate: Rate
  isSimulated: boolean
}

export type SimVehicle = {
  id: string // local id for React, e.g. "v12"
  plate: string
  vehicleType: VehicleType
  hue: number // colour, fixed per vehicle
  state: VehicleState

  // Position on the drawing and the direction the vehicle faces (radians).
  x: number
  y: number
  heading: number

  // The route it is driving: a list of points, and which one it is heading to next.
  path: Point[]
  nextPoint: number

  ticket?: SimTicket // set once the gate admits it
  spotCode?: string
  spotFloor?: number
  stayMinutes?: number | null // chosen "Leave" stay; null = the real time parked
  parkedAt?: number // Date.now() when it parked
  leaveAt?: number // auto mode: Date.now() when it should leave

  // Idempotency keys for gate calls; reused if the same call is retried.
  idempotencyKeys: { entry?: string; exit?: string; pay?: string }
}

/** A popup waiting to be shown. Only one shows at a time. */
export type Popup =
  | { kind: 'entry'; vehicleId: string; data: EntryResponse }
  | { kind: 'refused'; vehicleId: string; stage: 'entry' | 'exit'; code: string; message: string; plate: string; registerUrl?: string }
  | { kind: 'payment'; vehicleId: string; data: ExitResponse }
  | { kind: 'error'; vehicleId: string; stage: 'entry' | 'exit'; message: string }

export type Speed = 1 | 2 | 4
