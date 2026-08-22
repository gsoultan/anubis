package authep

import (
	"github.com/gsoultan/anubis/internal/auth/app/signin"
	"github.com/gsoultan/anubis/internal/identity/app/registration"
)

// Rate-limit key adapters: the limiter must key the ACCOUNT UNDER ATTACK
// (tenant/realm/username), which only the request knows.

type loginRateKeys struct{ signin.LoginInput }

func (r loginRateKeys) RateTenant() string { return r.Tenant }
func (r loginRateKeys) RateAccount() string {
	return r.Tenant + "/" + r.Realm + "/" + r.Username
}

type registerRateKeys struct{ registration.RegisterInput }

func (r registerRateKeys) RateTenant() string  { return r.Tenant }
func (r registerRateKeys) RateAccount() string { return "" }
