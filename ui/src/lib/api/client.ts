/* The single seam between the console and the backend.

   Everything above this file is transport-agnostic: screens import this, and
   only this, so what they talk to can change without touching them.

   LIVE — every function here reaches the Go service (see live.ts), reads and
   writes alike: the Create drawers commit real rows, and the overview counts
   real ones. There is no sample data left in the console's data path.

   NO RPC EXISTS, and the seam says so instead of pretending:
     createPermission (manifests own the catalog), deleteRole,
     setNodeTypeParents

   Keep this ledger accurate. It is the first thing anyone reads to find out
   what the console actually talks to. */
import * as live from './live'

/* A tenant's slug is derived once, at creation, and never edited: it appears
   in URLs, tokens and every hosted page path. */
function slugify(name: string): string {
  return name.toLowerCase().trim().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '').slice(0, 63)
}
import type {
  AuthorizeRequest, NewAxisInput, NewGrantInput, NewIdentityInput,
  NewNodeInput, NewPermissionInput, NewRoleInput, Uuid,
} from './types'

export const api = {
  tenants: () => live.tenants(),
  tenantStats: (id: Uuid) => live.tenantStats(id),
  createTenant: (i: { name: string; slug?: string }) =>
    live.createTenant({ slug: i.slug ?? slugify(i.name), name: i.name }),
  updateTenant: (id: Uuid, name: string) => live.updateTenant(id, name),
  setTenantStatus: (id: Uuid, status: 'active' | 'suspended' | 'archived') =>
    live.setTenantStatus(id, status),
  /* The tenant being edited comes from the header picker, not an argument:
     the server scopes it from the caller's session, so passing an id here
     would let a screen ask about a tenant the caller cannot reach. */
  /* auth_pages, not the legacy signin_pages pair — the hosted page renders
     from the former. See live.ts. */
  authPages: (kind?: import('./types').PageKind) => live.authPages(kind),
  authPage: (id: string) => live.authPage(id),
  saveAuthPage: (page: import('./types').AuthPage) => live.saveAuthPage(page),
  createAuthPage: (input: Parameters<typeof live.createAuthPage>[0]) => live.createAuthPage(input),
  deleteAuthPage: (id: string) => live.deleteAuthPage(id),
  setDefaultAuthPage: (id: string) => live.setDefaultAuthPage(id),
  validatePageConfig: (kind: import('./types').PageKind, cfg: import('./types').PageConfig) =>
    live.validatePageConfig(kind, cfg),

  realms: () => live.realms(),
  updateApplication: (a: Parameters<typeof live.updateApplication>[0]) => live.updateApplication(a),
  verifyAuditChain: () => live.verifyAuditChain(),
  realmCategories: (realmId?: Uuid) => live.realmCategories(realmId),
  axes: () => live.axes(),
  nodeTypes: () => live.nodeTypes(),

  /** Children of a node, or axis roots when parentId is null. Lazy by design:
      a production customer axis holds ~20k nodes and must never be sent whole. */
  scopeChildren: (axisCode: string, parentId: Uuid | null) => live.scopeChildren(axisCode, parentId),

  scopeSearch: (axisCode: string, q: string) => live.scopeSearch(axisCode, q),

  scopeNode: (id: Uuid) => live.scopeNode(id),
  scopeNodesByIds: (ids: Uuid[]) => live.scopeNodesByIds(ids),
  ancestorPath: (id: Uuid) => live.ancestorPath(id),

  identities: (realmId?: string, q?: string) => live.identities(realmId, q),
  identity: (id: Uuid) => live.identity(id),
  /* Encrypted at rest and read through its own call, because it is the only
     part of an identity this server cannot see without a key (ADR-0013). */
  identityAttributes: (id: Uuid) => live.identityAttributes(id),
  setIdentityAttributes: (id: Uuid, a: Record<string, string>) =>
    live.setIdentityAttributes(id, a),

  permissions: () => live.permissions(),
  roles: () => live.roles(),
  rolePermissions: (roleId: string) => live.rolePermissions(roleId),

  grants: (identityId?: Uuid) => live.grants(identityId),
  searchGrants: live.searchGrants,

  authorize: (req: AuthorizeRequest) => live.authorize(req),
  applications: () => live.applicationChoices(),

  // ---- write operations -------------------------------------------------
  createIdentity: (i: NewIdentityInput) => live.createIdentity(i),
  createRealmCategory: (i: { realm_id: Uuid; display_name: string }) =>
    live.createRealmCategory(i),
  setIdentityStatus: (id: Uuid, status: 'active' | 'disabled') =>
    live.setIdentityStatus(id, status),
  /* There is deliberately no CreatePermission RPC: permissions are declared
     by the owning application's manifest, so the catalog and the code that
     enforces it cannot drift. */
  createPermission: (_i: NewPermissionInput): Promise<never> =>
    Promise.reject(new Error('Permissions are declared by the owning application\u2019s manifest — Applications \u2192 Manifest.')),
  createRole: (i: NewRoleInput) => live.createRole(i),
  deleteRole: (_roleId: Uuid): Promise<never> =>
    Promise.reject(new Error('Roles cannot be deleted while grants may reference them; remove it from the owning manifest instead.')),
  updateRole: (i: { role_id: Uuid; description: string
    allowed_realm_kinds: import('./types').RealmKind[]; permission_keys: string[] }) =>
    live.updateRole(i),
  createGrant: (i: NewGrantInput) => live.createGrant(i),
  memberships: () => live.memberships(),
  createMembership: (i: { name: string; description: string
    entries: { role_id: string; scopes: import('./types').GrantScope[] }[] }) =>
    live.createMembership(i),
  assignMembership: (identityId: Uuid, membershipId: Uuid) =>
    live.assignMembership(identityId, membershipId),
  unassignMembership: (identityId: Uuid, membershipId: Uuid) =>
    live.unassignMembership(identityId, membershipId),
  revokeGrant: (id: Uuid) => live.revokeGrant(id),
  createScopeNode: (i: NewNodeInput) => live.createScopeNode(i),
  createNodeType: (i: { axis_code: string; display_name: string; parent_types: string[] }) =>
    live.createNodeType(i),
  setNodeTypeParents: (_axis: string, _code: string, _parents: string[]): Promise<never> =>
    Promise.reject(new Error('Node-type parents are set when the type is created.')),
  createAxis: (i: NewAxisInput) => live.createAxis(i),
  dashboard: () => live.dashboard(),
  audit: () => live.audit(),
  strictDryRun: (axis: string) => live.strictDryRun(axis),
  syncSources: () => live.syncSources(),
  syncRuns: (sourceId: Uuid) => live.syncRuns(sourceId),
  syncPlan: (sourceId: Uuid) => live.runSync(sourceId, true),
  syncApply: (sourceId: Uuid) => live.runSync(sourceId, false),
  createSyncSource: (i: {
    axis_code: string; kind: import('./types').SyncKind
    target: string; default_node_type: string
    /** Database kinds: the SOURCE system's connection. The scheme picks the
        engine, so postgres://, mysql:// and mariadb:// all work. */
    dsn?: string
    /** db_table: which of its columns mean ref / parent_ref / name / node_type. */
    columns?: Record<string, string>
    /** http: optional Authorization header. */
    auth_header?: string
  }) => live.createSyncSource(i),
}
