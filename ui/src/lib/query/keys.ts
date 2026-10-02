/* Query keys as a factory rather than inline arrays: invalidating "everything
   under scope" must be one call, not a grep for string literals. */
export const qk = {
  realms: () => ['realms'] as const,
  signingKeys: () => ['signing-keys'] as const,
  credentials: (id: string) => ['credentials', id] as const,
  tenants: () => ['tenants'] as const,
  tenantStats: (id: string) => ['tenant-stats', id] as const,
  signin: (tenantId: string) => ['signin', tenantId] as const,
  authPages: (kind: string) => ['auth-pages', kind] as const,
  realmCategories: (realmId?: string) => ['realm-categories', realmId ?? 'all'] as const,
  axes: () => ['axes'] as const,
  nodeTypes: () => ['node-types'] as const,

  scope: () => ['scope'] as const,
  /* `archived` adds a segment only when on, so every picker keeps sharing the
     one cached listing and only the Structure page's "show archived" differs. */
  scopeChildren: (axis: string, parent: string | null, archived = false) =>
    [...qk.scope(), axis, 'children', parent ?? 'root', ...(archived ? ['archived'] : [])] as const,
  scopeSearch: (axis: string, q: string, archived = false) =>
    [...qk.scope(), axis, 'search', q, ...(archived ? ['archived'] : [])] as const,
  scopeNode: (id: string) => [...qk.scope(), 'node', id] as const,
  ancestorPath: (id: string) => [...qk.scope(), 'path', id] as const,

  identities: (realm?: string | null, q?: string) =>
    ['identities', realm ?? 'all', q ?? ''] as const,
  identity: (id: string) => ['identities', 'detail', id] as const,

  permissions: () => ['permissions'] as const,
  roles: () => ['roles'] as const,
  rolePermissions: (id: string) => ['roles', id, 'permissions'] as const,
  grants: (identityId?: string) => ['grants', identityId ?? 'all'] as const,
  memberships: () => ['memberships'] as const,
  /** Under memberships(), so a write that invalidates memberships also
      refreshes every roster and every person's list of them. */
  membershipAssignments: (by: { membershipId?: string; identityId?: string }) =>
    ['memberships', 'assignments', by.membershipId ?? '', by.identityId ?? ''] as const,
  syncSources: () => ['sync-sources'] as const,
  syncRuns: (sourceId: string) => ['sync-runs', sourceId] as const,
  catalogSources: () => ['catalog-sources'] as const,
  catalogRuns: (sourceId: string) => ['catalog-runs', sourceId] as const,

  dashboard: () => ['dashboard'] as const,
  audit: () => ['audit'] as const,
}
