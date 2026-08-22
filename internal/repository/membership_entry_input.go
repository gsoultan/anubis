package repository

// MembershipEntryInput replaces one entry when entries are (re)defined.
type MembershipEntryInput struct {
	RoleID string
	Scopes []GrantScopeInput
}
