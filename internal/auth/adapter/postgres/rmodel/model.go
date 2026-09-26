// Package authrmodel declares the auth tables storm generates code for.
//
// It is a PROJECTION of the schema, not its source of truth: anubis's schema
// of record stays migrations/ (forward-only, checksummed), and this model is
// validated against the live schema the same way the raw queries are.
//
// Most of this context's statements are NOT builders, and that is the shape of
// the context rather than a shortfall. A session and a token are state
// MACHINES: nearly every write is a guarded transition stamped with the
// server's clock — `SET status = 'consumed' WHERE status = 'active'` — and the
// guard belongs in the statement, because a read-then-write is exactly the
// replay window these tables exist to close. The model is what the plain reads
// and inserts are built from.
//
// `identities` and `realms` belong to the identity context and `applications`
// to tenancy, so the columns that reference them are plain rather than
// relations — the boundary AGENTS.md draws. Their foreign keys live in
// migrations/, which is where every constraint lives.
package authrmodel

import (
	"net/netip"
	"time"

	"github.com/gsoultan/storm"
)

// Session is public.sessions: one sign-in, alive until it expires or is
// revoked.
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

	// CookieHash is cleared on revocation, so a revoked session's cookie
	// stops resolving immediately rather than at expiry.
	CookieHash []byte
}

func (m *Session) Schema(t *storm.Table) {
	t.Name("sessions")
	t.PrimaryKey(&m.ID)
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.CreatedAt).Default("now()")
	t.Col(&m.LastSeenAt).Default("now()")
	// auth_time is when the credentials were last PROVEN, which a step-up
	// restarts — it is what a max_age check reads, not created_at.
	t.Col(&m.AuthTime).Default("now()")
	t.Col(&m.Amr).Default("'{}'::text[]")
	t.Col(&m.ActiveScopes).Default("'{}'::jsonb")

	t.UniqueNamed("sessions_id_tenant_id_key", &m.ID, &m.TenantID)
	// Unique over LIVE sessions only: a revoked row keeps its hash column
	// until it is cleared, and a full unique index would collide with it.
	t.Index(&m.CookieHash).Unique().
		Where("(cookie_hash IS NOT NULL) AND (revoked_at IS NULL)").Named("sessions_cookie")
	t.Index(&m.ExpiresAt).Where("revoked_at IS NULL").Named("sessions_expiry")
	t.Index(&m.IdentityID, storm.Desc(&m.CreatedAt)).
		Where("revoked_at IS NULL").Named("sessions_identity")
}

// RefreshToken is public.refresh_tokens: one generation of one family.
//
// RANGE partitioned on expires_at so an expired month can be dropped whole
// rather than deleted row by row; the partitions are made by
// ensure_month_partitions on a schedule, and storm leaves them alone.
type RefreshToken struct {
	CreatedAt  time.Time
	ExpiresAt  time.Time
	ConsumedAt *time.Time
	RevokedAt  *time.Time
	ID         [16]byte
	SessionID  [16]byte
	TenantID   [16]byte

	// FamilyID ties every rotation of one sign-in together, which is what
	// lets a single theft response kill all of them.
	FamilyID    [16]byte
	SuccessorID *[16]byte
	Generation  int32
	Status      string
	TokenHash   []byte
	BoundKey    *string
	// ClientID is the application the family was issued to, so a rotation
	// re-issues for it (0051). NULL: issued with no client.
	ClientID *string
}

func (m *RefreshToken) Schema(t *storm.Table) {
	t.Name("refresh_tokens")
	// expires_at is in the key because PostgreSQL requires the partition key
	// in every unique key on a partitioned table. The identity is id alone.
	t.PrimaryKey(&m.ID, &m.ExpiresAt)
	t.PartitionBy(storm.RangePartition, &m.ExpiresAt)

	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.CreatedAt).Default("now()")
	t.Col(&m.Generation).Default("0")
	t.Col(&m.Status).Default("'active'::text")
	t.Col(&m.TokenHash).NotNull()

	t.CheckNamed("refresh_tokens_status_check",
		"status = ANY (ARRAY['active'::text, 'consumed'::text, 'revoked'::text])")
	t.Index(&m.FamilyID).Where("status = 'active'::text").Named("refresh_tokens_family")
	t.Index(&m.TokenHash, &m.ExpiresAt).Unique().Named("refresh_tokens_hash")
	t.Index(&m.SessionID).Named("refresh_tokens_session")
}

// OneTimeToken is public.one_time_tokens: a single-use credential consumed by
// DELETE ... RETURNING, so a second presentation finds no row.
type OneTimeToken struct {
	CreatedAt time.Time
	ExpiresAt time.Time
	ID        [16]byte
	TenantID  [16]byte
	Kind      string
	TokenHash []byte
	Payload   storm.JSON
}

func (m *OneTimeToken) Schema(t *storm.Table) {
	t.Name("one_time_tokens")
	t.PrimaryKey(&m.ID)
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.CreatedAt).Default("now()")
	t.Col(&m.TokenHash).NotNull()
	t.Col(&m.Payload).Default("'{}'::jsonb")
	t.UniqueNamed("one_time_tokens_token_hash_key", &m.TokenHash)
	t.CheckNamed("one_time_tokens_kind_check",
		"kind = ANY (ARRAY['mfa'::text, 'browser_mfa'::text, 'auth_code'::text, 'device_challenge'::text, "+
			"'email_verify'::text, 'password_reset'::text])")
	t.Index(&m.ExpiresAt).Named("one_time_tokens_expiry")
}

// APIKey is public.api_keys: a tenant's machine credential.
type APIKey struct {
	CreatedAt  time.Time
	ID         [16]byte
	TenantID   [16]byte
	Label      string
	Lookup     string
	SecretHash string
	CreatedBy  *[16]byte
	LastUsedAt *time.Time
	ExpiresAt  *time.Time
	RevokedAt  *time.Time
}

func (m *APIKey) Schema(t *storm.Table) {
	t.Name("api_keys")
	t.PrimaryKey(&m.ID)
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.CreatedAt).Default("now()")
	t.Col(&m.Label).Default("''::text")
	t.Index(&m.Lookup).Unique().Where("revoked_at IS NULL").Named("api_keys_lookup")
	t.Index(&m.TenantID, &m.RevokedAt).Named("api_keys_tenant")
}

// SigningKey is public.signing_keys: the keypairs tokens are signed and
// verified with. The private half is stored sealed.
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
}

func (m *SigningKey) Schema(t *storm.Table) {
	t.Name("signing_keys")
	t.PrimaryKey(&m.ID)
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.CreatedAt).Default("now()")
	t.Col(&m.Alg).Default("'Ed25519'::text")
	// A new key is PENDING: it can be verified against before it is promoted
	// to signing, which is what makes a rotation seamless for tokens already
	// in flight.
	t.Col(&m.Status).Default("'pending'::text")
	t.Col(&m.Purpose).Default("'access'::text")
	t.Col(&m.PublicKey).NotNull()
	t.Col(&m.PrivateKeyEnc).NotNull()
	t.UniqueNamed("signing_keys_kid_key", &m.Kid)
	t.CheckNamed("signing_keys_alg_check", "alg = ANY (ARRAY['Ed25519'::text, 'ES256'::text])")
	t.CheckNamed("signing_keys_purpose_check", "purpose = ANY (ARRAY['access'::text, 'local'::text])")
}

func All() []any {
	return []any{&Session{}, &RefreshToken{}, &OneTimeToken{}, &APIKey{}, &SigningKey{}}
}
