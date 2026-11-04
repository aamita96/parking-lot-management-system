import { useMutation, useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { LogIn, LogOut, Mail, RotateCw, Search, Ticket as TicketIcon } from 'lucide-react'
import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { api, queryString } from '@/api/client'
import type { Page, Ticket, TicketDetail } from '@/api/types'
import { ManualCheckInDialog, ManualExitDialog } from '@/components/admin/ActionDialogs'
import { AdminLayout } from '@/components/layout/AdminLayout'
import { EmailStatusBadge, KafkaStatusBadge, TicketStatusBadge } from '@/components/StatusBadges'
import { Badge, Button, Card, DetailList, ErrorBox, Input, MessageRow, Modal, Pager, Select, Spinner, Table } from '@/components/ui'
import { errorMessage } from '@/lib/errors'
import { calculateFee, minutesSince, rateSummary } from '@/lib/fee'
import { ago, dateTimeIST, humanDuration, paymentMethodLabel, rupees, spotTypeLabel, vehicleTypeLabel } from '@/lib/format'
import { keys, queryClient } from '@/lib/queryClient'
import { requireAdmin } from '@/lib/requireAdmin'

// URL: /admin/tickets?q=KA01&status=OPEN&simulated=false&page=2
// The filters are kept in the URL, so a filtered view can be bookmarked or shared.
export const Route = createFileRoute('/admin/tickets')({
  validateSearch: (search: Record<string, unknown>): { q?: string; status?: string; simulated?: string; page?: number } => ({
    q: typeof search.q === 'string' ? search.q : undefined,
    status: typeof search.status === 'string' ? search.status : undefined,
    simulated: typeof search.simulated === 'string' ? search.simulated : undefined,
    page: typeof search.page === 'number' ? search.page : undefined,
  }),
  beforeLoad: requireAdmin,
  component: TicketsPage,
})

function TicketsPage() {
  const filters = Route.useSearch()
  const navigate = Route.useNavigate()
  const page = filters.page ?? 1
  const [searchText, setSearchText] = useState(filters.q ?? '')
  const [openTicketId, setOpenTicketId] = useState<string | null>(null)
  const [checkInOpen, setCheckInOpen] = useState(false)

  // Put the search text into the URL 0.35 s after the user stops typing.
  useEffect(() => {
    const timer = setTimeout(() => {
      if (searchText !== (filters.q ?? '')) navigate({ search: { ...filters, q: searchText || undefined, page: undefined } })
    }, 350)
    return () => clearTimeout(timer)
  }, [searchText, filters, navigate])

  const tickets = useQuery({
    queryKey: [...keys.tickets, filters],
    queryFn: () =>
      api<Page<Ticket>>(`/admin/tickets${queryString({ q: filters.q, status: filters.status, simulated: filters.simulated, page, pageSize: 25 })}`, {
        auth: 'admin',
      }),
    refetchInterval: 5000,
  })
  const items = tickets.data?.items ?? []

  return (
    <AdminLayout
      title="Tickets"
      description="Active tickets show live elapsed time and a running fee. Simulated tickets are left out of revenue."
      actions={
        <Button onClick={() => setCheckInOpen(true)}>
          <LogIn /> Manual check-in
        </Button>
      }
    >
      <Card>
        <div className="flex flex-wrap items-center gap-2 border-b border-slate-100 p-3">
          <div className="relative min-w-60 flex-1">
            <Search className="absolute left-2.5 top-2.5 size-4 text-slate-400" aria-hidden />
            <Input value={searchText} onChange={(e) => setSearchText(e.target.value)} placeholder="Ticket number, plate, owner email or spot" className="pl-8" aria-label="Search tickets" />
          </div>
          <Select
            aria-label="Status"
            value={filters.status ?? ''}
            onChange={(e) => navigate({ search: { ...filters, status: e.target.value || undefined, page: undefined } })}
          >
            <option value="">All statuses</option>
            <option value="OPEN">Open (parked or paying)</option>
            <option value="ACTIVE">Parked</option>
            <option value="PAYMENT_PENDING">Awaiting payment</option>
            <option value="CLOSED">Closed</option>
            <option value="CANCELLED">Cancelled</option>
          </Select>
          <Select
            aria-label="Simulated"
            value={filters.simulated ?? ''}
            onChange={(e) => navigate({ search: { ...filters, simulated: e.target.value || undefined, page: undefined } })}
          >
            <option value="">Real + simulated</option>
            <option value="false">Real only</option>
            <option value="true">Simulated only</option>
          </Select>
        </div>

        {tickets.error && <ErrorBox>{errorMessage(tickets.error)}</ErrorBox>}
        <Table>
          <thead>
            <tr>
              <th>Ticket</th>
              <th>Vehicle</th>
              <th>Spot</th>
              <th>Entry</th>
              <th>Duration</th>
              <th>Fee</th>
              <th>Status</th>
            </tr>
          </thead>
          <tbody>
            {tickets.isLoading && <MessageRow columns={7} loading />}
            {!tickets.isLoading && items.length === 0 && <MessageRow columns={7}>No tickets match.</MessageRow>}
            {items.map((ticket) => (
              <tr key={ticket.id} onClick={() => setOpenTicketId(ticket.id)} className="cursor-pointer hover:bg-brand-50/40">
                <td className="font-mono text-xs">{ticket.ticketNumber}</td>
                <td>
                  <span className="font-mono font-semibold">{ticket.vehicleNumber}</span>
                  <span className="block text-xs text-slate-500">{vehicleTypeLabel[ticket.vehicleType]}</span>
                </td>
                <td>{ticket.spotCode}</td>
                <td className="whitespace-nowrap">{dateTimeIST(ticket.entryTime)}</td>
                <td>
                  <Duration ticket={ticket} />
                </td>
                <td>
                  <Fee ticket={ticket} />
                </td>
                <td className="space-x-1">
                  <TicketStatusBadge status={ticket.status} />
                  {ticket.isSimulated && <Badge color="violet">Simulated</Badge>}
                </td>
              </tr>
            ))}
          </tbody>
        </Table>
        {tickets.data && <Pager page={page} pageSize={25} total={tickets.data.total} onPage={(p) => navigate({ search: { ...filters, page: p } })} />}
      </Card>

      {openTicketId && <TicketDetailDialog ticketId={openTicketId} onClose={() => setOpenTicketId(null)} />}
      {checkInOpen && <ManualCheckInDialog onClose={() => setCheckInOpen(false)} />}
    </AdminLayout>
  )
}

/** Re-renders this component every `ms` milliseconds, for live timers. */
function useNow(ms: number) {
  const [now, setNow] = useState(Date.now())
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), ms)
    return () => clearInterval(timer)
  }, [ms])
  return now
}

function Duration({ ticket }: { ticket: Ticket }) {
  const now = useNow(1000)
  if (ticket.durationMinutes !== null) return <>{humanDuration(ticket.durationMinutes)}</>
  return <span className="text-brand-700">{humanDuration(minutesSince(ticket.entryTime, now))}</span>
}

function Fee({ ticket }: { ticket: Ticket }) {
  const now = useNow(5000)
  if (ticket.status === 'CLOSED') {
    return (
      <span>
        {rupees(ticket.paidPaise)} <span className="text-xs text-slate-500">{paymentMethodLabel[ticket.paymentMethod ?? '']}</span>
      </span>
    )
  }
  if (ticket.feePaise !== null) return <>{rupees(ticket.feePaise)}</>
  if (ticket.status === 'ACTIVE') {
    // Still parked: show what it would cost if it left now.
    return <span className="text-slate-500">~{rupees(calculateFee(minutesSince(ticket.entryTime, now), ticket.rate).feePaise)}</span>
  }
  return <>—</>
}

function TicketDetailDialog({ ticketId, onClose }: { ticketId: string; onClose: () => void }) {
  const [exitOpen, setExitOpen] = useState(false)
  const detail = useQuery({
    queryKey: [...keys.tickets, ticketId],
    queryFn: () => api<TicketDetail>(`/admin/tickets/${ticketId}`, { auth: 'admin' }),
    refetchInterval: 3000, // watch the email status change live
  })
  const resend = useMutation({
    mutationFn: (kind: 'entry' | 'exit') => api(`/admin/tickets/${ticketId}/notifications/${kind}/resend`, { method: 'POST', auth: 'admin' }),
    onSuccess: () => {
      toast.success('Email queued again')
      queryClient.invalidateQueries({ queryKey: [...keys.tickets, ticketId] })
    },
    onError: (err) => toast.error(errorMessage(err)),
  })

  const t = detail.data
  return (
    <Modal open onClose={onClose} title={t ? `Ticket ${t.ticketNumber}` : 'Ticket'} icon={<TicketIcon />} wide>
      {!t && <Spinner />}
      {t && (
        <div className="space-y-4">
          <div className="flex flex-wrap gap-2">
            <TicketStatusBadge status={t.status} />
            {t.isSimulated && <Badge color="violet">Simulated</Badge>}
            <Badge>{t.source}</Badge>
          </div>
          <DetailList
            items={[
              ['Vehicle', `${t.vehicleNumber} · ${vehicleTypeLabel[t.vehicleType]}`],
              ['Owner email (at entry)', t.ownerEmail],
              ['Spot', `${t.spotCode} · ${spotTypeLabel[t.spotType]} · floor ${t.spotFloor}`],
              ['Rate', rateSummary(t.rate)],
              ['Entry', dateTimeIST(t.entryTime)],
              ['Exit', dateTimeIST(t.exitTime)],
              ['Duration', t.durationMinutes !== null ? `${humanDuration(t.durationMinutes)}${t.billableHours ? `, billed as ${t.billableHours} h` : ''}` : '—'],
              ['Fee', t.breakdown ?? '—'],
              ['Paid', t.paidAt ? `${rupees(t.paidPaise)} · ${paymentMethodLabel[t.paymentMethod ?? ''] ?? t.paymentMethod} · ${dateTimeIST(t.paidAt)}` : '—'],
            ]}
          />

          <div>
            <h3 className="mb-2 flex items-center gap-1.5 text-sm font-semibold">
              <Mail className="size-4" /> Emails
            </h3>
            <div className="divide-y divide-slate-100 rounded-xl border border-slate-200">
              {(['ENTRY', 'EXIT'] as const).map((kind) => {
                const email = t.notifications.find((n) => n.kind === kind)
                const event = t.events.find((e) => e.eventType === (kind === 'ENTRY' ? 'VehicleEntered' : 'VehicleExited'))
                return (
                  <div key={kind} className="flex flex-wrap items-center gap-3 px-3 py-2.5 text-sm">
                    <span className="w-12 font-medium">{kind === 'ENTRY' ? 'Entry' : 'Exit'}</span>
                    {event && <KafkaStatusBadge status={event.kafkaStatus} />}
                    <EmailStatusBadge status={email?.status ?? event?.emailStatus ?? 'WAITING'} />
                    <span className="min-w-0 flex-1 truncate text-xs text-slate-500">
                      {email?.sentAt ? `Sent ${ago(email.sentAt)} to ${email.recipient}` : email?.lastError ?? (event ? '' : 'No event yet')}
                    </span>
                    {email && email.status !== 'QUEUED' && (
                      <Button size="small" variant="ghost" loading={resend.isPending} onClick={() => resend.mutate(kind === 'ENTRY' ? 'entry' : 'exit')}>
                        <RotateCw /> Resend
                      </Button>
                    )}
                  </div>
                )
              })}
            </div>
          </div>

          {(t.status === 'ACTIVE' || t.status === 'PAYMENT_PENDING') && (
            <div className="flex justify-end">
              <Button variant="secondary" onClick={() => setExitOpen(true)}>
                <LogOut /> Manual exit
              </Button>
            </div>
          )}
        </div>
      )}
      {t && exitOpen && <ManualExitDialog ticket={t} onClose={() => setExitOpen(false)} />}
    </Modal>
  )
}
