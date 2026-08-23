package database

import (
	"context"
	"log/slog"

	gen "github.com/gsoultan/anubis/internal/platform/database/gen"
)

// TryLock takes a session-scoped advisory lock so exactly one replica runs a
// maintenance job. The lock lives on a dedicated connection and is released
// by the returned function; a caller that forgets to release leaks one
// connection until shutdown, which is why release is a defer-shaped value.
func (d *DB) TryLock(ctx context.Context, id int64) (bool, func(), error) {
	conn, err := d.pool.Acquire(ctx)
	if err != nil {
		return false, nil, err
	}
	q := gen.New(conn)
	acquired, err := q.TryAdvisoryLock(ctx, id)
	if err != nil {
		conn.Release()
		return false, nil, err
	}
	if !acquired {
		conn.Release()
		return false, nil, nil // another replica has it; not an error
	}
	return true, func() {
		// Unlock on the SAME connection, and outside the caller's cancelled
		// context — a cancelled cleanup would leak the lock until the
		// connection is recycled.
		if _, err := gen.New(conn).AdvisoryUnlock(context.WithoutCancel(ctx), id); err != nil {
			slog.Warn("advisory unlock failed", "lock", id, "error", err)
		}
		conn.Release()
	}, nil
}
