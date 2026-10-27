package authep

import (
	"github.com/gsoultan/anubis/internal/auth/app/accesslink"
	"github.com/gsoultan/anubis/internal/auth/app/clientcreds"
	"github.com/gsoultan/anubis/internal/auth/app/signin"
	identityrecovery "github.com/gsoultan/anubis/internal/identity/app/recovery"
	"github.com/gsoultan/anubis/internal/identity/app/registration"
	"github.com/gsoultan/anubis/internal/platform/crypto/secret"
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

type clientCredsRateKeys struct {
	clientcreds.ClientCredentialsInput
}

func (r clientCredsRateKeys) RateTenant() string  { return r.Tenant }
func (r clientCredsRateKeys) RateAccount() string { return r.Tenant + "/client/" + r.ClientID }

// WrapClientCredentials adapts the input for the limiter; transports use it
// so the rate-key carriers stay unexported.
func WrapClientCredentials(in clientcreds.ClientCredentialsInput) any {
	return clientCredsRateKeys{in}
}

// resetRateKeys keys a reset request the way a sign-in is keyed, so the
// per-account budget is shared with the account it names.
type resetRateKeys struct {
	identityrecovery.RequestResetInput
}

func (r resetRateKeys) RateTenant() string { return r.Tenant }
func (r resetRateKeys) RateAccount() string {
	return r.Tenant + "/" + r.Realm + "/" + r.Username
}

// linkRateKeys keys an access-link open by the link presented, so one
// address guessing at many links and many addresses hammering one link are
// both bounded. The hash, never the token, is what the key carries.
type linkRateKeys struct{ accesslink.OpenInput }

func (r linkRateKeys) RateTenant() string { return r.Tenant }
func (r linkRateKeys) RateAccount() string {
	return r.Tenant + "/link/" + secret.Hex(secret.Hash(r.Token))[:16]
}
