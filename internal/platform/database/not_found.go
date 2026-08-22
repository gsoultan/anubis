package database

import "github.com/gsoultan/anubis/internal/shared/apperr"

// NotFound is the canonical "no such row" error every adapter returns, so no
// pgx error text ever reaches a caller.
func NotFound() error { return apperr.ErrNotFound }
