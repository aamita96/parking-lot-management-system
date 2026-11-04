// Command notifier consumes parking.ticket-events.v1 and sends entry and exit emails via Resend (SRS 8.4, 9).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"plms/internal/config"
	"plms/internal/email"
	"plms/internal/kafkax"
	"plms/internal/notifier"
	"plms/internal/platform/db"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "plms-notifier")
	slog.SetDefault(log)
	if err := run(log); err != nil {
		log.Error("notifier stopped", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// The API owns migrations; wait for them so the tables exist.
	if err := db.Migrate(cfg.DatabaseURL); err != nil {
		return err
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	producer, err := kafkax.NewProducer(cfg.KafkaBrokers)
	if err != nil {
		return err
	}
	defer producer.Close()
	kafkax.EnsureTopics(ctx, producer, log,
		kafkax.TopicSpec{Name: cfg.KafkaTopic, Partitions: cfg.KafkaPartitions, Replication: cfg.KafkaReplicationFactor,
			RetentionMs: kafkax.Retention(7 * 24 * time.Hour)},
		kafkax.TopicSpec{Name: cfg.KafkaDLQTopic, Partitions: 1, Replication: cfg.KafkaReplicationFactor,
			RetentionMs: kafkax.Retention(14 * 24 * time.Hour)})

	group, err := notifier.NewGroupClient(cfg.KafkaBrokers, cfg.KafkaGroup, cfg.KafkaTopic)
	if err != nil {
		return err
	}
	defer group.Close()

	var sender email.Sender = email.NewDevSender()
	if cfg.EmailEnabled {
		sender = email.NewResend(cfg.ResendAPIKey, cfg.EmailFrom, cfg.EmailReplyTo)
	} else {
		log.Warn("EMAIL_ENABLED=false: emails are logged, not sent")
	}

	nudge := make(chan struct{}, 1)
	consumer := &notifier.Consumer{Pool: pool, Client: group, Producer: producer, DLQTopic: cfg.KafkaDLQTopic,
		Group: cfg.KafkaGroup, Log: log, Nudge: nudge}
	dispatcher := &notifier.Dispatcher{Pool: pool, Sender: sender, Producer: producer, DLQTopic: cfg.KafkaDLQTopic,
		Log: log, Nudge: nudge}
	go consumer.Run(ctx)
	go dispatcher.Run(ctx)
	go notifier.WatchLag(ctx, producer, cfg.KafkaGroup, log)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"status":"ok"}`)) })
	mux.Handle("/metrics", promhttp.Handler())
	srv := &http.Server{Addr: cfg.NotifierHTTPAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Info("notifier running", "addr", cfg.NotifierHTTPAddr, "group", cfg.KafkaGroup, "email", cfg.EmailEnabled)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("metrics server failed", "err", err)
		}
	}()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdown)
}
