package signin

import (
	"context"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	auditport "github.com/gsoultan/anubis/internal/audit/port"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	"github.com/gsoultan/anubis/internal/identity/domain/credential"
	identityport "github.com/gsoultan/anubis/internal/identity/port"
	"github.com/gsoultan/anubis/internal/platform/crypto/kdf"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
	"github.com/gsoultan/anubis/internal/shared/clock"
	"github.com/gsoultan/anubis/internal/shared/jsonx"
	"github.com/gsoultan/anubis/internal/shared/validate"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
)

// Surfaces name the door a submission arrived at. The difference belongs in
// the audit detail and nowhere else — a factor belongs to the identity, not
// to the door.
const (
	SurfaceAPI     = "api"
	SurfaceBrowser = "browser"
)

// PasswordAuthenticator verifies a password and applies the realm's factor
// policy. It is the single implementation of that question.
//
// It exists because login has TWO doors. AuthService.Login and the hosted
// sign-in page each resolved the tenant, realm, identity and credential,
// verified the password and decided about second factors on their own, so
// every property held in one of them, or in both only by coincidence. Two
// of those coincidences were holes: the page ignored an ENROLLED factor
// entirely, and once that was fixed it still admitted a member whose realm
// had passed its enrolment deadline — EnrolmentStanceFor had exactly one
// caller in the tree. Both were reachable with a stolen password, through
// the door most humans use.
//
// A guard that lives somewhere other than where the thing happens is not a
// guard. Password verification is the thing that happens; this is where it
// happens.
type PasswordAuthenticator struct {
	tenants tenancyport.TenantRepository
	realms  identityport.RealmRepository
	ids     identityport.IdentityRepository
	creds   identityport.CredentialRepository
	clock   clock.Clock
	audit   auditport.Auditor
}

func NewPasswordAuthenticator(
	tenants tenancyport.TenantRepository,
	realms identityport.RealmRepository,
	ids identityport.IdentityRepository,
	creds identityport.CredentialRepository,
	clk clock.Clock,
	audit auditport.Auditor,
) *PasswordAuthenticator {
	return &PasswordAuthenticator{
		tenants: tenants, realms: realms, ids: ids, creds: creds,
		clock: clk, audit: audit,
	}
}

// Authenticate checks the password and returns what the caller must do next.
//
// Every refusal is audited here rather than by the caller, because a refusal
// is complete the moment it is decided and because the browser door proved
// that an audit left to each caller is an audit one of them skips: not one
// failed sign-in through the hosted page was ever recorded. The success
// event stays with the caller — only the door knows what it ended up
// issuing, and a session id is worth more on that record than uniformity.
func (a *PasswordAuthenticator) Authenticate(ctx context.Context, in LoginInput, surface string) Decision {
	if in.Realm == "" {
		in.Realm = "internal"
	}
	if !validate.ValidSlug(in.Tenant) || !validate.ValidUsername(in.Username) ||
		in.Password == "" || len(in.Password) > 512 {
		// Same KDF burn as every other failure: input shape must not be a
		// faster rejection than a wrong password.
		a.burnKDF(in.Password)
		return Decision{Step: StepDeny, Err: apperr.ErrInvalidCredentials}
	}

	// Resolve tenant, realm, identity, credential — collecting rather than
	// early-returning, so every failure converges on ONE verify call.
	var (
		identity *identitydomain.Identity
		cred     *credential.Credential
		realm    *identitydomain.Realm
		tenant   *tenancydomain.TenantRef
	)
	tenant, err := a.tenants.TenantBySlug(ctx, in.Tenant)
	if err == nil && tenant != nil {
		realm, err = a.realms.RealmByCode(ctx, tenant.ID, in.Realm)
		if err == nil && realm != nil && realm.AllowsFactor("password") {
			identity, _ = a.ids.IdentityForLogin(ctx, tenant.ID, realm.ID, in.Username)
			if identity != nil {
				cred, _ = a.creds.PasswordCredential(ctx, identity.ID)
			}
		}
	}

	hash := kdf.Dummy()
	if cred != nil && cred.Secret != "" {
		hash = cred.Secret
	}
	ok, needsRehash, kerr := kdf.Verify(in.Password, hash)
	if kerr != nil || cred == nil || identity == nil || !ok {
		a.AuditLogin(ctx, tenant, identity, surface, "deny", "invalid_credentials", "")
		return Decision{Step: StepDeny, Tenant: tenant, Realm: realm, Err: apperr.ErrInvalidCredentials}
	}
	if aerr := identity.CanAuthenticate(); aerr != nil {
		a.AuditLogin(ctx, tenant, identity, surface, "deny", apperr.AsError(aerr).Code, "")
		return Decision{Step: StepDeny, Tenant: tenant, Realm: realm, Identity: identity, Err: aerr}
	}

	if needsRehash {
		if newHash, herr := kdf.Hash(in.Password); herr == nil {
			// No kid: a password is hashed, not sealed, so no key opens it.
			_ = a.creds.UpdateCredentialSecret(ctx, cred.ID, newHash, "")
		}
	}

	d := Decision{
		Tenant: tenant, Realm: realm, Identity: identity,
		Deadline: realm.FactorEnrolmentDeadline,
	}

	// Second factor: the identity holds one and the realm still accepts it.
	// Enrolment is honoured on its own, before any realm policy — somebody
	// who added an authenticator is asked for it either way.
	enrolled, lerr := a.enrolledFactorKinds(ctx, identity.ID)
	if lerr != nil {
		// We could not find out what this identity has enrolled, so we
		// cannot apply the factor policy — and admitting somebody whose
		// second factor we failed to look up is the single thing this
		// function exists to prevent.
		a.AuditLogin(ctx, tenant, identity, surface, "deny", "factor_lookup_failed", "")
		d.Step, d.Err = StepDeny, lerr
		return d
	}
	if methods := allowedOf(realm, enrolled); len(methods) > 0 {
		a.AuditLogin(ctx, tenant, identity, surface, "allow", "mfa_challenge", "")
		d.Step, d.Methods = StepFactor, methods
		return d
	}

	// Required but NOT enrolled. Which of the two answers this gets is the
	// difference between a rollout and a lockout, so it is decided by a date
	// the operator set, not by a flag.
	stance, missing := realm.EnrolmentStanceFor(enrolled, a.clock.Now())
	d.Missing = missing
	if stance == identitydomain.EnrolmentOverdue {
		a.AuditLogin(ctx, tenant, identity, surface, "deny", "enrolment_required", "")
		d.Step = StepEnrol
		return d
	}
	d.Step, d.Due = StepAllow, stance == identitydomain.EnrolmentDue
	return d
}

// MissingFactors reports which factors the realm requires that this identity
// has not enrolled.
//
// The hosted page needs this after a sign-in has already succeeded, to offer
// enrolment to somebody inside the grace period. It goes through the same
// fail-closed lookup as Authenticate rather than being re-derived at the
// transport — that re-derivation is what this type exists to prevent.
func (a *PasswordAuthenticator) MissingFactors(ctx context.Context, realm *identitydomain.Realm, identityID string) ([]string, error) {
	enrolled, err := a.enrolledFactorKinds(ctx, identityID)
	if err != nil {
		return nil, err
	}
	_, missing := realm.EnrolmentStanceFor(enrolled, a.clock.Now())
	return missing, nil
}

// AuditLogin writes one auth.login event. Both doors call it so the record
// has one shape whichever was used, and the surface says which.
func (a *PasswordAuthenticator) AuditLogin(
	ctx context.Context,
	tenant *tenancydomain.TenantRef, identity *identitydomain.Identity,
	surface, result, detail, sessionID string,
) {
	if tenant == nil {
		return // nothing to chain the event to; transport logs carry the rest
	}
	ev := auditdomain.AuditEvent{
		TenantID:  tenant.ID,
		ActorKind: "identity",
		SessionID: sessionID,
		Action:    "auth.login",
		Result:    result,
		IP:        authctx.ClientIP(ctx),
		Detail:    jsonx.Must(map[string]string{"detail": detail, "surface": surface}),
	}
	if identity != nil {
		ev.ActorID = identity.ID
	}
	a.audit.Emit(ctx, ev)
}

func (a *PasswordAuthenticator) burnKDF(password string) {
	_, _, _ = kdf.Verify(password, kdf.Dummy())
}

// enrolledFactorKinds lists the second factors this identity actually holds.
//
// A factor is demanded when the identity HAS IT ENROLLED — not only when the
// realm requires it. Enrolment is an opt-in to stronger authentication, and
// honouring it only in realms that already mandate MFA would mean a user who
// deliberately added an authenticator still signs in with a password alone.
// That is the entire value of the feature, silently discarded.
//
// The realm's allowed_factors still governs, in allowedOf: a factor the realm
// forbids is not offered even if a credential row survives from before the
// policy changed.
//
// Required-but-unenrolled is handled separately, by the realm's enrolment
// deadline: see Realm.EnrolmentStanceFor and docs/enrolment-rollout.md.
//
// The lookup error is returned rather than swallowed. A failed read is not
// evidence that nobody enrolled anything, and reading it as "no factors"
// signs somebody in without the authenticator they registered — a fail-OPEN
// on the exact question this answers. The two doors disagreed about this
// before they shared an implementation: the hosted page answered "a factor
// exists" on error and the interactor returned nil and carried on. Merging
// them had to take the safer reading rather than the shorter one.
func (a *PasswordAuthenticator) enrolledFactorKinds(ctx context.Context, identityID string) ([]string, error) {
	kinds, err := a.creds.ActiveCredentialKinds(ctx, identityID)
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	var out []string
	for _, k := range kinds {
		switch k {
		case "totp", "device_key":
			out = append(out, k)
		}
	}
	return out, nil
}

// allowedOf keeps only the factors the realm still accepts. A credential row
// can outlive the policy that allowed it.
func allowedOf(realm *identitydomain.Realm, kinds []string) []string {
	var out []string
	for _, k := range kinds {
		if realm.AllowsFactor(k) {
			out = append(out, k)
		}
	}
	return out
}
