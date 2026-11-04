package service

import (
	"context"
	"errors"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"

	"plms/internal/apperr"
	"plms/internal/auth"
	"plms/internal/domain"
	"plms/internal/platform/db"
	"plms/internal/platform/metrics"
	"plms/internal/store"
)

const dedupeWindow = 30 * time.Second

// ---- gate device authentication (SRS 7.1) ----

// GateByKey authenticates a real device by its X-Gate-Key.
func (s *Service) GateByKey(ctx context.Context, key string) (store.GateDevice, error) {
	if key == "" {
		return store.GateDevice{}, apperr.ErrUnauthenticated
	}
	g, err := store.GateByKeyHash(ctx, s.Pool, auth.HashGateKey(key))
	if db.IsNoRows(err) || (err == nil && (!g.Active || g.IsSimulator)) {
		return store.GateDevice{}, apperr.New(401, "UNAUTHENTICATED", "Unknown or inactive gate key.")
	}
	if err != nil {
		return store.GateDevice{}, err
	}
	_ = store.TouchGate(ctx, s.Pool, g.ID)
	return g, nil
}

// SimulatorGate resolves X-Gate-Id for an admin acting as the simulator.
func (s *Service) SimulatorGate(ctx context.Context, name string) (store.GateDevice, error) {
	if !s.Cfg.SimulatorEnabled {
		return store.GateDevice{}, apperr.New(403, "FORBIDDEN", "The simulator is turned off on this server.")
	}
	g, err := store.GateByName(ctx, s.Pool, name)
	if db.IsNoRows(err) || (err == nil && (!g.IsSimulator || !g.Active)) {
		return store.GateDevice{}, apperr.New(403, "FORBIDDEN", "X-Gate-Id must name an active simulator device.")
	}
	if err != nil {
		return store.GateDevice{}, err
	}
	_ = store.TouchGate(ctx, s.Pool, g.ID)
	return g, nil
}

// ---- response shapes (SRS 7.4, 7.5) ----

type GateTicket struct {
	ID           string    `json:"id"`
	TicketNumber string    `json:"ticketNumber"`
	Status       string    `json:"status"`
	EntryTime    time.Time `json:"entryTime"`
	IsSimulated  bool      `json:"isSimulated"`
}

type GateVehicle struct {
	VehicleNumber string             `json:"vehicleNumber"`
	VehicleType   domain.VehicleType `json:"vehicleType"`
}

type GateOwner struct {
	Email       string `json:"email,omitempty"` // simulator and admin callers only (FR-20)
	EmailMasked string `json:"emailMasked"`
}

type GateSpot struct {
	Code     string          `json:"code"`
	SpotType domain.SpotType `json:"spotType"`
	RowLabel string          `json:"rowLabel"`
	Position int             `json:"position"`
	Floor    int             `json:"floor"`
}

type EntryResponse struct {
	Result       string      `json:"result"`
	Ticket       GateTicket  `json:"ticket"`
	Vehicle      GateVehicle `json:"vehicle"`
	Owner        GateOwner   `json:"owner"`
	Spot         GateSpot    `json:"spot"`
	Rate         domain.Rate `json:"rate"`
	Deduplicated bool        `json:"deduplicated,omitempty"`
}

type ExitResponse struct {
	Result          string      `json:"result"`
	Ticket          GateTicket  `json:"ticket"`
	Vehicle         GateVehicle `json:"vehicle"`
	Owner           GateOwner   `json:"owner"`
	Spot            GateSpot    `json:"spot"`
	EntryTime       time.Time   `json:"entryTime"`
	ExitTime        time.Time   `json:"exitTime"`
	DurationMinutes int         `json:"durationMinutes"`
	BillableHours   int         `json:"billableHours"`
	FeePaise        int64       `json:"feePaise"`
	Currency        string      `json:"currency"`
	Breakdown       string      `json:"breakdown"`
	Fee             domain.Fee  `json:"fee"`
	Rate            domain.Rate `json:"rate"`
	OpenBarrier     bool        `json:"openBarrier"`
	PaidPaise       *int64      `json:"paidPaise,omitempty"`
	PaymentMethod   *string     `json:"paymentMethod,omitempty"`
	PaidAt          *time.Time  `json:"paidAt,omitempty"`
	Deduplicated    bool        `json:"deduplicated,omitempty"`
}

type Receipt struct {
	Result        string     `json:"result"`
	TicketID      string     `json:"ticketId"`
	TicketNumber  string     `json:"ticketNumber"`
	Status        string     `json:"status"`
	VehicleNumber string     `json:"vehicleNumber"`
	FeePaise      int64      `json:"feePaise"`
	PaidPaise     int64      `json:"paidPaise"`
	PaymentMethod string     `json:"paymentMethod"`
	PaidAt        *time.Time `json:"paidAt"`
	OpenBarrier   bool       `json:"openBarrier"`
}

func gateParts(t store.Ticket, g store.GateDevice) (GateTicket, GateVehicle, GateOwner, GateSpot) {
	owner := GateOwner{EmailMasked: domain.MaskEmail(t.OwnerEmail)}
	if g.IsSimulator {
		owner.Email = t.OwnerEmail
	}
	return GateTicket{ID: t.ID, TicketNumber: t.TicketNumber, Status: t.Status, EntryTime: t.EntryTime, IsSimulated: t.IsSimulated},
		GateVehicle{VehicleNumber: t.VehicleNumber, VehicleType: t.VehicleType},
		owner,
		GateSpot{Code: t.SpotCode, SpotType: t.SpotType, RowLabel: t.SpotRow, Position: t.SpotPosition, Floor: t.SpotFloor}
}

func entryResponse(t store.Ticket, g store.GateDevice) EntryResponse {
	tk, v, o, sp := gateParts(t, g)
	return EntryResponse{Result: domain.ScanAdmitted, Ticket: tk, Vehicle: v, Owner: o, Spot: sp, Rate: t.Rate}
}

func exitResponse(t store.Ticket, g store.GateDevice) ExitResponse {
	tk, v, o, sp := gateParts(t, g)
	r := ExitResponse{Ticket: tk, Vehicle: v, Owner: o, Spot: sp, EntryTime: t.EntryTime, Currency: "INR", Rate: t.Rate}
	if t.ExitTime != nil {
		r.ExitTime = *t.ExitTime
	}
	if t.DurationMinutes != nil {
		r.DurationMinutes = *t.DurationMinutes
	}
	r.Fee = domain.CalculateFee(r.DurationMinutes, t.Rate)
	if t.FeePaise != nil {
		r.FeePaise = *t.FeePaise
		r.Fee.FeePaise = *t.FeePaise
	}
	r.BillableHours = r.Fee.BillableHours
	r.Breakdown = domain.FeeBreakdown(r.Fee, t.Rate)
	if t.Status == domain.TicketClosed {
		r.Result = domain.ScanExitClosed
		r.OpenBarrier = true
		r.PaidPaise, r.PaymentMethod, r.PaidAt = t.PaidPaise, t.PaymentMethod, t.PaidAt
	} else {
		r.Result = domain.ScanExitPendingPayment
	}
	return r
}

func receipt(t store.Ticket) Receipt {
	r := Receipt{Result: domain.ScanExitClosed, TicketID: t.ID, TicketNumber: t.TicketNumber, Status: t.Status,
		VehicleNumber: t.VehicleNumber, PaidAt: t.PaidAt, OpenBarrier: t.Status == domain.TicketClosed}
	if t.FeePaise != nil {
		r.FeePaise = *t.FeePaise
	}
	if t.PaidPaise != nil {
		r.PaidPaise = *t.PaidPaise
	}
	if t.PaymentMethod != nil {
		r.PaymentMethod = *t.PaymentMethod
	}
	return r
}

// rejection decorates a reason error with the gate body fields (FR-17).
func (s *Service) rejection(base *apperr.Error, plate string) *apperr.Error {
	e := base.With("result", domain.ScanRejected).With("plate", plate)
	if base.Code == ErrNotRegistered.Code || base.Code == ErrNotVerified.Code {
		e = e.With("registerUrl", s.Cfg.PublicURL+"/register?plate="+url.QueryEscape(plate))
	}
	return e
}

func reasonError(code string) *apperr.Error {
	for _, e := range []*apperr.Error{ErrNotRegistered, ErrNotVerified, ErrAlreadyParked, ErrNoSpot, ErrUnreadablePlate, ErrNoActiveTicket} {
		if e.Code == code {
			return e
		}
	}
	return apperr.Conflict(code, "The vehicle was refused.")
}

func (s *Service) logScan(ctx context.Context, q db.DBTX, g store.GateDevice, raw, plate, result, reason string, ticketID *string, dedup bool) {
	sc := store.Scan{GateID: g.ID, PlateRaw: raw, Result: result, TicketID: ticketID, Deduplicated: dedup}
	if plate != "" {
		p := plate
		if len(p) > 15 {
			p = p[:15]
		}
		sc.PlateNormalised = &p
	}
	if reason != "" {
		sc.RejectReason = &reason
	}
	if _, err := store.InsertScan(ctx, q, sc); err != nil {
		s.logError(ctx, "scan log failed", err)
	}
	metrics.Scans.WithLabelValues(g.Kind, result, reason).Inc()
}

// ---- entry ----

// EntryScan handles a plate read at an IN gate (FR-14 to FR-19, SRS 6.3).
func (s *Service) EntryScan(ctx context.Context, g store.GateDevice, raw string) (EntryResponse, error) {
	if g.Kind != domain.GateEntry {
		return EntryResponse{}, apperr.New(403, "FORBIDDEN", "This device is not an entry gate.")
	}
	plate := domain.NormalisePlate(raw)
	if !domain.ValidPlate(plate) {
		s.logScan(ctx, s.Pool, g, raw, plate, domain.ScanRejected, ErrUnreadablePlate.Code, nil, false)
		return EntryResponse{}, s.rejection(ErrUnreadablePlate, plate)
	}

	// FR-19: a repeat read within 30 s returns the first result instead of acting again.
	if prev, err := store.RecentScan(ctx, s.Pool, g.ID, plate, time.Now().Add(-dedupeWindow)); err == nil {
		switch {
		case prev.Result == domain.ScanRejected && prev.RejectReason != nil:
			s.logScan(ctx, s.Pool, g, raw, plate, domain.ScanRejected, *prev.RejectReason, nil, true)
			return EntryResponse{}, s.rejection(reasonError(*prev.RejectReason), plate).With("deduplicated", true)
		case prev.Result == domain.ScanAdmitted && prev.TicketID != nil:
			if t, err := store.TicketByID(ctx, s.Pool, *prev.TicketID); err == nil && t.Open() {
				s.logScan(ctx, s.Pool, g, raw, plate, domain.ScanAdmitted, "", &t.ID, true)
				r := entryResponse(t, g)
				r.Deduplicated = true
				return r, nil
			}
		}
	} else if !db.IsNoRows(err) {
		return EntryResponse{}, err
	}

	var t store.Ticket
	err := db.InTx(ctx, s.Pool, func(tx pgx.Tx) error {
		v, err := store.VehicleByNumber(ctx, tx, plate)
		if db.IsNoRows(err) {
			return ErrNotRegistered
		}
		if err != nil {
			return err
		}
		if v.Status != domain.VehicleActive {
			return ErrNotVerified
		}
		gid := g.ID
		t, err = s.admitTx(ctx, tx, v, &gid, g.IsSimulator, sourceFor(g), "")
		if err != nil {
			return err
		}
		s.logScan(ctx, tx, g, raw, plate, domain.ScanAdmitted, "", &t.ID, false)
		return nil
	})
	if err != nil {
		var ae *apperr.Error
		if errors.As(err, &ae) {
			// The transaction rolled back; log the rejection on its own (SRS 6.3 step 7).
			s.logScan(ctx, s.Pool, g, raw, plate, domain.ScanRejected, ae.Code, nil, false)
			return EntryResponse{}, s.rejection(ae, plate)
		}
		return EntryResponse{}, err
	}
	return entryResponse(t, g), nil
}

// ---- exit ----

type ExitInput struct {
	Plate                string `json:"plate"`
	SimulatedStayMinutes *int   `json:"simulatedStayMinutes"`
}

// ExitScan handles a plate read at the PAY kiosk: records exit time and locks the fee (FR-21, FR-24, FR-25).
func (s *Service) ExitScan(ctx context.Context, g store.GateDevice, in ExitInput) (ExitResponse, error) {
	if g.Kind != domain.GateExit {
		return ExitResponse{}, apperr.New(403, "FORBIDDEN", "This device is not an exit gate.")
	}
	if in.SimulatedStayMinutes != nil && !g.IsSimulator {
		return ExitResponse{}, apperr.BadRequest("VALIDATION_ERROR", "simulatedStayMinutes is accepted only from the simulator.")
	}
	if in.SimulatedStayMinutes != nil && (*in.SimulatedStayMinutes < 0 || *in.SimulatedStayMinutes > 60*24*60) {
		return ExitResponse{}, apperr.BadRequest("VALIDATION_ERROR", "simulatedStayMinutes must be between 0 and 86400.")
	}
	raw := in.Plate
	plate := domain.NormalisePlate(raw)
	if !domain.ValidPlate(plate) {
		s.logScan(ctx, s.Pool, g, raw, plate, domain.ScanRejected, ErrUnreadablePlate.Code, nil, false)
		return ExitResponse{}, s.rejection(ErrUnreadablePlate, plate)
	}

	if prev, err := store.RecentScan(ctx, s.Pool, g.ID, plate, time.Now().Add(-dedupeWindow)); err == nil {
		switch {
		case prev.Result == domain.ScanRejected && prev.RejectReason != nil:
			s.logScan(ctx, s.Pool, g, raw, plate, domain.ScanRejected, *prev.RejectReason, nil, true)
			return ExitResponse{}, s.rejection(reasonError(*prev.RejectReason), plate).With("deduplicated", true)
		case prev.TicketID != nil:
			t, err := store.TicketByID(ctx, s.Pool, *prev.TicketID)
			if err == nil {
				// Only replay if the vehicle has not started a newer visit since.
				newer, nerr := store.OpenTicketForVehicle(ctx, s.Pool, t.VehicleID, false)
				if db.IsNoRows(nerr) || (nerr == nil && newer.ID == t.ID) {
					s.logScan(ctx, s.Pool, g, raw, plate, prev.Result, "", &t.ID, true)
					r := exitResponse(t, g)
					r.Deduplicated = true
					return r, nil
				}
			}
		}
	} else if !db.IsNoRows(err) {
		return ExitResponse{}, err
	}

	var t store.Ticket
	err := db.InTx(ctx, s.Pool, func(tx pgx.Tx) error {
		v, err := store.VehicleByNumber(ctx, tx, plate)
		if db.IsNoRows(err) {
			return ErrNoActiveTicket
		}
		if err != nil {
			return err
		}
		t, err = store.OpenTicketForVehicle(ctx, tx, v.ID, true)
		if db.IsNoRows(err) {
			return ErrNoActiveTicket
		}
		if err != nil {
			return err
		}
		if t.Status == domain.TicketActive {
			exit := time.Now().UTC()
			if in.SimulatedStayMinutes != nil && t.IsSimulated {
				exit = t.EntryTime.Add(time.Duration(*in.SimulatedStayMinutes) * time.Minute)
			}
			if exit.Before(t.EntryTime) {
				exit = t.EntryTime
			}
			gid := g.ID
			t, _, err = s.lockFeeTx(ctx, tx, t, exit, &gid, sourceFor(g))
			if err != nil {
				return err
			}
		}
		result := domain.ScanExitPendingPayment
		if t.Status == domain.TicketClosed {
			result = domain.ScanExitClosed
		}
		s.logScan(ctx, tx, g, raw, plate, result, "", &t.ID, false)
		return nil
	})
	if err != nil {
		var ae *apperr.Error
		if errors.As(err, &ae) {
			s.logScan(ctx, s.Pool, g, raw, plate, domain.ScanRejected, ae.Code, nil, false)
			return ExitResponse{}, s.rejection(ae, plate)
		}
		return ExitResponse{}, err
	}
	return exitResponse(t, g), nil
}

// ---- payment ----

type PaymentInput struct {
	TicketID    string `json:"ticketId"`
	AmountPaise int64  `json:"amountPaise"`
	Method      string `json:"method"`
}

// Pay records a simulated payment, closes the ticket and frees the spot (FR-23, FR-26).
func (s *Service) Pay(ctx context.Context, g store.GateDevice, in PaymentInput) (Receipt, error) {
	if g.Kind != domain.GateExit {
		return Receipt{}, apperr.New(403, "FORBIDDEN", "Payments are taken at the exit kiosk only.")
	}
	var v apperr.Validation
	if !isUUID(in.TicketID) {
		v.Add("ticketId", "ticketId is required.")
	}
	if in.Method != domain.PaySimUPI && in.Method != domain.PaySimCash {
		v.Add("method", "Use SIMULATED_UPI or SIMULATED_CASH.")
	}
	if in.AmountPaise < 0 {
		v.Add("amountPaise", "Amount cannot be negative.")
	}
	if err := v.Err(); err != nil {
		return Receipt{}, err
	}
	var out Receipt
	err := db.InTx(ctx, s.Pool, func(tx pgx.Tx) error {
		t, err := store.TicketByIDForUpdate(ctx, tx, in.TicketID)
		if db.IsNoRows(err) {
			return apperr.NotFound("Ticket")
		}
		if err != nil {
			return err
		}
		switch t.Status {
		case domain.TicketClosed, domain.TicketCancelled:
			return ErrTicketClosed.With("receipt", receipt(t))
		case domain.TicketActive:
			return ErrPaymentNotDue
		}
		fee := int64(0)
		if t.FeePaise != nil {
			fee = *t.FeePaise
		}
		if in.AmountPaise != fee {
			return ErrAmountMismatch.With("feePaise", fee)
		}
		closed, err := s.closeTx(ctx, tx, t, in.AmountPaise, in.Method, sourceFor(g))
		if err != nil {
			return err
		}
		out = receipt(closed)
		return nil
	})
	return out, err
}
