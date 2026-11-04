import { create } from 'zustand'
import type { EntryResponse, ExitResponse, Layout, Vehicle, VehicleType } from '@/api/types'
import { buildGeometry, entrySlot, type Geometry, pathToExit, pathToPay, pathToRamp, pathToSpot, pathTurnBack, paySlot } from './geometry'
import { moveAlongPath } from './movement'
import type { Popup, SimVehicle, Speed } from './types'
import { arrivingVehicle, syncWithServer } from './vehicles'

/*
 * The simulator's memory (a Zustand store). Components read it with
 *     const vehicles = useSim((s) => s.vehicles)
 * and other code with useSim.getState().
 *
 * How it fits together:
 *   - The page calls tick() about 60 times a second. tick() moves every vehicle a little and
 *     reports which vehicles just reached the IN gate or the PAY kiosk.
 *   - The page then calls the gate API for those vehicles (see gateCalls.ts), which in turn
 *     calls the actions below (admitted, refused, showPayment, ...) with the server's answer.
 *   - Popups (entry, refused, payment) wait in `popups`; GatePopups.tsx shows the first one.
 *   - Only the server decides tickets and spots; this store only animates.
 */

type TickResult = {
  arrivedAtEntry: string[] // vehicle ids that just stopped at the IN gate
  arrivedAtPay: string[] // vehicle ids that just stopped at the PAY kiosk
  autoVehicleDue: boolean // auto mode: time to send another vehicle
}

type SimState = {
  layout: Layout | null
  geometry: Geometry | null
  floor: number
  vehicles: SimVehicle[]
  entryQueue: string[] // vehicle ids waiting for the IN gate; the first one is at the gate
  payQueue: string[] // vehicle ids waiting for the PAY kiosk; the first one is at the kiosk
  popups: Popup[]
  inBarrierOpenUntil: number // Date.now() time until which the barrier stays up
  outBarrierOpenUntil: number
  now: number
  speed: Speed
  autoMode: boolean
  autoEverySeconds: number
  secondsSinceAutoVehicle: number
  selectedVehicleId: string | null // parked vehicle whose ticket card is open
  registeredVehicles: Vehicle[] // used by "Random" and auto mode

  setLayout: (layout: Layout) => void
  setFloor: (floor: number) => void
  setSpeed: (speed: Speed) => void
  setAutoMode: (on: boolean) => void
  setAutoEverySeconds: (seconds: number) => void
  setRegisteredVehicles: (vehicles: Vehicle[]) => void
  selectVehicle: (id: string | null) => void

  addVehicle: (plate: string, vehicleType: VehicleType, colorSeed: string) => boolean
  tick: (milliseconds: number) => TickResult

  admitted: (vehicleId: string, answer: EntryResponse) => void
  refused: (popup: Extract<Popup, { kind: 'refused' }>) => void
  showPayment: (vehicleId: string, answer: ExitResponse) => void
  showError: (popup: Extract<Popup, { kind: 'error' }>) => void
  closePopup: (vehicleId: string) => void

  driveToSpot: (vehicleId: string) => void
  turnBack: (vehicleId: string) => void
  leave: (vehicleId: string, stayMinutes: number | null) => void
  driveOut: (vehicleId: string) => void
  removeVehicle: (vehicleId: string) => void
  setIdempotencyKey: (vehicleId: string, call: 'entry' | 'exit' | 'pay', key: string) => void
  reset: () => void
}

const FACING_UP = -Math.PI / 2
const FACING_DOWN = Math.PI / 2
const RANDOM_STAYS = [null, 25, 45, 90, 200, 330, 1570] // minutes; null = the real time parked

export const useSim = create<SimState>()((set, get) => ({
  layout: null,
  geometry: null,
  floor: 0,
  vehicles: [],
  entryQueue: [],
  payQueue: [],
  popups: [],
  inBarrierOpenUntil: 0,
  outBarrierOpenUntil: 0,
  now: Date.now(),
  speed: 1,
  autoMode: false,
  autoEverySeconds: 8,
  secondsSinceAutoVehicle: 0,
  selectedVehicleId: null,
  registeredVehicles: [],

  // ---------- settings ----------

  /** Called with every layout poll. Rebuilds the drawing and syncs parked vehicles. */
  setLayout: (layout) => {
    const { floor, vehicles } = get()
    const geometry = buildGeometry(layout.rows.filter((row) => row.floor === floor))
    set({ layout, geometry, vehicles: syncWithServer(vehicles, layout, floor, geometry) })
  },
  setFloor: (floor) => {
    set({ floor })
    const layout = get().layout
    if (layout) get().setLayout(layout)
  },
  setSpeed: (speed) => set({ speed }),
  setAutoMode: (autoMode) => set({ autoMode, secondsSinceAutoVehicle: 0 }),
  setAutoEverySeconds: (seconds) => set({ autoEverySeconds: Math.min(120, Math.max(2, seconds)) }),
  setRegisteredVehicles: (registeredVehicles) => set({ registeredVehicles }),
  selectVehicle: (selectedVehicleId) => set({ selectedVehicleId }),

  // ---------- the animation ----------

  /** Adds a vehicle at the back of the IN queue. Returns false if it is already in the lot. */
  addVehicle: (plate, vehicleType, colorSeed) => {
    const { geometry, vehicles, entryQueue } = get()
    if (!geometry || vehicles.some((v) => v.plate === plate)) return false
    const vehicle = arrivingVehicle(plate, vehicleType, colorSeed, geometry, entryQueue.length)
    set({ vehicles: [...vehicles, vehicle], entryQueue: [...entryQueue, vehicle.id] })
    return true
  },

  /** Moves everything forward by `milliseconds` of real time. */
  tick: (milliseconds) => {
    const state = get()
    const result: TickResult = { arrivedAtEntry: [], arrivedAtPay: [], autoVehicleDue: false }
    const geometry = state.geometry
    if (!geometry) return result

    // Cap the step (e.g. after the tab was hidden) and apply the 1×/2×/4× speed.
    const seconds = (Math.min(milliseconds, 100) / 1000) * state.speed
    const now = Date.now()

    const vehicles: SimVehicle[] = []
    for (const original of state.vehicles) {
      // Step 1: vehicles in a queue head for their current place in it.
      const aimed = aimAtQueuePlace(original, state.entryQueue, state.payQueue, geometry)

      // Step 2: drive.
      const { vehicle, arrived } = moveAlongPath(aimed, seconds)
      if (!arrived) {
        vehicles.push(vehicle)
        continue
      }

      // Step 3: it reached the end of its path. What happens next depends on where it was going.
      switch (vehicle.state) {
        case 'TO_ENTRY':
          if (state.entryQueue[0] === vehicle.id) {
            vehicles.push({ ...vehicle, state: 'AT_ENTRY', heading: FACING_UP })
            result.arrivedAtEntry.push(vehicle.id) // the page will call the entry-scan API
          } else {
            vehicles.push({ ...vehicle, state: 'QUEUED' }) // wait behind the vehicle in front
          }
          break
        case 'TO_SPOT': {
          const bay = vehicle.spotCode ? geometry.spots.get(vehicle.spotCode) : undefined
          if (bay && vehicle.spotFloor === state.floor) {
            vehicles.push({ ...vehicle, state: 'PARKED', x: bay.cx, y: bay.cy, heading: bay.heading, parkedAt: now })
          }
          // No bay on this floor: it drove up the ramp, so it leaves this drawing.
          break
        }
        case 'TO_PAY':
          if (state.payQueue[0] === vehicle.id) {
            vehicles.push({ ...vehicle, state: 'AT_PAY', heading: FACING_DOWN })
            result.arrivedAtPay.push(vehicle.id) // the page will call the exit-scan API
          } else {
            vehicles.push(vehicle) // wait in the PAY queue
          }
          break
        case 'EXITING':
        case 'TURNING_BACK':
          break // drove off the bottom of the drawing: remove it
        default:
          vehicles.push(vehicle)
      }
    }

    // Forget queue places of vehicles that are gone.
    const stillHere = new Set(vehicles.map((v) => v.id))
    const entryQueue = state.entryQueue.filter((id) => stillHere.has(id))
    const payQueue = state.payQueue.filter((id) => stillHere.has(id))

    // Auto mode (FR-46): a new vehicle every N seconds; parked simulator vehicles leave after a while.
    let secondsSinceAutoVehicle = state.secondsSinceAutoVehicle
    let leavingId: string | null = null
    if (state.autoMode) {
      secondsSinceAutoVehicle += seconds
      if (secondsSinceAutoVehicle >= state.autoEverySeconds) {
        secondsSinceAutoVehicle = 0
        result.autoVehicleDue = true
      }
      for (let i = 0; i < vehicles.length; i++) {
        const v = vehicles[i]
        if (v.state !== 'PARKED' || !v.ticket?.isSimulated) continue
        if (v.leaveAt === undefined) {
          const waitSeconds = (10 + Math.random() * 25) / state.speed
          vehicles[i] = { ...v, leaveAt: now + waitSeconds * 1000 }
        } else if (v.leaveAt <= now && leavingId === null && state.popups.length === 0) {
          leavingId = v.id
        }
      }
    }

    set({ vehicles, entryQueue, payQueue, now, secondsSinceAutoVehicle })
    if (leavingId) get().leave(leavingId, RANDOM_STAYS[Math.floor(Math.random() * RANDOM_STAYS.length)])
    return result
  },

  // ---------- answers from the gate API ----------

  /** The IN gate let the vehicle in: remember its ticket and show the entry popup. */
  admitted: (vehicleId, answer) =>
    set((s) => ({
      vehicles: updateVehicle(s.vehicles, vehicleId, {
        spotCode: answer.spot.code,
        spotFloor: answer.spot.floor,
        ticket: {
          id: answer.ticket.id,
          ticketNumber: answer.ticket.ticketNumber,
          entryTime: answer.ticket.entryTime,
          rate: answer.rate,
          isSimulated: answer.ticket.isSimulated,
        },
      }),
      popups: [...withoutPopupFor(s.popups, vehicleId), { kind: 'entry', vehicleId, data: answer }],
    })),

  refused: (popup) => set((s) => ({ popups: [...withoutPopupFor(s.popups, popup.vehicleId), popup] })),
  showPayment: (vehicleId, answer) => set((s) => ({ popups: [...withoutPopupFor(s.popups, vehicleId), { kind: 'payment', vehicleId, data: answer }] })),
  showError: (popup) => set((s) => ({ popups: [...withoutPopupFor(s.popups, popup.vehicleId), popup] })),
  closePopup: (vehicleId) => set((s) => ({ popups: withoutPopupFor(s.popups, vehicleId) })),

  // ---------- moving a vehicle on ----------

  /** "Park": the IN barrier lifts and the vehicle drives to its bay. */
  driveToSpot: (vehicleId) => {
    const { geometry, floor, vehicles, speed } = get()
    if (!geometry) return
    const vehicle = vehicles.find((v) => v.id === vehicleId)
    const bayOnThisFloor = vehicle?.spotCode && vehicle.spotFloor === floor && geometry.spots.has(vehicle.spotCode)
    set((s) => ({
      popups: withoutPopupFor(s.popups, vehicleId),
      inBarrierOpenUntil: Date.now() + 2600 / speed,
      entryQueue: s.entryQueue.filter((id) => id !== vehicleId),
      vehicles: updateVehicle(s.vehicles, vehicleId, {
        state: 'TO_SPOT',
        path: bayOnThisFloor ? pathToSpot(geometry, vehicle!.spotCode!) : pathToRamp(geometry),
        nextPoint: 0,
      }),
    }))
  },

  /** "Turn back": a refused vehicle reverses and drives away. */
  turnBack: (vehicleId) => {
    const geometry = get().geometry
    if (!geometry) return
    set((s) => ({
      popups: withoutPopupFor(s.popups, vehicleId),
      entryQueue: s.entryQueue.filter((id) => id !== vehicleId),
      vehicles: updateVehicle(s.vehicles, vehicleId, { state: 'TURNING_BACK', path: pathTurnBack(geometry), nextPoint: 0 }),
    }))
  },

  /** "Leave" (FR-44): drive to the PAY queue. The stay is sent with the exit scan. */
  leave: (vehicleId, stayMinutes) => {
    const { geometry, vehicles, payQueue } = get()
    const vehicle = vehicles.find((v) => v.id === vehicleId)
    if (!geometry || !vehicle || vehicle.state !== 'PARKED') return
    const slot = paySlot(geometry, payQueue.length)
    set((s) => ({
      selectedVehicleId: null,
      payQueue: [...s.payQueue, vehicleId],
      vehicles: updateVehicle(s.vehicles, vehicleId, {
        state: 'TO_PAY',
        stayMinutes,
        path: pathToPay(geometry, vehicle.spotCode, slot),
        nextPoint: 0,
        leaveAt: undefined,
        idempotencyKeys: {}, // new visit to the kiosk, new keys
      }),
    }))
  },

  /** Paid (or free): the OUT barrier lifts and the vehicle drives away. */
  driveOut: (vehicleId) => {
    const { geometry, speed } = get()
    if (!geometry) return
    set((s) => ({
      popups: withoutPopupFor(s.popups, vehicleId),
      outBarrierOpenUntil: Date.now() + 2600 / speed,
      payQueue: s.payQueue.filter((id) => id !== vehicleId),
      vehicles: updateVehicle(s.vehicles, vehicleId, { state: 'EXITING', path: pathToExit(geometry), nextPoint: 0 }),
    }))
  },

  removeVehicle: (vehicleId) =>
    set((s) => ({
      popups: withoutPopupFor(s.popups, vehicleId),
      entryQueue: s.entryQueue.filter((id) => id !== vehicleId),
      payQueue: s.payQueue.filter((id) => id !== vehicleId),
      vehicles: s.vehicles.filter((v) => v.id !== vehicleId),
    })),

  setIdempotencyKey: (vehicleId, call, key) =>
    set((s) => ({
      vehicles: s.vehicles.map((v) => (v.id === vehicleId ? { ...v, idempotencyKeys: { ...v.idempotencyKeys, [call]: key } } : v)),
    })),

  reset: () => set({ vehicles: [], entryQueue: [], payQueue: [], popups: [], selectedVehicleId: null, secondsSinceAutoVehicle: 0 }),
}))

// ---------- small helpers ----------

function updateVehicle(vehicles: SimVehicle[], vehicleId: string, changes: Partial<SimVehicle>): SimVehicle[] {
  return vehicles.map((v) => (v.id === vehicleId ? { ...v, ...changes } : v))
}

function withoutPopupFor(popups: Popup[], vehicleId: string): Popup[] {
  return popups.filter((p) => p.vehicleId !== vehicleId)
}

/**
 * Queued vehicles always drive towards their current place in the queue. When the vehicle in
 * front leaves, everyone's place moves up by one, so their target changes here.
 */
function aimAtQueuePlace(vehicle: SimVehicle, entryQueue: string[], payQueue: string[], geometry: Geometry): SimVehicle {
  const lastPoint = vehicle.path[vehicle.path.length - 1]

  const entryPlace = entryQueue.indexOf(vehicle.id)
  if (entryPlace >= 0 && (vehicle.state === 'QUEUED' || vehicle.state === 'TO_ENTRY')) {
    const slot = entrySlot(geometry, entryPlace)
    if (!lastPoint || Math.abs(lastPoint.y - slot.y) > 0.5) {
      return { ...vehicle, state: 'TO_ENTRY', path: [slot], nextPoint: 0 }
    }
  }

  const payPlace = payQueue.indexOf(vehicle.id)
  if (payPlace >= 0 && vehicle.state === 'TO_PAY') {
    const slot = paySlot(geometry, payPlace)
    if (lastPoint && Math.abs(lastPoint.y - slot.y) > 0.5) {
      const finished = vehicle.nextPoint >= vehicle.path.length
      // Standing still: drive straight to the new place. Still driving: just move the end point.
      return finished
        ? { ...vehicle, path: [slot], nextPoint: 0 }
        : { ...vehicle, path: [...vehicle.path.slice(0, -1), { ...lastPoint, y: slot.y }] }
    }
  }
  return vehicle
}
