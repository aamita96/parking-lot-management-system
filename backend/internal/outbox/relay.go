// Package outbox publishes outbox rows to Kafka (transactional outbox, SRS 8.3).
package outbox

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kgo"

	"plms/internal/platform/db"
	"plms/internal/platform/httpx"
	"plms/internal/platform/metrics"
	"plms/internal/store"
)

type Relay struct {
	Pool     *pgxpool.Pool
	Client   *kgo.Client
	Topic    string
	Log      *slog.Logger
	Interval time.Duration
	Batch    int
}

// Run polls every 500 ms, publishes a batch and marks it published; on failure it backs off
// exponentially up to 30 s. Events stay in Postgres until Kafka acknowledges them (NFR-5).
func (r *Relay) Run(ctx context.Context) {
	if r.Interval == 0 {
		r.Interval = 500 * time.Millisecond
	}
	if r.Batch == 0 {
		r.Batch = 100
	}
	backoff := time.Duration(0)
	for {
		wait := r.Interval + backoff
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		n, err := r.publishBatch(ctx)
		if err != nil {
			if backoff == 0 {
				backoff = time.Second
			} else if backoff < 30*time.Second {
				backoff *= 2
			}
			r.Log.Warn("outbox publish failed", "err", err, "retry_in", (r.Interval + backoff).String())
			continue
		}
		backoff = 0
		if n == r.Batch {
			// More waiting: go again straight away.
			backoff = -r.Interval
		}
	}
}

type envelopeTrace struct {
	TraceID string `json:"traceId"`
}

func (r *Relay) publishBatch(ctx context.Context) (int, error) {
	n := 0
	var publishErr error
	err := db.InTx(ctx, r.Pool, func(tx pgx.Tx) error {
		batch, err := store.LockUnpublished(ctx, tx, r.Batch)
		if err != nil || len(batch) == 0 {
			return err
		}
		records := make([]*kgo.Record, 0, len(batch))
		ids := make([]int64, 0, len(batch))
		for _, e := range batch {
			var et envelopeTrace
			_ = json.Unmarshal(e.Payload, &et)
			records = append(records, &kgo.Record{
				Topic: r.Topic,
				Key:   []byte(e.AggregateID),
				Value: e.Payload,
				Headers: []kgo.RecordHeader{
					{Key: "event_type", Value: []byte(e.EventType)},
					{Key: "event_id", Value: []byte(e.EventID)},
					{Key: "traceparent", Value: []byte(httpx.Traceparent(et.TraceID))},
				},
			})
			ids = append(ids, e.ID)
		}
		pctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		if err := r.Client.ProduceSync(pctx, records...).FirstErr(); err != nil {
			publishErr = err
			metrics.OutboxPublished.WithLabelValues("error").Add(float64(len(ids)))
			// Keep the attempt count even though the batch stays unpublished.
			return store.MarkPublishFailed(ctx, tx, ids, truncate(err.Error(), 500))
		}
		metrics.OutboxPublished.WithLabelValues("ok").Add(float64(len(ids)))
		n = len(ids)
		return store.MarkPublished(ctx, tx, ids)
	})
	if err != nil {
		return 0, err
	}
	return n, publishErr
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
