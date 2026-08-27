package identitydomain

import (
	"testing"
	"time"

	"github.com/gsoultan/anubis/internal/shared/apperr"
)

func TestIdentityGate(t *testing.T) {
	cases := []struct {
		id   Identity
		want error
	}{
		{Identity{Status: "active"}, nil},
		{Identity{Status: "disabled"}, apperr.ErrIdentityDisabled},
		{Identity{Status: "active", Disabled: true}, apperr.ErrIdentityDisabled},
		{Identity{Status: "active", Anonymized: true}, apperr.ErrIdentityDisabled},
		{Identity{Status: "locked"}, apperr.ErrIdentityLocked},
		{Identity{Status: "pending"}, apperr.ErrInvalidCredentials},
	}
	for _, c := range cases {
		if got := c.id.CanAuthenticate(); got != c.want {
			t.Errorf("%+v: got %v want %v", c.id, got, c.want)
		}
	}
}

func TestPasswordPolicy(t *testing.T) {
	p := ParsePasswordPolicy(nil)
	if p.MinLength != 12 {
		t.Fatalf("default min length: %d", p.MinLength)
	}
	if err := p.Check("short"); err == nil {
		t.Fatal("short password accepted")
	}
	if err := p.Check("a long enough passphrase"); err != nil {
		t.Fatalf("good password rejected: %v", err)
	}
	long := make([]byte, 600)
	for i := range long {
		long[i] = 'a'
	}
	if err := p.Check(string(long)); err == nil {
		t.Fatal("absurdly long password accepted (KDF DoS vector)")
	}
}

// --- realm enrolment stance (docs/enrolment-rollout.md) ---

func realmRequiring(required, allowed []string, deadline time.Time) *Realm {
	return &Realm{
		RequiredFactors: required, AllowedFactors: allowed,
		FactorEnrolmentDeadline: deadline,
	}
}

var (
	now       = time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	yesterday = now.Add(-24 * time.Hour)
	tomorrow  = now.Add(24 * time.Hour)
)

// The default has to be "do nothing". Every realm in every existing
// installation has no deadline, and a stance that enforced without one would
// lock out an entire directory on upgrade.
func TestNoDeadlineMeansNoEnforcement(t *testing.T) {
	r := realmRequiring([]string{"password", "totp"}, []string{"password", "totp"}, time.Time{})
	if got, missing := r.EnrolmentStanceFor(nil, now); got != EnrolmentNotEnforced {
		t.Fatalf("stance %v (missing %v) — an upgrade would lock this realm out", got, missing)
	}
}

func TestBeforeTheDeadlineTheMemberIsWarnedNotRefused(t *testing.T) {
	r := realmRequiring([]string{"totp"}, []string{"password", "totp"}, tomorrow)
	got, missing := r.EnrolmentStanceFor([]string{}, now)
	if got != EnrolmentDue {
		t.Fatalf("want EnrolmentDue, got %v", got)
	}
	if len(missing) != 1 || missing[0] != "totp" {
		t.Fatalf("missing factors: %v", missing)
	}
}

func TestAfterTheDeadlineItIsEnrolOrDeny(t *testing.T) {
	r := realmRequiring([]string{"totp"}, []string{"password", "totp"}, yesterday)
	if got, _ := r.EnrolmentStanceFor(nil, now); got != EnrolmentOverdue {
		t.Fatalf("want EnrolmentOverdue, got %v", got)
	}
}

// Somebody who already complied must never be caught by the policy, deadline
// or no deadline — that would lock out exactly the people who did as asked.
func TestAnEnrolledMemberIsNeverCaught(t *testing.T) {
	r := realmRequiring([]string{"totp"}, []string{"password", "totp"}, yesterday)
	if got, _ := r.EnrolmentStanceFor([]string{"totp"}, now); got != EnrolmentNotEnforced {
		t.Fatalf("an enrolled member got stance %v", got)
	}
}

// password is verified before any of this runs. Treating it as an enrolable
// factor would make every member of every realm overdue at once.
func TestPasswordIsNotAnEnrolableFactor(t *testing.T) {
	r := realmRequiring([]string{"password"}, []string{"password"}, yesterday)
	if got, missing := r.EnrolmentStanceFor(nil, now); got != EnrolmentNotEnforced {
		t.Fatalf("password demanded enrolment: %v %v", got, missing)
	}
}

// A realm can require a factor it no longer allows — required_factors and
// allowed_factors are edited separately. Demanding enrolment of a factor the
// realm refuses to accept is an unsatisfiable policy: nobody could comply.
func TestAFactorTheRealmForbidsIsNotDemanded(t *testing.T) {
	r := realmRequiring([]string{"totp"}, []string{"password"}, yesterday)
	if got, missing := r.EnrolmentStanceFor(nil, now); got != EnrolmentNotEnforced {
		t.Fatalf("unsatisfiable policy enforced: %v %v", got, missing)
	}
}

// The deadline is a moment, not a day: at exactly the deadline the policy is
// in force. Ambiguity here is an argument with a customer about a lockout.
func TestTheDeadlineItselfIsEnforced(t *testing.T) {
	r := realmRequiring([]string{"totp"}, []string{"password", "totp"}, now)
	if got, _ := r.EnrolmentStanceFor(nil, now); got != EnrolmentOverdue {
		t.Fatalf("at the deadline the stance was %v", got)
	}
}
