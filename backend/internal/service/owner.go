package service

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"plms/internal/apperr"
	"plms/internal/domain"
	"plms/internal/email"
	"plms/internal/platform/db"
	"plms/internal/store"
)

type MyVehicle struct {
	store.Vehicle
	ActiveTicket *store.Ticket `json:"activeTicket"`
}

func (s *Service) Me(ctx context.Context, ownerID string) (OwnerView, error) {
	o, err := store.OwnerByID(ctx, s.Pool, ownerID)
	if db.IsNoRows(err) {
		return OwnerView{}, apperr.ErrUnauthenticated
	}
	return ownerView(o), err
}

// MyVehicles lists the owner's vehicles with any open ticket (FR-7).
func (s *Service) MyVehicles(ctx context.Context, ownerID string) ([]MyVehicle, error) {
	vs, err := store.VehiclesByOwner(ctx, s.Pool, ownerID)
	if err != nil {
		return nil, err
	}
	out := make([]MyVehicle, 0, len(vs))
	for _, v := range vs {
		mv := MyVehicle{Vehicle: v}
		if t, err := store.OpenTicketForVehicle(ctx, s.Pool, v.ID, false); err == nil {
			mv.ActiveTicket = &t
		} else if !db.IsNoRows(err) {
			return nil, err
		}
		out = append(out, mv)
	}
	return out, nil
}

// AddMyVehicle registers another vehicle for a signed-in (verified) owner without a new code (FR-6).
func (s *Service) AddMyVehicle(ctx context.Context, ownerID string, in VehicleInput) (MyVehicle, bool, error) {
	c, err := validateVehicle(in, false)
	if err != nil {
		return MyVehicle{}, false, err
	}
	var out MyVehicle
	created := false
	err = db.InTx(ctx, s.Pool, func(tx pgx.Tx) error {
		owner, err := store.OwnerByID(ctx, tx, ownerID)
		if err != nil {
			return apperr.ErrUnauthenticated
		}
		if owner.EmailVerifiedAt == nil {
			return ErrNotVerified
		}
		existing, err := store.VehicleByNumber(ctx, tx, c.Number)
		switch {
		case err == nil && existing.OwnerID == ownerID:
			if existing.Status == domain.VehiclePending {
				if err := store.UpdatePendingVehicle(ctx, tx, existing.ID, c.Type, c.RegID); err != nil {
					return err
				}
				if err := store.SetVehicleStatus(ctx, tx, existing.ID, domain.VehicleActive); err != nil {
					return err
				}
				created = true
			}
			out.Vehicle, err = store.VehicleByID(ctx, tx, existing.ID)
			return err
		case err == nil && existing.Status == domain.VehiclePending && time.Since(existing.CreatedAt) > 24*time.Hour:
			if err := store.SetVehicleStatus(ctx, tx, existing.ID, domain.VehicleRemoved); err != nil {
				return err
			}
		case err == nil:
			return ErrVehicleTaken.With("canDispute", true).With("vehicleNumber", c.Number)
		case !db.IsNoRows(err):
			return err
		}
		id, err := store.InsertVehicle(ctx, tx, c.Number, c.Type, ownerID, c.RegID, domain.VehicleActive)
		if err != nil {
			if db.IsUniqueViolation(err, "uq_vehicle_number") {
				return ErrVehicleTaken.With("canDispute", true)
			}
			return err
		}
		created = true
		out.Vehicle, err = store.VehicleByID(ctx, tx, id)
		return err
	})
	return out, created, err
}

type VehiclePatch struct {
	RegistrationID *string `json:"registrationId"`
	Mobile         *string `json:"mobile"`
}

// UpdateMyVehicle edits the registration ID and the owner's mobile (FR-7). An empty string clears a field.
func (s *Service) UpdateMyVehicle(ctx context.Context, ownerID, vehicleID string, p VehiclePatch) (MyVehicle, error) {
	var v apperr.Validation
	var regID, mobile *string
	if p.RegistrationID != nil && *p.RegistrationID != "" {
		if !domain.ValidRegistrationID(*p.RegistrationID) {
			v.Add("registrationId", "Use up to 50 letters, digits and hyphens.")
		}
		regID = p.RegistrationID
	}
	if p.Mobile != nil && *p.Mobile != "" {
		m, ok := domain.NormaliseMobile(*p.Mobile)
		if !ok {
			v.Add("mobile", "Enter a 10-digit Indian mobile number, optionally starting with +91.")
		}
		mobile = &m
	}
	if err := v.Err(); err != nil {
		return MyVehicle{}, err
	}
	var out MyVehicle
	err := db.InTx(ctx, s.Pool, func(tx pgx.Tx) error {
		veh, err := s.ownVehicle(ctx, tx, ownerID, vehicleID)
		if err != nil {
			return err
		}
		if p.RegistrationID != nil {
			if err := store.SetVehicleRegistrationID(ctx, tx, veh.ID, regID); err != nil {
				return err
			}
		}
		if p.Mobile != nil {
			if err := store.SetOwnerMobile(ctx, tx, ownerID, mobile); err != nil {
				return err
			}
		}
		out.Vehicle, err = store.VehicleByID(ctx, tx, veh.ID)
		return err
	})
	return out, err
}

func (s *Service) ownVehicle(ctx context.Context, q db.DBTX, ownerID, vehicleID string) (store.Vehicle, error) {
	if !isUUID(vehicleID) {
		return store.Vehicle{}, apperr.NotFound("Vehicle")
	}
	v, err := store.VehicleByID(ctx, q, vehicleID)
	if db.IsNoRows(err) || (err == nil && (v.OwnerID != ownerID || v.Status == domain.VehicleRemoved)) {
		return store.Vehicle{}, apperr.NotFound("Vehicle")
	}
	return v, err
}

// RemoveMyVehicle removes a vehicle that is not parked (FR-8).
func (s *Service) RemoveMyVehicle(ctx context.Context, ownerID, vehicleID string) error {
	return db.InTx(ctx, s.Pool, func(tx pgx.Tx) error {
		v, err := s.ownVehicle(ctx, tx, ownerID, vehicleID)
		if err != nil {
			return err
		}
		if _, err := store.OpenTicketForVehicle(ctx, tx, v.ID, true); err == nil {
			return ErrVehicleParked
		} else if !db.IsNoRows(err) {
			return err
		}
		return store.SetVehicleStatus(ctx, tx, v.ID, domain.VehicleRemoved)
	})
}

// MyTickets lists active and past tickets for the owner's vehicles.
func (s *Service) MyTickets(ctx context.Context, ownerID string, limit, offset int) ([]store.Ticket, int, error) {
	return store.SearchTickets(ctx, s.Pool, store.TicketFilter{OwnerID: ownerID, Limit: limit, Offset: offset})
}

func (s *Service) UpdateMyMobile(ctx context.Context, ownerID string, raw *string) (OwnerView, error) {
	var mobile *string
	if raw != nil && *raw != "" {
		m, ok := domain.NormaliseMobile(*raw)
		if !ok {
			var v apperr.Validation
			v.Add("mobile", "Enter a 10-digit Indian mobile number, optionally starting with +91.")
			return OwnerView{}, v.Err()
		}
		mobile = &m
	}
	if err := store.SetOwnerMobile(ctx, s.Pool, ownerID, mobile); err != nil {
		return OwnerView{}, err
	}
	return s.Me(ctx, ownerID)
}

// StartEmailChange sends a code to the new address; the change applies once it is entered (FR-7).
func (s *Service) StartEmailChange(ctx context.Context, ownerID, rawEmail string) (SignInStart, error) {
	e := domain.NormaliseEmail(rawEmail)
	if !domain.ValidEmail(e) {
		var v apperr.Validation
		v.Add("email", "Enter a valid email address.")
		return SignInStart{}, v.Err()
	}
	res := SignInStart{CodeSentTo: domain.MaskEmail(e), ResendAfter: int(resendCooldown.Seconds())}
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		owner, err := store.OwnerByID(ctx, tx, ownerID)
		if err != nil {
			return apperr.ErrUnauthenticated
		}
		if owner.Email == e {
			return apperr.BadRequest("VALIDATION_ERROR", "That is already your email.")
		}
		if _, err := store.OwnerByEmail(ctx, tx, e); err == nil {
			return ErrEmailInUse
		} else if !db.IsNoRows(err) {
			return err
		}
		code, exp, err := s.issueCode(ctx, tx, ownerID, email.PurposeChangeEmail, e, &e, "", "")
		if err != nil {
			return err
		}
		res.CodeExpiresAt = &exp
		if s.Cfg.ExposeDevCodes() {
			res.DevCode = code
		}
		return nil
	})
	return res, err
}

func (s *Service) ConfirmEmailChange(ctx context.Context, ownerID, code string) (OwnerSession, error) {
	var out OwnerSession
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		row, err := s.checkCode(ctx, tx, ownerID, email.PurposeChangeEmail, code)
		if err != nil {
			return err
		}
		if row.TargetEmail == nil {
			return ErrCodeInvalid
		}
		if err := store.SetOwnerEmail(ctx, tx, ownerID, *row.TargetEmail); err != nil {
			if db.IsUniqueViolation(err, "") {
				return ErrEmailInUse
			}
			return err
		}
		owner, err := store.OwnerByID(ctx, tx, ownerID)
		if err != nil {
			return err
		}
		out, err = s.ownerSession(owner)
		return err
	})
	return out, err
}

// RefreshOwnerToken re-issues a 30-day owner token from a valid one (SRS 7.1 "refreshable").
func (s *Service) RefreshOwnerToken(ctx context.Context, ownerID string) (OwnerSession, error) {
	o, err := store.OwnerByID(ctx, s.Pool, ownerID)
	if err != nil {
		return OwnerSession{}, apperr.ErrUnauthenticated
	}
	return s.ownerSession(o)
}
