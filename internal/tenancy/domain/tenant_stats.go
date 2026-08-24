package tenancydomain

// TenantStats is what a tenant holds, counted live. A stale number beside a
// tenant somebody is about to retire is worse than no number.
type TenantStats struct {
	Identities  int
	Grants      int
	ScopeNodes  int
	Memberships int
}
