// Package tenancyrmodel declares the tenancy tables storm generates code for.
//
// It is a PROJECTION of the schema, not its source of truth: anubis's schema
// of record stays migrations/ (forward-only, checksummed), and this model is
// validated against the live schema the same way the raw queries are — by
// preparing generated statements against it. Columns and defaults here must
// match `\d` on the table exactly.
//
// Two tables this context reads are NOT modelled here. `permissions` belongs
// to authz and `realms` to identity; a route policy and an auth page each
// carry one as a plain column rather than a relation, because modelling
// another context's table would put its shape in this package's hands — the
// boundary AGENTS.md draws. Their foreign keys live in migrations/, which is
// where every constraint lives.
//
// `signin_pages` is absent because it is a VIEW over auth_pages, not a table.
package tenancyrmodel

import (
	"time"

	"github.com/gsoultan/storm"
)

// Tenant is public.tenants. There is no delete path: every grant, identity,
// scope node and audit record in the installation hangs off this row, so
// 'archived' is what deletion means here.
type Tenant struct {
	storm.Model

	Status   string
	Slug     string
	Name     string
	Settings storm.JSON
}

func (m *Tenant) Schema(t *storm.Table) {
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.Status).Default("'active'::text")
	t.Col(&m.Settings).Default("'{}'::jsonb")
	t.UniqueNamed("tenants_slug_key", &m.Slug)
	t.CheckNamed("tenants_slug_check", "slug ~ '^[a-z0-9][a-z0-9_-]{1,62}$'::text")
	t.CheckNamed("tenants_status_check",
		"status = ANY (ARRAY['active'::text, 'suspended'::text, 'archived'::text])")
}

// Application is public.applications: a tenant's relying parties, the things
// its people sign in to. Since 0029 nothing else lives here — Anubis itself
// registers no applications anywhere.
type Application struct {
	storm.Model

	AccessTokenTtl  storm.Interval
	RefreshTokenTtl storm.Interval
	Tenant          Tenant

	// ManifestVersion is bumped whenever the application's public shape
	// changes, so a client can tell it has stale configuration.
	ManifestVersion int32

	Kind                   string
	Status                 string
	Slug                   string
	Name                   string
	ClientSecretHash       *string
	RedirectUris           []string
	BackchannelLogoutURI   *string
	TokenFormat            string
	PostLogoutRedirectUris []string
}

func (m *Application) Schema(t *storm.Table) {
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.AccessTokenTtl).Default("'00:10:00'::interval")
	t.Col(&m.RefreshTokenTtl).Default("'30 days'::interval")
	t.Col(&m.ManifestVersion).Default("0")
	t.Col(&m.Status).Default("'active'::text")
	t.Col(&m.RedirectUris).Default("'{}'::text[]")
	t.Col(&m.TokenFormat).Default("'v4.public'::text")
	t.Col(&m.PostLogoutRedirectUris).Default("'{}'::text[]")
	t.Col(&m.Tenant).ConstraintName("applications_tenant_id_fkey")
	t.Col(&m.Tenant).OnDelete(storm.Restrict)

	// (id, tenant_id) is unique so that CHILD tables can carry a composite
	// key back to it and keep the tenant in the reference — which is what
	// stops a page in one tenant binding an application in another.
	t.UniqueNamed("applications_id_tenant_id_key", &m.ID, &m.Tenant)
	t.UniqueNamed("applications_id_slug_key", &m.ID, &m.Slug)
	t.UniqueNamed("applications_tenant_id_slug_key", &m.Tenant, &m.Slug)

	t.CheckNamed("applications_kind_check",
		"kind = ANY (ARRAY['spa'::text, 'native'::text, 'server'::text, 'service'::text])")
	t.CheckNamed("applications_slug_check", "slug ~ '^[a-z0-9][a-z0-9_-]{1,62}$'::text")
	t.CheckNamed("applications_status_check",
		"status = ANY (ARRAY['active'::text, 'disabled'::text])")
	t.CheckNamed("applications_token_format_check",
		"token_format = ANY (ARRAY['v4.public'::text, 'jws.eddsa'::text])")
}

// RoutePolicy is public.route_policies: the gate's rules for one application,
// evaluated in priority order.
type RoutePolicy struct {
	CreatedAt     time.Time
	ID            [16]byte
	ApplicationID [16]byte
	TenantID      [16]byte

	// PermissionID belongs to authz; see the package comment.
	PermissionID  *[16]byte
	Priority      int32
	Effect        string
	PathPattern   string
	HostPattern   *string
	Methods       []string
	ScopeBindings storm.JSON
}

func (m *RoutePolicy) Schema(t *storm.Table) {
	var app Application

	t.Name("route_policies")
	t.PrimaryKey(&m.ID)
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.CreatedAt).Default("now()")
	t.Col(&m.Methods).Default("'{*}'::text[]")
	t.Col(&m.ScopeBindings).Default("'{}'::jsonb")

	t.UniqueNamed("route_policies_application_id_priority_key", &m.ApplicationID, &m.Priority)
	// A rule that requires a permission must NAME one, or the gate has a rule
	// it cannot evaluate and would have to decide what to do at request time.
	t.CheckNamed("route_policies_check",
		"(effect <> 'require_permission'::text) OR (permission_id IS NOT NULL)")
	t.CheckNamed("route_policies_effect_check",
		"effect = ANY (ARRAY['public'::text, 'require_auth'::text, 'require_permission'::text, 'deny'::text])")
	t.Index(&m.ApplicationID, &m.Priority).Named("route_policies_app")

	// Composite, so a policy cannot point at an application in another tenant.
	t.ForeignKey(&m.ApplicationID, &m.TenantID).
		References(&app, &app.ID, &app.Tenant).
		Named("route_policies_application_id_tenant_id_fkey").
		OnDelete(storm.Cascade)
}

// AuthPage is public.auth_pages: the sign-in and sign-out pages a tenant
// serves. A page may be bound to one application OR one realm, never both,
// and exactly one per kind is the tenant default.
type AuthPage struct {
	storm.Model

	Tenant        Tenant
	ApplicationID *[16]byte

	// RealmID belongs to identity; see the package comment.
	RealmID   *[16]byte
	IsDefault bool
	Kind      string
	Status    string
	Slug      string
	Name      string
	Config    storm.JSON
}

func (m *AuthPage) Schema(t *storm.Table) {
	var app Application

	t.Name("auth_pages")
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.IsDefault).Default("false")
	t.Col(&m.Status).Default("'active'::text")
	t.Col(&m.Config).Default("'{}'::jsonb")
	t.Col(&m.Tenant).ConstraintName("auth_pages_tenant_id_fkey")
	t.Col(&m.Tenant).OnDelete(storm.Cascade)

	t.UniqueNamed("auth_pages_tenant_id_kind_slug_key", &m.Tenant, &m.Kind, &m.Slug)
	t.CheckNamed("auth_pages_kind_check",
		"kind = ANY (ARRAY['signin'::text, 'signout'::text])")
	t.CheckNamed("auth_pages_one_binding",
		"(application_id IS NULL) OR (realm_id IS NULL)")
	t.CheckNamed("auth_pages_slug_check", "slug ~ '^[a-z0-9][a-z0-9_-]{1,62}$'::text")
	t.CheckNamed("auth_pages_status_check",
		"status = ANY (ARRAY['active'::text, 'disabled'::text])")

	t.Index(&m.Tenant, &m.Kind, &m.Status).Named("auth_pages_lookup")
	// Three partial uniques: one default per kind, one page per application,
	// one per realm. Partial because a page that is not the default, or not
	// bound, must not occupy the slot.
	t.Index(&m.Tenant, &m.Kind).Unique().Where("is_default").Named("auth_pages_one_default")
	t.Index(&m.ApplicationID, &m.Kind).Unique().
		Where("application_id IS NOT NULL").Named("auth_pages_one_per_app")
	t.Index(&m.RealmID, &m.Kind).Unique().
		Where("realm_id IS NOT NULL").Named("auth_pages_one_per_realm")

	// Composite, so a page cannot bind an application from another tenant.
	// The realm key is the same shape and lives in migrations/, because the
	// realms table is not modelled here.
	t.ForeignKey(&m.ApplicationID, &m.Tenant).
		References(&app, &app.ID, &app.Tenant).
		Named("auth_pages_application_id_tenant_id_fkey").
		OnDelete(storm.SetNull)
}

// CatalogVersion is public.catalog_version: the invalidation counter a gate
// snapshot is built from. One row per tenant, bumped by triggers.
type CatalogVersion struct {
	ChangedAt time.Time
	Version   int64
	Tenant    Tenant
}

func (m *CatalogVersion) Schema(t *storm.Table) {
	t.Name("catalog_version")
	t.PrimaryKey(&m.Tenant)
	t.Col(&m.ChangedAt).Default("now()")
	t.Col(&m.Version).Default("1")
	t.Col(&m.Tenant).ConstraintName("catalog_version_tenant_id_fkey")
	t.Col(&m.Tenant).OnDelete(storm.Cascade)
}

func All() []any {
	return []any{&Tenant{}, &Application{}, &RoutePolicy{}, &AuthPage{}, &CatalogVersion{}}
}
