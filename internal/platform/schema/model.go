// Package platformschema is anubis's SCHEMA OF RECORD.
//
// Every table, function, view, trigger and partition the database has is
// declared here, and `cmd/stormddl` turns a change to it into the next
// numbered file in migrations/. The model is what you edit; the migration is
// what you review and what runs.
//
// # Why this is one package and not seven
//
// Each bounded context already has an `rmodel` — but those are PROJECTIONS,
// deliberately partial: a context models the tables it queries and declares
// another context's tables as plain columns, because `scripts/check/
// context-boundary.sh` forbids one context's adapter importing another's.
// A composite foreign key that carries a tenant across that line —
// grants -> identities -> realms -> tenants — cannot be declared from inside
// either context. It can be declared here, where everything is visible.
//
// So there are two kinds of model and one schema. This package is the schema;
// the rmodels are views onto it. Neither can drift from the database without
// CI saying so: `stormddl -check` fails if this model and the live schema
// disagree, and `cmd/stormgen` PREPAREs every context's raw queries against
// that same schema. They are both anchored to the database, which is what
// makes two models safe rather than two sources of truth.
//
// # Reading a declaration
//
// A `t.Name(...)` line is not decoration: storm derives a table name from the
// Go type by naive pluralisation, and where English does not follow those
// rules — audit_log, catalog_version, scope_axes, role_permissions_effective —
// the name is pinned so a diff never proposes renaming a live table.
//
// Twenty-five of these tables also appear in a context's `rmodel`, where their
// invariants are written down against the queries that depend on them. Those
// declarations say so and do not repeat the prose. The twenty that appear only
// here are documented here, because there is nowhere else they are explained.
//
// # The migrations already applied
//
// migrations/0001 through 0046 predate this package and stay exactly as they
// are: forward-only, checksummed, already run. This model describes the schema
// they produce, and owns everything after them.

package platformschema

import (
	"net/netip"
	"time"

	"github.com/gsoultan/storm"
)

// APIKey is public.api_keys. The authrmodel package documents this table's
// invariants; this declaration is the DDL half of the same thing.
type APIKey struct {
	CreatedAt  time.Time
	ID         [16]byte
	Tenant     Tenant
	Label      string
	Lookup     string
	SecretHash string
	CreatedBy  *PlatformUser
	LastUsedAt *time.Time
	ExpiresAt  *time.Time
	RevokedAt  *time.Time
}

func (m *APIKey) Schema(t *storm.Table) {
	t.PrimaryKey(&m.ID)
	t.Name("api_keys")
	t.Col(&m.CreatedAt).Default("now()")
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.Tenant).ConstraintName("api_keys_tenant_id_fkey")
	t.Col(&m.Tenant).OnDelete(storm.Cascade)
	t.Col(&m.Label).Default("''::text")
	t.Col(&m.CreatedBy).Named("created_by")
	t.Col(&m.CreatedBy).ConstraintName("api_keys_created_by_fkey")
	t.Col(&m.CreatedBy).OnDelete(storm.SetNull)
	t.Col(&m.CreatedBy).NoIndex()
	t.Index(&m.Lookup).Unique().Where("revoked_at IS NULL").Named("api_keys_lookup")
	t.Index(&m.Tenant, &m.RevokedAt).Named("api_keys_tenant")
}

// Application is public.applications. The tenancyrmodel package documents this table's
// invariants; this declaration is the DDL half of the same thing.
type Application struct {
	storm.Model

	AccessTokenTtl         storm.Interval
	RefreshTokenTtl        storm.Interval
	Tenant                 Tenant
	ManifestVersion        int32
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
	t.Col(&m.AccessTokenTtl).Default("'00:10:00'::interval")
	t.Col(&m.RefreshTokenTtl).Default("'30 days'::interval")
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.Tenant).ConstraintName("applications_tenant_id_fkey")
	t.Col(&m.Tenant).OnDelete(storm.Restrict)
	t.Col(&m.ManifestVersion).Default("0")
	t.Col(&m.Status).Default("'active'::text")
	t.Col(&m.RedirectUris).Default("'{}'::text[]")
	t.Col(&m.TokenFormat).Default("'v4.public'::text")
	t.Col(&m.PostLogoutRedirectUris).Default("'{}'::text[]")
	t.UniqueNamed("applications_id_slug_key", &m.ID, &m.Slug)
	t.UniqueNamed("applications_id_tenant_id_key", &m.ID, &m.Tenant)
	t.UniqueNamed("applications_tenant_id_slug_key", &m.Tenant, &m.Slug)
	t.CheckNamed("applications_kind_check", "kind = ANY (ARRAY['spa'::text, 'native'::text, 'server'::text, 'service'::text])")
	t.CheckNamed("applications_slug_check", "slug ~ '^[a-z0-9][a-z0-9_-]{1,62}$'::text")
	t.CheckNamed("applications_status_check", "status = ANY (ARRAY['active'::text, 'disabled'::text])")
	t.CheckNamed("applications_token_format_check", "token_format = ANY (ARRAY['v4.public'::text, 'jws.eddsa'::text])")
}

// AuditLog is public.audit_log. The auditrmodel package documents this table's
// invariants; this declaration is the DDL half of the same thing.
type AuditLog struct {
	OccurredAt time.Time
	ID         [16]byte
	TenantID   [16]byte
	ActorID    *[16]byte
	TargetID   *[16]byte
	SessionID  *[16]byte
	Seq        int64
	Action     string
	Result     string
	ActorKind  string
	IP         *netip.Prefix
	Detail     storm.JSON
	PrevHash   []byte
	EntryHash  []byte
}

func (m *AuditLog) Schema(t *storm.Table) {
	t.PrimaryKey(&m.ID, &m.OccurredAt)
	t.Name("audit_log")
	t.Col(&m.OccurredAt).Default("now()")
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.ActorKind).Default("'identity'::text")
	t.Col(&m.Detail).Default("'{}'::jsonb")
	t.Col(&m.EntryHash).NotNull()
	t.CheckNamed("audit_log_result_check", "result = ANY (ARRAY['allow'::text, 'deny'::text, 'error'::text])")
	t.Index(&m.ActorID, storm.Desc(&m.OccurredAt)).Where("actor_id IS NOT NULL").Named("audit_log_actor")
	t.Index(&m.TenantID, &m.Action, storm.Desc(&m.OccurredAt)).Named("audit_log_tenant_action")
	t.Index(&m.TenantID, storm.Desc(&m.Seq)).Named("audit_log_tenant_seq")
	t.Index(&m.OccurredAt).Using(storm.BRIN).With("pages_per_range", "32").Named("audit_log_time_brin")
	t.PartitionBy(storm.RangePartition, &m.OccurredAt)
}

// AuthPage is public.auth_pages. The tenancyrmodel package documents this table's
// invariants; this declaration is the DDL half of the same thing.
type AuthPage struct {
	storm.Model

	Tenant        Tenant
	ApplicationID *[16]byte
	IsDefault     bool
	Kind          string
	Status        string
	Slug          string
	Name          string
	Config        storm.JSON
	RealmID       *[16]byte
}

func (m *AuthPage) Schema(t *storm.Table) {
	var ref0 Application
	var ref1 Realm
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.Tenant).ConstraintName("auth_pages_tenant_id_fkey")
	t.Col(&m.Tenant).OnDelete(storm.Cascade)
	t.Col(&m.IsDefault).Default("false")
	t.Col(&m.Status).Default("'active'::text")
	t.Col(&m.Config).Default("'{}'::jsonb")
	t.UniqueNamed("auth_pages_tenant_id_kind_slug_key", &m.Tenant, &m.Kind, &m.Slug)
	t.CheckNamed("auth_pages_kind_check", "kind = ANY (ARRAY['signin'::text, 'signout'::text])")
	t.CheckNamed("auth_pages_one_binding", "(application_id IS NULL) OR (realm_id IS NULL)")
	t.CheckNamed("auth_pages_slug_check", "slug ~ '^[a-z0-9][a-z0-9_-]{1,62}$'::text")
	t.CheckNamed("auth_pages_status_check", "status = ANY (ARRAY['active'::text, 'disabled'::text])")
	t.Index(&m.Tenant, &m.Kind, &m.Status).Named("auth_pages_lookup")
	t.Index(&m.Tenant, &m.Kind).Unique().Where("is_default").Named("auth_pages_one_default")
	t.Index(&m.ApplicationID, &m.Kind).Unique().Where("application_id IS NOT NULL").Named("auth_pages_one_per_app")
	t.Index(&m.RealmID, &m.Kind).Unique().Where("realm_id IS NOT NULL").Named("auth_pages_one_per_realm")
	t.ForeignKey(&m.ApplicationID, &m.Tenant).References(&ref0, &ref0.ID, &ref0.Tenant).Named("auth_pages_application_id_tenant_id_fkey").OnDelete(storm.SetNull)
	t.ForeignKey(&m.RealmID, &m.Tenant).References(&ref1, &ref1.ID, &ref1.Tenant).Named("auth_pages_realm_fkey").OnDelete(storm.SetNull)
}

// CatalogSyncRun is what one catalog sync attempt did.
//
// document_sha is the digest of the manifest that was applied: re-applying an
// unchanged document is a no-op the run log can prove, which is what makes a
// nightly sync safe to leave running.
type CatalogSyncRun struct {
	StartedAt   time.Time
	FinishedAt  *time.Time
	ID          [16]byte
	Source      CatalogSyncSource
	Tenant      Tenant
	Dry         bool
	Status      string
	Actor       string
	DocumentSha string
	Report      *storm.JSON
	Error       string
}

func (m *CatalogSyncRun) Schema(t *storm.Table) {
	t.PrimaryKey(&m.ID)
	t.Col(&m.StartedAt).Default("now()")
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.Source).ConstraintName("catalog_sync_runs_source_id_fkey")
	t.Col(&m.Source).OnDelete(storm.Cascade)
	t.Col(&m.Tenant).ConstraintName("catalog_sync_runs_tenant_id_fkey")
	t.Col(&m.Tenant).OnDelete(storm.Cascade)
	t.Col(&m.Tenant).NoIndex()
	t.Col(&m.Dry).Default("false")
	t.Col(&m.Status).Default("'running'::text")
	t.Col(&m.Actor).Default("'system'::text")
	t.Col(&m.DocumentSha).Default("''::text")
	t.Col(&m.Error).Default("''::text")
	t.CheckNamed("catalog_sync_runs_status_check", "status = ANY (ARRAY['running'::text, 'ok'::text, 'failed'::text, 'dry_run'::text, 'skipped'::text])")
	t.Index(&m.Source, storm.Desc(&m.StartedAt)).Named("catalog_sync_runs_by_source")
}

// CatalogSyncSource is a feed that keeps an application's permission catalog
// in step with what the application actually declares.
//
// The alternative is an operator retyping permission keys, which drifts the
// day somebody ships a new endpoint. next_run_at drives the scheduler the same
// way a scope sync source does.
type CatalogSyncSource struct {
	storm.Model

	LastRunAt       *time.Time
	NextRunAt       *time.Time
	Tenant          Tenant
	ApplicationID   [16]byte
	Kind            string
	Format          string
	Status          string
	Name            string
	Config          storm.JSON
	IntervalSeconds int32
}

func (m *CatalogSyncSource) Schema(t *storm.Table) {
	var ref0 Application
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.Tenant).ConstraintName("catalog_sync_sources_tenant_id_fkey")
	t.Col(&m.Tenant).OnDelete(storm.Cascade)
	t.Col(&m.Status).Default("'active'::text")
	t.Col(&m.IntervalSeconds).Default("0")
	t.UniqueNamed("catalog_sync_sources_tenant_id_name_key", &m.Tenant, &m.Name)
	t.CheckNamed("catalog_sync_sources_format_check", "format = ANY (ARRAY['json'::text, 'csv'::text])")
	t.CheckNamed("catalog_sync_sources_interval_seconds_check", "(interval_seconds = 0) OR (interval_seconds >= 300)")
	t.CheckNamed("catalog_sync_sources_kind_check", "kind = 'http'::text")
	t.CheckNamed("catalog_sync_sources_status_check", "status = ANY (ARRAY['active'::text, 'disabled'::text])")
	t.Index(&m.NextRunAt).Where("(status = 'active'::text) AND (next_run_at IS NOT NULL)").Named("catalog_sync_sources_due")
	t.ForeignKey(&m.ApplicationID, &m.Tenant).References(&ref0, &ref0.ID, &ref0.Tenant).Named("catalog_sync_sources_application_id_tenant_id_fkey").OnDelete(storm.Cascade).NoIndex()
}

// CatalogVersion is public.catalog_version. The tenancyrmodel package documents this table's
// invariants; this declaration is the DDL half of the same thing.
type CatalogVersion struct {
	ChangedAt time.Time
	Version   int64
	Tenant    Tenant
}

func (m *CatalogVersion) Schema(t *storm.Table) {
	t.PrimaryKey(&m.Tenant)
	t.Name("catalog_version")
	t.Col(&m.ChangedAt).Default("now()")
	t.Col(&m.Version).Default("1")
	t.Col(&m.Tenant).ConstraintName("catalog_version_tenant_id_fkey")
	t.Col(&m.Tenant).OnDelete(storm.Cascade)
}

// Consent is public.consents. The identityrmodel package documents this table's
// invariants; this declaration is the DDL half of the same thing.
type Consent struct {
	GrantedAt     time.Time
	WithdrawnAt   *time.Time
	ExpiresAt     *time.Time
	ID            [16]byte
	TenantID      [16]byte
	IdentityID    [16]byte
	Purpose       string
	PolicyVersion string
	Evidence      storm.JSON
}

func (m *Consent) Schema(t *storm.Table) {
	var ref0 Identity
	t.PrimaryKey(&m.ID)
	t.Col(&m.GrantedAt).Default("now()")
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.Evidence).Default("'{}'::jsonb")
	t.Index(&m.IdentityID, &m.Purpose).Where("withdrawn_at IS NULL").Named("consents_identity")
	t.ForeignKey(&m.IdentityID, &m.TenantID).References(&ref0, &ref0.ID, &ref0.Tenant).Named("consents_identity_id_tenant_id_fkey").OnDelete(storm.Cascade)
}

// Credential is public.credentials. The identityrmodel package documents this table's
// invariants; this declaration is the DDL half of the same thing.
type Credential struct {
	storm.Model

	LastUsedAt  *time.Time
	ExpiresAt   *time.Time
	RevokedAt   *time.Time
	IdentityID  [16]byte
	TenantID    [16]byte
	SignCounter int64
	Kind        string
	Secret      *string
	// SecretKid names the local key Secret was sealed under. NULL means the
	// row predates the column: the reader falls back to whichever local key
	// is active and records the answer, because a secret sealed once and read
	// for years cannot depend on which key happens to be active today.
	//
	// Without it, `keys promote local` unsealed nothing and every enrolled
	// second factor stopped working at once.
	SecretKid *string
	LookupKey *string
	Label     *string
	Params    storm.JSON
}

func (m *Credential) Schema(t *storm.Table) {
	var ref0 Identity
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.SignCounter).Default("0")
	t.Col(&m.Params).Default("'{}'::jsonb")
	t.Col(&m.SecretKid).Size(64)
	t.CheckNamed("credentials_kind_check", "kind = ANY (ARRAY['password'::text, 'device_key'::text, 'totp'::text, 'recovery_code'::text, 'oidc_link'::text])")
	t.Index(&m.IdentityID, &m.Kind).Where("revoked_at IS NULL").Named("credentials_identity")
	t.Index(&m.LookupKey).Unique().Where("(lookup_key IS NOT NULL) AND (revoked_at IS NULL)").Named("credentials_lookup")
	t.Index(&m.IdentityID).Unique().Where("(kind = 'password'::text) AND (revoked_at IS NULL)").Named("credentials_one_password")
	t.ForeignKey(&m.IdentityID, &m.TenantID).References(&ref0, &ref0.ID, &ref0.Tenant).Named("credentials_identity_id_tenant_id_fkey").OnDelete(storm.Cascade)
}

// GrantScope binds a grant to a place in one axis's tree.
//
// mode is 'include' or 'exclude' and the two are not symmetric: a grant with
// no rows on an axis reaches everywhere on it, includes narrow it to the named
// subtrees, and an exclude carves a hole out of what the includes reached. The
// gate loads mode as a bool for exactly this reason — a snapshot that dropped
// the excludes would allow, in memory, what the database denies.
//
// inherit says whether the binding covers the node's descendants or only the
// node itself.
type GrantScope struct {
	GrantID     [16]byte
	TenantID    [16]byte
	ScopeNodeID [16]byte
	Inherit     bool
	AxisCode    ScopeAx
	Mode        string
}

func (m *GrantScope) Schema(t *storm.Table) {
	var ref0 Grant
	var ref1 ScopeNode
	t.PrimaryKey(&m.GrantID, &m.AxisCode, &m.ScopeNodeID)
	t.Col(&m.Inherit).Default("true")
	t.Col(&m.AxisCode).Named("axis_code")
	t.Col(&m.AxisCode).ConstraintName("grant_scopes_axis_code_fkey")
	t.Col(&m.AxisCode).OnDelete(storm.Restrict)
	t.Col(&m.AxisCode).NoIndex()
	t.Col(&m.Mode).Default("'include'::text")
	t.CheckNamed("grant_scopes_mode_check", "mode = ANY (ARRAY['include'::text, 'exclude'::text])")
	t.Index(&m.ScopeNodeID, &m.AxisCode).Named("grant_scopes_node")
	t.ForeignKey(&m.GrantID, &m.TenantID).References(&ref0, &ref0.ID, &ref0.Tenant).Named("grant_scopes_grant_id_tenant_id_fkey").OnDelete(storm.Cascade)
	t.ForeignKey(&m.ScopeNodeID, &m.TenantID, &m.AxisCode).References(&ref1, &ref1.ID, &ref1.Tenant, &ref1.AxisCode).Named("grant_scopes_scope_node_id_tenant_id_axis_code_fkey").OnDelete(storm.Restrict)
}

// Grant is one role given to one identity, optionally scoped.
//
// This is the row authorize() reads. A grant is alive when revoked_at is null
// and now() is inside [valid_from, valid_until) — expiry is a COLUMN rather
// than a sweep, so a lapsed grant stops granting at the instant it lapses and
// not whenever a job next runs.
//
// self_scoped means "only over the identity's own records", which is a
// different thing from having no scope rows at all: no rows means everywhere.
//
// ViaMembership and ViaEntry are set when the grant was fanned out from a
// membership rather than given directly. They are what lets unassigning a
// membership find exactly the grants it created and leave direct ones alone.
type Grant struct {
	CreatedAt     time.Time
	ValidFrom     time.Time
	ValidUntil    *time.Time
	RevokedAt     *time.Time
	ID            [16]byte
	Tenant        Tenant
	IdentityID    [16]byte
	RoleID        [16]byte
	GrantedBy     [16]byte
	Reason        *string
	SelfScoped    bool
	ViaMembership *Membership
	ViaEntry      *MembershipEntry
}

func (m *Grant) Schema(t *storm.Table) {
	var ref0 Identity
	var ref1 Role
	t.PrimaryKey(&m.ID)
	t.Col(&m.CreatedAt).Default("now()")
	t.Col(&m.ValidFrom).Default("now()")
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.Tenant).ConstraintName("grants_tenant_id_fkey")
	t.Col(&m.Tenant).OnDelete(storm.Cascade)
	t.Col(&m.Tenant).NoIndex()
	t.Col(&m.SelfScoped).Default("false")
	t.Col(&m.ViaMembership).ConstraintName("grants_via_membership_id_fkey")
	t.Col(&m.ViaMembership).OnDelete(storm.Restrict)
	t.Col(&m.ViaEntry).ConstraintName("grants_via_entry_id_fkey")
	t.Col(&m.ViaEntry).OnDelete(storm.Restrict)
	t.Col(&m.ViaEntry).NoIndex()
	t.UniqueNamed("grants_id_tenant_id_key", &m.ID, &m.Tenant)
	t.CheckNamed("grants_check", "(valid_until IS NULL) OR (valid_until > valid_from)")
	t.Index(&m.ValidUntil).Where("(revoked_at IS NULL) AND (valid_until IS NOT NULL)").Named("grants_expiring")
	t.Index(&m.IdentityID).Include(&m.RoleID, &m.ID, &m.ValidFrom, &m.ValidUntil).Where("revoked_at IS NULL").Named("grants_identity_live")
	t.Index(&m.RoleID).Where("revoked_at IS NULL").Named("grants_role")
	t.Index(&m.ViaMembership, &m.IdentityID).Where("(via_membership_id IS NOT NULL) AND (revoked_at IS NULL)").Named("grants_via_membership")
	t.ForeignKey(&m.IdentityID, &m.Tenant).References(&ref0, &ref0.ID, &ref0.Tenant).Named("grants_identity_id_tenant_id_fkey").OnDelete(storm.Cascade)
	t.ForeignKey(&m.RoleID, &m.Tenant).References(&ref1, &ref1.ID, &ref1.Tenant).Named("grants_role_id_tenant_id_fkey").OnDelete(storm.Restrict)
}

// Identity is one person (or service principal) inside one tenant.
//
// token_epoch is the revocation lever: every issued token carries the epoch it
// was minted under, so bumping it invalidates all of them at once without
// touching a session row. Disabling, anonymising and an explicit revocation all
// bump it.
//
// anonymized_at is erasure, and erasure here does not DELETE: the row and its
// referential integrity survive so the audit trail stays readable, while the
// direct identifiers are blanked and authorize() denies from that moment
// (migrations/0009 gate 1). retention_until is the statutory clock that
// triggers it.
type Identity struct {
	storm.Model

	DisabledAt          *time.Time
	LastLoginAt         *time.Time
	Tenant              Tenant
	TokenEpoch          int32
	Status              string
	Username            string
	Email               *string
	ExternalRef         *string
	Attributes          storm.JSON
	RealmID             *[16]byte
	AssuranceLevel      int16
	RetentionUntil      *time.Time
	DeletionRequestedAt *time.Time
	AnonymizedAt        *time.Time
	PiiKey              *PiiKey
	CategoryID          *[16]byte
}

func (m *Identity) Schema(t *storm.Table) {
	var ref0 RealmCategory
	var ref1 Realm
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.Tenant).ConstraintName("identities_tenant_id_fkey")
	t.Col(&m.Tenant).OnDelete(storm.Restrict)
	t.Col(&m.TokenEpoch).Default("1")
	t.Col(&m.Status).Default("'active'::text")
	t.Col(&m.Attributes).Default("'{}'::jsonb")
	t.Col(&m.AssuranceLevel).Default("1")
	t.Col(&m.PiiKey).ConstraintName("identities_pii_key_fk")
	t.Col(&m.PiiKey).OnDelete(storm.SetNull)
	t.Col(&m.PiiKey).NoIndex()
	t.UniqueNamed("identities_id_tenant_id_key", &m.ID, &m.Tenant)
	t.CheckNamed("identities_assurance_level_check", "(assurance_level >= 1) AND (assurance_level <= 3)")
	t.CheckNamed("identities_attributes_sealed", "(attributes = '{}'::jsonb) OR ((attributes ? 'v'::text) AND (attributes ? 'sealed'::text) AND (jsonb_typeof((attributes -> 'sealed'::text)) = 'string'::text) AND (length((attributes ->> 'sealed'::text)) > 0))")
	t.CheckNamed("identities_status_check", "status = ANY (ARRAY['active'::text, 'disabled'::text, 'locked'::text, 'pending'::text])")
	t.Index(&m.CategoryID).Where("category_id IS NOT NULL").Named("identities_category")
	t.Index(&m.Tenant, &m.RealmID, storm.Lower(&m.Email)).Unique().Where("email IS NOT NULL").Named("identities_email")
	t.Index(&m.Tenant, &m.ExternalRef).Unique().Where("external_ref IS NOT NULL").Named("identities_extref")
	t.Index(&m.RetentionUntil).Where("(retention_until IS NOT NULL) AND (anonymized_at IS NULL)").Named("identities_retention")
	t.Index(&m.Tenant, &m.RealmID, storm.Lower(&m.Username)).Unique().Named("identities_username")
	t.ForeignKey(&m.CategoryID, &m.RealmID).References(&ref0, &ref0.ID, &ref0.RealmID).Named("identities_category_same_realm").OnDelete(storm.SetNull)
	t.ForeignKey(&m.RealmID, &m.Tenant).References(&ref1, &ref1.ID, &ref1.Tenant).Named("identities_realm_fk").OnDelete(storm.Restrict).NoIndex()
}

// IdentityLink records that two identities are the same person.
//
// A link, not a merge: both rows keep their grants and their audit history,
// because merging them would rewrite history to say things that did not
// happen. method and evidence record HOW the claim was established.
type IdentityLink struct {
	LinkedAt    time.Time
	ID          [16]byte
	TenantID    [16]byte
	PrimaryID   [16]byte
	SecondaryID [16]byte
	LinkedBy    [16]byte
	Method      string
	Evidence    storm.JSON
}

func (m *IdentityLink) Schema(t *storm.Table) {
	var ref0 Identity
	var ref1 Identity
	t.PrimaryKey(&m.ID)
	t.Col(&m.LinkedAt).Default("now()")
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.Evidence).Default("'{}'::jsonb")
	t.UniqueNamed("identity_links_primary_id_secondary_id_key", &m.PrimaryID, &m.SecondaryID)
	t.CheckNamed("identity_links_check", "primary_id <> secondary_id")
	t.CheckNamed("identity_links_method_check", "method = ANY (ARRAY['manual'::text, 'email_proof'::text, 'document'::text, 'hr_sync'::text])")
	t.ForeignKey(&m.PrimaryID, &m.TenantID).References(&ref0, &ref0.ID, &ref0.Tenant).Named("identity_links_primary_id_tenant_id_fkey").OnDelete(storm.Cascade)
	t.ForeignKey(&m.SecondaryID, &m.TenantID).References(&ref1, &ref1.ID, &ref1.Tenant).Named("identity_links_secondary_id_tenant_id_fkey").OnDelete(storm.Cascade).NoIndex()
}

// MembershipEntry is one role inside a membership — the template a Grant is
// fanned out from.
type MembershipEntry struct {
	ID           [16]byte
	MembershipID [16]byte
	TenantID     [16]byte
	RoleID       [16]byte
}

func (m *MembershipEntry) Schema(t *storm.Table) {
	var ref0 Membership
	var ref1 Role
	t.PrimaryKey(&m.ID)
	t.Col(&m.ID).Default("uuidv7()")
	t.UniqueNamed("membership_entries_id_tenant_id_key", &m.ID, &m.TenantID)
	t.ForeignKey(&m.MembershipID, &m.TenantID).References(&ref0, &ref0.ID, &ref0.Tenant).Named("membership_entries_membership_id_tenant_id_fkey").OnDelete(storm.Cascade).NoIndex()
	t.ForeignKey(&m.RoleID, &m.TenantID).References(&ref1, &ref1.ID, &ref1.Tenant).Named("membership_entries_role_id_tenant_id_fkey").OnDelete(storm.Restrict).NoIndex()
}

// MembershipEntryScope is the scope binding on one membership entry, carrying
// the same include/exclude and inherit semantics a GrantScope does.
//
// It has to, because the grants fanned out from this entry get these rows
// copied onto them verbatim.
type MembershipEntryScope struct {
	EntryID     [16]byte
	TenantID    [16]byte
	ScopeNodeID [16]byte
	Inherit     bool
	AxisCode    ScopeAx
	Mode        string
}

func (m *MembershipEntryScope) Schema(t *storm.Table) {
	var ref0 MembershipEntry
	var ref1 ScopeNode
	t.PrimaryKey(&m.EntryID, &m.AxisCode, &m.ScopeNodeID)
	t.Col(&m.Inherit).Default("true")
	t.Col(&m.AxisCode).Named("axis_code")
	t.Col(&m.AxisCode).ConstraintName("membership_entry_scopes_axis_code_fkey")
	t.Col(&m.AxisCode).OnDelete(storm.Restrict)
	t.Col(&m.AxisCode).NoIndex()
	t.Col(&m.Mode).Default("'include'::text")
	t.CheckNamed("membership_entry_scopes_mode_check", "mode = ANY (ARRAY['include'::text, 'exclude'::text])")
	t.ForeignKey(&m.EntryID, &m.TenantID).References(&ref0, &ref0.ID, &ref0.TenantID).Named("membership_entry_scopes_entry_id_tenant_id_fkey").OnDelete(storm.Cascade)
	t.ForeignKey(&m.ScopeNodeID, &m.TenantID, &m.AxisCode).References(&ref1, &ref1.ID, &ref1.Tenant, &ref1.AxisCode).Named("membership_entry_scopes_scope_node_id_tenant_id_axis_code_fkey").OnDelete(storm.Restrict).NoIndex()
}

// MembershipMember is one identity's assignment to a membership.
//
// assigned_by and assigned_at are kept because removing somebody from a
// membership revokes real grants, and "who put them in and when" is the
// question asked afterwards.
type MembershipMember struct {
	AssignedAt   time.Time
	MembershipID [16]byte
	IdentityID   [16]byte
	TenantID     [16]byte
	AssignedBy   [16]byte
}

func (m *MembershipMember) Schema(t *storm.Table) {
	var ref0 Identity
	var ref1 Membership
	t.PrimaryKey(&m.MembershipID, &m.IdentityID)
	t.Col(&m.AssignedAt).Default("now()")
	t.ForeignKey(&m.IdentityID, &m.TenantID).References(&ref0, &ref0.ID, &ref0.Tenant).Named("membership_members_identity_id_tenant_id_fkey").OnDelete(storm.Cascade).NoIndex()
	t.ForeignKey(&m.MembershipID, &m.TenantID).References(&ref1, &ref1.ID, &ref1.Tenant).Named("membership_members_membership_id_tenant_id_fkey").OnDelete(storm.Cascade)
}

// Membership is a named, reusable bundle of role-and-scope assignments.
//
// Assigning one fans its entries out into real Grant rows rather than creating
// a layer the decision path has to consult: authorize() reads grants and knows
// nothing about memberships. That is why a membership can be changed and
// re-synced without the gate learning a second way to be granted.
type Membership struct {
	CreatedAt   time.Time
	ID          [16]byte
	Tenant      Tenant
	Name        string
	Description string
}

func (m *Membership) Schema(t *storm.Table) {
	t.PrimaryKey(&m.ID)
	t.Col(&m.CreatedAt).Default("now()")
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.Tenant).ConstraintName("memberships_tenant_id_fkey")
	t.Col(&m.Tenant).OnDelete(storm.Cascade)
	t.Col(&m.Description).Default("''::text")
	t.UniqueNamed("memberships_id_tenant_id_key", &m.ID, &m.Tenant)
	t.UniqueNamed("memberships_tenant_id_name_key", &m.Tenant, &m.Name)
}

// OneTimeToken is public.one_time_tokens. The authrmodel package documents this table's
// invariants; this declaration is the DDL half of the same thing.
type OneTimeToken struct {
	CreatedAt time.Time
	ExpiresAt time.Time
	ID        [16]byte
	Tenant    Tenant
	Kind      string
	TokenHash []byte
	Payload   storm.JSON
}

func (m *OneTimeToken) Schema(t *storm.Table) {
	t.PrimaryKey(&m.ID)
	t.Col(&m.CreatedAt).Default("now()")
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.Tenant).ConstraintName("one_time_tokens_tenant_id_fkey")
	t.Col(&m.Tenant).OnDelete(storm.Cascade)
	t.Col(&m.Tenant).NoIndex()
	t.Col(&m.TokenHash).NotNull()
	t.Col(&m.Payload).Default("'{}'::jsonb")
	t.UniqueNamed("one_time_tokens_token_hash_key", &m.TokenHash)
	t.CheckNamed("one_time_tokens_kind_check", "kind = ANY (ARRAY['mfa'::text, 'auth_code'::text, 'device_challenge'::text, 'email_verify'::text, 'password_reset'::text])")
	t.Index(&m.ExpiresAt).Named("one_time_tokens_expiry")
}

// Permission is one thing that can be granted, and the assurance it demands.
//
// key is the dotted name a request carries. min_assurance, requires_amr and
// max_auth_age are the step-up policy: a permission can demand a stronger
// factor or a more recent authentication than the session currently has, which
// is how a privileged action forces re-authentication without every session
// being privileged.
//
// deprecated_at retires a permission from the catalog without deleting it:
// grants that already name it keep working, and nothing new can be written
// against it.
type Permission struct {
	CreatedAt    time.Time
	DeprecatedAt *time.Time
	MaxAuthAge   *storm.Interval
	ID           [16]byte
	Application  Application
	TenantID     [16]byte
	Risk         string
	Resource     string
	Action       string
	AppSlug      string
	Key          *string
	Description  string
	RequiresAmr  []string
	MinAssurance int16
}

func (m *Permission) Schema(t *storm.Table) {
	var ref0 Application
	var ref1 Application
	t.PrimaryKey(&m.ID)
	t.Col(&m.CreatedAt).Default("now()")
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.Application).ConstraintName("permissions_application_id_fkey")
	t.Col(&m.Application).OnDelete(storm.Cascade)
	t.Col(&m.Risk).Default("'normal'::text")
	t.Col(&m.Description).Default("''::text")
	t.Col(&m.RequiresAmr).Default("'{}'::text[]")
	t.Col(&m.MinAssurance).Default("1")
	t.UniqueNamed("permissions_application_id_resource_action_key", &m.Application, &m.Resource, &m.Action)
	t.CheckNamed("permissions_min_assurance_check", "(min_assurance >= 1) AND (min_assurance <= 3)")
	t.CheckNamed("permissions_risk_check", "risk = ANY (ARRAY['normal'::text, 'sensitive'::text, 'critical'::text])")
	t.Index(&m.Application).Where("deprecated_at IS NULL").Named("permissions_app")
	t.Index(&m.TenantID, &m.Key).Unique().Named("permissions_key")
	t.ForeignKey(&m.Application, &m.AppSlug).References(&ref0, &ref0.ID, &ref0.Slug).Named("permissions_application_id_app_slug_fkey").OnUpdate(storm.Cascade)
	t.ForeignKey(&m.Application, &m.TenantID).References(&ref1, &ref1.ID, &ref1.Tenant).Named("permissions_application_id_tenant_id_fkey").OnDelete(storm.Cascade)
}

// PiiKeyTombstone is the record that a PII key was destroyed.
//
// Crypto-shredding leaves nothing to point at, so the proof that an erasure
// happened has to be a row somewhere else. This is that row: which key, when,
// and under what reason.
type PiiKeyTombstone struct {
	ShreddedAt time.Time
	KeyID      [16]byte
	TenantID   [16]byte
	Reason     string
}

func (m *PiiKeyTombstone) Schema(t *storm.Table) {
	t.PrimaryKey(&m.KeyID)
	t.Col(&m.ShreddedAt).Default("now()")
	t.CheckNamed("pii_key_tombstones_reason_check", "reason = ANY (ARRAY['erasure_request'::text, 'retention'::text, 'admin'::text])")
}

// PiiKey is public.pii_keys. The identityrmodel package documents this table's
// invariants; this declaration is the DDL half of the same thing.
type PiiKey struct {
	CreatedAt  time.Time
	ShreddedAt *time.Time
	ID         [16]byte
	Tenant     Tenant
	KeyEnc     []byte
	KmsKeyRef  *string
}

func (m *PiiKey) Schema(t *storm.Table) {
	t.PrimaryKey(&m.ID)
	t.Col(&m.CreatedAt).Default("now()")
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.Tenant).ConstraintName("pii_keys_tenant_id_fkey")
	t.Col(&m.Tenant).OnDelete(storm.Cascade)
	t.Col(&m.KeyEnc).NotNull()
	t.Index(&m.Tenant).Named("pii_keys_tenant")
}

// PlatformAPIKey is public.platform_api_keys. The controlrmodel package documents this table's
// invariants; this declaration is the DDL half of the same thing.
type PlatformAPIKey struct {
	ID           [16]byte
	PlatformUser PlatformUser
	Label        string
	Lookup       string
	SecretHash   string
	CreatedAt    time.Time
	CreatedBy    *PlatformUser
	LastUsedAt   *time.Time
	ExpiresAt    time.Time
	RevokedAt    *time.Time
}

func (m *PlatformAPIKey) Schema(t *storm.Table) {
	t.PrimaryKey(&m.ID)
	t.Name("platform_api_keys")
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.PlatformUser).ConstraintName("platform_api_keys_platform_user_id_fkey")
	t.Col(&m.PlatformUser).OnDelete(storm.Cascade)
	t.Col(&m.Label).Default("''::text")
	t.Col(&m.CreatedAt).Default("now()")
	t.Col(&m.CreatedBy).Named("created_by")
	t.Col(&m.CreatedBy).ConstraintName("platform_api_keys_created_by_fkey")
	t.Col(&m.CreatedBy).OnDelete(storm.SetNull)
	t.Col(&m.CreatedBy).NoIndex()
	t.Index(&m.Lookup).Unique().Where("revoked_at IS NULL").Named("platform_api_keys_lookup")
	t.Index(&m.PlatformUser, &m.RevokedAt).Named("platform_api_keys_owner")
}

// PlatformAssignment is public.platform_assignments. The controlrmodel package documents this table's
// invariants; this declaration is the DDL half of the same thing.
type PlatformAssignment struct {
	storm.Model

	Operator   PlatformUser
	Tenant     *Tenant
	Role       string
	GrantedBy  *PlatformUser
	Reason     string
	ValidUntil *time.Time
	RevokedAt  *time.Time
}

func (m *PlatformAssignment) Schema(t *storm.Table) {
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.Operator).ConstraintName("platform_assignments_operator_id_fkey")
	t.Col(&m.Operator).OnDelete(storm.Cascade)
	t.Col(&m.Tenant).ConstraintName("platform_assignments_tenant_id_fkey")
	t.Col(&m.Tenant).OnDelete(storm.Cascade)
	t.Col(&m.Tenant).NoIndex()
	t.Col(&m.GrantedBy).Named("granted_by")
	t.Col(&m.GrantedBy).ConstraintName("platform_assignments_granted_by_fkey")
	t.Col(&m.GrantedBy).OnDelete(storm.SetNull)
	t.Col(&m.GrantedBy).NoIndex()
	t.Col(&m.Reason).Default("''::text")
	t.CheckNamed("platform_assignments_role_check", "role = ANY (ARRAY['support'::text, 'admin'::text, 'owner'::text])")
	t.Index(&m.Operator, &m.Tenant).Unique().Where("(revoked_at IS NULL) AND (tenant_id IS NOT NULL)").Named("platform_assignments_live")
	t.Index(&m.Operator).Unique().Where("(revoked_at IS NULL) AND (tenant_id IS NULL)").Named("platform_assignments_live_global")
	t.Index(&m.Operator, &m.Tenant, &m.RevokedAt).Named("platform_assignments_lookup")
}

// PlatformRefreshToken is public.platform_refresh_tokens. The controlrmodel package documents this table's
// invariants; this declaration is the DDL half of the same thing.
type PlatformRefreshToken struct {
	ID           [16]byte
	PlatformUser PlatformUser
	FamilyID     [16]byte
	TokenHash    []byte
	CreatedAt    time.Time
	ExpiresAt    time.Time
	UsedAt       *time.Time
	RevokedAt    *time.Time
}

func (m *PlatformRefreshToken) Schema(t *storm.Table) {
	t.PrimaryKey(&m.ID)
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.PlatformUser).ConstraintName("platform_refresh_tokens_platform_user_id_fkey")
	t.Col(&m.PlatformUser).OnDelete(storm.Cascade)
	t.Col(&m.PlatformUser).NoIndex()
	t.Col(&m.TokenHash).NotNull()
	t.Col(&m.CreatedAt).Default("now()")
	t.CheckNamed("platform_refresh_hash_len", "octet_length(token_hash) = 32")
	t.Index(&m.FamilyID).Named("platform_refresh_by_family")
	t.Index(&m.TokenHash).Unique().Named("platform_refresh_by_hash")
}

// PlatformUser is public.platform_users. The controlrmodel package documents this table's
// invariants; this declaration is the DDL half of the same thing.
type PlatformUser struct {
	storm.Model

	Username       string
	Email          *string
	PasswordHash   string
	Status         string
	TokenEpoch     int32
	LastLoginAt    *time.Time
	DisabledAt     *time.Time
	TotpSecretEnc  []byte
	TotpEnrolledAt *time.Time
	TotpLastStep   int64
}

func (m *PlatformUser) Schema(t *storm.Table) {
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.Status).Default("'active'::text")
	t.Col(&m.TokenEpoch).Default("0")
	t.Col(&m.TotpLastStep).Default("0")
	t.CheckNamed("platform_users_status_check", "status = ANY (ARRAY['active'::text, 'disabled'::text])")
	t.CheckNamed("platform_users_totp_complete", "(totp_enrolled_at IS NULL) OR (totp_secret_enc IS NOT NULL)")
	t.CheckNamed("platform_users_username_check", "username ~ '^[a-zA-Z0-9][a-zA-Z0-9._-]{1,62}$'::text")
	t.Index(storm.Lower(&m.Email)).Unique().Where("(email IS NOT NULL) AND (email <> ''::text)").Named("platform_users_email")
	t.Index(storm.Lower(&m.Username)).Unique().Named("platform_users_username")
}

// RealmCategory is public.realm_categories. The identityrmodel package documents this table's
// invariants; this declaration is the DDL half of the same thing.
type RealmCategory struct {
	CreatedAt   time.Time
	SortOrder   int32
	ID          [16]byte
	TenantID    [16]byte
	RealmID     [16]byte
	Code        string
	DisplayName string
}

func (m *RealmCategory) Schema(t *storm.Table) {
	var ref0 Realm
	t.PrimaryKey(&m.ID)
	t.Col(&m.CreatedAt).Default("now()")
	t.Col(&m.SortOrder).Default("100")
	t.Col(&m.ID).Default("uuidv7()")
	t.UniqueNamed("realm_categories_id_realm_id_key", &m.ID, &m.RealmID)
	t.UniqueNamed("realm_categories_realm_id_code_key", &m.RealmID, &m.Code)
	t.CheckNamed("realm_categories_code_check", "code ~ '^[a-z][a-z0-9_]{1,30}$'::text")
	t.ForeignKey(&m.RealmID, &m.TenantID).References(&ref0, &ref0.ID, &ref0.Tenant).Named("realm_categories_realm_id_tenant_id_fkey").OnDelete(storm.Cascade)
}

// Realm is a POPULATION and the policy that governs it — employees,
// customers, suppliers — inside one tenant.
//
// code and kind decide which roles the realm's members may hold, and
// migrations/0010 enforces that on every grant. That is why neither is
// editable once the realm has members: changing them would retroactively
// re-decide access that has already been granted and evaluated.
//
// The three TTLs and the factor policy are per-population because a supplier's
// session should not last as long as an employee's.
type Realm struct {
	storm.Model

	SessionTtl                storm.Interval
	AccessTokenTtl            storm.Interval
	RefreshTokenTtl           storm.Interval
	DefaultRetention          *storm.Interval
	Tenant                    Tenant
	MinAssurance              int16
	SelfRegistration          bool
	EmailVerificationRequired bool
	PiiEncryption             bool
	Kind                      string
	Code                      string
	DisplayName               string
	AllowedFactors            []string
	RequiredFactors           []string
	PasswordPolicy            storm.JSON
	FactorEnrolmentDeadline   *time.Time
}

func (m *Realm) Schema(t *storm.Table) {
	t.Col(&m.SessionTtl).Default("'12:00:00'::interval")
	t.Col(&m.AccessTokenTtl).Default("'00:10:00'::interval")
	t.Col(&m.RefreshTokenTtl).Default("'30 days'::interval")
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.Tenant).ConstraintName("realms_tenant_id_fkey")
	t.Col(&m.Tenant).OnDelete(storm.Cascade)
	t.Col(&m.MinAssurance).Default("1")
	t.Col(&m.SelfRegistration).Default("false")
	t.Col(&m.EmailVerificationRequired).Default("true")
	t.Col(&m.PiiEncryption).Default("false")
	t.Col(&m.AllowedFactors).Default("'{password}'::text[]")
	t.Col(&m.RequiredFactors).Default("'{password}'::text[]")
	t.Col(&m.PasswordPolicy).Default("'{}'::jsonb")
	t.UniqueNamed("realms_id_tenant_id_key", &m.ID, &m.Tenant)
	t.UniqueNamed("realms_tenant_id_code_key", &m.Tenant, &m.Code)
	t.CheckNamed("realms_code_check", "code ~ '^[a-z][a-z0-9_]{1,30}$'::text")
	t.CheckNamed("realms_kind_check", "kind = ANY (ARRAY['internal'::text, 'partner'::text, 'public'::text, 'service'::text])")
	t.CheckNamed("realms_min_assurance_check", "(min_assurance >= 1) AND (min_assurance <= 3)")
}

// RefreshToken is public.refresh_tokens. The authrmodel package documents this table's
// invariants; this declaration is the DDL half of the same thing.
type RefreshToken struct {
	CreatedAt   time.Time
	ExpiresAt   time.Time
	ConsumedAt  *time.Time
	RevokedAt   *time.Time
	ID          [16]byte
	SessionID   [16]byte
	TenantID    [16]byte
	FamilyID    [16]byte
	SuccessorID *[16]byte
	Generation  int32
	Status      string
	TokenHash   []byte
	BoundKey    *string
}

func (m *RefreshToken) Schema(t *storm.Table) {
	t.PrimaryKey(&m.ID, &m.ExpiresAt)
	t.Col(&m.CreatedAt).Default("now()")
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.Generation).Default("0")
	t.Col(&m.Status).Default("'active'::text")
	t.Col(&m.TokenHash).NotNull()
	t.CheckNamed("refresh_tokens_status_check", "status = ANY (ARRAY['active'::text, 'consumed'::text, 'revoked'::text])")
	t.Index(&m.FamilyID).Where("status = 'active'::text").Named("refresh_tokens_family")
	t.Index(&m.TokenHash, &m.ExpiresAt).Unique().Named("refresh_tokens_hash")
	t.Index(&m.SessionID).Named("refresh_tokens_session")
	t.PartitionBy(storm.RangePartition, &m.ExpiresAt)
}

// RoleGrantable says which roles a role's holder may grant to somebody else.
//
// Separate from inheritance on purpose: being able to USE a permission and
// being able to HAND IT OUT are different authorities, and conflating them is
// how an ordinary role quietly becomes an administrative one.
type RoleGrantable struct {
	Role      Role
	Grantable Role
}

func (m *RoleGrantable) Schema(t *storm.Table) {
	t.PrimaryKey(&m.Role, &m.Grantable)
	t.Name("role_grantable")
	t.Col(&m.Role).ConstraintName("role_grantable_role_id_fkey")
	t.Col(&m.Role).OnDelete(storm.Cascade)
	t.Col(&m.Grantable).ConstraintName("role_grantable_grantable_id_fkey")
	t.Col(&m.Grantable).OnDelete(storm.Cascade)
	t.Col(&m.Grantable).NoIndex()
	t.CheckNamed("role_grantable_check", "role_id <> grantable_id")
}

// RoleParent is one edge of the role inheritance graph: role gets everything
// parent has.
//
// A role may have several parents. A cycle is TOLERATED rather than refused:
// the effective-set recompute in migrations/0004 walks the graph with
// PostgreSQL's CYCLE clause, which stops the recursion at a repeat instead of
// running forever. Nothing rejects the edge that closes the loop, so a cycle
// is a modelling mistake the schema survives rather than one it prevents.
type RoleParent struct {
	Role   Role
	Parent Role
}

func (m *RoleParent) Schema(t *storm.Table) {
	t.PrimaryKey(&m.Role, &m.Parent)
	t.Col(&m.Role).ConstraintName("role_parents_role_id_fkey")
	t.Col(&m.Role).OnDelete(storm.Cascade)
	t.Col(&m.Parent).ConstraintName("role_parents_parent_id_fkey")
	t.Col(&m.Parent).OnDelete(storm.Cascade)
	t.CheckNamed("role_parents_check", "role_id <> parent_id")
	t.Index(&m.Parent).Named("role_parents_parent")
}

// RolePermissionPattern is a glob a role grants rather than a named permission.
//
// It exists so a role can say "every read in this application" and keep meaning
// that as the application adds permissions. The expansion is recomputed by
// trigger when either side moves, which is why a new permission joins the roles
// that match it without anyone re-saving a role.
type RolePermissionPattern struct {
	ID      [16]byte
	Role    Role
	Pattern string
}

func (m *RolePermissionPattern) Schema(t *storm.Table) {
	t.PrimaryKey(&m.ID)
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.Role).ConstraintName("role_permission_patterns_role_id_fkey")
	t.Col(&m.Role).OnDelete(storm.Cascade)
	t.UniqueNamed("role_permission_patterns_role_id_pattern_key", &m.Role, &m.Pattern)
	t.CheckNamed("role_permission_patterns_pattern_check", "pattern ~ '^[a-z0-9_:*-]+$'::text")
}

// RolePermission is a permission named DIRECTLY on a role.
//
// The other two ways a role acquires permissions are a pattern
// (RolePermissionPattern) and inheritance from a parent (RoleParent). All
// three are folded into RolePermissionsEffective by trigger.
type RolePermission struct {
	Role       Role
	Permission Permission
}

func (m *RolePermission) Schema(t *storm.Table) {
	t.PrimaryKey(&m.Role, &m.Permission)
	t.Col(&m.Role).ConstraintName("role_permissions_role_id_fkey")
	t.Col(&m.Role).OnDelete(storm.Cascade)
	t.Col(&m.Permission).ConstraintName("role_permissions_permission_id_fkey")
	t.Col(&m.Permission).OnDelete(storm.Cascade)
	t.Col(&m.Permission).NoIndex()
}

// RolePermissionsEffective is the DERIVED closure: every permission every role
// actually has, from all three sources — direct, pattern, and inherited.
//
// Maintained entirely by trigger (migrations 0005/0006). Nothing in Go writes
// it. The gate snapshot reads this table rather than recomputing the closure
// per request, which is the difference between a decision that costs one map
// lookup and one that walks a role graph.
//
// via_role records WHICH role in the inheritance chain contributed the
// permission, which is what makes a decision explainable rather than merely
// correct.
type RolePermissionsEffective struct {
	Role       Role
	Permission Permission
	ViaRole    Role
}

func (m *RolePermissionsEffective) Schema(t *storm.Table) {
	t.PrimaryKey(&m.Role, &m.Permission)
	t.Name("role_permissions_effective")
	t.Col(&m.Role).ConstraintName("role_permissions_effective_role_id_fkey")
	t.Col(&m.Role).OnDelete(storm.Cascade)
	t.Col(&m.Permission).ConstraintName("role_permissions_effective_permission_id_fkey")
	t.Col(&m.Permission).OnDelete(storm.Cascade)
	t.Col(&m.ViaRole).ConstraintName("role_permissions_effective_via_role_id_fkey")
	t.Col(&m.ViaRole).OnDelete(storm.Cascade)
	t.Col(&m.ViaRole).NoIndex()
	t.Index(&m.Permission, &m.Role).Named("rpe_permission")
}

// Role is public.roles. The authzrmodel package documents this table's
// invariants; this declaration is the DDL half of the same thing.
type Role struct {
	storm.Model

	Tenant            Tenant
	ApplicationID     *[16]byte
	IsSystem          bool
	Name              string
	Description       string
	AssignableAt      []string
	AllowedRealmKinds []string
	DeprecatedAt      *time.Time
}

func (m *Role) Schema(t *storm.Table) {
	var ref0 Application
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.Tenant).ConstraintName("roles_tenant_id_fkey")
	t.Col(&m.Tenant).OnDelete(storm.Cascade)
	t.Col(&m.IsSystem).Default("false")
	t.Col(&m.Description).Default("''::text")
	t.Col(&m.AssignableAt).Default("'{}'::text[]")
	t.Col(&m.AllowedRealmKinds).Default("'{internal}'::text[]")
	t.UniqueNamed("roles_id_tenant_id_key", &m.ID, &m.Tenant)
	t.UniqueNamed("roles_tenant_id_name_key", &m.Tenant, &m.Name)
	t.ForeignKey(&m.ApplicationID, &m.Tenant).References(&ref0, &ref0.ID, &ref0.Tenant).Named("roles_application_id_tenant_id_fkey").OnDelete(storm.Cascade).NoIndex()
}

// RoutePolicy is public.route_policies. The tenancyrmodel package documents this table's
// invariants; this declaration is the DDL half of the same thing.
type RoutePolicy struct {
	CreatedAt     time.Time
	ID            [16]byte
	ApplicationID [16]byte
	TenantID      [16]byte
	Permission    *Permission
	Priority      int32
	Effect        string
	PathPattern   string
	HostPattern   *string
	Methods       []string
	ScopeBindings storm.JSON
}

func (m *RoutePolicy) Schema(t *storm.Table) {
	var ref0 Application
	t.PrimaryKey(&m.ID)
	t.Col(&m.CreatedAt).Default("now()")
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.Permission).ConstraintName("route_policies_permission_id_fkey")
	t.Col(&m.Permission).OnDelete(storm.Restrict)
	t.Col(&m.Permission).NoIndex()
	t.Col(&m.Methods).Default("'{*}'::text[]")
	t.Col(&m.ScopeBindings).Default("'{}'::jsonb")
	t.UniqueNamed("route_policies_application_id_priority_key", &m.ApplicationID, &m.Priority)
	t.CheckNamed("route_policies_check", "(effect <> 'require_permission'::text) OR (permission_id IS NOT NULL)")
	t.CheckNamed("route_policies_effect_check", "effect = ANY (ARRAY['public'::text, 'require_auth'::text, 'require_permission'::text, 'deny'::text])")
	t.Index(&m.ApplicationID, &m.Priority).Named("route_policies_app")
	t.ForeignKey(&m.ApplicationID, &m.TenantID).References(&ref0, &ref0.ID, &ref0.Tenant).Named("route_policies_application_id_tenant_id_fkey").OnDelete(storm.Cascade)
}

// SchemaMigration is the migration runner's ledger: which files have been
// applied, and the checksum each had when it ran.
//
// The checksum is the point. A migration edited after it was applied is a
// database that no longer matches its own history, and the runner refuses to
// proceed rather than guessing which version is real.
type SchemaMigration struct {
	AppliedAt time.Time
	Version   string
	Checksum  string
}

func (m *SchemaMigration) Schema(t *storm.Table) {
	t.PrimaryKey(&m.Version)
	t.Col(&m.AppliedAt).Default("now()")
}

// ScopeAx is public.scope_axes. The scopermodel package documents this table's
// invariants; this declaration is the DDL half of the same thing.
type ScopeAx struct {
	CreatedAt     time.Time
	SortOrder     int32
	Code          string
	DisplayName   string
	DefaultEffect string
	Status        string
	Resolution    storm.JSON
	UiSchema      storm.JSON
}

func (m *ScopeAx) Schema(t *storm.Table) {
	t.PrimaryKey(&m.Code)
	t.Col(&m.CreatedAt).Default("now()")
	t.Col(&m.SortOrder).Default("100")
	t.Col(&m.DefaultEffect).Default("'unconstrained'::text")
	t.Col(&m.Status).Default("'active'::text")
	t.Col(&m.Resolution).Default("'{\"from\": \"context\"}'::jsonb")
	t.Col(&m.UiSchema).Default("'{}'::jsonb")
	t.CheckNamed("scope_axes_code_check", "code ~ '^[a-z][a-z0-9_]{1,30}$'::text")
	t.CheckNamed("scope_axes_default_effect_check", "default_effect = ANY (ARRAY['unconstrained'::text, 'deny'::text])")
	t.CheckNamed("scope_axes_status_check", "status = ANY (ARRAY['active'::text, 'deprecated'::text])")
}

// ScopeClosure is the DERIVED ancestor-of relation over the scope trees, one
// row per (ancestor, descendant, depth).
//
// Maintained entirely by trigger; nothing in Go writes it. authorize() probes
// it to answer "does a grant on node A reach target B" as an index lookup
// rather than a recursive walk, which is what keeps the decision flat as the
// tree deepens.
//
// The gate does NOT load this table — it loads parent pointers and walks them,
// because one row per node beats one row per pair at a million nodes. The two
// must agree, and snapshot_parity_test.go is what proves they do.
type ScopeClosure struct {
	Ancestor   ScopeNode
	Descendant ScopeNode
	Depth      int16
}

func (m *ScopeClosure) Schema(t *storm.Table) {
	t.PrimaryKey(&m.Ancestor, &m.Descendant)
	t.Name("scope_closure")
	t.Col(&m.Ancestor).ConstraintName("scope_closure_ancestor_id_fkey")
	t.Col(&m.Ancestor).OnDelete(storm.Cascade)
	t.Col(&m.Descendant).ConstraintName("scope_closure_descendant_id_fkey")
	t.Col(&m.Descendant).OnDelete(storm.Cascade)
	t.CheckNamed("scope_closure_depth_check", "depth >= 0")
	t.Index(&m.Descendant, &m.Ancestor).Include(&m.Depth).Named("scope_closure_up")
}

// ScopeNodeType is public.scope_node_types. The scopermodel package documents this table's
// invariants; this declaration is the DDL half of the same thing.
type ScopeNodeType struct {
	Code        string
	AxisCode    ScopeAx
	DisplayName string
	ParentTypes []string
}

func (m *ScopeNodeType) Schema(t *storm.Table) {
	t.PrimaryKey(&m.Code)
	t.Col(&m.AxisCode).Named("axis_code")
	t.Col(&m.AxisCode).ConstraintName("scope_node_types_axis_code_fkey")
	t.Col(&m.AxisCode).OnDelete(storm.Restrict)
	t.Col(&m.AxisCode).NoIndex()
	t.Col(&m.ParentTypes).Default("'{}'::text[]")
	t.UniqueNamed("scope_node_types_code_axis_code_key", &m.Code, &m.AxisCode)
	t.CheckNamed("scope_node_types_code_check", "code ~ '^[a-z][a-z0-9_]{1,30}$'::text")
}

// ScopeNode is public.scope_nodes. The scopermodel package documents this table's
// invariants; this declaration is the DDL half of the same thing.
type ScopeNode struct {
	storm.Model

	Tenant      Tenant
	ParentID    *[16]byte
	IsAxisRoot  bool
	Status      string
	AxisCode    string
	NodeType    string
	Slug        string
	Name        string
	ExternalRef *string
	Attributes  storm.JSON
}

func (m *ScopeNode) Schema(t *storm.Table) {
	var ref0 ScopeNodeType
	var ref1 ScopeNode
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.Tenant).ConstraintName("scope_nodes_tenant_id_fkey")
	t.Col(&m.Tenant).OnDelete(storm.Restrict)
	t.Col(&m.IsAxisRoot).Default("false")
	t.Col(&m.Status).Default("'active'::text")
	t.Col(&m.Attributes).Default("'{}'::jsonb")
	t.UniqueNamed("scope_nodes_id_tenant_id_axis_code_key", &m.ID, &m.Tenant, &m.AxisCode)
	t.CheckNamed("nonroot_has_parent", "is_axis_root OR (parent_id IS NOT NULL)")
	t.CheckNamed("root_has_no_parent", "(NOT is_axis_root) OR (parent_id IS NULL)")
	t.CheckNamed("scope_nodes_slug_check", "slug <> ''::text")
	t.CheckNamed("scope_nodes_status_check", "status = ANY (ARRAY['active'::text, 'archived'::text])")
	t.Index(&m.Tenant, &m.AxisCode).Include(&m.ParentID, &m.NodeType).Where("status = 'active'::text").Named("scope_nodes_axis")
	t.Index(&m.Tenant, &m.AxisCode, &m.ExternalRef).Unique().Where("external_ref IS NOT NULL").Named("scope_nodes_extref")
	t.Index(&m.Tenant, &m.AxisCode).Unique().Where("is_axis_root").Named("scope_nodes_one_root")
	t.Index(&m.Tenant, &m.AxisCode, &m.Name, &m.ID).Named("scope_nodes_paging")
	t.Index(&m.ParentID, &m.Slug).Unique().Where("parent_id IS NOT NULL").Named("scope_nodes_sibling_slug")
	t.ForeignKey(&m.NodeType, &m.AxisCode).References(&ref0, &ref0.Code, &ref0.AxisCode).Named("scope_nodes_node_type_axis_code_fkey").OnDelete(storm.Restrict).NoIndex()
	t.ForeignKey(&m.ParentID, &m.Tenant, &m.AxisCode).References(&ref1, &ref1.ID, &ref1.Tenant, &ref1.AxisCode).Named("scope_nodes_parent_id_tenant_id_axis_code_fkey").OnDelete(storm.Restrict)
}

// ScopeSyncRun is public.scope_sync_runs. The scopermodel package documents this table's
// invariants; this declaration is the DDL half of the same thing.
type ScopeSyncRun struct {
	StartedAt  time.Time
	FinishedAt *time.Time
	ID         [16]byte
	Source     ScopeSyncSource
	Dry        bool
	Status     string
	Report     storm.JSON
}

func (m *ScopeSyncRun) Schema(t *storm.Table) {
	t.PrimaryKey(&m.ID)
	t.Col(&m.StartedAt).Default("now()")
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.Source).ConstraintName("scope_sync_runs_source_id_fkey")
	t.Col(&m.Source).OnDelete(storm.Cascade)
	t.Col(&m.Dry).Default("false")
	t.Col(&m.Status).Default("'running'::text")
	t.Col(&m.Report).Default("'{}'::jsonb")
	t.CheckNamed("scope_sync_runs_status_check", "status = ANY (ARRAY['running'::text, 'ok'::text, 'failed'::text, 'dry_run'::text])")
	t.Index(&m.Source, storm.Desc(&m.StartedAt)).Named("scope_sync_runs_src")
}

// ScopeSyncSource is public.scope_sync_sources. The scopermodel package documents this table's
// invariants; this declaration is the DDL half of the same thing.
type ScopeSyncSource struct {
	CreatedAt       time.Time
	LastRunAt       *time.Time
	ID              [16]byte
	Tenant          Tenant
	AxisCode        ScopeAx
	Kind            string
	Status          string
	Config          storm.JSON
	NextRunAt       *time.Time
	IntervalSeconds int32
}

func (m *ScopeSyncSource) Schema(t *storm.Table) {
	t.PrimaryKey(&m.ID)
	t.Col(&m.CreatedAt).Default("now()")
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.Tenant).ConstraintName("scope_sync_sources_tenant_id_fkey")
	t.Col(&m.Tenant).OnDelete(storm.Cascade)
	t.Col(&m.AxisCode).Named("axis_code")
	t.Col(&m.AxisCode).ConstraintName("scope_sync_sources_axis_code_fkey")
	t.Col(&m.AxisCode).OnDelete(storm.Cascade)
	t.Col(&m.AxisCode).NoIndex()
	t.Col(&m.Status).Default("'active'::text")
	t.Col(&m.IntervalSeconds).Default("0")
	t.UniqueNamed("scope_sync_sources_tenant_id_axis_code_key", &m.Tenant, &m.AxisCode)
	t.CheckNamed("scope_sync_sources_interval_seconds_check", "(interval_seconds = 0) OR (interval_seconds >= 300)")
	t.CheckNamed("scope_sync_sources_kind_check", "kind = ANY (ARRAY['http'::text, 'db_query'::text, 'db_table'::text])")
	t.CheckNamed("scope_sync_sources_status_check", "status = ANY (ARRAY['active'::text, 'disabled'::text])")
	t.Index(&m.NextRunAt).Where("(status = 'active'::text) AND (next_run_at IS NOT NULL)").Named("scope_sync_sources_due")
}

// Session is public.sessions. The authrmodel package documents this table's
// invariants; this declaration is the DDL half of the same thing.
type Session struct {
	CreatedAt     time.Time
	LastSeenAt    time.Time
	AuthTime      time.Time
	ExpiresAt     time.Time
	RevokedAt     *time.Time
	ID            [16]byte
	IdentityID    [16]byte
	TenantID      [16]byte
	ApplicationID *[16]byte
	Amr           []string
	RevokeReason  *string
	DeviceFp      *string
	IP            *netip.Prefix
	UserAgent     *string
	ActiveScopes  storm.JSON
	CookieHash    []byte
}

func (m *Session) Schema(t *storm.Table) {
	var ref0 Application
	var ref1 Identity
	t.PrimaryKey(&m.ID)
	t.Col(&m.CreatedAt).Default("now()")
	t.Col(&m.LastSeenAt).Default("now()")
	t.Col(&m.AuthTime).Default("now()")
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.Amr).Default("'{}'::text[]")
	t.Col(&m.ActiveScopes).Default("'{}'::jsonb")
	t.UniqueNamed("sessions_id_tenant_id_key", &m.ID, &m.TenantID)
	t.Index(&m.CookieHash).Unique().Where("(cookie_hash IS NOT NULL) AND (revoked_at IS NULL)").Named("sessions_cookie")
	t.Index(&m.ExpiresAt).Where("revoked_at IS NULL").Named("sessions_expiry")
	t.Index(&m.IdentityID, storm.Desc(&m.CreatedAt)).Where("revoked_at IS NULL").Named("sessions_identity")
	t.ForeignKey(&m.ApplicationID, &m.TenantID).References(&ref0, &ref0.ID, &ref0.Tenant).Named("sessions_application_id_tenant_id_fkey").OnDelete(storm.Cascade).NoIndex()
	t.ForeignKey(&m.IdentityID, &m.TenantID).References(&ref1, &ref1.ID, &ref1.Tenant).Named("sessions_identity_id_tenant_id_fkey").OnDelete(storm.Cascade)
}

// SigningKey is public.signing_keys. The authrmodel package documents this table's
// invariants; this declaration is the DDL half of the same thing.
type SigningKey struct {
	CreatedAt     time.Time
	NotBefore     time.Time
	NotAfter      time.Time
	RetiredAt     *time.Time
	ID            [16]byte
	Kid           string
	Alg           string
	Status        string
	Purpose       string
	PublicKey     []byte
	PrivateKeyEnc []byte
	KmsKeyRef     *string
}

func (m *SigningKey) Schema(t *storm.Table) {
	t.PrimaryKey(&m.ID)
	t.Col(&m.CreatedAt).Default("now()")
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.Alg).Default("'Ed25519'::text")
	t.Col(&m.Status).Default("'pending'::text")
	t.Col(&m.Purpose).Default("'access'::text")
	t.Col(&m.PublicKey).NotNull()
	t.Col(&m.PrivateKeyEnc).NotNull()
	t.UniqueNamed("signing_keys_kid_key", &m.Kid)
	t.CheckNamed("signing_keys_alg_check", "alg = ANY (ARRAY['Ed25519'::text, 'ES256'::text])")
	t.CheckNamed("signing_keys_purpose_check", "purpose = ANY (ARRAY['access'::text, 'local'::text])")
	t.CheckNamed("signing_keys_status_check", "status = ANY (ARRAY['pending'::text, 'active'::text, 'retiring'::text, 'retired'::text])")
	t.Index(&m.Purpose).Unique().Where("status = 'active'::text").Named("signing_keys_one_active")
}

// Tenant is public.tenants. The tenancyrmodel package documents this table's
// invariants; this declaration is the DDL half of the same thing.
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
	t.CheckNamed("tenants_status_check", "status = ANY (ARRAY['active'::text, 'suspended'::text, 'archived'::text])")
}
