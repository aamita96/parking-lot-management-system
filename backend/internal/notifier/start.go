package notifier

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kgo"

	"plms/internal/config"
	"plms/internal/email"
)

// Start runs the email side of PLMS in the background until ctx ends:
//   - the Kafka consumer, which stores one email row per ticket event, and
//   - the dispatcher, which sends those emails through Resend (with retries).
//
// It is used by cmd/notifier, and by cmd/api when RUN_NOTIFIER=true (one-service deployments).
// The returned function closes the Kafka consumer; call it on shutdown.
func Start(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, producer *kgo.Client, log *slog.Logger) (stop func(), err error) {
	group, err := NewGroupClient(cfg)
	if err != nil {
		return nil, err
	}

	var sender email.Sender = email.NewDevSender()
	if cfg.EmailEnabled {
		sender = email.NewResend(cfg.ResendAPIKey, cfg.EmailFrom, cfg.EmailReplyTo)
	} else {
		log.Warn("EMAIL_ENABLED=false: emails are logged, not sent")
	}

	nudge := make(chan struct{}, 1) // wakes the dispatcher as soon as a new email is queued
	consumer := &Consumer{Pool: pool, Client: group, Producer: producer, DLQTopic: cfg.KafkaDLQTopic, Group: cfg.KafkaGroup, Log: log, Nudge: nudge}
	dispatcher := &Dispatcher{Pool: pool, Sender: sender, Producer: producer, DLQTopic: cfg.KafkaDLQTopic, Log: log, Nudge: nudge}

	go consumer.Run(ctx)
	go dispatcher.Run(ctx)
	go WatchLag(ctx, producer, cfg.KafkaGroup, log)
	log.Info("notifier running", "group", cfg.KafkaGroup, "email", cfg.EmailEnabled)
	return group.Close, nil
}
