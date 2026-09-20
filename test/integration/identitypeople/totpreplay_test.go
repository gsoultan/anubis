//go:build integration

package identitypeople

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	credentialdomain "github.com/gsoultan/anubis/internal/identity/domain/credential"
)

func totpCred(t *testing.T, name string) string {
	t.Helper()
	r := repo(t)
	ctx := context.Background()
	id := newIdentity(t, r, name)
	credID, err := r.CreateCredential(ctx, credentialdomain.CredentialInput{
		IdentityID: id, TenantID: tenant, Kind: "totp", Params: []byte(`{"last_step":10}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	return credID
}

// A TOTP code may be accepted ONCE. verify_mfa_interactor.go says so in a
// comment and then reads the last accepted step, compares it in Go, and
// writes the new one — with nothing between the read and the write.
//
// The control plane implements the same guard correctly and its comment names
// this exact failure: "read-then-write would let two presentations of the
// same code both pass the read". Two concurrent submissions of one code both
// see the old step, both pass, and both proceed to mint a session.
func TestOneCodeCannotBeAcceptedTwice(t *testing.T) {
	r := repo(t)
	ctx := context.Background()

	// Rounds with a fresh credential each: the window between the read and
	// the write is small, so one pair of goroutines rarely lands in it.
	// Widening the contention is what makes this deterministic, exactly as
	// it was for the statement-cache race.
	const (
		rounds    = 25
		attempts  = 8
		firstStep = uint64(11)
	)

	for round := 0; round < rounds; round++ {
		credID := totpCred(t, fmt.Sprintf("zztotp%d%d", time.Now().UnixNano()%1_000_000, round))
		owner := ownerOf(t, credID)

		start := make(chan struct{})
		var wg sync.WaitGroup
		accepted := make(chan struct{}, attempts)
		for i := 0; i < attempts; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				// Exactly what the interactor does now: ask the database
				// to record the step, and treat losing as a replay.
				if _, err := r.ActiveCredentialOfKind(ctx, owner, "totp"); err != nil {
					return
				}
				won, err := r.AdvanceCredentialStep(ctx, credID, firstStep)
				if err != nil || !won {
					return
				}
				accepted <- struct{}{}
			}()
		}
		close(start)
		wg.Wait()
		close(accepted)

		if n := len(accepted); n != 1 {
			t.Fatalf("round %d: the same code was accepted %d times; "+
				"a TOTP code may be accepted once", round, n)
		}
	}
}

// ownerOf returns the identity a credential belongs to.
func ownerOf(t *testing.T, credID string) string {
	t.Helper()
	r := repo(t)
	id, _, _, err := r.CredentialOwner(context.Background(), credID)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// A realm belongs to a tenant, and the UPDATE has to say so.
//
// The tenant was checked only by whoever loaded the record first, and the
// repository discarded the tenant it was handed — safe with the one caller
// that lists by tenant and scans, and a cross-tenant write the moment a
// second caller skips that. This drives the repository DIRECTLY, which is
// exactly the second caller that used to be unguarded.
func TestARealmCannotBeUpdatedFromAnotherTenant(t *testing.T) {
	r := repo(t)
	ctx := context.Background()

	realms, err := r.ListRealms(ctx, tenant)
	if err != nil || len(realms) == 0 {
		t.Fatalf("need a realm in the probe tenant: %v", err)
	}
	victim := realms[0]

	changed := victim
	changed.DisplayName = "owned by somebody else"
	if err := r.UpdateRealm(ctx, other, changed); err == nil {
		t.Fatal("a realm was updated by a tenant that does not own it")
	}

	after, err := r.ListRealms(ctx, tenant)
	if err != nil {
		t.Fatal(err)
	}
	for _, rl := range after {
		if rl.ID == victim.ID && rl.DisplayName != victim.DisplayName {
			t.Fatalf("the realm changed anyway: %q", rl.DisplayName)
		}
	}

	// And the owning tenant must still be able to update it, or the guard
	// has simply broken the feature.
	changed.DisplayName = victim.DisplayName + " (edited)"
	if err := r.UpdateRealm(ctx, tenant, changed); err != nil {
		t.Fatalf("the owning tenant could not update its own realm: %v", err)
	}
}
