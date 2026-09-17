//go:build integration

package authtokens

import (
	"context"
	"testing"
	"time"

	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
)

// The request-path reads this context owns, measured through storm.
//
// These exist because the budget that guards the decision —
// TestAuthorizeLatencyBudget — probes `SELECT authorize(...)` through the pool
// DIRECTLY. It measures the SQL engine and the round trip, which the migration
// to storm did not touch, and says nothing about the repository code that did
// change. Nothing measured the reads on the way IN: every authenticated
// request resolves a session, and a cookie-bearing one resolves it by hash.
//
// There is no before-number to diff against, because the sqlc code they
// replaced is deleted. What these are is a BASELINE: run them before and after
// a change to the auth adapter, and `benchstat` has something to compare.
//
//	ANUBIS_DB_URL=... go test -tags integration -run '^$' -bench . -benchmem \
//	  ./test/integration/authtokens/

func benchSetup(b *testing.B) (context.Context, string, []byte) {
	b.Helper()
	if pool == nil {
		b.Skip("ANUBIS_DB_URL not set")
	}
	ctx := context.Background()
	r := repoForBench(b)
	s, err := r.CreateSession(ctx, authdomain.SessionInput{
		IdentityID: identity, TenantID: tenant,
		AMR: []string{"pwd"}, IP: "203.0.113.9", UserAgent: "bench/1",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		b.Fatal(err)
	}
	ck := make([]byte, 32)
	for i := range ck {
		ck[i] = byte(i + 1)
	}
	if err := r.SetSessionCookieHash(ctx, s.ID, ck); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM sessions WHERE id = $1`, s.ID)
	})
	return ctx, s.ID, ck
}

// SessionLive is on EVERY authenticated request: it resolves the session and
// re-reads the identity's epoch and status in one query, so that disabling
// somebody takes their live sessions with them.
func BenchmarkSessionLive(b *testing.B) {
	ctx, id, _ := benchSetup(b)
	r := repoForBench(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := r.SessionLive(ctx, id); err != nil {
			b.Fatal(err)
		}
	}
}

// SessionByCookieHash is the browser path's equivalent, and additionally
// exercises a bytea predicate — which storm has none of, so it is one of the
// raw declarations.
func BenchmarkSessionByCookieHash(b *testing.B) {
	ctx, _, ck := benchSetup(b)
	r := repoForBench(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := r.SessionByCookieHash(ctx, ck); err != nil {
			b.Fatal(err)
		}
	}
}

// SessionState is what token introspection calls, and the cheapest of the
// three — it reads six columns and joins one table.
func BenchmarkSessionState(b *testing.B) {
	ctx, id, _ := benchSetup(b)
	r := repoForBench(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, _, _, err := r.SessionState(ctx, tenant, id); err != nil {
			b.Fatal(err)
		}
	}
}
