package recordshares

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

var pool *pgxpool.Pool

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
