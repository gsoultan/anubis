//go:build integration

// Package authtokens covers the auth context's state machines against a real
// database.
//
// Every write here is a guarded transition — `SET status = 'consumed' WHERE
// status = 'active'` — and the guard is the security property, not an
// optimisation: a read-then-write would let two presentations of one token
// both pass the read and both be issued a successor. These assert that the
// SECOND caller loses.
package authtokens

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	authpg "github.com/gsoultan/anubis/internal/auth/adapter/postgres"
	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	pool     *pgxpool.Pool
	tenant   string
	identity string
)

func TestMain(m *testing.M) {
	dsn := os.Getenv("ANUBIS_DB_URL")
	if dsn == "" {
		os.Exit(0)
	}
	p, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		panic(err)
	}
	pool = p
	ctx := context.Background()
	slug := fmt.Sprintf("zztok%d", time.Now().UnixNano()%1_000_000_000)
	if err := p.QueryRow(ctx,
		`INSERT INTO tenants (slug, name) VALUES ($1, 'Auth token probe') RETURNING id`,
		slug).Scan(&tenant); err != nil {
		panic("create probe tenant: " + err.Error())
	}
	if err := p.QueryRow(ctx,
		`INSERT INTO identities (tenant_id, username) VALUES ($1, $2) RETURNING id`,
		tenant, slug).Scan(&identity); err != nil {
		panic("create probe identity: " + err.Error())
	}
	code := m.Run()
	for _, q := range []string{
		`DELETE FROM refresh_tokens WHERE tenant_id = $1`,
		`DELETE FROM one_time_tokens WHERE tenant_id = $1`,
		`DELETE FROM sessions WHERE tenant_id = $1`,
		`DELETE FROM identities WHERE tenant_id = $1`,
		`DELETE FROM tenants WHERE id = $1`,
	} {
		_, _ = p.Exec(ctx, q, tenant)
	}
	p.Close()
	os.Exit(code)
}

func repo(t *testing.T) *authpg.Repository {
	t.Helper()
	if pool == nil {
		t.Skip("ANUBIS_DB_URL not set")
	}
	return authpg.New(database.New(pool))
}

func hash(t *testing.T) []byte {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return b
}

func newSession(t *testing.T, r *authpg.Repository) *authdomain.Session {
	t.Helper()
	s, err := r.CreateSession(context.Background(), authdomain.SessionInput{
		IdentityID: identity, TenantID: tenant,
		AMR: []string{"pwd"}, IP: "203.0.113.9", UserAgent: "probe/1",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return s
}

// The rotation core. Exactly one caller may consume a token; the second gets
// nothing back and the interactor reads that as theft.
func TestClaimRefreshIsSingleUse(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	sess := newSession(t, r)
	h := hash(t)
	expires := time.Now().Add(24 * time.Hour)

	if _, err := r.CreateRefresh(ctx, authdomain.RefreshInput{
		SessionID: sess.ID, TenantID: tenant, FamilyID: sess.ID,
		Generation: 0, TokenHash: h, ExpiresAt: expires,
	}); err != nil {
		t.Fatal(err)
	}

	first, err := r.ClaimRefresh(ctx, h)
	if err != nil {
		t.Fatal(err)
	}
	if first == nil {
		t.Fatal("the first claim of a live token failed")
	}
	second, err := r.ClaimRefresh(ctx, h)
	if err != nil {
		t.Fatal(err)
	}
	if second != nil {
		t.Fatal("a refresh token was claimed twice — the rotation guard is not guarding")
	}

	// And the token is still READABLE in its consumed state, which is what
	// tells the interactor this was theft rather than a miss.
	info, err := r.RefreshByHash(ctx, h)
	if err != nil {
		t.Fatal(err)
	}
	if info == nil || info.Status != "consumed" {
		t.Fatalf("after claiming, the token reads %+v", info)
	}
}

// The theft response: the whole family dies, whatever generation the attacker
// holds.
func TestRevokeRefreshFamilyKillsEveryGeneration(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	sess := newSession(t, r)
	expires := time.Now().Add(24 * time.Hour)

	var hashes [][]byte
	for gen := 0; gen < 3; gen++ {
		h := hash(t)
		hashes = append(hashes, h)
		if _, err := r.CreateRefresh(ctx, authdomain.RefreshInput{
			SessionID: sess.ID, TenantID: tenant, FamilyID: sess.ID,
			Generation: gen, TokenHash: h, ExpiresAt: expires,
		}); err != nil {
			t.Fatal(err)
		}
	}

	n, err := r.RevokeRefreshFamily(ctx, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("revoked %d of 3 generations", n)
	}
	for i, h := range hashes {
		info, err := r.RefreshByHash(ctx, h)
		if err != nil {
			t.Fatal(err)
		}
		if info == nil || info.Status != "revoked" {
			t.Fatalf("generation %d reads %+v after a family revocation", i, info)
		}
		// And none of them can be claimed.
		claim, err := r.ClaimRefresh(ctx, h)
		if err != nil {
			t.Fatal(err)
		}
		if claim != nil {
			t.Fatalf("a revoked token (generation %d) was still claimable", i)
		}
	}
}

// Revoking a session clears its cookie hash in the SAME statement, so the
// cookie stops resolving immediately rather than at expiry.
func TestRevokeSessionClearsTheCookie(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	sess := newSession(t, r)
	ck := hash(t)

	if err := r.SetSessionCookieHash(ctx, sess.ID, ck); err != nil {
		t.Fatal(err)
	}
	got, err := r.SessionByCookieHash(ctx, ck)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != sess.ID {
		t.Fatalf("cookie resolved to %+v", got)
	}

	revoked, err := r.RevokeSession(ctx, tenant, sess.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if revoked == nil {
		t.Fatal("revoking a live session returned nothing")
	}
	if _, err := r.SessionByCookieHash(ctx, ck); err == nil {
		t.Fatal("a revoked session's cookie still resolves")
	}

	// A second revocation changes nothing and says so.
	again, err := r.RevokeSession(ctx, tenant, sess.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if again != nil {
		t.Fatal("a session was revoked twice")
	}
}

// Step-up: the recency window restarts, which is what a max_age check reads.
func TestUpgradeSessionAMRRestartsAuthTime(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	sess := newSession(t, r)

	time.Sleep(10 * time.Millisecond)
	after, err := r.UpgradeSessionAMR(ctx, sess.ID, []string{"pwd", "otp"})
	if err != nil {
		t.Fatal(err)
	}
	if !after.After(sess.AuthTime) {
		t.Fatalf("auth_time did not move: %s -> %s", sess.AuthTime, after)
	}
	live, err := r.SessionLive(ctx, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(live.AMR) != 2 {
		t.Fatalf("amr is %v after a step-up", live.AMR)
	}
}

// A one-time token is consumed by DELETE ... RETURNING, so a second
// presentation finds no row — there is no window between the read and the
// delete because there is no read.
func TestOneTimeTokenIsConsumedExactlyOnce(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	h := hash(t)
	payload, _ := json.Marshal(map[string]string{"purpose": "probe"})

	if _, err := r.CreateOneTime(ctx, tenant, "mfa", h, payload, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	gotTenant, gotPayload, err := r.ConsumeOneTime(ctx, "mfa", h)
	if err != nil {
		t.Fatal(err)
	}
	if gotTenant != tenant {
		t.Fatalf("consumed token belongs to %s, want %s", gotTenant, tenant)
	}
	var p map[string]string
	if err := json.Unmarshal(gotPayload, &p); err != nil || p["purpose"] != "probe" {
		t.Fatalf("payload came back %s (%v)", gotPayload, err)
	}

	if _, _, err := r.ConsumeOneTime(ctx, "mfa", h); err == nil {
		t.Fatal("a one-time token was consumed twice")
	}
}

// The kind is part of the lookup: a token minted for MFA must not be spendable
// as a password reset.
func TestOneTimeTokenIsScopedToItsKind(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	h := hash(t)

	if _, err := r.CreateOneTime(ctx, tenant, "mfa", h, nil, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.ConsumeOneTime(ctx, "password_reset", h); err == nil {
		t.Fatal("an mfa token was consumed as a password reset")
	}
	// And it is still there for its own kind.
	if _, _, err := r.ConsumeOneTime(ctx, "mfa", h); err != nil {
		t.Fatalf("the token was destroyed by the wrong-kind attempt: %v", err)
	}
}
