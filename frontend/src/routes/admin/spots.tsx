import { useMutation, useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { Ban, CheckCircle2, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { api } from '@/api/client'
import type { Items, Spot, SpotType } from '@/api/types'
import { AdminLayout } from '@/components/layout/AdminLayout'
import { Badge, Button, Card, CardHeader, ErrorBox, Field, Input, MessageRow, Select, Table } from '@/components/ui'
import { errorMessage } from '@/lib/errors'
import { spotTypeLabel, spotTypeLetter } from '@/lib/format'
import { keys, refreshLotData } from '@/lib/queryClient'
import { requireAdmin } from '@/lib/requireAdmin'

// URL: /admin/spots — create spots and take them out of service (FR-33, FR-34).
// Row and position also decide where each bay is drawn in the simulator.
export const Route = createFileRoute('/admin/spots')({
  beforeLoad: requireAdmin,
  component: SpotsPage,
})

const statusBadge: Record<string, [string, 'green' | 'blue' | 'amber']> = {
  AVAILABLE: ['Free', 'green'],
  OCCUPIED: ['Occupied', 'blue'],
  OUT_OF_SERVICE: ['Out of service', 'amber'],
}

function SpotsPage() {
  const spots = useQuery({ queryKey: keys.spots, queryFn: () => api<Items<Spot>>('/admin/spots', { auth: 'admin' }), refetchInterval: 5000 })

  // One mutation for all spot edits: change status or change type.
  const updateSpot = useMutation({
    mutationFn: ({ spot, change }: { spot: Spot; change: { status?: string; spotType?: string } }) =>
      api<Spot>(`/admin/spots/${spot.id}`, { method: 'PATCH', auth: 'admin', body: { ...change, reason: 'Changed on the Spots page' } }),
    onSuccess: (spot) => {
      toast.success(`${spot.code} updated`)
      refreshLotData()
    },
    onError: (err) => toast.error(errorMessage(err)),
  })

  const deleteSpot = useMutation({
    mutationFn: (spot: Spot) => api(`/admin/spots/${spot.id}`, { method: 'DELETE', auth: 'admin' }),
    onSuccess: () => {
      toast.success('Spot deleted')
      refreshLotData()
    },
    onError: (err) => toast.error(errorMessage(err)),
  })

  const items = spots.data?.items ?? []
  return (
    <AdminLayout title="Spots" description="Rows and positions here also place each bay in the simulator drawing.">
      <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_22rem]">
        <Card>
          {spots.error && <ErrorBox>{errorMessage(spots.error)}</ErrorBox>}
          <Table>
            <thead>
              <tr>
                <th>Code</th>
                <th>Floor</th>
                <th>Row</th>
                <th>Position</th>
                <th>Type</th>
                <th>Status</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {spots.isLoading && <MessageRow columns={7} loading />}
              {items.map((spot) => {
                const [label, color] = statusBadge[spot.status]
                const occupied = spot.status === 'OCCUPIED'
                return (
                  <tr key={spot.id} className={spot.status === 'OUT_OF_SERVICE' ? 'hatched' : undefined}>
                    <td className="font-mono font-bold">{spot.code}</td>
                    <td>{spot.floor}</td>
                    <td>{spot.rowLabel}</td>
                    <td>{spot.position}</td>
                    <td>
                      <Select
                        aria-label={`Type of ${spot.code}`}
                        value={spot.spotType}
                        disabled={occupied}
                        className="h-8 text-xs"
                        onChange={(e) => updateSpot.mutate({ spot, change: { spotType: e.target.value } })}
                      >
                        {(['COMPACT', 'REGULAR', 'OVERSIZED'] as const).map((type) => (
                          <option key={type} value={type}>
                            {spotTypeLetter[type]} · {spotTypeLabel[type]}
                          </option>
                        ))}
                      </Select>
                    </td>
                    <td>
                      <Badge color={color}>{label}</Badge>
                    </td>
                    <td className="whitespace-nowrap text-right">
                      {spot.status === 'OUT_OF_SERVICE' ? (
                        <Button size="small" variant="secondary" onClick={() => updateSpot.mutate({ spot, change: { status: 'AVAILABLE' } })}>
                          <CheckCircle2 /> Back in service
                        </Button>
                      ) : (
                        <Button size="small" variant="secondary" disabled={occupied} onClick={() => updateSpot.mutate({ spot, change: { status: 'OUT_OF_SERVICE' } })}>
                          <Ban /> Out of service
                        </Button>
                      )}
                      <Button
                        size="icon"
                        variant="ghost"
                        className="text-red-600"
                        aria-label={`Delete ${spot.code}`}
                        disabled={occupied}
                        onClick={() => confirm(`Delete spot ${spot.code}? Spots with ticket history can only be marked out of service.`) && deleteSpot.mutate(spot)}
                      >
                        <Trash2 />
                      </Button>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </Table>
        </Card>
        <AddSpots />
      </div>
    </AdminLayout>
  )
}

/** Create one spot or a whole row of positions at once. */
function AddSpots() {
  const [spotType, setSpotType] = useState<SpotType>('REGULAR')
  const [floor, setFloor] = useState(0)
  const [rowLabel, setRowLabel] = useState('C')
  const [fromPosition, setFromPosition] = useState(1)
  const [toPosition, setToPosition] = useState(6)

  const create = useMutation({
    mutationFn: () => api<Items<Spot>>('/admin/spots', { method: 'POST', auth: 'admin', body: { spotType, floor, rowLabel, fromPosition, toPosition } }),
    onSuccess: (result) => {
      toast.success(`Created ${result.items.length} spots`, { description: result.items.map((s) => s.code).join(', ') })
      refreshLotData()
    },
  })

  const firstCode = floor === 0 ? `${rowLabel}${fromPosition}` : `F${floor}-${rowLabel}${fromPosition}`
  return (
    <Card className="h-fit">
      <CardHeader title="Add spots" />
      <form
        className="space-y-3 px-5 py-4"
        onSubmit={(e) => {
          e.preventDefault()
          create.mutate()
        }}
      >
        <Field label="Spot type">
          <Select value={spotType} onChange={(e) => setSpotType(e.target.value as SpotType)} className="w-full">
            <option value="COMPACT">C · Compact (motorcycles)</option>
            <option value="REGULAR">R · Regular (cars)</option>
            <option value="OVERSIZED">L · Oversized (trucks)</option>
          </Select>
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Floor">
            <Input type="number" min={0} max={20} value={floor} onChange={(e) => setFloor(Number(e.target.value))} />
          </Field>
          <Field label="Row">
            <Input maxLength={1} value={rowLabel} onChange={(e) => setRowLabel(e.target.value.toUpperCase())} />
          </Field>
          <Field label="From position">
            <Input type="number" min={1} max={60} value={fromPosition} onChange={(e) => setFromPosition(Number(e.target.value))} />
          </Field>
          <Field label="To position">
            <Input type="number" min={1} max={60} value={toPosition} onChange={(e) => setToPosition(Number(e.target.value))} />
          </Field>
        </div>
        <p className="text-xs text-slate-500">Codes start at {firstCode}.</p>
        {create.error && <ErrorBox>{errorMessage(create.error)}</ErrorBox>}
        <Button type="submit" className="w-full" loading={create.isPending}>
          <Plus /> Create spots
        </Button>
      </form>
    </Card>
  )
}
