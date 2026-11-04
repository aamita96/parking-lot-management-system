import { Clock } from 'lucide-react'
import type { EventSummary } from '@/api/types'
import { EmailStatusBadge, KafkaStatusBadge } from '@/components/StatusBadges'
import { Badge } from '@/components/ui'
import { clockIST, rupees } from '@/lib/format'

// The side panel with the last 20 entry/exit events and whether they reached Kafka and email (FR-47).
export function EventList({ events, loading }: { events: EventSummary[]; loading: boolean }) {
  return (
    <section className="flex h-full flex-col rounded-xl border border-slate-200 bg-white shadow-card">
      <div className="flex items-center justify-between border-b border-slate-100 px-4 py-3">
        <h2 className="text-sm font-semibold">Ticket events</h2>
        <span className="text-xs text-slate-400">Last 20 · Kafka + email</span>
      </div>
      <ol className="flex-1 divide-y divide-slate-100 overflow-y-auto">
        {!loading && events.length === 0 && <li className="px-4 py-8 text-center text-sm text-slate-500">No events yet. Send a vehicle.</li>}
        {events.map((event) => (
          <li key={event.eventId} className="px-4 py-2.5">
            <div className="flex items-center justify-between gap-2">
              <span className="inline-flex items-center gap-1.5">
                <Badge color={event.eventType === 'VehicleEntered' ? 'blue' : 'green'}>{event.eventType === 'VehicleEntered' ? 'Entered' : 'Exited'}</Badge>
                <span className="font-mono text-sm font-bold">{event.vehicleNumber}</span>
              </span>
              <span className="flex items-center gap-1 text-[11px] text-slate-400">
                <Clock className="size-3" /> {clockIST(event.occurredAt, true)}
              </span>
            </div>
            <div className="mt-1.5 flex flex-wrap items-center gap-1.5 text-xs text-slate-500">
              <span>Spot {event.spotCode}</span>
              {event.eventType === 'VehicleExited' && event.feePaise !== null && <span>· {rupees(event.feePaise)}</span>}
              {!event.simulated && <Badge color="violet">Real</Badge>}
              <span className="flex-1" />
              <KafkaStatusBadge status={event.kafkaStatus} />
              <EmailStatusBadge status={event.emailStatus} />
            </div>
          </li>
        ))}
      </ol>
    </section>
  )
}
