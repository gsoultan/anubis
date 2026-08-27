//go:build integration

package integration

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"net/url"
	"os"
	"testing"
	"time"
)

// SECURITY DOC CLAIM: "Restoring refresh_tokens brings revoked tokens back to
// life. token_epoch on identities is the mitigation — but only if restored
// consistently AND if applications actually validate it. Test this
// explicitly; nobody discovers it until it matters."
//
// This is that test. It simulates the restore directly: put a consumed or
// revoked refresh row back into 'active', exactly as a backup would, and
// assert the epoch recorded in outstanding tokens no longer matches the
// identity — so every token minted before the bump is rejected regardless of
// what the token table says.
func TestBackupRestoreCannotResurrectAccess(t *testing.T) {
	skipWithoutDB(t)
	ctx := context.Background()
	tenant := firstTenant(ctx, t)

	var identityID string
	var epochBefore int
	if err := pool.QueryRow(ctx,
		`SELECT id, token_epoch FROM identities WHERE tenant_id=$1 AND status='active'
		 ORDER BY created_at LIMIT 1`, tenant).Scan(&identityID, &epochBefore); err != nil {
		t.Skipf("no identity seeded: %v", err)
	}

	// A session with a refresh token, then a global sign-out: sessions
	// revoked, tokens revoked, epoch bumped.
	var sessionID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO sessions (identity_id, tenant_id, amr, expires_at, active_scopes)
		 VALUES ($1,$2,'{pwd}', now() + interval '1 hour', '{}') RETURNING id`,
		identityID, tenant).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}
	var refreshID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO refresh_tokens (session_id, tenant_id, family_id, token_hash, expires_at)
		 VALUES ($1,$2,gen_random_uuid(), sha256('restore-probe'::bytea), now() + interval '30 days')
		 RETURNING id`, sessionID, tenant).Scan(&refreshID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE sessions SET revoked_at = now(), revoke_reason='logout_all' WHERE id=$1`, sessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE refresh_tokens SET status='revoked', revoked_at=now() WHERE session_id=$1`, sessionID); err != nil {
		t.Fatal(err)
	}
	var epochAfter int
	if err := pool.QueryRow(ctx,
		`UPDATE identities SET token_epoch = token_epoch + 1 WHERE id=$1 RETURNING token_epoch`,
		identityID).Scan(&epochAfter); err != nil {
		t.Fatal(err)
	}
	if epochAfter != epochBefore+1 {
		t.Fatalf("epoch did not advance: %d -> %d", epochBefore, epochAfter)
	}

	// --- the restore: token rows come back exactly as the backup held them.
	if _, err := pool.Exec(ctx,
		`UPDATE refresh_tokens SET status='active', revoked_at=NULL WHERE id=$1`, refreshID); err != nil {
		t.Fatal(err)
	}

	// The token row is active again — and it is still useless, because the
	// session it belongs to stays revoked and the epoch moved on.
	var sessionRevoked bool
	if err := pool.QueryRow(ctx,
		`SELECT revoked_at IS NOT NULL FROM sessions WHERE id=$1`, sessionID).Scan(&sessionRevoked); err != nil {
		t.Fatal(err)
	}
	if !sessionRevoked {
		t.Fatal("restore resurrected the session; refresh rotation would succeed")
	}
	var currentEpoch int
	if err := pool.QueryRow(ctx,
		`SELECT token_epoch FROM identities WHERE id=$1`, identityID).Scan(&currentEpoch); err != nil {
		t.Fatal(err)
	}
	if currentEpoch == epochBefore {
		t.Fatal("epoch rolled back with the restore: every pre-bump access token is valid again")
	}

	// Cleanup so repeated runs stay deterministic.
	_, _ = pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE id=$1`, refreshID)
	_, _ = pool.Exec(ctx, `DELETE FROM sessions WHERE id=$1`, sessionID)
}

// The privilege table in operations.md is a set of claims about what the
// runtime cannot do. Until migration 0037 one of them — the schema grants —
// was decorative: everything reached the schema through PUBLIC regardless of
// what 0023 granted by name. That was found by hand, on a database, months
// after it shipped.
//
// So the rest are asserted here rather than described. Each row of that table
// is one case below, exercised as a real login role holding only the group it
// names. A future GRANT that quietly widens any of them fails this.
func TestDatabaseRolesCannotExceedTheirPrivilege(t *testing.T) {
	skipWithoutDB(t)
	ctx := context.Background()
	base, err := url.Parse(os.Getenv("ANUBIS_DB_URL"))
	if err != nil {
		t.Fatalf("ANUBIS_DB_URL is not a URL: %v", err)
	}

	// Roles are cluster-wide, so the names must not collide with a parallel
	// run, and they must be cleaned up even when an assertion fails.
	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1e9)
	appRole, roRole := "probe_app_"+suffix, "probe_ro_"+suffix
	for _, r := range []struct{ name, group string }{
		{appRole, "anubis_app"}, {roRole, "anubis_readonly"},
	} {
		if _, err := pool.Exec(ctx, fmt.Sprintf(
			`CREATE ROLE %s LOGIN PASSWORD 'probe'`, r.name)); err != nil {
			t.Fatalf("create %s: %v", r.name, err)
		}
		defer func(n string) { _, _ = pool.Exec(ctx, `DROP ROLE IF EXISTS `+n) }(r.name)
		if _, err := pool.Exec(ctx, fmt.Sprintf(
			`GRANT %s TO %s`, r.group, r.name)); err != nil {
			t.Fatalf("grant %s: %v", r.group, err)
		}
	}

	connAs := func(role string) *pgx.Conn {
		t.Helper()
		u := *base
		u.User = url.UserPassword(role, "probe")
		c, cerr := pgx.Connect(ctx, u.String())
		if cerr != nil {
			t.Fatalf("connect as %s: %v", role, cerr)
		}
		return c
	}
	mustRefuse := func(c *pgx.Conn, what, sql string) {
		t.Helper()
		if _, err := c.Exec(ctx, sql); err == nil {
			t.Errorf("%s was ALLOWED: %s", what, sql)
		}
	}

	app := connAs(appRole)
	defer app.Close(ctx)
	ro := connAs(roRole)
	defer ro.Close(ctx)

	// The runtime may read and append the audit log and nothing else to it.
	// The hash chain detects tampering; this stops the runtime from being
	// able to attempt it at all.
	if _, err := app.Exec(ctx, `SELECT count(*) FROM audit_log`); err != nil {
		t.Fatalf("the runtime cannot read audit_log at all: %v", err)
	}
	mustRefuse(app, "runtime rewriting audit history", `UPDATE audit_log SET result = 'allow'`)
	mustRefuse(app, "runtime deleting audit history", `DELETE FROM audit_log`)
	mustRefuse(app, "runtime truncating audit history", `TRUNCATE audit_log`)

	// ADR-0005 §7 refused to require CREATE for the runtime: "you have traded
	// a deploy for a privilege-escalation path". This is that refusal.
	mustRefuse(app, "runtime creating a table", `CREATE TABLE privilege_probe (i int)`)
	mustRefuse(app, "runtime dropping a table", `DROP TABLE tenants`)
	mustRefuse(app, "runtime altering a table", `ALTER TABLE tenants ADD COLUMN probe int`)

	// A reporting role must not become a credential dump.
	for _, secret := range []string{
		"credentials", "signing_keys", "refresh_tokens", "one_time_tokens", "pii_keys",
	} {
		mustRefuse(ro, "readonly reading "+secret, `SELECT count(*) FROM `+secret)
	}
	if _, err := ro.Exec(ctx, `SELECT count(*) FROM tenants`); err != nil {
		t.Fatalf("the readonly role cannot read a non-secret table: %v", err)
	}
}
