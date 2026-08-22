package identitydomain

import (
	"testing"

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
