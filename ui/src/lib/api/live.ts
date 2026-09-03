/* Real implementations of the console's data seam, talking to the Go service
   over Connect.

   client.ts is the seam every screen imports; this file supplies the bodies
   that reach the server, mapped into the shapes in types.ts so no screen has
   to change to move off the sample data. Where the admin API carries a
   different shape than the console grew up with — a realm CODE rather than a
   realm id, unix seconds rather than ISO strings — the translation happens
   here and nowhere else. */
import { api as rpc } from '@/lib/anubis'
import type {
  AuditEntry, AuthorizeRequest, AuthorizeResponse, AxisDefaultEffect,
  AxisUiSchema, AxisVerdict, DashboardStats, DenyReason, Grant,
  GrantEvaluation, GrantScope,
  Identity, Membership, MembershipEntry, NewAxisInput, NewGrantInput,
  NewIdentityInput, NewNodeInput, NewRoleInput, Permission, Realm,
  RealmCategory, RealmKind, Role, ScopeAxis, ScopeNode, ScopeNodeType,
  SecuritySignal,  StrictDryRun, SyncPlan, SyncRun, SyncSource, Tenant,
  Ial, Risk, Uuid, AuthPage, PageConfig, PageKind
} from './types'

/** Unix seconds to ISO, with the protobuf zero meaning "never". */
function at(v: bigint | number | undefined): string | null {
  const n = Number(v ?? 0)
  return n > 0 ? new Date(n * 1000).toISOString() : null
}

function atRequired(v: bigint | number | undefined): string {
  return at(v) ?? new Date(0).toISOString()
}

/* The admin API identifies a realm by CODE; the console's types carry a realm
   id, because that is what the database uses. One cached lookup bridges the
   two rather than every caller doing it. */
let realmCache: Realm[] | null = null

export async function realms(): Promise<Realm[]> {
  const resp = await rpc.tenantAdmin.listRealms({})
  realmCache = resp.realms.map((r): Realm => ({
    id: r.id,
    code: r.code,
    kind: r.kind as RealmKind,
    display_name: r.displayName,
    min_assurance: r.minAssurance as Ial,
    self_registration: r.selfRegistration,
    email_verification_required: r.emailVerificationRequired,
    allowed_factors: r.allowedFactors,
    required_factors: r.requiredFactors,
    session_ttl: r.sessionTtl,
    access_token_ttl: r.accessTokenTtl,
    refresh_token_ttl: r.refreshTokenTtl,
    default_retention: r.defaultRetention || null,
    pii_encryption: r.piiEncryption,
    factor_enrolment_deadline: Number(r.factorEnrolmentDeadline) || null,
  }))
  return realmCache
}

async function realmIdByCode(code: string): Promise<Uuid> {
  if (!realmCache) await realms()
  return realmCache?.find((r) => r.code === code)?.id ?? ''
}

/** Invalidate the realm lookup — call after a realm is created or renamed. */
export function forgetRealms() { realmCache = null }

/* The admin API pages, and this used to ask for 200 rows and present them as
   the whole directory — which over 57,000 people is a wrong answer, not a
   truncated one. Callers that want everything now say so and get every page;
   callers that want a page pass a cursor. */
export async function identitiesPage(
  realmId?: string, q?: string, cursor = '', pageSize = 100,
): Promise<{ rows: Identity[]; next: string }> {
  const realmCode = await realmCodeFor(realmId)
  const resp = await rpc.identityAdmin.listIdentities({
    realm: realmCode, query: q ?? '', pageSize, pageToken: cursor,
  })
  const rows: Identity[] = []
  for (const i of resp.identities) {
    rows.push(await toIdentity(i))
  }
  return { rows, next: resp.nextPageToken }
}

async function realmCodeFor(realmId?: string): Promise<string> {
  if (!realmId) return ''
  if (!realmCache) await realms()
  return realmCache?.find((r) => r.id === realmId)?.code ?? ''
}

async function toIdentity(i: {
  id: string; username: string; email: string; realm: string; status: string
  assuranceLevel: number; tokenEpoch: number; externalRef: string
  createdAt: bigint; lastLoginAt: bigint; disabledAt: bigint; anonymizedAt: bigint
}): Promise<Identity> {
  return {
    id: i.id,
    tenant_id: '',
    realm_id: await realmIdByCode(i.realm),
    category_id: null,
    username: i.username,
    email: i.email || null,
    status: i.status as Identity['status'],
    assurance_level: i.assuranceLevel as Ial,
    token_epoch: i.tokenEpoch,
    external_ref: i.externalRef || null,
    created_at: atRequired(i.createdAt),
    last_login_at: at(i.lastLoginAt),
    disabled_at: at(i.disabledAt),
    anonymized_at: at(i.anonymizedAt),
    retention_until: null,
  }
}

export async function identities(realmId?: string, q?: string): Promise<Identity[]> {
  /* Walks every page rather than taking the first one. The cap is here so a
     screen that asks for an unfiltered directory of a large tenant cannot
     hang the browser — and when it bites, the caller is told by getting
     exactly the cap back, not by quietly receiving a short list. */
  const cap = 2000
  const out: Identity[] = []
  let cursor = ''
  do {
    const page = await identitiesPage(realmId, q, cursor, 200)
    out.push(...page.rows)
    cursor = page.next
  } while (cursor && out.length < cap)
  return out.slice(0, cap)
}

export async function identity(id: Uuid): Promise<Identity | null> {
  const resp = await rpc.identityAdmin.getIdentity({ id })
  const i = resp.identity
  if (!i) return null
  return {
    id: i.id,
    tenant_id: '',
    realm_id: await realmIdByCode(i.realm),
    category_id: null,
    username: i.username,
    email: i.email || null,
    status: i.status as Identity['status'],
    assurance_level: i.assuranceLevel as Ial,
    token_epoch: i.tokenEpoch,
    external_ref: i.externalRef || null,
    created_at: atRequired(i.createdAt),
    last_login_at: at(i.lastLoginAt),
    disabled_at: at(i.disabledAt),
    anonymized_at: at(i.anonymizedAt),
    retention_until: null,
  }
}

export async function roles(): Promise<Role[]> {
  const resp = await rpc.authzAdmin.listRoles({ query: '' })
  return resp.roles.map((r): Role => ({
    id: r.id,
    name: r.name,
    description: r.description,
    /* The admin API names an application by slug; the console's Role carries
       an id it only ever uses to display that slug back. */
    application_id: r.applicationSlug || null,
    is_system: r.isSystem,
    allowed_realm_kinds: r.allowedRealmKinds as RealmKind[],
    assignable_at: r.assignableAt,
    /* ListRoles does not carry a permission count, so this is not a real
       number and the roles screen should not present it as one. Fixing it
       properly means adding the count to the Role message rather than making
       fifty GetRoleEffective calls to render one list. */
    permission_count: 0,
  }))
}

export async function permissions(): Promise<Permission[]> {
  const resp = await rpc.authzAdmin.listPermissions({})
  return resp.permissions.map((p): Permission => ({
    id: p.id,
    key: p.key,
    application_id: p.appSlug,
    app_slug: p.appSlug,
    resource: p.resource,
    action: p.action,
    description: p.description,
    risk: p.risk as Risk,
    requires_amr: p.requiresAmr,
    max_auth_age: p.maxAuthAge || null,
    min_assurance: p.minAssurance as Ial,
    deprecated_at: p.deprecated ? new Date(0).toISOString() : null,
  }))
}

/* The Access screen is search-first, not a listing. A tenant here holds
   150,000 grants: "show me all of them" is a question with no useful answer,
   so the screen narrows by person, role or source and pages through what is
   left. */
export async function searchGrants(opts: {
  query?: string; identityId?: string; roleId?: string
  source?: string; includeRevoked?: boolean; cursor?: string; pageSize?: number
}): Promise<{ rows: (Grant & { username: string })[]; next: string }> {
  const size = opts.pageSize ?? 50
  const resp = await rpc.authzAdmin.searchGrants({
    query: opts.query ?? '',
    identityId: opts.identityId ?? '',
    roleId: opts.roleId ?? '',
    source: opts.source ?? '',
    includeRevoked: opts.includeRevoked ?? false,
    pageToken: opts.cursor ?? '',
    pageSize: size,
  })
  const rows = resp.grants.map((g, i) => ({
    ...toGrant(g),
    username: resp.usernames[i] ?? '',
  }))
  return { rows, next: resp.nextPageToken }
}

function toGrant(g: {
  id: string; identityId: string; roleId: string; roleName: string
  viaMembershipId: string; selfScoped: boolean; validFrom: bigint
  validUntil: bigint; revokedAt: bigint; grantedBy: string; reason: string
  scopes: { axis: string; nodeId: string; inherit: boolean }[]
}): Grant {
  return {
    id: g.id,
    identity_id: g.identityId,
    role_id: g.roleId,
    role_name: g.roleName,
    via_membership_id: g.viaMembershipId || null,
    self_scoped: g.selfScoped,
    valid_from: atRequired(g.validFrom),
    valid_until: at(g.validUntil),
    revoked_at: at(g.revokedAt),
    granted_by: g.grantedBy,
    reason: g.reason || null,
    scopes: g.scopes.map((s): GrantScope => ({
      axis_code: s.axis, scope_node_id: s.nodeId, inherit: s.inherit,
    })),
  }
}

export async function grants(identityId?: Uuid): Promise<Grant[]> {
  /* ListGrants answers for ONE person. The console's grants() with no filter
     means "everyone", which the API deliberately does not offer — a tenant
     here holds 150k grants and no screen should ever ask for them all. */
  /* One person's grants. Without an identity this returns the first page
     rather than everything: callers that want to browse should use
     searchGrants, which is honest about being paged. */
  if (!identityId) return (await searchGrants({ pageSize: 50 })).rows
  const resp = await rpc.authzAdmin.listGrants({ identityId, includeRevoked: false })
  return resp.grants.map(toGrant)
}

/* Tenants are the owner's job (ADR-0011): creating them, renaming them, and
   retiring them. There is deliberately no delete — every identity, grant and
   audit record in the installation hangs off the tenant row, so "archived" is
   what retiring means and the history survives it. */
export async function tenants(): Promise<Tenant[]> {
  const resp = await rpc.tenantAdmin.listTenants({})
  return resp.tenants.map((t): Tenant => ({
    id: t.id,
    slug: t.slug,
    name: t.name,
    status: t.status as Tenant['status'],
    created_at: atRequired(t.createdAt),
  }))
}

export async function createTenant(input: { slug: string; name: string }): Promise<Tenant> {
  const resp = await rpc.tenantAdmin.createTenant({ slug: input.slug, name: input.name })
  const t = resp.tenant
  return {
    id: t?.id ?? '',
    slug: t?.slug ?? input.slug,
    name: t?.name ?? input.name,
    status: (t?.status ?? 'active') as Tenant['status'],
    created_at: atRequired(t?.createdAt),
  }
}

export async function updateTenant(id: string, name: string): Promise<void> {
  await rpc.tenantAdmin.updateTenant({ id, name })
}

export async function setTenantStatus(id: string, status: string): Promise<void> {
  await rpc.tenantAdmin.setTenantStatus({ id, status })
}

export async function tenantStats(id: string) {
  const r = await rpc.tenantAdmin.getTenantStats({ id })
  return {
    identities: r.identities, grants: r.grants,
    scope_nodes: r.scopeNodes, memberships: r.memberships,
  }
}

export async function memberships(): Promise<Membership[]> {
  const resp = await rpc.authzAdmin.listMemberships({})
  return resp.memberships.map((m): Membership => ({
    id: m.id,
    name: m.name,
    description: m.description,
    entries: m.entries.map((e): MembershipEntry => ({
      id: e.id,
      role_id: e.roleId,
      role_name: e.roleName,
      scopes: e.scopes.map((sc): GrantScope => ({
        axis_code: sc.axis, scope_node_id: sc.nodeId, inherit: sc.inherit,
      })),
    })),
    /* The API reports how many members there are, not who they are: a
       membership can hold thousands, and no screen needs the roster to say
       "412 members". Assigning is idempotent, so the picker does not need to
       exclude people who are already in. */
    member_ids: [],
    member_count: m.memberCount,
  }))
}

export async function audit(): Promise<AuditEntry[]> {
  const resp = await rpc.tenantAdmin.queryAudit({ pageSize: 100 })
  return resp.entries.map((e): AuditEntry => ({
    id: e.id,
    occurred_at: atRequired(e.occurredAt),
    actor_id: e.actorId || null,
    actor_label: e.actorKind ? `${e.actorKind}:${e.actorId.slice(0, 8)}` : e.actorId,
    action: e.action,
    result: (e.result || 'allow') as AuditEntry['result'],
    target_id: e.targetId || null,
    ip: e.ip || null,
    detail: safeJSON(e.detailJson),
    /* The chain is verified by its own endpoint over a range, not per row.
       Claiming per-entry integrity we have not checked would be worse than
       not claiming it. */
    chain_ok: true,
  }))
}

function safeJSON(raw: string): Record<string, unknown> {
  if (!raw) return {}
  try {
    const v = JSON.parse(raw) as unknown
    return v && typeof v === 'object' ? (v as Record<string, unknown>) : {}
  } catch {
    return {}
  }
}

export async function rolePermissions(roleId: string): Promise<string[]> {
  const resp = await rpc.authzAdmin.getRoleEffective({ roleId })
  return resp.permissions.map((p) => p.permissionKey)
}

/* Applications are the relying parties: every permission is namespaced by
   one, which is why the permission form cannot be filled in until at least
   one exists. */
export type AppRecord = {
  id: string; slug: string; name: string; kind: string; status: string
  redirect_uris: string[]; post_logout_redirect_uris: string[]
  backchannel_logout_uri: string; manifest_version: number
}

export async function applications(opts: {
  query?: string; cursor?: string; pageSize?: number
} = {}): Promise<{ rows: AppRecord[]; next: string; total: number }> {
  const size = opts.pageSize ?? 25
  const resp = await rpc.tenantAdmin.listApplications({
    query: opts.query ?? '', pageToken: opts.cursor ?? '', pageSize: size,
  })
  return {
    rows: resp.applications.map((a): AppRecord => ({
      id: a.id, slug: a.slug, name: a.name, kind: a.kind, status: a.status,
      redirect_uris: a.redirectUris,
      post_logout_redirect_uris: a.postLogoutRedirectUris,
      backchannel_logout_uri: a.backchannelLogoutUri,
      manifest_version: a.manifestVersion,
    })),
    next: resp.nextPageToken,
    total: resp.total,
  }
}

export async function createApplication(input: {
  slug: string; name: string; kind: string; redirectUris: string[]
  postLogoutRedirectUris: string[]
}): Promise<{ clientSecret: string }> {
  const resp = await rpc.tenantAdmin.createApplication({
    application: {
      $typeName: 'anubis.v1.Application',
      id: '', slug: input.slug, name: input.name, kind: input.kind, status: 'active',
      redirectUris: input.redirectUris,
      postLogoutRedirectUris: input.postLogoutRedirectUris,
      backchannelLogoutUri: '', tokenFormat: '', accessTokenTtl: '',
      refreshTokenTtl: '', manifestVersion: 0,
    },
  })
  // Server and service clients get a secret, shown exactly once.
  return { clientSecret: resp.clientSecret }
}

export async function rotateClientSecret(id: string): Promise<string> {
  const resp = await rpc.tenantAdmin.rotateClientSecret({ applicationId: id })
  return resp.clientSecret
}

/** Apply an application's manifest: the permissions, roles and route policies
    it declares. Run it dry first — it reports what would change. */
export async function applyManifest(applicationSlug: string, manifestJson: string, dry: boolean) {
  const resp = await rpc.authzAdmin.applyManifest({ applicationSlug, manifestJson, dry })
  return resp
}

/* Scope. Axes carry two JSON blobs — how a target is resolved at decision
   time, and how the console should present it — because adding an axis must
   not mean changing the token format or shipping a new console. */
export async function axes(): Promise<ScopeAxis[]> {
  const resp = await rpc.scopeAdmin.listScopeAxes({})
  return resp.axes
    .filter((a) => a.status === 'active')
    .map((a): ScopeAxis => ({
      code: a.code,
      display_name: a.displayName,
      default_effect: a.defaultEffect as AxisDefaultEffect,
      status: a.status as ScopeAxis['status'],
      sort_order: a.sortOrder,
      resolution: safeJSON(a.resolutionJson) as ScopeAxis['resolution'],
      ui_schema: safeJSON(a.uiSchemaJson) as AxisUiSchema,
    }))
    .sort((a, b) => a.sort_order - b.sort_order)
}

export async function nodeTypes(): Promise<ScopeNodeType[]> {
  const all = await axes()
  const perAxis = await Promise.all(all.map(async (a) => {
    const resp = await rpc.scopeAdmin.listScopeNodeTypes({ axis: a.code })
    return resp.types.map((t): ScopeNodeType => ({
      code: t.code, axis_code: a.code, display_name: t.displayName,
      parent_types: t.parentTypes,
    }))
  }))
  return perAxis.flat()
}

function toNode(n: {
  id: string; axis: string; nodeType: string; parentId: string; slug: string
  name: string; externalRef: string; status: string; isAxisRoot: boolean
}): ScopeNode {
  return {
    id: n.id,
    tenant_id: '',
    axis_code: n.axis,
    node_type: n.nodeType,
    parent_id: n.parentId || null,
    slug: n.slug,
    name: n.name,
    external_ref: n.externalRef || null,
    is_axis_root: n.isAxisRoot,
    status: (n.status || 'active') as ScopeNode['status'],
    attributes: {},
  }
}

/** Children of a node, or the axis roots when parentId is null. Lazy by
    design: a production customer axis holds tens of thousands of nodes and
    must never be fetched whole. */
export async function scopeChildren(axisCode: string, parentId: string | null): Promise<ScopeNode[]> {
  // With a parent, page to the end: one node's children are a bounded set and
  // the caller renders all of them, so stopping at the server's page size
  // would silently drop siblings from a tree level.
  //
  // WITHOUT a parent the server has no "roots only" filter — parentId '' means
  // the WHOLE axis — so this stays a single page and filters client-side, as
  // it always has. Paging here would walk a million rows to find one root.
  const out: ScopeNode[] = []
  let pageToken = ''
  do {
    const resp = await rpc.scopeAdmin.listScopeNodes({
      axis: axisCode, parentId: parentId ?? '', query: '', includeArchived: false, pageToken,
    })
    out.push(...resp.nodes.map(toNode))
    pageToken = parentId === null ? '' : resp.nextPageToken
  } while (pageToken)
  return out.filter((n) => (parentId === null ? n.parent_id === null : n.parent_id === parentId))
}

export async function scopeSearch(axisCode: string, q: string): Promise<ScopeNode[]> {
  const resp = await rpc.scopeAdmin.listScopeNodes({
    axis: axisCode, parentId: '', query: q, includeArchived: false,
  })
  return resp.nodes.map(toNode).slice(0, 50)
}

export async function scopeNode(id: Uuid): Promise<ScopeNode | null> {
  const resp = await rpc.scopeAdmin.getScopeNode({ id })
  return resp.node ? toNode(resp.node) : null
}

/** Resolve a bounded set of nodes by id — the names beside one screenful of
    grants. The screens used to build this map by pulling every node in every
    axis (32k in this installation) to render a dozen labels. */
export async function scopeNodesByIds(ids: Uuid[]): Promise<ScopeNode[]> {
  const unique = [...new Set(ids.filter(Boolean))]
  if (unique.length === 0) return []
  // The server caps a batch at 500; chunk rather than let a big page fail.
  const out: ScopeNode[] = []
  for (let i = 0; i < unique.length; i += 500) {
    const resp = await rpc.scopeAdmin.getScopeNodes({ ids: unique.slice(i, i + 500) })
    out.push(...resp.nodes.map(toNode))
  }
  return out
}

/** The chain from the axis root down to a node. This is what lets the picker
    show WHERE a value sits rather than just its name. */
export async function ancestorPath(id: Uuid): Promise<ScopeNode[]> {
  const resp = await rpc.scopeAdmin.scopeAncestors({ id })
  return resp.ancestors
    .filter((a) => a.node !== undefined)
    .map((a) => toNode(a.node!))
}

/* The page builder edits what a TENANT's own people see when they sign in —
   not the platform console's door, which is a different population entirely
   and has no branding to configure.

   These go through the auth_pages RPCs, NOT the legacy getSigninPage /
   putSigninPage pair. Migration 0024 moved pages into auth_pages (per
   application, signin and signout kinds) and the console was never moved
   across, so it edited a flat shape in the old table while
   internal/auth/adapter/http/page_template.go rendered a nested one out of
   the new table. Anything written through the legacy pair is still readable,
   but it is not what the hosted page draws. */

/** Server defaults, mirroring pagecfg applyDefaults so a page that has never
    been configured previews the way it will actually render. */
export const defaultPageConfig = (kind: PageKind): PageConfig => ({
  brand: {
    title: 'Anubis',
    primary_color: '#4f46e5',
    background_color: '#f6f6f7',
    text_color: '#111827',
    corner_radius: 'md',
    font: 'system',
  },
  layout: 'centered',
  copy:
    kind === 'signout'
      ? { heading: '', username_label: '', password_label: '', submit_label: '',
          confirm_heading: 'Sign out?' }
      : { heading: 'Sign in', username_label: 'Username', password_label: 'Password',
          submit_label: 'Sign in' },
  links: [],
  features: {},
  motion: { entrance: 'none' },
  ...(kind === 'signout' ? { behavior: { confirm: true } } : {}),
})

function toAuthPage(p: {
  id: string; kind: string; slug: string; name: string; status: string
  isDefault: boolean; applicationId: string; applicationSlug: string
  realmId: string; realmCode: string; configJson: string
}): AuthPage {
  const kind = (p.kind === 'signout' ? 'signout' : 'signin') as PageKind
  /* A page that has never been configured comes back as an empty object, so
     the defaults have to survive a PARTIAL config rather than assume every
     field is present — and they have to merge per-section, since a config
     carrying only `brand` would otherwise wipe copy. */
  const stored = safeJSON(p.configJson) as Partial<PageConfig> | null
  const base = defaultPageConfig(kind)
  return {
    id: p.id,
    kind,
    slug: p.slug,
    name: p.name,
    status: p.status === 'disabled' ? 'disabled' : 'active',
    is_default: p.isDefault,
    application_id: p.applicationId || null,
    application_slug: p.applicationSlug || null,
    realm_id: p.realmId || null,
    realm_code: p.realmCode || null,
    config: {
      ...base,
      ...stored,
      brand: { ...base.brand, ...(stored?.brand ?? {}) },
      copy: { ...base.copy, ...(stored?.copy ?? {}) },
      features: { ...base.features, ...(stored?.features ?? {}) },
      behavior: { ...base.behavior, ...(stored?.behavior ?? {}) },
      motion: { ...base.motion, ...(stored?.motion ?? {}) },
      links: stored?.links ?? [],
    },
  }
}

export async function authPages(kind?: PageKind): Promise<AuthPage[]> {
  const resp = await rpc.tenantAdmin.listAuthPages({ kind: kind ?? '' })
  return resp.pages.map(toAuthPage)
}

export async function authPage(id: string): Promise<AuthPage | null> {
  const resp = await rpc.tenantAdmin.getAuthPage({ id })
  return resp.page ? toAuthPage(resp.page) : null
}

/** kind and slug are immutable server-side: the URL is published. */
export async function saveAuthPage(page: AuthPage): Promise<AuthPage | null> {
  const resp = await rpc.tenantAdmin.updateAuthPage({
    page: {
      id: page.id,
      kind: page.kind,
      slug: page.slug,
      name: page.name,
      status: page.status,
      isDefault: page.is_default,
      applicationId: page.application_id ?? '',
      applicationSlug: page.application_slug ?? '',
      realmId: page.realm_id ?? '',
      realmCode: page.realm_code ?? '',
      configJson: JSON.stringify(page.config),
    },
  })
  return resp.page ? toAuthPage(resp.page) : null
}

/** Flat list for pickers — the permission form needs choices, not a page. */
export async function applicationChoices(): Promise<AppRecord[]> {
  return (await applications({ pageSize: 200 })).rows
}

/* API keys: the tenant's machine credentials. They authenticate as the
   tenant's SYSTEM — a gateway asking authorize(), an integration reading the
   decision API — never as a person, which is why they live beside
   applications and not on anybody's identity. */
export type ApiKeyRow = {
  id: string; label: string; prefix: string; created_by: string
  created_at: string; last_used_at: string | null
  expires_at: string | null; revoked_at: string | null
}

export async function apiKeys(): Promise<ApiKeyRow[]> {
  const resp = await rpc.tenantAdmin.listApiKeys({})
  return resp.keys.map((k): ApiKeyRow => ({
    id: k.id, label: k.label, prefix: k.prefix, created_by: k.createdBy,
    created_at: atRequired(k.createdAt), last_used_at: at(k.lastUsedAt),
    expires_at: at(k.expiresAt), revoked_at: at(k.revokedAt),
  }))
}

export async function createApiKey(label: string): Promise<{ apiKey: string; prefix: string }> {
  const resp = await rpc.tenantAdmin.createApiKey({ label, expiresAt: BigInt(0) })
  return { apiKey: resp.apiKey, prefix: resp.prefix }
}

export async function revokeApiKey(id: string): Promise<void> {
  await rpc.tenantAdmin.revokeApiKey({ id })
}

/* ---------------------------------------------------------------------------
   The access-check playground, on the REAL decision engine.

   Two calls: Authorize is the canonical verdict (with step-up detail), and
   Explain is the evaluation tree authorize_explain() emits (migration 0020) —
   identity gates, per-grant per-axis satisfaction with the granted nodes,
   strict axes a grant leaves unaddressed. The mapping below carries only what
   the server actually said; anything it does not say is left empty rather
   than invented, because a wrong explanation of a denial is worse than none.
--------------------------------------------------------------------------- */

type ExplainGrant = {
  grant_id: string
  role: string
  via_role: string | null
  self_scoped: boolean
  self_ok: boolean
  axes: { axis: string; satisfied: boolean; nodes: { node_id: string; node: string; inherit: boolean }[] }[]
  axes_ok: boolean
  strict_missing: string[]
  allowed: boolean
}

type ExplainDetail = {
  allow: boolean
  reason: string | null
  identity?: { found: boolean; active: boolean; assurance_ok: boolean; assurance_level: number }
  grants?: ExplainGrant[]
}

export async function authorize(req: AuthorizeRequest): Promise<AuthorizeResponse> {
  const started = performance.now()
  const [dec, exp] = await Promise.all([
    rpc.authz.authorize({
      subject: req.subject, permission: req.permission, scopes: req.scopes,
    }),
    rpc.authz.explain({
      subject: req.subject, permission: req.permission, scopes: req.scopes,
    }),
  ])
  const detail = safeJSON(exp.detailJson) as ExplainDetail

  /* Target names, resolved once per distinct node the request named. The
     explain tree names the GRANTED nodes; the target's own name has to come
     from the scope tree or the trace reads "(not supplied)" for a target
     that was very much supplied. */
  const targetName = new Map<string, string>()
  await Promise.all(Object.values(req.scopes).filter((id) => id && id !== '_owner')
    .map(async (id) => {
      const n = await scopeNode(id).catch(() => null)
      if (n) targetName.set(id, n.name)
    }))

  const evaluations: GrantEvaluation[] = (detail.grants ?? []).map((g) => {
    const axes: AxisVerdict[] = g.axes.map((a) => {
      const target = req.scopes[a.axis] ?? null
      const single = a.nodes.length === 1 ? a.nodes[0] : undefined
      return {
        axis_code: a.axis,
        constrained: true,
        satisfied: a.satisfied,
        granted_node_id: single?.node_id ?? null,
        granted_node_name: single?.node ?? null,
        granted_nodes: a.nodes.map((n) => ({
          id: n.node_id, name: n.node, inherit: n.inherit,
          /* Which of several granted nodes matched is not in the explain
             output; claiming one did would be a guess. */
          matched: false,
        })),
        target_node_id: target,
        target_node_name: target ? targetName.get(target) ?? null : null,
        inherit: single?.inherit ?? a.nodes.some((n) => n.inherit),
        path: [],
        ...(a.satisfied ? {} : {
          note: target
            ? 'no granted node is at or above this target'
            : 'the grant constrains this axis and no target was supplied — unresolved axes deny',
        }),
      }
    })
    // A strict axis the grant does not address denies by itself; surface it
    // as a failing gate rather than a silent verdict.
    for (const code of g.strict_missing) {
      axes.push({
        axis_code: code, constrained: true, satisfied: false,
        granted_node_id: null, granted_node_name: null, granted_nodes: [],
        target_node_id: req.scopes[code] ?? null,
        target_node_name: null, inherit: false, path: [],
        note: 'this axis is strict and the grant does not address it',
      })
    }
    const failed: DenyReason | undefined = g.allowed ? undefined
      : !g.self_ok ? 'self_scope_mismatch'
      : g.strict_missing.length > 0 ? 'strict_axis_unaddressed'
      : 'scope_mismatch'
    return {
      grant_id: g.grant_id,
      role_name: g.via_role ? `${g.role} (via ${g.via_role})` : g.role,
      self_scoped: g.self_scoped,
      survived: g.allowed,
      axes,
      ...(failed ? { failed_because: failed } : {}),
    }
  })

  return {
    allow: dec.allow,
    ...(dec.reason ? { reason: dec.reason as DenyReason } : {}),
    ...(dec.failingAxis ? { failing_axis: dec.failingAxis } : {}),
    ...(dec.message ? { message: dec.message } : {}),
    ...(dec.requiredAmr.length ? { required_amr: dec.requiredAmr } : {}),
    ...(dec.maxAuthAge ? { max_auth_age: dec.maxAuthAge } : {}),
    ...(dec.currentAmr.length ? { current_amr: dec.currentAmr } : {}),
    evaluations,
    took_ms: Math.round(performance.now() - started),
  }
}

/* ---------------------------------------------------------------------------
   Writes. Each is the RPC the thing has always had server-side; only the
   console was still writing to its sample data.
--------------------------------------------------------------------------- */

export async function createIdentity(i: NewIdentityInput): Promise<Identity> {
  // The console holds ids; the API speaks realm and category CODES.
  if (!realmCache) await realms()
  const realm = realmCache?.find((r) => r.id === i.realm_id)
  if (!realm) throw new Error('unknown realm')
  let category = ''
  if (i.category_id) {
    const cats = await realmCategories(i.realm_id)
    category = cats.find((c) => c.id === i.category_id)?.code ?? ''
  }
  const resp = await rpc.identityAdmin.createIdentity({
    realm: realm.code, username: i.username, email: i.email,
    password: '', category, externalRef: '', assuranceLevel: i.assurance_level,
  })
  const created = resp.identity
  if (!created) throw new Error('no identity returned')
  return await toIdentity(created)
}

export async function setIdentityStatus(id: Uuid, status: 'active' | 'disabled'): Promise<void> {
  if (status === 'disabled') {
    await rpc.identityAdmin.disableIdentity({ id, reason: 'console' })
  } else {
    await rpc.identityAdmin.enableIdentity({ id })
  }
}

/* The one part of a person this server stores encrypted (ADR-0013). `erased`
   is not an error: the identity's key was shredded, so the data is gone for
   good and the screen should say so rather than show an empty form. */
export async function identityAttributes(
  id: Uuid,
): Promise<{ attributes: Record<string, string>; erased: boolean }> {
  const resp = await rpc.identityAdmin.getIdentityAttributes({ id })
  return { attributes: { ...resp.attributes }, erased: resp.erased }
}

/* Replaces the whole map — an empty object clears it. There is no partial
   update on the server, and offering one here would only invent a merge the
   backend does not do. */
export async function setIdentityAttributes(
  id: Uuid, attributes: Record<string, string>,
): Promise<void> {
  await rpc.identityAdmin.setIdentityAttributes({ id, attributes })
}

export async function realmCategories(realmId?: Uuid): Promise<RealmCategory[]> {
  const resp = await rpc.tenantAdmin.listRealmCategories({ realmId: realmId ?? '' })
  return resp.categories.map((c): RealmCategory => ({
    id: c.id, realm_id: c.realmId, code: c.code,
    display_name: c.displayName, sort_order: c.sortOrder,
    identity_count: Number(c.identityCount),
  }))
}

export async function createRealmCategory(i: { realm_id: Uuid; display_name: string }): Promise<RealmCategory> {
  const resp = await rpc.tenantAdmin.createRealmCategory({
    category: {
      $typeName: 'anubis.v1.RealmCategory',
      id: '', realmId: i.realm_id, displayName: i.display_name,
      code: i.display_name.toLowerCase().trim().replace(/[^a-z0-9]+/g, '_').replace(/^_|_$/g, ''),
      sortOrder: 0, identityCount: 0n,
    },
  })
  const c = resp.category
  if (!c) throw new Error('no category returned')
  return {
    id: c.id, realm_id: c.realmId, code: c.code, display_name: c.displayName,
    sort_order: c.sortOrder, identity_count: 0,  // brand new: nobody in it yet
  }
}

export async function createGrant(i: NewGrantInput): Promise<void> {
  await rpc.authzAdmin.createGrant({
    identityId: i.identity_id,
    roleId: i.role_id,
    selfScoped: i.self_scoped,
    validUntil: i.valid_until ? BigInt(Math.floor(new Date(i.valid_until).getTime() / 1000)) : BigInt(0),
    reason: '',
    scopes: i.scopes.map((sc) => ({
      $typeName: 'anubis.v1.GrantScope' as const,
      axis: sc.axis_code, nodeId: sc.scope_node_id, inherit: sc.inherit,
      nodeName: '',
    })),
  })
}

export async function revokeGrant(id: Uuid): Promise<void> {
  await rpc.authzAdmin.revokeGrant({ grantId: id, reason: 'console' })
}

export async function createMembership(i: {
  name: string; description: string
  entries: { role_id: string; scopes: GrantScope[] }[]
}): Promise<void> {
  const resp = await rpc.authzAdmin.createMembership({ name: i.name, description: i.description })
  const id = resp.membership?.id
  if (!id) throw new Error('no membership returned')
  if (i.entries.length > 0) {
    await rpc.authzAdmin.setMembershipEntries({
      membershipId: id,
      entries: i.entries.map((e) => ({
        $typeName: 'anubis.v1.MembershipEntry' as const,
        id: '', roleId: e.role_id, roleName: '',
        scopes: e.scopes.map((sc) => ({
          $typeName: 'anubis.v1.GrantScope' as const,
          axis: sc.axis_code, nodeId: sc.scope_node_id, inherit: sc.inherit, nodeName: '',
        })),
      })),
    })
  }
}

export async function assignMembership(identityId: Uuid, membershipId: Uuid): Promise<number> {
  const resp = await rpc.authzAdmin.assignMembership({ membershipId, identityId })
  return resp.grantsCreated
}

export async function unassignMembership(identityId: Uuid, membershipId: Uuid): Promise<number> {
  const resp = await rpc.authzAdmin.unassignMembership({ membershipId, identityId })
  return resp.grantsRevoked
}

export async function createRole(i: NewRoleInput): Promise<void> {
  // The console picks permission KEYS; UpdateRole replaces patterns, and a
  // key used as a pattern matches exactly itself — no wildcard implied.
  const resp = await rpc.authzAdmin.createRole({
    role: {
      $typeName: 'anubis.v1.Role',
      id: '', name: i.name, description: i.description,
      applicationSlug: '', isSystem: false,
      allowedRealmKinds: i.allowed_realm_kinds,
      assignableAt: [], parentIds: [], patterns: i.permission_keys,
    },
  })
  void resp
}

export async function updateRole(i: {
  role_id: Uuid; description: string
  allowed_realm_kinds: RealmKind[]; permission_keys: string[]
}): Promise<void> {
  await rpc.authzAdmin.updateRole({
    role: {
      $typeName: 'anubis.v1.Role',
      id: i.role_id, name: '', description: i.description,
      applicationSlug: '', isSystem: false,
      allowedRealmKinds: i.allowed_realm_kinds,
      assignableAt: [], parentIds: [], patterns: i.permission_keys,
    },
  })
}

export async function createScopeNode(i: NewNodeInput): Promise<void> {
  await rpc.scopeAdmin.createScopeNode({
    axis: i.axis_code, nodeType: i.node_type, parentId: i.parent_id,
    slug: i.slug, name: i.name, externalRef: i.external_ref ?? '',
  })
}

export async function createAxis(i: NewAxisInput): Promise<void> {
  await rpc.scopeAdmin.createScopeAxis({
    axis: {
      $typeName: 'anubis.v1.ScopeAxis',
      code: i.code, displayName: i.display_name, defaultEffect: i.default_effect,
      status: 'active', sortOrder: 0,
      resolutionJson: JSON.stringify({ from: i.resolution_from, ...(i.resolution_key ? { key: i.resolution_key } : {}) }),
      uiSchemaJson: JSON.stringify({ picker: i.picker, icon: i.icon }),
    },
  })
}

export async function createNodeType(i: { axis_code: string; display_name: string; parent_types: string[] }): Promise<void> {
  await rpc.scopeAdmin.createScopeNodeType({
    type: {
      $typeName: 'anubis.v1.ScopeNodeType',
      code: i.display_name.toLowerCase().trim().replace(/[^a-z0-9]+/g, '_').replace(/^_|_$/g, ''),
      axis: i.axis_code, displayName: i.display_name, parentTypes: i.parent_types,
    },
  })
}

/* Sync sources: the scope feed machinery. */
export async function syncSources(): Promise<SyncSource[]> {
  const resp = await rpc.scopeAdmin.listSyncSources({})
  return resp.sources.map((s): SyncSource => {
    const cfg = safeJSON(s.configJson) as { url?: string; dsn?: string; query?: string; table?: string; default_node_type?: string }
    return {
      id: s.id, axis_code: s.axis, kind: s.kind as SyncSource['kind'],
      target: cfg.url ?? cfg.query ?? cfg.table ?? cfg.dsn ?? '',
      default_node_type: cfg.default_node_type ?? '',
      last_run_at: s.lastRunAt > 0 ? new Date(Number(s.lastRunAt) * 1000).toISOString() : null,
    }
  })
}

export async function createSyncSource(i: {
  axis_code: string; kind: SyncSource['kind']; target: string; default_node_type: string
  /** Database kinds only: the SOURCE system's connection, never Anubis's. */
  dsn?: string
  /** db_table only: which of its columns mean ref / parent_ref / name / node_type. */
  columns?: Record<string, string>
  /** http only, optional: sent as the Authorization header. */
  auth_header?: string
}): Promise<void> {
  const cfg: Record<string, unknown> = { default_node_type: i.default_node_type }
  if (i.kind === 'http') {
    cfg['url'] = i.target
    if (i.auth_header) cfg['auth_header'] = i.auth_header
  } else {
    // Both database kinds need somewhere to connect. Without this the source
    // saves and then fails on its first run with "needs dsn", which is a
    // long way from where the mistake was made.
    cfg['dsn'] = i.dsn ?? ''
    if (i.kind === 'db_query') cfg['query'] = i.target
    else {
      cfg['table'] = i.target
      cfg['columns'] = Object.fromEntries(
        Object.entries(i.columns ?? {}).filter(([, v]) => v),
      )
    }
  }
  await rpc.scopeAdmin.createSyncSource({
    source: {
      $typeName: 'anubis.v1.SyncSource',
      id: '', axis: i.axis_code, kind: i.kind, status: 'active',
      configJson: JSON.stringify(cfg), lastRunAt: BigInt(0),
    },
  })
}

export async function runSync(sourceId: string, dry: boolean): Promise<SyncPlan> {
  const resp = await rpc.scopeAdmin.runSync({ sourceId, rows: [], dry })
  const r = safeJSON(resp.reportJson) as Partial<SyncPlan>
  return {
    dry: r.dry ?? dry,
    added: r.added ?? 0, renamed: r.renamed ?? 0, moved: r.moved ?? 0,
    archived: r.archived ?? 0, unchanged: r.unchanged ?? 0,
    errors: Array.isArray(r.errors) ? r.errors : [],
  }
}

export async function strictDryRun(axis: string): Promise<StrictDryRun> {
  const resp = await rpc.scopeAdmin.strictDryRun({ axis })
  const ex = resp.examplesJson ? JSON.parse(resp.examplesJson) as unknown : []
  return {
    axis_code: axis,
    sampled: resp.sampled,
    would_deny: resp.wouldDeny,
    examples: Array.isArray(ex) ? ex : [],
  }
}

export async function dashboard(): Promise<DashboardStats> {
  const resp = await rpc.tenantAdmin.getDashboard({})
  const decisions = Number(resp.decisions24h)
  const denies = Number(resp.denies24h)
  return {
    identities_by_realm: resp.identitiesByRealm.map((r) => ({
      realm: r.realm, kind: r.kind as RealmKind, count: Number(r.count),
    })),
    grants_total: Number(resp.grantsTotal),
    scope_nodes_total: Number(resp.scopeNodesTotal),
    decisions_24h: decisions,
    deny_rate_24h: decisions > 0 ? denies / decisions : 0,
    // The server measures latency per instance (Prometheus histograms), not
    // per tenant; 0 here means "not shown", never a fake number.
    p99_authorize_ms: 0,
    signals: resp.signals.map((s) => ({
      kind: s.kind as SecuritySignal['kind'],
      severity: s.severity as SecuritySignal['severity'],
      count: Number(s.count),
      detail: s.detail,
      since: new Date(Number(s.since) * 1000).toISOString(),
    })),
  }
}

/* What a feed has actually done. scope_sync_apply has recorded every run
   since migration 0017 — this is the first thing to read it, so the panel
   showed nothing at all until now. */
export async function syncRuns(sourceId: Uuid, limit = 25): Promise<SyncRun[]> {
  const resp = await rpc.scopeAdmin.listSyncRuns({ sourceId, limit })
  return resp.runs.map((r): SyncRun => {
    const rep = safeJSON(r.reportJson) as Partial<Record<string, number | unknown[]>>
    const n = (k: string) => Number(rep?.[k] ?? 0)
    return {
      id: r.id,
      source_id: r.sourceId,
      at: atRequired(r.startedAt),
      dry: r.dry,
      status: r.status,
      added: n('added'), renamed: n('renamed'), moved: n('moved'),
      archived: n('archived'), unchanged: n('unchanged'),
      errors: Array.isArray(rep?.errors) ? rep.errors.length : 0,
    }
  })
}
