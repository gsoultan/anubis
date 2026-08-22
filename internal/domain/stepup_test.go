package domain

import (
	"testing"
	"time"
)

func TestEvaluateStepUp(t *testing.T) {
	now := time.Now()
	req := PermissionRequirements{RequiresAMR: []string{"otp"}, MaxAuthAge: 5 * time.Minute}

	if d := EvaluateStepUp(req, []string{"pwd", "otp"}, now.Add(-time.Minute), now); d != nil {
		t.Fatalf("fresh otp auth must pass, got %+v", d)
	}
	if d := EvaluateStepUp(req, []string{"pwd"}, now.Add(-time.Minute), now); d == nil {
		t.Fatal("missing otp must demand step-up")
	}
	if d := EvaluateStepUp(req, []string{"pwd", "otp"}, now.Add(-41*time.Minute), now); d == nil {
		t.Fatal("stale auth must demand step-up")
	} else if d.AuthAge < 40*time.Minute {
		t.Errorf("auth age misreported: %v", d.AuthAge)
	}
	// Unknown auth time with a recency requirement fails CLOSED.
	if d := EvaluateStepUp(req, []string{"pwd", "otp"}, time.Time{}, now); d == nil {
		t.Fatal("zero auth_time with max_auth_age must fail closed")
	}
	// No requirements: always satisfied.
	if d := EvaluateStepUp(PermissionRequirements{}, nil, time.Time{}, now); d != nil {
		t.Fatalf("no requirements must pass, got %+v", d)
	}
}

func TestIdentityGate(t *testing.T) {
	cases := []struct {
		id   Identity
		want error
	}{
		{Identity{Status: "active"}, nil},
		{Identity{Status: "disabled"}, ErrIdentityDisabled},
		{Identity{Status: "active", Disabled: true}, ErrIdentityDisabled},
		{Identity{Status: "active", Anonymized: true}, ErrIdentityDisabled},
		{Identity{Status: "locked"}, ErrIdentityLocked},
		{Identity{Status: "pending"}, ErrInvalidCredentials},
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
