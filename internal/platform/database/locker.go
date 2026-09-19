package database

import (
	"context"
	"log/slog"

	// The generated package's init registers the raw-row scanners; the blank
	// import is what makes the declarations below executable.
	_ "github.com/gsoultan/anubis/internal/platform/database/rgen"
	platformrquery "github.com/gsoultan/anubis/internal/platform/database/rquery"
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
	// Bound to THIS connection, not to the pool: the lock is held by the
	// session that took it, so a release routed through the pool would run on
	// whichever connection came back next and release nothing.
	ex := Executor(conn)
	row, _, err := platformrquery.TryAdvisoryLock.One(ctx, ex, id)
	if err != nil {
		conn.Release()
		return false, nil, err
	}
	if !row.Acquired {
		conn.Release()
		return false, nil, nil // another replica has it; not an error
	}
	return true, func() {
		// Unlock on the SAME connection, and outside the caller's cancelled
		// context — a cancelled cleanup would leak the lock until the
		// connection is recycled.
		if _, _, err := platformrquery.AdvisoryUnlock.One(context.WithoutCancel(ctx), ex, id); err != nil {
			slog.Warn("advisory unlock failed", "lock", id, "error", err)
		}
		conn.Release()
	}, nil
}
