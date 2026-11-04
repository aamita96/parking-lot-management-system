// Money is integer paise; times are UTC from the API and shown in IST (SRS 1.4).

const IST = 'Asia/Kolkata'

const rupeeFmt = new Intl.NumberFormat('en-IN', { style: 'currency', currency: 'INR', minimumFractionDigits: 2 })
const rupeeShortFmt = new Intl.NumberFormat('en-IN', { style: 'currency', currency: 'INR', minimumFractionDigits: 0, maximumFractionDigits: 2 })

/** ₹1,300.00 */
export function rupees(paise: number | null | undefined): string {
  return rupeeFmt.format((paise ?? 0) / 100)
}

/** ₹40 or ₹40.50 */
export function rupeesShort(paise: number | null | undefined): string {
  return rupeeShortFmt.format((paise ?? 0) / 100)
}

const dateTimeFmt = new Intl.DateTimeFormat('en-IN', {
  timeZone: IST,
  day: '2-digit',
  month: 'short',
  year: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hour12: true,
})
const clockFmt = new Intl.DateTimeFormat('en-IN', { timeZone: IST, hour: '2-digit', minute: '2-digit', hour12: true })
const clockSecFmt = new Intl.DateTimeFormat('en-IN', {
  timeZone: IST,
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hour12: true,
})
const dateFmt = new Intl.DateTimeFormat('en-IN', { timeZone: IST, day: '2-digit', month: 'short', year: 'numeric' })

const upperAmPm = (s: string) => s.replace(/\b(am|pm)\b/gi, (m) => m.toUpperCase())

/** 04 Nov 2026, 10:05 AM IST */
export function dateTimeIST(iso: string | null | undefined): string {
  if (!iso) return '—'
  const d = new Date(iso)
  const parts = Object.fromEntries(dateTimeFmt.formatToParts(d).map((p) => [p.type, p.value]))
  return `${parts.day} ${parts.month} ${parts.year}, ${parts.hour}:${parts.minute} ${(parts.dayPeriod ?? '').toUpperCase()} IST`
}

/** 10:05 AM */
export function clockIST(iso: string | null | undefined, seconds = false): string {
  if (!iso) return '—'
  return upperAmPm((seconds ? clockSecFmt : clockFmt).format(new Date(iso)))
}

/** 04 Nov 2026 */
export function dateIST(iso: string | null | undefined): string {
  if (!iso) return '—'
  return dateFmt.format(new Date(iso))
}

/** 3 h 20 min · 45 min · 1 d 2 h 10 min */
export function humanDuration(minutes: number): string {
  if (minutes < 60) return `${Math.max(0, Math.floor(minutes))} min`
  const d = Math.floor(minutes / 1440)
  const h = Math.floor((minutes % 1440) / 60)
  const m = Math.floor(minutes % 60)
  return [d && `${d} d`, h && `${h} h`, m && `${m} min`].filter(Boolean).join(' ')
}

/** "2 min ago" style relative time for event panels. */
export function ago(iso: string | null | undefined, now = Date.now()): string {
  if (!iso) return '—'
  const s = Math.max(0, Math.round((now - new Date(iso).getTime()) / 1000))
  if (s < 5) return 'just now'
  if (s < 60) return `${s}s ago`
  if (s < 3600) return `${Math.floor(s / 60)} min ago`
  return `${Math.floor(s / 3600)} h ago`
}

export const vehicleTypeLabel: Record<string, string> = { MOTORCYCLE: 'Motorcycle', CAR: 'Car', TRUCK: 'Truck' }
export const spotTypeLabel: Record<string, string> = { COMPACT: 'Compact', REGULAR: 'Regular', OVERSIZED: 'Oversized' }
export const spotTypeLetter: Record<string, string> = { COMPACT: 'C', REGULAR: 'R', OVERSIZED: 'L' }

export const paymentMethodLabel: Record<string, string> = {
  SIMULATED_UPI: 'UPI (simulated)',
  SIMULATED_CASH: 'Cash (simulated)',
  WAIVED: 'Waived',
  MANUAL: 'Paid at office',
  NO_CHARGE: 'No charge',
}

/** o***@example.com */
export function maskEmail(e: string): string {
  const at = e.lastIndexOf('@')
  return at > 0 ? `${e[0]}***${e.slice(at)}` : '***'
}
