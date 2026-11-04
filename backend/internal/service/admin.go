package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"plms/internal/apperr"
	"plms/internal/auth"
	"plms/internal/domain"
	"plms/internal/platform/db"
	"plms/internal/store"
)

// ---- auth ----

type AdminView struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type AdminSession struct {
	Admin        AdminView `json:"admin"`
	AccessToken  string    `json:"accessToken"`
	ExpiresAt    time.Time `json:"expiresAt"`
	RefreshToken string    `json:"-"` // sent as an HttpOnly cookie (NFR-7)
	RefreshExp   time.Time `json:"-"`
}

// EnsureBootstrapAdmin creates the first admin from the environment when none exist.
func (s *Service) EnsureBootstrapAdmin(ctx context.Context) error {
	if s.Cfg.BootstrapAdmin == "" || s.Cfg.BootstrapPass == "" {
		return nil
	}
	n, err := store.CountAdmins(ctx, s.Pool)
	if err != nil || n > 0 {
		return err
	}
	h, err := auth.HashPassword(s.Cfg.BootstrapPass)
	if err != nil {
		return err
	}
	s.Log.Info("creating bootstrap admin", "email", domain.MaskEmail(s.Cfg.BootstrapAdmin))
	return store.InsertAdmin(ctx, s.Pool, domain.NormaliseEmail(s.Cfg.BootstrapAdmin), "Administrator", h)
}

var invalidLogin = apperr.New(401, "UNAUTHENTICATED", "Email or password is incorrect.")

func (s *Service) AdminLogin(ctx context.Context, rawEmail, password string) (AdminSession, error) {
	a, err := store.AdminByEmail(ctx, s.Pool, domain.NormaliseEmail(rawEmail))
	if db.IsNoRows(err) {
		auth.VerifyPassword(password, "$argon2id$v=19$m=65536,t=2,p=2$c2FsdHNhbHRzYWx0c2FsdA$aGFzaGhhc2hoYXNoaGFzaGhhc2hoYXNoaGFzaGhhc2g") // equalise timing
		return AdminSession{}, invalidLogin
	}
	if err != nil {
		return AdminSession{}, err
	}
	if !auth.VerifyPassword(password, a.PasswordHash) {
		return AdminSession{}, invalidLogin
	}
	_ = store.TouchAdminLogin(ctx, s.Pool, a.ID)
	return s.adminSession(a)
}

func (s *Service) adminSession(a store.AdminUser) (AdminSession, error) {
	access, exp, err := s.Tokens.Issue(a.ID, auth.RoleAdmin, a.Email, s.Cfg.AdminAccessTTL)
	if err != nil {
		return AdminSession{}, err
	}
	refresh, rexp, err := s.Tokens.Issue(a.ID, auth.RoleAdminRefresh, a.Email, s.Cfg.AdminRefreshTTL)
	if err != nil {
		return AdminSession{}, err
	}
	return AdminSession{Admin: AdminView{a.ID, a.Email, a.Name}, AccessToken: access, ExpiresAt: exp,
		RefreshToken: refresh, RefreshExp: rexp}, nil
}

func (s *Service) AdminRefresh(ctx context.Context, refreshToken string) (AdminSession, error) {
	c, err := s.Tokens.Parse(refreshToken, auth.RoleAdminRefresh)
	if err != nil {
		return AdminSession{}, apperr.ErrUnauthenticated
	}
	a, err := store.AdminByID(ctx, s.Pool, c.Subject)
	if err != nil {
		return AdminSession{}, apperr.ErrUnauthenticated
	}
	return s.adminSession(a)
}

func (s *Service) audit(ctx context.Context, q db.DBTX, adminID, action, targetType, targetID, reason string, details map[string]any) error {
	_, err := store.InsertAction(ctx, q, store.NewAction{AdminID: &adminID, Action: action, TargetType: targetType,
		TargetID: targetID, Reason: reason, Details: details})
	return err
}

// ---- spots (FR-33, FR-34) ----

type SpotInput struct {
	SpotType     string `json:"spotType"`
	Floor        int    `json:"floor"`
	RowLabel     string `json:"rowLabel"`
	Position     int    `json:"position"`     // single spot
	FromPosition int    `json:"fromPosition"` // bulk range
	ToPosition   int    `json:"toPosition"`
	Code         string `json:"code"` // optional custom code for a single spot
}

var rowRe = regexp.MustCompile(`^[A-Z]$`)

func spotCode(floor int, row string, pos int) string {
	if floor == 0 {
		return fmt.Sprintf("%s%d", row, pos)
	}
	return fmt.Sprintf("F%d-%s%d", floor, row, pos)
}

// CreateSpots creates one spot or a range of positions in a row.
func (s *Service) CreateSpots(ctx context.Context, adminID string, in SpotInput) ([]store.Spot, error) {
	var v apperr.Validation
	st := domain.SpotType(in.SpotType)
	if !st.Valid() {
		v.Add("spotType", "Choose COMPACT, REGULAR or OVERSIZED.")
	}
	row := strings.ToUpper(strings.TrimSpace(in.RowLabel))
	if !rowRe.MatchString(row) {
		v.Add("rowLabel", "Use a single letter A–Z.")
	}
	if in.Floor < 0 || in.Floor > 20 {
		v.Add("floor", "Floor must be between 0 and 20.")
	}
	from, to := in.FromPosition, in.ToPosition
	if in.Position > 0 {
		from, to = in.Position, in.Position
	}
	if from < 1 || to < from || to > 60 {
		v.Add("position", "Positions must be 1–60, with 'to' not before 'from'.")
	}
	if in.Code != "" && from != to {
		v.Add("code", "A custom code is only allowed for a single spot.")
	}
	if in.Code != "" && (len(in.Code) > 20 || !regexp.MustCompile(`^[A-Za-z0-9-]+$`).MatchString(in.Code)) {
		v.Add("code", "Use up to 20 letters, digits and hyphens.")
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	var out []store.Spot
	err := db.InTx(ctx, s.Pool, func(tx pgx.Tx) error {
		for pos := from; pos <= to; pos++ {
			code := spotCode(in.Floor, row, pos)
			if in.Code != "" {
				code = strings.ToUpper(in.Code)
			}
			sp, err := store.InsertSpot(ctx, tx, code, st, in.Floor, row, pos)
			if err != nil {
				if db.IsUniqueViolation(err, "") {
					return apperr.Conflict("SPOT_EXISTS", fmt.Sprintf("Spot %s (floor %d, row %s, position %d) already exists.", code, in.Floor, row, pos))
				}
				return err
			}
			out = append(out, sp)
		}
		return s.audit(ctx, tx, adminID, "SPOTS_CREATED", "SPOT", "", "", map[string]any{
			"count": len(out), "spotType": st, "floor": in.Floor, "row": row, "from": from, "to": to})
	})
	return out, err
}

type SpotPatch struct {
	Status   *string `json:"status"`
	SpotType *string `json:"spotType"`
	Reason   string  `json:"reason"`
}

// UpdateSpot toggles out-of-service or changes the type of a free spot (FR-34).
func (s *Service) UpdateSpot(ctx context.Context, adminID, id string, p SpotPatch) (store.Spot, error) {
	if !isUUID(id) {
		return store.Spot{}, apperr.NotFound("Spot")
	}
	var out store.Spot
	err := db.InTx(ctx, s.Pool, func(tx pgx.Tx) error {
		sp, err := store.SpotByIDForUpdate(ctx, tx, id)
		if db.IsNoRows(err) {
			return apperr.NotFound("Spot")
		}
		if err != nil {
			return err
		}
		status, st := sp.Status, sp.SpotType
		if p.Status != nil {
			if *p.Status != domain.SpotAvailable && *p.Status != domain.SpotOutOfService {
				return apperr.BadRequest("VALIDATION_ERROR", "Status must be AVAILABLE or OUT_OF_SERVICE.")
			}
			if sp.Status == domain.SpotOccupied {
				return ErrSpotOccupied
			}
			status = *p.Status
		}
		if p.SpotType != nil {
			nt := domain.SpotType(*p.SpotType)
			if !nt.Valid() {
				return apperr.BadRequest("VALIDATION_ERROR", "Spot type must be COMPACT, REGULAR or OVERSIZED.")
			}
			if sp.Status == domain.SpotOccupied && nt != sp.SpotType {
				return apperr.Conflict("SPOT_OCCUPIED", "Change the type after the vehicle leaves.")
			}
			st = nt
		}
		out, err = store.UpdateSpot(ctx, tx, id, st, status)
		if err != nil {
			return err
		}
		return s.audit(ctx, tx, adminID, "SPOT_UPDATED", "SPOT", id, p.Reason, map[string]any{
			"code": sp.Code, "fromStatus": sp.Status, "toStatus": status, "fromType": sp.SpotType, "toType": st})
	})
	return out, err
}

// DeleteSpot removes a spot that has never been used.
func (s *Service) DeleteSpot(ctx context.Context, adminID, id string) error {
	if !isUUID(id) {
		return apperr.NotFound("Spot")
	}
	return db.InTx(ctx, s.Pool, func(tx pgx.Tx) error {
		sp, err := store.SpotByIDForUpdate(ctx, tx, id)
		if db.IsNoRows(err) {
			return apperr.NotFound("Spot")
		}
		if err != nil {
			return err
		}
		used, err := store.SpotHasHistory(ctx, tx, id)
		if err != nil {
			return err
		}
		if used {
			return apperr.Conflict("SPOT_IN_USE", "This spot has ticket history. Mark it out of service instead.")
		}
		if err := store.DeleteSpot(ctx, tx, id); err != nil {
			return err
		}
		return s.audit(ctx, tx, adminID, "SPOT_DELETED", "SPOT", id, "", map[string]any{"code": sp.Code})
	})
}

// ---- pricing (FR-35, FR-36) ----

type PricingView struct {
	Current store.PricingRule   `json:"current"`
	History []store.PricingRule `json:"history"`
}

func (s *Service) Pricing(ctx context.Context, vt domain.VehicleType) (PricingView, error) {
	if !vt.Valid() {
		return PricingView{}, apperr.NotFound("Vehicle type")
	}
	cur, err := store.CurrentPricing(ctx, s.Pool, vt)
	if err != nil {
		return PricingView{}, err
	}
	hist, err := store.PricingHistory(ctx, s.Pool, vt)
	return PricingView{Current: cur, History: hist}, err
}

func (s *Service) AllPricing(ctx context.Context) ([]store.PricingRule, error) {
	return store.ListCurrentPricing(ctx, s.Pool)
}

type PricingInput struct {
	HourlyRatePaise int64  `json:"hourlyRatePaise"`
	DailyCapPaise   int64  `json:"dailyCapPaise"`
	GraceMinutes    int    `json:"graceMinutes"`
	Reason          string `json:"reason"`
}

// SetPricing versions the rate card; open tickets keep the rule they started with (BR-11).
func (s *Service) SetPricing(ctx context.Context, adminID string, vt domain.VehicleType, in PricingInput) (store.PricingRule, error) {
	if !vt.Valid() {
		return store.PricingRule{}, apperr.NotFound("Vehicle type")
	}
	var v apperr.Validation
	if in.HourlyRatePaise < 0 || in.HourlyRatePaise > 10_000_000 {
		v.Add("hourlyRatePaise", "Hourly rate must be between ₹0 and ₹1,00,000.")
	}
	if in.DailyCapPaise < 0 || in.DailyCapPaise > 100_000_000 {
		v.Add("dailyCapPaise", "Daily cap must be between ₹0 and ₹10,00,000.")
	}
	if in.DailyCapPaise < in.HourlyRatePaise {
		v.Add("dailyCapPaise", "Daily cap cannot be less than the hourly rate.")
	}
	if in.GraceMinutes < 0 || in.GraceMinutes > 120 {
		v.Add("graceMinutes", "Grace must be 0–120 minutes.")
	}
	if err := v.Err(); err != nil {
		return store.PricingRule{}, err
	}
	var out store.PricingRule
	err := db.InTx(ctx, s.Pool, func(tx pgx.Tx) error {
		prev, err := store.CurrentPricing(ctx, tx, vt)
		if err != nil {
			return err
		}
		out, err = store.ReplacePricing(ctx, tx, vt, domain.Rate{HourlyRatePaise: in.HourlyRatePaise,
			DailyCapPaise: in.DailyCapPaise, GraceMinutes: in.GraceMinutes})
		if err != nil {
			return err
		}
		return s.audit(ctx, tx, adminID, "PRICING_CHANGED", "PRICING", out.ID, in.Reason, map[string]any{
			"vehicleType": vt, "from": prev.Rate(), "to": out.Rate()})
	})
	return out, err
}

type Settings struct {
	OverflowEnabled  bool `json:"overflowEnabled"`
	SimulatorEnabled bool `json:"simulatorEnabled"`
}

func (s *Service) Settings(ctx context.Context) (Settings, error) {
	on, err := store.OverflowEnabled(ctx, s.Pool)
	return Settings{OverflowEnabled: on, SimulatorEnabled: s.Cfg.SimulatorEnabled}, err
}

func (s *Service) SetOverflow(ctx context.Context, adminID string, on bool, reason string) (Settings, error) {
	err := db.InTx(ctx, s.Pool, func(tx pgx.Tx) error {
		if err := store.SetOverflow(ctx, tx, on); err != nil {
			return err
		}
		return s.audit(ctx, tx, adminID, "OVERFLOW_TOGGLED", "SETTINGS", "overflow", reason, map[string]any{"enabled": on})
	})
	if err != nil {
		return Settings{}, err
	}
	return s.Settings(ctx)
}

// ---- gate devices (FR-37) ----

type GateWithKey struct {
	store.GateDevice
	APIKey string `json:"apiKey,omitempty"` // shown once
}

var gateNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]{2,50}$`)

func (s *Service) ListGates(ctx context.Context) ([]store.GateDevice, error) {
	return store.ListGates(ctx, s.Pool)
}

func (s *Service) CreateGate(ctx context.Context, adminID, name, kind string) (GateWithKey, error) {
	var v apperr.Validation
	name = strings.ToUpper(strings.TrimSpace(name))
	if !gateNameRe.MatchString(name) {
		v.Add("name", "Use 2–50 letters, digits, hyphens or underscores, e.g. IN-2.")
	}
	if strings.HasPrefix(name, "SIM-") {
		v.Add("name", "Names starting with SIM- are reserved for the simulator.")
	}
	if kind != domain.GateEntry && kind != domain.GateExit {
		v.Add("kind", "Kind must be ENTRY or EXIT.")
	}
	if err := v.Err(); err != nil {
		return GateWithKey{}, err
	}
	key, hash := auth.NewGateKey()
	var out GateWithKey
	err := db.InTx(ctx, s.Pool, func(tx pgx.Tx) error {
		g, err := store.InsertGate(ctx, tx, name, kind, hash)
		if err != nil {
			if db.IsUniqueViolation(err, "") {
				return apperr.Conflict("GATE_EXISTS", "A device with this name already exists.")
			}
			return err
		}
		out = GateWithKey{GateDevice: g, APIKey: key}
		return s.audit(ctx, tx, adminID, "GATE_CREATED", "GATE", g.ID, "", map[string]any{"name": name, "kind": kind})
	})
	return out, err
}

func (s *Service) RotateGateKey(ctx context.Context, adminID, id string) (GateWithKey, error) {
	if !isUUID(id) {
		return GateWithKey{}, apperr.NotFound("Gate device")
	}
	key, hash := auth.NewGateKey()
	var out GateWithKey
	err := db.InTx(ctx, s.Pool, func(tx pgx.Tx) error {
		g, err := store.SetGateKey(ctx, tx, id, hash)
		if db.IsNoRows(err) {
			return apperr.NotFound("Gate device").With("hint", "Simulator devices have no API key.")
		}
		if err != nil {
			return err
		}
		out = GateWithKey{GateDevice: g, APIKey: key}
		return s.audit(ctx, tx, adminID, "GATE_KEY_ROTATED", "GATE", id, "", map[string]any{"name": g.Name})
	})
	return out, err
}

type GatePatch struct {
	Name   *string `json:"name"`
	Active *bool   `json:"active"`
}

func (s *Service) UpdateGate(ctx context.Context, adminID, id string, p GatePatch) (store.GateDevice, error) {
	if !isUUID(id) {
		return store.GateDevice{}, apperr.NotFound("Gate device")
	}
	var out store.GateDevice
	err := db.InTx(ctx, s.Pool, func(tx pgx.Tx) error {
		g, err := store.GateByID(ctx, tx, id)
		if db.IsNoRows(err) {
			return apperr.NotFound("Gate device")
		}
		if err != nil {
			return err
		}
		name, active := g.Name, g.Active
		if p.Name != nil && !g.IsSimulator {
			n := strings.ToUpper(strings.TrimSpace(*p.Name))
			if !gateNameRe.MatchString(n) || strings.HasPrefix(n, "SIM-") {
				return apperr.BadRequest("VALIDATION_ERROR", "Use 2–50 letters, digits, hyphens or underscores (not SIM-).")
			}
			name = n
		}
		if p.Active != nil {
			active = *p.Active
		}
		out, err = store.UpdateGate(ctx, tx, id, name, active)
		if err != nil {
			if db.IsUniqueViolation(err, "") {
				return apperr.Conflict("GATE_EXISTS", "A device with this name already exists.")
			}
			return err
		}
		return s.audit(ctx, tx, adminID, "GATE_UPDATED", "GATE", id, "", map[string]any{"name": name, "active": active})
	})
	return out, err
}

// ---- audit ----

func (s *Service) AuditLog(ctx context.Context, limit, offset int) ([]store.AdminAction, int, error) {
	return store.ListActions(ctx, s.Pool, limit, offset)
}
