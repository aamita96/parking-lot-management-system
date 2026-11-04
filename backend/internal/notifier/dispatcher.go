package notifier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kgo"

	"plms/internal/domain"
	"plms/internal/email"
	"plms/internal/events"
	"plms/internal/platform/db"
	"plms/internal/platform/metrics"
	"plms/internal/store"
)

// Backoff for 5xx / network errors (SRS 9.5 step 4).
var retryBackoff = []time.Duration{time.Second, 5 * time.Second, 30 * time.Second, 2 * time.Minute, 10 * time.Minute}

const quotaDeferral = time.Hour

type Dispatcher struct {
	Pool     *pgxpool.Pool
	Sender   email.Sender
	Producer *kgo.Client
	DLQTopic string
	Log      *slog.Logger
	Nudge    <-chan struct{}
	// MinInterval throttles sends client-side (2 per second, SRS 9.5 step 6).
	MinInterval time.Duration
}

// Run sends due emails one at a time until ctx ends. Both new and deferred rows are picked up
// here, so this is also the scheduler that retries DEFERRED emails (SRS 9.5 step 5).
func (d *Dispatcher) Run(ctx context.Context) {
	if d.MinInterval == 0 {
		d.MinInterval = 500 * time.Millisecond
	}
	idle := time.NewTicker(2 * time.Second)
	defer idle.Stop()
	for {
		sent, err := d.sendOne(ctx)
		if err != nil && ctx.Err() == nil {
			d.Log.Warn("dispatch error", "err", err)
		}
		wait := d.MinInterval
		if !sent {
			select {
			case <-ctx.Done():
				return
			case <-d.Nudge:
			case <-idle.C:
			}
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// sendOne claims the oldest due email, sends it and records the outcome. It reports whether
// a row was processed.
func (d *Dispatcher) sendOne(ctx context.Context) (bool, error) {
	processed := false
	err := db.InTx(ctx, d.Pool, func(tx pgx.Tx) error {
		rows, err := store.ClaimDueNotifications(ctx, tx, 1)
		if err != nil || len(rows) == 0 {
			return err
		}
		n := rows[0]
		processed = true

		// An exit email never goes before its entry email (SRS 9.5).
		if n.Kind == "EXIT" {
			entry, err := store.NotificationByTicketKind(ctx, tx, n.TicketID, "ENTRY")
			if err == nil && (entry.Status == "QUEUED" || entry.Status == "DEFERRED") {
				next := time.Now().Add(2 * time.Second)
				if entry.NextAttemptAt != nil && entry.NextAttemptAt.After(next) {
					next = entry.NextAttemptAt.Add(time.Second)
				}
				return store.RescheduleNotification(ctx, tx, n.ID, n.Status, "waiting for entry email", next, false)
			} else if err != nil && !db.IsNoRows(err) {
				return err
			}
		}

		var env events.Envelope
		if err := json.Unmarshal(n.Payload, &env); err != nil {
			return d.fail(ctx, tx, n, "bad payload: "+err.Error())
		}
		var msg email.Message
		if n.Kind == "ENTRY" {
			msg, err = email.EntryEmail(env)
		} else {
			msg, err = email.ExitEmail(env)
		}
		if err != nil {
			return d.fail(ctx, tx, n, "render: "+err.Error())
		}
		msg.To = n.Recipient
		msg.IdempotencyKey = fmt.Sprintf("%s-%d", n.EventID, n.Generation) // FR-30 even across crashes
		msg.Tags = map[string]string{"kind": n.Kind}

		sctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		providerID, sendErr := d.Sender.Send(sctx, msg)
		cancel()
		switch {
		case sendErr == nil:
			metrics.Emails.WithLabelValues(n.Kind, "SENT").Inc()
			d.Log.Info("email sent", "event_id", n.EventID, "kind", n.Kind, "provider_id", providerID,
				"to", domain.MaskEmail(n.Recipient), "trace_id", env.TraceID)
			return store.MarkNotificationSent(ctx, tx, n.ID, providerID)
		case errors.Is(sendErr, email.ErrQuota):
			metrics.Emails.WithLabelValues(n.Kind, "DEFERRED").Inc()
			d.Log.Warn("email quota reached, deferring", "event_id", n.EventID)
			return store.RescheduleNotification(ctx, tx, n.ID, "DEFERRED", sendErr.Error(), time.Now().Add(quotaDeferral), false)
		case errors.Is(sendErr, email.ErrRateLimit):
			return store.RescheduleNotification(ctx, tx, n.ID, "QUEUED", sendErr.Error(), time.Now().Add(2*time.Second), false)
		case errors.Is(sendErr, email.ErrPermanent):
			return d.fail(ctx, tx, n, sendErr.Error())
		default:
			if n.Attempts >= len(retryBackoff) {
				return d.fail(ctx, tx, n, sendErr.Error())
			}
			metrics.Emails.WithLabelValues(n.Kind, "RETRY").Inc()
			next := time.Now().Add(retryBackoff[n.Attempts])
			d.Log.Warn("email send failed, will retry", "event_id", n.EventID, "attempt", n.Attempts+1, "err", sendErr)
			return store.RescheduleNotification(ctx, tx, n.ID, "QUEUED", sendErr.Error(), next, true)
		}
	})
	return processed, err
}

func (d *Dispatcher) fail(ctx context.Context, tx pgx.Tx, n store.EmailNotification, reason string) error {
	metrics.Emails.WithLabelValues(n.Kind, "FAILED").Inc()
	d.Log.Error("email failed", "event_id", n.EventID, "kind", n.Kind, "reason", reason)
	if d.Producer != nil {
		rec := &kgo.Record{Topic: d.DLQTopic, Key: []byte(n.TicketID), Value: n.Payload, Headers: []kgo.RecordHeader{
			{Key: "error", Value: []byte(reason)}, {Key: "stage", Value: []byte("email")}}}
		pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if err := d.Producer.ProduceSync(pctx, rec).FirstErr(); err != nil {
			d.Log.Warn("DLQ publish failed", "err", err)
		}
		cancel()
	}
	return store.MarkNotificationFailed(ctx, tx, n.ID, reason)
}
