package database

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/gsoultan/storm"
	"github.com/gsoultan/storm/runtime"
	"github.com/gsoultan/storm/runtime/pgxdrv"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Executor adapts the ambient connection to storm's executor port.
//
// Conn has exactly two producers — the ambient transaction or the pool — and
// both concrete types already satisfy storm's pgx adapter, so there is no
// anubis-side Rows shim and a transactional call stays transactional for free.
//
// It lives here rather than in each context's adapter because it is plumbing,
// not policy: nine copies of this switch is nine places for one of them to
// keep an older branch when database.Conn grows a third producer.
func Executor(c Querier) runtime.Executor {
	switch t := c.(type) {
	case pgx.Tx:
		return pgxdrv.Tx{T: t}
	case *pgxpool.Pool:
		return pgxdrv.Pool{P: t}
	default:
		// Unreachable while Conn keeps its two branches. Failing closed per
		// call rather than panicking means a future refactor of Conn degrades
		// to explicit errors instead of a crash.
		return errExecutor{fmt.Errorf("database: no storm adapter for %T", c)}
	}
}

// StormExec returns the executor for this call's connection.
func (d *DB) StormExec(ctx context.Context) runtime.Executor { return Executor(d.Conn(ctx)) }

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

// ParseUUID reads a canonical uuid into storm's 16-byte form.
//
// The domain carries identifiers as strings and storm's columns are [16]byte,
// so every adapter crossing that boundary needs this. It rejects anything that
// is not exactly canonical rather than accepting the several forms PostgreSQL
// would: a value that reaches a query having silently changed shape is worse
// than one that fails to.
func ParseUUID(s string) ([16]byte, error) {
	var u [16]byte
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return u, fmt.Errorf("database: %q is not a canonical uuid", s)
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
		return u, fmt.Errorf("database: %q is not a uuid: %w", s, err)
	}
	return u, nil
}

// UUIDStr renders storm's uuid back as the canonical string the domain uses.
func UUIDStr(u [16]byte) string { return storm.UUID(u).String() }
