import type { Panel } from '@/api/types'
import { VehicleIcon } from '@/components/brand/VehicleArt'
import { Badge, Card } from '@/components/ui'
import { spotTypeLabel } from '@/lib/format'
import { cn } from '@/lib/utils'

const panelHue: Record<string, number> = { MOTORCYCLE: 28, CAR: 215, TRUCK: 150 }

// One availability panel per vehicle type (FR-10, FR-11, FR-13). "Full" is shown as text, not
// only colour (WCAG).
export function AvailabilityPanel({ panel, showCodes = true }: { panel: Panel; showCodes?: boolean }) {
  const used = panel.total > 0 ? Math.round((panel.occupied / panel.total) * 100) : 0
  const tone = panel.full ? 'red' : panel.available <= Math.max(1, Math.floor(panel.total * 0.2)) ? 'amber' : 'green'
  const home = panel.freeSpots.filter((s) => !s.overflow)
  const overflow = panel.freeSpots.filter((s) => s.overflow)
  return (
    <Card className={cn('flex flex-col overflow-hidden', panel.full && 'ring-1 ring-red-200')}>
      <div className="flex items-center gap-4 px-5 pt-5">
        <div className="grid size-16 shrink-0 place-items-center rounded-2xl bg-slate-100">
          <VehicleIcon type={panel.vehicleType} hue={panelHue[panel.vehicleType]} size={56} />
        </div>
        <div className="min-w-0 flex-1">
          <h3 className="text-lg font-bold text-slate-900">{panel.label}</h3>
          <p className="text-xs text-slate-500">{spotTypeLabel[panel.spotType]} spots</p>
        </div>
        {panel.full ? (
          <Badge color="red" className="text-sm">
            Full
          </Badge>
        ) : (
          <Badge color={tone}>Open</Badge>
        )}
      </div>

      <div className="px-5 pt-4">
        <p className="flex items-baseline gap-1.5" aria-live="polite">
          <span className={cn('tabular text-4xl font-extrabold', panel.full ? 'text-red-600' : 'text-slate-900')}>{panel.available}</span>
          <span className="tabular text-sm font-medium text-slate-500">/ {panel.total} available</span>
        </p>
        <div
          className="mt-3 h-2.5 overflow-hidden rounded-full bg-slate-100"
          role="meter"
          aria-label={`${panel.label} occupancy`}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={used}
        >
          <div
            className={cn('h-full rounded-full transition-all duration-700', {
              'bg-emerald-500': tone === 'green',
              'bg-amber-500': tone === 'amber',
              'bg-red-500': tone === 'red',
            })}
            style={{ width: `${used}%` }}
          />
        </div>
        <p className="mt-1.5 text-xs text-slate-500">
          {used}% occupied{panel.outOfService > 0 && ` · ${panel.outOfService} out of service`}
        </p>
      </div>

      {showCodes && (
        <div className="mt-4 flex-1 border-t border-slate-100 px-5 py-4">
          {home.length === 0 && overflow.length === 0 ? (
            <p className="text-sm text-slate-500">No free {panel.label.toLowerCase()} spots right now.</p>
          ) : (
            <>
              <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-slate-500">Free spots</p>
              <ul className="flex flex-wrap gap-1.5">
                {home.map((s) => (
                  <li key={s.code} className="rounded-md bg-emerald-50 px-2 py-1 font-mono text-xs font-semibold text-emerald-800 ring-1 ring-emerald-200">
                    {s.code}
                  </li>
                ))}
                {overflow.map((s) => (
                  <li
                    key={s.code}
                    title={`Overflow: ${spotTypeLabel[s.spotType]} spot`}
                    className="rounded-md bg-sky-50 px-2 py-1 font-mono text-xs font-semibold text-sky-800 ring-1 ring-sky-200"
                  >
                    {s.code}
                    <span className="sr-only"> (overflow)</span>
                  </li>
                ))}
              </ul>
              {overflow.length > 0 && <p className="mt-2 text-xs text-slate-500">Blue spots are larger bays used as overflow.</p>}
            </>
          )}
        </div>
      )}
    </Card>
  )
}
