package platformschema

// All is the whole schema: every table, then every function, view and trigger.
//
// It is what cmd/stormddl builds a migration from, and what `stormddl -check`
// diffs against the live database.
func All() []any {
	out := []any{
		&APIKey{},
		&Application{},
		&AuditAnchor{},
		&AuditLog{},
		&AuthPage{},
		&CatalogSyncRun{},
		&CatalogSyncSource{},
		&CatalogVersion{},
		&Consent{},
		&Credential{},
		&GrantScope{},
		&Grant{},
		&Identity{},
		&IdentityLink{},
		&MembershipEntry{},
		&MembershipEntryScope{},
		&MembershipMember{},
		&Membership{},
		&OneTimeToken{},
		&Permission{},
		&PiiKeyTombstone{},
		&PiiKey{},
		&PlatformAPIKey{},
		&PlatformAssignment{},
		&PlatformRefreshToken{},
		&PlatformUser{},
		&RealmCategory{},
		&Realm{},
		&RefreshToken{},
		&RoleGrantable{},
		&RoleParent{},
		&RolePermissionPattern{},
		&RolePermission{},
		&RolePermissionsEffective{},
		&Role{},
		&RoutePolicy{},
		&SchemaMigration{},
		&ScopeAx{},
		&ScopeClosure{},
		&ScopeNodeType{},
		&ScopeNode{},
		&ScopeSyncRun{},
		&ScopeSyncSource{},
		&Session{},
		&SigningKey{},
		&Tenant{},
	}
	return append(out, routines()...)
}
