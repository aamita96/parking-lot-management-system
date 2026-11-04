import { useMutation } from '@tanstack/react-query'
import { Banknote, CarFront, CircleX, LogOut, RotateCw, Smartphone, TicketCheck, TriangleAlert } from 'lucide-react'
import { QRCodeSVG } from 'qrcode.react'
import { useEffect } from 'react'
import type { EventSummary } from '@/api/types'
import { EmailStatusBadge } from '@/components/StatusBadges'
import { Button, DetailList, ErrorBox, Modal, ModalFooter } from '@/components/ui'
import { errorMessage, rejectReason } from '@/lib/errors'
import { clockIST, dateTimeIST, humanDuration, rupees, spotTypeLabel, vehicleTypeLabel } from '@/lib/format'
import { pay, scanAtEntry, scanAtExit } from './gateCalls'
import { useSim } from './store'
import type { Popup } from './types'

// The popups at the gates (SRS 11.3). Several may be waiting; only the first one shows.

export function GatePopups({ events }: { events: EventSummary[] }) {
  const popup = useSim((s) => s.popups[0])
  if (!popup) return null
  // `key` makes React start a fresh popup for each vehicle.
  if (popup.kind === 'entry') return <EntryPopup key={popup.vehicleId} popup={popup} events={events} />
  if (popup.kind === 'refused') return <RefusedPopup key={popup.vehicleId} popup={popup} />
  if (popup.kind === 'payment') return <PaymentPopup key={popup.vehicleId} popup={popup} />
  return <ErrorPopup key={popup.vehicleId} popup={popup} />
}

/** In auto mode, a popup presses its main button by itself after a short wait. */
function useAutoPress(pressButton: () => void, waitMs: number) {
  const autoMode = useSim((s) => s.autoMode)
  const speed = useSim((s) => s.speed)
  useEffect(() => {
    if (!autoMode) return
    const timer = setTimeout(pressButton, waitMs / Math.min(speed, 2))
    return () => clearTimeout(timer)
    // Run once when the popup opens (or auto mode is switched on), not on every render.
  }, [autoMode])
  return autoMode
}

function EntryPopup({ popup, events }: { popup: Extract<Popup, { kind: 'entry' }>; events: EventSummary[] }) {
  const answer = popup.data
  const park = () => useSim.getState().driveToSpot(popup.vehicleId)
  const autoMode = useAutoPress(park, 3000)
  // The event list is polled every 2 s, so the email status updates live (queued → sent).
  const event = events.find((e) => e.ticketId === answer.ticket.id && e.eventType === 'VehicleEntered')

  return (
    <Modal open onClose={park} title="Vehicle at entry gate" description="Scan admitted. The barrier lifts when the vehicle drives on." icon={<TicketCheck />} color="success">
      <DetailList
        items={[
          ['Vehicle number', <span className="font-mono text-base font-bold">{answer.vehicle.vehicleNumber}</span>],
          ['Owner email', answer.owner.email ?? answer.owner.emailMasked],
          ['Vehicle type', vehicleTypeLabel[answer.vehicle.vehicleType]],
          ['Assigned spot', <span><strong className="text-lg">{answer.spot.code}</strong> · {spotTypeLabel[answer.spot.spotType]}</span>],
          ['Ticket number', <span className="font-mono">{answer.ticket.ticketNumber}</span>],
          ['Entry time', dateTimeIST(answer.ticket.entryTime)],
          ['Entry email', <EmailStatusBadge status={event?.emailStatus ?? 'WAITING'} />],
        ]}
      />
      <ModalFooter>
        {autoMode && <span className="mr-auto text-xs text-slate-500">Auto mode: parking in 3 s…</span>}
        <Button onClick={park} autoFocus>
          <CarFront /> Park
        </Button>
      </ModalFooter>
    </Modal>
  )
}

function RefusedPopup({ popup }: { popup: Extract<Popup, { kind: 'refused' }> }) {
  // Refused at the IN gate: the vehicle turns back. Refused at exit: just remove it from the drawing.
  const goAway = () => (popup.stage === 'entry' ? useSim.getState().turnBack(popup.vehicleId) : useSim.getState().removeVehicle(popup.vehicleId))
  const autoMode = useAutoPress(goAway, 3000)
  const showQrCode = popup.stage === 'entry' && popup.registerUrl && (popup.code === 'NOT_REGISTERED' || popup.code === 'NOT_VERIFIED')

  return (
    <Modal open onClose={goAway} title={popup.stage === 'entry' ? 'Entry refused' : 'Exit refused'} description="The barrier stays closed." icon={<CircleX />} color="danger">
      <DetailList
        items={[
          ['Vehicle number', <span className="font-mono text-base font-bold">{popup.plate}</span>],
          ['Reason', <span className="text-red-700">{rejectReason[popup.code] ?? popup.code}</span>],
        ]}
      />
      <p className="mt-3 text-sm text-slate-600">{popup.message}</p>
      {showQrCode && (
        <div className="mt-4 flex items-center gap-4 rounded-xl border border-slate-200 p-3">
          <QRCodeSVG value={popup.registerUrl!} size={112} marginSize={1} role="img" aria-label="Registration QR code" />
          <div className="text-sm">
            <p className="font-semibold text-slate-900">Scan to register</p>
            <p className="mt-1 break-all text-xs text-slate-500">{popup.registerUrl}</p>
          </div>
        </div>
      )}
      {popup.stage === 'exit' && <p className="mt-2 text-xs text-slate-500">Sent to the admin exception queue.</p>}
      <ModalFooter>
        {autoMode && <span className="mr-auto text-xs text-slate-500">Auto mode: turning back…</span>}
        <Button variant="secondary" onClick={goAway} autoFocus>
          {popup.stage === 'entry' ? 'Turn back' : 'Remove vehicle'}
        </Button>
      </ModalFooter>
    </Modal>
  )
}

function PaymentPopup({ popup }: { popup: Extract<Popup, { kind: 'payment' }> }) {
  const answer = popup.data
  const isFree = answer.result === 'EXIT_CLOSED' // within the free minutes: already closed, nothing to pay

  const payment = useMutation({
    mutationFn: (method: 'SIMULATED_UPI' | 'SIMULATED_CASH') => pay(popup.vehicleId, answer.ticket.id, answer.feePaise, method),
  })
  const exitForFree = () => useSim.getState().driveOut(popup.vehicleId)

  const autoMode = useAutoPress(() => (isFree ? exitForFree() : payment.mutate(Math.random() < 0.7 ? 'SIMULATED_UPI' : 'SIMULATED_CASH')), 2600)
  const sameDay = answer.exitTime.slice(0, 10) === answer.entryTime.slice(0, 10)

  return (
    <Modal
      open
      onClose={() => useSim.getState().closePopup(popup.vehicleId)}
      title="Payment at exit"
      description="PAY kiosk · payment is simulated, no money moves."
      icon={<Banknote />}
      color={isFree ? 'success' : 'default'}
    >
      <DetailList
        items={[
          ['Vehicle number', <span className="font-mono text-base font-bold">{answer.vehicle.vehicleNumber}</span>],
          ['Owner email', answer.owner.email ?? answer.owner.emailMasked],
          ['Entry → exit', sameDay ? `${clockIST(answer.entryTime)} → ${clockIST(answer.exitTime)} IST` : `${dateTimeIST(answer.entryTime)} → ${dateTimeIST(answer.exitTime)}`],
          ['Duration', isFree ? humanDuration(answer.durationMinutes) : `${humanDuration(answer.durationMinutes)}, billed as ${answer.billableHours} h`],
          ['Fee', <span className="font-semibold">{answer.breakdown}</span>],
        ]}
      />
      <div className="mt-4 flex items-center justify-between rounded-xl bg-slate-50 px-4 py-3">
        <span className="text-sm font-medium text-slate-600">{isFree ? 'No charge, within the free minutes' : 'To pay'}</span>
        <span className="text-2xl font-extrabold text-slate-900">{rupees(answer.feePaise)}</span>
      </div>
      {payment.error && (
        <div className="mt-3">
          <ErrorBox>{errorMessage(payment.error)}</ErrorBox>
        </div>
      )}
      <ModalFooter>
        {autoMode && <span className="mr-auto text-xs text-slate-500">Auto mode: paying…</span>}
        {isFree ? (
          <Button variant="success" onClick={exitForFree} autoFocus>
            <LogOut /> Exit
          </Button>
        ) : (
          <>
            <Button variant="secondary" disabled={payment.isPending} onClick={() => payment.mutate('SIMULATED_CASH')}>
              <Banknote /> Pay cash
            </Button>
            <Button variant="success" loading={payment.isPending} onClick={() => payment.mutate('SIMULATED_UPI')} autoFocus>
              <Smartphone /> Pay {rupees(answer.feePaise)} by UPI
            </Button>
          </>
        )}
      </ModalFooter>
    </Modal>
  )
}

function ErrorPopup({ popup }: { popup: Extract<Popup, { kind: 'error' }> }) {
  // The vehicle waits at the gate until the call works (SRS 11.5).
  const retry = () => {
    useSim.getState().closePopup(popup.vehicleId)
    if (popup.stage === 'entry') scanAtEntry(popup.vehicleId)
    else scanAtExit(popup.vehicleId)
  }
  return (
    <Modal open onClose={() => useSim.getState().closePopup(popup.vehicleId)} title="Gate call failed" description="The vehicle is waiting at the gate." icon={<TriangleAlert />} color="warning">
      <ErrorBox>{popup.message}</ErrorBox>
      <p className="mt-3 text-xs text-slate-500">Retry sends the same Idempotency-Key, so nothing happens twice.</p>
      <ModalFooter>
        <Button variant="ghost" onClick={() => useSim.getState().removeVehicle(popup.vehicleId)}>
          Remove vehicle
        </Button>
        <Button onClick={retry} autoFocus>
          <RotateCw /> Retry
        </Button>
      </ModalFooter>
    </Modal>
  )
}
