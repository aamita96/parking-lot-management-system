import { useMutation, useQuery } from '@tanstack/react-query'
import { createFileRoute, Link } from '@tanstack/react-router'
import { Car, Info, LogOut, Mail, Pencil, Plus, Trash2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { api, queryString } from '@/api/client'
import type { CodeSent, MyVehicle, Owner, OwnerSession, Page, Ticket } from '@/api/types'
import { VehicleIcon } from '@/components/brand/VehicleArt'
import { CodeInput, useCountdown } from '@/components/CodeInput'
import { PublicLayout } from '@/components/layout/PublicLayout'
import { TicketStatusBadge } from '@/components/StatusBadges'
import { Badge, Button, Card, CardHeader, EmptyState, ErrorBox, Field, Input, LinkButton, MessageRow, Modal, ModalFooter, Pager, Spinner, Table } from '@/components/ui'
import { errorMessage } from '@/lib/errors'
import { calculateFee, minutesSince, rateSummary } from '@/lib/fee'
import { dateTimeIST, humanDuration, paymentMethodLabel, rupees, vehicleTypeLabel } from '@/lib/format'
import { keys, queryClient } from '@/lib/queryClient'
import { useOwnerAuth } from '@/stores/owner'

// URL: /my — "My vehicles" (FR-7, FR-8). Shows a sign-in form until the owner has a token.
export const Route = createFileRoute('/my')({
  component: MyVehiclesPage,
})

function MyVehiclesPage() {
  const token = useOwnerAuth((s) => s.token)
  return (
    <PublicLayout>
      <div className="mx-auto max-w-5xl px-4 py-8">{token ? <Dashboard /> : <SignIn />}</div>
    </PublicLayout>
  )
}

// ---------------------------------------------------------------- Sign in with an email code

function SignIn() {
  const [email, setEmail] = useState('')
  const [codeSent, setCodeSent] = useState<CodeSent | null>(null)
  const secondsLeft = useCountdown(30, codeSent)

  const sendCode = useMutation({
    mutationFn: () => api<CodeSent>('/public/sign-in', { method: 'POST', body: { email } }),
    onSuccess: setCodeSent,
  })
  const verify = useMutation({
    mutationFn: (code: string) => api<OwnerSession>('/public/sign-in/verify', { method: 'POST', body: { email, code } }),
    onSuccess: (session) => {
      useOwnerAuth.getState().signIn(session)
      toast.success('Signed in')
    },
  })

  return (
    <Card className="mx-auto max-w-md p-6">
      <div className="mx-auto grid size-14 place-items-center rounded-full bg-brand-50 text-brand-700">
        <Mail className="size-7" aria-hidden />
      </div>
      <h1 className="mt-4 text-center text-2xl font-bold">My vehicles</h1>

      {!codeSent && (
        <form
          className="mt-5 space-y-4"
          onSubmit={(e) => {
            e.preventDefault()
            sendCode.mutate()
          }}
        >
          <p className="text-center text-sm text-slate-500">Enter the email you registered with. We'll send you a sign-in code.</p>
          <Field label="Email">
            <Input type="email" autoComplete="email" required value={email} onChange={(e) => setEmail(e.target.value)} placeholder="you@example.com" />
          </Field>
          {sendCode.error && <ErrorBox>{errorMessage(sendCode.error)}</ErrorBox>}
          <Button type="submit" className="w-full" size="large" loading={sendCode.isPending}>
            Send code
          </Button>
          <p className="text-center text-sm text-slate-500">
            New here?{' '}
            <Link to="/register" className="font-semibold text-brand-700 hover:underline">
              Register a vehicle
            </Link>
          </p>
        </form>
      )}

      {codeSent && (
        <div className="mt-5 space-y-4 text-center">
          <p className="text-sm text-slate-600">
            If <strong>{codeSent.codeSentTo}</strong> has registered vehicles, we've sent it a 6-digit code.
          </p>
          {codeSent.devCode && (
            <p className="inline-flex items-center gap-2 rounded-lg bg-amber-50 px-3 py-1.5 text-xs text-amber-900 ring-1 ring-amber-200">
              <Info className="size-3.5" aria-hidden /> Demo mode: your code is <strong className="font-mono">{codeSent.devCode}</strong>
            </p>
          )}
          <CodeInput onComplete={(code) => verify.mutate(code)} disabled={verify.isPending} invalid={!!verify.error} />
          {verify.error && (
            <p className="text-sm font-medium text-red-600" role="alert">
              {errorMessage(verify.error)}
            </p>
          )}
          <div className="flex justify-center gap-2">
            <Button variant="ghost" disabled={secondsLeft > 0} loading={sendCode.isPending} onClick={() => sendCode.mutate()}>
              {secondsLeft > 0 ? `Resend in ${secondsLeft}s` : 'Resend code'}
            </Button>
            <Button variant="link" onClick={() => setCodeSent(null)}>
              Use another email
            </Button>
          </div>
        </div>
      )}
    </Card>
  )
}

// ---------------------------------------------------------------- Signed in

function Dashboard() {
  const owner = useQuery({ queryKey: keys.me, queryFn: () => api<Owner>('/me', { auth: 'owner' }) })
  const vehicles = useQuery({
    queryKey: keys.myVehicles,
    queryFn: () => api<{ items: MyVehicle[] }>('/me/vehicles', { auth: 'owner' }),
    refetchInterval: 10000,
  })

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold">My vehicles</h1>
          <p className="text-sm text-slate-500">{owner.data?.email}</p>
        </div>
        <div className="flex gap-2">
          <LinkButton to="/register" variant="secondary">
            <Plus /> Add vehicle
          </LinkButton>
          <Button variant="ghost" onClick={() => useOwnerAuth.getState().signOut()}>
            <LogOut /> Sign out
          </Button>
        </div>
      </div>

      {vehicles.isLoading && <Spinner />}
      {vehicles.error && <ErrorBox>{errorMessage(vehicles.error)}</ErrorBox>}
      {vehicles.data?.items.length === 0 && (
        <Card>
          <EmptyState icon={<Car />} title="No vehicles yet">
            <Link to="/register" className="font-semibold text-brand-700">
              Register your first vehicle
            </Link>
          </EmptyState>
        </Card>
      )}
      <div className="grid gap-4 md:grid-cols-2">
        {vehicles.data?.items.map((vehicle) => <VehicleCard key={vehicle.id} vehicle={vehicle} />)}
      </div>

      {owner.data && <ContactDetails owner={owner.data} />}
      <TicketHistory />
    </div>
  )
}

function VehicleCard({ vehicle }: { vehicle: MyVehicle }) {
  const [editing, setEditing] = useState(false)
  const [confirmRemove, setConfirmRemove] = useState(false)
  const ticket = vehicle.activeTicket

  const remove = useMutation({
    mutationFn: () => api(`/me/vehicles/${vehicle.id}`, { method: 'DELETE', auth: 'owner' }),
    onSuccess: () => {
      toast.success(`${vehicle.vehicleNumber} removed`)
      queryClient.invalidateQueries({ queryKey: keys.myVehicles })
    },
    onError: (err) => toast.error(errorMessage(err)),
    onSettled: () => setConfirmRemove(false),
  })

  return (
    <Card>
      <div className="flex items-center gap-4 p-5">
        <div className="grid size-14 place-items-center rounded-xl bg-slate-100">
          <VehicleIcon type={vehicle.vehicleType} hue={215} size={48} />
        </div>
        <div className="min-w-0 flex-1">
          <p className="font-mono text-lg font-bold">{vehicle.vehicleNumber}</p>
          <p className="text-sm text-slate-500">
            {vehicleTypeLabel[vehicle.vehicleType]}
            {vehicle.registrationId && ` · RC ${vehicle.registrationId}`}
          </p>
        </div>
        {ticket ? <Badge color="blue">Parked</Badge> : vehicle.status === 'ACTIVE' ? <Badge color="green">Ready</Badge> : <Badge color="amber">Unverified</Badge>}
      </div>

      {ticket && <RunningFee ticket={ticket} />}

      <div className="flex justify-end gap-2 border-t border-slate-100 px-5 py-3">
        <Button size="small" variant="ghost" onClick={() => setEditing(true)}>
          <Pencil /> Edit
        </Button>
        <Button size="small" variant="ghost" className="text-red-600 hover:bg-red-50" disabled={!!ticket} onClick={() => setConfirmRemove(true)}>
          <Trash2 /> Remove
        </Button>
      </div>

      {editing && <EditVehicle vehicle={vehicle} onClose={() => setEditing(false)} />}
      <Modal
        open={confirmRemove}
        onClose={() => setConfirmRemove(false)}
        title={`Remove ${vehicle.vehicleNumber}?`}
        description="It won't be able to enter until it's registered again."
        icon={<Trash2 />}
        color="danger"
      >
        <ModalFooter>
          <Button variant="secondary" onClick={() => setConfirmRemove(false)}>
            Keep
          </Button>
          <Button variant="danger" loading={remove.isPending} onClick={() => remove.mutate()}>
            Remove vehicle
          </Button>
        </ModalFooter>
      </Modal>
    </Card>
  )
}

/** The live estimate for a parked vehicle. Updates every 15 s. The real fee comes from the PAY kiosk. */
function RunningFee({ ticket }: { ticket: Ticket }) {
  const [now, setNow] = useState(Date.now())
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 15000)
    return () => clearInterval(timer)
  }, [])
  const fee = calculateFee(minutesSince(ticket.entryTime, now), ticket.rate)

  return (
    <div className="mx-5 mb-4 rounded-xl bg-brand-50 p-4 text-sm ring-1 ring-brand-100">
      <div className="flex items-baseline justify-between">
        <span className="font-semibold text-brand-900">Spot {ticket.spotCode}</span>
        <span className="text-xs text-brand-800">{ticket.ticketNumber}</span>
      </div>
      <p className="mt-1 text-slate-600">In since {dateTimeIST(ticket.entryTime)}</p>
      <div className="mt-2 flex items-baseline justify-between">
        <span className="text-slate-600">{humanDuration(fee.durationMinutes)} so far</span>
        <span className="text-lg font-bold text-slate-900">{rupees(fee.feePaise)}</span>
      </div>
      <p className="mt-1 text-xs text-slate-500">Running estimate. {rateSummary(ticket.rate)}.</p>
    </div>
  )
}

function EditVehicle({ vehicle, onClose }: { vehicle: MyVehicle; onClose: () => void }) {
  const [registrationId, setRegistrationId] = useState(vehicle.registrationId ?? '')
  const save = useMutation({
    mutationFn: () => api(`/me/vehicles/${vehicle.id}`, { method: 'PATCH', auth: 'owner', body: { registrationId } }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: keys.myVehicles })
      toast.success('Saved')
      onClose()
    },
  })
  return (
    <Modal open onClose={onClose} title={`Edit ${vehicle.vehicleNumber}`} icon={<Pencil />}>
      <form
        className="space-y-3"
        onSubmit={(e) => {
          e.preventDefault()
          save.mutate()
        }}
      >
        <Field label="Registration (RC) number" optional>
          <Input value={registrationId} onChange={(e) => setRegistrationId(e.target.value)} />
        </Field>
        {save.error && <ErrorBox>{errorMessage(save.error)}</ErrorBox>}
        <ModalFooter>
          <Button type="button" variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" loading={save.isPending}>
            Save
          </Button>
        </ModalFooter>
      </form>
    </Modal>
  )
}

/** Change mobile, or change email (a code is sent to the new address first). */
function ContactDetails({ owner }: { owner: Owner }) {
  const [mobile, setMobile] = useState(owner.mobile ?? '')
  const [newEmail, setNewEmail] = useState('')
  const [codeSent, setCodeSent] = useState<CodeSent | null>(null)

  const saveMobile = useMutation({
    mutationFn: () => api('/me', { method: 'PATCH', auth: 'owner', body: { mobile } }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: keys.me })
      toast.success('Mobile saved')
    },
  })
  const startEmailChange = useMutation({
    mutationFn: () => api<CodeSent>('/me/email-change', { method: 'POST', auth: 'owner', body: { email: newEmail } }),
    onSuccess: setCodeSent,
  })
  const confirmEmailChange = useMutation({
    mutationFn: (code: string) => api<OwnerSession>('/me/email-change/verify', { method: 'POST', auth: 'owner', body: { code } }),
    onSuccess: (session) => {
      useOwnerAuth.getState().signIn(session)
      setCodeSent(null)
      setNewEmail('')
      queryClient.invalidateQueries({ queryKey: keys.me })
      toast.success('Email changed', { description: 'Tickets already open keep sending to the old address.' })
    },
  })

  return (
    <Card>
      <CardHeader title="Contact details" />
      <div className="grid gap-6 px-5 py-4 md:grid-cols-2">
        <form
          className="space-y-3"
          onSubmit={(e) => {
            e.preventDefault()
            saveMobile.mutate()
          }}
        >
          <Field label="Mobile" optional error={saveMobile.error ? errorMessage(saveMobile.error) : undefined}>
            <Input type="tel" value={mobile} onChange={(e) => setMobile(e.target.value)} placeholder="+91 98765 43210" />
          </Field>
          <Button type="submit" variant="secondary" size="small" loading={saveMobile.isPending}>
            Save mobile
          </Button>
        </form>

        {!codeSent ? (
          <form
            className="space-y-3"
            onSubmit={(e) => {
              e.preventDefault()
              startEmailChange.mutate()
            }}
          >
            <Field
              label="Change email"
              hint={`Currently ${owner.email}. We'll send a code to the new address.`}
              error={startEmailChange.error ? errorMessage(startEmailChange.error) : undefined}
            >
              <Input type="email" value={newEmail} onChange={(e) => setNewEmail(e.target.value)} placeholder="new@example.com" />
            </Field>
            <Button type="submit" variant="secondary" size="small" loading={startEmailChange.isPending} disabled={!newEmail}>
              Send code
            </Button>
          </form>
        ) : (
          <div className="space-y-3">
            <p className="text-sm text-slate-600">
              Enter the code sent to <strong>{codeSent.codeSentTo}</strong>.
              {codeSent.devCode && <span className="ml-1 font-mono text-amber-700">(demo: {codeSent.devCode})</span>}
            </p>
            <CodeInput onComplete={(code) => confirmEmailChange.mutate(code)} invalid={!!confirmEmailChange.error} disabled={confirmEmailChange.isPending} />
            {confirmEmailChange.error && <p className="text-sm text-red-600">{errorMessage(confirmEmailChange.error)}</p>}
            <Button variant="link" size="small" onClick={() => setCodeSent(null)}>
              Cancel
            </Button>
          </div>
        )}
      </div>
    </Card>
  )
}

function TicketHistory() {
  const [page, setPage] = useState(1)
  const tickets = useQuery({
    queryKey: [...keys.myTickets, page],
    queryFn: () => api<Page<Ticket>>(`/me/tickets${queryString({ page, pageSize: 10 })}`, { auth: 'owner' }),
  })
  const items = tickets.data?.items ?? []

  return (
    <Card>
      <CardHeader title="Parking history" />
      <Table>
        <thead>
          <tr>
            <th>Ticket</th>
            <th>Vehicle</th>
            <th>Spot</th>
            <th>Entry</th>
            <th>Duration</th>
            <th>Paid</th>
            <th>Status</th>
          </tr>
        </thead>
        <tbody>
          {tickets.isLoading && <MessageRow columns={7} loading />}
          {!tickets.isLoading && items.length === 0 && <MessageRow columns={7}>No visits yet.</MessageRow>}
          {items.map((t) => (
            <tr key={t.id}>
              <td className="font-mono text-xs">{t.ticketNumber}</td>
              <td className="font-mono font-semibold">{t.vehicleNumber}</td>
              <td>{t.spotCode}</td>
              <td>{dateTimeIST(t.entryTime)}</td>
              <td>{t.durationMinutes !== null ? humanDuration(t.durationMinutes) : '—'}</td>
              <td>
                {t.status === 'CLOSED' ? (
                  <>
                    {rupees(t.paidPaise)} <span className="text-xs text-slate-500">{paymentMethodLabel[t.paymentMethod ?? '']}</span>
                  </>
                ) : (
                  '—'
                )}
              </td>
              <td>
                <TicketStatusBadge status={t.status} />
              </td>
            </tr>
          ))}
        </tbody>
      </Table>
      {tickets.data && tickets.data.total > 10 && <Pager page={page} pageSize={10} total={tickets.data.total} onPage={setPage} />}
    </Card>
  )
}
