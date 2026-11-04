package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"plms/internal/platform/db"
)

// InsertOutbox stores an event in the same transaction as the ticket change (SRS 8.3 step 1).
func InsertOutbox(ctx context.Context, q db.DBTX, eventID, eventType, aggregateID string, payload []byte) error {
	_, err := q.Exec(ctx, `INSERT INTO outbox_events (event_id, event_type, aggregate_id, payload) VALUES ($1, $2, $3, $4)`,
		eventID, eventType, aggregateID, payload)
	return err
}

// LockUnpublished claims a batch of unpublished events (SRS 8.3 step 2).
func LockUnpublished(ctx context.Context, q db.DBTX, limit int) ([]OutboxEvent, error) {
	rows, err := q.Query(ctx, `SELECT id, event_id, event_type, aggregate_id, payload, created_at, attempts
		FROM outbox_events WHERE published_at IS NULL ORDER BY id LIMIT $1 FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OutboxEvent
	for rows.Next() {
		var e OutboxEvent
		if err := rows.Scan(&e.ID, &e.EventID, &e.EventType, &e.AggregateID, &e.Payload, &e.CreatedAt, &e.Attempts); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func MarkPublished(ctx context.Context, q db.DBTX, ids []int64) error {
	_, err := q.Exec(ctx, `UPDATE outbox_events SET published_at = now(), last_error = NULL WHERE id = ANY($1)`, ids)
	return err
}

func MarkPublishFailed(ctx context.Context, q db.DBTX, ids []int64, msg string) error {
	_, err := q.Exec(ctx, `UPDATE outbox_events SET attempts = attempts + 1, last_error = $2 WHERE id = ANY($1)`, ids, msg)
	return err
}

// OutboxLag is the age of the oldest unpublished event (0 when none).
func OutboxLag(ctx context.Context, q db.DBTX) (time.Duration, error) {
	var oldest *time.Time
	if err := q.QueryRow(ctx, `SELECT min(created_at) FROM outbox_events WHERE published_at IS NULL`).Scan(&oldest); err != nil {
		return 0, err
	}
	if oldest == nil {
		return 0, nil
	}
	return time.Since(*oldest), nil
}

func PurgePublishedOutbox(ctx context.Context, q db.DBTX, olderThan time.Duration) (int64, error) {
	tag, err := q.Exec(ctx, `DELETE FROM outbox_events WHERE published_at IS NOT NULL
		AND published_at < now() - make_interval(secs => $1)`, olderThan.Seconds())
	return tag.RowsAffected(), err
}

// EventSummary is one row of the simulator event panel (FR-47).
type EventSummary struct {
	EventID       string     `json:"eventId"`
	EventType     string     `json:"eventType"`
	TicketID      string     `json:"ticketId"`
	TicketNumber  string     `json:"ticketNumber"`
	VehicleNumber string     `json:"vehicleNumber"`
	SpotCode      string     `json:"spotCode"`
	Simulated     bool       `json:"simulated"`
	OccurredAt    time.Time  `json:"occurredAt"`
	PublishedAt   *time.Time `json:"publishedAt"`
	Attempts      int        `json:"publishAttempts"`
	PublishError  *string    `json:"publishError"`
	KafkaStatus   string     `json:"kafkaStatus"` // PENDING | PUBLISHED | RETRYING
	EmailStatus   string     `json:"emailStatus"` // WAITING | QUEUED | SENT | FAILED | DEFERRED
	FeePaise      *int64     `json:"feePaise"`
}

func RecentEvents(ctx context.Context, q db.DBTX, limit int, ticketID string) ([]EventSummary, error) {
	rows, err := q.Query(ctx, `SELECT o.event_id, o.event_type, o.aggregate_id, t.ticket_number, v.vehicle_number, s.code,
		t.is_simulated, o.created_at, o.published_at, o.attempts, o.last_error, n.status, t.fee_paise
		FROM outbox_events o
		JOIN tickets t ON t.id = o.aggregate_id
		JOIN vehicles v ON v.id = t.vehicle_id
		JOIN parking_spots s ON s.id = t.spot_id
		LEFT JOIN email_notifications n ON n.event_id = o.event_id
		WHERE ($2 = '' OR o.aggregate_id::text = $2)
		ORDER BY o.id DESC LIMIT $1`, clampLimit(limit), ticketID)
	if err != nil {
		return nil, err
	}
	return collectEvents(rows)
}

func collectEvents(rows pgx.Rows) ([]EventSummary, error) {
	defer rows.Close()
	out := []EventSummary{}
	for rows.Next() {
		var e EventSummary
		var emailStatus *string
		if err := rows.Scan(&e.EventID, &e.EventType, &e.TicketID, &e.TicketNumber, &e.VehicleNumber, &e.SpotCode,
			&e.Simulated, &e.OccurredAt, &e.PublishedAt, &e.Attempts, &e.PublishError, &emailStatus, &e.FeePaise); err != nil {
			return nil, err
		}
		switch {
		case e.PublishedAt != nil:
			e.KafkaStatus = "PUBLISHED"
		case e.Attempts > 0:
			e.KafkaStatus = "RETRYING"
		default:
			e.KafkaStatus = "PENDING"
		}
		switch {
		case emailStatus != nil:
			e.EmailStatus = *emailStatus
		case e.PublishedAt != nil:
			e.EmailStatus = "QUEUED"
		default:
			e.EmailStatus = "WAITING"
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
