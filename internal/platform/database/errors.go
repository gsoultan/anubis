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
