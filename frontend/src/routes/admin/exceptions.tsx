import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { CircleCheck, Clock, ScanLine, ShieldAlert, UserCheck } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { toast } from 'sonner'
import { api } from '@/api/client'
import type { AdminAction, Exceptions, Scan, Ticket } from '@/api/types'
import { ManualCheckInDialog, ManualExitDialog, ReasonDialog } from '@/components/admin/ActionDialogs'
import { AdminLayout } from '@/components/layout/AdminLayout'
import { Badge, Button, Card, CardHeader, EmptyState, ErrorBox, MessageRow, Table } from '@/components/ui'
import { errorMessage, rejectReason } from '@/lib/errors'
import { ago, dateTimeIST, rupees } from '@/lib/format'
import { keys } from '@/lib/queryClient'
import { requireAdmin } from '@/lib/requireAdmin'

// URL: /admin/exceptions — the queue of things that need a person (FR-25, FR-27, FR-38).
export const Route = createFileRoute('/admin/exceptions')({
  beforeLoad: requireAdmin,
  component: ExceptionsPage,
})

function ExceptionsPage() {
  const exceptions = useQuery({
    queryKey: keys.exceptions,
    queryFn: () => api<Exceptions>('/admin/exceptions', { auth: 'admin' }),
    refetchInterval: 10000,
  })
  const data = exceptions.data

  // Which popup is open (only one at a time).
  const [checkInScan, setCheckInScan] = useState<Scan | null>(null)
  const [dismissScan, setDismissScan] = useState<Scan | null>(null)
  const [exitTicket, setExitTicket] = useState<Ticket | null>(null)
  const [reassignDispute, setReassignDispute] = useState<AdminAction | null>(null)
  const [closeDispute, setCloseDispute] = useState<AdminAction | null>(null)

  return (
    <AdminLayout title="Exception queue" description="Rejected scans, unpaid tickets and ownership disputes. There is no gate operator; these come to you.">
      {exceptions.error && <ErrorBox>{errorMessage(exceptions.error)}</ErrorBox>}

      <div className="grid gap-3 sm:grid-cols-3">
        <Counter icon={<ScanLine />} label="Rejected scans" value={data?.counts.rejectedScans} />
        <Counter icon={<Clock />} label="Unpaid > 15 min" value={data?.counts.unpaidTickets} />
        <Counter icon={<ShieldAlert />} label="Open disputes" value={data?.counts.disputes} />
      </div>

      <Card>
        <CardHeader title="Rejected scans" />
        <Table>
          <thead>
            <tr>
              <th>When</th>
              <th>Gate</th>
              <th>Plate as read</th>
              <th>Reason</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {exceptions.isLoading && <MessageRow columns={5} loading />}
            {data?.rejectedScans.length === 0 && <MessageRow columns={5}>No rejected scans.</MessageRow>}
            {data?.rejectedScans.map((scan) => (
              <tr key={scan.id}>
                <td title={dateTimeIST(scan.scannedAt)}>{ago(scan.scannedAt)}</td>
                <td>
                  <Badge color={scan.gateKind === 'ENTRY' ? 'blue' : 'green'}>{scan.gateName}</Badge>
                </td>
                <td className="font-mono font-semibold">{scan.plateRaw || '—'}</td>
                <td className="text-red-700">{rejectReason[scan.rejectReason ?? ''] ?? scan.rejectReason}</td>
                <td className="text-right">
                  {scan.gateKind === 'ENTRY' && scan.rejectReason !== 'ALREADY_PARKED' && (
                    <Button size="small" variant="secondary" onClick={() => setCheckInScan(scan)}>
                      Check in
                    </Button>
                  )}
                  <Button size="small" variant="ghost" onClick={() => setDismissScan(scan)}>
                    Dismiss
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </Table>
      </Card>

      <Card>
        <CardHeader title="Unpaid tickets" />
        <Table>
          <thead>
            <tr>
              <th>Ticket</th>
              <th>Vehicle</th>
              <th>At PAY since</th>
              <th>Fee</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {data?.unpaidTickets.length === 0 && <MessageRow columns={5}>Nobody is stuck at the PAY kiosk.</MessageRow>}
            {data?.unpaidTickets.map((ticket) => (
              <tr key={ticket.id}>
                <td className="font-mono text-xs">{ticket.ticketNumber}</td>
                <td className="font-mono font-semibold">
                  {ticket.vehicleNumber} {ticket.isSimulated && <Badge color="violet">Simulated</Badge>}
                </td>
                <td>{ago(ticket.exitTime)}</td>
                <td>{rupees(ticket.feePaise)}</td>
                <td className="text-right">
                  <Button size="small" variant="secondary" onClick={() => setExitTicket(ticket)}>
                    Manual exit
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </Table>
      </Card>

      <Card>
        <CardHeader title="Ownership disputes" />
        {data?.disputes.length === 0 && <EmptyState icon={<CircleCheck />} title="No open disputes" />}
        <ul className="divide-y divide-slate-100">
          {data?.disputes.map((dispute) => {
            const d = dispute.details as Record<string, string>
            return (
              <li key={dispute.id} className="flex flex-wrap items-start gap-4 px-5 py-4">
                <div className="min-w-0 flex-1 space-y-1 text-sm">
                  <p>
                    <span className="font-mono text-base font-bold">{d.vehicleNumber}</span>
                    <span className="ml-2 text-xs text-slate-400">{ago(dispute.createdAt)}</span>
                  </p>
                  <p>
                    Reported by <strong>{d.reporterEmail}</strong>
                    {d.reporterMobile && ` · ${d.reporterMobile}`}
                    {d.registrationId && ` · RC ${d.registrationId}`}
                  </p>
                  <p className="text-slate-500">
                    Currently registered to {d.currentOwnerEmail ?? 'nobody'}
                    {d.currentRegistrationId && ` · RC ${d.currentRegistrationId}`}
                  </p>
                  {d.message && <p className="rounded-lg bg-slate-50 p-2 text-slate-700">“{d.message}”</p>}
                </div>
                <div className="flex gap-2">
                  {dispute.targetId && (
                    <Button size="small" onClick={() => setReassignDispute(dispute)}>
                      <UserCheck /> Reassign to reporter
                    </Button>
                  )}
                  <Button size="small" variant="secondary" onClick={() => setCloseDispute(dispute)}>
                    Close
                  </Button>
                </div>
              </li>
            )
          })}
        </ul>
      </Card>

      {/* Popups */}
      {checkInScan && <ManualCheckInDialog plate={checkInScan.plateNormalised ?? checkInScan.plateRaw} scanId={checkInScan.id} onClose={() => setCheckInScan(null)} />}
      {exitTicket && <ManualExitDialog ticket={exitTicket} onClose={() => setExitTicket(null)} />}
      {dismissScan && (
        <ReasonDialog
          title={`Dismiss scan of ${dismissScan.plateRaw || 'unknown plate'}`}
          confirmLabel="Dismiss"
          onClose={() => setDismissScan(null)}
          onConfirm={(reason) => api(`/admin/exceptions/scans/${dismissScan.id}/resolve`, { method: 'POST', auth: 'admin', body: { reason } })}
        />
      )}
      {reassignDispute && (
        <ReasonDialog
          title={`Reassign ${reassignDispute.details.vehicleNumber}`}
          description={`The vehicle moves to ${reassignDispute.details.reporterEmail}. Open tickets keep the email from entry.`}
          confirmLabel="Reassign vehicle"
          onClose={() => setReassignDispute(null)}
          onConfirm={async (reason) => {
            await api(`/admin/vehicles/${reassignDispute.targetId}`, {
              method: 'PATCH',
              auth: 'admin',
              body: { ownerEmail: reassignDispute.details.reporterEmail, reason, disputeId: reassignDispute.id },
            })
            toast.success('Vehicle reassigned')
          }}
        />
      )}
      {closeDispute && (
        <ReasonDialog
          title="Close dispute"
          confirmLabel="Close dispute"
          onClose={() => setCloseDispute(null)}
          onConfirm={(reason) => api(`/admin/disputes/${closeDispute.id}/resolve`, { method: 'POST', auth: 'admin', body: { reason } })}
        />
      )}
    </AdminLayout>
  )
}

function Counter({ icon, label, value }: { icon: ReactNode; label: string; value?: number }) {
  return (
    <Card className="flex items-center gap-3 p-4">
      <div className="grid size-10 place-items-center rounded-lg bg-slate-100 text-slate-600 [&_svg]:size-5">{icon}</div>
      <div>
        <p className="tabular text-2xl font-bold">{value ?? '—'}</p>
        <p className="text-xs text-slate-500">{label}</p>
      </div>
    </Card>
  )
}
