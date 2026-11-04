// Command api runs the PLMS REST API and the outbox relay (SRS 3, 8.3).
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

	"plms/internal/api"
	"plms/internal/auth"
	"plms/internal/config"
	"plms/internal/email"
	"plms/internal/kafkax"
	"plms/internal/notifier"
	"plms/internal/outbox"
	"plms/internal/platform/db"
	"plms/internal/service"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)
	if err := run(log); err != nil {
		log.Error("api stopped", "err", err)
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

	if err := db.Migrate(cfg.DatabaseURL); err != nil {
		return err
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	var mailer email.Sender = email.NewDevSender()
	if cfg.EmailEnabled {
		mailer = email.NewResend(cfg.ResendAPIKey, cfg.EmailFrom, cfg.EmailReplyTo)
	}
	svc := service.New(pool, cfg, auth.NewTokens(cfg.JWTSecret), mailer, log)
	if err := svc.EnsureBootstrapAdmin(ctx); err != nil {
		return err
	}
	if err := svc.SeedDemo(ctx); err != nil {
		return err
	}

	go svc.RunJobs(ctx)

	// Kafka is downstream of the gate (NFR-4): if it is not set up correctly yet, the API still
	// runs and events simply wait in the outbox table until Kafka works and the API restarts.
	producer, err := kafkax.NewProducer(cfg)
	if err != nil {
		log.Error("Kafka settings are invalid; ticket events will wait in the outbox", "err", err)
	} else {
		defer producer.Close()
		go kafkax.EnsureTopics(ctx, producer, log, kafkax.Topics(cfg)...)
		relay := &outbox.Relay{Pool: pool, Client: producer, Topic: cfg.KafkaTopic, Log: log}
		go relay.Run(ctx)

		// One-service deployments (e.g. Render free plan): send the emails from this process too.
		if cfg.RunNotifier {
			stopNotifier, err := notifier.Start(ctx, cfg, pool, producer, log)
			if err != nil {
				log.Error("could not start the email sender", "err", err)
			} else {
				defer stopNotifier()
			}
		}
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.New(svc).Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Info("api listening", "addr", cfg.HTTPAddr, "simulator", cfg.SimulatorEnabled, "email", cfg.EmailEnabled)
		errCh <- srv.ListenAndServe()
	}()
	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdown)
}
