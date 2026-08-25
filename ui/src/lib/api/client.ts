/* The single seam between the console and the backend.

   Everything above this file is transport-agnostic. The migration off the
   sample data happens here, one function at a time, and this comment is the
   record of where each one stands.

   LIVE — reaches the Go service (see live.ts): everything except the two
   below. Reads AND writes: the Create drawers commit real rows now.

   NOT live, deliberately:
     dashboard()   LIVE — GetDashboard: counts, decisions, signals
     syncRuns()    run history is not recorded server-side; returns [] —
                   an empty list is honest where fake history is not
   NO RPC EXISTS, and the seam says so instead of pretending:
     createPermission (manifests own the catalog), deleteRole,
     setNodeTypeParents

   SAMPLE DATA — no server equivalent yet:
     grants()  the Access screen asks for every grant at once, which the admin
               API deliberately does not offer: a tenant here holds 150k of
               them. It needs a per-person view or a paginated RPC first.
     everything else below.

   A function moves only once its screen still works afterwards, so the
   console is never half-broken in between. */
import * as mock from './mock'
import * as live from './live'

/* A tenant's slug is derived once, at creation, and never edited: it appears
   in URLs, tokens and every hosted page path. */
function slugify(name: string): string {
  return name.toLowerCase().trim().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '').slice(0, 63)
}
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
  signin: (_tenantId: Uuid) => live.signin(),
  saveSignin: (_tenantId: Uuid, cfg: import('./types').SignInConfig) => live.saveSignin(cfg),

  realms: () => live.realms(),
  realmCategories: (realmId?: Uuid) => live.realmCategories(realmId),
  axes: () => live.axes(),
  nodeTypes: () => live.nodeTypes(),

  /** Children of a node, or axis roots when parentId is null. Lazy by design:
      a production customer axis holds ~20k nodes and must never be sent whole. */
  scopeChildren: (axisCode: string, parentId: Uuid | null) => live.scopeChildren(axisCode, parentId),

  scopeSearch: (axisCode: string, q: string) => live.scopeSearch(axisCode, q),

  scopeNode: (id: Uuid) => live.scopeNode(id),
  ancestorPath: (id: Uuid) => live.ancestorPath(id),

  identities: (realmId?: string, q?: string) => live.identities(realmId, q),
  identity: (id: Uuid) => live.identity(id),

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
  /* Run history is not recorded server-side yet; an empty list is honest
     where fake history for a real source is not. */
  syncRuns: (_sourceId: Uuid) => Promise.resolve([] as import('./types').SyncRun[]),
  syncPlan: (sourceId: Uuid) => live.runSync(sourceId, true),
  syncApply: (sourceId: Uuid) => live.runSync(sourceId, false),
  createSyncSource: (i: { axis_code: string; kind: import('./types').SyncKind
    target: string; default_node_type: string }) => live.createSyncSource(i),
}
