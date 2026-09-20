package signin

import (
	"time"

	authapp "github.com/gsoultan/anubis/internal/auth/app"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
	"github.com/gsoultan/anubis/internal/platform/crypto/localtoken"
	"github.com/gsoultan/anubis/internal/platform/crypto/secret"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/clock"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
)

// enrolGrantTTL is longer than an MFA challenge on purpose: the holder has to
// install an authenticator app, transfer a key and type a number, not read
// six digits they already have. Still short enough that a leaked grant is a
// narrow window.
const enrolGrantTTL = 15 * time.Minute

// EnrolmentGranter mints the token that makes an enrolment refusal
// actionable.
//
// Both doors refuse the same member and both must offer the same way out, so
// the token is minted in one place. The API hands it to the caller; the
// hosted page keeps it server-side and spends it on the visitor's behalf,
// which is the only difference between them.
//
// The grant is minted only for a member with NONE of the required factors
// enrolled, and the enrolment usecase re-checks that when it is redeemed —
// otherwise a leaked grant would REPLACE somebody's authenticator rather than
// add their first, which is an account takeover wearing a compliance hat.
type EnrolmentGranter struct {
	ring  *keyring.Manager
	clock clock.Clock
}

func NewEnrolmentGranter(ring *keyring.Manager, clk clock.Clock) *EnrolmentGranter {
	return &EnrolmentGranter{ring: ring, clock: clk}
}

// Grant returns the challenge a refused member is answered with. Its holder
// has already presented the correct password — the same bar as an MFA
// challenge token, and it buys strictly less: no session, no scopes.
func (g *EnrolmentGranter) Grant(
	tenant *tenancydomain.TenantRef, realm *identitydomain.Realm,
	identity *identitydomain.Identity, missing []string,
) (*authapp.EnrolmentChallenge, error) {
	key, err := g.ring.Ring().ActiveLocal()
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	jti, err := secret.New(16)
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	token, err := localtoken.Seal(key.Secret, key.Kid, "enrol_grant", jti, authapp.EnrolmentGrant{
		TenantID:   tenant.ID,
		TenantSlug: tenant.Slug,
		IdentityID: identity.ID,
		RealmID:    realm.ID,
		Factors:    missing,
	}, enrolGrantTTL, g.clock.Now())
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	return &authapp.EnrolmentChallenge{
		Factors:    missing,
		Deadline:   realm.FactorEnrolmentDeadline,
		GrantToken: token,
		ExpiresIn:  int(enrolGrantTTL.Seconds()),
	}, nil
}
