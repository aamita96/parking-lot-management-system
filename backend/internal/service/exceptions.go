package service

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"plms/internal/apperr"
	"plms/internal/domain"
	"plms/internal/platform/db"
	"plms/internal/store"
)

const unpaidAfter = 15 * time.Minute

type Exceptions struct {
	RejectedScans []store.Scan        `json:"rejectedScans"`
	UnpaidTickets []store.Ticket      `json:"unpaidTickets"`
	Disputes      []store.AdminAction `json:"disputes"`
	Counts        map[string]int      `json:"counts"`
}

// Exceptions is the admin queue of rejected scans, stuck payments and disputes (FR-25, FR-27, FR-38).
func (s *Service) Exceptions(ctx context.Context) (Exceptions, error) {
	scans, err := store.OpenRejectedScans(ctx, s.Pool, 200)
	if err != nil {
		return Exceptions{}, err
	}
	unpaid, err := store.StalePaymentPending(ctx, s.Pool, unpaidAfter)
	if err != nil {
		return Exceptions{}, err
	}
	disputes, err := store.OpenDisputes(ctx, s.Pool)
	if err != nil {
		return Exceptions{}, err
	}
	return Exceptions{RejectedScans: scans, UnpaidTickets: unpaid, Disputes: disputes, Counts: map[string]int{
		"rejectedScans": len(scans), "unpaidTickets": len(unpaid), "disputes": len(disputes),
		"total": len(scans) + len(unpaid) + len(disputes)}}, nil
}

func requireReason(reason string) error {
	if len(reason) < 3 {
		var v apperr.Validation
		v.Add("reason", "Give a short reason (at least 3 characters).")
		return v.Err()
	}
	return nil
}

func (s *Service) ResolveScan(ctx context.Context, adminID string, scanID int64, reason string) error {
	if err := requireReason(reason); err != nil {
		return err
	}
	return db.InTx(ctx, s.Pool, func(tx pgx.Tx) error {
		sc, err := store.ScanByID(ctx, tx, scanID)
		if db.IsNoRows(err) {
			return apperr.NotFound("Scan")
		}
		if err != nil {
			return err
		}
		if err := store.ResolveScan(ctx, tx, scanID, adminID, reason); err != nil {
			return err
		}
		return s.audit(ctx, tx, adminID, "SCAN_DISMISSED", "SCAN", strconv.FormatInt(scanID, 10), reason,
			map[string]any{"plate": sc.PlateRaw, "rejectReason": sc.RejectReason, "gate": sc.GateName})
	})
}

type ManualCheckIn struct {
	VehicleNumber string `json:"vehicleNumber"`
	SpotID        string `json:"spotId"`
	Reason        string `json:"reason"`
	ScanID        *int64 `json:"scanId"`
}

// ManualCheckInTicket opens a ticket by hand, e.g. after an unreadable plate (FR-38).
func (s *Service) ManualCheckInTicket(ctx context.Context, adminID string, in ManualCheckIn) (store.Ticket, error) {
	if err := requireReason(in.Reason); err != nil {
		return store.Ticket{}, err
	}
	plate := domain.NormalisePlate(in.VehicleNumber)
	if !domain.ValidPlate(plate) {
		var v apperr.Validation
		v.Add("vehicleNumber", "Enter a valid vehicle number.")
		return store.Ticket{}, v.Err()
	}
	if in.SpotID != "" && !isUUID(in.SpotID) {
		return store.Ticket{}, apperr.NotFound("Spot")
	}
	var t store.Ticket
	err := db.InTx(ctx, s.Pool, func(tx pgx.Tx) error {
		v, err := store.VehicleByNumber(ctx, tx, plate)
		if db.IsNoRows(err) {
			return ErrNotRegistered.With("detail", "Register the vehicle first (the owner can use the QR code).")
		}
		if err != nil {
			return err
		}
		if v.Status != domain.VehicleActive {
			return ErrNotVerified
		}
		t, err = s.admitTx(ctx, tx, v, nil, false, domain.SourceAdmin, in.SpotID)
		if err != nil {
			return err
		}
		if in.ScanID != nil {
			if err := store.ResolveScan(ctx, tx, *in.ScanID, adminID, "Manual check-in: "+t.TicketNumber); err != nil {
				return err
			}
		}
		if err := store.ResolveScansForPlate(ctx, tx, plate, adminID, "Manual check-in: "+t.TicketNumber); err != nil {
			return err
		}
		return s.audit(ctx, tx, adminID, "MANUAL_CHECK_IN", "TICKET", t.ID, in.Reason, map[string]any{
			"vehicleNumber": plate, "spot": t.SpotCode, "ticketNumber": t.TicketNumber})
	})
	return t, err
}

type ManualExit struct {
	Mode   string `json:"mode"` // CHARGE | WAIVE
	Reason string `json:"reason"`
}

// ManualExitTicket closes a ticket with the fee charged at the office or waived (FR-38).
func (s *Service) ManualExitTicket(ctx context.Context, adminID, ticketID string, in ManualExit) (store.Ticket, error) {
	if err := requireReason(in.Reason); err != nil {
		return store.Ticket{}, err
	}
	if in.Mode != "CHARGE" && in.Mode != "WAIVE" {
		return store.Ticket{}, apperr.BadRequest("VALIDATION_ERROR", "Mode must be CHARGE or WAIVE.")
	}
	if !isUUID(ticketID) {
		return store.Ticket{}, apperr.NotFound("Ticket")
	}
	var out store.Ticket
	err := db.InTx(ctx, s.Pool, func(tx pgx.Tx) error {
		t, err := store.TicketByIDForUpdate(ctx, tx, ticketID)
		if db.IsNoRows(err) {
			return apperr.NotFound("Ticket")
		}
		if err != nil {
			return err
		}
		if !t.Open() {
			return ErrTicketClosed.With("receipt", receipt(t))
		}
		if t.Status == domain.TicketActive {
			exit := time.Now().UTC()
			if exit.Before(t.EntryTime) {
				exit = t.EntryTime
			}
			if err := store.LockFee(ctx, tx, t.ID, exit, nil, domain.CalculateFee(domain.DurationMinutes(t.EntryTime, exit), t.Rate),
				domain.TicketPaymentPending); err != nil {
				return err
			}
			if t, err = store.TicketByID(ctx, tx, t.ID); err != nil {
				return err
			}
		}
		fee := int64(0)
		if t.FeePaise != nil {
			fee = *t.FeePaise
		}
		paid, method := fee, domain.PayManual
		if in.Mode == "WAIVE" {
			paid, method = 0, domain.PayWaived
		} else if fee == 0 {
			method = domain.PayNoCharge
		}
		out, err = s.closeTx(ctx, tx, t, paid, method, domain.SourceAdmin)
		if err != nil {
			return err
		}
		return s.audit(ctx, tx, adminID, "MANUAL_EXIT", "TICKET", t.ID, in.Reason, map[string]any{
			"mode": in.Mode, "feePaise": fee, "paidPaise": paid, "ticketNumber": t.TicketNumber, "vehicleNumber": t.VehicleNumber})
	})
	return out, err
}

type Reassign struct {
	OwnerEmail string `json:"ownerEmail"`
	Reason     string `json:"reason"`
	DisputeID  string `json:"disputeId"`
}

// ReassignVehicle moves a vehicle to its rightful owner (FR-38). Open tickets keep the
// email snapshotted at entry.
func (s *Service) ReassignVehicle(ctx context.Context, adminID, vehicleID string, in Reassign) (store.Vehicle, error) {
	if err := requireReason(in.Reason); err != nil {
		return store.Vehicle{}, err
	}
	e := domain.NormaliseEmail(in.OwnerEmail)
	if !domain.ValidEmail(e) {
		var v apperr.Validation
		v.Add("ownerEmail", "Enter a valid email address.")
		return store.Vehicle{}, v.Err()
	}
	if !isUUID(vehicleID) {
		return store.Vehicle{}, apperr.NotFound("Vehicle")
	}
	var out store.Vehicle
	err := db.InTx(ctx, s.Pool, func(tx pgx.Tx) error {
		v, err := store.VehicleByID(ctx, tx, vehicleID)
		if db.IsNoRows(err) || (err == nil && v.Status == domain.VehicleRemoved) {
			return apperr.NotFound("Vehicle")
		}
		if err != nil {
			return err
		}
		owner, err := store.UpsertOwner(ctx, tx, e, nil)
		if err != nil {
			return err
		}
		if err := store.SetVehicleOwner(ctx, tx, v.ID, owner.ID); err != nil {
			return err
		}
		// An admin decision stands in for the email code; the new owner verifies by signing in.
		if v.Status == domain.VehiclePending {
			if err := store.SetVehicleStatus(ctx, tx, v.ID, domain.VehicleActive); err != nil {
				return err
			}
		}
		if in.DisputeID != "" && isUUID(in.DisputeID) {
			if err := store.ResolveAction(ctx, tx, in.DisputeID, adminID, "Reassigned to "+e); err != nil {
				return err
			}
		}
		if err := s.audit(ctx, tx, adminID, "VEHICLE_REASSIGNED", "VEHICLE", v.ID, in.Reason, map[string]any{
			"vehicleNumber": v.VehicleNumber, "fromOwner": v.OwnerEmail, "toOwner": e, "disputeId": in.DisputeID}); err != nil {
			return err
		}
		out, err = store.VehicleByID(ctx, tx, v.ID)
		return err
	})
	return out, err
}

// ResolveDispute closes a dispute without reassigning (e.g. the claim was not supported).
func (s *Service) ResolveDispute(ctx context.Context, adminID, disputeID, reason string) error {
	if err := requireReason(reason); err != nil {
		return err
	}
	if !isUUID(disputeID) {
		return apperr.NotFound("Dispute")
	}
	return db.InTx(ctx, s.Pool, func(tx pgx.Tx) error {
		d, err := store.ActionByID(ctx, tx, disputeID)
		if db.IsNoRows(err) || (err == nil && d.Action != "DISPUTE_OPENED") {
			return apperr.NotFound("Dispute")
		}
		if err != nil {
			return err
		}
		if err := store.ResolveAction(ctx, tx, disputeID, adminID, reason); err != nil {
			return err
		}
		return s.audit(ctx, tx, adminID, "DISPUTE_RESOLVED", "DISPUTE", disputeID, reason, d.Details)
	})
}

// ---- search & detail (FR-39, FR-32) ----

type TicketDetail struct {
	store.Ticket
	Notifications []store.EmailNotification `json:"notifications"`
	Events        []store.EventSummary      `json:"events"`
	Breakdown     string                    `json:"breakdown,omitempty"`
}

func (s *Service) SearchTickets(ctx context.Context, f store.TicketFilter) ([]store.Ticket, int, error) {
	return store.SearchTickets(ctx, s.Pool, f)
}

func (s *Service) TicketDetail(ctx context.Context, id string) (TicketDetail, error) {
	if !isUUID(id) {
		return TicketDetail{}, apperr.NotFound("Ticket")
	}
	t, err := store.TicketByID(ctx, s.Pool, id)
	if db.IsNoRows(err) {
		return TicketDetail{}, apperr.NotFound("Ticket")
	}
	if err != nil {
		return TicketDetail{}, err
	}
	n, err := store.NotificationsForTicket(ctx, s.Pool, id)
	if err != nil {
		return TicketDetail{}, err
	}
	ev, err := store.RecentEvents(ctx, s.Pool, 10, id)
	if err != nil {
		return TicketDetail{}, err
	}
	d := TicketDetail{Ticket: t, Notifications: n, Events: ev}
	if t.DurationMinutes != nil && t.FeePaise != nil {
		f := domain.CalculateFee(*t.DurationMinutes, t.Rate)
		f.FeePaise = *t.FeePaise
		d.Breakdown = domain.FeeBreakdown(f, t.Rate)
	}
	return d, nil
}

// ResendNotification re-queues an entry or exit email (FR-32); the notifier sends it.
func (s *Service) ResendNotification(ctx context.Context, adminID, ticketID, kind string) (store.EmailNotification, error) {
	if kind != "ENTRY" && kind != "EXIT" {
		return store.EmailNotification{}, apperr.BadRequest("VALIDATION_ERROR", "Kind must be ENTRY or EXIT.")
	}
	if !isUUID(ticketID) {
		return store.EmailNotification{}, apperr.NotFound("Ticket")
	}
	var out store.EmailNotification
	err := db.InTx(ctx, s.Pool, func(tx pgx.Tx) error {
		n, err := store.NotificationByTicketKind(ctx, tx, ticketID, kind)
		if db.IsNoRows(err) {
			return apperr.NotFound("Email").With("hint", "The event has not reached the notification service yet.")
		}
		if err != nil {
			return err
		}
		if n.Status == "QUEUED" {
			return apperr.Conflict("ALREADY_QUEUED", "This email is already waiting to be sent.")
		}
		if err := store.RequeueNotification(ctx, tx, n.ID); err != nil {
			return err
		}
		if err := s.audit(ctx, tx, adminID, "EMAIL_RESENT", "TICKET", ticketID, "", map[string]any{"kind": kind}); err != nil {
			return err
		}
		out, err = store.NotificationByTicketKind(ctx, tx, ticketID, kind)
		return err
	})
	return out, err
}

func (s *Service) SearchVehicles(ctx context.Context, f store.VehicleFilter) ([]store.Vehicle, int, error) {
	return store.SearchVehicles(ctx, s.Pool, f)
}

func (s *Service) SearchOwners(ctx context.Context, q string, limit, offset int) ([]store.OwnerSummary, int, error) {
	return store.SearchOwners(ctx, s.Pool, q, limit, offset)
}

func (s *Service) Spots(ctx context.Context) ([]store.Spot, error) {
	return store.ListSpots(ctx, s.Pool)
}
