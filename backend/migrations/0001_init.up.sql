-- PLMS core schema (SRS section 6).
-- Extra columns beyond the SRS DDL are marked "-- ext:" with the requirement they serve.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

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
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id     UUID NOT NULL REFERENCES owners(id),
  purpose      VARCHAR(10) NOT NULL,                   -- VERIFY | SIGN_IN | CHG_EMAIL
  code_hash    VARCHAR(100) NOT NULL,                  -- never store the code itself
  sent_to      VARCHAR(254) NOT NULL,                  -- ext: FR-4 rate limit per email address
  target_email VARCHAR(254),                           -- ext: FR-7 new address for CHG_EMAIL
  expires_at   TIMESTAMPTZ NOT NULL,                   -- now() + 10 min
  attempts     SMALLINT NOT NULL DEFAULT 0,            -- max 5
  consumed_at  TIMESTAMPTZ,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_email_codes_owner ON email_codes (owner_id, purpose, created_at DESC);
CREATE INDEX idx_email_codes_sent_to ON email_codes (sent_to, created_at DESC);

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
CREATE INDEX idx_vehicles_owner ON vehicles (owner_id);

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
  grace_minutes     INT    NOT NULL DEFAULT 10 CHECK (grace_minutes >= 0),
  effective_from    TIMESTAMPTZ NOT NULL,
  effective_to      TIMESTAMPTZ                        -- NULL = current
);
CREATE UNIQUE INDEX uq_pricing_current ON pricing_rules (vehicle_type) WHERE effective_to IS NULL;

CREATE TABLE gate_devices (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name         VARCHAR(50) NOT NULL UNIQUE,            -- e.g. IN-1, OUT-1, SIM-IN
  kind         gate_kind   NOT NULL,
  api_key_hash VARCHAR(100),                           -- NULL for simulator devices
  is_simulator BOOLEAN     NOT NULL DEFAULT false,
  active       BOOLEAN     NOT NULL DEFAULT true,
  last_seen_at TIMESTAMPTZ,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_gate_key ON gate_devices (api_key_hash) WHERE api_key_hash IS NOT NULL;

CREATE SEQUENCE ticket_number_seq;

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
  payment_method   VARCHAR(20),                        -- SIMULATED_UPI | SIMULATED_CASH | WAIVED | MANUAL | NO_CHARGE
  paid_at          TIMESTAMPTZ,
  status           ticket_status NOT NULL DEFAULT 'ACTIVE',
  is_simulated     BOOLEAN NOT NULL DEFAULT false,
  source           VARCHAR(10) NOT NULL DEFAULT 'GATE', -- ext: GATE | SIMULATOR | ADMIN (event envelope)
  CHECK (exit_time IS NULL OR exit_time >= entry_time)
);
CREATE UNIQUE INDEX uq_open_ticket_vehicle ON tickets (vehicle_id)
  WHERE status IN ('ACTIVE', 'PAYMENT_PENDING');
CREATE UNIQUE INDEX uq_open_ticket_spot ON tickets (spot_id)
  WHERE status IN ('ACTIVE', 'PAYMENT_PENDING');
CREATE INDEX idx_tickets_entry ON tickets (entry_time DESC);
CREATE INDEX idx_tickets_vehicle ON tickets (vehicle_id, entry_time DESC);

CREATE TABLE gate_scans (
  id               BIGSERIAL PRIMARY KEY,
  gate_id          UUID NOT NULL REFERENCES gate_devices(id),
  plate_raw        VARCHAR(30) NOT NULL,
  plate_normalised VARCHAR(15),
  result           scan_result NOT NULL,
  reject_reason    VARCHAR(30),                        -- NOT_REGISTERED, NO_SPOT_AVAILABLE, ...
  ticket_id        UUID REFERENCES tickets(id),
  scanned_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  deduplicated     BOOLEAN NOT NULL DEFAULT false,     -- ext: FR-19 repeat read answered from an earlier scan
  resolved_at      TIMESTAMPTZ,                        -- ext: FR-38 exception queue
  resolved_by      UUID,
  resolution       VARCHAR(200)
);
CREATE INDEX idx_scans_recent ON gate_scans (gate_id, plate_normalised, scanned_at DESC);
CREATE INDEX idx_scans_rejected_open ON gate_scans (scanned_at DESC)
  WHERE result = 'REJECTED' AND resolved_at IS NULL AND deduplicated = false;

CREATE TABLE outbox_events (
  id           BIGSERIAL PRIMARY KEY,
  event_id     UUID        NOT NULL UNIQUE,
  event_type   VARCHAR(50) NOT NULL,                   -- VehicleEntered | VehicleExited
  aggregate_id UUID        NOT NULL,                   -- ticket id (Kafka key)
  payload      JSONB       NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ,
  attempts     INT         NOT NULL DEFAULT 0,
  last_error   TEXT                                    -- ext: shown in simulator event panel
);
CREATE INDEX idx_outbox_unpublished ON outbox_events (id) WHERE published_at IS NULL;
CREATE INDEX idx_outbox_aggregate ON outbox_events (aggregate_id);

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
  sent_at             TIMESTAMPTZ,
  payload             JSONB NOT NULL,                  -- ext: event body, so retries and resends survive outbox purge
  generation          INT NOT NULL DEFAULT 0           -- ext: bumped on admin resend (Resend idempotency key)
);
CREATE INDEX idx_email_due ON email_notifications (next_attempt_at)
  WHERE status IN ('QUEUED', 'DEFERRED');
CREATE INDEX idx_email_ticket ON email_notifications (ticket_id);

CREATE TABLE admin_users (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email         VARCHAR(254) NOT NULL UNIQUE,
  name          VARCHAR(100) NOT NULL DEFAULT '',
  password_hash VARCHAR(200) NOT NULL,                 -- argon2id PHC string
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_login_at TIMESTAMPTZ
);

-- Audit trail and dispute records (FR-38, FR-40).
-- A public dispute is a row with action = 'DISPUTE_OPENED' and admin_id NULL.
CREATE TABLE admin_actions (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  admin_id    UUID REFERENCES admin_users(id),
  action      VARCHAR(40) NOT NULL,
  target_type VARCHAR(20) NOT NULL,                    -- TICKET | VEHICLE | SPOT | GATE | PRICING | SCAN | SETTINGS | DISPUTE
  target_id   VARCHAR(64),
  reason      TEXT,
  details     JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  resolved_at TIMESTAMPTZ,
  resolved_by UUID REFERENCES admin_users(id),
  resolution  TEXT
);
CREATE INDEX idx_admin_actions_created ON admin_actions (created_at DESC);
CREATE INDEX idx_disputes_open ON admin_actions (created_at DESC)
  WHERE action = 'DISPUTE_OPENED' AND resolved_at IS NULL;

-- ext: FR-36 overflow switch (single-row settings table).
CREATE TABLE lot_settings (
  id               BOOLEAN PRIMARY KEY DEFAULT true CHECK (id),
  overflow_enabled BOOLEAN NOT NULL DEFAULT false,
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ext: section 7.2 Idempotency-Key on /gate/* (24 h replay window).
CREATE TABLE idempotency_keys (
  gate_id     UUID NOT NULL REFERENCES gate_devices(id),
  key         VARCHAR(100) NOT NULL,
  endpoint    VARCHAR(50) NOT NULL,
  status_code INT NOT NULL,
  response    JSONB NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (gate_id, key)
);
