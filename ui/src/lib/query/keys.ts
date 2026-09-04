/* Query keys as a factory rather than inline arrays: invalidating "everything
   under scope" must be one call, not a grep for string literals. */
export const qk = {
  realms: () => ['realms'] as const,
  signingKeys: () => ['signing-keys'] as const,
  tenants: () => ['tenants'] as const,
  tenantStats: (id: string) => ['tenant-stats', id] as const,
  signin: (tenantId: string) => ['signin', tenantId] as const,
  authPages: (kind: string) => ['auth-pages', kind] as const,
  realmCategories: (realmId?: string) => ['realm-categories', realmId ?? 'all'] as const,
  axes: () => ['axes'] as const,
  nodeTypes: () => ['node-types'] as const,

  scope: () => ['scope'] as const,
  scopeChildren: (axis: string, parent: string | null) =>
    [...qk.scope(), axis, 'children', parent ?? 'root'] as const,
  scopeSearch: (axis: string, q: string) => [...qk.scope(), axis, 'search', q] as const,
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
  syncSources: () => ['sync-sources'] as const,
  syncRuns: (sourceId: string) => ['sync-runs', sourceId] as const,

  dashboard: () => ['dashboard'] as const,
  audit: () => ['audit'] as const,
}
