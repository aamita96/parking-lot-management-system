package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"plms/internal/apperr"
	"plms/internal/domain"
	"plms/internal/platform/httpx"
	"plms/internal/service"
	"plms/internal/store"
)

// ---- public ----

func (a *API) availability(w http.ResponseWriter, r *http.Request) {
	v, err := a.svc.Availability(r.Context())
	respond(w, r, 200, v, err)
}

func (a *API) publicConfig(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, 200, map[string]any{
		"simulatorEnabled": a.svc.Cfg.SimulatorEnabled,
		"devCodes":         a.svc.Cfg.ExposeDevCodes(),
		"publicUrl":        a.svc.Cfg.PublicURL,
	})
}

func (a *API) startRegistration(w http.ResponseWriter, r *http.Request) {
	var in service.VehicleInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	res, err := a.svc.StartRegistration(r.Context(), in)
	status := http.StatusAccepted
	if res.Status == "ALREADY_REGISTERED" {
		status = http.StatusOK
	}
	respond(w, r, status, res, err)
}

type codeBody struct {
	Code  string `json:"code"`
	Email string `json:"email"`
}

func (a *API) verifyRegistration(w http.ResponseWriter, r *http.Request) {
	var in codeBody
	if err := httpx.Decode(r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	res, err := a.svc.VerifyRegistration(r.Context(), chi.URLParam(r, "id"), strings.TrimSpace(in.Code))
	respond(w, r, 200, res, err)
}

func (a *API) resendRegistration(w http.ResponseWriter, r *http.Request) {
	res, err := a.svc.ResendRegistrationCode(r.Context(), chi.URLParam(r, "id"))
	respond(w, r, http.StatusAccepted, res, err)
}

func (a *API) startSignIn(w http.ResponseWriter, r *http.Request) {
	var in codeBody
	if err := httpx.Decode(r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	res, err := a.svc.StartSignIn(r.Context(), in.Email)
	respond(w, r, http.StatusAccepted, res, err)
}

func (a *API) verifySignIn(w http.ResponseWriter, r *http.Request) {
	var in codeBody
	if err := httpx.Decode(r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	res, err := a.svc.VerifySignIn(r.Context(), in.Email, strings.TrimSpace(in.Code))
	respond(w, r, 200, res, err)
}

func (a *API) openDispute(w http.ResponseWriter, r *http.Request) {
	var in service.DisputeInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	res, err := a.svc.OpenDispute(r.Context(), in)
	respond(w, r, http.StatusCreated, res, err)
}

func (a *API) checkNumber(w http.ResponseWriter, r *http.Request) {
	res, err := a.svc.CheckNumber(r.Context(), chi.URLParam(r, "number"))
	respond(w, r, 200, res, err)
}

// ---- owner ----

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	res, err := a.svc.Me(r.Context(), ownerFrom(r.Context()).ID)
	respond(w, r, 200, res, err)
}

func (a *API) updateMe(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Mobile *string `json:"mobile"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	res, err := a.svc.UpdateMyMobile(r.Context(), ownerFrom(r.Context()).ID, in.Mobile)
	respond(w, r, 200, res, err)
}

func (a *API) refreshOwner(w http.ResponseWriter, r *http.Request) {
	res, err := a.svc.RefreshOwnerToken(r.Context(), ownerFrom(r.Context()).ID)
	respond(w, r, 200, res, err)
}

func (a *API) startEmailChange(w http.ResponseWriter, r *http.Request) {
	var in codeBody
	if err := httpx.Decode(r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	res, err := a.svc.StartEmailChange(r.Context(), ownerFrom(r.Context()).ID, in.Email)
	respond(w, r, http.StatusAccepted, res, err)
}

func (a *API) confirmEmailChange(w http.ResponseWriter, r *http.Request) {
	var in codeBody
	if err := httpx.Decode(r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	res, err := a.svc.ConfirmEmailChange(r.Context(), ownerFrom(r.Context()).ID, strings.TrimSpace(in.Code))
	respond(w, r, 200, res, err)
}

func (a *API) myVehicles(w http.ResponseWriter, r *http.Request) {
	res, err := a.svc.MyVehicles(r.Context(), ownerFrom(r.Context()).ID)
	respond(w, r, 200, map[string]any{"items": res}, err)
}

func (a *API) addMyVehicle(w http.ResponseWriter, r *http.Request) {
	var in service.VehicleInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	res, created, err := a.svc.AddMyVehicle(r.Context(), ownerFrom(r.Context()).ID, in)
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	respond(w, r, status, res, err)
}

func (a *API) updateMyVehicle(w http.ResponseWriter, r *http.Request) {
	var in service.VehiclePatch
	if err := httpx.Decode(r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	res, err := a.svc.UpdateMyVehicle(r.Context(), ownerFrom(r.Context()).ID, chi.URLParam(r, "id"), in)
	respond(w, r, 200, res, err)
}

func (a *API) removeMyVehicle(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.RemoveMyVehicle(r.Context(), ownerFrom(r.Context()).ID, chi.URLParam(r, "id")); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) myTickets(w http.ResponseWriter, r *http.Request) {
	limit, offset, page, size := paging(r)
	list, total, err := a.svc.MyTickets(r.Context(), ownerFrom(r.Context()).ID, limit, offset)
	respond(w, r, 200, pageOf[store.Ticket]{list, total, page, size}, err)
}

// ---- gate ----

func (a *API) entryScan(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Plate string `json:"plate"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	res, err := a.svc.EntryScan(r.Context(), gateFrom(r.Context()), in.Plate)
	respond(w, r, http.StatusCreated, res, err)
}

func (a *API) exitScan(w http.ResponseWriter, r *http.Request) {
	var in service.ExitInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	res, err := a.svc.ExitScan(r.Context(), gateFrom(r.Context()), in)
	respond(w, r, 200, res, err)
}

func (a *API) payment(w http.ResponseWriter, r *http.Request) {
	var in service.PaymentInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	res, err := a.svc.Pay(r.Context(), gateFrom(r.Context()), in)
	respond(w, r, 200, res, err)
}

// ---- admin auth ----

const refreshCookie = "plms_admin_refresh"

func (a *API) setRefreshCookie(w http.ResponseWriter, s service.AdminSession) {
	http.SetCookie(w, &http.Cookie{
		Name: refreshCookie, Value: s.RefreshToken, Path: "/api/v1/admin/auth", Expires: s.RefreshExp,
		HttpOnly: true, Secure: a.svc.Cfg.CookieSecure, SameSite: http.SameSiteStrictMode,
	})
}

func (a *API) adminLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	s, err := a.svc.AdminLogin(r.Context(), in.Email, in.Password)
	if err == nil {
		a.setRefreshCookie(w, s)
	}
	respond(w, r, 200, s, err)
}

func (a *API) adminRefresh(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(refreshCookie)
	if err != nil {
		httpx.WriteError(w, r, apperr.ErrUnauthenticated)
		return
	}
	s, err := a.svc.AdminRefresh(r.Context(), c.Value)
	if err == nil {
		a.setRefreshCookie(w, s)
	}
	respond(w, r, 200, s, err)
}

func (a *API) adminLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: refreshCookie, Value: "", Path: "/api/v1/admin/auth", MaxAge: -1,
		HttpOnly: true, Secure: a.svc.Cfg.CookieSecure, SameSite: http.SameSiteStrictMode})
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) adminMe(w http.ResponseWriter, r *http.Request) {
	p := adminFrom(r.Context())
	httpx.WriteJSON(w, 200, service.AdminView{ID: p.ID, Email: p.Email})
}

// ---- simulator ----

func (a *API) simLayout(w http.ResponseWriter, r *http.Request) {
	res, err := a.svc.Layout(r.Context())
	respond(w, r, 200, res, err)
}

func (a *API) simEvents(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	res, err := a.svc.SimulatorEvents(r.Context(), limit)
	respond(w, r, 200, map[string]any{"items": res}, err)
}

func (a *API) simReset(w http.ResponseWriter, r *http.Request) {
	n, err := a.svc.ResetSimulator(r.Context(), adminFrom(r.Context()).ID)
	respond(w, r, 200, map[string]any{"cancelledTickets": n}, err)
}

// ---- exceptions ----

func (a *API) exceptions(w http.ResponseWriter, r *http.Request) {
	res, err := a.svc.Exceptions(r.Context())
	respond(w, r, 200, res, err)
}

type reasonBody struct {
	Reason string `json:"reason"`
}

func (a *API) resolveScan(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.WriteError(w, r, apperr.NotFound("Scan"))
		return
	}
	var in reasonBody
	if err := httpx.Decode(r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	err = a.svc.ResolveScan(r.Context(), adminFrom(r.Context()).ID, id, in.Reason)
	respond(w, r, 200, map[string]string{"status": "RESOLVED"}, err)
}

func (a *API) resolveDispute(w http.ResponseWriter, r *http.Request) {
	var in reasonBody
	if err := httpx.Decode(r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	err := a.svc.ResolveDispute(r.Context(), adminFrom(r.Context()).ID, chi.URLParam(r, "id"), in.Reason)
	respond(w, r, 200, map[string]string{"status": "RESOLVED"}, err)
}

// ---- tickets ----

func (a *API) searchTickets(w http.ResponseWriter, r *http.Request) {
	limit, offset, page, size := paging(r)
	q := r.URL.Query()
	f := store.TicketFilter{Query: strings.TrimSpace(q.Get("q")), Status: q.Get("status"), Limit: limit, Offset: offset}
	switch q.Get("simulated") {
	case "true":
		t := true
		f.Simulated = &t
	case "false":
		t := false
		f.Simulated = &t
	}
	if f.Status != "" && f.Status != "OPEN" && f.Status != domain.TicketActive && f.Status != domain.TicketPaymentPending &&
		f.Status != domain.TicketClosed && f.Status != domain.TicketCancelled {
		httpx.WriteError(w, r, apperr.BadRequest("VALIDATION_ERROR", "Unknown status filter."))
		return
	}
	list, total, err := a.svc.SearchTickets(r.Context(), f)
	respond(w, r, 200, pageOf[store.Ticket]{list, total, page, size}, err)
}

func (a *API) manualCheckIn(w http.ResponseWriter, r *http.Request) {
	var in service.ManualCheckIn
	if err := httpx.Decode(r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	res, err := a.svc.ManualCheckInTicket(r.Context(), adminFrom(r.Context()).ID, in)
	respond(w, r, http.StatusCreated, res, err)
}

func (a *API) ticketDetail(w http.ResponseWriter, r *http.Request) {
	res, err := a.svc.TicketDetail(r.Context(), chi.URLParam(r, "id"))
	respond(w, r, 200, res, err)
}

func (a *API) manualExit(w http.ResponseWriter, r *http.Request) {
	var in service.ManualExit
	if err := httpx.Decode(r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	res, err := a.svc.ManualExitTicket(r.Context(), adminFrom(r.Context()).ID, chi.URLParam(r, "id"), in)
	respond(w, r, 200, res, err)
}

func (a *API) resendNotification(w http.ResponseWriter, r *http.Request) {
	res, err := a.svc.ResendNotification(r.Context(), adminFrom(r.Context()).ID, chi.URLParam(r, "id"),
		strings.ToUpper(chi.URLParam(r, "kind")))
	respond(w, r, http.StatusAccepted, res, err)
}

// ---- owners & vehicles ----

func (a *API) searchOwners(w http.ResponseWriter, r *http.Request) {
	limit, offset, page, size := paging(r)
	list, total, err := a.svc.SearchOwners(r.Context(), strings.TrimSpace(r.URL.Query().Get("q")), limit, offset)
	respond(w, r, 200, pageOf[store.OwnerSummary]{list, total, page, size}, err)
}

func (a *API) searchVehicles(w http.ResponseWriter, r *http.Request) {
	limit, offset, page, size := paging(r)
	q := r.URL.Query()
	f := store.VehicleFilter{Query: strings.TrimSpace(q.Get("q")), Status: q.Get("status"), Type: q.Get("type"), Limit: limit, Offset: offset}
	if f.Status != "" && f.Status != domain.VehicleActive && f.Status != domain.VehiclePending && f.Status != domain.VehicleRemoved {
		httpx.WriteError(w, r, apperr.BadRequest("VALIDATION_ERROR", "Unknown status filter."))
		return
	}
	if f.Type != "" && !domain.VehicleType(f.Type).Valid() {
		httpx.WriteError(w, r, apperr.BadRequest("VALIDATION_ERROR", "Unknown vehicle type."))
		return
	}
	list, total, err := a.svc.SearchVehicles(r.Context(), f)
	respond(w, r, 200, pageOf[store.Vehicle]{list, total, page, size}, err)
}

func (a *API) reassignVehicle(w http.ResponseWriter, r *http.Request) {
	var in service.Reassign
	if err := httpx.Decode(r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	res, err := a.svc.ReassignVehicle(r.Context(), adminFrom(r.Context()).ID, chi.URLParam(r, "id"), in)
	respond(w, r, 200, res, err)
}

// ---- spots ----

func (a *API) listSpots(w http.ResponseWriter, r *http.Request) {
	res, err := a.svc.Spots(r.Context())
	respond(w, r, 200, map[string]any{"items": res}, err)
}

func (a *API) createSpots(w http.ResponseWriter, r *http.Request) {
	var in service.SpotInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	res, err := a.svc.CreateSpots(r.Context(), adminFrom(r.Context()).ID, in)
	respond(w, r, http.StatusCreated, map[string]any{"items": res}, err)
}

func (a *API) updateSpot(w http.ResponseWriter, r *http.Request) {
	var in service.SpotPatch
	if err := httpx.Decode(r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	res, err := a.svc.UpdateSpot(r.Context(), adminFrom(r.Context()).ID, chi.URLParam(r, "id"), in)
	respond(w, r, 200, res, err)
}

func (a *API) deleteSpot(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.DeleteSpot(r.Context(), adminFrom(r.Context()).ID, chi.URLParam(r, "id")); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- pricing & settings ----

func (a *API) allPricing(w http.ResponseWriter, r *http.Request) {
	res, err := a.svc.AllPricing(r.Context())
	respond(w, r, 200, map[string]any{"items": res}, err)
}

func (a *API) getPricing(w http.ResponseWriter, r *http.Request) {
	res, err := a.svc.Pricing(r.Context(), domain.VehicleType(strings.ToUpper(chi.URLParam(r, "vehicleType"))))
	respond(w, r, 200, res, err)
}

func (a *API) putPricing(w http.ResponseWriter, r *http.Request) {
	var in service.PricingInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	res, err := a.svc.SetPricing(r.Context(), adminFrom(r.Context()).ID,
		domain.VehicleType(strings.ToUpper(chi.URLParam(r, "vehicleType"))), in)
	respond(w, r, 200, res, err)
}

func (a *API) getSettings(w http.ResponseWriter, r *http.Request) {
	res, err := a.svc.Settings(r.Context())
	respond(w, r, 200, res, err)
}

func (a *API) putOverflow(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled bool   `json:"enabled"`
		Reason  string `json:"reason"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	res, err := a.svc.SetOverflow(r.Context(), adminFrom(r.Context()).ID, in.Enabled, in.Reason)
	respond(w, r, 200, res, err)
}

// ---- gate devices ----

func (a *API) listGates(w http.ResponseWriter, r *http.Request) {
	res, err := a.svc.ListGates(r.Context())
	respond(w, r, 200, map[string]any{"items": res}, err)
}

func (a *API) createGate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
		Kind string `json:"kind"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	res, err := a.svc.CreateGate(r.Context(), adminFrom(r.Context()).ID, in.Name, strings.ToUpper(in.Kind))
	respond(w, r, http.StatusCreated, res, err)
}

func (a *API) updateGate(w http.ResponseWriter, r *http.Request) {
	var in service.GatePatch
	if err := httpx.Decode(r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	res, err := a.svc.UpdateGate(r.Context(), adminFrom(r.Context()).ID, chi.URLParam(r, "id"), in)
	respond(w, r, 200, res, err)
}

func (a *API) rotateGateKey(w http.ResponseWriter, r *http.Request) {
	res, err := a.svc.RotateGateKey(r.Context(), adminFrom(r.Context()).ID, chi.URLParam(r, "id"))
	respond(w, r, 200, res, err)
}

func (a *API) auditLog(w http.ResponseWriter, r *http.Request) {
	limit, offset, page, size := paging(r)
	list, total, err := a.svc.AuditLog(r.Context(), limit, offset)
	respond(w, r, 200, pageOf[store.AdminAction]{list, total, page, size}, err)
}
