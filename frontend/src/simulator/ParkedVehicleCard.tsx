import { LogOut, X } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Button } from '@/components/ui'
import { calculateFee, feeBreakdown, minutesSince } from '@/lib/fee'
import { dateTimeIST, humanDuration, rupees, vehicleTypeLabel } from '@/lib/format'
import { useSim } from './store'

// The card that opens when you click a parked vehicle (FR-44): its ticket, running fee and "Leave".

const stayChoices = [
  { label: 'Actual', minutes: null }, // the real time it has been parked
  { label: '30 min', minutes: 30 },
  { label: '3 h', minutes: 180 },
  { label: '26 h', minutes: 1560 },
]

export function ParkedVehicleCard() {
  const vehicle = useSim((s) => s.vehicles.find((v) => v.id === s.selectedVehicleId))
  const geometry = useSim((s) => s.geometry)
  const close = () => useSim.getState().selectVehicle(null)

  // Update the elapsed time and fee every second, and close with the Esc key.
  const [now, setNow] = useState(Date.now())
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000)
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && close()
    window.addEventListener('keydown', onKey)
    return () => {
      clearInterval(timer)
      window.removeEventListener('keydown', onKey)
    }
  }, [])

  if (!vehicle || !geometry || vehicle.state !== 'PARKED' || !vehicle.ticket) return null
  const ticket = vehicle.ticket
  const minutes = minutesSince(ticket.entryTime, now)
  const fee = calculateFee(minutes, ticket.rate)

  // Place the card next to the vehicle (as a % of the drawing), below it for the top half.
  const left = Math.min(85, Math.max(15, (vehicle.x / geometry.width) * 100))
  const top = (vehicle.y / geometry.height) * 100
  const showBelow = vehicle.y < geometry.height / 2

  return (
    <div
      role="dialog"
      aria-label={`Ticket for ${vehicle.plate}`}
      className="absolute z-20 w-72 rounded-xl border border-slate-200 bg-white p-4 shadow-2xl"
      style={{ left: `${left}%`, top: `${top}%`, transform: showBelow ? 'translate(-50%, 40px)' : 'translate(-50%, calc(-100% - 40px))' }}
    >
      <div className="flex items-start justify-between">
        <div>
          <p className="font-mono text-lg font-bold">{vehicle.plate}</p>
          <p className="text-xs text-slate-500">
            {vehicleTypeLabel[vehicle.vehicleType]} · spot {vehicle.spotCode}
          </p>
        </div>
        <button onClick={close} className="rounded p-1 text-slate-400 hover:bg-slate-100" aria-label="Close">
          <X className="size-4" />
        </button>
      </div>

      <dl className="mt-3 space-y-1 text-sm">
        <Row label="Ticket" value={<span className="font-mono text-xs">{ticket.ticketNumber}</span>} />
        <Row label="Entry" value={dateTimeIST(ticket.entryTime)} />
        <Row label="Elapsed" value={humanDuration(minutes)} />
        <Row label="Running fee" value={<strong>{rupees(fee.feePaise)}</strong>} />
      </dl>
      <p className="mt-1 text-[11px] text-slate-400">{feeBreakdown(fee, ticket.rate)}</p>

      {ticket.isSimulated ? (
        <div className="mt-3 border-t border-slate-100 pt-3">
          <p className="mb-2 flex items-center gap-1 text-xs font-semibold text-slate-600">
            <LogOut className="size-3.5" /> Leave with a stay of
          </p>
          <div className="grid grid-cols-4 gap-1.5">
            {stayChoices.map((choice) => (
              <Button
                key={choice.label}
                size="small"
                variant={choice.minutes === null ? 'primary' : 'secondary'}
                onClick={() => useSim.getState().leave(vehicle.id, choice.minutes)}
              >
                {choice.label}
              </Button>
            ))}
          </div>
        </div>
      ) : (
        <p className="mt-3 rounded-lg bg-amber-50 p-2 text-xs text-amber-800">Parked via a real gate or by an admin. Use Tickets → Manual exit.</p>
      )}
    </div>
  )
}

function Row({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex justify-between gap-2">
      <dt className="text-slate-500">{label}</dt>
      <dd>{value}</dd>
    </div>
  )
}
