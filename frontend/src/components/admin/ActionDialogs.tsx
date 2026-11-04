import { useMutation, useQuery } from '@tanstack/react-query'
import { LogIn, LogOut, MessageSquareText } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { toast } from 'sonner'
import { api } from '@/api/client'
import type { Items, Spot, Ticket } from '@/api/types'
import { Button, DetailList, ErrorBox, Field, Input, Modal, ModalFooter, Select, Textarea } from '@/components/ui'
import { errorMessage } from '@/lib/errors'
import { calculateFee, feeBreakdown, minutesSince } from '@/lib/fee'
import { dateTimeIST, humanDuration, rupees, spotTypeLabel } from '@/lib/format'
import { keys, refreshLotData } from '@/lib/queryClient'
import { normalisePlate } from '@/lib/validation'

// Popups for admin actions. Every admin action asks for a reason, which goes to the audit log (FR-40).

/**
 * Asks for a reason, then calls `onConfirm(reason)`.
 * Extra inputs (like "new owner email") can be passed as children.
 */
export function ReasonDialog({
  title,
  description,
  confirmLabel,
  onConfirm,
  onClose,
  children,
}: {
  title: string
  description?: string
  confirmLabel: string
  onConfirm: (reason: string) => Promise<unknown>
  onClose: () => void
  children?: ReactNode
}) {
  const [reason, setReason] = useState('')
  const confirm = useMutation({
    mutationFn: () => onConfirm(reason),
    onSuccess: () => {
      refreshLotData()
      onClose()
    },
  })
  return (
    <Modal open onClose={onClose} title={title} description={description} icon={<MessageSquareText />}>
      <form
        className="space-y-3"
        onSubmit={(e) => {
          e.preventDefault()
          confirm.mutate()
        }}
      >
        {children}
        <Field label="Reason" hint="Saved in the audit log.">
          <Textarea required value={reason} onChange={(e) => setReason(e.target.value)} autoFocus />
        </Field>
        {confirm.error && <ErrorBox>{errorMessage(confirm.error)}</ErrorBox>}
        <ModalFooter>
          <Button type="button" variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" loading={confirm.isPending} disabled={reason.trim().length < 3}>
            {confirmLabel}
          </Button>
        </ModalFooter>
      </form>
    </Modal>
  )
}

/** Manual check-in, e.g. when the camera couldn't read the plate (FR-38). */
export function ManualCheckInDialog({ plate = '', scanId, onClose }: { plate?: string; scanId?: number; onClose: () => void }) {
  const [vehicleNumber, setVehicleNumber] = useState(plate)
  const [spotId, setSpotId] = useState('') // '' = let the server pick the nearest spot
  const [reason, setReason] = useState('')

  const spots = useQuery({ queryKey: keys.spots, queryFn: () => api<Items<Spot>>('/admin/spots', { auth: 'admin' }) })
  const freeSpots = (spots.data?.items ?? []).filter((spot) => spot.status === 'AVAILABLE')

  const checkIn = useMutation({
    mutationFn: () =>
      api<Ticket>('/admin/tickets', {
        method: 'POST',
        auth: 'admin',
        body: { vehicleNumber: normalisePlate(vehicleNumber), spotId: spotId || undefined, reason, scanId },
      }),
    onSuccess: (ticket) => {
      toast.success(`Checked in ${ticket.vehicleNumber}`, { description: `Spot ${ticket.spotCode} · ${ticket.ticketNumber}` })
      refreshLotData()
      onClose()
    },
  })

  return (
    <Modal open onClose={onClose} title="Manual check-in" description="Opens a ticket and assigns a spot, as if the gate had let the vehicle in." icon={<LogIn />}>
      <form
        className="space-y-3"
        onSubmit={(e) => {
          e.preventDefault()
          checkIn.mutate()
        }}
      >
        <Field label="Vehicle number">
          <Input required value={vehicleNumber} onChange={(e) => setVehicleNumber(e.target.value)} className="font-mono uppercase" />
        </Field>
        <Field label="Spot" hint="Leave on automatic to use the normal allocation rules.">
          <Select value={spotId} onChange={(e) => setSpotId(e.target.value)} className="w-full">
            <option value="">Automatic (nearest compatible)</option>
            {freeSpots.map((spot) => (
              <option key={spot.id} value={spot.id}>
                {spot.code} · {spotTypeLabel[spot.spotType]} · floor {spot.floor}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Reason">
          <Textarea required value={reason} onChange={(e) => setReason(e.target.value)} placeholder="Plate unreadable; checked RC at the gate." />
        </Field>
        {checkIn.error && <ErrorBox>{errorMessage(checkIn.error)}</ErrorBox>}
        <ModalFooter>
          <Button type="button" variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" loading={checkIn.isPending} disabled={reason.trim().length < 3}>
            Check in
          </Button>
        </ModalFooter>
      </form>
    </Modal>
  )
}

/** Close a ticket by hand: charge the fee at the office, or waive it (FR-38). */
export function ManualExitDialog({ ticket, onClose }: { ticket: Ticket; onClose: () => void }) {
  const [mode, setMode] = useState<'CHARGE' | 'WAIVE'>('CHARGE')
  const [reason, setReason] = useState('')

  // If the exit kiosk already fixed the fee, use it; otherwise show what it would be now.
  const fee =
    ticket.status === 'PAYMENT_PENDING' && ticket.feePaise !== null
      ? { ...calculateFee(ticket.durationMinutes ?? 0, ticket.rate), feePaise: ticket.feePaise }
      : calculateFee(minutesSince(ticket.entryTime), ticket.rate)

  const close = useMutation({
    mutationFn: () => api<Ticket>(`/admin/tickets/${ticket.id}/manual-exit`, { method: 'POST', auth: 'admin', body: { mode, reason } }),
    onSuccess: (closed) => {
      toast.success(`${closed.vehicleNumber} checked out`, { description: mode === 'WAIVE' ? 'Fee waived' : `Charged ${rupees(closed.paidPaise)}` })
      refreshLotData()
      onClose()
    },
  })

  return (
    <Modal open onClose={onClose} title={`Manual exit · ${ticket.vehicleNumber}`} description="Closes the ticket, frees the spot and sends the exit email." icon={<LogOut />}>
      <DetailList
        items={[
          ['Ticket', <span className="font-mono">{ticket.ticketNumber}</span>],
          ['Spot', ticket.spotCode],
          ['Entry', dateTimeIST(ticket.entryTime)],
          ['Duration', humanDuration(fee.durationMinutes)],
          ['Fee', <span className="font-semibold">{feeBreakdown(fee, ticket.rate)}</span>],
        ]}
      />
      <form
        className="mt-4 space-y-3"
        onSubmit={(e) => {
          e.preventDefault()
          close.mutate()
        }}
      >
        <div className="grid grid-cols-2 gap-2" role="radiogroup" aria-label="Charge or waive">
          <ChoiceButton selected={mode === 'CHARGE'} onClick={() => setMode('CHARGE')} title={`Charge ${rupees(fee.feePaise)}`} text="Paid at the office" />
          <ChoiceButton selected={mode === 'WAIVE'} onClick={() => setMode('WAIVE')} title="Waive the fee" text="No payment taken" />
        </div>
        <Field label="Reason">
          <Textarea required value={reason} onChange={(e) => setReason(e.target.value)} />
        </Field>
        {close.error && <ErrorBox>{errorMessage(close.error)}</ErrorBox>}
        <ModalFooter>
          <Button type="button" variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" loading={close.isPending} disabled={reason.trim().length < 3}>
            Close ticket
          </Button>
        </ModalFooter>
      </form>
    </Modal>
  )
}

function ChoiceButton({ selected, onClick, title, text }: { selected: boolean; onClick: () => void; title: string; text: string }) {
  return (
    <button
      type="button"
      role="radio"
      aria-checked={selected}
      onClick={onClick}
      className={`rounded-lg border-2 px-3 py-2 text-left text-sm ${selected ? 'border-brand-600 bg-brand-50' : 'border-slate-200'}`}
    >
      <span className="block font-semibold">{title}</span>
      <span className="text-xs text-slate-500">{text}</span>
    </button>
  )
}

/** Small text input used inside ReasonDialog, e.g. for the new owner's email. */
export function ExtraInput({ label, value, onChange, type = 'text' }: { label: string; value: string; onChange: (v: string) => void; type?: string }) {
  return (
    <Field label={label}>
      <Input type={type} required value={value} onChange={(e) => onChange(e.target.value)} />
    </Field>
  )
}
