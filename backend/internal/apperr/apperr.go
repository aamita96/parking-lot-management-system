// Package apperr defines the error type that services return and handlers render
// as RFC 9457 Problem Details with a stable `code` (SRS 7.7).
package apperr

import (
	"errors"
	"fmt"
	"net/http"
)

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type Error struct {
	Status  int
	Code    string
	Message string
	Fields  []FieldError
	Extra   map[string]any // merged into the problem body (e.g. result, plate, receipt)
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

func New(status int, code, msg string) *Error {
	return &Error{Status: status, Code: code, Message: msg}
}

// With returns a copy carrying extra body fields.
func (e *Error) With(k string, v any) *Error {
	cp := *e
	cp.Extra = map[string]any{}
	for kk, vv := range e.Extra {
		cp.Extra[kk] = vv
	}
	cp.Extra[k] = v
	return &cp
}

func As(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// Validation collects field errors; Err() returns nil when there are none.
type Validation struct{ fields []FieldError }

func (v *Validation) Add(field, msg string) { v.fields = append(v.fields, FieldError{field, msg}) }

func (v *Validation) Err() error {
	if len(v.fields) == 0 {
		return nil
	}
	return &Error{Status: http.StatusBadRequest, Code: "VALIDATION_ERROR",
		Message: "Some fields need attention.", Fields: v.fields}
}

// Common errors.
var (
	ErrUnauthenticated = New(http.StatusUnauthorized, "UNAUTHENTICATED", "Sign in to continue.")
	ErrForbidden       = New(http.StatusForbidden, "FORBIDDEN", "You are not allowed to do this.")
	ErrNotFound        = New(http.StatusNotFound, "NOT_FOUND", "Not found.")
	ErrRateLimited     = New(http.StatusTooManyRequests, "RATE_LIMITED", "Too many requests. Try again later.")
)

func NotFound(what string) *Error {
	return New(http.StatusNotFound, "NOT_FOUND", what+" not found.")
}

func Conflict(code, msg string) *Error { return New(http.StatusConflict, code, msg) }

func BadRequest(code, msg string) *Error { return New(http.StatusBadRequest, code, msg) }
