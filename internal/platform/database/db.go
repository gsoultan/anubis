// Package database is the shared Postgres plumbing every context adapter
// builds on: one pool, one transaction mechanism, one error-mapping table.
// It holds no queries of its own — each bounded context owns its SQL in
// db/queries/<context> and its generated package (ADR-0009).
package database

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gsoultan/anubis/internal/shared/apperr"
)

// Querier is what the sqlc-generated packages need; both a pool and a
// transaction satisfy it, which is how WithinTx stays invisible to callers.
type Querier interface {
	Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error)
	Query(context.Context, string, ...interface{}) (pgx.Rows, error)
	QueryRow(context.Context, string, ...interface{}) pgx.Row
}

// DB owns the pool and the ambient-transaction convention.
type DB struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *DB { return &DB{pool: pool} }

// Pool exposes the raw pool for the few callers that need their own
// transaction options (the snapshot loader's REPEATABLE READ).
func (d *DB) Pool() *pgxpool.Pool { return d.pool }

// txKey carries an open transaction through context so every repository
// method transparently joins WithinTx.
type txKey struct{}

// WithinTx implements txm.TxManager.
func (d *DB) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return fn(ctx) // nested WithinTx joins the outer transaction
	}
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return apperr.ErrInternal.Wrap(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

	if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return apperr.ErrInternal.Wrap(err)
	}
	return nil
}

// Conn returns the ambient transaction when one is open, else the pool.
func (d *DB) Conn(ctx context.Context) Querier {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return d.pool
}
