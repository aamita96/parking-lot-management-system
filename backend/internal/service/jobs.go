package service

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"plms/internal/domain"
	"plms/internal/platform/db"
	"plms/internal/platform/metrics"
	"plms/internal/store"
)

// RunJobs runs periodic housekeeping until ctx ends.
func (s *Service) RunJobs(ctx context.Context) {
	gauges := time.NewTicker(15 * time.Second)
	hourly := time.NewTicker(time.Hour)
	daily := time.NewTicker(24 * time.Hour)
	defer gauges.Stop()
	defer hourly.Stop()
	defer daily.Stop()
	s.updateGauges(ctx)
	s.hourly(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-gauges.C:
			s.updateGauges(ctx)
		case <-hourly.C:
			s.hourly(ctx)
		case <-daily.C:
			if n, err := store.PurgePublishedOutbox(ctx, s.Pool, 7*24*time.Hour); err != nil {
				s.logError(ctx, "outbox purge failed", err)
			} else if n > 0 {
				s.Log.Info("purged published outbox events", "count", n)
			}
		}
	}
}

func (s *Service) hourly(ctx context.Context) {
	if n, err := store.ExpirePendingVehicles(ctx, s.Pool); err != nil {
		s.logError(ctx, "expire registrations failed", err)
	} else if n > 0 {
		s.Log.Info("expired unverified registrations", "count", n)
	}
	if _, err := store.PurgeIdempotency(ctx, s.Pool); err != nil {
		s.logError(ctx, "idempotency purge failed", err)
	}
}

func (s *Service) updateGauges(ctx context.Context) {
	counts, err := store.CountSpotsByType(ctx, s.Pool)
	if err == nil {
		for _, st := range []domain.SpotType{domain.Compact, domain.Regular, domain.Oversized} {
			c := counts[st]
			inService := c.Total - c.OutOfService
			ratio := 0.0
			if inService > 0 {
				ratio = float64(c.Occupied) / float64(inService)
			}
			metrics.Occupancy.WithLabelValues(string(st)).Set(ratio)
		}
	}
	if lag, err := store.OutboxLag(ctx, s.Pool); err == nil {
		metrics.OutboxLag.Set(lag.Seconds())
	}
}

// demoVehicles use Resend's test inbox (delivered+label@resend.dev), so demo emails don't
// bounce or reach real people.
var demoVehicles = []struct {
	Number string
	Type   domain.VehicleType
}{
	{"KA05MN4321", domain.Motorcycle}, {"MH12XY4455", domain.Car}, {"DL3CAB7788", domain.Car},
	{"TN09BC2020", domain.Car}, {"KA03HG9001", domain.Truck}, {"22BH1234AB", domain.Car},
	{"GJ01KL3344", domain.Motorcycle}, {"HR26DQ5551", domain.Truck}, {"MH01AE0007", domain.Car},
	{"KL07CD8080", domain.Motorcycle}, {"RJ14CV6262", domain.Car}, {"UP32GH1001", domain.Car},
}

// SeedDemo adds verified demo owners and vehicles once (SEED_DEMO=true, local only).
func (s *Service) SeedDemo(ctx context.Context) error {
	if !s.Cfg.SeedDemo {
		return nil
	}
	return db.InTx(ctx, s.Pool, func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM vehicles`).Scan(&n); err != nil || n > 0 {
			return err
		}
		for _, d := range demoVehicles {
			o, err := store.UpsertOwner(ctx, tx, "delivered+"+lower(d.Number)+"@resend.dev", nil)
			if err != nil {
				return err
			}
			if err := store.MarkOwnerVerified(ctx, tx, o.ID); err != nil {
				return err
			}
			if _, err := store.InsertVehicle(ctx, tx, d.Number, d.Type, o.ID, nil, domain.VehicleActive); err != nil {
				return err
			}
		}
		s.Log.Info("seeded demo vehicles", "count", len(demoVehicles))
		return nil
	})
}

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}
