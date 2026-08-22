/* The single seam between the console and the backend.
   Everything above this file is transport-agnostic; replacing these bodies with
   fetch() calls to the Go service is the whole migration. */
import * as mock from './mock'
import { useSession } from '@/stores/session'
import type {
  AuthorizeRequest, NewAxisInput, NewGrantInput, NewIdentityInput,
  NewNodeInput, NewPermissionInput, NewRoleInput, Uuid,
} from './types'

/** Simulated latency so loading states are exercised during development
    instead of only appearing in production. */
const delay = <T>(value: T, ms = 120): Promise<T> =>
  new Promise((resolve) => setTimeout(() => resolve(value), ms))

/* Mutations sleep first, THEN run the mock — so validation errors surface the
   way a real HTTP 4xx would (after a round-trip, while the button shows its
   loading state) instead of synchronously on click. */
const mut = async <T>(fn: () => T, ms = 200): Promise<T> => {
  await new Promise((r) => setTimeout(r, ms))
  return fn()
}

/* Workspace guard: list endpoints answer for the SELECTED tenant. The mock
   only carries data for impack, so any other tenant truthfully reads empty —
   which is exactly what a freshly created tenant is. */
const activeTenant = () => useSession.getState().currentTenantId
const scoped = <T>(rows: T[]): T[] => (activeTenant() === 'tnt_impack' ? rows : [])

export const api = {
  tenants: () => delay(mock.tenantsList),
  tenantStats: (id: Uuid) => delay(mock.tenantStats(id)),
  createTenant: (i: { name: string }) => mut(() => mock.createTenant(i)),
  setTenantStatus: (id: Uuid, status: 'active' | 'suspended') =>
    mut(() => mock.setTenantStatus(id, status)),
  signin: (tenantId: Uuid) => delay(mock.getSignin(tenantId)),
  saveSignin: (tenantId: Uuid, cfg: import('./types').SignInConfig) =>
    mut(() => mock.saveSignin(tenantId, cfg)),

  realms: () => delay(scoped(mock.realms)),
  realmCategories: (realmId?: Uuid) =>
    delay(mock.realmCategories
      .filter((c) => !realmId || c.realm_id === realmId)
      .sort((a, b) => a.sort_order - b.sort_order)),
  axes: () => delay(mock.axes.filter((a) => a.status === 'active')
    .sort((a, b) => a.sort_order - b.sort_order)),
  nodeTypes: () => delay(mock.nodeTypes),

  /** Children of a node, or axis roots when parentId is null. Lazy by design:
      a production customer axis holds ~20k nodes and must never be sent whole. */
  scopeChildren: (axisCode: string, parentId: Uuid | null) =>
    delay(mock.nodes.filter((n) =>
      n.axis_code === axisCode && n.status === 'active' &&
      (parentId === null ? n.parent_id === null : n.parent_id === parentId))),

  scopeSearch: (axisCode: string, q: string) =>
    delay(mock.nodes
      .filter((n) => n.axis_code === axisCode && n.status === 'active' &&
        (n.name.toLowerCase().includes(q.toLowerCase()) ||
         n.slug.toLowerCase().includes(q.toLowerCase())))
      .slice(0, 50)),

  scopeNode: (id: Uuid) => delay(mock.nodes.find((n) => n.id === id) ?? null),
  ancestorPath: (id: Uuid) => delay(mock.ancestorPath(id)),

  identities: (realmId?: string, q?: string) =>
    delay(scoped(mock.identities).filter((i) =>
      (!realmId || i.realm_id === realmId) &&
      (!q || i.username.includes(q) || (i.email ?? '').includes(q)))),
  identity: (id: Uuid) => delay(mock.identities.find((i) => i.id === id) ?? null),

  permissions: () => delay(mock.permissions),
  roles: () => delay(mock.roles),
  rolePermissions: (roleId: string) => delay(mock.rolePermissions[roleId] ?? []),

  grants: (identityId?: Uuid) =>
    delay(scoped(mock.grants).filter((g) => !identityId || g.identity_id === identityId)),

  authorize: (req: AuthorizeRequest) => delay(mock.authorize(req), 60),
  applications: () => delay(mock.applications),

  // ---- write operations -------------------------------------------------
  createIdentity: (i: NewIdentityInput) => mut(() => mock.createIdentity(i)),
  createRealmCategory: (i: { realm_id: Uuid; display_name: string }) =>
    mut(() => mock.createRealmCategory(i)),
  setIdentityStatus: (id: Uuid, status: 'active' | 'disabled') =>
    mut(() => mock.setIdentityStatus(id, status)),
  createPermission: (i: NewPermissionInput) => mut(() => mock.createPermission(i)),
  createRole: (i: NewRoleInput) => mut(() => mock.createRole(i)),
  deleteRole: (roleId: Uuid) => mut(() => mock.deleteRole(roleId)),
  updateRole: (i: { role_id: Uuid; description: string
    allowed_realm_kinds: import('./types').RealmKind[]; permission_keys: string[] }) =>
    mut(() => mock.updateRole(i)),
  createGrant: (i: NewGrantInput) => mut(() => mock.createGrant(i)),
  memberships: () => delay(scoped(mock.memberships)),
  createMembership: (i: { name: string; description: string
    entries: { role_id: string; scopes: import('./types').GrantScope[] }[] }) =>
    mut(() => mock.createMembership(i)),
  assignMembership: (identityId: Uuid, membershipId: Uuid) =>
    mut(() => mock.assignMembership(identityId, membershipId)),
  unassignMembership: (identityId: Uuid, membershipId: Uuid) =>
    mut(() => mock.unassignMembership(identityId, membershipId)),
  revokeGrant: (id: Uuid) => mut(() => mock.revokeGrant(id)),
  createScopeNode: (i: NewNodeInput) => mut(() => mock.createScopeNode(i)),
  createNodeType: (i: { axis_code: string; display_name: string; parent_types: string[] }) =>
    mut(() => mock.createNodeType(i)),
  setNodeTypeParents: (axis: string, code: string, parents: string[]) =>
    mut(() => mock.setNodeTypeParents(axis, code, parents)),
  createAxis: (i: NewAxisInput) => mut(() => mock.createAxis(i)),
  dashboard: () => delay(mock.dashboard()),
  audit: () => delay(scoped(mock.audit)),
  strictDryRun: (axis: string) => delay(mock.strictDryRun(axis), 400),
  syncSources: () => delay(mock.syncSources),
  syncRuns: (sourceId: Uuid) =>
    delay(mock.syncRuns.filter((r) => r.source_id === sourceId).slice(0, 3)),
  syncPlan: (sourceId: Uuid) => mut(() => mock.syncPlan(sourceId), 350),
  syncApply: (sourceId: Uuid) => mut(() => mock.syncApply(sourceId), 500),
  createSyncSource: (i: { axis_code: string; kind: import('./types').SyncKind
    target: string; default_node_type: string }) => mut(() => mock.createSyncSource(i)),
}
