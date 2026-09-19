// Package identityrmodel declares the identity tables storm generates code
// for.
//
// It is a PROJECTION of the schema, not its source of truth: anubis's schema
// of record stays migrations/ (forward-only, checksummed), and this model is
// validated against the live schema the same way the raw queries are.
//
// Only four tables are here, and deliberately. `identities` and `realms` have
// no plain reads at all — every one of them LEFT JOINs the realm and category
// for display, or renders an interval as text and seconds, and every write is
// a guarded transition that bumps token_epoch in the database. Modelling them
// would produce builders nothing could use. What IS modelled is the part of
// this context that is ordinary: credentials, consents, PII keys, categories.
//
// `identities`, `realms` and `tenants` are referenced as plain columns rather
// than relations for the same reason the other contexts do it — the foreign
// keys live in migrations/, which is where every constraint lives.
package identityrmodel

import (
	"time"

	"github.com/gsoultan/storm"
)

// Credential is public.credentials: one way an identity proves itself.
type Credential struct {
	storm.Model

	LastUsedAt *time.Time
	ExpiresAt  *time.Time
	RevokedAt  *time.Time
	IdentityID [16]byte
	TenantID   [16]byte

	// SignCounter is the WebAuthn replay guard; it only ever increases.
	SignCounter int64
	Kind        string
	Secret      *string
	// SecretKid names the local key Secret was sealed under. NULL means the
	// row predates the column.
	SecretKid *string
	LookupKey *string
	Label     *string
	Params    storm.JSON
}

func (m *Credential) Schema(t *storm.Table) {
	t.Name("credentials")
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.SignCounter).Default("0")
	t.Col(&m.Params).Default("'{}'::jsonb")
	t.Col(&m.SecretKid).Size(64)
	t.CheckNamed("credentials_kind_check",
		"kind = ANY (ARRAY['password'::text, 'device_key'::text, 'totp'::text, "+
			"'recovery_code'::text, 'oidc_link'::text])")
	t.Index(&m.IdentityID, &m.Kind).Where("revoked_at IS NULL").Named("credentials_identity")
	t.Index(&m.LookupKey).Unique().
		Where("(lookup_key IS NOT NULL) AND (revoked_at IS NULL)").Named("credentials_lookup")
	// One LIVE password per identity. Partial, because revoked ones stay for
	// the audit trail and would otherwise collide.
	t.Index(&m.IdentityID).Unique().
		Where("(kind = 'password'::text) AND (revoked_at IS NULL)").
		Named("credentials_one_password")
}

// Consent is public.consents: the lawful basis for processing an identity's
// personal data. Append-only — a withdrawal stamps the row it withdraws.
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
	t.Name("consents")
	t.PrimaryKey(&m.ID)
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.GrantedAt).Default("now()")
	t.Col(&m.Evidence).Default("'{}'::jsonb")
	t.Index(&m.IdentityID, &m.Purpose).
		Where("withdrawn_at IS NULL").Named("consents_identity")
}

// PIIKey is public.pii_keys: the per-tenant key an identity's attributes are
// sealed with. Erasure shreds the key rather than the rows, so referential
// integrity survives for audit while the data becomes unrecoverable.
type PIIKey struct {
	CreatedAt  time.Time
	ShreddedAt *time.Time
	ID         [16]byte
	TenantID   [16]byte
	KeyEnc     []byte
	KmsKeyRef  *string
}

func (m *PIIKey) Schema(t *storm.Table) {
	t.Name("pii_keys")
	t.PrimaryKey(&m.ID)
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.CreatedAt).Default("now()")
	t.Col(&m.KeyEnc).NotNull()
	t.Index(&m.TenantID).Named("pii_keys_tenant")
}

// RealmCategory is public.realm_categories: a subdivision of a population.
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
	t.Name("realm_categories")
	t.PrimaryKey(&m.ID)
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.CreatedAt).Default("now()")
	t.Col(&m.SortOrder).Default("100")
	t.UniqueNamed("realm_categories_id_realm_id_key", &m.ID, &m.RealmID)
	t.UniqueNamed("realm_categories_realm_id_code_key", &m.RealmID, &m.Code)
	t.CheckNamed("realm_categories_code_check", "code ~ '^[a-z][a-z0-9_]{1,30}$'::text")
}

func All() []any {
	return []any{&Credential{}, &Consent{}, &PIIKey{}, &RealmCategory{}}
}
