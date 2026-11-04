import { ApiError } from '@/api/client'

// Turns an API error code into a plain sentence for the screen (SRS 10.6).
const messages: Record<string, string> = {
  NOT_REGISTERED: 'This vehicle is not registered yet.',
  NOT_VERIFIED: "The owner hasn't confirmed their email yet.",
  ALREADY_PARKED: 'This vehicle is already parked.',
  UNREADABLE_PLATE: "The plate couldn't be read.",
  NO_ACTIVE_TICKET: 'No open ticket for this vehicle.',
  NO_SPOT_AVAILABLE: 'No free spot for this vehicle type right now.',
  AMOUNT_MISMATCH: "The amount doesn't match the fee.",
  TICKET_ALREADY_CLOSED: 'This ticket is already paid and closed.',
  PAYMENT_NOT_DUE: 'Scan the vehicle at the exit before paying.',
  SPOT_OCCUPIED: "An occupied spot can't be taken out of service.",
  VEHICLE_ALREADY_REGISTERED: 'This number is registered to someone else.',
  CODE_INVALID: "That code isn't right. Check the email and try again.",
  CODE_EXPIRED: 'That code has expired. Ask for a new one.',
  RATE_LIMITED: 'Too many tries. Please wait a little and try again.',
  UNAUTHENTICATED: 'Please sign in again.',
  FORBIDDEN: "You don't have access to this.",
  EMAIL_UNAVAILABLE: "We couldn't send the email. Try again in a minute.",
  VEHICLE_PARKED: 'This vehicle is parked right now. Remove it after it leaves.',
  EMAIL_IN_USE: 'Another owner already uses this email.',
  NETWORK: "Can't reach the server. Check your connection.",
  INTERNAL: 'Something went wrong on our side. Please try again.',
}

export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.code === 'VALIDATION_ERROR' && Array.isArray(err.body.errors)) {
      return err.body.errors.map((e: { message: string }) => e.message).join(' ')
    }
    return messages[err.code] ?? err.message
  }
  if (err instanceof Error) return err.message
  return 'Something went wrong.'
}

/** Short reasons for gate refusals, shown in the simulator and the exception queue. */
export const rejectReason: Record<string, string> = {
  NOT_REGISTERED: 'Not registered',
  NOT_VERIFIED: 'Email not verified',
  ALREADY_PARKED: 'Already parked',
  NO_SPOT_AVAILABLE: 'No free spot for this type',
  UNREADABLE_PLATE: 'Plate unreadable',
  NO_ACTIVE_TICKET: 'No open ticket',
}
