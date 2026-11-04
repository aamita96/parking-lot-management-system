package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"plms/internal/domain"
	"plms/internal/events"
	"plms/internal/platform/db"
	"plms/internal/platform/httpx"
	"plms/internal/platform/metrics"
	"plms/internal/store"
)

// admitTx allocates a spot and opens a ticket for a verified vehicle, and writes VehicleEntered
// to the outbox, all inside tx (FR-15, SRS 6.3). spotID forces a spot (admin manual check-in).
func (s *Service) admitTx(ctx context.Context, tx pgx.Tx, v store.Vehicle, gateID *string, simulated bool, source, spotID string) (store.Ticket, error) {
	if _, err := store.OpenTicketForVehicle(ctx, tx, v.ID, false); err == nil {
		return store.Ticket{}, ErrAlreadyParked
	} else if !db.IsNoRows(err) {
		return store.Ticket{}, err
	}
	rule, err := store.CurrentPricing(ctx, tx, v.VehicleType)
	if err != nil {
		return store.Ticket{}, err
	}

	var spot store.Spot
	if spotID != "" {
		spot, err = store.SpotByIDForUpdate(ctx, tx, spotID)
		if db.IsNoRows(err) {
			return store.Ticket{}, ErrNoSpot.With("detail", "That spot does not exist.")
		}
		if err != nil {
			return store.Ticket{}, err
		}
		if spot.Status != domain.SpotAvailable || !fits(v.VehicleType, spot.SpotType) {
			return store.Ticket{}, ErrNoSpot.With("detail", "That spot is not free or is too small for this vehicle.")
		}
	} else {
		overflow, err := store.OverflowEnabled(ctx, tx)
		if err != nil {
			return store.Ticket{}, err
		}
		found := false
		for _, st := range domain.AllowedSpots(v.VehicleType, overflow) {
			spot, err = store.LockFreeSpot(ctx, tx, st)
			if err == nil {
				found = true
				break
			}
			if !db.IsNoRows(err) {
				return store.Ticket{}, err
			}
		}
		if !found {
			return store.Ticket{}, ErrNoSpot.With("vehicleType", v.VehicleType)
		}
	}

	if err := store.SetSpotStatus(ctx, tx, spot.ID, domain.SpotOccupied); err != nil {
		return store.Ticket{}, err
	}
	number, err := store.NextTicketNumber(ctx, tx, time.Now())
	if err != nil {
		return store.Ticket{}, err
	}
	id, err := store.InsertTicket(ctx, tx, store.NewTicket{
		TicketNumber: number, VehicleID: v.ID, SpotID: spot.ID, PricingRuleID: rule.ID, EntryGateID: gateID,
		VehicleType: v.VehicleType, SpotType: spot.SpotType, OwnerEmail: v.OwnerEmail, IsSimulated: simulated, Source: source,
	})
	if err != nil {
		if db.IsUniqueViolation(err, "uq_open_ticket_vehicle") {
			return store.Ticket{}, ErrAlreadyParked
		}
		return store.Ticket{}, err
	}
	t, err := store.TicketByID(ctx, tx, id)
	if err != nil {
		return store.Ticket{}, err
	}
	if err := writeEvent(ctx, tx, domain.EventEntered, source, t); err != nil {
		return store.Ticket{}, err
	}
	return t, nil
}

// fits applies BR-1: home spot type or larger, never smaller.
func fits(v domain.VehicleType, st domain.SpotType) bool {
	for _, ok := range domain.AllowedSpots(v, true) {
		if ok == st {
			return true
		}
	}
	return false
}

// closeTx marks a ticket paid/closed, frees its spot and writes VehicleExited (FR-23).
func (s *Service) closeTx(ctx context.Context, tx pgx.Tx, t store.Ticket, paid int64, method, source string) (store.Ticket, error) {
	if err := store.CloseTicket(ctx, tx, t.ID, paid, method, domain.TicketClosed); err != nil {
		return t, err
	}
	if err := store.SetSpotStatus(ctx, tx, t.SpotID, domain.SpotAvailable); err != nil {
		return t, err
	}
	closed, err := store.TicketByID(ctx, tx, t.ID)
	if err != nil {
		return t, err
	}
	if err := writeEvent(ctx, tx, domain.EventExited, source, closed); err != nil {
		return t, err
	}
	metrics.Payments.WithLabelValues(method).Inc()
	if !closed.IsSimulated {
		metrics.PaymentsPaise.Add(float64(paid))
	}
	return closed, nil
}

// lockFeeTx records the exit time and locks the fee (FR-21). Fee 0 closes immediately (FR-24).
func (s *Service) lockFeeTx(ctx context.Context, tx pgx.Tx, t store.Ticket, exit time.Time, exitGate *string, source string) (store.Ticket, domain.Fee, error) {
	fee := domain.CalculateFee(domain.DurationMinutes(t.EntryTime, exit), t.Rate)
	if err := store.LockFee(ctx, tx, t.ID, exit, exitGate, fee, domain.TicketPaymentPending); err != nil {
		return t, fee, err
	}
	t, err := store.TicketByID(ctx, tx, t.ID)
	if err != nil {
		return t, fee, err
	}
	if fee.FeePaise == 0 {
		t, err = s.closeTx(ctx, tx, t, 0, domain.PayNoCharge, source)
	}
	return t, fee, err
}

func writeEvent(ctx context.Context, tx pgx.Tx, eventType, source string, t store.Ticket) error {
	env := buildEnvelope(ctx, eventType, source, t)
	payload, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return store.InsertOutbox(ctx, tx, env.EventID, eventType, t.ID, payload)
}

func buildEnvelope(ctx context.Context, eventType, source string, t store.Ticket) events.Envelope {
	trace := httpx.TraceID(ctx)
	if trace == "" {
		trace = httpx.NewTraceID()
	}
	d := events.TicketData{
		TicketID:     t.ID,
		TicketNumber: t.TicketNumber,
		Vehicle: events.VehicleRef{ID: t.VehicleID, VehicleNumber: t.VehicleNumber, VehicleType: t.VehicleType,
			OwnerEmail: t.OwnerEmail},
		Spot:      events.SpotRef{ID: t.SpotID, Code: t.SpotCode, SpotType: t.SpotType, Floor: t.SpotFloor},
		EntryTime: t.EntryTime,
		Rate:      t.Rate,
	}
	if eventType == domain.EventExited {
		d.ExitTime = t.ExitTime
		d.DurationMinutes = t.DurationMinutes
		d.BillableHours = t.BillableHours
		d.FeePaise = t.FeePaise
		d.Currency = "INR"
		d.PaidPaise = t.PaidPaise
		if t.PaymentMethod != nil {
			d.PaymentMethod = *t.PaymentMethod
		}
		d.PaidAt = t.PaidAt
	}
	return events.Envelope{
		EventID:      uuid.NewString(),
		EventType:    eventType,
		EventVersion: events.Version,
		OccurredAt:   time.Now().UTC(),
		Producer:     events.Producer,
		Source:       source,
		Simulated:    t.IsSimulated,
		TraceID:      trace,
		Data:         d,
	}
}

func sourceFor(g store.GateDevice) string {
	if g.IsSimulator {
		return domain.SourceSim
	}
	return domain.SourceGate
}
