// Package postgres implements every repository interface over the
// sqlc-generated query layer (internal/adapter... no: gen). The ONLY place
// SQL executes; the SQL itself lives in db/queries (ADR-0009).
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	gen "github.com/gsoultan/anubis/internal/adapter/postgres/gen"
	"github.com/gsoultan/anubis/internal/domain"
)

// Store implements repository.Store. One struct; methods live in topical
// files per the one-type-per-file convention.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// txKey carries an open transaction through context so every Store method
// transparently joins WithinTx.
type txKey struct{}

// WithinTx implements repository.TxManager.
func (s *Store) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return fn(ctx) // nested WithinTx joins the outer transaction
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.ErrInternal.Wrap(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

	if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.ErrInternal.Wrap(err)
	}
	return nil
}

// q returns the query handle bound to the ambient transaction when present.
func (s *Store) q(ctx context.Context) *gen.Queries {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return gen.New(tx)
	}
	return gen.New(s.pool)
}

// mapErr translates driver errors into the domain vocabulary so no pgx error
// text leaks to callers.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return domain.ErrConflict.Wrap(err)
		case "23503", "23514", "0A000", "42501", "P0001":
			// FK, CHECK, guard-trigger and raised exceptions: the schema said
			// no. That is an invalid request, not an internal fault.
			return domain.ErrInvalidArgument.Wrap(err)
		}
	}
	return domain.ErrInternal.Wrap(err)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefT(t *time.Time) *time.Time { return t }

func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func optTime(t *time.Time) *time.Time {
	if t == nil || t.IsZero() {
		return nil
	}
	return t
}
