package endpoint

import "github.com/gsoultan/anubis/internal/usecase"

// Rate-limit key adapters: the limiter must key the ACCOUNT UNDER ATTACK
// (tenant/realm/username), which only the request knows.

type loginRateKeys struct{ usecase.LoginInput }

func (r loginRateKeys) RateTenant() string { return r.Tenant }
func (r loginRateKeys) RateAccount() string {
	return r.Tenant + "/" + r.Realm + "/" + r.Username
}

type registerRateKeys struct{ usecase.RegisterInput }

func (r registerRateKeys) RateTenant() string  { return r.Tenant }
func (r registerRateKeys) RateAccount() string { return "" }
