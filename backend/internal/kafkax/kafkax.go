// Package kafkax builds franz-go clients and makes sure the PLMS topics exist (SRS 8.1).
package kafkax

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"

	"plms/internal/config"
)

// ConnectionOptions holds the broker list plus TLS and login settings for managed Kafka.
// With no KAFKA_SASL_MECHANISM / KAFKA_TLS set (local Docker), it is a plain connection.
func ConnectionOptions(cfg config.Config) ([]kgo.Opt, error) {
	opts := []kgo.Opt{kgo.SeedBrokers(cfg.KafkaBrokers...)}

	if cfg.KafkaTLS {
		tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
		if cfg.KafkaCACert != "" {
			// Aiven signs its brokers with its own CA, so we must trust that CA explicitly.
			caPEM, err := readPEM(cfg.KafkaCACert)
			if err != nil {
				return nil, fmt.Errorf("KAFKA_CA_CERT: %w", err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(caPEM) {
				return nil, errors.New("KAFKA_CA_CERT is not a valid PEM certificate")
			}
			tlsConfig.RootCAs = pool
		}
		if cfg.KafkaClientCert != "" || cfg.KafkaClientKey != "" {
			// Log in with a client certificate (Aiven's default "Client certificate" method).
			certPEM, err := readPEM(cfg.KafkaClientCert)
			if err != nil {
				return nil, fmt.Errorf("KAFKA_CLIENT_CERT: %w", err)
			}
			keyPEM, err := readPEM(cfg.KafkaClientKey)
			if err != nil {
				return nil, fmt.Errorf("KAFKA_CLIENT_KEY: %w", err)
			}
			cert, err := tls.X509KeyPair(certPEM, keyPEM)
			if err != nil {
				return nil, fmt.Errorf("KAFKA_CLIENT_CERT / KAFKA_CLIENT_KEY: %w", err)
			}
			tlsConfig.Certificates = []tls.Certificate{cert}
		}
		opts = append(opts, kgo.DialTLSConfig(tlsConfig))
	}

	login := struct{ user, pass string }{cfg.KafkaUsername, cfg.KafkaPassword}
	switch cfg.KafkaSASLMechanism {
	case "":
	case "PLAIN":
		opts = append(opts, kgo.SASL(plain.Auth{User: login.user, Pass: login.pass}.AsMechanism()))
	case "SCRAM-SHA-256":
		opts = append(opts, kgo.SASL(scram.Auth{User: login.user, Pass: login.pass}.AsSha256Mechanism()))
	case "SCRAM-SHA-512":
		opts = append(opts, kgo.SASL(scram.Auth{User: login.user, Pass: login.pass}.AsSha512Mechanism()))
	default:
		return nil, errors.New("KAFKA_SASL_MECHANISM must be PLAIN, SCRAM-SHA-256 or SCRAM-SHA-512")
	}
	return opts, nil
}

// readPEM accepts either the PEM text itself or the path of a file that contains it
// (for example a Render "Secret File" at /etc/secrets/ca.pem).
func readPEM(value string) ([]byte, error) {
	if strings.HasPrefix(strings.TrimSpace(value), "-----BEGIN") {
		return []byte(value), nil
	}
	return os.ReadFile(value)
}

// NewProducer returns an idempotent producer with acks=all (SRS 8.1).
func NewProducer(cfg config.Config) (*kgo.Client, error) {
	opts, err := ConnectionOptions(cfg)
	if err != nil {
		return nil, err
	}
	return kgo.NewClient(append(opts,
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.ProducerBatchMaxBytes(1<<20),
		kgo.RecordDeliveryTimeout(10*time.Second),
		kgo.ProduceRequestTimeout(5*time.Second),
		kgo.RecordPartitioner(kgo.StickyKeyPartitioner(nil)), // key = ticket id keeps a ticket's events in order
	)...)
}

// Topics are the two PLMS topics (SRS 8.1): ticket events (7 days) and the dead-letter topic (14 days).
// KAFKA_PARTITIONS / KAFKA_REPLICATION_FACTOR adapt them to small hosted plans
// (a replication factor of -1 means "use the broker's default").
func Topics(cfg config.Config) []TopicSpec {
	return []TopicSpec{
		{Name: cfg.KafkaTopic, Partitions: cfg.KafkaPartitions, Replication: cfg.KafkaReplicationFactor, RetentionMs: Retention(7 * 24 * time.Hour)},
		{Name: cfg.KafkaDLQTopic, Partitions: 1, Replication: cfg.KafkaReplicationFactor, RetentionMs: Retention(14 * 24 * time.Hour)},
	}
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
