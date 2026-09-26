//go:build integration

// Package identitypeople covers the identity context against a real database.
//
// The cases here are the ones where being wrong is not a wrong screen: the
// case-insensitive login lookup, the epoch the database increments, the
// erasure that must be atomic, and the two filters that were once scoped by
// the wrong column and answered with another tenant's data.
package identitypeople

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	identitypg "github.com/gsoultan/anubis/internal/identity/adapter/postgres"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	credentialdomain "github.com/gsoultan/anubis/internal/identity/domain/credential"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	pool    *pgxpool.Pool
	tenant  string
	other   string
	realmID string
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
	n := time.Now().UnixNano()
	mk := func(slug string) string {
		var id string
		if err := p.QueryRow(ctx,
			`INSERT INTO tenants (slug, name) VALUES ($1, 'Identity probe') RETURNING id`,
			slug).Scan(&id); err != nil {
			panic("create probe tenant: " + err.Error())
		}
		return id
	}
	tenant = mk(fmt.Sprintf("zzid%d", n%1_000_000_000))
	other = mk(fmt.Sprintf("zzid%d", (n+1)%1_000_000_000))
	if err := p.QueryRow(ctx,
		`INSERT INTO realms (tenant_id, code, kind, display_name)
		 VALUES ($1, 'probe', 'internal', 'Probe') RETURNING id`,
		tenant).Scan(&realmID); err != nil {
		panic("create probe realm: " + err.Error())
	}
	code := m.Run()
	for _, t := range []string{tenant, other} {
		for _, q := range []string{
			`DELETE FROM credentials WHERE tenant_id = $1`,
			`DELETE FROM consents WHERE tenant_id = $1`,
			`DELETE FROM identities WHERE tenant_id = $1`,
			`DELETE FROM realm_categories WHERE tenant_id = $1`,
			`DELETE FROM realms WHERE tenant_id = $1`,
			`DELETE FROM tenants WHERE id = $1`,
		} {
			_, _ = p.Exec(ctx, q, t)
		}
	}
	p.Close()
	os.Exit(code)
}

func repo(t *testing.T) *identitypg.Repository {
	t.Helper()
	if pool == nil {
		t.Skip("ANUBIS_DB_URL not set")
	}
	return identitypg.New(database.New(pool))
}

func newIdentity(t *testing.T, r *identitypg.Repository, username string) string {
	t.Helper()
	id, err := r.CreateIdentity(context.Background(), identitydomain.IdentityCreate{
		TenantID: tenant, RealmID: realmID, Username: username,
		Email: username + "@example.test", AssuranceLevel: 1, Status: "active",
	})
	if err != nil {
		t.Fatalf("create identity: %v", err)
	}
	return id
}

// The login lookup matches lower(username), which is the expression the unique
// index is built on. A comparison that lost the lower() would find the row
// typed exactly and fail for everybody else.
func TestIdentityForLoginIsCaseInsensitive(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	id := newIdentity(t, r, fmt.Sprintf("zzperson%d", time.Now().UnixNano()%1_000_000))

	got, err := r.Identity(ctx, tenant, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, probe := range []string{got.Username, upper(got.Username)} {
		found, err := r.IdentityForLogin(ctx, tenant, realmID, probe)
		if err != nil {
			t.Fatalf("lookup %q: %v", probe, err)
		}
		if found.ID != id {
			t.Fatalf("lookup %q returned %s, want %s", probe, found.ID, id)
		}
		// The realm's policy rides along, which is what saves the hot path a
		// second round trip.
		if found.RealmCode != "probe" {
			t.Fatalf("realm did not ride along: %+v", found)
		}
	}
	if _, err := r.IdentityForLogin(ctx, tenant, realmID, "zznosuchperson"); err == nil {
		t.Fatal("a username that does not exist resolved")
	}
}

func upper(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'a' && b[i] <= 'z' {
			b[i] -= 32
		}
	}
	return string(b)
}

// token_epoch + 1 is computed by the DATABASE: in Go it is a
// read-modify-write, and two concurrent revocations would leave one set of
// tokens live.
func TestBumpTokenEpochIncrementsInTheDatabase(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	id := newIdentity(t, r, fmt.Sprintf("zzepoch%d", time.Now().UnixNano()%1_000_000))

	before, err := r.Identity(ctx, tenant, id)
	if err != nil {
		t.Fatal(err)
	}
	e1, err := r.BumpTokenEpoch(ctx, tenant, id)
	if err != nil {
		t.Fatal(err)
	}
	e2, err := r.BumpTokenEpoch(ctx, tenant, id)
	if err != nil {
		t.Fatal(err)
	}
	if e1 != before.TokenEpoch+1 || e2 != e1+1 {
		t.Fatalf("epoch went %d -> %d -> %d", before.TokenEpoch, e1, e2)
	}
}

// Disabling is guarded, so the second attempt reports that it changed nothing.
func TestDisableAndEnableAreGuarded(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	id := newIdentity(t, r, fmt.Sprintf("zzdis%d", time.Now().UnixNano()%1_000_000))

	if err := r.DisableIdentity(ctx, tenant, id); err != nil {
		t.Fatal(err)
	}
	got, err := r.Identity(ctx, tenant, id)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Disabled || got.Status != "disabled" {
		t.Fatalf("after disabling: %+v", got)
	}
	if err := r.DisableIdentity(ctx, tenant, id); err == nil {
		t.Fatal("an already-disabled identity was disabled again")
	}
	if err := r.EnableIdentity(ctx, tenant, id); err != nil {
		t.Fatal(err)
	}
	got, err = r.Identity(ctx, tenant, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Disabled || got.Status != "active" {
		t.Fatalf("after enabling: %+v", got)
	}
}

// Right-to-erasure: the identifiers are blanked and the epoch bumped in ONE
// statement. Blanking without bumping would leave live tokens for a person who
// no longer exists.
func TestAnonymizeIsAtomicAndRefusesARepeat(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	id := newIdentity(t, r, fmt.Sprintf("zzanon%d", time.Now().UnixNano()%1_000_000))

	before, err := r.Identity(ctx, tenant, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Anonymize(ctx, tenant, id); err != nil {
		t.Fatal(err)
	}
	after, err := r.Identity(ctx, tenant, id)
	if err != nil {
		t.Fatal(err)
	}
	if !after.Anonymized {
		t.Fatal("the identity is not marked anonymised")
	}
	if after.Email != "" {
		t.Fatalf("email survived erasure: %q", after.Email)
	}
	if after.Username == before.Username {
		t.Fatalf("username survived erasure: %q", after.Username)
	}
	if after.TokenEpoch <= before.TokenEpoch {
		t.Fatalf("epoch did not move: %d -> %d — outstanding tokens would survive",
			before.TokenEpoch, after.TokenEpoch)
	}

	// A second erasure is an ERROR, not a quiet success. The caller emits an
	// identity.erased audit event on the non-error path, and that record is
	// the compliance evidence — a second "erased" entry for an erasure that
	// did not happen is a false entry in the trail.
	if _, err := r.Anonymize(ctx, tenant, id); err == nil {
		t.Fatal("a second erasure reported success, which would log a second identity.erased event")
	}
}

// ListCredentials is scoped by TENANT as well as identity. It was the second
// predicate and never the first, so an admin RPC carrying somebody else's
// identity id answered with their inventory of how they sign in.
func TestListCredentialsIsTenantScoped(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	id := newIdentity(t, r, fmt.Sprintf("zzcred%d", time.Now().UnixNano()%1_000_000))

	if _, err := r.CreateCredential(ctx, credentialdomain.CredentialInput{
		IdentityID: id, TenantID: tenant, Kind: "password", Secret: "hash",
	}); err != nil {
		t.Fatalf("create credential: %v", err)
	}

	mine, err := r.ListCredentials(ctx, tenant, id, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(mine) != 1 {
		t.Fatalf("own tenant sees %d credentials, want 1", len(mine))
	}

	// The SAME identity id, read as the OTHER tenant, must see nothing.
	theirs, err := r.ListCredentials(ctx, other, id, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(theirs) != 0 {
		t.Fatalf("another tenant read %d of this identity's credentials", len(theirs))
	}
}

// ListRealmCategories is tenant-scoped ALWAYS. It used to key on realm_id
// alone, so an operator who learned another tenant's realm id read that
// tenant's categories.
func TestListRealmCategoriesIsTenantScoped(t *testing.T) {
	r := repo(t)
	ctx := context.Background()

	code := fmt.Sprintf("zzc%d", time.Now().UnixNano()%1_000_000)
	if _, err := r.CreateRealmCategory(ctx, tenant, identitydomain.RealmCategoryRecord{
		RealmID: realmID, Code: code, DisplayName: "Probe", SortOrder: 10,
	}); err != nil {
		t.Fatal(err)
	}

	mine, err := r.ListRealmCategories(ctx, tenant, realmID)
	if err != nil {
		t.Fatal(err)
	}
	if len(mine) != 1 {
		t.Fatalf("own tenant sees %d categories, want 1", len(mine))
	}

	// The same REALM id, asked for by the other tenant.
	theirs, err := r.ListRealmCategories(ctx, other, realmID)
	if err != nil {
		t.Fatal(err)
	}
	if len(theirs) != 0 {
		t.Fatalf("another tenant read %d categories of this realm", len(theirs))
	}

	// And a tenant-wide listing — realm '' — must not be an invalid uuid.
	all, err := r.ListRealmCategories(ctx, tenant, "")
	if err != nil {
		t.Fatalf("a tenant-wide category listing failed: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("tenant-wide listing sees %d categories, want 1", len(all))
	}
}

// CorrectEmptyRealmIdentity tests emptiness in the STATEMENT, so an identity
// created between a check and an update cannot slip through.
func TestCorrectEmptyRealmRefusesAPopulatedRealm(t *testing.T) {
	r := repo(t)
	ctx := context.Background()

	var emptyRealm string
	code := fmt.Sprintf("zze%d", time.Now().UnixNano()%1_000_000)
	if err := pool.QueryRow(ctx,
		`INSERT INTO realms (tenant_id, code, kind, display_name)
		 VALUES ($1, $2, 'internal', 'Empty') RETURNING id`,
		tenant, code).Scan(&emptyRealm); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM realms WHERE id = $1`, emptyRealm); err != nil {
			t.Errorf("realm %v not removed: %v", emptyRealm, err)
		}
	})

	ok, err := r.CorrectEmptyRealmIdentity(ctx, tenant, emptyRealm, code+"x", "internal")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("an empty realm's code could not be corrected")
	}

	// The seeded realm has members, so it must refuse.
	newIdentity(t, r, fmt.Sprintf("zzpop%d", time.Now().UnixNano()%1_000_000))
	ok, err = r.CorrectEmptyRealmIdentity(ctx, tenant, realmID, "renamed", "internal")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("a populated realm's code was corrected — grants already made would be re-decided")
	}
}
