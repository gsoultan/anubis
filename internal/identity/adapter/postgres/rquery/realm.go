package identityrquery

import (
	"time"

	"github.com/gsoultan/storm"
	"github.com/gsoultan/storm/runtime"
)

// RealmRow is one population and the policy that governs it.
//
// The TTLs arrive as text, which is what an operator edits and what
// round-trips exactly. GetRealm and GetRealmByCode additionally carry them as
// seconds, because the token issuer arithmetics on them and parsing
// PostgreSQL's interval grammar in Go is a larger thing than it sounds.
type RealmRow struct {
	ID                        string
	TenantID                  string
	Code                      string
	Kind                      string
	DisplayName               string
	MinAssurance              int16
	SelfRegistration          bool
	EmailVerificationRequired bool
	PiiEncryption             bool
	AllowedFactors            []string
	RequiredFactors           []string
	PasswordPolicy            runtime.JSON
	FactorEnrolmentDeadline   runtime.Null[time.Time]
	SessionTtl                string
	AccessTokenTtl            string
	RefreshTokenTtl           string
	DefaultRetention          string
}

// RealmWithSecsRow is a realm plus its TTLs in seconds, for the token path.
type RealmWithSecsRow struct {
	ID                        string
	TenantID                  string
	Code                      string
	Kind                      string
	DisplayName               string
	MinAssurance              int16
	SelfRegistration          bool
	EmailVerificationRequired bool
	PiiEncryption             bool
	AllowedFactors            []string
	RequiredFactors           []string
	PasswordPolicy            runtime.JSON
	FactorEnrolmentDeadline   runtime.Null[time.Time]
	SessionTtl                string
	AccessTokenTtl            string
	RefreshTokenTtl           string
	DefaultRetention          string
	SessionTtlSecs            int64
	AccessTokenTtlSecs        int64
	RefreshTokenTtlSecs       int64
}

const realmCols = `
       id::text AS id, tenant_id::text AS tenant_id, code, kind, display_name,
       min_assurance, self_registration, email_verification_required,
       pii_encryption, allowed_factors, required_factors, password_policy,
       factor_enrolment_deadline,
       session_ttl::text  AS session_ttl,
       access_token_ttl::text AS access_token_ttl,
       refresh_token_ttl::text AS refresh_token_ttl,
       COALESCE(default_retention::text, '')::text AS default_retention`

const realmSecs = `,
       extract(epoch FROM session_ttl)::bigint       AS session_ttl_secs,
       extract(epoch FROM access_token_ttl)::bigint  AS access_token_ttl_secs,
       extract(epoch FROM refresh_token_ttl)::bigint AS refresh_token_ttl_secs`

var GetRealmByCode = storm.SQL[RealmWithSecsRow](`
SELECT` + realmCols + realmSecs + `
FROM realms
WHERE tenant_id = $1 AND code = $2`)

var GetRealm = storm.SQL[RealmWithSecsRow](`
SELECT` + realmCols + realmSecs + `
FROM realms
WHERE id = $1`)

var ListRealms = storm.SQL[RealmRow](`
SELECT` + realmCols + `
FROM realms
WHERE tenant_id = $1
ORDER BY code`)

// RealmIDRow is a realm's id.
type RealmIDRow struct {
	ID string
}

// CreateRealm registers a population.
//
// nullif on default_retention: ” means "no statutory limit", and an empty
// string is not an interval — the column has to be NULL for
// SetRetentionFromRealm to skip it rather than compute a date from nothing.
var CreateRealm = storm.SQL[RealmIDRow](`
INSERT INTO realms (tenant_id, code, kind, display_name, min_assurance,
                    self_registration, email_verification_required, pii_encryption,
                    allowed_factors, required_factors, password_policy,
                    session_ttl, access_token_ttl, refresh_token_ttl,
                    default_retention, factor_enrolment_deadline)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::text[], $10::text[], $11::jsonb,
        $12::text::interval, $13::text::interval, $14::text::interval,
        nullif($15, '')::interval, $16::timestamptz)
RETURNING id::text AS id`)

// UpdateRealm changes a realm's POLICY.
//
// code and kind are deliberately absent: they decide which roles a realm's
// members may hold (migrations/0010 enforces it on every grant), so changing
// them on a populated realm would retroactively re-decide access that has
// already been granted and evaluated. CorrectEmptyRealmIdentity is the only
// path that moves them, and only while nobody is in the realm.
var UpdateRealm = storm.SQL[RealmIDRow](`
UPDATE realms SET
    display_name = $2,
    min_assurance = $3,
    self_registration = $4,
    email_verification_required = $5,
    pii_encryption = $6,
    allowed_factors = $7::text[],
    required_factors = $8::text[],
    password_policy = $9::jsonb,
    session_ttl = $10::text::interval,
    access_token_ttl = $11::text::interval,
    refresh_token_ttl = $12::text::interval,
    default_retention = nullif($13, '')::interval,
    -- NULL takes the policy out of force again; enrolments already made
    -- survive, so a second rollout starts ahead of the first.
    factor_enrolment_deadline = $14::timestamptz,
    updated_at = now()
WHERE id = $1
RETURNING id::text AS id`)

// CorrectEmptyRealmIdentity fixes a typo in a realm's code or kind.
//
// The emptiness test lives in the STATEMENT, not in the caller: an identity
// created between a check and an update cannot slip through, because the
// UPDATE simply matches no row. A realm with no members has decided nothing
// yet, and that is when a typo is actually caught.
var CorrectEmptyRealmIdentity = storm.SQLExec(`
UPDATE realms SET code = $3, kind = $4, updated_at = now()
WHERE realms.id = $1
  AND realms.tenant_id = $2
  AND NOT EXISTS (SELECT 1 FROM identities i WHERE i.realm_id = realms.id)`)
