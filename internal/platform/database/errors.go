package database

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/gsoultan/anubis/internal/shared/apperr"
)

// MapErr translates driver errors into the domain vocabulary so no pgx error
// text ever leaks to callers.
func MapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return apperr.ErrConflict.Wrap(err)
		case "23503", "23514", "0A000", "42501", "P0001":
			// FK, CHECK, guard-trigger and raised exceptions: the schema said
			// no. That is an invalid request, not an internal fault.
			return apperr.ErrInvalidArgument.Wrap(err)
		}
	}
	return apperr.ErrInternal.Wrap(err)
}

// MapErrSaying is MapErr for statements whose refusals are sentences this
// schema wrote: a guard trigger or function that RAISEs. The sentence travels
// as the "reason" detail, because "check constraint violated" tells an
// operator that something is wrong and nothing about what.
//
// Opt-in per call site, not the default. MapErr's promise is that no driver
// text reaches a caller, and a raised message is only safe to show when it
// names nothing outside the caller's own tenant — which is a property of each
// guard, read before its call site switches to this. A violation of a declared
// constraint (ConstraintName set) is never included: PostgreSQL writes that
// message, and its detail carries the row.
func MapErrSaying(err error) error {
	mapped := MapErr(err)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.ConstraintName != "" {
		return mapped
	}
	if pgErr.Code != "23514" && pgErr.Code != "P0001" {
		return mapped
	}
	return apperr.AsError(mapped).With("reason", pgErr.Message)
}
