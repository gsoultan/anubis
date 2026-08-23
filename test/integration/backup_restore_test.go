//go:build integration

package integration

import (
	"context"
	"testing"
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
