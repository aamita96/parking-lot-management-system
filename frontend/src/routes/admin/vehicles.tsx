import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { Search, UserCog } from 'lucide-react'
import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { api, queryString } from '@/api/client'
import type { OwnerSummary, Page, Vehicle } from '@/api/types'
import { ExtraInput, ReasonDialog } from '@/components/admin/ActionDialogs'
import { AdminLayout } from '@/components/layout/AdminLayout'
import { Badge, Button, Card, Input, MessageRow, Pager, Table } from '@/components/ui'
import { dateIST, vehicleTypeLabel } from '@/lib/format'
import { keys } from '@/lib/queryClient'
import { requireAdmin } from '@/lib/requireAdmin'
import { cn } from '@/lib/utils'

// URL: /admin/vehicles?tab=owners&q=example — search owners and vehicles (FR-39), reassign (FR-38).
export const Route = createFileRoute('/admin/vehicles')({
  validateSearch: (search: Record<string, unknown>): { q?: string; tab?: 'vehicles' | 'owners'; page?: number } => ({
    q: typeof search.q === 'string' ? search.q : undefined,
    tab: search.tab === 'owners' ? 'owners' : undefined, // undefined = the vehicles tab
    page: typeof search.page === 'number' ? search.page : undefined,
  }),
  beforeLoad: requireAdmin,
  component: VehiclesPage,
})

const statusBadge: Record<string, [string, 'green' | 'amber' | 'slate']> = {
  ACTIVE: ['Active', 'green'],
  PENDING_VERIFICATION: ['Unverified', 'amber'],
  REMOVED: ['Removed', 'slate'],
}

function VehiclesPage() {
  const filters = Route.useSearch()
  const navigate = Route.useNavigate()
  const tab = filters.tab ?? 'vehicles'
  const page = filters.page ?? 1
  const [searchText, setSearchText] = useState(filters.q ?? '')
  const [reassigning, setReassigning] = useState<Vehicle | null>(null)
  const [newOwnerEmail, setNewOwnerEmail] = useState('')

  useEffect(() => {
    const timer = setTimeout(() => {
      if (searchText !== (filters.q ?? '')) navigate({ search: { ...filters, q: searchText || undefined, page: undefined } })
    }, 350)
    return () => clearTimeout(timer)
  }, [searchText, filters, navigate])

  const vehicles = useQuery({
    queryKey: [...keys.vehicles, filters.q, page],
    queryFn: () => api<Page<Vehicle>>(`/admin/vehicles${queryString({ q: filters.q, page, pageSize: 25 })}`, { auth: 'admin' }),
    enabled: tab === 'vehicles',
  })
  const owners = useQuery({
    queryKey: [...keys.owners, filters.q, page],
    queryFn: () => api<Page<OwnerSummary>>(`/admin/owners${queryString({ q: filters.q, page, pageSize: 25 })}`, { auth: 'admin' }),
    enabled: tab === 'owners',
  })
  const current = tab === 'vehicles' ? vehicles : owners

  return (
    <AdminLayout title="Owners & vehicles" description="Search everyone who registered, and move a vehicle to its rightful owner.">
      <Card>
        <div className="flex flex-wrap items-center gap-2 border-b border-slate-100 p-3">
          <div className="flex rounded-lg bg-slate-100 p-0.5">
            {(['vehicles', 'owners'] as const).map((option) => (
              <button
                key={option}
                onClick={() => navigate({ search: { ...filters, tab: option === 'owners' ? 'owners' : undefined, page: undefined } })}
                className={cn('rounded-md px-3 py-1.5 text-sm font-medium capitalize', tab === option ? 'bg-white shadow-sm' : 'text-slate-500')}
              >
                {option}
              </button>
            ))}
          </div>
          <div className="relative min-w-60 flex-1">
            <Search className="absolute left-2.5 top-2.5 size-4 text-slate-400" aria-hidden />
            <Input value={searchText} onChange={(e) => setSearchText(e.target.value)} placeholder="Number, email, mobile or RC" className="pl-8" aria-label="Search" />
          </div>
        </div>

        {tab === 'vehicles' ? (
          <Table>
            <thead>
              <tr>
                <th>Number</th>
                <th>Type</th>
                <th>Owner</th>
                <th>RC</th>
                <th>Status</th>
                <th>Registered</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {vehicles.isLoading && <MessageRow columns={7} loading />}
              {vehicles.data?.items.length === 0 && <MessageRow columns={7}>No vehicles match.</MessageRow>}
              {vehicles.data?.items.map((vehicle) => {
                const [label, color] = statusBadge[vehicle.status]
                return (
                  <tr key={vehicle.id}>
                    <td className="font-mono font-semibold">{vehicle.vehicleNumber}</td>
                    <td>{vehicleTypeLabel[vehicle.vehicleType]}</td>
                    <td>{vehicle.ownerEmail}</td>
                    <td>{vehicle.registrationId ?? '—'}</td>
                    <td>
                      <Badge color={color}>{label}</Badge>
                    </td>
                    <td>{dateIST(vehicle.createdAt)}</td>
                    <td className="text-right">
                      <Button
                        size="small"
                        variant="ghost"
                        onClick={() => {
                          setReassigning(vehicle)
                          setNewOwnerEmail('')
                        }}
                      >
                        <UserCog /> Reassign
                      </Button>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </Table>
        ) : (
          <Table>
            <thead>
              <tr>
                <th>Email</th>
                <th>Mobile</th>
                <th>Verified</th>
                <th>Vehicles</th>
                <th>Since</th>
              </tr>
            </thead>
            <tbody>
              {owners.isLoading && <MessageRow columns={5} loading />}
              {owners.data?.items.length === 0 && <MessageRow columns={5}>No owners match.</MessageRow>}
              {owners.data?.items.map((owner) => (
                <tr key={owner.id}>
                  <td className="font-medium">{owner.email}</td>
                  <td>{owner.mobile ?? '—'}</td>
                  <td>{owner.emailVerifiedAt ? <Badge color="green">Yes</Badge> : <Badge color="amber">No</Badge>}</td>
                  <td className="space-x-1">
                    {owner.vehicles.map((v) => (
                      <span key={v.id} className="rounded bg-slate-100 px-1.5 py-0.5 font-mono text-xs">
                        {v.vehicleNumber}
                      </span>
                    ))}
                  </td>
                  <td>{dateIST(owner.createdAt)}</td>
                </tr>
              ))}
            </tbody>
          </Table>
        )}
        {current.data && <Pager page={page} pageSize={25} total={current.data.total} onPage={(p) => navigate({ search: { ...filters, page: p } })} />}
      </Card>

      {reassigning && (
        <ReasonDialog
          title={`Reassign ${reassigning.vehicleNumber}`}
          description={`Currently registered to ${reassigning.ownerEmail}. Open tickets keep the email from entry.`}
          confirmLabel="Reassign"
          onClose={() => setReassigning(null)}
          onConfirm={async (reason) => {
            await api(`/admin/vehicles/${reassigning.id}`, { method: 'PATCH', auth: 'admin', body: { ownerEmail: newOwnerEmail, reason } })
            toast.success('Vehicle reassigned')
          }}
        >
          <ExtraInput label="New owner email" type="email" value={newOwnerEmail} onChange={setNewOwnerEmail} />
        </ReasonDialog>
      )}
    </AdminLayout>
  )
}
