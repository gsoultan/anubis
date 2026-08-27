package authzpg

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	// The generated package's init registers this context's raw-row scanners;
	// the blank import is what makes the rquery declarations executable.
	_ "github.com/gsoultan/anubis/internal/authz/adapter/postgres/rgen"
	"github.com/gsoultan/storm"
	"github.com/gsoultan/storm/runtime"
	"github.com/gsoultan/storm/runtime/pgxdrv"
)

// rex adapts the ambient connection to storm's executor port. database.Conn
// has exactly two producers — the ambient transaction or the pool — and both
// concrete types already satisfy storm's pgx adapter, so no anubis-side Rows
// shim exists and transactional calls stay transactional for free.
func (s *Repository) rex(ctx context.Context) runtime.Executor {
	switch c := s.Conn(ctx).(type) {
	case pgx.Tx:
		return pgxdrv.Tx{T: c}
	case *pgxpool.Pool:
		return pgxdrv.Pool{P: c}
	default:
		// Unreachable while Conn keeps its two branches. Fail closed per
		// call rather than panic, so a future refactor of database.Conn
		// degrades to explicit errors instead of a crash.
		return errExecutor{fmt.Errorf("authzpg: no storm adapter for %T", c)}
	}
}

type errExecutor struct{ err error }

func (e errExecutor) Query(context.Context, string, []any) (runtime.Rows, error) {
	return nil, e.err
}
func (e errExecutor) Exec(context.Context, string, []any) (int64, error) { return 0, e.err }
func (e errExecutor) CopyFrom(context.Context, string, []string, runtime.CopySource) (int64, error) {
	return 0, e.err
}
func (e errExecutor) Batch(context.Context, []runtime.BatchOp, func(int, runtime.Rows, int64, error) error) error {
	return e.err
}

// The domain layer speaks string ids end to end — a sqlc-era contract this
// migration preserves (rquery casts uuid columns ::text for the same reason).
// storm builders speak [16]byte, and these two functions are the one crossing.

func parseUUID(s string) ([16]byte, error) {
	var u [16]byte
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return u, fmt.Errorf("authzpg: %q is not a canonical uuid", s)
	}
	var raw [32]byte
	n := 0
	for i := 0; i < len(s); i++ {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue
		}
		raw[n] = s[i]
		n++
	}
	if _, err := hex.Decode(u[:], raw[:]); err != nil {
		return u, fmt.Errorf("authzpg: %q is not a canonical uuid", s)
	}
	return u, nil
}

func uuidStr(u [16]byte) string { return storm.UUID(u).String() }
