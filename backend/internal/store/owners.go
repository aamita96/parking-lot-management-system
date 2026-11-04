package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"plms/internal/platform/db"
)

const ownerCols = `id, email, mobile, email_verified_at, created_at`

func scanOwner(row pgx.Row) (Owner, error) {
	var o Owner
	err := row.Scan(&o.ID, &o.Email, &o.Mobile, &o.EmailVerifiedAt, &o.CreatedAt)
	return o, err
}

func OwnerByEmail(ctx context.Context, q db.DBTX, email string) (Owner, error) {
	return scanOwner(q.QueryRow(ctx, `SELECT `+ownerCols+` FROM owners WHERE email = $1`, email))
}

func OwnerByID(ctx context.Context, q db.DBTX, id string) (Owner, error) {
	return scanOwner(q.QueryRow(ctx, `SELECT `+ownerCols+` FROM owners WHERE id = $1`, id))
}

// UpsertOwner returns the owner for email, creating it if needed. A supplied mobile is stored only
// while the owner is unverified or has none, so an unverified request cannot overwrite a verified owner's number.
func UpsertOwner(ctx context.Context, q db.DBTX, email string, mobile *string) (Owner, error) {
	return scanOwner(q.QueryRow(ctx, `
		INSERT INTO owners (email, mobile) VALUES ($1, $2)
		ON CONFLICT (email) DO UPDATE SET mobile = CASE
		  WHEN owners.email_verified_at IS NULL OR owners.mobile IS NULL THEN COALESCE(EXCLUDED.mobile, owners.mobile)
		  ELSE owners.mobile END
		RETURNING `+ownerCols, email, mobile))
}

func MarkOwnerVerified(ctx context.Context, q db.DBTX, id string) error {
	_, err := q.Exec(ctx, `UPDATE owners SET email_verified_at = COALESCE(email_verified_at, now()) WHERE id = $1`, id)
	return err
}

func SetOwnerMobile(ctx context.Context, q db.DBTX, id string, mobile *string) error {
	_, err := q.Exec(ctx, `UPDATE owners SET mobile = $2 WHERE id = $1`, id, mobile)
	return err
}

func SetOwnerEmail(ctx context.Context, q db.DBTX, id, email string) error {
	_, err := q.Exec(ctx, `UPDATE owners SET email = $2, email_verified_at = now() WHERE id = $1`, id, email)
	return err
}

// ---- email codes ----

func InsertEmailCode(ctx context.Context, q db.DBTX, id, ownerID, purpose, hash, sentTo string, target *string, ttl time.Duration) (EmailCode, error) {
	var c EmailCode
	err := q.QueryRow(ctx, `
		INSERT INTO email_codes (id, owner_id, purpose, code_hash, sent_to, target_email, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, now() + make_interval(secs => $7))
		RETURNING id, owner_id, purpose, code_hash, sent_to, target_email, expires_at, attempts, consumed_at, created_at`,
		id, ownerID, purpose, hash, sentTo, target, ttl.Seconds()).
		Scan(&c.ID, &c.OwnerID, &c.Purpose, &c.CodeHash, &c.SentTo, &c.TargetEmail, &c.ExpiresAt, &c.Attempts, &c.ConsumedAt, &c.CreatedAt)
	return c, err
}

// CountCodesSince counts codes sent to an address in a window (FR-4: max 3 per hour).
func CountCodesSince(ctx context.Context, q db.DBTX, sentTo string, since time.Time) (int, error) {
	var n int
	err := q.QueryRow(ctx, `SELECT count(*) FROM email_codes WHERE sent_to = $1 AND created_at > $2`, sentTo, since).Scan(&n)
	return n, err
}

// LatestCode returns the newest unconsumed code for owner+purpose, locked for update.
func LatestCode(ctx context.Context, q db.DBTX, ownerID, purpose string) (EmailCode, error) {
	var c EmailCode
	err := q.QueryRow(ctx, `
		SELECT id, owner_id, purpose, code_hash, sent_to, target_email, expires_at, attempts, consumed_at, created_at
		FROM email_codes WHERE owner_id = $1 AND purpose = $2 AND consumed_at IS NULL
		ORDER BY created_at DESC LIMIT 1 FOR UPDATE`, ownerID, purpose).
		Scan(&c.ID, &c.OwnerID, &c.Purpose, &c.CodeHash, &c.SentTo, &c.TargetEmail, &c.ExpiresAt, &c.Attempts, &c.ConsumedAt, &c.CreatedAt)
	return c, err
}

func IncrementCodeAttempts(ctx context.Context, q db.DBTX, id string) error {
	_, err := q.Exec(ctx, `UPDATE email_codes SET attempts = attempts + 1 WHERE id = $1`, id)
	return err
}

func ConsumeCode(ctx context.Context, q db.DBTX, id string) error {
	_, err := q.Exec(ctx, `UPDATE email_codes SET consumed_at = now() WHERE id = $1`, id)
	return err
}
