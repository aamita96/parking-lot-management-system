// Package service holds the business logic between HTTP handlers and the store (NFR-15).
package service

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"plms/internal/apperr"
	"plms/internal/auth"
	"plms/internal/config"
	"plms/internal/email"
	"plms/internal/platform/db"
)

type Service struct {
	Pool   *pgxpool.Pool
	Cfg    config.Config
	Tokens *auth.Tokens
	Mailer email.Sender
	Log    *slog.Logger
}

func New(pool *pgxpool.Pool, cfg config.Config, tokens *auth.Tokens, mailer email.Sender, log *slog.Logger) *Service {
	return &Service{Pool: pool, Cfg: cfg, Tokens: tokens, Mailer: mailer, Log: log}
}

// Error codes returned by the gate and owner flows (SRS 7.7).
var (
	ErrUnreadablePlate = apperr.BadRequest("UNREADABLE_PLATE", "The plate could not be read.")
	ErrNotRegistered   = apperr.New(http.StatusNotFound, "NOT_REGISTERED", "This vehicle is not registered. Scan the QR code to register.")
	ErrNotVerified     = apperr.Conflict("NOT_VERIFIED", "The owner has not confirmed their email yet. Check your inbox for the code.")
	ErrAlreadyParked   = apperr.Conflict("ALREADY_PARKED", "This vehicle is already parked.")
	ErrNoSpot          = apperr.Conflict("NO_SPOT_AVAILABLE", "No compatible spot is free right now.")
	ErrNoActiveTicket  = apperr.New(http.StatusNotFound, "NO_ACTIVE_TICKET", "No open ticket for this vehicle. Please contact the car park office.")
	ErrAmountMismatch  = apperr.Conflict("AMOUNT_MISMATCH", "The amount does not match the fee.")
	ErrTicketClosed    = apperr.Conflict("TICKET_ALREADY_CLOSED", "This ticket is already paid and closed.")
	ErrPaymentNotDue   = apperr.Conflict("PAYMENT_NOT_DUE", "This ticket has not been scanned at the exit yet.")
	ErrSpotOccupied    = apperr.Conflict("SPOT_OCCUPIED", "An occupied spot cannot be taken out of service.")
	ErrVehicleTaken    = apperr.Conflict("VEHICLE_ALREADY_REGISTERED", "This vehicle number is registered to someone else.")
	ErrCodeInvalid     = apperr.New(http.StatusUnprocessableEntity, "CODE_INVALID", "That code is not right. Check the email and try again.")
	ErrCodeExpired     = apperr.New(http.StatusUnprocessableEntity, "CODE_EXPIRED", "That code has expired. Ask for a new one.")
	ErrTooManyCodes    = apperr.New(http.StatusTooManyRequests, "RATE_LIMITED", "Too many codes requested for this email. Try again in an hour.")
	ErrEmailQuota      = apperr.New(http.StatusTooManyRequests, "RATE_LIMITED", "We can't send email right now. Try again later.")
	ErrVehicleParked   = apperr.Conflict("VEHICLE_PARKED", "This vehicle is parked right now. Remove it after it leaves.")
	ErrEmailInUse      = apperr.Conflict("EMAIL_IN_USE", "Another owner already uses this email.")
)

// committedError marks an error whose transaction must still commit (e.g. counting a wrong code guess).
type committedError struct{ err error }

func (c committedError) Error() string { return c.err.Error() }

func commitThen(err error) error { return committedError{err} }

// inTx runs fn in a transaction. If fn returns commitThen(err), the work so far is committed
// and err is returned to the caller.
func (s *Service) inTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	var after error
	err := db.InTx(ctx, s.Pool, func(tx pgx.Tx) error {
		err := fn(tx)
		var ce committedError
		if errors.As(err, &ce) {
			after = ce.err
			return nil
		}
		return err
	})
	if err != nil {
		return err
	}
	return after
}

func (s *Service) logError(ctx context.Context, msg string, err error, args ...any) {
	s.Log.ErrorContext(ctx, msg, append(args, "err", err)...)
}

func isUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}
