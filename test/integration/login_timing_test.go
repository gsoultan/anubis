//go:build integration

package integration

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	authpg "github.com/gsoultan/anubis/internal/auth/adapter/postgres"
	"github.com/gsoultan/anubis/internal/auth/app/signin"
	controlpg "github.com/gsoultan/anubis/internal/control/adapter/postgres"
	controlapp "github.com/gsoultan/anubis/internal/control/app"
	identitypg "github.com/gsoultan/anubis/internal/identity/adapter/postgres"
	"github.com/gsoultan/anubis/internal/platform/crypto/kdf"
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

	// The uniform-timing rule lives in the authenticator now, which is the
	// piece both login doors call — so this measures the hosted sign-in page
	// as much as it measures AuthService.Login.
	passwords := signin.NewPasswordAuthenticator(tenancy, identity, identity,
		identity, systemClock{}, nopAuditor{})
	login := signin.NewLoginInteractor(passwords,
		signin.NewEnrolmentGranter(keyring.NewManager(ring), systemClock{}), identity,
		auth, auth, nopIssuer{}, keyring.NewManager(ring), auth, systemClock{})

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

// The same property, on the plane that administers the whole installation.
//
// There are three kdf.Verify call sites in the tree. Two default the hash to
// kdf.Dummy() when the account does not exist; the control plane passes
// whatever its repository returned, and PlatformUserByUsername returns an
// EMPTY STRING for an unknown operator. kdf.Verify parses the encoded hash
// before doing any work and returns on a parse error, so an empty one costs
// nothing at all.
//
// The comment above that call says: "Verify even when nobody was found, so a
// missing account and a wrong password cost the same time and cannot be told
// apart from outside." It does call Verify. It calls it against nothing.
//
// Platform operators are the highest-privilege accounts Anubis has, and their
// usernames are what somebody collects before spraying passwords at them.
// Rate limits do not help: enumeration varies the USERNAME, so a per-account
// limit never trips, and a thousand-fold gap needs very few samples.
func TestPlatformLoginTimingDoesNotRevealOperatorExistence(t *testing.T) {
	skipWithoutDB(t)
	if testing.Short() {
		t.Skip("timing histogram runs full-cost KDF")
	}
	ctx := context.Background()
	db := database.New(pool)
	control := controlpg.New(db)

	local, err := keyring.GenerateLocalKey(time.Now(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	local.Status = keyring.StatusActive
	ring, err := keyring.NewRing([]*keyring.Key{local})
	if err != nil {
		t.Fatal(err)
	}
	// Neither path reaches a session: one has the wrong password, the other
	// has no account. The keyring and master key only satisfy the
	// constructor.
	auth := controlapp.NewPlatformAuthInteractor(control, control,
		tenancypg.New(db), control,
		keyring.NewManager(ring), systemClock{}, nopAuditor{},
		"http://localhost:7448", make([]byte, 32))

	var existing string
	if err := pool.QueryRow(ctx,
		`SELECT username FROM platform_users WHERE status='active' ORDER BY created_at LIMIT 1`).
		Scan(&existing); err != nil {
		t.Skipf("no platform user seeded: %v", err)
	}

	const samples = 24
	measure := func(username string) []time.Duration {
		out := make([]time.Duration, 0, samples)
		for i := 0; i < samples; i++ {
			start := time.Now()
			_, _ = auth.Login(ctx, username, "definitely-not-the-password")
			out = append(out, time.Since(start))
		}
		sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
		return out
	}
	real := measure(existing)
	fake := measure(fmt.Sprintf("nobody-%d", time.Now().UnixNano()))
	mr, mf := real[len(real)/2], fake[len(fake)/2]
	pct90 := func(d []time.Duration) time.Duration { return d[(len(d)*9)/10] }

	t.Logf("existing operator : median=%v p90=%v min=%v", mr, pct90(real), real[0])
	t.Logf("unknown operator  : median=%v p90=%v min=%v", mf, pct90(fake), fake[0])

	// The floor is the load-bearing check, as on the tenant plane: a path
	// that skipped the KDF collapses to ~0 while the other stays at PBKDF2
	// cost.
	if fake[0] < time.Millisecond {
		t.Fatalf("an unknown operator was rejected in %v — no KDF ran, so "+
			"every operator username in this installation is discoverable "+
			"by timing alone", fake[0])
	}
	hi, lo := mr, mf
	if lo > hi {
		hi, lo = mf, mr
	}
	if ratio := float64(hi-lo) / float64(hi); ratio > 0.30 {
		t.Fatalf("median gap %.1f%% (existing=%v unknown=%v) is a timing oracle",
			ratio*100, mr, mf)
	}
}

// A verified operator password at stale KDF cost is upgraded in place.
//
// Nothing in the API changes a platform password — PlatformAuthService offers
// login, MFA, refresh, logout and TOTP enrolment, and operator admin offers
// create, assign and set-status. There is no self-service change and no admin
// reset. So a hash written at install time keeps that day's iteration count
// for the life of the installation unless a successful login upgrades it,
// which makes this the ONLY migration path for the most privileged
// credentials in the system.
//
// Login discarded the rehash flag (`ok, _, _ :=`), exactly as the hosted
// sign-in page did before 2026-09-20.
func TestAStaleOperatorPasswordIsUpgradedOnLogin(t *testing.T) {
	skipWithoutDB(t)
	if testing.Short() {
		t.Skip("runs full-cost KDF")
	}
	ctx := context.Background()
	db := database.New(pool)
	control := controlpg.New(db)

	local, err := keyring.GenerateLocalKey(time.Now(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	local.Status = keyring.StatusActive
	ring, err := keyring.NewRing([]*keyring.Key{local})
	if err != nil {
		t.Fatal(err)
	}
	auth := controlapp.NewPlatformAuthInteractor(control, control,
		tenancypg.New(db), control,
		keyring.NewManager(ring), systemClock{}, nopAuditor{},
		"http://localhost:7448", make([]byte, 32))

	// A hash in the stored format at a cost far below the current default,
	// standing in for one written before the iteration count was raised.
	const password = "an-operator-password-from-2019"
	const staleIterations = 1000
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		t.Fatal(err)
	}
	dk, err := pbkdf2.Key(sha256.New, password, salt, staleIterations, 32)
	if err != nil {
		t.Fatal(err)
	}
	b64 := base64.RawStdEncoding
	stale := fmt.Sprintf("$pbkdf2-sha256$i=%d$%s$%s", staleIterations,
		b64.EncodeToString(salt), b64.EncodeToString(dk))

	username := fmt.Sprintf("stale-op-%d", time.Now().UnixNano())
	var id string
	if err := pool.QueryRow(ctx,
		`INSERT INTO platform_users (username, email, password_hash, status)
		 VALUES ($1, $2, $3, 'active') RETURNING id`,
		username, username+"@example.test", stale).Scan(&id); err != nil {
		t.Fatalf("seed operator: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM platform_users WHERE id = $1`, id); err != nil {
			t.Errorf("operator %v not removed: %v", username, err)
		}
	})

	// The sign-in itself is refused for having no assignment, which is a
	// separate property with its own test. What is asserted here is the
	// credential: a password that VERIFIED must not be left at a cost the
	// installation has already moved past.
	_, _ = auth.Login(ctx, username, password)

	var got string
	if err := pool.QueryRow(ctx,
		`SELECT password_hash FROM platform_users WHERE id = $1`, id).Scan(&got); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got == stale {
		t.Fatalf("a password verified at %d iterations was left there; the "+
			"installation's default is %d and no other path ever changes it",
			staleIterations, kdf.DefaultIterations)
	}
	if !strings.Contains(got, fmt.Sprintf("i=%d$", kdf.DefaultIterations)) {
		t.Fatalf("rehashed to something other than the current cost: %.40s", got)
	}
	// And the new hash must still accept the same password, or the upgrade
	// locked the operator out of an account nobody can reset.
	ok, _, err := kdf.Verify(password, got)
	if err != nil || !ok {
		t.Fatalf("the upgraded hash rejects the password it was made from (err=%v)", err)
	}
}
