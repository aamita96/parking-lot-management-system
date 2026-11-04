# PLMS — Parking Lot Management System

An unattended car park: owners register their own vehicles with an email code, the gate reads the
plate and assigns a spot, the PAY kiosk charges a duration-based fee, every entry and exit is
published to Kafka, and the owner gets an email for each. An animated simulator drives the whole
thing end to end without hardware.

Built from `Parking Lot Management System — Software Requirements Specification.md` (v1.1).

## Run it

Requirements: Docker Desktop.

```bash
cp .env.example .env          # optional — defaults work for a local demo
docker compose up --build
```

| What | URL |
| --- | --- |
| App (availability, register, my vehicles) | http://localhost:3000 |
| Admin console + simulator | http://localhost:3000/admin — `admin@plms.local` / `admin12345` |
| API | http://localhost:8080/api/v1 (OpenAPI: `backend/api/openapi.yaml`) |
| Kafka UI | http://localhost:8090 |
| Metrics | http://localhost:8080/metrics, http://localhost:8081/metrics |

**Email.** By default `EMAIL_ENABLED=false`: emails are logged instead of sent, and the
registration and sign-in screens show the 6-digit code ("Demo mode") so you can try everything
without a Resend account. To send real email, set `EMAIL_ENABLED=true`, `RESEND_API_KEY` and an
`EMAIL_FROM` on a domain verified in Resend (`onboarding@resend.dev` only delivers to your own
Resend account address). Set `DEV_EXPOSE_CODES=false` once real email works.

**Demo data.** `SEED_DEMO=true` adds 12 verified vehicles whose owners use Resend's test inbox
(`delivered+…@resend.dev`), so the simulator has traffic straight away and real sends don't reach
anyone.

### Try the full journey

1. Open http://localhost:3000/register, register a car (e.g. `ka 01 ab 1234`), enter the code.
2. Open http://localhost:3000/admin/simulator → **Send vehicle** → pick your car.
3. The entry dialog shows the ticket and spot (A3 on a fresh lot) → **Park**.
4. Click the parked car → **Leave** → **3 h** → the PAY dialog shows `3 h × ₹40 = ₹120.00` → **Pay**.
5. Watch the event panel: Kafka *Published*, email *Sent*. Turn on **Auto** for continuous traffic.

## Layout

```text
backend/                     Go 1.23+ (built with 1.27)
  cmd/api                    REST API + outbox relay + housekeeping jobs
  cmd/notifier               Kafka consumer + email dispatcher (Resend)
  internal/api               HTTP handlers, routing, auth & idempotency middleware
  internal/service           business logic (registration, gate, admin, simulator)
  internal/store             repository layer: parameterised SQL over pgx v5
  internal/domain            pure rules: plate format, fee calculator, money/time formatting
  internal/email             Resend sender + html/text templates (entry, exit, code)
  internal/outbox, kafkax    transactional outbox relay, franz-go clients, topic setup
  internal/notifier          idempotent consumer, retry/backoff/quota dispatcher, DLQ
  migrations/                golang-migrate SQL (schema + seed), embedded in the binary
  api/openapi.yaml           API contract (frontend types are generated from it)
frontend/                    React 19 + Vite + TanStack Router + React Query + Tailwind
  src/main.tsx               where the app starts
  src/routes/                one file = one URL (index.tsx = /, register.tsx = /register, admin/tickets.tsx = /admin/tickets)
  src/components/ui/         small shared components: Button, Card, Field/Input, Modal, Table
  src/components/layout/     PublicLayout and AdminLayout (each page wraps itself in one)
  src/api/                   api() fetch helper and the JSON types (types.ts)
  src/lib/                   formatting, fee calculator, form checks, query client, admin guard
  src/stores/                who is signed in (owner, admin)
  src/simulator/             the animated lot: store.ts (state + tick), gateCalls.ts (server calls),
                             geometry.ts (lanes/bays), movement.ts, vehicles.ts, and the UI pieces
  e2e/                       Playwright journey tests
deploy/prometheus/alerts.yml NFR-13 alert rules
docker-compose.yml           Postgres 16, Kafka 3.9 (KRaft), Kafka UI, API, notifier, web
```

## Tests

```bash
cd backend
go test ./...                                   # unit: fee rules (100% of the calculator), plates, auth, email templates
go test -tags integration ./internal/service/   # Testcontainers Postgres: SRS acceptance criteria,
                                                # 50 concurrent scans for the last spot, notifier idempotency

cd frontend
npm test                                        # fee mirror + validation (Vitest)
npx playwright install chromium                 # once
npm run e2e                                     # against http://localhost:3000 (stack running)
```

## How the main requirements are met

| Area | Where |
| --- | --- |
| Self-registration, codes (10 min, 5 tries, 3/h), FR-1–9 | `service/registration.go`, `frontend/src/routes/register.tsx` |
| Allocation BR-1–5 (`FOR UPDATE SKIP LOCKED`, partial unique indexes) | `service/tickets.go`, `store/spots.go`, `migrations/0001_init.up.sql` |
| Fee BR-6–12 (integer paise, rate snapshot per ticket) | `domain/fee.go` (+ `frontend/src/lib/fee.ts` for the running estimate) |
| Gate API FR-14–27, repeat reads (30 s), Idempotency-Key (24 h) | `service/gate.go`, `api/middleware.go` |
| Outbox → Kafka, keyed by ticket ID, `acks=all`, idempotent producer | `outbox/relay.go`, `kafkax/kafkax.go` |
| One email per event, retries 1 s…10 min, quota → DEFERRED, DLQ, entry-before-exit | `notifier/consumer.go`, `notifier/dispatcher.go` |
| Admin FR-33–40 with audit trail | `service/admin.go`, `service/exceptions.go`, `frontend/src/routes/admin/*` |
| Simulator FR-41–49 | `frontend/src/simulator/*` (start with `routes/admin/simulator.tsx`), `service/simulator.go` |
| Security NFR-7–10 (argon2id, hashed codes/keys, HttpOnly refresh cookie, masking) | `auth/auth.go`, `api/handlers.go`, `domain/format.go` |
| Observability NFR-11–13 (JSON logs with trace IDs, Prometheus, alerts) | `platform/httpx`, `platform/metrics`, `deploy/prometheus` |

## Notes and deviations from the SRS

- **Frontend libraries**: to keep the code readable for new developers, forms use plain `useState`
  with small check functions (`lib/validation.ts`) instead of TanStack Form + Zod, tables are plain
  `<table>` markup instead of TanStack Table, and API types are hand-written in `src/api/types.ts`
  (kept in step with `backend/api/openapi.yaml`) instead of generated.
- **sqlc** is replaced by hand-written, parameterised pgx queries in `internal/store` (sqlc needs
  cgo for its Postgres parser on Windows). The handler → service → repository split is kept.
- **OpenTelemetry**: every request gets a W3C trace ID (an incoming `traceparent` is honoured)
  that flows into logs, the event envelope and a `traceparent` Kafka header. No OTel exporter
  is wired up yet.
- **Extra tables/columns**, all marked `ext:` in the migration: `lot_settings` (overflow switch),
  `idempotency_keys`, and a few columns (e.g. `email_notifications.payload` so retries and
  resends survive the 7-day outbox purge).
- **Status codes** follow the SRS error table (7.7), e.g. `NOT_REGISTERED` is 404 even though the
  7.4 example shows 409.
- **Simulator reset** cancels open simulator tickets without exit events, so no emails are sent.
- **Icons and images**: [Lucide](https://lucide.dev) icons (ISC licence); the logo, vehicles, lot
  and hero illustration are original SVG drawn for this project. No external image assets.
