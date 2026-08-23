//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"sort"
	"testing"
	"time"
)

// BUDGET (docs/benchmarks.md measured 0.045 ms in-database): the decision
// through pgx must stay under 2 ms at p95. The gap between the two numbers is
// the round trip, not the engine — so a regression here means we added work
// around the query, which is exactly the thing ADR-0005 §4 warns about
// ("benchmark the decision, not the subquery").
func TestAuthorizeLatencyBudget(t *testing.T) {
	skipWithoutDB(t)
	if testing.Short() {
		t.Skip("latency budget")
	}
	ctx := context.Background()
	tenant := firstTenant(ctx, t)
	identity, permission, targets := realDecisionProbe(ctx, t, tenant)

	const (
		warmup  = 50
		samples = 500
		budget  = 2 * time.Millisecond
	)
	raw, _ := json.Marshal(targets)

	for i := 0; i < warmup; i++ {
		var allow bool
		if err := pool.QueryRow(ctx, `SELECT authorize($1,$2,$3,$4::jsonb)`,
			identity, tenant, permission, string(raw)).Scan(&allow); err != nil {
			t.Fatal(err)
		}
	}

	lat := make([]time.Duration, 0, samples)
	for i := 0; i < samples; i++ {
		start := time.Now()
		var allow bool
		if err := pool.QueryRow(ctx, `SELECT authorize($1,$2,$3,$4::jsonb)`,
			identity, tenant, permission, string(raw)).Scan(&allow); err != nil {
			t.Fatal(err)
		}
		lat = append(lat, time.Since(start))
	}
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	p50, p95, p99 := lat[len(lat)/2], lat[(len(lat)*95)/100], lat[(len(lat)*99)/100]
	t.Logf("authorize over pgx: p50=%v p95=%v p99=%v (n=%d)", p50, p95, p99, samples)

	if p95 > budget {
		t.Fatalf("p95 %v exceeds the %v budget — the decision path regressed", p95, budget)
	}
}

// realDecisionProbe finds a grant that actually exists and builds the target
// map from that grant's OWN constraints. Development.md learned this the hard
// way: a probe that supplies only `org` while the grant also constrains
// `product` produces a correct fail-closed deny that looks like a regression.
func realDecisionProbe(ctx context.Context, t *testing.T, tenant string) (string, string, map[string]string) {
	t.Helper()
	var identity, permission string
	err := pool.QueryRow(ctx, `
		SELECT g.identity_id, p.key
		  FROM grants g
		  JOIN role_permissions_effective rpe ON rpe.role_id = g.role_id
		  JOIN permissions p ON p.id = rpe.permission_id
		 WHERE g.tenant_id = $1 AND g.revoked_at IS NULL AND p.deprecated_at IS NULL
		 LIMIT 1`, tenant).Scan(&identity, &permission)
	if err != nil {
		t.Skipf("no grant to measure: %v", err)
	}
	targets := map[string]string{}
	rows, err := pool.Query(ctx, `
		SELECT gs.axis_code, gs.scope_node_id
		  FROM grants g JOIN grant_scopes gs ON gs.grant_id = g.id
		 WHERE g.identity_id = $1 AND g.tenant_id = $2 AND g.revoked_at IS NULL`,
		identity, tenant)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var axis, node string
		if err := rows.Scan(&axis, &node); err != nil {
			t.Fatal(err)
		}
		targets[axis] = node
	}
	return identity, permission, targets
}

// BenchmarkAuthorizeEngine is the benchstat-comparable form of the same
// measurement, for tracking drift release over release.
func BenchmarkAuthorizeEngine(b *testing.B) {
	if pool == nil {
		b.Skip("ANUBIS_DB_URL not set")
	}
	ctx := context.Background()
	var tenant string
	if err := pool.QueryRow(ctx, `SELECT id FROM tenants ORDER BY created_at LIMIT 1`).Scan(&tenant); err != nil {
		b.Skip("no tenant")
	}
	var identity, permission string
	if err := pool.QueryRow(ctx, `
		SELECT g.identity_id, p.key FROM grants g
		  JOIN role_permissions_effective rpe ON rpe.role_id = g.role_id
		  JOIN permissions p ON p.id = rpe.permission_id
		 WHERE g.tenant_id=$1 AND g.revoked_at IS NULL LIMIT 1`, tenant).Scan(&identity, &permission); err != nil {
		b.Skip("no grant")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var allow bool
		if err := pool.QueryRow(ctx, `SELECT authorize($1,$2,$3,'{}'::jsonb)`,
			identity, tenant, permission).Scan(&allow); err != nil {
			b.Fatal(err)
		}
	}
}
