package main

import (
	"strings"
	"testing"
)

func TestFindsCleanupsThatCannotRun(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want []string // substrings, one per expected finding
	}{
		{
			// reset_test.go as it was: every run left its operator behind.
			name: "deferred close before a cleanup that needs the pool",
			body: `
	pool, err := pgxpool.New(ctx, dsn)
	defer pool.Close()
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, q, id)
	})`,
			want: []string{"defer pool.Close()", "discards an Exec error"},
		},
		{
			name: "close registered first, failure reported",
			body: `
	pool, err := pgxpool.New(ctx, dsn)
	t.Cleanup(pool.Close)
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, q, id); err != nil {
			t.Errorf("not removed: %v", err)
		}
	})`,
		},
		{
			name: "an Exec called as a bare statement discards its error too",
			body: `
	t.Cleanup(func() {
		pool.Exec(ctx, q, id)
	})`,
			want: []string{"discards an Exec error"},
		},
		{
			name: "sql.Open is a pool as well",
			body: `
	db, err := sql.Open("pgx", dsn)
	defer db.Close()`,
			want: []string{"defer db.Close()"},
		},
		{
			// A defer inside a closure runs when the closure returns.
			name: "a defer inside a cleanup closure is fine",
			body: `
	t.Cleanup(func() {
		db, err := sql.Open("pgx", dsn)
		if err != nil {
			return
		}
		defer db.Close()
		if _, err := db.ExecContext(ctx, q, id); err != nil {
			t.Errorf("not removed: %v", err)
		}
	})`,
		},
		{
			name: "a subtest closure has the same bug as a test",
			body: `
	t.Run("sub", func(t *testing.T) {
		pool, err := pgxpool.New(ctx, dsn)
		defer pool.Close()
	})`,
			want: []string{"defer pool.Close()"},
		},
		{
			name: "closing something that is not a pool is not this bug",
			body: `
	rows, err := pool.Query(ctx, q)
	defer rows.Close()`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "package p\n\nfunc TestX(t *testing.T) {" + tc.body + "\n}\n"
			got, err := checkSource("x_test.go", src)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %d findings, want %d:\n%s", len(got), len(tc.want), strings.Join(got, "\n"))
			}
			for i, w := range tc.want {
				if !strings.Contains(got[i], w) {
					t.Errorf("finding %d = %q, want it to mention %q", i, got[i], w)
				}
			}
		})
	}
}
