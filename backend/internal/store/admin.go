package store

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"plms/internal/platform/db"
)

func AdminByEmail(ctx context.Context, q db.DBTX, email string) (AdminUser, error) {
	var a AdminUser
	err := q.QueryRow(ctx, `SELECT id, email, name, password_hash, created_at, last_login_at FROM admin_users WHERE email = $1`, email).
		Scan(&a.ID, &a.Email, &a.Name, &a.PasswordHash, &a.CreatedAt, &a.LastLoginAt)
	return a, err
}

func AdminByID(ctx context.Context, q db.DBTX, id string) (AdminUser, error) {
	var a AdminUser
	err := q.QueryRow(ctx, `SELECT id, email, name, password_hash, created_at, last_login_at FROM admin_users WHERE id = $1`, id).
		Scan(&a.ID, &a.Email, &a.Name, &a.PasswordHash, &a.CreatedAt, &a.LastLoginAt)
	return a, err
}

func CountAdmins(ctx context.Context, q db.DBTX) (int, error) {
	var n int
	err := q.QueryRow(ctx, `SELECT count(*) FROM admin_users`).Scan(&n)
	return n, err
}

func InsertAdmin(ctx context.Context, q db.DBTX, email, name, hash string) error {
	_, err := q.Exec(ctx, `INSERT INTO admin_users (email, name, password_hash) VALUES ($1, $2, $3) ON CONFLICT (email) DO NOTHING`,
		email, name, hash)
	return err
}

func TouchAdminLogin(ctx context.Context, q db.DBTX, id string) error {
	_, err := q.Exec(ctx, `UPDATE admin_users SET last_login_at = now() WHERE id = $1`, id)
	return err
}

// ---- audit trail & disputes ----

type NewAction struct {
	AdminID    *string
	Action     string
	TargetType string
	TargetID   string
	Reason     string
	Details    map[string]any
}

// InsertAction records who did what, when and why (FR-40).
func InsertAction(ctx context.Context, q db.DBTX, a NewAction) (string, error) {
	if a.Details == nil {
		a.Details = map[string]any{}
	}
	d, err := json.Marshal(a.Details)
	if err != nil {
		return "", err
	}
	var reason *string
	if a.Reason != "" {
		reason = &a.Reason
	}
	var target *string
	if a.TargetID != "" {
		target = &a.TargetID
	}
	var id string
	err = q.QueryRow(ctx, `INSERT INTO admin_actions (admin_id, action, target_type, target_id, reason, details)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`, a.AdminID, a.Action, a.TargetType, target, reason, d).Scan(&id)
	return id, err
}

const actionSelect = `SELECT a.id, a.admin_id, u.email, a.action, a.target_type, a.target_id, a.reason, a.details,
	a.created_at, a.resolved_at, a.resolution FROM admin_actions a LEFT JOIN admin_users u ON u.id = a.admin_id`

func scanAction(row pgx.Row) (AdminAction, error) {
	var a AdminAction
	var details []byte
	err := row.Scan(&a.ID, &a.AdminID, &a.AdminEmail, &a.Action, &a.TargetType, &a.TargetID, &a.Reason, &details,
		&a.CreatedAt, &a.ResolvedAt, &a.Resolution)
	if err == nil {
		_ = json.Unmarshal(details, &a.Details)
	}
	return a, err
}

func collectActions(rows pgx.Rows, err error) ([]AdminAction, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AdminAction{}
	for rows.Next() {
		a, err := scanAction(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func ListActions(ctx context.Context, q db.DBTX, limit, offset int) ([]AdminAction, int, error) {
	var total int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM admin_actions`).Scan(&total); err != nil {
		return nil, 0, err
	}
	list, err := collectActions(q.Query(ctx, actionSelect+` ORDER BY a.created_at DESC LIMIT $1 OFFSET $2`, clampLimit(limit), offset))
	return list, total, err
}

func OpenDisputes(ctx context.Context, q db.DBTX) ([]AdminAction, error) {
	return collectActions(q.Query(ctx, actionSelect+` WHERE a.action = 'DISPUTE_OPENED' AND a.resolved_at IS NULL
		ORDER BY a.created_at DESC LIMIT 200`))
}

func ActionByID(ctx context.Context, q db.DBTX, id string) (AdminAction, error) {
	return scanAction(q.QueryRow(ctx, actionSelect+` WHERE a.id = $1`, id))
}

func ResolveAction(ctx context.Context, q db.DBTX, id, adminID, resolution string) error {
	_, err := q.Exec(ctx, `UPDATE admin_actions SET resolved_at = now(), resolved_by = $2, resolution = $3
		WHERE id = $1 AND resolved_at IS NULL`, id, adminID, resolution)
	return err
}
