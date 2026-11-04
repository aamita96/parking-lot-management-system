package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"plms/internal/domain"
	"plms/internal/platform/db"
)

const spotCols = `id, code, spot_type, floor, row_label, position, status, version`

func scanSpot(row pgx.Row) (Spot, error) {
	var s Spot
	err := row.Scan(&s.ID, &s.Code, &s.SpotType, &s.Floor, &s.RowLabel, &s.Position, &s.Status, &s.Version)
	return s, err
}

func ListSpots(ctx context.Context, q db.DBTX) ([]Spot, error) {
	rows, err := q.Query(ctx, `SELECT `+spotCols+` FROM parking_spots ORDER BY floor, row_label, position`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Spot{}
	for rows.Next() {
		s, err := scanSpot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func SpotByID(ctx context.Context, q db.DBTX, id string) (Spot, error) {
	return scanSpot(q.QueryRow(ctx, `SELECT `+spotCols+` FROM parking_spots WHERE id = $1`, id))
}

func SpotByIDForUpdate(ctx context.Context, q db.DBTX, id string) (Spot, error) {
	return scanSpot(q.QueryRow(ctx, `SELECT `+spotCols+` FROM parking_spots WHERE id = $1 FOR UPDATE`, id))
}

// LockFreeSpot picks the nearest free spot of a type (BR-2) and locks it, skipping rows
// another transaction is allocating (BR-4). Returns pgx.ErrNoRows when none is free.
func LockFreeSpot(ctx context.Context, q db.DBTX, st domain.SpotType) (Spot, error) {
	return scanSpot(q.QueryRow(ctx, `SELECT `+spotCols+` FROM parking_spots
		WHERE spot_type = $1 AND status = 'AVAILABLE'
		ORDER BY floor, row_label, position LIMIT 1 FOR UPDATE SKIP LOCKED`, string(st)))
}

func SetSpotStatus(ctx context.Context, q db.DBTX, id, status string) error {
	_, err := q.Exec(ctx, `UPDATE parking_spots SET status = $2, version = version + 1 WHERE id = $1`, id, status)
	return err
}

func UpdateSpot(ctx context.Context, q db.DBTX, id string, st domain.SpotType, status string) (Spot, error) {
	return scanSpot(q.QueryRow(ctx, `UPDATE parking_spots SET spot_type = $2, status = $3, version = version + 1
		WHERE id = $1 RETURNING `+spotCols, id, string(st), status))
}

func InsertSpot(ctx context.Context, q db.DBTX, code string, st domain.SpotType, floor int, row string, pos int) (Spot, error) {
	return scanSpot(q.QueryRow(ctx, `INSERT INTO parking_spots (code, spot_type, floor, row_label, position)
		VALUES ($1, $2, $3, $4, $5) RETURNING `+spotCols, code, string(st), floor, row, pos))
}

// SpotHasHistory reports whether any ticket ever used the spot (such spots cannot be deleted).
func SpotHasHistory(ctx context.Context, q db.DBTX, id string) (bool, error) {
	var ok bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM tickets WHERE spot_id = $1)`, id).Scan(&ok)
	return ok, err
}

func DeleteSpot(ctx context.Context, q db.DBTX, id string) error {
	_, err := q.Exec(ctx, `DELETE FROM parking_spots WHERE id = $1`, id)
	return err
}

type SpotTypeCount struct {
	SpotType     domain.SpotType
	Total        int
	Available    int
	Occupied     int
	OutOfService int
}

func CountSpotsByType(ctx context.Context, q db.DBTX) (map[domain.SpotType]SpotTypeCount, error) {
	rows, err := q.Query(ctx, `SELECT spot_type,
		count(*), count(*) FILTER (WHERE status = 'AVAILABLE'), count(*) FILTER (WHERE status = 'OCCUPIED'),
		count(*) FILTER (WHERE status = 'OUT_OF_SERVICE')
		FROM parking_spots GROUP BY spot_type`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[domain.SpotType]SpotTypeCount{}
	for rows.Next() {
		var c SpotTypeCount
		if err := rows.Scan(&c.SpotType, &c.Total, &c.Available, &c.Occupied, &c.OutOfService); err != nil {
			return nil, err
		}
		out[c.SpotType] = c
	}
	return out, rows.Err()
}

// ---- pricing ----

const pricingCols = `id, vehicle_type, hourly_rate_paise, daily_cap_paise, grace_minutes, effective_from, effective_to`

func scanPricing(row pgx.Row) (PricingRule, error) {
	var p PricingRule
	err := row.Scan(&p.ID, &p.VehicleType, &p.HourlyRatePaise, &p.DailyCapPaise, &p.GraceMinutes, &p.EffectiveFrom, &p.EffectiveTo)
	return p, err
}

// CurrentPricing is the rate card in force now for a vehicle type (BR-11).
func CurrentPricing(ctx context.Context, q db.DBTX, vt domain.VehicleType) (PricingRule, error) {
	return scanPricing(q.QueryRow(ctx, `SELECT `+pricingCols+` FROM pricing_rules
		WHERE vehicle_type = $1 AND effective_to IS NULL`, string(vt)))
}

func ListCurrentPricing(ctx context.Context, q db.DBTX) ([]PricingRule, error) {
	return listPricing(q.Query(ctx, `SELECT `+pricingCols+` FROM pricing_rules WHERE effective_to IS NULL ORDER BY vehicle_type`))
}

func PricingHistory(ctx context.Context, q db.DBTX, vt domain.VehicleType) ([]PricingRule, error) {
	return listPricing(q.Query(ctx, `SELECT `+pricingCols+` FROM pricing_rules WHERE vehicle_type = $1
		ORDER BY effective_from DESC LIMIT 20`, string(vt)))
}

func listPricing(rows pgx.Rows, err error) ([]PricingRule, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PricingRule{}
	for rows.Next() {
		p, err := scanPricing(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ReplacePricing closes the current rule and inserts a new one effective now (FR-35).
func ReplacePricing(ctx context.Context, q db.DBTX, vt domain.VehicleType, r domain.Rate) (PricingRule, error) {
	if _, err := q.Exec(ctx, `UPDATE pricing_rules SET effective_to = now()
		WHERE vehicle_type = $1 AND effective_to IS NULL`, string(vt)); err != nil {
		return PricingRule{}, err
	}
	return scanPricing(q.QueryRow(ctx, `INSERT INTO pricing_rules
		(vehicle_type, hourly_rate_paise, daily_cap_paise, grace_minutes, effective_from)
		VALUES ($1, $2, $3, $4, now()) RETURNING `+pricingCols,
		string(vt), r.HourlyRatePaise, r.DailyCapPaise, r.GraceMinutes))
}

// ---- settings ----

func OverflowEnabled(ctx context.Context, q db.DBTX) (bool, error) {
	var on bool
	err := q.QueryRow(ctx, `SELECT overflow_enabled FROM lot_settings WHERE id`).Scan(&on)
	if db.IsNoRows(err) {
		return false, nil
	}
	return on, err
}

func SetOverflow(ctx context.Context, q db.DBTX, on bool) error {
	_, err := q.Exec(ctx, `INSERT INTO lot_settings (id, overflow_enabled, updated_at) VALUES (true, $1, now())
		ON CONFLICT (id) DO UPDATE SET overflow_enabled = EXCLUDED.overflow_enabled, updated_at = now()`, on)
	return err
}
