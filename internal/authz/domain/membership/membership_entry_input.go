package membership

import "github.com/gsoultan/anubis/internal/authz/domain/grant"

// MembershipEntryInput replaces one entry when entries are (re)defined.
type MembershipEntryInput struct {
	RoleID string
	Scopes []grant.GrantScopeInput
}
