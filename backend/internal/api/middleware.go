package api

import (
	"bytes"
	"context"
	"net/http"
	"strings"

	"plms/internal/apperr"
	"plms/internal/auth"
	"plms/internal/platform/db"
	"plms/internal/platform/httpx"
	"plms/internal/store"
)

type ctxKey int

const (
	keyOwner ctxKey = iota
	keyAdmin
	keyGate
)

type principal struct {
	ID    string
	Email string
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

func ownerFrom(ctx context.Context) principal { p, _ := ctx.Value(keyOwner).(principal); return p }
func adminFrom(ctx context.Context) principal { p, _ := ctx.Value(keyAdmin).(principal); return p }
func gateFrom(ctx context.Context) store.GateDevice {
	g, _ := ctx.Value(keyGate).(store.GateDevice)
	return g
}

// requireRole checks a bearer JWT for the role (NFR-8: role checks on every endpoint).
func (a *API) requireRole(role string, key ctxKey) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := a.svc.Tokens.Parse(bearer(r), role)
			if err != nil {
				httpx.WriteError(w, r, apperr.ErrUnauthenticated)
				return
			}
			ctx := context.WithValue(r.Context(), key, principal{ID: c.Subject, Email: c.Email})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// gateAuth accepts a device X-Gate-Key, or an admin JWT plus X-Gate-Id for simulator devices (SRS 7.1).
func (a *API) gateAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		var g store.GateDevice
		var err error
		if key := r.Header.Get("X-Gate-Key"); key != "" {
			g, err = a.svc.GateByKey(ctx, key)
		} else if gid := r.Header.Get("X-Gate-Id"); gid != "" {
			c, perr := a.svc.Tokens.Parse(bearer(r), auth.RoleAdmin)
			if perr != nil {
				httpx.WriteError(w, r, apperr.ErrUnauthenticated)
				return
			}
			ctx = context.WithValue(ctx, keyAdmin, principal{ID: c.Subject, Email: c.Email})
			g, err = a.svc.SimulatorGate(ctx, gid)
		} else {
			err = apperr.New(401, "UNAUTHENTICATED", "Send X-Gate-Key, or an admin token with X-Gate-Id.")
		}
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, keyGate, g)))
	})
}

type captureWriter struct {
	http.ResponseWriter
	status int
	buf    bytes.Buffer
}

func (c *captureWriter) WriteHeader(code int) {
	c.status = code
	c.ResponseWriter.WriteHeader(code)
}

func (c *captureWriter) Write(b []byte) (int, error) {
	c.buf.Write(b)
	return c.ResponseWriter.Write(b)
}

// idempotency replays the first response for a repeated Idempotency-Key within 24 h (SRS 7.2).
func (a *API) idempotency(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Idempotency-Key")
		if key == "" || len(key) > 100 {
			next.ServeHTTP(w, r)
			return
		}
		g := gateFrom(r.Context())
		endpoint := r.URL.Path
		if prev, err := store.GetIdempotency(r.Context(), a.svc.Pool, g.ID, key); err == nil {
			if prev.Endpoint != endpoint {
				httpx.WriteError(w, r, apperr.Conflict("IDEMPOTENCY_KEY_REUSED", "This Idempotency-Key was used for another request."))
				return
			}
			ct := "application/json"
			if prev.StatusCode >= 400 {
				ct = "application/problem+json"
			}
			w.Header().Set("Content-Type", ct)
			w.Header().Set("Idempotent-Replayed", "true")
			w.WriteHeader(prev.StatusCode)
			_, _ = w.Write(prev.Body)
			return
		} else if !db.IsNoRows(err) {
			httpx.WriteError(w, r, err)
			return
		}
		cw := &captureWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(cw, r)
		if cw.status < 500 && cw.buf.Len() > 0 {
			if err := store.PutIdempotency(r.Context(), a.svc.Pool, g.ID, key, endpoint, cw.status, bytes.TrimSpace(cw.buf.Bytes())); err != nil {
				a.svc.Log.WarnContext(r.Context(), "idempotency store failed", "err", err)
			}
		}
	})
}
