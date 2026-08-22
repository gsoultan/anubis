// Package migrate is the embedded, hand-written migration runner
// (docs/roadmap.md phase 0: "the migration runner comes first").
//
// Contract shared with scripts/db.sh — the two must never diverge:
//   - tracked in schema_migrations(version text PK, checksum text, applied_at)
//   - version = filename without .sql, applied in filename order
//   - checksum = hex sha256 of the file bytes; drift after apply is an error
//     surfaced loudly (the file changed after it ran)
//   - forward-only; no down migrations (a rollback on an auth database is a
//     data-loss event; roll forward instead)
//   - the whole run holds pg_advisory_lock so two replicas cannot race
package migrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

// advisoryLockKey is arbitrary but fixed: one lock guards schema evolution.
const advisoryLockKey = 0x616e7562 // "anub"

// ErrNeedsBaseline: the schema exists but nothing is tracked. Re-applying
// would fail on objects that already exist, so the operator must say
// explicitly whether this database is already at head (`anubisd baseline`,
// scripts/db.sh baseline) or should be rebuilt (scripts/db.sh reset).
var ErrNeedsBaseline = errors.New(
	"migrate: schema exists but no migrations are recorded — run `anubisd baseline` if it is already at head, or scripts/db.sh reset to rebuild")

// Runner applies embedded migrations over a single connection.
type Runner struct {
	fsys   fs.FS
	logger *slog.Logger
}

func NewRunner(fsys fs.FS, logger *slog.Logger) *Runner {
	return &Runner{fsys: fsys, logger: logger}
}

// Result reports what one run did.
type Result struct {
	Applied []string
	Skipped int
	Drifted []string
}

// Run applies all pending migrations. conn must be a dedicated connection
// (the advisory lock is connection-scoped).
func (r *Runner) Run(ctx context.Context, conn *pgx.Conn) (*Result, error) {
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", advisoryLockKey); err != nil {
		return nil, fmt.Errorf("migrate: advisory lock: %w", err)
	}
	defer conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", advisoryLockKey) //nolint:errcheck

	files, err := r.files()
	if err != nil {
		return nil, err
	}
	applied, err := r.appliedVersions(ctx, conn)
	if err != nil {
		return nil, err
	}
	// A database whose schema exists but whose tracking is empty predates
	// this runner, or was rebuilt by bench/rebuild.sh. Re-applying 0001 would
	// fail on "relation already exists"; demand an explicit baseline instead
	// of guessing — the same call scripts/db.sh makes.
	if len(applied) == 0 {
		n, terr := publicTableCount(ctx, conn)
		if terr == nil && n > 1 {
			return nil, ErrNeedsBaseline
		}
	}

	res := &Result{}
	for _, name := range files {
		version := strings.TrimSuffix(name, ".sql")
		raw, err := fs.ReadFile(r.fsys, name)
		if err != nil {
			return nil, fmt.Errorf("migrate: read %s: %w", name, err)
		}
		sum := sha256.Sum256(raw)
		checksum := hex.EncodeToString(sum[:])

		if seen, ok := applied[version]; ok {
			if seen != checksum {
				res.Drifted = append(res.Drifted, version)
				r.logger.Error("migration modified after being applied (checksum drift)",
					"version", version)
			}
			res.Skipped++
			continue
		}

		// One migration = one transaction: a failed step leaves no half-state.
		tx, err := conn.Begin(ctx)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, string(raw)); err != nil {
			_ = tx.Rollback(ctx)
			return res, fmt.Errorf("migrate: %s failed: %w", version, err)
		}
		if _, err := tx.Exec(ctx,
			"INSERT INTO schema_migrations (version, checksum) VALUES ($1, $2) ON CONFLICT (version) DO NOTHING",
			version, checksum); err != nil {
			_ = tx.Rollback(ctx)
			return res, fmt.Errorf("migrate: record %s: %w", version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return res, err
		}
		r.logger.Info("migration applied", "version", version)
		res.Applied = append(res.Applied, version)
	}
	return res, nil
}

func (r *Runner) files() ([]string, error) {
	entries, err := fs.Glob(r.fsys, "*.sql")
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, errors.New("migrate: no migrations embedded")
	}
	sort.Strings(entries)
	return entries, nil
}

// Baseline records every embedded migration as applied WITHOUT running any
// of them. For adopting an existing database (or one rebuilt by
// bench/rebuild.sh), never for a fresh one.
func (r *Runner) Baseline(ctx context.Context, conn *pgx.Conn) (int, error) {
	files, err := r.files()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, name := range files {
		raw, err := fs.ReadFile(r.fsys, name)
		if err != nil {
			return n, err
		}
		sum := sha256.Sum256(raw)
		if _, err := conn.Exec(ctx,
			"INSERT INTO schema_migrations (version, checksum) VALUES ($1, $2) ON CONFLICT (version) DO NOTHING",
			strings.TrimSuffix(name, ".sql"), hex.EncodeToString(sum[:])); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func publicTableCount(ctx context.Context, conn *pgx.Conn) (int, error) {
	var n int
	err := conn.QueryRow(ctx, "SELECT count(*) FROM pg_tables WHERE schemaname='public'").Scan(&n)
	return n, err
}

// appliedVersions tolerates the pre-schema case: 0001 creates
// schema_migrations, so a fresh database has neither the table nor rows —
// but a database migrated by scripts/db.sh has both and must not re-run.
func (r *Runner) appliedVersions(ctx context.Context, conn *pgx.Conn) (map[string]string, error) {
	out := map[string]string{}
	rows, err := conn.Query(ctx, "SELECT version, checksum FROM schema_migrations")
	if err != nil {
		if isUndefinedTable(err) {
			if n, terr := publicTableCount(ctx, conn); terr == nil && n > 1 {
				return nil, ErrNeedsBaseline
			}
			return out, nil
		}
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var v, c string
		if err := rows.Scan(&v, &c); err != nil {
			return nil, err
		}
		out[v] = c
	}
	return out, rows.Err()
}

func isUndefinedTable(err error) bool {
	return err != nil && strings.Contains(err.Error(), "42P01")
}
