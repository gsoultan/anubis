//go:build integration

// Package memberships holds memberships (0054) to authorize() rather than to
// the rows they write. Its own package because test/integration is at the
// ten-file limit, and each test here builds and rolls back its own world.
//
//	ANUBIS_DB_URL=postgres://anubis:anubis@localhost:7449/anubis?sslmode=disable \
//	  go test -tags integration ./test/integration/memberships/
package memberships

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

var pool *pgxpool.Pool

var errRollback = errors.New("deliberate rollback")

func TestMain(m *testing.M) {
	dsn := os.Getenv("ANUBIS_DB_URL")
	if dsn == "" {
		os.Exit(0) // nothing to test against; the e2e job sets this
	}
	p, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		panic(err)
	}
	pool = p
	code := m.Run()
	p.Close()
	os.Exit(code)
}

func skipWithoutDB(t *testing.T) {
	t.Helper()
	if pool == nil {
		t.Skip("ANUBIS_DB_URL not set")
	}
}

func firstTenant(ctx context.Context, t *testing.T) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `SELECT id FROM tenants ORDER BY created_at LIMIT 1`).Scan(&id); err != nil {
		t.Fatalf("no tenant — bootstrap creates one: %v", err)
	}
	return id
}
