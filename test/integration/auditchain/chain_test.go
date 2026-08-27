//go:build integration

// Package auditchain proves the property migrations/0005 claims for the
// audit log: "an attacker with UPDATE rights on this table still cannot
// silently rewrite history."
//
// Its own package because test/integration sits at the ten-file limit the
// architecture enforces, and these tests are one aggregate — the chain, and
// what it does when somebody edits or deletes an entry.
//
//	ANUBIS_DB_URL=postgres://anubis:anubis@localhost:7449/anubis?sslmode=disable \
//	  go test -tags integration ./test/integration/auditchain/
package auditchain

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	auditpg "github.com/gsoultan/anubis/internal/audit/adapter/postgres"
	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
)

var (
	pool *pgxpool.Pool
	// probeTenant is created for these tests and removed afterwards. It is
	// NOT the installation's first tenant, and that distinction is the whole
	// point: TestAuditChainDetectsADeletedRow removes an entry, which breaks
	// that tenant's chain from there on for good. Pointed at a real tenant it
	// destroys real evidence — it did, to a development database, before this
	// harness existed.
	probeTenant string
)

const probeSlug = "auditchain-probe"

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
	ctx := context.Background()

	if err := p.QueryRow(ctx,
		`INSERT INTO tenants (slug, name) VALUES ($1, 'Audit chain probe')
		 ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name
		 RETURNING id`, probeSlug).Scan(&probeTenant); err != nil {
		panic("create probe tenant: " + err.Error())
	}

	code := m.Run()

	// Take the scratch tenant and its entries with us. A half-torn chain left
	// behind would fail somebody else's verification for reasons that have
	// nothing to do with them.
	_, _ = p.Exec(ctx, `DELETE FROM audit_log WHERE tenant_id = $1`, probeTenant)
	_, _ = p.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, probeTenant)
	p.Close()
	os.Exit(code)
}

// freshChain empties the probe tenant so each test starts from a chain it
// built itself, rather than one an earlier test already broke.
func freshChain(t *testing.T) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`DELETE FROM audit_log WHERE tenant_id = $1`, probeTenant); err != nil {
		t.Fatalf("clear probe tenant: %v", err)
	}
}

func skipWithoutDB(t *testing.T) {
	t.Helper()
	if pool == nil {
		t.Skip("ANUBIS_DB_URL not set")
	}
}

func auditorFor(t *testing.T) *auditpg.ChainedAuditor {
	t.Helper()
	repo := auditpg.New(database.New(pool))
	return auditpg.NewChainedAuditor(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// emitAudited writes n events for a tenant and waits for the queue to drain,
// so the rows and their hashes exist before anything reads them.
func emitAudited(t *testing.T, tenant string, n int) *auditpg.ChainedAuditor {
	t.Helper()
	a := auditorFor(t)
	for i := 0; i < n; i++ {
		a.Emit(context.Background(), auditdomain.AuditEvent{
			TenantID: tenant, Action: "tamper.probe", Result: "allow",
			Detail: []byte(`{"probe":` + itoa(i) + `}`),
		})
	}
	a.Close() // drains; the writer is asynchronous by design
	return auditorFor(t)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

// An intact chain must verify, or every failure below proves nothing.
func TestAuditChainVerifiesWhenIntact(t *testing.T) {
	skipWithoutDB(t)
	ctx := context.Background()
	freshChain(t)
	tenant := probeTenant

	a := emitAudited(t, tenant, 8)
	checked, brokenAt, err := a.VerifyChain(ctx, tenant, nil, nil)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if brokenAt != 0 {
		t.Fatalf("an untouched chain reported broken at seq %d", brokenAt)
	}
	if checked == 0 {
		t.Fatal("verified nothing — the walk is not reading rows")
	}
	t.Logf("intact: %d entries verified", checked)
}

// Editing a recorded fact — turning a denial into an approval, the edit an
// attacker actually wants — must not pass verification.
func TestAuditChainDetectsAnEditedRow(t *testing.T) {
	skipWithoutDB(t)
	ctx := context.Background()
	freshChain(t)
	tenant := probeTenant
	a := emitAudited(t, tenant, 8)

	var seq int64
	if err := pool.QueryRow(ctx,
		`SELECT seq FROM audit_log WHERE tenant_id=$1 AND action='tamper.probe'
		  ORDER BY seq DESC OFFSET 3 LIMIT 1`, tenant).Scan(&seq); err != nil {
		t.Fatalf("pick a row: %v", err)
	}
	// The attacker rewrites the RESULT and leaves the hashes alone — the
	// naive edit, and the one a plain table would accept silently.
	if _, err := pool.Exec(ctx,
		`UPDATE audit_log SET result='deny' WHERE tenant_id=$1 AND seq=$2`, tenant, seq); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	defer pool.Exec(ctx, `UPDATE audit_log SET result='allow' WHERE tenant_id=$1 AND seq=$2`, tenant, seq)

	_, brokenAt, err := a.VerifyChain(ctx, tenant, nil, nil)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if brokenAt == 0 {
		t.Fatal("a rewritten audit entry passed verification — the chain is decorative")
	}
	if brokenAt != seq {
		t.Fatalf("chain broke at seq %d, expected the edited row %d", brokenAt, seq)
	}
	t.Logf("edited seq %d detected at seq %d", seq, brokenAt)
}

// Deleting an inconvenient entry must be as detectable as editing one:
// removing the record of what happened is the same attack.
func TestAuditChainDetectsADeletedRow(t *testing.T) {
	skipWithoutDB(t)
	ctx := context.Background()
	freshChain(t)
	tenant := probeTenant
	a := emitAudited(t, tenant, 8)

	// The whole row, as jsonb, so the deletion below can be UNDONE. The
	// edited-row test one function up already restores what it breaks with a
	// defer; this one captured the fields to do the same and never did, so
	// every run left a permanent hole in whatever database it touched — and
	// a hole in a hash chain cannot be repaired afterwards, because the rows
	// that follow it hash the entry that is now gone. Three runs against a
	// shared dev database is three tests that fail from then on, for a real
	// reason, with nothing left to fix them with.
	//
	// to_jsonb/jsonb_populate_record rather than a column list: this table
	// gains columns, and a restore that silently drops one would be worse
	// than the bug being fixed.
	var seq int64
	var snapshot []byte
	if err := pool.QueryRow(ctx,
		`SELECT seq, to_jsonb(a) FROM audit_log a
		  WHERE tenant_id=$1 AND action='tamper.probe' ORDER BY seq DESC OFFSET 3 LIMIT 1`,
		tenant).Scan(&seq, &snapshot); err != nil {
		t.Fatalf("pick a row: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`DELETE FROM audit_log WHERE tenant_id=$1 AND seq=$2`, tenant, seq); err != nil {
		t.Fatalf("delete: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(),
			`INSERT INTO audit_log SELECT * FROM jsonb_populate_record(NULL::audit_log, $1::jsonb)`,
			string(snapshot)); err != nil {
			t.Errorf("could not put seq %d back — this database's chain is now broken from there on: %v", seq, err)
		}
	})

	_, brokenAt, err := a.VerifyChain(ctx, tenant, nil, nil)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if brokenAt == 0 {
		t.Fatal("a deleted audit entry left no trace — history can be quietly shortened")
	}
	t.Logf("deleted seq %d detected at seq %d", seq, brokenAt)
}

// The fix must verify rows written by the OLD code. Current writers emit
// compact, key-sorted JSON already, so canonicalising what the database
// returns lands on the same bytes the old writer hashed — no rehash, no
// migration. This asserts that against whatever history the database holds.
func TestAuditChainVerifiesHistoryWrittenBeforeTheFix(t *testing.T) {
	skipWithoutDB(t)
	ctx := context.Background()
	var tenant string
	var n int64
	if err := pool.QueryRow(ctx,
		`SELECT tenant_id, count(*) FROM audit_log
		  WHERE detail IS NOT NULL AND detail::text <> '{}'
		  GROUP BY tenant_id ORDER BY count(*) DESC LIMIT 1`).Scan(&tenant, &n); err != nil {
		t.Skipf("no audit history with details: %v", err)
	}
	a := auditorFor(t)
	checked, brokenAt, err := a.VerifyChain(ctx, tenant, nil, nil)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if brokenAt != 0 {
		t.Fatalf("history broke at seq %d after %d entries — the fix is not "+
			"backward compatible with rows the old writer hashed", brokenAt, checked)
	}
	t.Logf("verified %d entries of real history (%d carry a detail)", checked, n)
}
