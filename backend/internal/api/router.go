// Package api is the HTTP layer: routing, auth middleware and handlers (SRS 7).
package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"plms/internal/apperr"
	"plms/internal/auth"
	"plms/internal/platform/httpx"
	"plms/internal/service"
)

type API struct {
	svc *service.Service
}

func New(svc *service.Service) *API { return &API{svc: svc} }

func (a *API) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(httpx.RequestContext, httpx.Recover, httpx.AccessLog)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   a.svc.Cfg.CORSOrigins, // NFR-8: frontend origin only
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "Idempotency-Key", "X-Gate-Key", "X-Gate-Id", "X-Request-Id", "traceparent"},
		ExposedHeaders:   []string{"X-Request-Id", "X-Trace-Id", "Idempotent-Replayed"},
		AllowCredentials: true,
		MaxAge:           600,
	}))
	r.Use(securityHeaders)

	r.Get("/healthz", a.healthz)
	r.Get("/readyz", a.readyz)
	r.Handle("/metrics", promhttp.Handler())

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/healthz", a.healthz)
		r.Get("/readyz", a.readyz)
		r.Get("/availability", a.availability)
		r.Get("/public/config", a.publicConfig)

		// Owner self-service; code endpoints are rate-limited per IP (NFR-8).
		r.Route("/public", func(r chi.Router) {
			r.Use(httprate.Limit(30, time.Minute, httprate.WithKeyFuncs(func(r *http.Request) (string, error) {
				return httpx.ClientIP(r), nil
			}), httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
				httpx.WriteError(w, r, apperr.ErrRateLimited)
			})))
			r.Post("/registrations", a.startRegistration)
			r.Post("/registrations/{id}/verify", a.verifyRegistration)
			r.Post("/registrations/{id}/resend", a.resendRegistration)
			r.Post("/sign-in", a.startSignIn)
			r.Post("/sign-in/verify", a.verifySignIn)
			r.Post("/disputes", a.openDispute)
			r.Get("/vehicles/{number}/check", a.checkNumber)
		})

		r.Route("/me", func(r chi.Router) {
			r.Use(a.requireRole(auth.RoleOwner, keyOwner))
			r.Get("/", a.me)
			r.Patch("/", a.updateMe)
			r.Post("/token/refresh", a.refreshOwner)
			r.Post("/email-change", a.startEmailChange)
			r.Post("/email-change/verify", a.confirmEmailChange)
			r.Get("/vehicles", a.myVehicles)
			r.Post("/vehicles", a.addMyVehicle)
			r.Patch("/vehicles/{id}", a.updateMyVehicle)
			r.Delete("/vehicles/{id}", a.removeMyVehicle)
			r.Get("/tickets", a.myTickets)
		})

		r.Route("/gate", func(r chi.Router) {
			r.Use(a.gateAuth, a.idempotency)
			r.Post("/entry-scans", a.entryScan)
			r.Post("/exit-scans", a.exitScan)
			r.Post("/payments", a.payment)
		})

		r.Route("/admin", func(r chi.Router) {
			r.With(httprate.LimitByIP(10, time.Minute)).Post("/auth/login", a.adminLogin)
			r.Post("/auth/refresh", a.adminRefresh)
			r.Post("/auth/logout", a.adminLogout)

			r.Group(func(r chi.Router) {
				r.Use(a.requireRole(auth.RoleAdmin, keyAdmin))
				r.Get("/me", a.adminMe)

				r.Get("/simulator/layout", a.simLayout)
				r.Get("/simulator/events", a.simEvents)
				r.Post("/simulator/reset", a.simReset)

				r.Get("/exceptions", a.exceptions)
				r.Post("/exceptions/scans/{id}/resolve", a.resolveScan)
				r.Post("/disputes/{id}/resolve", a.resolveDispute)

				r.Get("/tickets", a.searchTickets)
				r.Post("/tickets", a.manualCheckIn)
				r.Get("/tickets/{id}", a.ticketDetail)
				r.Post("/tickets/{id}/manual-exit", a.manualExit)
				r.Post("/tickets/{id}/notifications/{kind}/resend", a.resendNotification)

				r.Get("/owners", a.searchOwners)
				r.Get("/vehicles", a.searchVehicles)
				r.Patch("/vehicles/{id}", a.reassignVehicle)

				r.Get("/spots", a.listSpots)
				r.Post("/spots", a.createSpots)
				r.Patch("/spots/{id}", a.updateSpot)
				r.Delete("/spots/{id}", a.deleteSpot)

				r.Get("/pricing", a.allPricing)
				r.Get("/pricing/{vehicleType}", a.getPricing)
				r.Put("/pricing/{vehicleType}", a.putPricing)
				r.Get("/settings", a.getSettings)
				r.Put("/settings/overflow", a.putOverflow)

				r.Get("/gate-devices", a.listGates)
				r.Post("/gate-devices", a.createGate)
				r.Patch("/gate-devices/{id}", a.updateGate)
				r.Post("/gate-devices/{id}/rotate-key", a.rotateGateKey)
				r.Patch("/gate-devices/{id}/rotate-key", a.rotateGateKey)

				r.Get("/audit", a.auditLog)
			})
		})
	})

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteError(w, r, apperr.NotFound("Route"))
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteError(w, r, apperr.New(http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed."))
	})
	return r
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func (a *API) healthz(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, 200, map[string]string{"status": "ok"})
}

func (a *API) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.svc.Pool.Ping(ctx); err != nil {
		httpx.WriteJSON(w, 503, map[string]string{"status": "database unavailable"})
		return
	}
	httpx.WriteJSON(w, 200, map[string]string{"status": "ready"})
}

// ---- helpers ----

func paging(r *http.Request) (limit, offset, page, size int) {
	page, _ = strconv.Atoi(r.URL.Query().Get("page"))
	size, _ = strconv.Atoi(r.URL.Query().Get("pageSize"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 500 {
		size = 25
	}
	return size, (page - 1) * size, page, size
}

type pageOf[T any] struct {
	Items    []T `json:"items"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
}

func respond(w http.ResponseWriter, r *http.Request, status int, v any, err error) {
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, status, v)
}
