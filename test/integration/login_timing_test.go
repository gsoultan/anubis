//go:build integration

package integration

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	authpg "github.com/gsoultan/anubis/internal/auth/adapter/postgres"
	"github.com/gsoultan/anubis/internal/auth/app/signin"
	identitypg "github.com/gsoultan/anubis/internal/identity/adapter/postgres"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
	"github.com/gsoultan/anubis/internal/platform/database"
	tenancypg "github.com/gsoultan/anubis/internal/tenancy/adapter/postgres"
)

// SECURITY DOC: "The KDF runs even when the user does not exist, compared
// against a fixed dummy hash, so response timing matches. This is invisible
// in functional testing and visible in a timing histogram. Test it with a
// histogram, not an assertion."
//
// So this measures distributions rather than asserting an error code. It runs
// against the LOGIN USECASE, not HTTP: the rate limiter would otherwise
// dominate the sample, and the property under test lives in the interactor.
//
// The load-bearing check is the floor. If the unknown-user path returned
// early instead of burning the KDF, its median would collapse to ~0ms while
// the real-user path stays at PBKDF2 cost — a difference any attacker can
// measure over the network.
func TestLoginTimingDoesNotRevealUserExistence(t *testing.T) {
	skipWithoutDB(t)
	if testing.Short() {
		t.Skip("timing histogram runs full-cost KDF")
	}
	ctx := context.Background()
	db := database.New(pool)
	identity := identitypg.New(db)
	auth := authpg.New(db)
	tenancy := tenancypg.New(db)

	// A keyring is only touched on the MFA branch; a local key satisfies the
	// constructor without reaching for the database.
	local, err := keyring.GenerateLocalKey(time.Now(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	local.Status = keyring.StatusActive
	ring, err := keyring.NewRing([]*keyring.Key{local})
	if err != nil {
		t.Fatal(err)
	}

	login := signin.NewLoginInteractor(tenancy, identity, identity, identity,
		auth, auth, nopIssuer{}, keyring.NewManager(ring), auth, systemClock{}, nopAuditor{})

	var slug string
	if err := pool.QueryRow(ctx, `SELECT slug FROM tenants ORDER BY created_at LIMIT 1`).Scan(&slug); err != nil {
		t.Skipf("no tenant: %v", err)
	}
	var existing string
	if err := pool.QueryRow(ctx, `
		SELECT i.username FROM identities i
		  JOIN credentials c ON c.identity_id = i.id AND c.kind='password' AND c.revoked_at IS NULL
		  JOIN realms r ON r.id = i.realm_id AND r.code='internal'
		 WHERE i.status='active' LIMIT 1`).Scan(&existing); err != nil {
		t.Skipf("no password identity seeded: %v", err)
	}

	const samples = 24
	measure := func(username string) []time.Duration {
		out := make([]time.Duration, 0, samples)
		for i := 0; i < samples; i++ {
			start := time.Now()
			_, _ = login.Execute(ctx, signin.LoginInput{
				Tenant: slug, Realm: "internal",
				Username: username, Password: "definitely-not-the-password",
			})
			out = append(out, time.Since(start))
		}
		sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
		return out
	}

	real := measure(existing)
	fake := measure("no-such-user-anywhere-" + fmt.Sprint(time.Now().UnixNano()))

	median := func(d []time.Duration) time.Duration { return d[len(d)/2] }
	p90 := func(d []time.Duration) time.Duration { return d[(len(d)*9)/10] }
	mr, mf := median(real), median(fake)

	t.Logf("existing user : median=%v p90=%v min=%v", mr, p90(real), real[0])
	t.Logf("unknown user  : median=%v p90=%v min=%v", mf, p90(fake), fake[0])

	// 1. Both paths must actually pay for the KDF. This is the check that
	//    catches an early return for unknown users.
	const kdfFloor = 20 * time.Millisecond
	if mf < kdfFloor {
		t.Fatalf("unknown-user login answered in %v (< %v): the KDF was skipped, "+
			"so response time is a user-enumeration oracle", mf, kdfFloor)
	}
	if mr < kdfFloor {
		t.Fatalf("existing-user login answered in %v (< %v): KDF cost is too low to hide anything", mr, kdfFloor)
	}

	// 2. The two distributions must be close. A large relative gap is
	//    measurable over the network even when both paths run the KDF.
	hi, lo := mr, mf
	if lo > hi {
		hi, lo = mf, mr
	}
	if ratio := float64(hi-lo) / float64(hi); ratio > 0.30 {
		t.Fatalf("median gap %.1f%% (existing=%v unknown=%v) is a timing oracle", ratio*100, mr, mf)
	}
}
