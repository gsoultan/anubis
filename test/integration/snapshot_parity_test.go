//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	gatepg "github.com/gsoultan/anubis/internal/gate/adapter/postgres"
	"github.com/gsoultan/anubis/internal/gate/snapshot"
	"github.com/gsoultan/anubis/internal/platform/database"
)

// THE test for the gate. The gate answers from an in-memory snapshot; the
// decision endpoint answers from authorize() in SQL. Two implementations of
// one rule set is a standing invitation to drift, and drift here is not a
// wrong number on a dashboard — it is either access granted that SQL would
// deny, or a deny storm on a path SQL allows.
//
// So: load a real snapshot, replay real grants through both, and require
// EXACT agreement on every probe. math/rand/v2 is fine here — this is test
// sampling, not security (the repository-wide ban is about crypto).
func TestSnapshotAgreesWithAuthorizeEngine(t *testing.T) {
	skipWithoutDB(t)
	ctx := context.Background()
	tenant, slug := firstTenantWithSlug(ctx, t)

	repo := gatepg.New(database.New(pool))
	snap, err := repo.LoadSnapshot(ctx, tenant, slug, 30*time.Minute)
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if len(snap.GrantsByIdentity) == 0 {
		t.Skip("no grants in this database to compare")
	}

	type probe struct {
		identity   string
		permission string
		targets    map[string]string
	}
	var probes []probe

	// Probes built from REAL grants: the permissions those grants confer and
	// the nodes they are scoped to, plus deliberate near-misses (a sibling
	// node, a missing axis) because agreement on allows proves little.
	for identity, grants := range snap.GrantsByIdentity {
		for _, g := range grants {
			var perm string
			for key := range snap.RolePermissions[g.RoleID] {
				perm = key
				break
			}
			if perm == "" {
				continue
			}
			exact := map[string]string{}
			for axis, cs := range g.Scopes {
				if len(cs) > 0 {
					exact[axis] = cs[0].NodeID
				}
			}
			probes = append(probes, probe{identity, perm, exact})

			// same permission, no targets at all: exercises fail-closed
			probes = append(probes, probe{identity, perm, map[string]string{}})

			// same permission, a descendant target: exercises inheritance
			for axis, cs := range g.Scopes {
				if len(cs) == 0 {
					continue
				}
				if child := someDescendant(ctx, cs[0].NodeID); child != "" {
					deep := map[string]string{}
					for a, c := range exact {
						deep[a] = c
					}
					deep[axis] = child
					probes = append(probes, probe{identity, perm, deep})
				}
				break
			}
			// a permission this grant does NOT confer
			probes = append(probes, probe{identity, "nonexistent:probe:action", exact})

			if len(probes) > 400 {
				break
			}
		}
		if len(probes) > 400 {
			break
		}
	}
	rand.Shuffle(len(probes), func(i, j int) { probes[i], probes[j] = probes[j], probes[i] })

	now := time.Now()
	var disagreements int
	for _, p := range probes {
		targets, _ := json.Marshal(p.targets)
		var sqlAnswer bool
		if err := pool.QueryRow(ctx, `SELECT authorize($1,$2,$3,$4::jsonb)`,
			p.identity, tenant, p.permission, string(targets)).Scan(&sqlAnswer); err != nil {
			t.Fatalf("engine: %v", err)
		}
		memAnswer := snap.Evaluate(p.identity, p.permission, p.targets, now)
		if sqlAnswer != memAnswer {
			disagreements++
			if disagreements <= 5 {
				t.Errorf("DISAGREEMENT sub=%s perm=%s targets=%s: engine=%v snapshot=%v",
					p.identity, p.permission, targets, sqlAnswer, memAnswer)
			}
		}
	}
	if disagreements > 0 {
		t.Fatalf("%d/%d probes disagreed between the SQL engine and the gate snapshot",
			disagreements, len(probes))
	}
	t.Logf("%d probes, 0 disagreements", len(probes))
}

func someDescendant(ctx context.Context, node string) string {
	var id string
	err := pool.QueryRow(ctx,
		`SELECT descendant_id FROM scope_closure
		  WHERE ancestor_id=$1 AND depth > 0 LIMIT 1`, node).Scan(&id)
	if err != nil {
		return ""
	}
	return id
}

func firstTenantWithSlug(ctx context.Context, t *testing.T) (string, string) {
	t.Helper()
	var id, slug string
	if err := pool.QueryRow(ctx,
		`SELECT id, slug FROM tenants ORDER BY created_at LIMIT 1`).Scan(&id, &slug); err != nil {
		t.Skipf("no tenant: %v", err)
	}
	return id, slug
}

var _ = fmt.Sprintf

// The gate loads the hierarchy as parent pointers (SnapshotNodes) while
// authorize() probes scope_closure. Neither looks at scope_nodes.status, so
// an archived node still carries grants and still links its children to their
// ancestors. Adding "AND status = 'active'" to SnapshotNodes is the obvious
// tidy-up and it is WRONG: it would drop the archived node from the index,
// break every chain that passes through it, and make the gate deny what the
// SQL engine allows. That divergence would show up as a partial outage for
// one subtree, which is exactly the kind of thing nobody attributes to a
// scope archive three weeks earlier.
//
// This pins the behaviour on both sides at once.
func TestArchivedNodeStaysInSnapshotHierarchy(t *testing.T) {
	skipWithoutDB(t)
	ctx := context.Background()
	tenant, slug := firstTenantWithSlug(ctx, t)

	const axis = "archivetest"
	exec := func(sql string, args ...any) {
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture %q: %v", sql, err)
		}
	}
	exec(`INSERT INTO scope_axes (code, display_name) VALUES ($1,'Archive Test')
	      ON CONFLICT (code) DO NOTHING`, axis)
	exec(`INSERT INTO scope_node_types (code, axis_code, display_name, parent_types)
	      VALUES ('ar_root',$1,'Root','{}') ON CONFLICT (code) DO NOTHING`, axis)
	exec(`INSERT INTO scope_node_types (code, axis_code, display_name, parent_types)
	      VALUES ('ar_node',$1,'Node','{ar_root,ar_node}') ON CONFLICT (code) DO NOTHING`, axis)

	var root string
	if err := pool.QueryRow(ctx, `SELECT scope_ensure_root($1,$2)`, tenant, axis).Scan(&root); err != nil {
		t.Fatal(err)
	}
	add := func(slug, parent string) string {
		var id string
		if err := pool.QueryRow(ctx,
			`SELECT COALESCE((SELECT id FROM scope_nodes WHERE tenant_id=$1 AND axis_code=$2 AND slug=$3),
			                 scope_add_node($1,$2,'ar_node',$4,$3,$3,NULL))`,
			tenant, axis, slug, parent).Scan(&id); err != nil {
			t.Fatalf("add %s: %v", slug, err)
		}
		return id
	}
	middle := add("ar-middle", root)
	child := add("ar-child", middle)

	// Archive the INTERMEDIATE node — the case that breaks a chain, not a leaf.
	exec(`UPDATE scope_nodes SET status='archived' WHERE id=$1`, middle)
	t.Cleanup(func() {
		if _, err := pool.Exec(context.WithoutCancel(ctx),
			`UPDATE scope_nodes SET status='active' WHERE id=$1`, middle); err != nil {
			t.Logf("restore archived node: %v", err)
		}
	})

	// SQL side: the closure still says middle is an ancestor of child.
	var inClosure bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM scope_closure WHERE ancestor_id=$1 AND descendant_id=$2)`,
		middle, child).Scan(&inClosure); err != nil {
		t.Fatal(err)
	}
	if !inClosure {
		t.Fatal("precondition: closure lost the archived ancestor, so there is nothing to compare")
	}

	// Snapshot side: it must reach the same conclusion.
	repo := gatepg.New(database.New(pool))
	snap, err := repo.LoadSnapshot(ctx, tenant, slug, 30*time.Minute)
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if _, ok := snap.Scope.Resolve(middle); !ok {
		t.Error("archived node is missing from the snapshot index — SnapshotNodes is filtering on status")
	}
	target, ok := snap.Scope.Resolve(child)
	if !ok {
		t.Fatal("child node missing from the snapshot index")
	}
	cs := []snapshot.ScopeConstraint{{NodeID: middle, Inherit: true}}
	snap.Scope.Intern(cs)
	if !snap.Scope.CoveredBy(target, cs) {
		t.Error("snapshot says the archived ancestor does not cover its child; authorize() says it does")
	}
}

// What the snapshot actually costs in memory, measured on whatever is in the
// database. The gate holds one of these per tenant, permanently, on every
// instance — so this number, not the row count, is what decides how large a
// scope hierarchy a deployment can carry.
//
// Run it against a big database to size a deployment:
//
//	ANUBIS_DB_URL=... go test -tags integration ./test/integration \
//	    -run xxx -bench BenchmarkSnapshotMemory -benchtime 1x
func BenchmarkSnapshotMemory(b *testing.B) {
	if pool == nil {
		b.Skip("ANUBIS_DB_URL not set")
	}
	ctx := context.Background()
	var tenant, slug string
	if err := pool.QueryRow(ctx,
		`SELECT id, slug FROM tenants ORDER BY created_at LIMIT 1`).Scan(&tenant, &slug); err != nil {
		b.Skipf("no tenant seeded: %v", err)
	}
	repo := gatepg.New(database.New(pool))

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	start := time.Now()
	snap, err := repo.LoadSnapshot(ctx, tenant, slug, 30*time.Minute)
	if err != nil {
		b.Fatalf("load snapshot: %v", err)
	}
	load := time.Since(start)
	runtime.GC()
	runtime.ReadMemStats(&after)

	nodes := snap.Scope.Len()
	if nodes == 0 {
		b.Skip("no scope nodes in this database")
	}
	resident := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	b.ReportMetric(float64(nodes), "nodes")
	b.ReportMetric(float64(resident)/(1<<20), "MB-resident")
	b.ReportMetric(float64(resident)/float64(nodes), "B/node")
	b.ReportMetric(float64(load.Milliseconds()), "ms-load")
	b.ReportMetric(float64(int64(after.HeapObjects)-int64(before.HeapObjects))/1e6, "M-objects")
	b.ReportMetric(0, "ns/op")
	runtime.KeepAlive(snap)
}

// Which snapshot tables reach the gate by PUSH and which only by POLL.
//
// This classification is what licenses Manager.load to skip rebuilding a
// snapshot whose catalog version has not moved. That shortcut is only sound
// while EVERY table carrying authorization state bumps the version — a table
// that changes decisions without bumping would simply never reach the gate.
//
// Migrations 0005/0006 covered scope_nodes, grants, grant_scopes, roles,
// permissions and route_policies. Migration 0040 closed the rest: sessions
// (revocation), identities (blocked / token_epoch / assurance), scope_axes
// (strict flip, global so it fans out over tenants), applications (the slug
// routes match on) and role_permissions_effective (via its role).
//
// Only true metadata may sit on the poll side now.
//
// This test parses db/queries/gate/snapshot.sql, asks Postgres which of those
// identifiers are real tables, and fails if any is unclassified or if the
// classification stops matching the triggers that actually exist. It caught
// `applications` when the lists were first written by hand.
func TestSnapshotTablesAreClassifiedPushOrPoll(t *testing.T) {
	skipWithoutDB(t)
	ctx := context.Background()

	// Bumped by trigger => pushed to the gate via LISTEN/NOTIFY.
	push := map[string]bool{
		// migrations 0005/0006
		"scope_nodes": true, "grants": true, "grant_scopes": true,
		"roles": true, "permissions": true, "route_policies": true,
		// migration 0040
		"identities": true, "sessions": true, "applications": true,
		"role_permissions_effective": true, "scope_axes": true,
	}
	// No trigger, and none needed: metadata, not authorization state.
	// Anything landing here that CAN change a decision is a bug — a
	// version-gated refresh would never see it.
	poll := map[string]bool{
		"tenants": true, "catalog_version": true,
	}

	raw, err := os.ReadFile(filepath.Join("..", "..", "db", "queries", "gate", "snapshot.sql"))
	if err != nil {
		t.Fatalf("read snapshot queries: %v", err)
	}
	// Candidate identifiers after FROM/JOIN; aliases are filtered out below by
	// asking Postgres which of them are actually tables.
	re := regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+([a-z_][a-z0-9_]*)`)
	seen := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(string(raw), -1) {
		seen[strings.ToLower(m[1])] = true
	}

	var tables []string
	for name := range seen {
		var isTable bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
			                 WHERE c.relname=$1 AND c.relkind='r' AND n.nspname='public')`,
			name).Scan(&isTable); err != nil {
			t.Fatal(err)
		}
		if isTable {
			tables = append(tables, name)
		}
	}
	sort.Strings(tables)
	if len(tables) < 8 {
		t.Fatalf("only matched %d tables (%v); the parse is probably broken", len(tables), tables)
	}

	// Every table the snapshot reads must be classified.
	for _, tbl := range tables {
		if !push[tbl] && !poll[tbl] {
			t.Errorf("snapshot reads %q but it is neither push nor poll classified — "+
				"decide whether it needs a catalog-version trigger, then add it above", tbl)
		}
	}

	// And the classification must match what the database actually does.
	for _, tbl := range tables {
		var bumps bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (
			  SELECT 1 FROM pg_trigger t
			    JOIN pg_class c ON c.oid = t.tgrelid
			    JOIN pg_proc  p ON p.oid = t.tgfoid
			   WHERE NOT t.tgisinternal AND c.relname = $1
			     AND p.proname LIKE 'trg_bump_catalog%')`,
			tbl).Scan(&bumps); err != nil {
			t.Fatal(err)
		}
		switch {
		case bumps && poll[tbl]:
			t.Errorf("%q now bumps the catalog version but is listed as poll-only; "+
				"move it to push (and revisit whether refreshAll can be made conditional)", tbl)
		case !bumps && push[tbl]:
			t.Errorf("%q is listed as pushed but has no bump trigger — changes to it "+
				"would wait up to a full poll interval to reach the gate", tbl)
		}
	}
}

// Migration 0040 gave the five poll-only tables catalog-version triggers so
// Manager.load can skip rebuilding an unchanged snapshot. The triggers are
// deliberately narrow, and the narrowness is the risk in both directions:
//
//   - too narrow and a revocation never reaches the gate (a security hole);
//   - too wide and last_seen_at — written on EVERY authenticated request —
//     bumps the catalog version, turning a ~92 MB snapshot rebuild into a
//     per-request operation.
//
// Both directions are asserted here. Everything runs in one transaction that
// is rolled back.
func TestCatalogVersionBumpsOnlyOnDecisionChanges(t *testing.T) {
	skipWithoutDB(t)
	ctx := context.Background()
	tenant, _ := firstTenantWithSlug(ctx, t)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.WithoutCancel(ctx)) //nolint:errcheck

	version := func() int64 {
		var v int64
		if err := tx.QueryRow(ctx,
			`SELECT COALESCE((SELECT version FROM catalog_version WHERE tenant_id=$1), 0)`,
			tenant).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	// bumps runs op and reports whether the tenant's catalog version moved.
	bumps := func(what string, sql string, args ...any) bool {
		t.Helper()
		before := version()
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		return version() != before
	}

	// An identity and a session to mutate. The identity INSERT itself bumps,
	// which is intended and asserted below.
	var realm string
	if err := tx.QueryRow(ctx,
		`SELECT id FROM realms WHERE tenant_id=$1 ORDER BY created_at LIMIT 1`, tenant).Scan(&realm); err != nil {
		t.Skipf("no realm seeded: %v", err)
	}
	if !bumps("identity insert",
		`INSERT INTO identities (tenant_id, realm_id, username) VALUES ($1,$2,'bumptest')`,
		tenant, realm) {
		t.Error("a new identity did not bump: it would be denied until the next poll")
	}
	var ident string
	if err := tx.QueryRow(ctx,
		`SELECT id FROM identities WHERE tenant_id=$1 AND username='bumptest'`, tenant).Scan(&ident); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO sessions (tenant_id, identity_id, expires_at)
		 VALUES ($1,$2, now() + interval '1 hour')`, tenant, ident); err != nil {
		t.Fatal(err)
	}
	var sess string
	if err := tx.QueryRow(ctx,
		`SELECT id FROM sessions WHERE identity_id=$1`, ident).Scan(&sess); err != nil {
		t.Fatal(err)
	}

	// ── MUST NOT bump: the hot write paths ────────────────────────────────
	quiet := []struct {
		what string
		sql  string
		args []any
	}{
		{"session last_seen_at (every request)",
			`UPDATE sessions SET last_seen_at = now() WHERE id=$1`, []any{sess}},
		{"session cookie rotation",
			`UPDATE sessions SET cookie_hash = 'x' WHERE id=$1`, []any{sess}},
		{"session active_scopes",
			`UPDATE sessions SET active_scopes = '{"a":1}'::jsonb WHERE id=$1`, []any{sess}},
		{"identity last_login_at (every login)",
			`UPDATE identities SET last_login_at = now() WHERE id=$1`, []any{ident}},
		{"identity updated_at",
			`UPDATE identities SET updated_at = now() WHERE id=$1`, []any{ident}},
		{"session amr",
			`UPDATE sessions SET amr = '{pwd}'::text[] WHERE id=$1`, []any{sess}},
	}
	for _, c := range quiet {
		if bumps(c.what, c.sql, c.args...) {
			t.Errorf("%s bumped the catalog version — this makes snapshot reload a "+
				"per-request cost and is far worse than the polling it replaces", c.what)
		}
	}

	// ── MUST bump: anything that changes a decision ───────────────────────
	loud := []struct {
		what string
		sql  string
		args []any
	}{
		{"session revoked",
			`UPDATE sessions SET revoked_at = now() WHERE id=$1`, []any{sess}},
		{"identity disabled",
			`UPDATE identities SET status='disabled', disabled_at=now() WHERE id=$1`, []any{ident}},
		{"identity token_epoch",
			`UPDATE identities SET token_epoch = token_epoch + 1 WHERE id=$1`, []any{ident}},
		{"identity assurance_level",
			`UPDATE identities SET assurance_level = 3 WHERE id=$1`, []any{ident}},
	}
	for _, c := range loud {
		if !bumps(c.what, c.sql, c.args...) {
			t.Errorf("%s did NOT bump the catalog version — with a version-gated "+
				"refresh the gate would never learn about it", c.what)
		}
	}

	// Re-revoking an already-revoked session is not news.
	if bumps("re-revoke", `UPDATE sessions SET revoked_at = now() WHERE id=$1`, sess) {
		t.Error("re-revoking an already revoked session bumped the version")
	}

	// Deleting the identity must bump: one left in the snapshot keeps being
	// evaluated. (Sessions cascade, so this also clears the row above.)
	if !bumps("identity delete", `DELETE FROM identities WHERE id=$1`, ident) {
		t.Error("deleting an identity did not bump the catalog version")
	}
}

// scope_axes has no tenant_id: default_effect='deny' makes an axis strict for
// everyone at once, so every tenant has to be invalidated.
func TestStrictAxisFlipInvalidatesEveryTenant(t *testing.T) {
	skipWithoutDB(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.WithoutCancel(ctx)) //nolint:errcheck

	before := map[string]int64{}
	rows, err := tx.Query(ctx, `SELECT id FROM tenants`)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) < 1 {
		t.Skip("no tenants")
	}
	for _, id := range ids {
		var v int64
		if err := tx.QueryRow(ctx,
			`SELECT COALESCE((SELECT version FROM catalog_version WHERE tenant_id=$1),0)`, id).Scan(&v); err != nil {
			t.Fatal(err)
		}
		before[id] = v
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO scope_axes (code, display_name, default_effect)
		 VALUES ('striktest','Strict Test','deny')`); err != nil {
		t.Fatal(err)
	}

	for _, id := range ids {
		var v int64
		if err := tx.QueryRow(ctx,
			`SELECT COALESCE((SELECT version FROM catalog_version WHERE tenant_id=$1),0)`, id).Scan(&v); err != nil {
			t.Fatal(err)
		}
		if v == before[id] {
			t.Errorf("tenant %s was not invalidated by a strict-axis insert; its grants "+
				"would keep passing an axis they do not address", id)
		}
	}
}

// The whole chain, against the real database: revoke a session and confirm
// both that the catalog version moves (so a version-gated refresh rebuilds)
// and that the rebuilt snapshot actually denies it.
//
// The unit tests cover the Manager's gating with a fake loader and the
// trigger behaviour in isolation. This is the composition — the thing that
// would be a silent security hole if either half were wrong.
func TestRevokedSessionSurvivesTheVersionGate(t *testing.T) {
	skipWithoutDB(t)
	ctx := context.Background()
	tenant, slug := firstTenantWithSlug(ctx, t)

	var realm string
	if err := pool.QueryRow(ctx,
		`SELECT id FROM realms WHERE tenant_id=$1 ORDER BY created_at LIMIT 1`, tenant).Scan(&realm); err != nil {
		t.Skipf("no realm seeded: %v", err)
	}
	// Committed, not in a rolled-back transaction: LoadSnapshot reads the pool.
	// Uniqueness is (tenant_id, realm_id, lower(username)), so clear any
	// leftover from an interrupted run rather than guessing at ON CONFLICT.
	if _, err := pool.Exec(ctx,
		`DELETE FROM identities WHERE tenant_id=$1 AND realm_id=$2 AND lower(username)='revoketest'`,
		tenant, realm); err != nil {
		t.Fatalf("clear leftover identity: %v", err)
	}
	var ident string
	if err := pool.QueryRow(ctx,
		`INSERT INTO identities (tenant_id, realm_id, username) VALUES ($1,$2,'revoketest')
		 RETURNING id`, tenant, realm).Scan(&ident); err != nil {
		t.Fatalf("seed identity: %v", err) // not Skip: a skip here would hide the whole test
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.WithoutCancel(ctx), `DELETE FROM identities WHERE id=$1`, ident)
	})
	var sess string
	if err := pool.QueryRow(ctx,
		`INSERT INTO sessions (tenant_id, identity_id, expires_at)
		 VALUES ($1,$2, now() + interval '1 hour') RETURNING id`, tenant, ident).Scan(&sess); err != nil {
		t.Fatal(err)
	}

	repo := gatepg.New(database.New(pool))
	before, err := repo.CatalogVersion(ctx, tenant)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := repo.LoadSnapshot(ctx, tenant, slug, 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	epoch := snap.Identities[ident].TokenEpoch
	if !snap.SessionAlive(sess, epoch, ident) {
		t.Fatal("precondition: a fresh session should be alive in the snapshot")
	}

	if _, err := pool.Exec(ctx, `UPDATE sessions SET revoked_at = now() WHERE id=$1`, sess); err != nil {
		t.Fatal(err)
	}

	after, err := repo.CatalogVersion(ctx, tenant)
	if err != nil {
		t.Fatal(err)
	}
	if after == before {
		t.Fatal("revoking a session did not move the catalog version — a version-gated " +
			"refresh would skip the rebuild and the session would keep passing the gate")
	}

	rebuilt, err := repo.LoadSnapshot(ctx, tenant, slug, 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt.SessionAlive(sess, epoch, ident) {
		t.Error("the rebuilt snapshot still considers a revoked session alive")
	}
}

// The triggers added by 0040 are STATEMENT level, matching migration 0006.
// RevokeAllSessions revokes every session an identity holds in one UPDATE, so
// a row-level trigger would fire one catalog_version upsert plus one
// pg_notify per session, all serialised on the same catalog_version row. One
// statement must cost exactly one bump no matter how many rows it touches.
func TestABulkRevokeCostsOneBump(t *testing.T) {
	skipWithoutDB(t)
	ctx := context.Background()
	tenant, _ := firstTenantWithSlug(ctx, t)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.WithoutCancel(ctx)) //nolint:errcheck

	var realm string
	if err := tx.QueryRow(ctx,
		`SELECT id FROM realms WHERE tenant_id=$1 ORDER BY created_at LIMIT 1`, tenant).Scan(&realm); err != nil {
		t.Skipf("no realm seeded: %v", err)
	}
	var ident string
	if err := tx.QueryRow(ctx,
		`INSERT INTO identities (tenant_id, realm_id, username) VALUES ($1,$2,'bulkrevoke')
		 RETURNING id`, tenant, realm).Scan(&ident); err != nil {
		t.Fatal(err)
	}
	const sessions = 40
	if _, err := tx.Exec(ctx,
		`INSERT INTO sessions (tenant_id, identity_id, expires_at)
		 SELECT $1, $2, now() + interval '1 hour' FROM generate_series(1,$3)`,
		tenant, ident, sessions); err != nil {
		t.Fatal(err)
	}

	version := func() int64 {
		var v int64
		if err := tx.QueryRow(ctx,
			`SELECT COALESCE((SELECT version FROM catalog_version WHERE tenant_id=$1),0)`,
			tenant).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	before := version()
	tag, err := tx.Exec(ctx,
		`UPDATE sessions SET revoked_at = now()
		  WHERE identity_id=$1 AND tenant_id=$2 AND revoked_at IS NULL`, ident, tenant)
	if err != nil {
		t.Fatal(err)
	}
	if n := tag.RowsAffected(); n != sessions {
		t.Fatalf("revoked %d sessions, expected %d", n, sessions)
	}
	if delta := version() - before; delta != 1 {
		t.Errorf("revoking %d sessions in one statement bumped the catalog version %d times, want 1 "+
			"— the trigger is firing per row", sessions, delta)
	}
}
