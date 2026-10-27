package emaillink

import (
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
)

// Completed is what a consumed link proves: this person, in this realm of
// this tenant, asked for this OIDC flow. The door turns it into a session.
type Completed struct {
	Tenant   *tenancydomain.TenantRef
	Realm    *identitydomain.Realm
	Identity *identitydomain.Identity
	Flow     map[string]string
}
