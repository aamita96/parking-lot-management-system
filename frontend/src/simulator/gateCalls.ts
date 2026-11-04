import { ApiError, api } from '@/api/client'
import type { EntryResponse, ExitResponse, Receipt } from '@/api/types'
import { errorMessage } from '@/lib/errors'
import { refreshLotData } from '@/lib/queryClient'
import { newIdempotencyKey } from '@/lib/utils'
import { useSim } from './store'

/*
 * The only three moments the simulator talks to the server (SRS 11.5):
 *   1. a vehicle reaches the IN gate      → scanAtEntry()  → POST /gate/entry-scans
 *   2. a vehicle reaches the PAY kiosk    → scanAtExit()   → POST /gate/exit-scans
 *   3. "Pay" is pressed                   → pay()          → POST /gate/payments
 * The simulator pretends to be the gate devices SIM-IN and SIM-OUT (X-Gate-Id header),
 * using the admin's login.
 */

function callGate<T>(path: string, gate: 'SIM-IN' | 'SIM-OUT', body: unknown, idempotencyKey: string) {
  return api<T>(path, {
    method: 'POST',
    auth: 'admin',
    headers: { 'X-Gate-Id': gate, 'Idempotency-Key': idempotencyKey },
    body,
  })
}

/**
 * Each call gets an Idempotency-Key. If the same call is retried (e.g. after a network
 * error) the same key is sent again, so the server never does the work twice.
 */
function keyFor(vehicleId: string, call: 'entry' | 'exit' | 'pay'): string {
  const vehicle = useSim.getState().vehicles.find((v) => v.id === vehicleId)
  const key = vehicle?.idempotencyKeys[call] ?? newIdempotencyKey()
  useSim.getState().setIdempotencyKey(vehicleId, call, key)
  return key
}

export async function scanAtEntry(vehicleId: string) {
  const sim = useSim.getState()
  const vehicle = sim.vehicles.find((v) => v.id === vehicleId)
  if (!vehicle) return
  try {
    const answer = await callGate<EntryResponse>('/gate/entry-scans', 'SIM-IN', { plate: vehicle.plate }, keyFor(vehicleId, 'entry'))
    useSim.getState().admitted(vehicleId, answer)
  } catch (err) {
    if (err instanceof ApiError && err.body.result === 'REJECTED') {
      // The gate said no (not registered, no free spot, ...): show why.
      useSim.getState().refused({
        kind: 'refused',
        vehicleId,
        stage: 'entry',
        code: err.code,
        message: errorMessage(err),
        plate: vehicle.plate,
        registerUrl: err.body.registerUrl,
      })
    } else {
      // Network or server problem: the vehicle waits at the gate and the popup offers Retry.
      useSim.getState().showError({ kind: 'error', vehicleId, stage: 'entry', message: errorMessage(err) })
    }
  }
  refreshLotData()
}

export async function scanAtExit(vehicleId: string) {
  const vehicle = useSim.getState().vehicles.find((v) => v.id === vehicleId)
  if (!vehicle) return
  // A chosen stay ("3 h") makes the server pretend the vehicle stayed that long (FR-44).
  const body = vehicle.stayMinutes != null && vehicle.ticket?.isSimulated ? { plate: vehicle.plate, simulatedStayMinutes: vehicle.stayMinutes } : { plate: vehicle.plate }
  try {
    const answer = await callGate<ExitResponse>('/gate/exit-scans', 'SIM-OUT', body, keyFor(vehicleId, 'exit'))
    useSim.getState().showPayment(vehicleId, answer)
  } catch (err) {
    if (err instanceof ApiError && err.body.result === 'REJECTED') {
      useSim.getState().refused({ kind: 'refused', vehicleId, stage: 'exit', code: err.code, message: errorMessage(err), plate: vehicle.plate })
    } else {
      useSim.getState().showError({ kind: 'error', vehicleId, stage: 'exit', message: errorMessage(err) })
    }
  }
  refreshLotData()
}

export async function pay(vehicleId: string, ticketId: string, amountPaise: number, method: 'SIMULATED_UPI' | 'SIMULATED_CASH') {
  try {
    await callGate<Receipt>('/gate/payments', 'SIM-OUT', { ticketId, amountPaise, method }, keyFor(vehicleId, 'pay'))
  } catch (err) {
    // Already paid (e.g. Pay pressed twice): that's fine, the vehicle can go.
    if (!(err instanceof ApiError && err.code === 'TICKET_ALREADY_CLOSED')) throw err
  }
  useSim.getState().driveOut(vehicleId)
  refreshLotData()
}

const STATE_CODES = ['AP', 'BR', 'DL', 'GJ', 'HR', 'KA', 'KL', 'MH', 'MP', 'PB', 'RJ', 'TN', 'TS', 'UP', 'WB']
const LETTERS = 'ABCDEFGHJKLMNPRSTUVWXYZ'

function pick<T>(list: readonly T[]): T {
  return list[Math.floor(Math.random() * list.length)]
}

/** A made-up but valid-looking number, so the gate refuses it as NOT_REGISTERED (FR-42). */
export function sendUnregisteredVehicle() {
  const digits = (count: number) => String(Math.floor(Math.random() * 10 ** count)).padStart(count, '0')
  const plate = `${pick(STATE_CODES)}${digits(2)}${pick([...LETTERS])}${pick([...LETTERS])}${digits(4)}`
  useSim.getState().addVehicle(plate, 'CAR', plate)
}

/** Sends a random registered vehicle that is not already in the lot (mostly), or now and then an unknown one. */
export function sendRandomVehicle() {
  const sim = useSim.getState()
  if (sim.entryQueue.length >= 4) return // don't let the queue grow forever

  const inLot = new Set(sim.vehicles.map((v) => v.plate))
  for (const row of sim.layout?.rows ?? []) {
    for (const spot of row.spots) if (spot.vehicle) inLot.add(spot.vehicle.vehicleNumber)
  }
  const available = sim.registeredVehicles.filter((v) => !inLot.has(v.vehicleNumber))

  if (available.length === 0 || Math.random() < 0.08) {
    sendUnregisteredVehicle() // shows the rejection path now and then
    return
  }
  const vehicle = pick(available)
  sim.addVehicle(vehicle.vehicleNumber, vehicle.vehicleType, vehicle.id.slice(0, 8))
}
