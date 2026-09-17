// Package controlrmodel declares the control-plane tables storm generates
// code for.
//
// It is a PROJECTION of the schema, not its source of truth: anubis's schema
// of record stays migrations/ (forward-only, checksummed), and this model is
// validated against the live schema the same way the raw queries are — by
// preparing generated statements against it. Columns and defaults here must
// match `\d` on the table exactly.
//
// These are the PLATFORM tables (ADR-0011): a separate population from
// identities, with no join between them. A tenant's user is not an operator
// and cannot be made into one, and that separation is why these tables carry
// their own credentials rather than reusing the identity context's.
//
// platform_assignments.tenant_id references tenants, which the TENANCY context
// owns. It is declared here as a plain column rather than a relation: a
// projection only needs the column to read and filter it, and modelling
// another context's table would put its shape in this package's hands — the
// boundary AGENTS.md draws. The foreign key itself lives in migrations/, which
// is where every constraint lives.
package controlrmodel

import (
	"time"

	"github.com/gsoultan/storm"
)

// PlatformUser is public.platform_users: an operator of the control plane.
type PlatformUser struct {
	storm.Model

	Username     string
	Email        *string
	PasswordHash string
	Status       string

	// TokenEpoch invalidates every live token for this operator when it
	// moves. Disabling an account bumps it, which is what makes "disabled"
	// take effect now rather than whenever the token happened to expire.
	TokenEpoch int32

	LastLoginAt *time.Time
	DisabledAt  *time.Time

	TotpSecretEnc  []byte
	TotpEnrolledAt *time.Time
	// TotpLastStep is the last accepted time step, so a code cannot be
	// replayed inside its own validity window.
	TotpLastStep int64
}

func (m *PlatformUser) Schema(t *storm.Table) {
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.Status).Default("'active'::text")
	t.Col(&m.TokenEpoch).Default("0")
	t.Col(&m.TotpLastStep).Default("0")

	t.CheckNamed("platform_users_status_check",
		"status = ANY (ARRAY['active'::text, 'disabled'::text])")
	// An enrolled operator must still have a secret: dropping the secret
	// while leaving the enrolment would demand a factor nobody can produce.
	t.CheckNamed("platform_users_totp_complete",
		"(totp_enrolled_at IS NULL) OR (totp_secret_enc IS NOT NULL)")
	t.CheckNamed("platform_users_username_check",
		"username ~ '^[a-zA-Z0-9][a-zA-Z0-9._-]{1,62}$'::text")

	t.Index(storm.Lower(&m.Username)).Unique().Named("platform_users_username")
	t.Index(storm.Lower(&m.Email)).Unique().
		Where("(email IS NOT NULL) AND (email <> ''::text)").Named("platform_users_email")
}

// PlatformAPIKey is public.platform_api_keys: a machine credential that acts
// as its owner, and stops working the moment that owner does.
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
	t.Name("platform_api_keys")
	t.PrimaryKey(&m.ID)
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.Label).Default("''::text")
	t.Col(&m.CreatedAt).Default("now()")
	t.Col(&m.PlatformUser).ConstraintName("platform_api_keys_platform_user_id_fkey")
	t.Col(&m.PlatformUser).OnDelete(storm.Cascade)
	t.Col(&m.CreatedBy).Named("created_by")
	t.Col(&m.CreatedBy).ConstraintName("platform_api_keys_created_by_fkey")
	t.Col(&m.CreatedBy).OnDelete(storm.SetNull)
	t.Col(&m.CreatedBy).NoIndex()

	// Unique only over LIVE keys: a revoked key keeps its lookup value, so a
	// full unique index would let one revoked key hold that value forever.
	t.Index(&m.Lookup).Unique().Where("revoked_at IS NULL").Named("platform_api_keys_lookup")
	t.Index(&m.PlatformUser, &m.RevokedAt).Named("platform_api_keys_owner")
}

// PlatformRefreshToken is public.platform_refresh_tokens: single-use,
// family-revoked, hash-only.
type PlatformRefreshToken struct {
	ID           [16]byte
	PlatformUser PlatformUser

	// FamilyID is the root token's own id, which is what lets one revocation
	// kill a sign-in however many times it has rotated.
	FamilyID  [16]byte
	TokenHash []byte
	CreatedAt time.Time
	ExpiresAt time.Time
	UsedAt    *time.Time
	RevokedAt *time.Time
}

func (m *PlatformRefreshToken) Schema(t *storm.Table) {
	t.Name("platform_refresh_tokens")
	t.PrimaryKey(&m.ID)
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.CreatedAt).Default("now()")
	t.Col(&m.PlatformUser).ConstraintName("platform_refresh_tokens_platform_user_id_fkey")
	t.Col(&m.PlatformUser).OnDelete(storm.Cascade)
	t.Col(&m.PlatformUser).NoIndex()
	t.Col(&m.TokenHash).NotNull()

	// The column stores a hash and nothing else; the length check is what
	// stops a raw token being written here by mistake.
	t.CheckNamed("platform_refresh_hash_len", "octet_length(token_hash) = 32")
	t.Index(&m.TokenHash).Unique().Named("platform_refresh_by_hash")
	t.Index(&m.FamilyID).Named("platform_refresh_by_family")
}

// PlatformAssignment is public.platform_assignments: which tenants an operator
// may administer (ADR-0011). It is never a substitute for a grant, which is
// what a tenant's own members hold.
type PlatformAssignment struct {
	storm.Model

	Operator PlatformUser

	// TenantID is NULL for a global assignment — an owner of the whole
	// installation rather than of one tenant. See the package comment for why
	// this is a column and not a relation.
	TenantID   *[16]byte
	Role       string
	GrantedBy  *PlatformUser
	Reason     string
	ValidUntil *time.Time
	RevokedAt  *time.Time
}

func (m *PlatformAssignment) Schema(t *storm.Table) {
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.Reason).Default("''::text")
	t.Col(&m.Operator).ConstraintName("platform_assignments_operator_id_fkey")
	t.Col(&m.Operator).OnDelete(storm.Cascade)
	t.Col(&m.GrantedBy).Named("granted_by")
	t.Col(&m.GrantedBy).ConstraintName("platform_assignments_granted_by_fkey")
	t.Col(&m.GrantedBy).OnDelete(storm.SetNull)
	t.Col(&m.GrantedBy).NoIndex()

	t.CheckNamed("platform_assignments_role_check",
		"role = ANY (ARRAY['support'::text, 'admin'::text, 'owner'::text])")

	// Two partial uniques, not one: NULL is not equal to itself, so a single
	// index over (operator_id, tenant_id) would let an operator hold any
	// number of GLOBAL assignments. The second index is what bounds that.
	t.Index(&m.Operator, &m.TenantID).Unique().
		Where("(revoked_at IS NULL) AND (tenant_id IS NOT NULL)").
		Named("platform_assignments_live")
	t.Index(&m.Operator).Unique().
		Where("(revoked_at IS NULL) AND (tenant_id IS NULL)").
		Named("platform_assignments_live_global")
	t.Index(&m.Operator, &m.TenantID, &m.RevokedAt).Named("platform_assignments_lookup")
}

func All() []any {
	return []any{&PlatformUser{}, &PlatformAPIKey{}, &PlatformRefreshToken{}, &PlatformAssignment{}}
}
