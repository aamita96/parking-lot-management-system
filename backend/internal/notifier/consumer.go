// Package notifier consumes ticket events and sends one entry and one exit email per ticket (SRS 8.4, 9).
package notifier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"

	"plms/internal/domain"
	"plms/internal/events"
	"plms/internal/platform/metrics"
	"plms/internal/store"
)

const maxProcessAttempts = 5

type Consumer struct {
	Pool     *pgxpool.Pool
	Client   *kgo.Client // consumer-group client
	Producer *kgo.Client // for the DLQ
	DLQTopic string
	Group    string
	Log      *slog.Logger
	Nudge    chan<- struct{} // wakes the dispatcher when a new email is queued
}

// NewGroupClient builds the consumer-group client. Offsets are committed manually, only after
// the email row is stored (SRS 8.4).
func NewGroupClient(brokers []string, group, topic string) (*kgo.Client, error) {
	return kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topic),
		kgo.DisableAutoCommit(),
		kgo.BlockRebalanceOnPoll(),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.FetchMaxWait(500*time.Millisecond),
	)
}

func (c *Consumer) Run(ctx context.Context) {
	for {
		fetches := c.Client.PollRecords(ctx, 100)
		if ctx.Err() != nil {
			c.Client.AllowRebalance()
			return
		}
		fetches.EachError(func(topic string, p int32, err error) {
			c.Log.Warn("kafka fetch error", "topic", topic, "partition", p, "err", err)
		})
		// Records arrive in partition order, so an entry is always stored before its exit (SRS 9.5).
		fetches.EachRecord(func(r *kgo.Record) {
			c.handle(ctx, r)
		})
		if fetches.NumRecords() > 0 {
			if err := c.Client.CommitUncommittedOffsets(ctx); err != nil {
				c.Log.Warn("offset commit failed", "err", err)
			}
		}
		c.Client.AllowRebalance()
	}
}

// handle stores the email row for one record, retrying database errors; after 5 failures,
// or if the record cannot be parsed, it goes to the DLQ (SRS 8.4).
func (c *Consumer) handle(ctx context.Context, r *kgo.Record) {
	var env events.Envelope
	if err := json.Unmarshal(r.Value, &env); err != nil || env.EventID == "" || env.Data.TicketID == "" {
		if err == nil {
			err = errors.New("envelope missing eventId or ticketId")
		}
		c.toDLQ(ctx, r, "parse: "+err.Error())
		return
	}
	var kind string
	switch env.EventType {
	case domain.EventEntered:
		kind = "ENTRY"
	case domain.EventExited:
		kind = "EXIT"
	default:
		c.Log.Info("ignoring event type", "type", env.EventType)
		return
	}
	var lastErr error
	for attempt := 1; attempt <= maxProcessAttempts; attempt++ {
		inserted, err := store.InsertNotification(ctx, c.Pool, env.EventID, env.Data.TicketID, kind,
			env.Data.Vehicle.OwnerEmail, r.Value)
		if err == nil {
			if inserted {
				c.Log.Info("email queued", "event_id", env.EventID, "kind", kind, "trace_id", env.TraceID,
					"to", domain.MaskEmail(env.Data.Vehicle.OwnerEmail))
				select {
				case c.Nudge <- struct{}{}:
				default:
				}
			} else {
				c.Log.Info("duplicate event skipped", "event_id", env.EventID) // FR-30
			}
			return
		}
		lastErr = err
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23503" { // ticket does not exist: retrying will not help
			break
		}
		if ctx.Err() != nil {
			return
		}
		time.Sleep(time.Duration(attempt) * time.Second)
	}
	c.toDLQ(ctx, r, fmt.Sprintf("store: %v", lastErr))
}

func (c *Consumer) toDLQ(ctx context.Context, r *kgo.Record, reason string) {
	c.Log.Error("sending record to DLQ", "reason", reason, "offset", r.Offset, "partition", r.Partition)
	rec := &kgo.Record{
		Topic: c.DLQTopic, Key: r.Key, Value: r.Value,
		Headers: append(append([]kgo.RecordHeader{}, r.Headers...),
			kgo.RecordHeader{Key: "error", Value: []byte(reason)},
			kgo.RecordHeader{Key: "source_partition", Value: []byte(fmt.Sprint(r.Partition))},
			kgo.RecordHeader{Key: "source_offset", Value: []byte(fmt.Sprint(r.Offset))}),
	}
	pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := c.Producer.ProduceSync(pctx, rec).FirstErr(); err != nil {
		c.Log.Error("DLQ publish failed", "err", err)
	}
}

// WatchLag exports consumer-group lag for alerts (NFR-12, NFR-13).
func WatchLag(ctx context.Context, cl *kgo.Client, group string, log *slog.Logger) {
	adm := kadm.NewClient(cl)
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		lctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		lags, err := adm.Lag(lctx, group)
		cancel()
		if err != nil {
			continue
		}
		if l, ok := lags[group]; ok && l.Error() == nil {
			metrics.ConsumerLag.Set(float64(l.Lag.Total()))
		}
	}
}
