//go:build integration

// Package tenancyconfig exercises the tenancy context against a real database.
//
// It exists because the context moved from sqlc to storm, and a query that
// COMPILES proves nothing about the rows it returns. The cases here are the
// ones where being wrong is not a wrong screen: the page a sign-in resolves
// to, the default that must not be deletable, and the interval round trip that
// decides how long a token lives.
//
//	ANUBIS_DB_URL=postgres://anubis:anubis@localhost:7449/anubis?sslmode=disable \
//	  go test -tags integration ./test/integration/tenancyconfig/
package tenancyconfig

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gsoultan/anubis/internal/platform/database"
	tenancypg "github.com/gsoultan/anubis/internal/tenancy/adapter/postgres"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	pool   *pgxpool.Pool
	tenant string
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
	slug := fmt.Sprintf("zzten%d", time.Now().UnixNano()%1_000_000_000)
	if err := p.QueryRow(ctx,
		`INSERT INTO tenants (slug, name) VALUES ($1, 'Tenancy probe') RETURNING id`,
		slug).Scan(&tenant); err != nil {
		panic("create probe tenant: " + err.Error())
	}
	code := m.Run()
	// Its own tenant, so the cleanup cannot reach anybody's real pages.
	_, _ = p.Exec(ctx, `DELETE FROM auth_pages WHERE tenant_id = $1`, tenant)
	_, _ = p.Exec(ctx, `DELETE FROM route_policies WHERE tenant_id = $1`, tenant)
	_, _ = p.Exec(ctx, `DELETE FROM applications WHERE tenant_id = $1`, tenant)
	_, _ = p.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenant)
	p.Close()
	os.Exit(code)
}

func repo(t *testing.T) *tenancypg.Repository {
	t.Helper()
	if pool == nil {
		t.Skip("ANUBIS_DB_URL not set")
	}
	return tenancypg.New(database.New(pool))
}

func newApp(t *testing.T, r *tenancypg.Repository) string {
	t.Helper()
	slug := fmt.Sprintf("zzapp%d", time.Now().UnixNano()%1_000_000_000)
	id, err := r.CreateApplication(context.Background(), tenant, tenancydomain.ApplicationRecord{
		Slug: slug, Name: "probe", Kind: "spa",
		RedirectURIs:    []string{"https://example.test/cb"},
		AccessTokenTTL:  "15 minutes",
		RefreshTokenTTL: "7 days",
	})
	if err != nil {
		t.Fatalf("create application: %v", err)
	}
	return id
}

// The TTLs are stored as interval and read back as text and as seconds. A
// conversion that drifted would change how long every token from this
// application lives.
func TestApplicationIntervalsRoundTrip(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	id := newApp(t, r)

	got, err := r.ApplicationByID(ctx, tenant, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessTokenTTL != "00:15:00" {
		t.Fatalf("access ttl came back %q, want 00:15:00", got.AccessTokenTTL)
	}
	if got.RefreshTokenTTL != "7 days" {
		t.Fatalf("refresh ttl came back %q, want 7 days", got.RefreshTokenTTL)
	}

	bySlug, err := r.ApplicationBySlug(ctx, tenant, got.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if bySlug.AccessTokenTTLSecs != 900 {
		t.Fatalf("access ttl seconds = %d, want 900", bySlug.AccessTokenTTLSecs)
	}
	if bySlug.RefreshTokenTTLSecs != 604800 {
		t.Fatalf("refresh ttl seconds = %d, want 604800", bySlug.RefreshTokenTTLSecs)
	}
}

// manifest_version + 1 is computed by the database. Two callers must not both
// publish the same generation.
func TestBumpManifestVersionIncrements(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	id := newApp(t, r)

	before, err := r.ApplicationByID(ctx, tenant, id)
	if err != nil {
		t.Fatal(err)
	}
	v1, err := r.BumpManifestVersion(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	v2, err := r.BumpManifestVersion(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if v1 != before.ManifestVersion+1 || v2 != v1+1 {
		t.Fatalf("versions went %d -> %d -> %d", before.ManifestVersion, v1, v2)
	}
}

// An empty kind means EVERY kind. It reaches the query as NULL, not ”: ”
// would match no page and read as a tenant with nothing configured.
func TestListAuthPagesKindFilter(t *testing.T) {
	r := repo(t)
	ctx := context.Background()

	for _, kind := range []string{"signin", "signout"} {
		if _, err := r.CreateAuthPage(ctx, tenant, tenancydomain.AuthPageInput{
			Kind: kind, Slug: kind + "-probe", Name: "probe",
		}); err != nil {
			t.Fatalf("create %s page: %v", kind, err)
		}
	}

	// A new tenant is SEEDED with a default page per kind (trigger
	// seed_auth_pages), so the counts below are relative to that rather than
	// to an empty tenant.
	all, err := r.ListAuthPages(ctx, tenant, "")
	if err != nil {
		t.Fatal(err)
	}
	signin, signout := 0, 0
	for _, p := range all {
		switch p.Kind {
		case "signin":
			signin++
		case "signout":
			signout++
		default:
			t.Fatalf("unfiltered list returned kind %q", p.Kind)
		}
	}
	if signin < 1 || signout < 1 || len(all) != signin+signout {
		t.Fatalf("unfiltered list = %d pages (%d signin, %d signout)", len(all), signin, signout)
	}

	only, err := r.ListAuthPages(ctx, tenant, "signin")
	if err != nil {
		t.Fatal(err)
	}
	if len(only) != signin {
		t.Fatalf("kind filter returned %d signin pages, want %d", len(only), signin)
	}
	for _, p := range only {
		if p.Kind != "signin" {
			t.Fatalf("kind filter leaked a %q page", p.Kind)
		}
	}

	none, err := r.ListAuthPages(ctx, tenant, "nosuchkind")
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Fatalf("a kind that matches nothing returned %d pages", len(none))
	}
}

// Promoting a default demotes the incumbent in the same transaction; the
// partial unique index allows exactly one, so a swap that is not atomic is a
// constraint violation. And the default must not be deletable.
func TestDefaultAuthPageSwapsAndResistsDeletion(t *testing.T) {
	r := repo(t)
	ctx := context.Background()

	first, err := r.CreateAuthPage(ctx, tenant, tenancydomain.AuthPageInput{
		Kind: "signin", Slug: "first-default", Name: "first",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.CreateAuthPage(ctx, tenant, tenancydomain.AuthPageInput{
		Kind: "signin", Slug: "second-default", Name: "second",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := r.SetDefaultAuthPage(ctx, tenant, "signin", first); err != nil {
		t.Fatalf("promote first: %v", err)
	}
	def, err := r.DefaultAuthPage(ctx, tenant, "signin")
	if err != nil {
		t.Fatal(err)
	}
	if def.ID != first {
		t.Fatalf("default is %s, want %s", def.ID, first)
	}

	// The swap. If the demotion were not in the same transaction as the
	// promotion, this is where the unique index would refuse.
	if err := r.SetDefaultAuthPage(ctx, tenant, "signin", second); err != nil {
		t.Fatalf("swap default: %v", err)
	}
	def, err = r.DefaultAuthPage(ctx, tenant, "signin")
	if err != nil {
		t.Fatal(err)
	}
	if def.ID != second {
		t.Fatalf("after swap the default is %s, want %s", def.ID, second)
	}

	// The default is not deletable: /v1/authorize must always have a page.
	if err := r.DeleteAuthPage(ctx, tenant, second); err == nil {
		t.Fatal("the default page was deleted")
	}
	// A non-default one is.
	if err := r.DeleteAuthPage(ctx, tenant, first); err != nil {
		t.Fatalf("deleting a non-default page failed: %v", err)
	}
	if err := r.DeleteAuthPage(ctx, tenant, second); err == nil {
		t.Fatal("the default page was deleted on the second attempt")
	}
}

// A page bound to an application resolves for that application and NOT for
// another; the binding is what the resolver tries before the tenant default.
func TestAuthPageForApplicationIsScoped(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	appA := newApp(t, r)
	appB := newApp(t, r)

	bound, err := r.CreateAuthPage(ctx, tenant, tenancydomain.AuthPageInput{
		Kind: "signin", Slug: "app-bound", Name: "bound", ApplicationID: appA,
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := r.AuthPageForApplication(ctx, tenant, "signin", appA)
	if err != nil {
		t.Fatalf("resolve for its own application: %v", err)
	}
	if got.ID != bound {
		t.Fatalf("resolved %s, want %s", got.ID, bound)
	}
	if got.ApplicationID != appA {
		t.Fatalf("resolved page carries application %q, want %s", got.ApplicationID, appA)
	}
	if _, err := r.AuthPageForApplication(ctx, tenant, "signin", appB); err == nil {
		t.Fatal("another application resolved to a page bound to the first")
	}
}

// Replacing a rule set is one transaction: the rules are evaluated in priority
// order, so a partially applied set is a different policy.
func TestReplaceRoutePoliciesIsAtomicAndOrdered(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	app := newApp(t, r)

	in := []tenancydomain.RoutePolicyInput{
		{Priority: 10, Effect: "public", PathPattern: "/health"},
		{Priority: 20, Effect: "require_auth", PathPattern: "/api/*", HostPattern: "api.example.test"},
		{Priority: 30, Effect: "deny", PathPattern: "/admin/*"},
	}
	if err := r.ReplaceRoutePolicies(ctx, tenant, app, in); err != nil {
		t.Fatal(err)
	}
	got, err := r.ListRoutePolicies(ctx, app)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d policies, want 3", len(got))
	}
	for i := range got {
		if got[i].Priority != in[i].Priority || got[i].Effect != in[i].Effect {
			t.Fatalf("policy %d is %+v, want %+v", i, got[i], in[i])
		}
	}
	// An empty host pattern is "any host", stored as NULL and read back as "".
	if got[0].HostPattern != "" {
		t.Fatalf("an unset host pattern came back %q", got[0].HostPattern)
	}
	if got[1].HostPattern != "api.example.test" {
		t.Fatalf("host pattern came back %q", got[1].HostPattern)
	}

	// Replacing with fewer rules must leave exactly the new set.
	if err := r.ReplaceRoutePolicies(ctx, tenant, app, in[:1]); err != nil {
		t.Fatal(err)
	}
	got, err = r.ListRoutePolicies(ctx, app)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("after replacing with one rule there are %d", len(got))
	}
}

// Backchannel notification goes to active applications that asked for it, and
// to nobody else.
func TestBackchannelAppsExcludesDisabledAndUnset(t *testing.T) {
	r := repo(t)
	ctx := context.Background()

	withURI := newApp(t, r)
	rec, err := r.ApplicationByID(ctx, tenant, withURI)
	if err != nil {
		t.Fatal(err)
	}
	rec.BackchannelLogoutURI = "https://example.test/logout"
	if err := r.UpdateApplication(ctx, tenant, *rec); err != nil {
		t.Fatal(err)
	}
	newApp(t, r) // no URI at all

	slugs, uris, err := r.BackchannelApps(ctx, tenant)
	if err != nil {
		t.Fatal(err)
	}
	if len(slugs) != 1 || len(uris) != 1 {
		t.Fatalf("got %d backchannel apps, want 1", len(slugs))
	}
	if uris[0] != "https://example.test/logout" {
		t.Fatalf("uri came back %q", uris[0])
	}

	// Disabling it takes it out, which is the point of the status filter.
	rec.Status = "disabled"
	if err := r.UpdateApplication(ctx, tenant, *rec); err != nil {
		t.Fatal(err)
	}
	slugs, _, err = r.BackchannelApps(ctx, tenant)
	if err != nil {
		t.Fatal(err)
	}
	if len(slugs) != 0 {
		t.Fatalf("a disabled application still receives backchannel logout")
	}
}
