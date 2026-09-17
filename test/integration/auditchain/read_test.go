//go:build integration

package auditchain

import (
	"context"
	"testing"
	"time"

	auditpg "github.com/gsoultan/anubis/internal/audit/adapter/postgres"
	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
)

// The audit read paths, which the chain tests do not reach: the console's
// search, the two dashboard aggregates, and partition provisioning.
//
// They are here because they moved from sqlc to storm builders, and a query
// that compiles proves nothing about the rows it returns. Every filter is
// asserted to SELECT and to EXCLUDE — a predicate that quietly stopped
// applying returns a superset, which looks like a working search until
// somebody's audit log shows another actor's entries.

func repoFor(t *testing.T) *auditpg.Repository {
	t.Helper()
	return auditpg.New(database.New(pool))
}

func TestQueryAuditFiltersEachWay(t *testing.T) {
	skipWithoutDB(t)
	freshChain(t)
	ctx := context.Background()

	a := auditorFor(t)
	actor := "11111111-1111-1111-1111-111111111111"
	other := "22222222-2222-2222-2222-222222222222"
	for _, ev := range []auditdomain.AuditEvent{
		{TenantID: probeTenant, Action: "login", Result: "allow", ActorID: actor, IP: "203.0.113.7"},
		{TenantID: probeTenant, Action: "login", Result: "deny", ActorID: other},
		{TenantID: probeTenant, Action: "authorize", Result: "allow", ActorID: actor},
	} {
		a.Emit(ctx, ev)
	}
	a.Close()

	repo := repoFor(t)

	all, err := repo.QueryAudit(ctx, probeTenant, auditdomain.AuditQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("unfiltered search returned %d rows, want 3", len(all))
	}

	// Newest first: the console pages backwards through seq.
	if all[0].Seq < all[len(all)-1].Seq {
		t.Fatalf("rows are not newest-first: %d then %d", all[0].Seq, all[len(all)-1].Seq)
	}

	byActor, err := repo.QueryAudit(ctx, probeTenant, auditdomain.AuditQuery{ActorID: actor})
	if err != nil {
		t.Fatal(err)
	}
	if len(byActor) != 2 {
		t.Fatalf("actor filter returned %d rows, want 2", len(byActor))
	}
	for _, r := range byActor {
		if r.ActorID != actor {
			t.Fatalf("actor filter leaked %s", r.ActorID)
		}
	}

	byAction, err := repo.QueryAudit(ctx, probeTenant, auditdomain.AuditQuery{Action: "login"})
	if err != nil {
		t.Fatal(err)
	}
	if len(byAction) != 2 {
		t.Fatalf("action filter returned %d rows, want 2", len(byAction))
	}

	// Both at once — the combination is a different compiled shape, and a
	// builder that dropped one predicate would still pass the tests above.
	both, err := repo.QueryAudit(ctx, probeTenant,
		auditdomain.AuditQuery{ActorID: actor, Action: "login"})
	if err != nil {
		t.Fatal(err)
	}
	if len(both) != 1 {
		t.Fatalf("actor+action returned %d rows, want 1", len(both))
	}
	if both[0].IP != "203.0.113.7" {
		t.Fatalf("inet came back as %q, want the bare address", both[0].IP)
	}

	// A filter that matches nothing must return nothing, or "it worked" and
	// "it never applied" are the same observation.
	none, err := repo.QueryAudit(ctx, probeTenant, auditdomain.AuditQuery{Action: "no.such.action"})
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Fatalf("a filter matching nothing returned %d rows", len(none))
	}
}

func TestQueryAuditRespectsTimeAndCursor(t *testing.T) {
	skipWithoutDB(t)
	freshChain(t)
	ctx := context.Background()

	a := auditorFor(t)
	for i := 0; i < 5; i++ {
		a.Emit(ctx, auditdomain.AuditEvent{
			TenantID: probeTenant, Action: "paged", Result: "allow",
		})
	}
	a.Close()
	repo := repoFor(t)

	all, err := repo.QueryAudit(ctx, probeTenant, auditdomain.AuditQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(all))
	}

	cursor := all[0].Seq
	page, err := repo.QueryAudit(ctx, probeTenant, auditdomain.AuditQuery{BeforeSeq: &cursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 4 {
		t.Fatalf("before_seq returned %d rows, want 4", len(page))
	}
	for _, r := range page {
		if r.Seq >= cursor {
			t.Fatalf("before_seq is not strict: got seq %d with cursor %d", r.Seq, cursor)
		}
	}

	future := time.Now().Add(time.Hour)
	empty, err := repo.QueryAudit(ctx, probeTenant, auditdomain.AuditQuery{From: &future})
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("a from-filter in the future returned %d rows", len(empty))
	}

	past := time.Now().Add(-time.Hour)
	recent, err := repo.QueryAudit(ctx, probeTenant, auditdomain.AuditQuery{From: &past})
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 5 {
		t.Fatalf("a from-filter an hour ago returned %d rows, want 5", len(recent))
	}
}

func TestDashboardAggregates(t *testing.T) {
	skipWithoutDB(t)
	freshChain(t)
	ctx := context.Background()

	a := auditorFor(t)
	for i := 0; i < 3; i++ {
		a.Emit(ctx, auditdomain.AuditEvent{TenantID: probeTenant, Action: "authorize", Result: "allow"})
	}
	a.Emit(ctx, auditdomain.AuditEvent{TenantID: probeTenant, Action: "authorize", Result: "deny"})
	a.Emit(ctx, auditdomain.AuditEvent{TenantID: probeTenant, Action: "token.reuse_detected", Result: "deny"})
	a.Close()

	repo := repoFor(t)
	allows, denies, err := repo.CountDecisions24h(ctx, probeTenant)
	if err != nil {
		t.Fatal(err)
	}
	// FILTER splits one scan two ways; a lowering that lost a FILTER would
	// report the same number twice.
	if allows != 3 || denies != 1 {
		t.Fatalf("CountDecisions24h = (%d allow, %d deny), want (3, 1)", allows, denies)
	}

	n, latest, err := repo.ReuseSignal(ctx, probeTenant)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("ReuseSignal counted %d, want 1", n)
	}
	if time.Since(latest) > time.Hour {
		t.Fatalf("ReuseSignal latest = %s, which is not the row just written", latest)
	}
}

// The zero case, which COALESCE exists for: max() over no rows is NULL, and a
// tenant that has never had a reuse event is the ordinary case.
func TestReuseSignalWithNoEvents(t *testing.T) {
	skipWithoutDB(t)
	freshChain(t)
	n, _, err := repoFor(t).ReuseSignal(context.Background(), probeTenant)
	if err != nil {
		t.Fatalf("ReuseSignal over an empty window failed: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected no reuse events, got %d", n)
	}
}

// EnsurePartitions is idempotent by construction — it is run on a schedule,
// so the second call is the normal one.
func TestEnsurePartitionsIsIdempotent(t *testing.T) {
	skipWithoutDB(t)
	ctx := context.Background()
	repo := repoFor(t)
	before := partitionCount(t)
	for i := 0; i < 2; i++ {
		if err := repo.EnsurePartitions(ctx); err != nil {
			t.Fatalf("EnsurePartitions call %d: %v", i+1, err)
		}
	}
	if after := partitionCount(t); after < before {
		t.Fatalf("partitions went from %d to %d", before, after)
	}
}

func partitionCount(t *testing.T) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM pg_class c JOIN pg_namespace ns ON ns.oid = c.relnamespace
		 WHERE ns.nspname = 'public' AND c.relispartition`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
