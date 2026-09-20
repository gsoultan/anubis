//go:build integration

package auditchain

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
)

func anchorRing(t *testing.T) *keyring.Manager {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	r, err := keyring.NewRing([]*keyring.Key{{
		Kid: "anchor-probe", Purpose: keyring.PurposeAccess, Alg: "Ed25519",
		Status: keyring.StatusActive, Public: pub, Private: priv,
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
	}})
	if err != nil {
		t.Fatal(err)
	}
	return keyring.NewManager(r)
}

// freshAnchoredChain clears the entries AND the anchors. freshChain only
// takes the entries, and an anchor outliving the chain it anchors makes the
// next test count somebody else's row.
func freshAnchoredChain(t *testing.T) {
	t.Helper()
	freshChain(t)
	if _, err := pool.Exec(context.Background(),
		`DELETE FROM audit_anchors WHERE tenant_id = $1`, probeTenant); err != nil {
		t.Fatalf("clear anchors: %v", err)
	}
}

func emit(t *testing.T, n int) {
	t.Helper()
	a := auditorFor(t)
	for i := 0; i < n; i++ {
		a.Emit(context.Background(), auditdomain.AuditEvent{
			TenantID: probeTenant, ActorKind: "service",
			Action: "probe.anchor", Result: "allow",
		})
	}
	a.Close() // drains; the writer is asynchronous by design
}

// THE property the chain alone does not have.
//
// Walking the chain proves it is SELF-consistent, and a wholesale rewrite is
// self-consistent too: edit an entry, recompute every forward hash, and
// VerifyChain is satisfied. migrations/0005 claimed an attacker with UPDATE
// "cannot silently rewrite history"; what actually stood in the way was that
// anubis_app holds no UPDATE grant — a permissions property, not a
// cryptographic one.
//
// An anchor is the cryptographic half: the chain must still pass through a
// hash that was SIGNED at the time.
func TestAnAnchorCatchesARewriteTheChainAccepts(t *testing.T) {
	freshAnchoredChain(t)
	ctx := context.Background()
	a := auditorFor(t)
	ring := anchorRing(t)

	emit(t, 5)
	if err := a.AnchorChain(ctx, ring, probeTenant); err != nil {
		t.Fatalf("anchor: %v", err)
	}
	emit(t, 3)

	// Baseline: both halves agree the chain is intact.
	if _, broken, err := a.VerifyChain(ctx, probeTenant, nil, nil); err != nil || broken != 0 {
		t.Fatalf("chain reported broken before tampering: broken=%d err=%v", broken, err)
	}
	if _, broken, err := a.VerifyAnchors(ctx, ring, probeTenant); err != nil || broken != 0 {
		t.Fatalf("anchors reported broken before tampering: broken=%d err=%v", broken, err)
	}

	// Change the anchored entry's stored hash.
	//
	// This is what a rewrite LEAVES BEHIND at the anchored sequence. The
	// test does not recompute the forward chain to simulate one properly —
	// that would mean reimplementing chainHash here, and a test that
	// duplicates the production hash agrees with it by construction even
	// when both are wrong. What matters is the anchor's contract: the entry
	// at an anchored seq must still carry the hash that was signed. A
	// rewrite cannot satisfy that without the signing key, however
	// self-consistent it makes the rest.
	if _, err := pool.Exec(ctx,
		`UPDATE audit_log SET entry_hash = decode(repeat('ab', 32), 'hex')
		  WHERE tenant_id = $1 AND seq = 5`, probeTenant); err != nil {
		t.Fatalf("tamper: %v", err)
	}

	checked, broken, err := a.VerifyAnchors(ctx, ring, probeTenant)
	if err != nil {
		t.Fatalf("verify anchors: %v", err)
	}
	if broken == 0 {
		t.Fatalf("a changed entry passed anchor verification (%d anchors checked)", checked)
	}
	if broken != 5 {
		t.Fatalf("anchor broke at seq %d, want the anchored head 5", broken)
	}
}

// An anchor whose signature does not verify must break, not be skipped: a
// forged anchor claiming a hash nobody signed is worse than none.
func TestAForgedAnchorDoesNotVerify(t *testing.T) {
	freshAnchoredChain(t)
	ctx := context.Background()
	a := auditorFor(t)
	ring := anchorRing(t)

	emit(t, 3)
	if err := a.AnchorChain(ctx, ring, probeTenant); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE audit_anchors SET signature = decode(repeat('cd', 64), 'hex')
		  WHERE tenant_id = $1`, probeTenant); err != nil {
		t.Fatal(err)
	}
	if _, broken, err := a.VerifyAnchors(ctx, ring, probeTenant); err != nil {
		t.Fatalf("verify: %v", err)
	} else if broken == 0 {
		t.Fatal("an anchor with a forged signature verified")
	}
}

// Re-anchoring an unchanged head must not stack rows, because the job that
// writes anchors runs on a schedule and most tenants are idle.
func TestReAnchoringAnIdleTenantIsANoOp(t *testing.T) {
	freshAnchoredChain(t)
	ctx := context.Background()
	a := auditorFor(t)
	ring := anchorRing(t)

	emit(t, 2)
	for i := 0; i < 3; i++ {
		if err := a.AnchorChain(ctx, ring, probeTenant); err != nil {
			t.Fatalf("anchor %d: %v", i, err)
		}
	}
	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_anchors WHERE tenant_id = $1`, probeTenant).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("%d anchors for one unchanged head, want 1", n)
	}
}

// A tenant that has never emitted anything has no head to sign.
func TestAnchoringAnEmptyChainDoesNothing(t *testing.T) {
	freshAnchoredChain(t)
	ctx := context.Background()
	if err := auditorFor(t).AnchorChain(ctx, anchorRing(t), probeTenant); err != nil {
		t.Fatalf("anchoring an empty chain errored: %v", err)
	}
}
