// Shapes of the JSON the Go API sends and receives.
// They mirror backend/api/openapi.yaml. If you change a response in Go, update it here too.
// Money is always in paise (₹1 = 100 paise). Dates are ISO strings in UTC.

export type VehicleType = 'MOTORCYCLE' | 'CAR' | 'TRUCK'
export type SpotType = 'COMPACT' | 'REGULAR' | 'OVERSIZED'
export type VehicleStatus = 'PENDING_VERIFICATION' | 'ACTIVE' | 'REMOVED'
export type SpotStatus = 'AVAILABLE' | 'OCCUPIED' | 'OUT_OF_SERVICE'
export type TicketStatus = 'ACTIVE' | 'PAYMENT_PENDING' | 'CLOSED' | 'CANCELLED'
export type GateKind = 'ENTRY' | 'EXIT'

/** A list response: { items: [...] } */
export type Items<T> = { items: T[] }

/** A paged list response. */
export type Page<T> = { items: T[]; total: number; page: number; pageSize: number }

export type Rate = {
  hourlyRatePaise: number
  dailyCapPaise: number
  graceMinutes: number
}

// ---------- Public ----------

export type PublicConfig = {
  simulatorEnabled: boolean
  devCodes: boolean // true when the email code is shown on screen (local demo)
  publicUrl: string
}

export type Panel = {
  vehicleType: VehicleType
  label: string // "Motorcycles", "Cars", "Trucks"
  spotType: SpotType
  available: number
  total: number
  occupied: number
  outOfService: number
  overflowAvailable: number
  full: boolean
  freeSpots: { code: string; spotType: SpotType; floor: number; overflow: boolean }[]
}

export type Availability = {
  asOf: string
  overflowEnabled: boolean
  panels: Panel[]
}

export type VehicleSummary = {
  id: string
  vehicleNumber: string
  vehicleType: VehicleType
  status: VehicleStatus
}

export type RegistrationResult = {
  status: 'CODE_SENT' | 'ALREADY_REGISTERED'
  registrationId?: string
  vehicleNumber: string
  codeSentTo?: string
  codeExpiresAt?: string
  resendAfterSeconds?: number
  vehicle?: VehicleSummary
  devCode?: string
}

export type CodeSent = {
  codeSentTo: string
  codeExpiresAt?: string
  resendAfterSeconds: number
  devCode?: string
}

export type Owner = {
  id: string
  email: string
  mobile: string | null
  verified: boolean
}

export type OwnerSession = {
  owner: Owner
  ownerToken: string
  expiresAt: string
  vehicle?: VehicleSummary
}

export type NumberCheck = {
  vehicleNumber: string
  valid: boolean
  registered: boolean
}

// ---------- Vehicles and tickets ----------

export type Vehicle = {
  id: string
  vehicleNumber: string
  vehicleType: VehicleType
  ownerId: string
  ownerEmail: string
  registrationId: string | null
  status: VehicleStatus
  createdAt: string
  updatedAt: string
}

export type Ticket = {
  id: string
  ticketNumber: string // PLM-20261104-000123
  vehicleId: string
  spotId: string
  vehicleType: VehicleType
  spotType: SpotType
  ownerEmail: string // email at the time of entry
  entryTime: string
  exitTime: string | null
  durationMinutes: number | null
  billableHours: number | null
  feePaise: number | null
  paidPaise: number | null
  paymentMethod: string | null
  paidAt: string | null
  status: TicketStatus
  isSimulated: boolean
  source: 'GATE' | 'SIMULATOR' | 'ADMIN'
  vehicleNumber: string
  spotCode: string
  spotFloor: number
  rate: Rate
}

export type MyVehicle = Vehicle & { activeTicket: Ticket | null }

export type EmailNotification = {
  id: string
  kind: 'ENTRY' | 'EXIT'
  recipient: string
  status: 'QUEUED' | 'SENT' | 'FAILED' | 'DEFERRED'
  attempts: number
  lastError: string | null
  sentAt: string | null
}

export type EventSummary = {
  eventId: string
  eventType: 'VehicleEntered' | 'VehicleExited'
  ticketId: string
  ticketNumber: string
  vehicleNumber: string
  spotCode: string
  simulated: boolean
  occurredAt: string
  publishedAt: string | null
  kafkaStatus: 'PENDING' | 'PUBLISHED' | 'RETRYING'
  emailStatus: 'WAITING' | 'QUEUED' | 'SENT' | 'FAILED' | 'DEFERRED'
  feePaise: number | null
}

export type TicketDetail = Ticket & {
  notifications: EmailNotification[]
  events: EventSummary[]
  breakdown?: string // "4 h × ₹40 = ₹160.00"
}

// ---------- Gate (scanner, PAY kiosk, simulator) ----------

export type GateTicket = { id: string; ticketNumber: string; status: TicketStatus; entryTime: string; isSimulated: boolean }
export type GateOwner = { email?: string; emailMasked: string }
export type GateSpot = { code: string; spotType: SpotType; rowLabel: string; position: number; floor: number }

/** Answer to POST /gate/entry-scans when the vehicle is let in. */
export type EntryResponse = {
  result: 'ADMITTED'
  ticket: GateTicket
  vehicle: { vehicleNumber: string; vehicleType: VehicleType }
  owner: GateOwner
  spot: GateSpot
  rate: Rate
  deduplicated?: boolean
}

/** Answer to POST /gate/exit-scans: the fee to pay (or EXIT_CLOSED when it is free). */
export type ExitResponse = {
  result: 'EXIT_PENDING_PAYMENT' | 'EXIT_CLOSED'
  ticket: GateTicket
  vehicle: { vehicleNumber: string; vehicleType: VehicleType }
  owner: GateOwner
  entryTime: string
  exitTime: string
  durationMinutes: number
  billableHours: number
  feePaise: number
  breakdown: string
  rate: Rate
}

export type Receipt = {
  result: 'EXIT_CLOSED'
  ticketId: string
  ticketNumber: string
  status: TicketStatus
  paidPaise: number
  paymentMethod: string
  paidAt: string | null
}

// ---------- Admin ----------

export type AdminSession = {
  admin: { id: string; email: string; name: string }
  accessToken: string
  expiresAt: string
}

export type LayoutVehicle = {
  vehicleId: string
  vehicleNumber: string
  vehicleType: VehicleType
  colorSeed: string
  ownerEmail: string
  ticketId: string
  ticketNumber: string
  entryTime: string
  isSimulated: boolean
  rate: Rate
}

export type LayoutSpot = {
  id: string
  code: string
  spotType: SpotType
  letter: 'C' | 'R' | 'L'
  position: number
  status: SpotStatus
  vehicle: LayoutVehicle | null
}

export type LayoutRow = { floor: number; rowLabel: string; spots: LayoutSpot[] }

/** GET /admin/simulator/layout */
export type Layout = {
  asOf: string
  freeCount: number
  floors: number[]
  rows: LayoutRow[]
}

export type Scan = {
  id: number
  gateName: string
  gateKind: GateKind
  plateRaw: string
  plateNormalised: string | null
  rejectReason: string | null
  scannedAt: string
}

export type AdminAction = {
  id: string
  adminEmail: string | null
  action: string
  targetType: string
  targetId: string | null
  reason: string | null
  details: Record<string, unknown>
  createdAt: string
}

export type Exceptions = {
  rejectedScans: Scan[]
  unpaidTickets: Ticket[]
  disputes: AdminAction[]
  counts: { rejectedScans: number; unpaidTickets: number; disputes: number; total: number }
}

export type OwnerSummary = {
  id: string
  email: string
  mobile: string | null
  emailVerifiedAt: string | null
  createdAt: string
  vehicles: Vehicle[]
}

export type Spot = {
  id: string
  code: string
  spotType: SpotType
  floor: number
  rowLabel: string
  position: number
  status: SpotStatus
}

export type PricingRule = Rate & {
  id: string
  vehicleType: VehicleType
  effectiveFrom: string
  effectiveTo: string | null
}

export type Settings = { overflowEnabled: boolean; simulatorEnabled: boolean }

export type GateDevice = {
  id: string
  name: string
  kind: GateKind
  isSimulator: boolean
  active: boolean
  lastSeenAt: string | null
  apiKey?: string // only present right after create / rotate
}
