package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"plms/internal/platform/db"
)

const gateCols = `id, name, kind, is_simulator, active, api_key_hash IS NOT NULL, last_seen_at, created_at`

func scanGate(row pgx.Row) (GateDevice, error) {
	var g GateDevice
	err := row.Scan(&g.ID, &g.Name, &g.Kind, &g.IsSimulator, &g.Active, &g.HasKey, &g.LastSeenAt, &g.CreatedAt)
	return g, err
}

func GateByKeyHash(ctx context.Context, q db.DBTX, hash string) (GateDevice, error) {
	return scanGate(q.QueryRow(ctx, `SELECT `+gateCols+` FROM gate_devices WHERE api_key_hash = $1`, hash))
}

func GateByName(ctx context.Context, q db.DBTX, name string) (GateDevice, error) {
	return scanGate(q.QueryRow(ctx, `SELECT `+gateCols+` FROM gate_devices WHERE name = $1`, name))
}

func GateByID(ctx context.Context, q db.DBTX, id string) (GateDevice, error) {
	return scanGate(q.QueryRow(ctx, `SELECT `+gateCols+` FROM gate_devices WHERE id = $1`, id))
}

func ListGates(ctx context.Context, q db.DBTX) ([]GateDevice, error) {
	rows, err := q.Query(ctx, `SELECT `+gateCols+` FROM gate_devices ORDER BY is_simulator, kind, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GateDevice{}
	for rows.Next() {
		g, err := scanGate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func InsertGate(ctx context.Context, q db.DBTX, name, kind, keyHash string) (GateDevice, error) {
	return scanGate(q.QueryRow(ctx, `INSERT INTO gate_devices (name, kind, api_key_hash) VALUES ($1, $2, $3)
		RETURNING `+gateCols, name, kind, keyHash))
}

func SetGateKey(ctx context.Context, q db.DBTX, id, keyHash string) (GateDevice, error) {
	return scanGate(q.QueryRow(ctx, `UPDATE gate_devices SET api_key_hash = $2 WHERE id = $1 AND NOT is_simulator
		RETURNING `+gateCols, id, keyHash))
}

func UpdateGate(ctx context.Context, q db.DBTX, id, name string, active bool) (GateDevice, error) {
	return scanGate(q.QueryRow(ctx, `UPDATE gate_devices SET name = $2, active = $3 WHERE id = $1
		RETURNING `+gateCols, id, name, active))
}

// TouchGate records last_seen_at, at most every 10 s per device to spare writes.
func TouchGate(ctx context.Context, q db.DBTX, id string) error {
	_, err := q.Exec(ctx, `UPDATE gate_devices SET last_seen_at = now()
		WHERE id = $1 AND (last_seen_at IS NULL OR last_seen_at < now() - interval '10 seconds')`, id)
	return err
}

// ---- scans ----

func InsertScan(ctx context.Context, q db.DBTX, s Scan) (int64, error) {
	var id int64
	err := q.QueryRow(ctx, `INSERT INTO gate_scans (gate_id, plate_raw, plate_normalised, result, reject_reason, ticket_id, deduplicated)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		s.GateID, truncate(s.PlateRaw, 30), s.PlateNormalised, s.Result, s.RejectReason, s.TicketID, s.Deduplicated).Scan(&id)
	return id, err
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// RecentScan returns the latest acting (non-deduplicated) scan of a plate at a gate since a time (FR-19).
func RecentScan(ctx context.Context, q db.DBTX, gateID, plate string, since time.Time) (Scan, error) {
	var s Scan
	err := q.QueryRow(ctx, `SELECT id, gate_id, plate_raw, plate_normalised, result, reject_reason, ticket_id, scanned_at
		FROM gate_scans WHERE gate_id = $1 AND plate_normalised = $2 AND scanned_at > $3 AND NOT deduplicated
		ORDER BY scanned_at DESC LIMIT 1`, gateID, plate, since).
		Scan(&s.ID, &s.GateID, &s.PlateRaw, &s.PlateNormalised, &s.Result, &s.RejectReason, &s.TicketID, &s.ScannedAt)
	return s, err
}

const scanSelect = `SELECT s.id, s.gate_id, g.name, g.kind, s.plate_raw, s.plate_normalised, s.result, s.reject_reason,
	s.ticket_id, s.scanned_at, s.deduplicated, s.resolved_at, s.resolution
	FROM gate_scans s JOIN gate_devices g ON g.id = s.gate_id`

func scanScan(row pgx.Row) (Scan, error) {
	var s Scan
	err := row.Scan(&s.ID, &s.GateID, &s.GateName, &s.GateKind, &s.PlateRaw, &s.PlateNormalised, &s.Result, &s.RejectReason,
		&s.TicketID, &s.ScannedAt, &s.Deduplicated, &s.ResolvedAt, &s.Resolution)
	return s, err
}

// OpenRejectedScans lists unresolved rejected scans for the exception queue (FR-38).
func OpenRejectedScans(ctx context.Context, q db.DBTX, limit int) ([]Scan, error) {
	rows, err := q.Query(ctx, scanSelect+` WHERE s.result = 'REJECTED' AND s.resolved_at IS NULL AND NOT s.deduplicated
		ORDER BY s.scanned_at DESC LIMIT $1`, clampLimit(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Scan{}
	for rows.Next() {
		s, err := scanScan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func ScanByID(ctx context.Context, q db.DBTX, id int64) (Scan, error) {
	return scanScan(q.QueryRow(ctx, scanSelect+` WHERE s.id = $1`, id))
}

func ResolveScan(ctx context.Context, q db.DBTX, id int64, adminID, resolution string) error {
	_, err := q.Exec(ctx, `UPDATE gate_scans SET resolved_at = now(), resolved_by = $2, resolution = $3
		WHERE id = $1 AND resolved_at IS NULL`, id, adminID, truncate(resolution, 200))
	return err
}

// ResolveScansForPlate closes open rejected scans for a plate (e.g. after a manual check-in).
func ResolveScansForPlate(ctx context.Context, q db.DBTX, plate, adminID, resolution string) error {
	_, err := q.Exec(ctx, `UPDATE gate_scans SET resolved_at = now(), resolved_by = $2, resolution = $3
		WHERE plate_normalised = $1 AND result = 'REJECTED' AND resolved_at IS NULL`, plate, adminID, truncate(resolution, 200))
	return err
}

// ---- idempotency keys ----

type IdempotentResponse struct {
	Endpoint   string
	StatusCode int
	Body       []byte
}

func GetIdempotency(ctx context.Context, q db.DBTX, gateID, key string) (IdempotentResponse, error) {
	var r IdempotentResponse
	err := q.QueryRow(ctx, `SELECT endpoint, status_code, response FROM idempotency_keys
		WHERE gate_id = $1 AND key = $2 AND created_at > now() - interval '24 hours'`, gateID, key).
		Scan(&r.Endpoint, &r.StatusCode, &r.Body)
	return r, err
}

func PutIdempotency(ctx context.Context, q db.DBTX, gateID, key, endpoint string, status int, body []byte) error {
	_, err := q.Exec(ctx, `INSERT INTO idempotency_keys (gate_id, key, endpoint, status_code, response)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (gate_id, key) DO UPDATE SET endpoint = EXCLUDED.endpoint, status_code = EXCLUDED.status_code,
		  response = EXCLUDED.response, created_at = now()
		WHERE idempotency_keys.created_at <= now() - interval '24 hours'`, gateID, truncate(key, 100), endpoint, status, body)
	return err
}

func PurgeIdempotency(ctx context.Context, q db.DBTX) (int64, error) {
	tag, err := q.Exec(ctx, `DELETE FROM idempotency_keys WHERE created_at < now() - interval '24 hours'`)
	return tag.RowsAffected(), err
}
