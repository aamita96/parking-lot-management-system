// Client-side mirror of the Go fee calculator (BR-6 to BR-12), used only to display running fees.
// The charged fee always comes from the exit-scan response (SRS 10.5).

export type Rate = { hourlyRatePaise: number; dailyCapPaise: number; graceMinutes: number }

export type Fee = {
  durationMinutes: number
  billableHours: number
  withinGrace: boolean
  fullDays: number
  remainderHours: number
  remainderPaise: number
  remainderCapped: boolean
  feePaise: number
}

const DAY = 1440

export function calculateFee(minutes: number, r: Rate): Fee {
  const d = Math.max(0, Math.floor(minutes))
  const f: Fee = {
    durationMinutes: d,
    billableHours: 0,
    withinGrace: false,
    fullDays: 0,
    remainderHours: 0,
    remainderPaise: 0,
    remainderCapped: false,
    feePaise: 0,
  }
  if (d <= r.graceMinutes) {
    f.withinGrace = true
    return f
  }
  f.billableHours = Math.ceil(d / 60)
  f.fullDays = Math.floor(d / DAY)
  f.remainderHours = Math.ceil((d % DAY) / 60)
  let remainder = f.remainderHours * r.hourlyRatePaise
  if (remainder > r.dailyCapPaise) {
    remainder = r.dailyCapPaise
    f.remainderCapped = true
  }
  f.remainderPaise = remainder
  f.feePaise = f.fullDays * r.dailyCapPaise + remainder
  return f
}

export function minutesSince(iso: string, now = Date.now()): number {
  return Math.max(0, Math.floor((now - new Date(iso).getTime()) / 60000))
}

const short = (p: number) =>
  new Intl.NumberFormat('en-IN', { style: 'currency', currency: 'INR', minimumFractionDigits: 0, maximumFractionDigits: 2 }).format(p / 100)
const full = (p: number) =>
  new Intl.NumberFormat('en-IN', { style: 'currency', currency: 'INR', minimumFractionDigits: 2 }).format(p / 100)

/** "4 h × ₹40 = ₹160.00", matching the backend receipt line. */
export function feeBreakdown(f: Fee, r: Rate): string {
  if (f.withinGrace) return `No charge, within ${r.graceMinutes} free minutes`
  const parts: string[] = []
  if (f.fullDays > 0) parts.push(`${f.fullDays} ${f.fullDays > 1 ? 'days' : 'day'} × ${short(r.dailyCapPaise)}`)
  if (f.remainderHours > 0) {
    let p = `${f.remainderHours} h × ${short(r.hourlyRatePaise)}`
    if (f.remainderCapped) p += ` (capped at ${short(r.dailyCapPaise)})`
    parts.push(p)
  }
  return `${parts.join(' + ')} = ${full(f.feePaise)}`
}

/** "₹40/hour after 10 free minutes, max ₹300/day" */
export function rateSummary(r: Rate): string {
  return `${short(r.hourlyRatePaise)}/hour after ${r.graceMinutes} free minutes, max ${short(r.dailyCapPaise)}/day`
}
