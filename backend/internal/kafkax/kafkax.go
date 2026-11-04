// Package kafkax builds franz-go clients and makes sure the PLMS topics exist (SRS 8.1).
package kafkax

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
)

// NewProducer returns an idempotent producer with acks=all (SRS 8.1).
func NewProducer(brokers []string) (*kgo.Client, error) {
	return kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.ProducerBatchMaxBytes(1<<20),
		kgo.RecordDeliveryTimeout(10*time.Second),
		kgo.ProduceRequestTimeout(5*time.Second),
		kgo.RecordPartitioner(kgo.StickyKeyPartitioner(nil)), // key = ticket id keeps a ticket's events in order
	)
}

type TopicSpec struct {
	Name        string
	Partitions  int32
	Replication int16
	RetentionMs string
}

// EnsureTopics creates missing topics, retrying until Kafka answers or ctx ends.
// It never blocks start-up for long: the outbox keeps events until Kafka is back (NFR-4).
func EnsureTopics(ctx context.Context, cl *kgo.Client, log *slog.Logger, specs ...TopicSpec) {
	adm := kadm.NewClient(cl)
	for attempt := 0; ; attempt++ {
		ok := true
		for _, sp := range specs {
			cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			resp, err := adm.CreateTopic(cctx, sp.Partitions, sp.Replication, map[string]*string{"retention.ms": &sp.RetentionMs}, sp.Name)
			cancel()
			if err == nil {
				err = resp.Err
			}
			if err != nil && !errors.Is(err, kerr.TopicAlreadyExists) {
				ok = false
				if attempt%10 == 0 {
					log.Warn("kafka topic not ready yet", "topic", sp.Name, "err", err)
				}
			}
		}
		if ok {
			log.Info("kafka topics ready")
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func Retention(d time.Duration) string {
	return itoa(d.Milliseconds())
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
