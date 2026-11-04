// Package httpx has JSON helpers, Problem Details rendering and HTTP middleware.
package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"plms/internal/apperr"
	"plms/internal/platform/metrics"
)

type ctxKey int

const (
	keyRequestID ctxKey = iota
	keyTraceID
)

// WriteJSON writes v with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// ProblemBody builds the RFC 9457 body for an app error.
func ProblemBody(e *apperr.Error, r *http.Request) map[string]any {
	body := map[string]any{
		"type":   "https://plms.local/problems/" + strings.ToLower(strings.ReplaceAll(e.Code, "_", "-")),
		"title":  http.StatusText(e.Status),
		"status": e.Status,
		"code":   e.Code,
		"detail": e.Message,
		"message": e.Message,
	}
	if r != nil {
		body["instance"] = r.URL.Path
		body["traceId"] = TraceID(r.Context())
	}
	if len(e.Fields) > 0 {
		body["errors"] = e.Fields
	}
	for k, v := range e.Extra {
		body[k] = v
	}
	return body
}

// WriteError renders any error as Problem Details. Unknown errors become 500 INTERNAL.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	e, ok := apperr.As(err)
	if !ok {
		slog.ErrorContext(r.Context(), "unhandled error", "err", err, "path", r.URL.Path,
			"request_id", RequestID(r.Context()), "trace_id", TraceID(r.Context()))
		e = apperr.New(http.StatusInternalServerError, "INTERNAL", "Something went wrong. Please try again.")
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(e.Status)
	_ = json.NewEncoder(w).Encode(ProblemBody(e, r))
}

// Decode reads a JSON body (max 1 MiB) into v.
func Decode(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return apperr.BadRequest("VALIDATION_ERROR", "Request body is required.")
		}
		return apperr.BadRequest("VALIDATION_ERROR", "Request body is not valid JSON.")
	}
	return nil
}

func RequestID(ctx context.Context) string { s, _ := ctx.Value(keyRequestID).(string); return s }
func TraceID(ctx context.Context) string   { s, _ := ctx.Value(keyTraceID).(string); return s }

// WithTraceID returns a context carrying a trace ID (used by background jobs).
func WithTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyTraceID, id)
}

// NewTraceID returns a 16-byte W3C trace ID in hex.
func NewTraceID() string { return randHex(16) }

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Traceparent formats a W3C traceparent header for a trace ID.
func Traceparent(traceID string) string {
	if len(traceID) != 32 {
		traceID = NewTraceID()
	}
	return "00-" + traceID + "-" + randHex(8) + "-01"
}

// ParseTraceparent extracts the trace ID from a W3C traceparent header.
func ParseTraceparent(h string) string {
	parts := strings.Split(h, "-")
	if len(parts) == 4 && len(parts[1]) == 32 {
		return parts[1]
	}
	return ""
}

// RequestContext assigns a request ID and trace ID (honouring an incoming traceparent).
func RequestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := r.Header.Get("X-Request-Id")
		if rid == "" || len(rid) > 64 {
			rid = randHex(8)
		}
		tid := ParseTraceparent(r.Header.Get("traceparent"))
		if tid == "" {
			tid = NewTraceID()
		}
		ctx := context.WithValue(r.Context(), keyRequestID, rid)
		ctx = context.WithValue(ctx, keyTraceID, tid)
		w.Header().Set("X-Request-Id", rid)
		w.Header().Set("X-Trace-Id", tid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// AccessLog logs each request as structured JSON and records latency metrics (NFR-11, NFR-12).
func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		route := "unmatched"
		if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
			route = rc.RoutePattern()
		}
		elapsed := time.Since(start)
		metrics.HTTPDuration.WithLabelValues(r.Method, route, statusClass(rec.status)).Observe(elapsed.Seconds())
		if route == "/healthz" || route == "/readyz" || route == "/metrics" {
			return
		}
		slog.InfoContext(r.Context(), "http request",
			"method", r.Method, "route", route, "status", rec.status,
			"duration_ms", elapsed.Milliseconds(),
			"request_id", RequestID(r.Context()), "trace_id", TraceID(r.Context()))
	})
}

func statusClass(s int) string {
	switch {
	case s >= 500:
		return "5xx"
	case s >= 400:
		return "4xx"
	case s >= 300:
		return "3xx"
	}
	return "2xx"
}

// Recover turns panics into 500 problems.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				slog.ErrorContext(r.Context(), "panic", "value", v, "trace_id", TraceID(r.Context()))
				WriteError(w, r, errors.New("panic"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// ClientIP returns the caller IP (first X-Forwarded-For hop behind the frontend proxy).
func ClientIP(r *http.Request) string {
	if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
		return strings.TrimSpace(strings.Split(xf, ",")[0])
	}
	host := r.RemoteAddr
	if i := strings.LastIndexByte(host, ':'); i > 0 {
		host = host[:i]
	}
	return host
}
