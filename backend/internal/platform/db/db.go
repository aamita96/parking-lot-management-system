// Package db opens the Postgres pool, runs migrations and provides a transaction helper.
package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"plms/migrations"
)

// DBTX is satisfied by *pgxpool.Pool and pgx.Tx, so repositories work inside or outside a transaction.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Connect opens a pool and waits up to 60 s for the database to accept connections.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 20
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(60 * time.Second)
	for {
		pctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err = pool.Ping(pctx)
		cancel()
		if err == nil {
			return pool, nil
		}
		if time.Now().After(deadline) {
			pool.Close()
			return nil, fmt.Errorf("database not reachable: %w", err)
		}
		select {
		case <-ctx.Done():
			pool.Close()
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// Migrate applies all embedded migrations (golang-migrate).
func Migrate(url string) error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return err
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, toMigrateURL(url))
	if err != nil {
		return err
	}
	defer m.Close()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}

// golang-migrate's pgx/v5 driver registers the "pgx5" scheme.
func toMigrateURL(url string) string {
	for _, p := range []string{"postgres://", "postgresql://"} {
		if len(url) > len(p) && url[:len(p)] == p {
			return "pgx5://" + url[len(p):]
		}
	}
	return url
}

// InTx runs fn in a transaction, committing on nil and rolling back on error.
func InTx(ctx context.Context, pool *pgxpool.Pool, fn func(tx pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// IsUniqueViolation reports whether err is a unique violation on the named constraint/index
// (any unique violation when name is empty).
func IsUniqueViolation(err error, name string) bool {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return name == "" || pg.ConstraintName == name
	}
	return false
}

// IsNoRows reports pgx.ErrNoRows.
func IsNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
