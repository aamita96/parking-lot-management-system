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

	// The API owns migrations; running them here too is harmless and makes sure the tables exist.
	if err := db.Migrate(cfg.DatabaseURL); err != nil {
		return err
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	producer, err := kafkax.NewProducer(cfg) // used for the dead-letter topic
	if err != nil {
		return err
	}
	defer producer.Close()
	kafkax.EnsureTopics(ctx, producer, log, kafkax.Topics(cfg)...)

	stopNotifier, err := notifier.Start(ctx, cfg, pool, producer, log)
	if err != nil {
		return err
	}
	defer stopNotifier()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"status":"ok"}`)) })
	mux.Handle("/metrics", promhttp.Handler())
	srv := &http.Server{Addr: cfg.NotifierHTTPAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("metrics server failed", "err", err)
		}
	}()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdown)
}
