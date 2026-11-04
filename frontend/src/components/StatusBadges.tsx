import { Badge, type BadgeColor } from '@/components/ui'

// Coloured labels for statuses. Each entry is [colour, text].

const ticketStatus: Record<string, [BadgeColor, string]> = {
  ACTIVE: ['blue', 'Parked'],
  PAYMENT_PENDING: ['amber', 'Awaiting payment'],
  CLOSED: ['green', 'Closed'],
  CANCELLED: ['slate', 'Cancelled'],
}

export function TicketStatusBadge({ status }: { status: string }) {
  const [color, text] = ticketStatus[status] ?? ['slate', status]
  return <Badge color={color}>{text}</Badge>
}

const emailStatus: Record<string, [BadgeColor, string]> = {
  WAITING: ['slate', 'Waiting for Kafka'],
  QUEUED: ['amber', 'Email queued'],
  SENT: ['green', 'Email sent'],
  FAILED: ['red', 'Email failed'],
  DEFERRED: ['violet', 'Email deferred (quota)'],
}

/** Email status of an entry or exit email (FR-32). */
export function EmailStatusBadge({ status }: { status: string }) {
  const [color, text] = emailStatus[status] ?? ['slate', status]
  return <Badge color={color}>{text}</Badge>
}

const kafkaStatus: Record<string, [BadgeColor, string]> = {
  PENDING: ['slate', 'In outbox'],
  RETRYING: ['red', 'Kafka retrying'],
  PUBLISHED: ['green', 'On Kafka'],
}

/** Whether the event has been published to Kafka yet (FR-47). */
export function KafkaStatusBadge({ status }: { status: string }) {
  const [color, text] = kafkaStatus[status] ?? ['slate', status]
  return <Badge color={color}>{text}</Badge>
}
