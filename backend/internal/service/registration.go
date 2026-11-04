package service

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"plms/internal/apperr"
	"plms/internal/auth"
	"plms/internal/domain"
	"plms/internal/email"
	"plms/internal/platform/db"
	"plms/internal/store"
)

const (
	codeTTL         = 10 * time.Minute
	codeMaxAttempts = 5
	codesPerHour    = 3
	resendCooldown  = 30 * time.Second
)

type VehicleInput struct {
	VehicleNumber  string  `json:"vehicleNumber"`
	VehicleType    string  `json:"vehicleType"`
	OwnerEmail     string  `json:"ownerEmail"`
	Mobile         *string `json:"mobile"`
	RegistrationID *string `json:"registrationId"`
}

type cleanVehicle struct {
	Number string
	Type   domain.VehicleType
	Email  string
	Mobile *string
	RegID  *string
}

// validateVehicle normalises and checks the registration form (FR-1 to FR-3, SRS 10.4).
func validateVehicle(in VehicleInput, needEmail bool) (cleanVehicle, error) {
	var v apperr.Validation
	out := cleanVehicle{Number: domain.NormalisePlate(in.VehicleNumber), Type: domain.VehicleType(in.VehicleType)}
	switch {
	case out.Number == "":
		v.Add("vehicleNumber", "Enter the vehicle number.")
	case !domain.ValidPlate(out.Number):
		v.Add("vehicleNumber", "Use a standard number like KA01AB1234 or a BH number like 22BH1234AB.")
	}
	if !out.Type.Valid() {
		v.Add("vehicleType", "Choose Motorcycle, Car or Truck.")
	}
	if needEmail {
		out.Email = domain.NormaliseEmail(in.OwnerEmail)
		if !domain.ValidEmail(out.Email) {
			v.Add("ownerEmail", "Enter a valid email address.")
		}
	}
	if in.Mobile != nil && *in.Mobile != "" {
		m, ok := domain.NormaliseMobile(*in.Mobile)
		if !ok {
			v.Add("mobile", "Enter a 10-digit Indian mobile number, optionally starting with +91.")
		} else {
			out.Mobile = &m
		}
	}
	if in.RegistrationID != nil && *in.RegistrationID != "" {
		if !domain.ValidRegistrationID(*in.RegistrationID) {
			v.Add("registrationId", "Use up to 50 letters, digits and hyphens.")
		} else {
			r := *in.RegistrationID
			out.RegID = &r
		}
	}
	return out, v.Err()
}

type VehicleSummary struct {
	ID            string             `json:"id"`
	VehicleNumber string             `json:"vehicleNumber"`
	VehicleType   domain.VehicleType `json:"vehicleType"`
	Status        string             `json:"status"`
}

type RegistrationResult struct {
	Status         string          `json:"status"` // CODE_SENT | ALREADY_REGISTERED
	RegistrationID string          `json:"registrationId,omitempty"`
	VehicleNumber  string          `json:"vehicleNumber"`
	CodeSentTo     string          `json:"codeSentTo,omitempty"`
	CodeExpiresAt  *time.Time      `json:"codeExpiresAt,omitempty"`
	ResendAfter    int             `json:"resendAfterSeconds,omitempty"`
	Vehicle        *VehicleSummary `json:"vehicle,omitempty"`
	DevCode        string          `json:"devCode,omitempty"`
}

// StartRegistration stores a pending vehicle and emails a 6-digit code (SRS 3.4, FR-1 to FR-5).
func (s *Service) StartRegistration(ctx context.Context, in VehicleInput) (RegistrationResult, error) {
	c, err := validateVehicle(in, true)
	if err != nil {
		return RegistrationResult{}, err
	}
	var res RegistrationResult
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		existing, err := store.VehicleByNumber(ctx, tx, c.Number)
		switch {
		case err == nil:
			if existing.OwnerEmail == c.Email {
				if existing.Status == domain.VehicleActive {
					res = RegistrationResult{Status: "ALREADY_REGISTERED", VehicleNumber: c.Number,
						Vehicle: &VehicleSummary{existing.ID, existing.VehicleNumber, existing.VehicleType, existing.Status}}
					return nil
				}
				// Same owner retrying an unverified registration: update and send a fresh code.
				if err := store.UpdatePendingVehicle(ctx, tx, existing.ID, c.Type, c.RegID); err != nil {
					return err
				}
				owner, err := store.UpsertOwner(ctx, tx, c.Email, c.Mobile)
				if err != nil {
					return err
				}
				return s.sendRegistrationCode(ctx, tx, owner, existing.ID, c, &res)
			}
			if existing.Status == domain.VehiclePending && time.Since(existing.CreatedAt) > 24*time.Hour {
				if err := store.SetVehicleStatus(ctx, tx, existing.ID, domain.VehicleRemoved); err != nil {
					return err
				}
			} else {
				return ErrVehicleTaken.With("canDispute", true).With("vehicleNumber", c.Number)
			}
		case !db.IsNoRows(err):
			return err
		}
		owner, err := store.UpsertOwner(ctx, tx, c.Email, c.Mobile)
		if err != nil {
			return err
		}
		vid, err := store.InsertVehicle(ctx, tx, c.Number, c.Type, owner.ID, c.RegID, domain.VehiclePending)
		if err != nil {
			if db.IsUniqueViolation(err, "uq_vehicle_number") {
				return ErrVehicleTaken.With("canDispute", true).With("vehicleNumber", c.Number)
			}
			return err
		}
		return s.sendRegistrationCode(ctx, tx, owner, vid, c, &res)
	})
	return res, err
}

func (s *Service) sendRegistrationCode(ctx context.Context, tx pgx.Tx, owner store.Owner, vehicleID string, c cleanVehicle, res *RegistrationResult) error {
	code, exp, err := s.issueCode(ctx, tx, owner.ID, email.PurposeVerify, owner.Email, nil, c.Number, c.Type)
	if err != nil {
		return err
	}
	*res = RegistrationResult{
		Status:         "CODE_SENT",
		RegistrationID: vehicleID,
		VehicleNumber:  c.Number,
		CodeSentTo:     domain.MaskEmail(owner.Email),
		CodeExpiresAt:  &exp,
		ResendAfter:    int(resendCooldown.Seconds()),
	}
	if s.Cfg.ExposeDevCodes() {
		res.DevCode = code
	}
	return nil
}

// issueCode enforces the per-address limits, stores the hashed code and emails it straight
// through the mail provider (FR-4, FR-31). Runs inside the caller's transaction so a failed
// send leaves no usable code behind.
func (s *Service) issueCode(ctx context.Context, tx pgx.Tx, ownerID, purpose, sendTo string, target *string,
	vehicleNumber string, vt domain.VehicleType) (string, time.Time, error) {
	n, err := store.CountCodesSince(ctx, tx, sendTo, time.Now().Add(-time.Hour))
	if err != nil {
		return "", time.Time{}, err
	}
	if n >= codesPerHour {
		return "", time.Time{}, ErrTooManyCodes
	}
	if last, err := store.LatestCode(ctx, tx, ownerID, purpose); err == nil && time.Since(last.CreatedAt) < resendCooldown {
		return "", time.Time{}, apperr.New(http.StatusTooManyRequests, "RATE_LIMITED", "Please wait 30 seconds before asking for another code.")
	}
	code := auth.NewCode()
	id := uuid.NewString()
	row, err := store.InsertEmailCode(ctx, tx, id, ownerID, purpose, auth.HashCode(s.Cfg.CodeSecret, id, code), sendTo, target, codeTTL)
	if err != nil {
		return "", time.Time{}, err
	}
	msg, err := email.CodeEmail(sendTo, purpose, code, int(codeTTL.Minutes()), vehicleNumber, vt)
	if err != nil {
		return "", time.Time{}, err
	}
	msg.Tags = map[string]string{"kind": "code"}
	if _, err := s.Mailer.Send(ctx, msg); err != nil {
		s.logError(ctx, "code email failed", err, "to", domain.MaskEmail(sendTo))
		if errors.Is(err, email.ErrQuota) || errors.Is(err, email.ErrRateLimit) {
			return "", time.Time{}, ErrEmailQuota
		}
		return "", time.Time{}, apperr.New(http.StatusServiceUnavailable, "EMAIL_UNAVAILABLE",
			"We couldn't send the email. Try again in a minute.")
	}
	return code, row.ExpiresAt, nil
}

// ResendRegistrationCode sends a new code for a pending registration.
func (s *Service) ResendRegistrationCode(ctx context.Context, registrationID string) (RegistrationResult, error) {
	if _, err := uuid.Parse(registrationID); err != nil {
		return RegistrationResult{}, apperr.NotFound("Registration")
	}
	var res RegistrationResult
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		v, err := store.VehicleByID(ctx, tx, registrationID)
		if db.IsNoRows(err) || (err == nil && v.Status == domain.VehicleRemoved) {
			return apperr.NotFound("Registration")
		}
		if err != nil {
			return err
		}
		if v.Status == domain.VehicleActive {
			return apperr.Conflict("ALREADY_VERIFIED", "This vehicle is already verified. Sign in to My vehicles.")
		}
		owner, err := store.OwnerByID(ctx, tx, v.OwnerID)
		if err != nil {
			return err
		}
		return s.sendRegistrationCode(ctx, tx, owner, v.ID, cleanVehicle{Number: v.VehicleNumber, Type: v.VehicleType}, &res)
	})
	return res, err
}

type OwnerSession struct {
	Owner       OwnerView       `json:"owner"`
	OwnerToken  string          `json:"ownerToken"`
	ExpiresAt   time.Time       `json:"expiresAt"`
	Vehicle     *VehicleSummary `json:"vehicle,omitempty"`
}

type OwnerView struct {
	ID       string  `json:"id"`
	Email    string  `json:"email"`
	Mobile   *string `json:"mobile"`
	Verified bool    `json:"verified"`
}

func ownerView(o store.Owner) OwnerView {
	return OwnerView{ID: o.ID, Email: o.Email, Mobile: o.Mobile, Verified: o.EmailVerifiedAt != nil}
}

// checkCode validates the latest code for owner+purpose. A wrong guess is counted even
// though the caller's work is rolled back, so it is committed in its own statement.
func (s *Service) checkCode(ctx context.Context, tx pgx.Tx, ownerID, purpose, code string) (store.EmailCode, error) {
	row, err := store.LatestCode(ctx, tx, ownerID, purpose)
	if db.IsNoRows(err) {
		return row, ErrCodeExpired
	}
	if err != nil {
		return row, err
	}
	if time.Now().After(row.ExpiresAt) {
		return row, ErrCodeExpired
	}
	if row.Attempts >= codeMaxAttempts {
		return row, apperr.New(http.StatusUnprocessableEntity, "CODE_EXPIRED", "Too many wrong attempts. Ask for a new code.")
	}
	if !auth.CodeMatches(s.Cfg.CodeSecret, row.ID, code, row.CodeHash) {
		if err := store.IncrementCodeAttempts(ctx, tx, row.ID); err != nil {
			return row, err
		}
		left := codeMaxAttempts - row.Attempts - 1
		return row, commitThen(ErrCodeInvalid.With("attemptsLeft", left))
	}
	return row, store.ConsumeCode(ctx, tx, row.ID)
}

// VerifyRegistration checks the code, activates the vehicle and signs the owner in (FR-4).
func (s *Service) VerifyRegistration(ctx context.Context, registrationID, code string) (OwnerSession, error) {
	if _, err := uuid.Parse(registrationID); err != nil {
		return OwnerSession{}, apperr.NotFound("Registration")
	}
	var out OwnerSession
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		v, err := store.VehicleByID(ctx, tx, registrationID)
		if db.IsNoRows(err) || (err == nil && v.Status == domain.VehicleRemoved) {
			return apperr.NotFound("Registration").With("hint", "It may have expired. Register again.")
		}
		if err != nil {
			return err
		}
		if _, err := s.checkCode(ctx, tx, v.OwnerID, email.PurposeVerify, code); err != nil {
			return err
		}
		if err := store.MarkOwnerVerified(ctx, tx, v.OwnerID); err != nil {
			return err
		}
		if v.Status == domain.VehiclePending {
			if err := store.SetVehicleStatus(ctx, tx, v.ID, domain.VehicleActive); err != nil {
				return err
			}
		}
		owner, err := store.OwnerByID(ctx, tx, v.OwnerID)
		if err != nil {
			return err
		}
		out, err = s.ownerSession(owner)
		out.Vehicle = &VehicleSummary{v.ID, v.VehicleNumber, v.VehicleType, domain.VehicleActive}
		return err
	})
	return out, err
}

func (s *Service) ownerSession(o store.Owner) (OwnerSession, error) {
	tok, exp, err := s.Tokens.Issue(o.ID, auth.RoleOwner, o.Email, s.Cfg.OwnerTokenTTL)
	if err != nil {
		return OwnerSession{}, err
	}
	return OwnerSession{Owner: ownerView(o), OwnerToken: tok, ExpiresAt: exp}, nil
}

type SignInStart struct {
	CodeSentTo    string     `json:"codeSentTo"`
	CodeExpiresAt *time.Time `json:"codeExpiresAt,omitempty"`
	ResendAfter   int        `json:"resendAfterSeconds"`
	DevCode       string     `json:"devCode,omitempty"`
}

// StartSignIn emails a sign-in code if the address belongs to an owner (FR-7). The response is
// the same either way, so it cannot be used to discover registered emails.
func (s *Service) StartSignIn(ctx context.Context, rawEmail string) (SignInStart, error) {
	e := domain.NormaliseEmail(rawEmail)
	if !domain.ValidEmail(e) {
		var v apperr.Validation
		v.Add("email", "Enter a valid email address.")
		return SignInStart{}, v.Err()
	}
	res := SignInStart{CodeSentTo: domain.MaskEmail(e), ResendAfter: int(resendCooldown.Seconds())}
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		owner, err := store.OwnerByEmail(ctx, tx, e)
		if db.IsNoRows(err) {
			return nil
		}
		if err != nil {
			return err
		}
		code, exp, err := s.issueCode(ctx, tx, owner.ID, email.PurposeSignIn, owner.Email, nil, "", "")
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

func (s *Service) VerifySignIn(ctx context.Context, rawEmail, code string) (OwnerSession, error) {
	e := domain.NormaliseEmail(rawEmail)
	var out OwnerSession
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		owner, err := store.OwnerByEmail(ctx, tx, e)
		if db.IsNoRows(err) {
			return ErrCodeInvalid
		}
		if err != nil {
			return err
		}
		if _, err := s.checkCode(ctx, tx, owner.ID, email.PurposeSignIn, code); err != nil {
			return err
		}
		if err := store.MarkOwnerVerified(ctx, tx, owner.ID); err != nil {
			return err
		}
		owner, err = store.OwnerByID(ctx, tx, owner.ID)
		if err != nil {
			return err
		}
		out, err = s.ownerSession(owner)
		return err
	})
	return out, err
}

type NumberCheck struct {
	VehicleNumber string `json:"vehicleNumber"`
	Valid         bool   `json:"valid"`
	Registered    bool   `json:"registered"`
}

// CheckNumber powers the debounced "already registered" warning on the form (SRS 10.3).
func (s *Service) CheckNumber(ctx context.Context, raw string) (NumberCheck, error) {
	n := domain.NormalisePlate(raw)
	out := NumberCheck{VehicleNumber: n, Valid: domain.ValidPlate(n)}
	if !out.Valid {
		return out, nil
	}
	v, err := store.VehicleByNumber(ctx, s.Pool, n)
	if err == nil {
		out.Registered = v.Status == domain.VehicleActive || time.Since(v.CreatedAt) < 24*time.Hour
		return out, nil
	}
	if db.IsNoRows(err) {
		return out, nil
	}
	return out, err
}

type DisputeInput struct {
	VehicleNumber  string  `json:"vehicleNumber"`
	ReporterEmail  string  `json:"reporterEmail"`
	Mobile         *string `json:"mobile"`
	RegistrationID *string `json:"registrationId"`
	Message        string  `json:"message"`
}

// OpenDispute is "Report a problem" for a number registered by someone else (FR-5).
func (s *Service) OpenDispute(ctx context.Context, in DisputeInput) (map[string]any, error) {
	var v apperr.Validation
	number := domain.NormalisePlate(in.VehicleNumber)
	if !domain.ValidPlate(number) {
		v.Add("vehicleNumber", "Enter a valid vehicle number.")
	}
	e := domain.NormaliseEmail(in.ReporterEmail)
	if !domain.ValidEmail(e) {
		v.Add("reporterEmail", "Enter a valid email address.")
	}
	if len(in.Message) > 1000 {
		v.Add("message", "Keep the message under 1000 characters.")
	}
	var mobile string
	if in.Mobile != nil && *in.Mobile != "" {
		m, ok := domain.NormaliseMobile(*in.Mobile)
		if !ok {
			v.Add("mobile", "Enter a 10-digit Indian mobile number.")
		}
		mobile = m
	}
	var regID string
	if in.RegistrationID != nil && *in.RegistrationID != "" {
		if !domain.ValidRegistrationID(*in.RegistrationID) {
			v.Add("registrationId", "Use up to 50 letters, digits and hyphens.")
		}
		regID = *in.RegistrationID
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	details := map[string]any{
		"vehicleNumber":  number,
		"reporterEmail":  e,
		"reporterMobile": mobile,
		"registrationId": regID,
		"message":        in.Message,
	}
	targetID := ""
	if veh, err := store.VehicleByNumber(ctx, s.Pool, number); err == nil {
		targetID = veh.ID
		details["currentOwnerEmail"] = veh.OwnerEmail
		if veh.RegistrationID != nil {
			details["currentRegistrationId"] = *veh.RegistrationID
		}
	}
	id, err := store.InsertAction(ctx, s.Pool, store.NewAction{
		Action: "DISPUTE_OPENED", TargetType: "VEHICLE", TargetID: targetID, Reason: in.Message, Details: details,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"disputeId": id, "status": "OPEN",
		"message": "Thanks. The car park office will check the registration and contact you by email."}, nil
}
