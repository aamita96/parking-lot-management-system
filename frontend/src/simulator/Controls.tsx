import { Ban, CarFront, FastForward, RefreshCcw, Search, Shuffle } from 'lucide-react'
import { useState } from 'react'
import { VehicleIcon } from '@/components/brand/VehicleArt'
import { Button, Input, Modal, ModalFooter, Switch } from '@/components/ui'
import { vehicleTypeLabel } from '@/lib/format'
import { cn } from '@/lib/utils'
import { sendRandomVehicle, sendUnregisteredVehicle } from './gateCalls'
import { useSim } from './store'
import type { Speed } from './types'

// The button bar above the lot (SRS 11.4).
export function Controls({ onReset, resetting }: { onReset: () => void; resetting: boolean }) {
  const autoMode = useSim((s) => s.autoMode)
  const autoEverySeconds = useSim((s) => s.autoEverySeconds)
  const speed = useSim((s) => s.speed)
  const floor = useSim((s) => s.floor)
  const floors = useSim((s) => s.layout?.floors)
  const [choosing, setChoosing] = useState(false)
  const [confirmReset, setConfirmReset] = useState(false)

  return (
    <div className="flex flex-wrap items-center gap-2 rounded-xl border border-slate-200 bg-white p-3 shadow-card">
      <Button onClick={() => setChoosing(true)}>
        <CarFront /> Send vehicle
      </Button>
      <Button variant="secondary" onClick={sendUnregisteredVehicle}>
        <Ban /> Unregistered plate
      </Button>

      <span className="mx-1 h-6 w-px bg-slate-200" aria-hidden />

      <Switch label="Auto mode" checked={autoMode} onChange={(on) => useSim.getState().setAutoMode(on)} />
      <span className="text-sm font-medium text-slate-700">Auto every</span>
      <Input
        type="number"
        min={2}
        max={120}
        value={autoEverySeconds}
        onChange={(e) => useSim.getState().setAutoEverySeconds(Number(e.target.value) || 8)}
        className="h-8 w-16 px-2 text-center"
        aria-label="Seconds between auto vehicles"
      />
      <span className="text-sm text-slate-600">s</span>

      <div className="flex items-center rounded-lg bg-slate-100 p-0.5" role="radiogroup" aria-label="Animation speed">
        <FastForward className="mx-1.5 size-4 text-slate-500" aria-hidden />
        {([1, 2, 4] as Speed[]).map((option) => (
          <button
            key={option}
            role="radio"
            aria-checked={speed === option}
            onClick={() => useSim.getState().setSpeed(option)}
            className={cn('rounded-md px-2.5 py-1 text-xs font-bold', speed === option ? 'bg-white text-slate-900 shadow-sm' : 'text-slate-500')}
          >
            {option}×
          </button>
        ))}
      </div>

      {floors && floors.length > 1 && (
        <select
          aria-label="Floor"
          value={floor}
          onChange={(e) => useSim.getState().setFloor(Number(e.target.value))}
          className="h-9 rounded-lg border border-slate-300 bg-white px-2 text-sm"
        >
          {floors.map((f) => (
            <option key={f} value={f}>
              Floor {f}
            </option>
          ))}
        </select>
      )}

      <span className="flex-1" />
      <Button variant="ghost" className="text-red-600 hover:bg-red-50" onClick={() => setConfirmReset(true)}>
        <RefreshCcw /> Reset
      </Button>

      {choosing && <ChooseVehicle onClose={() => setChoosing(false)} />}
      <Modal
        open={confirmReset}
        onClose={() => setConfirmReset(false)}
        title="Reset the simulator?"
        description="Closes all simulator tickets and frees their spots. Real tickets are not touched. No emails are sent."
        icon={<RefreshCcw />}
        color="danger"
      >
        <ModalFooter>
          <Button variant="secondary" onClick={() => setConfirmReset(false)}>
            Cancel
          </Button>
          <Button
            variant="danger"
            loading={resetting}
            onClick={() => {
              onReset()
              setConfirmReset(false)
            }}
          >
            Reset simulator
          </Button>
        </ModalFooter>
      </Modal>
    </div>
  )
}

/** "Send vehicle": pick a registered vehicle (searchable), or a random one (FR-42). */
function ChooseVehicle({ onClose }: { onClose: () => void }) {
  const registered = useSim((s) => s.registeredVehicles)
  const simVehicles = useSim((s) => s.vehicles)
  const [search, setSearch] = useState('')

  const inLot = new Set(simVehicles.map((v) => v.plate))
  const term = search.toUpperCase().replace(/[\s-]/g, '')
  const choices = registered
    .filter((v) => !inLot.has(v.vehicleNumber))
    .filter((v) => term === '' || v.vehicleNumber.includes(term) || v.ownerEmail.toUpperCase().includes(term))
    .slice(0, 60)

  function send(plate: string, type: (typeof registered)[number]['vehicleType'], id: string) {
    useSim.getState().addVehicle(plate, type, id.slice(0, 8))
    onClose()
  }

  return (
    <Modal open onClose={onClose} title="Send a vehicle to the IN gate" icon={<CarFront />}>
      <div className="relative">
        <Search className="absolute left-2.5 top-2.5 size-4 text-slate-400" aria-hidden />
        <Input autoFocus value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Search number or email" className="pl-8" aria-label="Search vehicles" />
      </div>
      <button
        className="mt-2 flex w-full items-center gap-2 rounded-lg px-3 py-2.5 text-left text-sm font-semibold text-brand-800 hover:bg-brand-50"
        onClick={() => {
          sendRandomVehicle()
          onClose()
        }}
      >
        <Shuffle className="size-4" /> Random registered vehicle
      </button>
      <ul className="mt-1 max-h-72 overflow-y-auto border-t border-slate-100" aria-label="Registered vehicles">
        {choices.length === 0 && <li className="px-3 py-4 text-center text-sm text-slate-500">No free vehicles to send. Register more on /register.</li>}
        {choices.map((v) => (
          <li key={v.id}>
            <button className="flex w-full items-center gap-3 px-3 py-2 text-left hover:bg-slate-50" onClick={() => send(v.vehicleNumber, v.vehicleType, v.id)}>
              <VehicleIcon type={v.vehicleType} hue={215} size={32} />
              <span className="min-w-0 flex-1">
                <span className="block font-mono text-sm font-bold">{v.vehicleNumber}</span>
                <span className="block truncate text-xs text-slate-500">{v.ownerEmail}</span>
              </span>
              <span className="text-xs text-slate-400">{vehicleTypeLabel[v.vehicleType]}</span>
            </button>
          </li>
        ))}
      </ul>
    </Modal>
  )
}
