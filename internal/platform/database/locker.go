package database

import (
	"context"
	"log/slog"
)

// TryLock takes a session-scoped advisory lock so exactly one replica runs a
// maintenance job. The lock is held on a dedicated connection and released
// by the returned function; a caller that forgets to release leaks one
// connection until shutdown, which is why release is a defer-shaped value.
func (d *DB) TryLock(ctx context.Context, id int64) (bool, func(), error) {
	conn, err := d.pool.Acquire(ctx)
	if err != nil {
		return false, nil, err
	}
	var acquired bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", id).Scan(&acquired); err != nil {
		conn.Release()
		return false, nil, err
	}
	if !acquired {
		conn.Release()
		return false, nil, nil
	}
	return true, func() {
		if _, err := conn.Exec(context.WithoutCancel(ctx),
			"SELECT pg_advisory_unlock($1)", id); err != nil {
			slog.Warn("advisory unlock failed", "lock", id, "error", err)
		}
		conn.Release()
	}, nil
}
