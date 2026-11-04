package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"plms/internal/platform/db"
)

const notificationCols = `id, event_id, ticket_id, kind, recipient, status, provider_message_id, attempts, last_error,
	next_attempt_at, created_at, sent_at, payload, generation`

func scanNotification(row pgx.Row) (EmailNotification, error) {
	var n EmailNotification
	err := row.Scan(&n.ID, &n.EventID, &n.TicketID, &n.Kind, &n.Recipient, &n.Status, &n.ProviderMessageID, &n.Attempts,
		&n.LastError, &n.NextAttemptAt, &n.CreatedAt, &n.SentAt, &n.Payload, &n.Generation)
	return n, err
}

// InsertNotification stores the QUEUED row for an event; inserted is false when the event was
// already seen (Kafka redelivery), which makes the consumer idempotent (SRS 8.4, FR-30).
func InsertNotification(ctx context.Context, q db.DBTX, eventID, ticketID, kind, recipient string, payload []byte) (inserted bool, err error) {
	tag, err := q.Exec(ctx, `INSERT INTO email_notifications (event_id, ticket_id, kind, recipient, status, payload, next_attempt_at)
		VALUES ($1, $2, $3, $4, 'QUEUED', $5, now()) ON CONFLICT (event_id) DO NOTHING`,
		eventID, ticketID, kind, recipient, payload)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// ClaimDueNotifications locks emails that are due to be sent, oldest first.
func ClaimDueNotifications(ctx context.Context, q db.DBTX, limit int) ([]EmailNotification, error) {
	rows, err := q.Query(ctx, `SELECT `+notificationCols+` FROM email_notifications
		WHERE status IN ('QUEUED', 'DEFERRED') AND next_attempt_at <= now()
		ORDER BY next_attempt_at, created_at LIMIT $1 FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EmailNotification
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func NotificationsForTicket(ctx context.Context, q db.DBTX, ticketID string) ([]EmailNotification, error) {
	rows, err := q.Query(ctx, `SELECT `+notificationCols+` FROM email_notifications WHERE ticket_id = $1 ORDER BY created_at`, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EmailNotification{}
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func NotificationByTicketKind(ctx context.Context, q db.DBTX, ticketID, kind string) (EmailNotification, error) {
	return scanNotification(q.QueryRow(ctx, `SELECT `+notificationCols+` FROM email_notifications
		WHERE ticket_id = $1 AND kind = $2 ORDER BY created_at DESC LIMIT 1`, ticketID, kind))
}

func MarkNotificationSent(ctx context.Context, q db.DBTX, id, providerID string) error {
	_, err := q.Exec(ctx, `UPDATE email_notifications SET status = 'SENT', provider_message_id = $2, sent_at = now(),
		attempts = attempts + 1, last_error = NULL, next_attempt_at = NULL WHERE id = $1`, id, providerID)
	return err
}

// RescheduleNotification records a failed attempt and sets the next try (status QUEUED or DEFERRED).
func RescheduleNotification(ctx context.Context, q db.DBTX, id, status, lastErr string, next time.Time, countAttempt bool) error {
	inc := 0
	if countAttempt {
		inc = 1
	}
	_, err := q.Exec(ctx, `UPDATE email_notifications SET status = $2, last_error = $3, next_attempt_at = $4,
		attempts = attempts + $5 WHERE id = $1`, id, status, lastErr, next, inc)
	return err
}

func MarkNotificationFailed(ctx context.Context, q db.DBTX, id, lastErr string) error {
	_, err := q.Exec(ctx, `UPDATE email_notifications SET status = 'FAILED', last_error = $2, attempts = attempts + 1,
		next_attempt_at = NULL WHERE id = $1`, id, lastErr)
	return err
}

// RequeueNotification is the admin "Resend" action (FR-32).
func RequeueNotification(ctx context.Context, q db.DBTX, id string) error {
	_, err := q.Exec(ctx, `UPDATE email_notifications SET status = 'QUEUED', attempts = 0, last_error = NULL,
		next_attempt_at = now(), generation = generation + 1 WHERE id = $1`, id)
	return err
}

func EventPayload(ctx context.Context, q db.DBTX, ticketID, eventType string) ([]byte, string, error) {
	var p []byte
	var id string
	err := q.QueryRow(ctx, `SELECT payload, event_id FROM outbox_events WHERE aggregate_id = $1 AND event_type = $2
		ORDER BY id DESC LIMIT 1`, ticketID, eventType).Scan(&p, &id)
	return p, id, err
}
