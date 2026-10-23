//go:build integration

// Package accesschanges holds the console redesign's server half (0057) to a
// real database: bulk changes to grants and their undo, judged by
// authorize() rather than by the rows written; the insight queries; an app
// closed to role holders; and a group of companies.
//
// Every test builds its own world in a transaction that is rolled back.
//
//	ANUBIS_DB_URL=postgres://anubis:anubis@localhost:7449/anubis?sslmode=disable \
//	  go test -tags integration ./test/integration/accesschanges/
package accesschanges

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
