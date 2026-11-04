# Parking Lot Management System — Software Requirements Specification

Version 1.1 · Nov 4, 2026 · @Amit Khatri

## 1. Introduction

The Parking Lot Management System (PLMS) registers vehicles, issues a ticket and assigns a compatible spot on entry, charges a duration-based fee on exit, publishes every entry and exit to Kafka, and emails the owner on both events.

### 1.1 Purpose

This SRS is the single reference for the frontend (React + TanStack), backend (Go), database (PostgreSQL), messaging (Kafka) and email (Resend) work. Every requirement has an ID (FR-x, BR-x, NFR-x) so it can be traced to tickets and tests.

### 1.2 Scope

In scope for v1:

- Self-service vehicle registration by owners, confirmed with a one-time code sent to their email
- Spot inventory of three types: compact, regular, oversized
- Live availability shown separately for motorcycles, cars and trucks
- Unattended entry: a gate scanner reads the plate, the system issues a ticket and assigns a spot
- Unattended exit: the scanner reads the plate, the fee is shown at the PAY kiosk, payment closes the ticket and frees the spot
- Kafka events for entry and exit, consumed by an email notification service
- Entry, exit and verification emails via Resend
- A browser-based parking lot simulator that plays the scanner, the barriers and the PAY kiosk (section 11)
- Admin management of spots, pricing, gate devices and exceptions

Out of scope for v1: real camera/ANPR hardware and barrier controllers (the simulator stands in for both through the same gate API), real payment gateways (payment is simulated), reservations and multi-site lots.

### 1.3 Definitions

| Term | Meaning |
| --- | --- |
| Vehicle | A registered motorcycle, car or truck, identified by its vehicle number |
| Owner | The person who registers a vehicle; identified by a verified email |
| Spot | One physical bay, typed compact (C), regular (R) or oversized (L, large) |
| Ticket | The record of one parking session: vehicle, spot, entry time, exit time, fee, payment |
| Active ticket | A ticket with an entry time and not yet paid and closed |
| Gate device | An entry or exit scanner (real or simulated) that sends plate numbers to the gate API with its own API key |
| Scan | One plate read at a gate, logged whether it was admitted or rejected |
| Simulator | The admin screen that animates vehicles through the lot and calls the gate API as a virtual gate device |
| Outbox | A Postgres table that holds events until they are published to Kafka |
| Paise | Smallest INR unit; all money is stored as integer paise |

### 1.4 Conventions

- "Shall" marks a mandatory requirement; "should" marks a recommended one.
- Times are stored in UTC (`timestamptz`) and displayed in Asia/Kolkata (IST).
- Money is INR, stored in paise as `BIGINT`.

### 1.5 References

- [Resend Go SDK and docs](https://resend.com/docs)
- [Resend account quotas and limits](https://www.resend.com/docs/knowledge-base/account-quotas-and-limits.md)
- Transactional outbox pattern (for reliable event publishing)

## 2. Overall Description

PLMS is a new, standalone web application for one unattended parking lot: owners register their own vehicles, gate scanners handle entry and exit, and only admins log in.

### 2.1 Actors

| Actor | What they do | How they are identified |
| --- | --- | --- |
| Vehicle owner | Registers vehicles, views their vehicles and tickets, receives entry and exit emails | One-time code sent to their email; no password |
| Gate device | Sends the plate number when a vehicle reaches the IN or OUT gate; reports payment at the PAY kiosk | API key per device (`X-Gate-Key`) |
| Simulator | Plays every gate device and kiosk in the browser for demos and testing | Admin session, acting as the simulator gate devices |
| Admin | Manages spots, pricing, gate devices and exceptions (misreads, disputes, manual exit); runs the simulator | Email + password login |

There is no gate operator. Exceptions that used to need one (an unreadable plate, an unregistered vehicle, a disputed plate) go to an admin queue (FR-38).

### 2.2 Operating environment

- Browser: latest two versions of Chrome, Edge, Firefox and Safari; the registration page works on a phone (360 px wide) so owners can register from a QR code at the gate.
- Backend: Go 1.23+ services in Linux containers.
- PostgreSQL 16+, Apache Kafka 3.7+ in KRaft mode (no ZooKeeper).
- Email: Resend transactional API over HTTPS.

### 2.3 Assumptions

- One parking lot, laid out as rows of spots (A, B, …) along one driving lane, optionally on several floors.
- The server clock is the only source of entry and exit times; the one exception is the simulator's stay-length control (FR-44).
- A vehicle number uniquely identifies a vehicle; one vehicle has one type and one owner.
- An owner proves they own the email by entering the code sent to it; the system does not verify that they own the vehicle (registration ID is stored for disputes).
- Payment is simulated in v1: the PAY kiosk records the amount and method, no money moves.

### 2.4 Constraints

- Resend's free plan allows 100 emails/day and 3,000/month. A visit sends 2 emails and a new owner's code 1 more, so the free plan covers roughly 40–50 visits a day.
- Resend sends only from a verified domain; one domain is allowed on the free plan.
- Kafka delivery is at-least-once, so every consumer must be idempotent.
- Entry and exit must never fail because email or Kafka is down.

## 3. System Architecture

The gate path is synchronous and touches only the Go API and Postgres; Kafka, the notifier and Resend run downstream, so they can fail or lag without stopping check-in or check-out.

&#91;embedded content: system architecture · 6 components\]

The API writes the ticket and its event in one transaction; the relay publishes the event, and the notifier turns it into exactly one email.

### 3.1 Technology stack

| Layer | Technology |
| --- | --- |
| Frontend | React 19, TypeScript, Vite, TanStack Router / Query / Table / Form, Zod, Tailwind + shadcn/ui |
| API | Go 1.23+, chi router, pgx v5, sqlc, golang-migrate, log/slog, OpenTelemetry |
| Database | PostgreSQL 16 |
| Messaging | Apache Kafka 3.7+ (KRaft), franz-go client |
| Email | Resend (Go SDK), Go html/template |
| Local / CI | Docker Compose, Testcontainers, GitHub Actions |

### 3.2 Entry flow

1. The vehicle reaches the IN gate; the scanner (or the simulator) sends `POST /gate/entry-scans` with the plate.
2. The API checks registration, allocates a spot, creates the ticket and writes `VehicleEntered` to the outbox in one transaction.
3. The response carries vehicle number, owner email, ticket and spot, which the gate display (or the simulator dialog) shows; the barrier lifts.
4. The outbox relay publishes the event to `parking.ticket-events.v1`.
5. The notifier consumes it and sends the entry email via Resend.
6. Availability and the simulator show one fewer free spot on their next poll.

### 3.3 Exit flow

1. The vehicle reaches the PAY kiosk; the exit scanner sends `POST /gate/exit-scans`.
2. The API records exit time, locks the fee and sets the ticket to `PAYMENT_PENDING`; the kiosk shows the fee.
3. The kiosk sends `POST /gate/payments`; the API closes the ticket, frees the spot and writes `VehicleExited` to the outbox.
4. The OUT barrier lifts; the notifier sends the exit email with the fee breakdown.

### 3.4 Registration flow

1. The owner opens `/register` (or scans the QR code at the gate) and fills in the form.
2. The API stores a pending registration and emails a 6-digit code straight through Resend.
3. The owner enters the code; the vehicle becomes active and the owner is signed in to "My vehicles".

## 4. Functional Requirements

The system has seven functional areas; each requirement below is testable on its own.

### 4.1 Owner self-registration

| ID | Requirement | Priority |
| --- | --- | --- |
| FR-1 | A public page shall let anyone register a vehicle with **vehicle number**, **vehicle type** and **owner email** (mandatory), and **mobile** and **registration ID** (optional). No account or password is needed. | Must |
| FR-2 | The system shall normalise the vehicle number (trim, uppercase, remove spaces and hyphens), so `ka 01-ab 1234` is stored as `KA01AB1234`. | Must |
| FR-3 | The system shall validate email format, Indian mobile format (10 digits starting 6–9, optional +91), and vehicle number format (standard and BH series). | Must |
| FR-4 | The system shall email a 6-digit code, valid for 10 minutes, with at most 5 attempts and 3 codes per email per hour. The vehicle becomes eligible to park only after the code is entered; unverified registrations expire after 24 hours. | Must |
| FR-5 | If the number is already registered to the same verified owner, the page shall show that vehicle. If it belongs to another owner, the page shall refuse and offer "Report a problem", which opens a dispute for admins. | Must |
| FR-6 | An owner who has just verified (or signed in) shall be able to register more vehicles without another code. | Must |
| FR-7 | Owners shall sign in to "My vehicles" with an email code to see their vehicles, active ticket and past tickets, and edit mobile and registration ID. Changing the email requires verifying the new one. | Should |
| FR-8 | Owners shall be able to remove a vehicle that has no active ticket. | Should |
| FR-9 | The entry gate shall display a QR code linking to the registration page, for vehicles that arrive unregistered. | Should |

Vehicle type is mandatory: without it the system cannot pick a spot or a rate.

### 4.2 Availability

| ID | Requirement | Priority |
| --- | --- | --- |
| FR-10 | A public page shall show open spots in three panels: Motorcycles (compact), Cars (regular) and Trucks (oversized), each with available / total counts. | Must |
| FR-11 | Each panel shall list the individual open spots by code. | Must |
| FR-12 | Counts shall refresh within 5 seconds of any entry, exit or spot change. | Must |
| FR-13 | A panel shall show "Full" when no compatible spot is free, counting overflow spots if overflow is on (BR-3). | Must |

### 4.3 Entry at the gate

| ID | Requirement | Priority |
| --- | --- | --- |
| FR-14 | An entry gate device shall send each plate it reads to the gate API, authenticated with its own API key. | Must |
| FR-15 | The vehicle shall be admitted when it is registered and verified, has no active ticket, and a compatible spot is free. The system then allocates the spot (BR-1 to BR-4), creates the ticket with server-side entry time, marks the spot occupied and writes `VehicleEntered` to the outbox, in one transaction. | Must |
| FR-16 | The admit response shall carry everything the gate displays: ticket number, vehicle number and type, owner email, spot code, spot type, row and entry time. | Must |
| FR-17 | Otherwise the scan shall be rejected with a reason: `NOT_REGISTERED`, `NOT_VERIFIED`, `ALREADY_PARKED`, `NO_SPOT_AVAILABLE` or `UNREADABLE_PLATE`. The barrier stays closed. | Must |
| FR-18 | Every scan, admitted or rejected, shall be logged with gate, plate as read, result and time. | Must |
| FR-19 | Repeat reads of the same plate at the same gate within 30 seconds shall return the first result instead of acting again. | Must |
| FR-20 | A real gate display shall mask the email (`o***@example.com`); the simulator, used by admins, shows it in full. | Should |

### 4.4 Exit and payment

| ID | Requirement | Priority |
| --- | --- | --- |
| FR-21 | An exit scan shall find the vehicle's active ticket, record exit time, calculate and lock the fee (section 5), and set the ticket to `PAYMENT_PENDING`. | Must |
| FR-22 | The PAY kiosk shall show vehicle number, entry and exit times, duration, billable hours and fee. | Must |
| FR-23 | Recording payment (amount equal to the fee; method `SIMULATED_UPI` or `SIMULATED_CASH`) shall close the ticket, free the spot and write `VehicleExited` to the outbox, in one transaction; then the OUT barrier opens. | Must |
| FR-24 | A ticket with a fee of ₹0 (within grace) shall close at the exit scan without a payment step. | Must |
| FR-25 | An exit scan for a plate with no active ticket shall be rejected (`NO_ACTIVE_TICKET`) and sent to the admin queue. | Must |
| FR-26 | Paying a closed ticket shall return 409 `TICKET_ALREADY_CLOSED` with the original receipt; it shall never charge twice. | Must |
| FR-27 | A ticket left in `PAYMENT_PENDING` for more than 15 minutes shall appear in the admin queue. | Should |

### 4.5 Notifications

| ID | Requirement | Priority |
| --- | --- | --- |
| FR-28 | The system shall email the owner on entry with vehicle, spot, ticket and entry details. | Must |
| FR-29 | The system shall email the owner after payment with vehicle, spot, entry and exit times, duration, fee breakdown and amount paid. | Must |
| FR-30 | Each event shall produce at most one email, even if Kafka redelivers it. | Must |
| FR-31 | Verification codes shall be emailed directly by the API (not through Kafka), so they arrive within seconds. | Must |
| FR-32 | Admins shall see each ticket's email status (queued, sent, failed, deferred) and be able to resend. | Should |

### 4.6 Administration and exceptions

| ID | Requirement | Priority |
| --- | --- | --- |
| FR-33 | Admins shall create spots singly or in bulk (type, floor, row, positions), which also sets their place in the simulator layout. | Must |
| FR-34 | Admins shall mark a free spot out of service and back. An occupied spot cannot go out of service. | Must |
| FR-35 | Admins shall edit the rate card per vehicle type; changes apply to tickets opened after the change. | Must |
| FR-36 | Admins shall toggle overflow allocation (BR-3). | Should |
| FR-37 | Admins shall register gate devices (entry or exit), rotate their API keys and deactivate them. | Must |
| FR-38 | Admins shall work an exception queue of rejected scans, unpaid tickets and ownership disputes, with actions: manual check-in, manual exit (charge or waive, with a reason), and reassigning a vehicle to its rightful owner. | Must |
| FR-39 | Admins shall search all owners, vehicles and tickets. | Must |
| FR-40 | Every admin action shall be recorded with who, when and why. | Should |

### 4.7 Parking lot simulator

| ID | Requirement | Priority |
| --- | --- | --- |
| FR-41 | An admin screen shall draw the lot top-down from real spot data: spot codes and type letters (C, R, L), parked vehicles, a "P n OPEN" board, the IN gate with ticket kiosk and barrier, and the PAY kiosk with OUT barrier (section 11). | Must |
| FR-42 | "Send vehicle" shall drive a chosen registered vehicle (searchable list, or Random) to the IN gate. "Unregistered plate" sends a made-up number to test rejection. | Must |
| FR-43 | When the vehicle reaches the IN gate, the simulator shall call the entry-scan API and show a dialog with vehicle number, owner email, vehicle type, ticket number, assigned spot, entry time and email status. A rejected vehicle shows the reason and turns back. | Must |
| FR-44 | Clicking a parked vehicle shall show its ticket and a "Leave" action with a stay length (actual, 30 min, 3 h, 26 h). A chosen stay sets exit time to entry time plus that stay, for simulator tickets only, so fees can be tested without waiting. | Must |
| FR-45 | At the PAY kiosk the simulator shall call the exit-scan API and show a payment dialog with vehicle number, owner email, entry and exit times, duration, billable hours and fee; "Pay" records a simulated payment, then the OUT barrier lifts. | Must |
| FR-46 | Auto mode shall send a random registered vehicle every N seconds and make parked vehicles leave after random stays; speed 1×, 2× or 4×. | Should |
| FR-47 | An event panel shall list the last 20 entry and exit events with time, plate, Kafka publish status and email status. | Should |
| FR-48 | Simulator tickets shall be flagged as simulated and left out of revenue reports; "Reset" closes them and frees their spots. | Must |
| FR-49 | The simulator shall poll the lot every 2 seconds, so vehicles parked by real gates or admins also appear. | Must |

## 5. Business Rules

Each vehicle type has a home spot type; it can spill into larger spots only when overflow is on, and it always pays its own vehicle-type rate.

### 5.1 Spot compatibility and allocation

| Vehicle type | Home spot | Overflow order (when enabled) | Dashboard panel |
| --- | --- | --- | --- |
| Motorcycle | Compact | Regular, then Oversized | Motorcycles |
| Car | Regular | Oversized | Cars |
| Truck | Oversized | None | Trucks |

- **BR-1** A vehicle shall only be placed in its home spot type or a larger one; never a smaller one.
- **BR-2** Within the allowed spot type, the system shall pick the free spot with the lowest floor, then the lowest spot code (nearest first).
- **BR-3** Overflow is off by default. When on, the home type is tried first and larger types only when the home type is full.
- **BR-4** Allocation shall lock the chosen row with `SELECT … FOR UPDATE SKIP LOCKED`, so two simultaneous check-ins never get the same spot.
- **BR-5** A vehicle can hold at most one active ticket; a spot can hold at most one active ticket. Both are enforced by partial unique indexes (section 6).

### 5.2 Fee calculation

- **BR-6** The fee depends on vehicle type and duration, not on the spot type used. A motorcycle in an oversized spot pays the motorcycle rate.
- **BR-7** Duration = exit time − entry time, in whole minutes, both from the server clock.
- **BR-8** A stay within the grace period (default 10 minutes) is free.
- **BR-9** Otherwise, billable hours = duration rounded **up** to the next whole hour, minimum 1 hour.
- **BR-10** Each full 24-hour block is charged the daily cap; the remainder is charged hourly, but never more than the daily cap.
- **BR-11** The rate card in force at **entry** applies to the whole ticket; the ticket stores the `pricing_rule_id` it used.
- **BR-12** All money is calculated in integer paise; no floating point.

Fee for a stay of D minutes, with hourly rate r, daily cap c and grace g:

```latex
\text{fee} = \begin{cases} 0 & D \le g \\ \lfloor D / 1440 \rfloor \cdot c + \min\left(c,\ \lceil (D \bmod 1440) / 60 \rceil \cdot r\right) & D > g \end{cases}
```

When D is above the grace period but D mod 1440 is 0, the remainder term is 0.

### 5.3 Default rate card (placeholder, confirm before launch)

| Vehicle type | Hourly rate (₹) | Daily cap (₹) | Grace (min) |
| --- | --- | --- | --- |
| Motorcycle | 20 | 150 | 10 |
| Car | 40 | 300 | 10 |
| Truck | 100 | 1,000 | 10 |

### 5.4 Worked examples

| Case | Duration | Calculation | Fee (₹) |
| --- | --- | --- | --- |
| Car, 10:05 → 10:12 | 7 min | Within grace | 0 |
| Car, 10:05 → 13:25 | 200 min | ⌈200 / 60⌉ = 4 h × 40 | 160 |
| Motorcycle in an oversized spot, 09:00 → 09:50 | 50 min | 1 h × 20 (motorcycle rate) | 20 |
| Car, 08:00 → 18:30 | 630 min | 11 h × 40 = 440, capped at 300 | 300 |
| Truck, 26 h 10 min | 1,570 min | 1 day × 1,000 + min(1,000, 3 h × 100) | 1,300 |

## 6. Data Model (PostgreSQL)

Twelve tables hold the state; the ticket is the centre, linking one vehicle, one spot and one rate card, and partial unique indexes make double-parking impossible at the database level.

### 6.1 Tables

| Table | Purpose | Key columns |
| --- | --- | --- |
| `owners` | People who register vehicles | `email` (unique), `mobile`, `email_verified_at` |
| `email_codes` | One-time codes for verification and sign-in | `owner_id`, `code_hash`, `purpose`, `expires_at`, `attempts` |
| `vehicles` | Registered vehicles | `vehicle_number` (unique while not removed), `vehicle_type`, `owner_id`, `status`, `registration_id` |
| `parking_spots` | Physical bays and their place on the map | `code` (unique), `spot_type`, `floor`, `row_label`, `position`, `status` |
| `pricing_rules` | Rate card versions per vehicle type | `vehicle_type`, `hourly_rate_paise`, `daily_cap_paise`, `grace_minutes`, `effective_from` |
| `tickets` | One row per parking session | `ticket_number`, `vehicle_id`, `spot_id`, `entry_time`, `exit_time`, `status`, `fee_paise`, `paid_at`, `is_simulated` |
| `gate_devices` | Real and simulated scanners and kiosks | `name`, `kind`, `api_key_hash`, `is_simulator`, `active` |
| `gate_scans` | Every plate read, admitted or rejected | `gate_id`, `plate_raw`, `plate_normalised`, `result`, `ticket_id` |
| `outbox_events` | Events waiting to be published to Kafka | `event_id`, `event_type`, `aggregate_id`, `payload`, `published_at` |
| `email_notifications` | One row per email; dedupes Kafka redeliveries | `event_id` (unique), `ticket_id`, `kind`, `status` |
| `admin_users` | Admin logins | `email`, `password_hash` |
| `admin_actions` | Audit trail and dispute records (FR-38, FR-40) | `admin_id`, `action`, `target_id`, `reason`, `created_at` |

### 6.2 DDL (core tables)

```sql
CREATE TYPE vehicle_type   AS ENUM ('MOTORCYCLE', 'CAR', 'TRUCK');
CREATE TYPE vehicle_status AS ENUM ('PENDING_VERIFICATION', 'ACTIVE', 'REMOVED');
CREATE TYPE spot_type      AS ENUM ('COMPACT', 'REGULAR', 'OVERSIZED');
CREATE TYPE spot_status    AS ENUM ('AVAILABLE', 'OCCUPIED', 'OUT_OF_SERVICE');
CREATE TYPE ticket_status  AS ENUM ('ACTIVE', 'PAYMENT_PENDING', 'CLOSED', 'CANCELLED');
CREATE TYPE gate_kind      AS ENUM ('ENTRY', 'EXIT');
CREATE TYPE scan_result    AS ENUM ('ADMITTED', 'EXIT_PENDING_PAYMENT', 'EXIT_CLOSED', 'REJECTED');

CREATE TABLE owners (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email             VARCHAR(254) NOT NULL UNIQUE,     -- stored lower-case
  mobile            VARCHAR(15),                       -- E.164
  email_verified_at TIMESTAMPTZ,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE email_codes (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id    UUID NOT NULL REFERENCES owners(id),
  purpose     VARCHAR(10) NOT NULL,                    -- VERIFY | SIGN_IN
  code_hash   VARCHAR(100) NOT NULL,                   -- never store the code itself
  expires_at  TIMESTAMPTZ NOT NULL,                    -- now() + 10 min
  attempts    SMALLINT NOT NULL DEFAULT 0,             -- max 5
  consumed_at TIMESTAMPTZ,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE vehicles (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  vehicle_number  VARCHAR(15)    NOT NULL,             -- normalised, e.g. KA01AB1234
  vehicle_type    vehicle_type   NOT NULL,
  owner_id        UUID           NOT NULL REFERENCES owners(id),
  registration_id VARCHAR(50),
  status          vehicle_status NOT NULL DEFAULT 'PENDING_VERIFICATION',
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_vehicle_number ON vehicles (vehicle_number) WHERE status <> 'REMOVED';

CREATE TABLE parking_spots (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code       VARCHAR(20) NOT NULL UNIQUE,              -- e.g. A3
  spot_type  spot_type   NOT NULL,
  floor      SMALLINT    NOT NULL DEFAULT 0,
  row_label  CHAR(1)     NOT NULL,                     -- A, B, ... (simulator row)
  position   SMALLINT    NOT NULL,                     -- 1..n left to right
  status     spot_status NOT NULL DEFAULT 'AVAILABLE',
  version    INT         NOT NULL DEFAULT 0,
  UNIQUE (floor, row_label, position)
);
CREATE INDEX idx_spots_alloc ON parking_spots (spot_type, floor, row_label, position)
  WHERE status = 'AVAILABLE';

CREATE TABLE pricing_rules (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  vehicle_type      vehicle_type NOT NULL,
  hourly_rate_paise BIGINT NOT NULL CHECK (hourly_rate_paise >= 0),
  daily_cap_paise   BIGINT NOT NULL CHECK (daily_cap_paise >= 0),
  grace_minutes     INT    NOT NULL DEFAULT 10,
  effective_from    TIMESTAMPTZ NOT NULL,
  effective_to      TIMESTAMPTZ                        -- NULL = current
);

CREATE TABLE gate_devices (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name         VARCHAR(50) NOT NULL,                   -- e.g. IN-1, OUT-1, SIM-IN
  kind         gate_kind   NOT NULL,
  api_key_hash VARCHAR(100),                           -- NULL for simulator devices
  is_simulator BOOLEAN     NOT NULL DEFAULT false,
  active       BOOLEAN     NOT NULL DEFAULT true,
  last_seen_at TIMESTAMPTZ
);

CREATE TABLE tickets (
  id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  ticket_number    VARCHAR(30) NOT NULL UNIQUE,        -- PLM-20261104-000123
  vehicle_id       UUID NOT NULL REFERENCES vehicles(id),
  spot_id          UUID NOT NULL REFERENCES parking_spots(id),
  pricing_rule_id  UUID NOT NULL REFERENCES pricing_rules(id),
  entry_gate_id    UUID REFERENCES gate_devices(id),
  exit_gate_id     UUID REFERENCES gate_devices(id),
  vehicle_type     vehicle_type NOT NULL,              -- snapshot at entry
  spot_type        spot_type    NOT NULL,              -- snapshot at entry
  owner_email      VARCHAR(254) NOT NULL,              -- snapshot at entry
  entry_time       TIMESTAMPTZ  NOT NULL DEFAULT now(),
  exit_time        TIMESTAMPTZ,
  duration_minutes INT,
  billable_hours   INT,
  fee_paise        BIGINT,
  paid_paise       BIGINT,
  payment_method   VARCHAR(20),                        -- SIMULATED_UPI | SIMULATED_CASH | WAIVED
  paid_at          TIMESTAMPTZ,
  status           ticket_status NOT NULL DEFAULT 'ACTIVE',
  is_simulated     BOOLEAN NOT NULL DEFAULT false,
  CHECK (exit_time IS NULL OR exit_time >= entry_time)
);
CREATE UNIQUE INDEX uq_open_ticket_vehicle ON tickets (vehicle_id)
  WHERE status IN ('ACTIVE', 'PAYMENT_PENDING');
CREATE UNIQUE INDEX uq_open_ticket_spot ON tickets (spot_id)
  WHERE status IN ('ACTIVE', 'PAYMENT_PENDING');
CREATE INDEX idx_tickets_entry ON tickets (entry_time DESC);

CREATE TABLE gate_scans (
  id               BIGSERIAL PRIMARY KEY,
  gate_id          UUID NOT NULL REFERENCES gate_devices(id),
  plate_raw        VARCHAR(30) NOT NULL,
  plate_normalised VARCHAR(15),
  result           scan_result NOT NULL,
  reject_reason    VARCHAR(30),                        -- NOT_REGISTERED, NO_SPOT_AVAILABLE, ...
  ticket_id        UUID REFERENCES tickets(id),
  scanned_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_scans_recent ON gate_scans (gate_id, plate_normalised, scanned_at DESC);

CREATE TABLE outbox_events (
  id           BIGSERIAL PRIMARY KEY,
  event_id     UUID        NOT NULL UNIQUE,
  event_type   VARCHAR(50) NOT NULL,                   -- VehicleEntered | VehicleExited
  aggregate_id UUID        NOT NULL,                   -- ticket id (Kafka key)
  payload      JSONB       NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ,
  attempts     INT         NOT NULL DEFAULT 0
);
CREATE INDEX idx_outbox_unpublished ON outbox_events (id) WHERE published_at IS NULL;

CREATE TABLE email_notifications (
  id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  event_id            UUID NOT NULL UNIQUE,            -- idempotency
  ticket_id           UUID NOT NULL REFERENCES tickets(id),
  kind                VARCHAR(10) NOT NULL,            -- ENTRY | EXIT
  recipient           VARCHAR(254) NOT NULL,
  status              VARCHAR(10) NOT NULL,            -- QUEUED | SENT | FAILED | DEFERRED
  provider_message_id VARCHAR(100),
  attempts            INT NOT NULL DEFAULT 0,
  last_error          TEXT,
  next_attempt_at     TIMESTAMPTZ,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  sent_at             TIMESTAMPTZ
);
```

### 6.3 Entry-scan and payment transactions

Entry scan:

1. Normalise the plate; if the same plate was scanned at this gate in the last 30 seconds, return that scan's result (FR-19).
2. `BEGIN`
3. Load the vehicle with its owner; reject `NOT_REGISTERED` (no row), `NOT_VERIFIED` (status `PENDING_VERIFICATION`).
4. Select a spot: `SELECT id FROM parking_spots WHERE spot_type = $1 AND status = 'AVAILABLE' ORDER BY floor, row_label, position LIMIT 1 FOR UPDATE SKIP LOCKED` (repeat per overflow type); none → `NO_SPOT_AVAILABLE`.
5. Mark the spot `OCCUPIED`; insert the ticket. A violation of `uq_open_ticket_vehicle` maps to `ALREADY_PARKED`.
6. Insert `VehicleEntered` into `outbox_events` and the scan into `gate_scans`.
7. `COMMIT`. A rejected scan rolls back and is logged in its own short transaction.

Exit scan locks the open ticket `FOR UPDATE`, sets exit time, duration and fee, and moves it to `PAYMENT_PENDING` (or straight to `CLOSED` when the fee is 0). Payment locks the ticket, checks the amount, sets `CLOSED`, frees the spot and inserts `VehicleExited` into the outbox, all in one transaction.

Migrations are managed with golang-migrate; queries are generated with sqlc on top of pgx v5.

## 7. REST API Specification

The Go API exposes JSON over HTTPS under `/api/v1` with three kinds of caller: owners (email-code session), gate devices (API key) and admins (password login). It is documented as an OpenAPI 3.1 file in the repo.

### 7.1 Authentication

| Caller | Credential | Lifetime |
| --- | --- | --- |
| Owner | JWT (`role: OWNER`) issued after a correct email code | 30 days, refreshable |
| Gate device | `X-Gate-Key: <device API key>` header; key stored hashed | Until rotated |
| Simulator | Admin JWT plus `X-Gate-Id: SIM-IN` or `SIM-OUT`; accepted only for devices flagged `is_simulator` and only when `SIMULATOR_ENABLED=true` | Admin session |
| Admin | JWT (`role: ADMIN`) after email + password | 15 min access, 7 day refresh |

### 7.2 Endpoints

| Method | Path | Purpose | Caller |
| --- | --- | --- | --- |
| POST | `/public/registrations` | Start a registration; emails a code (FR-1 to FR-4) | Anyone |
| POST | `/public/registrations/{id}/verify` | Check the code; activates the vehicle, returns an owner token | Anyone |
| POST | `/public/sign-in` and `/public/sign-in/verify` | Email-code sign-in for returning owners (FR-7) | Anyone |
| POST | `/public/disputes` | "Report a problem" for a number registered by someone else (FR-5) | Anyone |
| GET | `/availability` | Counts and free spots per panel (FR-10) | Anyone |
| GET, POST | `/me/vehicles` | List or add own vehicles (FR-6) | Owner |
| PATCH, DELETE | `/me/vehicles/{id}` | Edit or remove own vehicle (FR-7, FR-8) | Owner |
| GET | `/me/tickets` | Own active and past tickets | Owner |
| POST | `/gate/entry-scans` | Plate read at the IN gate (FR-14 to FR-19) | Gate device, simulator |
| POST | `/gate/exit-scans` | Plate read at the PAY kiosk; locks the fee (FR-21) | Gate device, simulator |
| POST | `/gate/payments` | Record payment; closes the ticket (FR-23) | Gate device, simulator |
| GET | `/admin/simulator/layout` | Spots with row, position, status and parked vehicle (FR-41, FR-49) | Admin |
| GET | `/admin/simulator/events?limit=20` | Recent outbox events with publish and email status (FR-47) | Admin |
| POST | `/admin/simulator/reset` | Close simulated tickets, free their spots (FR-48) | Admin |
| GET | `/admin/exceptions` | Rejected scans, unpaid tickets, disputes (FR-38) | Admin |
| POST | `/admin/tickets` | Manual check-in | Admin |
| POST | `/admin/tickets/{id}/manual-exit` | Close with fee or waiver and a reason | Admin |
| GET | `/admin/tickets`, `/admin/tickets/{id}` | Search and detail, with email status | Admin |
| POST | `/admin/tickets/{id}/notifications/{kind}/resend` | Resend entry or exit email (FR-32) | Admin |
| GET, PATCH | `/admin/owners`, `/admin/vehicles/{id}` | Search; reassign a vehicle to another owner | Admin |
| POST, PATCH | `/admin/spots`, `/admin/spots/{id}` | Create spots, change status (FR-33, FR-34) | Admin |
| GET, PUT | `/admin/pricing/{vehicleType}` | Rate card (FR-35) | Admin |
| POST, PATCH | `/admin/gate-devices`, `/admin/gate-devices/{id}/rotate-key` | Manage devices and keys (FR-37) | Admin |
| POST | `/admin/auth/login` | Admin login | Anyone |
| GET | `/healthz`, `/readyz` | Liveness and readiness | Anyone |

All `/gate/*` calls accept an `Idempotency-Key` header; a repeated key within 24 hours returns the first response.

### 7.3 Registration

```json
POST /api/v1/public/registrations
{
  "vehicleNumber": "KA 01 AB 1234",
  "vehicleType": "CAR",
  "ownerEmail": "owner@example.com",
  "mobile": "+919876543210",
  "registrationId": null
}
202 Accepted
{ "registrationId": "r-5c…", "vehicleNumber": "KA01AB1234", "codeSentTo": "o***@example.com", "codeExpiresAt": "2026-11-04T11:40:00Z" }

POST /api/v1/public/registrations/r-5c…/verify
{ "code": "483920" }
200 OK
{
  "vehicle": { "id": "8b1c…", "vehicleNumber": "KA01AB1234", "vehicleType": "CAR", "status": "ACTIVE" },
  "ownerToken": "eyJhbGciOi…"
}
```

### 7.4 Entry scan

```json
POST /api/v1/gate/entry-scans
X-Gate-Key: …            (simulator: Authorization: Bearer <admin> + X-Gate-Id: SIM-IN)
{ "plate": "KA01AB1234" }

201 Created
{
  "result": "ADMITTED",
  "ticket": { "id": "t-91…", "ticketNumber": "PLM-20261104-000123", "entryTime": "2026-11-04T04:35:00Z" },
  "vehicle": { "vehicleNumber": "KA01AB1234", "vehicleType": "CAR" },
  "owner": { "email": "owner@example.com", "emailMasked": "o***@example.com" },
  "spot": { "code": "A3", "spotType": "REGULAR", "rowLabel": "A", "position": 3, "floor": 0 },
  "rate": { "hourlyRatePaise": 4000, "dailyCapPaise": 30000, "graceMinutes": 10 }
}

409 Conflict
{ "result": "REJECTED", "code": "NOT_REGISTERED", "plate": "MH12XY9999",
  "message": "This vehicle is not registered. Scan the QR code to register." }
```

The gate display uses `emailMasked`; the simulator shows `email` (FR-20). `owner.email` is returned only to simulator and admin callers.

### 7.5 Exit scan and payment

```json
POST /api/v1/gate/exit-scans
{ "plate": "KA01AB1234", "simulatedStayMinutes": 200 }     (simulator only, FR-44)
200 OK
{
  "result": "EXIT_PENDING_PAYMENT",
  "ticket": { "id": "t-91…", "ticketNumber": "PLM-20261104-000123", "status": "PAYMENT_PENDING" },
  "owner": { "email": "owner@example.com" },
  "entryTime": "2026-11-04T04:35:00Z",
  "exitTime": "2026-11-04T07:55:00Z",
  "durationMinutes": 200,
  "billableHours": 4,
  "feePaise": 16000,
  "currency": "INR"
}

POST /api/v1/gate/payments
{ "ticketId": "t-91…", "amountPaise": 16000, "method": "SIMULATED_UPI" }
200 OK
{ "result": "EXIT_CLOSED", "ticketNumber": "PLM-20261104-000123", "status": "CLOSED",
  "paidPaise": 16000, "paidAt": "2026-11-04T07:55:20Z", "openBarrier": true }
```

### 7.6 Simulator layout

```json
GET /api/v1/admin/simulator/layout
200 OK
{
  "asOf": "2026-11-04T07:55:21Z",
  "freeCount": 6,
  "rows": [
    { "rowLabel": "A", "spots": [
      { "code": "A1", "spotType": "COMPACT", "status": "OCCUPIED",
        "vehicle": { "vehicleNumber": "KA05MN4321", "vehicleType": "MOTORCYCLE", "colorSeed": "8b1c" } },
      { "code": "A2", "spotType": "COMPACT", "status": "AVAILABLE", "vehicle": null }
    ] }
  ],
  "gates": [ { "name": "SIM-IN", "kind": "ENTRY" }, { "name": "SIM-OUT", "kind": "EXIT" } ]
}
```

### 7.7 Errors

Errors use RFC 9457 Problem Details (`application/problem+json`) with a stable `code` field.

| HTTP | `code` | When |
| --- | --- | --- |
| 400 | `VALIDATION_ERROR` | Bad field; the response lists each field and message |
| 400 | `UNREADABLE_PLATE` | Scanned text is empty or not a valid number format |
| 401 | `UNAUTHENTICATED` | Missing or expired token, or bad gate key |
| 403 | `FORBIDDEN` | Caller not allowed, e.g. simulator headers with simulation off |
| 404 | `NOT_REGISTERED` | Plate scanned at entry has no vehicle |
| 404 | `NO_ACTIVE_TICKET` | Plate scanned at exit has no open ticket |
| 409 | `NOT_VERIFIED` | Vehicle's owner has not entered the email code |
| 409 | `VEHICLE_ALREADY_REGISTERED` | Number belongs to another owner |
| 409 | `ALREADY_PARKED` | Vehicle already has an open ticket |
| 409 | `NO_SPOT_AVAILABLE` | No compatible free spot |
| 409 | `AMOUNT_MISMATCH` | Payment amount differs from the locked fee |
| 409 | `TICKET_ALREADY_CLOSED` | Payment on a closed ticket; body includes the original receipt |
| 409 | `SPOT_OCCUPIED` | Out-of-service change on an occupied spot |
| 422 | `CODE_INVALID` / `CODE_EXPIRED` | Wrong or expired email code |
| 429 | `RATE_LIMITED` | Too many requests or codes |
| 500 | `INTERNAL` | Unexpected error; the response carries a `traceId` |

## 8. Event Design (Kafka)

Entry and exit events go to one topic keyed by ticket ID, published from a Postgres outbox, so an exit can never overtake its entry and no event is lost if Kafka is briefly down.

### 8.1 Topics

| Topic | Key | Partitions | Retention | Producer | Consumers |
| --- | --- | --- | --- | --- | --- |
| `parking.ticket-events.v1` | `ticket_id` | 6 | 7 days | Outbox relay | Notification service; Availability broadcaster (v2); Analytics (future) |
| `parking.ticket-events.v1.dlq` | `ticket_id` | 1 | 14 days | Notification service | Manual replay |

- One topic for both events (not one per event) keeps entry and exit for the same ticket in the same partition, in order.
- Replication factor 3 and `min.insync.replicas=2` in production; 1 in local development.
- Producer settings: `acks=all`, idempotent producer on.
- Go client: [franz-go](https://github.com/twmb/franz-go) (pure Go, no cgo).

### 8.2 Envelope and payloads

Every message uses the same envelope, with `event_type` also set as a Kafka header for cheap filtering.

```json
{
  "eventId": "0b6f6c1e-…",
  "eventType": "VehicleEntered",
  "eventVersion": 1,
  "occurredAt": "2026-11-04T04:35:00Z",
  "producer": "plms-api",
  "source": "GATE",                 // GATE | SIMULATOR | ADMIN
  "simulated": false,
  "traceId": "4bf92f35…",
  "data": {
    "ticketId": "t-91…",
    "ticketNumber": "PLM-20261104-000123",
    "vehicle": { "id": "8b1c…", "vehicleNumber": "KA01AB1234", "vehicleType": "CAR", "ownerEmail": "owner@example.com" },
    "spot": { "id": "s-22…", "code": "R-1-014", "spotType": "REGULAR", "floor": 1 },
    "entryTime": "2026-11-04T04:35:00Z",
    "rate": { "hourlyRatePaise": 4000, "dailyCapPaise": 30000, "graceMinutes": 10 }
  }
}
```

`VehicleExited` carries the same `ticketId`, `vehicle` and `spot` blocks plus `exitTime`, `durationMinutes`, `billableHours`, `feePaise` and `currency`.

Schema changes that add optional fields keep `eventVersion`; removing or renaming a field bumps it and gets a new topic suffix (`.v2`).

`VehicleExited` is written when payment closes the ticket, not at the exit scan, so it always carries the final, paid fee. Simulator events go to the same topic with `simulated: true`; consumers send their emails like any other, and analytics consumers skip them.

### 8.3 Outbox relay

1. Check-in and check-out insert the event into `outbox_events` in the same transaction as the ticket change.
2. A relay goroutine polls every 500 ms: `SELECT … WHERE published_at IS NULL ORDER BY id LIMIT 100 FOR UPDATE SKIP LOCKED`.
3. It publishes the batch to Kafka and waits for acknowledgement.
4. It sets `published_at` on success, or increments `attempts` with backoff on failure.
5. Published rows older than 7 days are purged by a daily job.

The relay runs inside the API process for v1 and can be split into its own deployment without code changes.

### 8.4 Consumer rules

- Consumer group `plms-notifications`; offsets are committed only after the email row is stored.
- Idempotency: the consumer inserts into `email_notifications` with `ON CONFLICT (event_id) DO NOTHING`; a redelivered event finds the row and skips sending.
- A message that fails to parse, or fails 5 times, goes to the DLQ with the error in a header.
- The consumer is stateless and scales up to the partition count (6).

## 9. Email Notifications (Resend)

A separate Go notification service consumes ticket events and sends one entry and one exit email per ticket through Resend; email trouble never blocks the gate.

### 9.1 Integration

- Provider: Resend, official Go SDK (`github.com/resend/resend-go`).
- Free plan: 100 emails/day, 3,000/month, one verified domain ([Resend quotas](https://www.resend.com/docs/knowledge-base/account-quotas-and-limits.md)).
- Sender: `PLMS <no-reply@parking.yourdomain.com>` on a domain verified with SPF and DKIM. The shared `onboarding@resend.dev` sender is for development only.
- Config via environment: `RESEND_API_KEY`, `EMAIL_FROM`, `EMAIL_REPLY_TO`, `EMAIL_ENABLED` (false in tests).
- Templates are Go `html/template` files with a plain-text alternative, rendered in IST, money formatted as ₹ with two decimals.

### 9.2 Entry email

Subject: `Checked in: KA01AB1234 at spot R-1-014 — Ticket PLM-20261104-000123`

| Field | Example |
| --- | --- |
| Ticket number | PLM-20261104-000123 |
| Vehicle number and type | KA01AB1234 · Car |
| Spot | R-1-014 · Regular · Floor 1 |
| Entry time | 04 Nov 2026, 10:05 AM IST |
| Rate | ₹40/hour after 10 free minutes, max ₹300/day |

### 9.3 Exit email

Subject: `Checked out: KA01AB1234 — ₹160.00 for 3 h 20 min`

| Field | Example |
| --- | --- |
| Ticket number | PLM-20261104-000123 |
| Vehicle number and type | KA01AB1234 · Car |
| Spot | R-1-014 · Regular · Floor 1 |
| Entry / exit time | 10:05 AM → 01:25 PM IST, 04 Nov 2026 |
| Duration | 3 h 20 min (billed as 4 h) |
| Fee breakdown | 4 h × ₹40 = ₹160.00 |
| Total | ₹160.00 |

The exit email is sent after payment and adds how and when it was paid, e.g. "Paid ₹160.00 by UPI (simulated) at 01:25 PM IST".

### 9.4 Verification code email

Subject: `Your PLMS code: 483920`

| Field | Example |
| --- | --- |
| Code | 483920 (valid 10 minutes) |
| Vehicle being registered | KA01AB1234 · Car |
| Safety line | "If you didn't try to register this vehicle, ignore this email." |

The API sends this email directly through Resend, not through Kafka (FR-31). It counts against the same daily quota; if the quota is used up, registration shows "Try again later" instead of failing silently.

### 9.5 Sending, retries and quota

1. On each event, insert an `email_notifications` row (`QUEUED`); skip if the `event_id` already exists.
2. Render the template and call Resend, passing the `event_id` as an idempotency key where the API supports it.
3. On success, store `provider_message_id` and set `SENT`.
4. On 5xx or network errors, retry with exponential backoff (1 s, 5 s, 30 s, 2 min, 10 min), then `FAILED` and DLQ.
5. On 429 `daily_quota_exceeded` or `monthly_quota_exceeded`, set `DEFERRED` with `next_attempt_at` one hour later; a scheduler retries deferred rows.
6. Sends are throttled client-side to 2 per second, below Resend's per-team API rate limit.
7. Ticket detail shows the email status; admins can resend (FR-28).

An exit email shall never arrive before its entry email: the consumer processes a partition in order, and if the entry email is still pending it sends that first.

## 10. Frontend Requirements

The frontend is one React + TypeScript app with a public area for owners and an admin area behind login, built with Vite and TanStack Router, Query, Table and Form.

### 10.1 Stack

| Concern | Choice | Why |
| --- | --- | --- |
| Build | Vite + React 19 + TypeScript (strict) | Fast dev server, typed end to end |
| Routing | TanStack Router (file-based, typed search params, route guards) | Public and admin areas in one app; filters live in the URL |
| Server state | TanStack Query | Caching, polling, invalidation after scans and payments |
| Tables | TanStack Table | Sort, filter and paginate vehicles, tickets and exceptions |
| Forms | TanStack Form + Zod | Same Zod schemas validate forms and parse API responses |
| Simulator drawing | SVG in React + Framer Motion | Vehicles move along lane paths; spots, kiosks and barriers are SVG shapes |
| Simulator state | Zustand store (or `useReducer`) per vehicle state machine | Animation state stays client-side; the server stays the source of truth |
| API types | Generated from the OpenAPI file (openapi-typescript) | Frontend and Go stay in sync |
| UI kit | Tailwind CSS + shadcn/ui (Dialog, Popover, Toast) | Accessible dialogs for the gate and payment pop-ups |

### 10.2 Screens

| Route | Screen | Who | Contents |
| --- | --- | --- | --- |
| `/` | Availability | Anyone | Three panels, Motorcycles / Cars / Trucks, with available/total, a fill bar and free spot codes; "Register your vehicle" button |
| `/register` | Register vehicle | Anyone | Three steps: details → email code → done (10.3); works on a phone |
| `/my` | My vehicles | Owner | Sign in with email code; vehicles, active ticket with running fee, past tickets |
| `/admin/login` | Admin login | Admin | Email and password |
| `/admin/simulator` | Parking lot simulator | Admin | Section 11 |
| `/admin/exceptions` | Exception queue | Admin | Rejected scans, unpaid tickets, disputes, with actions (FR-38) |
| `/admin/tickets` | Tickets | Admin | Active (live elapsed time, running fee) and history; simulated tickets marked |
| `/admin/vehicles` | Owners and vehicles | Admin | Search; reassign owner |
| `/admin/spots` | Spots | Admin | Spot list by row and position, bulk create, out-of-service toggle |
| `/admin/pricing` | Pricing | Admin | Rate card per vehicle type, overflow toggle |
| `/admin/gates` | Gate devices | Admin | Devices, API key rotation, last seen |

### 10.3 Registration page

Owners register themselves in three steps; nothing asks for a password.

1. **Details.** Vehicle number (normalised as typed: `KA 01 ab-1234` → `KA01AB1234`), vehicle type as three large buttons (Motorcycle, Car, Truck, each showing its spot type and free count), email, then optional mobile and registration ID. A debounced check warns at once if the number is already registered (FR-5).
2. **Code.** "We sent a 6-digit code to o\*\*\*@example.com"; six input boxes, paste supported, "Resend code" after 30 seconds.
3. **Done.** "KA01AB1234 is ready to park. You'll get an email each time it enters and leaves." Actions: "Register another vehicle" (no new code, FR-6) and "View my vehicles".

### 10.4 Vehicle form validation

| Field | Required | Rule |
| --- | --- | --- |
| Vehicle number | Yes | Normalised to uppercase without spaces/hyphens; standard pattern `^[A-Z]{2}[0-9]{1,2}[A-Z]{0,3}[0-9]{4}$` or BH series `^[0-9]{2}BH[0-9]{4}[A-Z]{1,2}$` |
| Vehicle type | Yes | Motorcycle, Car or Truck |
| Email | Yes | Valid email, max 254 characters, stored lower-case |
| Mobile | No | 10 digits starting 6–9, optional +91 prefix; stored as E.164 |
| Registration ID | No | Up to 50 characters, letters, digits and hyphens |

The server repeats every check; client validation is only for fast feedback.

### 10.5 Data freshness

- Availability polls every 5 seconds while the tab is visible; the simulator layout polls every 2 seconds.
- A scan or payment from the simulator invalidates `availability`, `layout` and `tickets` at once.
- Running fees shown before exit are computed client-side for display only; the charged fee always comes from the exit-scan response.
- v2: replace polling with Server-Sent Events fed by the ticket-events topic.

### 10.6 UX rules

- Every action button shows a pending state and is disabled until the request settles.
- Each gate request sends a fresh `Idempotency-Key`, reused on automatic retry.
- API error `code`s map to plain messages, e.g. `NO_SPOT_AVAILABLE` shows "No car spots free right now".
- WCAG 2.1 AA: labels on all inputs, colour never the only signal ("Full" is text too); simulator dialogs trap focus and close with Esc.

## 11. Parking Lot Simulator

The simulator (`/admin/simulator`) is a top-down, animated view of the lot that drives registered vehicles through the real gate API: it stands in for the scanner, the barriers and the PAY kiosk, so the whole system, including Kafka and the emails, runs end to end without hardware.

&#91;image: Reference layout for the simulator: P board, rows A and B of spots, one-way lane, IN gate with ticket kiosk, PAY kiosk and OUT gate\]

### 11.1 Layout

| Element | Where (reference image) | Behaviour |
| --- | --- | --- |
| P board | Top left, "P 6 OPEN" | Free spot count from the layout API; turns red with "FULL" at 0 |
| Spot rows | Row A (A1–A6) at the top, row B (B1–B6) at the bottom | Drawn from `row_label` and `position`; each shows its code and type letter C, R or L; out-of-service spots are hatched |
| Driving lane | Between and around the rows, with arrows | One-way loop: in at bottom left, up the left side, along the middle lane, down the right side to the exit |
| IN gate | Bottom left: TICKET kiosk and barrier | Vehicles stop here; the entry scan fires; the barrier lifts only on admit |
| PAY kiosk and OUT gate | Bottom right | Vehicles stop at PAY; exit scan and payment dialog; the OUT barrier lifts after payment |
| Vehicles | Coloured top-down shapes | Motorcycle small, car medium, truck long; colour fixed per vehicle (from its id); plate shown on hover |
| Landscaping | Green blocks | Decoration only |

The layout grows with the data: adding row C or more positions in the admin Spots screen adds them to the drawing.

### 11.2 Vehicle journey

&#91;embedded content: simulator vehicle journey · 2 dialogs, 1 decision\]

The entry dialog appears the moment the IN scan admits the vehicle; the payment dialog appears at the PAY kiosk, and only paying lifts the OUT barrier.

### 11.3 Dialogs

**Entry dialog** ("Vehicle at entry gate"), shown when the entry scan admits the vehicle:

| Field | Example |
| --- | --- |
| Vehicle number | KA01AB1234 |
| Owner email | owner@example.com |
| Vehicle type | Car |
| Assigned spot | A3 · Regular |
| Ticket number | PLM-20261104-000123 |
| Entry time | 04 Nov 2026, 10:05 AM IST |
| Entry email | Queued → Sent (live) |

Button: "Park" (in auto mode the dialog closes itself after 3 seconds). The barrier lifts and the vehicle drives to A3.

**Rejection dialog** ("Entry refused"): vehicle number, the reason in plain words (not registered, email not verified, already parked, no free spot for this type), and for unregistered vehicles the registration QR code. Button: "Turn back"; the vehicle reverses out.

**Payment dialog** ("Payment at exit"), shown when the vehicle reaches the PAY kiosk:

| Field | Example |
| --- | --- |
| Vehicle number | KA01AB1234 |
| Owner email | owner@example.com |
| Entry → exit | 10:05 AM → 01:25 PM IST |
| Duration | 3 h 20 min, billed as 4 h |
| Fee | 4 h × ₹40 = ₹160.00 |

Buttons: "Pay ₹160.00 by UPI" and "Pay cash" (both simulated). A ₹0 stay shows "No charge, within 10 free minutes" and a single "Exit" button.

### 11.4 Controls

| Control | Effect |
| --- | --- |
| Send vehicle | Searchable list of active registered vehicles, plus "Random"; the vehicle enters at the IN lane |
| Unregistered plate | Sends a made-up number to show the rejection path |
| Click a parked vehicle | Popover: ticket number, entry time, elapsed time, running fee, "Leave" with stay length (actual, 30 min, 3 h, 26 h) |
| Auto mode | Sends a random vehicle every N seconds (default 8) and makes parked vehicles leave after random stays |
| Speed | 1×, 2×, 4× animation speed |
| Event log | Side panel of the last 20 `VehicleEntered` / `VehicleExited` events with Kafka and email status |
| Reset | Closes all simulated tickets and frees their spots (confirm first) |

### 11.5 Build notes

- Each vehicle is a small state machine: `QUEUED → TO_ENTRY → AT_ENTRY → TO_SPOT → PARKED → TO_PAY → AT_PAY → EXITING → GONE`, with `AT_ENTRY → TURNING_BACK` on rejection.
- Only three moments call the server: arrival at IN (`/gate/entry-scans`), arrival at PAY (`/gate/exit-scans`), and Pay (`/gate/payments`). Everything else is animation.
- Paths are built from fixed lane waypoints plus a turn into the spot's column, so any spot code maps to a route.
- One vehicle at a time stands at each gate; others queue behind it.
- The 2-second layout poll reconciles the drawing with the server: an occupied spot with no animated vehicle is drawn parked at once, and a vehicle whose ticket closed elsewhere is removed.
- If a gate call fails, the dialog shows the error with "Retry"; the vehicle waits at the gate.
- The simulator sends real emails. To save Resend quota, register test vehicles with your own address or Resend's test inboxes.
- The screen and its gate devices exist only when `SIMULATOR_ENABLED=true` (on in local and staging, off in production).

## 12. Non-Functional Requirements

The gate path (check-in and check-out) must be fast and correct under concurrency; everything downstream of Kafka may lag without affecting it.

| ID | Area | Requirement |
| --- | --- | --- |
| NFR-1 | Performance | Entry scan, exit scan and payment p95 under 300 ms at 20 requests/second; availability read p95 under 150 ms. |
| NFR-2 | Performance | Entry/exit email sent within 60 seconds of the event, p95, while under quota. |
| NFR-3 | Correctness | Zero double allocations under 50 concurrent entry scans for the last free spot (load-tested). |
| NFR-4 | Reliability | API availability 99.5% monthly; gate scans and payments work with Kafka or Resend down (outbox buffers events). |
| NFR-5 | Reliability | No event loss: every committed ticket change has exactly one outbox row and is published at least once. |
| NFR-6 | Scalability | Supports 1,000 spots and 5,000 tickets/day on a single API instance; API and consumers are stateless and scale horizontally. |
| NFR-7 | Security | Admin passwords hashed with argon2id; admin JWT 15 min with 7-day refresh, owner JWT 30 days, refresh tokens in HttpOnly cookies. Email codes and gate API keys stored only as hashes; a key is shown once when created. |
| NFR-8 | Security | Role checks on every endpoint; /gate/\* accepts only gate keys or simulator headers; code requests rate-limited per email and per IP; parameterised SQL only (sqlc); CORS limited to the frontend origin; HTTPS only. |
| NFR-9 | Security | Secrets (DB, Kafka, Resend key, JWT key) from environment or a secret manager, never in the repo. |
| NFR-10 | Privacy | Email and mobile masked in logs (`o***@example.com`); only admins and the owner can view them, and real gate displays mask them. |
| NFR-11 | Observability | Structured JSON logs (`log/slog`) with request ID and trace ID; trace context passed in Kafka headers. |
| NFR-12 | Observability | Prometheus metrics: request latency, scans by result, payments, occupancy per spot type, outbox lag, consumer lag, emails by status. |
| NFR-13 | Observability | Alerts: outbox lag over 2 min, consumer lag over 1,000, email failure rate over 10% in 15 min, any type at 100% occupancy. |
| NFR-14 | Data | Daily Postgres backups kept 30 days; tickets kept at least 1 year for audit. |
| NFR-15 | Maintainability | Go services follow a handler → service → repository layout; fee calculator is a pure function with 100% branch coverage. |
| NFR-16 | Testing | Unit tests for rules; integration tests with Testcontainers (Postgres, Kafka); Playwright end-to-end that drives the simulator: register, enter, park, pay, exit. |
| NFR-17 | Deployment | Docker images per service; `docker compose up` starts Postgres, Kafka (KRaft), Kafka UI, API, notification service and frontend locally. |
| NFR-18 | Usability | A new owner registers a vehicle on a phone in under 60 seconds, including the email code. The simulator animates 20 vehicles smoothly (60 fps) on a mid-range laptop. |

### 12.1 Repository layout (suggested)

```text
plms/
  apps/web/                 React + TanStack frontend
    src/routes/(public)/    availability, register, my
    src/routes/admin/       simulator, exceptions, tickets, spots, pricing, gates
    src/simulator/          lot drawing, lane paths, vehicle state machine
  services/api/             Go REST API + outbox relay
    cmd/api/main.go
    internal/{vehicle,spot,ticket,pricing,auth,outbox}/
    migrations/
  services/notifier/        Go Kafka consumer + Resend sender
    templates/{entry,exit}.html
  api/openapi.yaml          Single API contract
  deploy/docker-compose.yml
```

## 13. Edge Cases, Acceptance Criteria and Open Questions

Every failure at the gate ends in a clear rejection, a deferred email or an admin queue item; none leaves a spot or ticket half-done.

### 13.1 Edge cases

| Case | Expected behaviour |
| --- | --- |
| Scanner reads the same plate 3 times as a car pulls up | The first scan acts; repeats within 30 seconds return its result (FR-19) |
| Two vehicles race for the last car spot | One is admitted; the other gets `NO_SPOT_AVAILABLE` (`SKIP LOCKED`) |
| Pay pressed twice / network retry | Same response via `Idempotency-Key`; otherwise `TICKET_ALREADY_CLOSED` with the original receipt |
| Exit within 10 minutes | Fee ₹0; the ticket closes at the exit scan with no payment step; exit email still sent |
| Stay longer than 24 hours | Daily cap per full day plus capped remainder (BR-10) |
| Rate card changed mid-stay | Ticket keeps the rate from entry (BR-11) |
| Owner changes email while parked | Exit email goes to the email snapshotted on the ticket at entry |
| Unregistered vehicle at IN | Rejected `NOT_REGISTERED`; the gate shows the registration QR code; logged for admins |
| Registered but code never entered | Rejected `NOT_VERIFIED`; the owner can request a new code from `/register` |
| Someone registers another person's number | Real owner uses "Report a problem"; an admin checks the registration ID and reassigns the vehicle |
| Plate unreadable | Rejected `UNREADABLE_PLATE`; an admin can check the vehicle in by hand |
| Vehicle at PAY does not pay | OUT barrier stays closed; ticket stays `PAYMENT_PENDING` and reaches the admin queue after 15 minutes |
| Exit scan with no open ticket | Rejected `NO_ACTIVE_TICKET`; admin queue |
| Spot set out of service while occupied | Refused with `SPOT_OCCUPIED` |
| Kafka down during a scan | The scan succeeds; the event waits in the outbox and publishes on recovery |
| Resend daily quota reached | Entry/exit emails `DEFERRED` and retried; registration shows "try again later" |
| Simulator and a real gate both active | Both write real tickets; simulator tickets are flagged and excluded from revenue |

### 13.2 Acceptance criteria (samples)

1. **Self-registration.** Given a new email, when an owner registers KA01AB1234 as a car and enters the emailed code, then the vehicle is `ACTIVE` and the next entry scan admits it.
2. **Unverified vehicles wait.** Given a registration whose code was never entered, when the plate is scanned at IN, then the scan is rejected `NOT_VERIFIED` and no ticket is created.
3. **Simulator entry dialog.** Given a verified car and free regular spots A3–A5, when the simulator sends it to the IN gate, then the dialog shows its number, owner email, spot A3 and the ticket number; the P board drops by 1; the entry email arrives within 60 seconds.
4. **Fee and payment.** Given that car, when it leaves with a 3 h 20 min simulated stay, then the payment dialog shows ₹160.00; after Pay the ticket is `CLOSED`, A3 is free, and the exit email shows ₹160.00 paid.
5. **Rejection path.** Given an unregistered plate, when the simulator sends it to IN, then the rejection dialog shows "not registered" with the QR code and the vehicle turns back.
6. **One email per event.** Given a `VehicleExited` event delivered twice, when the consumer processes both, then exactly one exit email is sent.

### 13.3 Open questions

- [x] Who registers vehicles: resolved. Owners register themselves; there is no gate operator (FR-1 to FR-9).
- [x] How exit is paid: resolved for v1. Payment is simulated at the PAY kiosk; a real gateway is a future enhancement.
- [ ] Confirm the rate card, grace period and daily caps in section 5.3.
- [ ] Should overflow allocation be on or off at launch?
- [ ] Should the public availability page show spot codes, or only counts?
- [ ] Should registration ID (RC number) be mandatory, to make ownership disputes easier to settle?
- [ ] Will daily volume exceed \~40–50 visits, requiring the paid Resend plan?

### 13.4 Future enhancements

- Real ANPR cameras and barrier controllers, plugged into the same `/gate/*` API the simulator uses
- Online payment (UPI / Razorpay) at the PAY kiosk, with a `PaymentCompleted` event
- Spot reservations and monthly passes
- Multi-lot support with a `lot_id` on spots and tickets
- SMS / WhatsApp notifications using the optional mobile number
- Occupancy and revenue dashboard fed from the ticket-events topic
