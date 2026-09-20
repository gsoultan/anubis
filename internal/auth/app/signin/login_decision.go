package signin

import (
	"time"

	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
)

// Step is what a caller must do next with a password submission.
type Step int

const (
	// StepDeny refuses the submission; Decision.Err carries the reason.
	StepDeny Step = iota
	// StepFactor means the password was right and the identity holds a
	// second factor the realm still accepts. No session yet.
	StepFactor
	// StepEnrol means a factor the realm REQUIRES is not enrolled and the
	// deadline for enrolling it has passed. No session may be issued, and
	// the refusal has to say which factor is missing or it is unactionable.
	StepEnrol
	// StepAllow means the password is sufficient on its own.
	StepAllow
)

// Decision is the outcome of checking a password against a realm's factor
// policy: everything both login doors must agree on, and nothing about how
// either of them then issues a session.
//
// That split is the point. The API door answers with tokens and the browser
// door with a cookie and an authorization code, so those parts cannot be
// shared — but WHETHER to issue anything is one question with one answer,
// and it is decided here.
type Decision struct {
	Step     Step
	Tenant   *tenancydomain.TenantRef
	Realm    *identitydomain.Realm
	Identity *identitydomain.Identity
	// Methods, on StepFactor, are the enrolled factors the realm still
	// accepts. A credential row can outlive the policy that allowed it.
	Methods []string
	// Missing names required factors this identity has not enrolled. Set on
	// StepEnrol, and on a StepAllow that carries Due.
	Missing []string
	// Deadline is the realm's factor enrolment deadline, for the message
	// that tells somebody when their sign-in stops working. Zero when the
	// realm has not set one.
	Deadline time.Time
	// Due marks a StepAllow inside the grace period: signed in, and owed a
	// warning that this will not last.
	Due bool
	// Err is the refusal on StepDeny.
	Err error
}
