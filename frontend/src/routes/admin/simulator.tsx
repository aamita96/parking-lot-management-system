import { useMutation, useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { useEffect } from 'react'
import { toast } from 'sonner'
import { api, queryString } from '@/api/client'
import type { EventSummary, Items, Layout, Page, PublicConfig, Vehicle } from '@/api/types'
import { AdminLayout } from '@/components/layout/AdminLayout'
import { ErrorBox, Spinner } from '@/components/ui'
import { errorMessage } from '@/lib/errors'
import { keys, refreshLotData } from '@/lib/queryClient'
import { requireAdmin } from '@/lib/requireAdmin'
import { Controls } from '@/simulator/Controls'
import { EventList } from '@/simulator/EventList'
import { scanAtEntry, scanAtExit, sendRandomVehicle } from '@/simulator/gateCalls'
import { GatePopups } from '@/simulator/GatePopups'
import { LotCanvas } from '@/simulator/LotCanvas'
import { ParkedVehicleCard } from '@/simulator/ParkedVehicleCard'
import { useSim } from '@/simulator/store'

// URL: /admin/simulator — the animated parking lot (SRS 11).
// Files in src/simulator/:
//   store.ts        what the simulator remembers, and tick() which moves the vehicles
//   gateCalls.ts    the three calls to the server (entry scan, exit scan, payment)
//   geometry.ts     where lanes, bays and gates are, and the routes between them
//   movement.ts     driving along a route
//   vehicles.ts     creating vehicles and syncing them with the server
//   LotCanvas.tsx   the drawing; GatePopups.tsx, Controls.tsx, ... the UI around it
export const Route = createFileRoute('/admin/simulator')({
  beforeLoad: requireAdmin,
  component: SimulatorPage,
})

function SimulatorPage() {
  const config = useQuery({ queryKey: keys.config, queryFn: () => api<PublicConfig>('/public/config') })

  // Ask the server for the lot every 2 s, so vehicles parked by real gates also appear (FR-49).
  const layout = useQuery({
    queryKey: keys.layout,
    queryFn: () => api<Layout>('/admin/simulator/layout', { auth: 'admin' }),
    refetchInterval: 2000,
  })
  const events = useQuery({
    queryKey: keys.events,
    queryFn: () => api<Items<EventSummary>>('/admin/simulator/events?limit=20', { auth: 'admin' }),
    refetchInterval: 2000,
  })
  const registered = useQuery({
    queryKey: [...keys.vehicles, 'active'],
    queryFn: () => api<Page<Vehicle>>(`/admin/vehicles${queryString({ status: 'ACTIVE', pageSize: 500 })}`, { auth: 'admin' }),
  })

  // Hand the server's data to the simulator store whenever it changes.
  useEffect(() => {
    if (layout.data) useSim.getState().setLayout(layout.data)
  }, [layout.data])
  useEffect(() => {
    if (registered.data) useSim.getState().setRegisteredVehicles(registered.data.items)
  }, [registered.data])

  // The animation loop: about 60 times a second, move the vehicles and make any gate calls.
  useEffect(() => {
    let frame = 0
    let lastTime = performance.now()
    function nextFrame(time: number) {
      const result = useSim.getState().tick(time - lastTime)
      lastTime = time
      result.arrivedAtEntry.forEach(scanAtEntry)
      result.arrivedAtPay.forEach(scanAtExit)
      if (result.autoVehicleDue) sendRandomVehicle()
      frame = requestAnimationFrame(nextFrame)
    }
    frame = requestAnimationFrame(nextFrame)
    return () => cancelAnimationFrame(frame) // stop when leaving the page
  }, [])

  const reset = useMutation({
    mutationFn: () => api<{ cancelledTickets: number }>('/admin/simulator/reset', { method: 'POST', auth: 'admin' }),
    onSuccess: (result) => {
      useSim.getState().reset()
      refreshLotData()
      toast.success('Simulator reset', { description: `${result.cancelledTickets} simulated tickets closed.` })
    },
    onError: (err) => toast.error(errorMessage(err)),
  })

  if (config.data && !config.data.simulatorEnabled) {
    return (
      <AdminLayout title="Parking lot simulator">
        <ErrorBox>The simulator is turned off on this server (SIMULATOR_ENABLED=false).</ErrorBox>
      </AdminLayout>
    )
  }

  const eventList = events.data?.items ?? []
  return (
    <AdminLayout
      title="Parking lot simulator"
      description="Drives registered vehicles through the real gate API: entry scan, spot, PAY kiosk, payment, Kafka and email."
    >
      <Controls onReset={() => reset.mutate()} resetting={reset.isPending} />

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_22rem]">
        <div className="space-y-2">
          <div className="relative overflow-hidden rounded-xl border border-slate-200 bg-white shadow-card">
            {layout.isLoading && (
              <div className="grid h-96 place-items-center">
                <Spinner label="Loading the lot" />
              </div>
            )}
            {layout.error && <ErrorBox>{errorMessage(layout.error)}</ErrorBox>}
            <LotCanvas />
            <ParkedVehicleCard />
          </div>
          <Legend />
        </div>
        <div className="xl:h-[calc(100vh-15rem)] xl:min-h-[30rem]">
          <EventList events={eventList} loading={events.isLoading} />
        </div>
      </div>

      <GatePopups events={eventList} />
    </AdminLayout>
  )
}

function Legend() {
  const letters = [
    ['C', 'Compact', 'bg-amber-500'],
    ['R', 'Regular', 'bg-blue-500'],
    ['L', 'Oversized (large)', 'bg-violet-500'],
  ]
  return (
    <div className="flex flex-wrap items-center gap-x-5 gap-y-2 px-1 text-xs text-slate-500">
      {letters.map(([letter, label, color]) => (
        <span key={letter} className="inline-flex items-center gap-1.5">
          <span className={`grid size-4 place-items-center rounded text-[10px] font-bold text-white ${color}`}>{letter}</span>
          {label}
        </span>
      ))}
      <span className="inline-flex items-center gap-1.5">
        <span className="hatched size-4 rounded bg-slate-600" /> Out of service
      </span>
      <span>Click a parked vehicle for its ticket and “Leave”. Hover to see the plate.</span>
    </div>
  )
}
