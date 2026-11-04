package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"plms/internal/domain"
	"plms/internal/platform/db"
)

const vehicleSelect = `
	SELECT v.id, v.vehicle_number, v.vehicle_type, v.owner_id, o.email, o.email_verified_at IS NOT NULL,
	       v.registration_id, v.status, v.created_at, v.updated_at
	FROM vehicles v JOIN owners o ON o.id = v.owner_id`

func scanVehicle(row pgx.Row) (Vehicle, error) {
	var v Vehicle
	err := row.Scan(&v.ID, &v.VehicleNumber, &v.VehicleType, &v.OwnerID, &v.OwnerEmail, &v.OwnerVerified,
		&v.RegistrationID, &v.Status, &v.CreatedAt, &v.UpdatedAt)
	return v, err
}

func collectVehicles(rows pgx.Rows, err error) ([]Vehicle, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Vehicle{}
	for rows.Next() {
		v, err := scanVehicle(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// VehicleByNumber returns the non-removed vehicle with that normalised number.
func VehicleByNumber(ctx context.Context, q db.DBTX, number string) (Vehicle, error) {
	return scanVehicle(q.QueryRow(ctx, vehicleSelect+` WHERE v.vehicle_number = $1 AND v.status <> 'REMOVED'`, number))
}

func VehicleByID(ctx context.Context, q db.DBTX, id string) (Vehicle, error) {
	return scanVehicle(q.QueryRow(ctx, vehicleSelect+` WHERE v.id = $1`, id))
}

func VehiclesByOwner(ctx context.Context, q db.DBTX, ownerID string) ([]Vehicle, error) {
	return collectVehicles(q.Query(ctx, vehicleSelect+` WHERE v.owner_id = $1 AND v.status <> 'REMOVED' ORDER BY v.created_at`, ownerID))
}

func InsertVehicle(ctx context.Context, q db.DBTX, number string, vt domain.VehicleType, ownerID string, regID *string, status string) (string, error) {
	var id string
	err := q.QueryRow(ctx, `
		INSERT INTO vehicles (vehicle_number, vehicle_type, owner_id, registration_id, status)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`, number, string(vt), ownerID, regID, status).Scan(&id)
	return id, err
}

func UpdatePendingVehicle(ctx context.Context, q db.DBTX, id string, vt domain.VehicleType, regID *string) error {
	_, err := q.Exec(ctx, `UPDATE vehicles SET vehicle_type = $2, registration_id = COALESCE($3, registration_id), updated_at = now()
		WHERE id = $1`, id, string(vt), regID)
	return err
}

func SetVehicleStatus(ctx context.Context, q db.DBTX, id, status string) error {
	_, err := q.Exec(ctx, `UPDATE vehicles SET status = $2, updated_at = now() WHERE id = $1`, id, status)
	return err
}

func SetVehicleRegistrationID(ctx context.Context, q db.DBTX, id string, regID *string) error {
	_, err := q.Exec(ctx, `UPDATE vehicles SET registration_id = $2, updated_at = now() WHERE id = $1`, id, regID)
	return err
}

func SetVehicleOwner(ctx context.Context, q db.DBTX, id, ownerID string) error {
	_, err := q.Exec(ctx, `UPDATE vehicles SET owner_id = $2, updated_at = now() WHERE id = $1`, id, ownerID)
	return err
}

// ExpirePendingVehicles removes registrations never verified within 24 h (FR-4).
func ExpirePendingVehicles(ctx context.Context, q db.DBTX) (int64, error) {
	tag, err := q.Exec(ctx, `UPDATE vehicles SET status = 'REMOVED', updated_at = now()
		WHERE status = 'PENDING_VERIFICATION' AND created_at < now() - interval '24 hours'`)
	return tag.RowsAffected(), err
}

type VehicleFilter struct {
	Query  string
	Status string
	Type   string
	Limit  int
	Offset int
}

// SearchVehicles searches by number, owner email or mobile (FR-39).
func SearchVehicles(ctx context.Context, q db.DBTX, f VehicleFilter) ([]Vehicle, int, error) {
	where := []string{"1=1"}
	args := []any{}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.Query != "" {
		add(`(v.vehicle_number ILIKE '%%' || $%[1]d || '%%' OR o.email ILIKE '%%' || $%[1]d || '%%' OR o.mobile ILIKE '%%' || $%[1]d || '%%' OR v.registration_id ILIKE '%%' || $%[1]d || '%%')`, f.Query)
	}
	if f.Status != "" {
		add(`v.status = $%d::vehicle_status`, f.Status)
	} else {
		where = append(where, `v.status <> 'REMOVED'`)
	}
	if f.Type != "" {
		add(`v.vehicle_type = $%d::vehicle_type`, f.Type)
	}
	cond := strings.Join(where, " AND ")
	var total int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM vehicles v JOIN owners o ON o.id = v.owner_id WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, clampLimit(f.Limit), f.Offset)
	list, err := collectVehicles(q.Query(ctx, vehicleSelect+` WHERE `+cond+
		fmt.Sprintf(` ORDER BY v.created_at DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...))
	return list, total, err
}

func clampLimit(n int) int {
	if n <= 0 {
		return 50
	}
	if n > 1000 {
		return 1000
	}
	return n
}

type OwnerSummary struct {
	Owner
	VehicleCount int       `json:"vehicleCount"`
	Vehicles     []Vehicle `json:"vehicles"`
}

// SearchOwners finds owners by email, mobile or one of their vehicle numbers (FR-39).
func SearchOwners(ctx context.Context, q db.DBTX, query string, limit, offset int) ([]OwnerSummary, int, error) {
	cond := `($1 = '' OR o.email ILIKE '%' || $1 || '%' OR o.mobile ILIKE '%' || $1 || '%'
		OR EXISTS (SELECT 1 FROM vehicles v WHERE v.owner_id = o.id AND v.vehicle_number ILIKE '%' || $1 || '%'))`
	var total int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM owners o WHERE `+cond, query).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := q.Query(ctx, `SELECT o.id, o.email, o.mobile, o.email_verified_at, o.created_at,
		(SELECT count(*) FROM vehicles v WHERE v.owner_id = o.id AND v.status <> 'REMOVED')
		FROM owners o WHERE `+cond+` ORDER BY o.created_at DESC LIMIT $2 OFFSET $3`, query, clampLimit(limit), offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []OwnerSummary{}
	for rows.Next() {
		var s OwnerSummary
		if err := rows.Scan(&s.ID, &s.Email, &s.Mobile, &s.EmailVerifiedAt, &s.CreatedAt, &s.VehicleCount); err != nil {
			return nil, 0, err
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	for i := range out {
		vs, err := VehiclesByOwner(ctx, q, out[i].ID)
		if err != nil {
			return nil, 0, err
		}
		out[i].Vehicles = vs
	}
	return out, total, nil
}
