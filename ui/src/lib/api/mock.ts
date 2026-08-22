/* ---------------------------------------------------------------------------
   In-memory backend.

   The Go service does not exist yet (see docs/roadmap.md), so the console runs
   against this. It is deliberately NOT a stub returning canned responses: the
   authorize() implementation below mirrors migrations/0009 rule for rule,
   including every fail-closed branch. That way the UI is developed against real
   semantics, and the screens that explain a denial have something true to
   explain. Swapping in the HTTP client should change nothing above this layer.
   --------------------------------------------------------------------------- */
import type {
  AuditEntry, Membership, SignInConfig, SyncPlan, SyncRun, SyncSource, Tenant, TenantStats, AuthorizeRequest, AuthorizeResponse, AxisVerdict, DashboardStats,
  Grant, GrantEvaluation, Identity, Permission, Realm, RealmCategory, Role,
  ScopeAxis, ScopeNode, ScopeNodeType, StrictDryRun, Uuid,
} from './types'

let seq = 0
const uid = (p: string) => `${p}_${(++seq).toString(36).padStart(6, '0')}`
const TENANT = 'tnt_impack'

/* ---------- realms ---------- */
export const realms: Realm[] = [
  { id: 'rlm_internal', code: 'internal', kind: 'internal', display_name: 'Internal',
    min_assurance: 3, self_registration: false, email_verification_required: true,
    allowed_factors: ['password', 'totp', 'device_key'], required_factors: ['password', 'totp'],
    session_ttl: '12h', access_token_ttl: '10m', refresh_token_ttl: '30d',
    default_retention: null, pii_encryption: false },
  { id: 'rlm_partner', code: 'partner', kind: 'partner', display_name: 'Partners',
    min_assurance: 2, self_registration: false, email_verification_required: true,
    allowed_factors: ['password', 'totp'], required_factors: ['password', 'totp'],
    session_ttl: '8h', access_token_ttl: '10m', refresh_token_ttl: '14d',
    default_retention: null, pii_encryption: true },
  { id: 'rlm_public', code: 'public', kind: 'public', display_name: 'Public',
    min_assurance: 1, self_registration: true, email_verification_required: true,
    allowed_factors: ['password', 'email_otp'], required_factors: ['password'],
    session_ttl: '1h', access_token_ttl: '10m', refresh_token_ttl: '7d',
    default_retention: '2 years', pii_encryption: true },
]

/* ---------- realm categories ----------
   Directory classification inside a realm — supplier vs contractor, applicant
   vs customer. Rows, not code: a tenant adds more at runtime, which is the
   whole point ("public can be anything"). */
export const realmCategories: RealmCategory[] = [
  { id: 'cat_employee',   realm_id: 'rlm_internal', code: 'employee',   display_name: 'Employee',   sort_order: 10 },
  { id: 'cat_supplier',   realm_id: 'rlm_partner', code: 'supplier',   display_name: 'Supplier',   sort_order: 10 },
  { id: 'cat_contractor', realm_id: 'rlm_partner', code: 'contractor', display_name: 'Contractor', sort_order: 20 },
  { id: 'cat_applicant',  realm_id: 'rlm_public',  code: 'applicant',  display_name: 'Applicant',  sort_order: 10 },
  { id: 'cat_customer',   realm_id: 'rlm_public',  code: 'customer',   display_name: 'Customer',   sort_order: 20 },
]

/* ---------- axes ----------
   Four axes today. The console never enumerates these in code -- it reads them
   from here (in production, from GET /v1/admin/scope-axes) and renders whatever
   it finds. Adding a fifth requires no frontend change. */
export const axes: ScopeAxis[] = [
  { code: 'org', display_name: 'Organisation', default_effect: 'unconstrained',
    status: 'active', sort_order: 10, resolution: { from: 'token' },
    ui_schema: { picker: 'tree', icon: 'building', searchable: true,
      help: 'Office, department or team the action is performed in.' } },
  { code: 'partner', display_name: 'Partner Organisation', default_effect: 'unconstrained',
    status: 'active', sort_order: 15, resolution: { from: 'token' },
    ui_schema: { picker: 'tree', icon: 'truck', searchable: true,
      help: 'Supplier or contractor company.' } },
  { code: 'product', display_name: 'Product Line', default_effect: 'unconstrained',
    status: 'active', sort_order: 20, resolution: { from: 'context', key: 'product_id' },
    ui_schema: { picker: 'tree', icon: 'box', searchable: true,
      help: 'Supplied by the calling application per request.' } },
  { code: 'customer', display_name: 'Customer', default_effect: 'unconstrained',
    status: 'active', sort_order: 30, resolution: { from: 'context', key: 'customer_id' },
    ui_schema: { picker: 'search', icon: 'users', searchable: true,
      help: 'Account or segment the record belongs to.' } },
]

export const nodeTypes: ScopeNodeType[] = [
  { code: 'org_root', axis_code: 'org', display_name: 'All Organisations', parent_types: [] },
  { code: 'org', axis_code: 'org', display_name: 'Organisation', parent_types: ['org_root'] },
  { code: 'office', axis_code: 'org', display_name: 'Work Office', parent_types: ['org'] },
  { code: 'division', axis_code: 'org', display_name: 'Division', parent_types: ['office'] },
  { code: 'department', axis_code: 'org', display_name: 'Department', parent_types: ['office', 'division'] },
  { code: 'team', axis_code: 'org', display_name: 'Team', parent_types: ['department'] },
  { code: 'partner_root', axis_code: 'partner', display_name: 'All Partners', parent_types: [] },
  { code: 'partner_org', axis_code: 'partner', display_name: 'Partner Company', parent_types: ['partner_root'] },
  { code: 'catalog', axis_code: 'product', display_name: 'Catalog', parent_types: [] },
  { code: 'product_line', axis_code: 'product', display_name: 'Product Line', parent_types: ['catalog'] },
  { code: 'product_family', axis_code: 'product', display_name: 'Family', parent_types: ['product_line'] },
  { code: 'sku', axis_code: 'product', display_name: 'SKU', parent_types: ['product_family'] },
  { code: 'accounts', axis_code: 'customer', display_name: 'Accounts', parent_types: [] },
  { code: 'segment', axis_code: 'customer', display_name: 'Segment', parent_types: ['accounts'] },
  { code: 'industry', axis_code: 'customer', display_name: 'Industry', parent_types: ['segment'] },
  { code: 'account', axis_code: 'customer', display_name: 'Account', parent_types: ['industry'] },
]

/* ---------- scope forest ---------- */
export const nodes: ScopeNode[] = []
/** ancestor -> descendants, mirroring the scope_closure table. */
const closure = new Map<Uuid, Set<Uuid>>()

function addNode(
  axis: string, type: string, parent: Uuid | null, slug: string, name: string,
): ScopeNode {
  const n: ScopeNode = {
    id: uid('scp'), tenant_id: TENANT, axis_code: axis, node_type: type,
    parent_id: parent, slug, name, external_ref: null,
    is_axis_root: parent === null, status: 'active', attributes: {}, child_count: 0,
  }
  nodes.push(n)
  // self edge, then inherit every ancestor's edge -- exactly scope_add_node()
  const anc = new Set<Uuid>([n.id])
  if (parent) {
    for (const [a, ds] of closure) if (ds.has(parent)) anc.add(a)
    const p = nodes.find((x) => x.id === parent)
    if (p) p.child_count = (p.child_count ?? 0) + 1
  }
  for (const a of anc) {
    if (!closure.has(a)) closure.set(a, new Set())
    closure.get(a)!.add(n.id)
  }
  return n
}

function buildForest() {
  // One tenant, several legal entities — the group-holding shape. The root is
  // a neutral anchor: granting THERE means group-wide; granting at one
  // organisation means that company only.
  const anchor = addNode('org', 'org_root', null, 'all-orgs', 'All Organisations')
  const orgRoot = addNode('org', 'org', anchor.id, 'impack', 'PT Impack Pratama')
  const mulford = addNode('org', 'org', anchor.id, 'mulford', 'PT Mulford Indonesia')
  const cikarang = addNode('org', 'office', mulford.id, 'cikarang', 'Cikarang Office')
  addNode('org', 'department', cikarang.id, 'sales', 'Sales')
  const offices = ['Jakarta', 'Surabaya', 'Medan', 'Bandung', 'Semarang']
  const depts = ['Finance', 'HR', 'Procurement', 'Sales', 'Operations']
  for (const o of offices) {
    const off = addNode('org', 'office', orgRoot.id, o.toLowerCase(), `${o} Office`)
    for (const d of depts) {
      const dep = addNode('org', 'department', off.id, d.toLowerCase(), d)
      for (let t = 1; t <= 3; t++) addNode('org', 'team', dep.id, `team-${t}`, `${d} Team ${t}`)
    }
  }
  // Deep-chain demo: Jakarta Office → Manufacturing Division → departments →
  // teams. Five levels — grants at division level see every department below.
  const jkt = nodes.find((n) => n.name === 'Jakarta Office')!
  const mfg = addNode('org', 'division', jkt.id, 'manufacturing', 'Manufacturing Division')
  const prodD = addNode('org', 'department', mfg.id, 'production', 'Production')
  const qaD = addNode('org', 'department', mfg.id, 'quality', 'Quality Assurance')
  addNode('org', 'team', prodD.id, 'line-1', 'Production Line 1')
  addNode('org', 'team', qaD.id, 'qa-lab', 'QA Lab')

  const pRoot = addNode('partner', 'partner_root', null, 'partners', 'All Partners')
  const sups = ['Acme Supplies', 'Globex Materials', 'Initech Logistics',
    'Umbrella Chemicals', 'Soylent Foods', 'Stark Components']
  sups.forEach((name, i) => {
    const n = addNode('partner', 'partner_org', pRoot.id, name.toLowerCase().replace(/\s+/g, '-'), name)
    n.external_ref = `ERP-SUP-${i + 1}`   // sync idempotency key
  })
  const cat = addNode('product', 'catalog', null, 'catalog', 'Catalog')
  for (const l of ['Rigid Packaging', 'Flexible Films', 'Industrial Sheets']) {
    const line = addNode('product', 'product_line', cat.id, l.toLowerCase().replace(/\s+/g, '-'), l)
    for (let f = 1; f <= 3; f++) {
      const fam = addNode('product', 'product_family', line.id, `fam-${f}`, `${l} Family ${f}`)
      for (let k = 1; k <= 4; k++) addNode('product', 'sku', fam.id, `sku-${f}${k}`, `SKU ${f}${k}`)
    }
  }
  const acc = addNode('customer', 'accounts', null, 'accounts', 'All Accounts')
  for (const seg of ['Enterprise', 'Mid-Market', 'SMB']) {
    const s = addNode('customer', 'segment', acc.id, seg.toLowerCase(), seg)
    for (const ind of ['Manufacturing', 'Retail', 'Food & Beverage']) {
      const i = addNode('customer', 'industry', s.id, ind.toLowerCase().replace(/\W+/g, '-'), ind)
      for (let a = 1; a <= 6; a++) addNode('customer', 'account', i.id, `acct-${a}`, `${ind} Account ${a}`)
    }
  }
}
buildForest()

export const isAncestorOrSelf = (ancestor: Uuid, target: Uuid) =>
  closure.get(ancestor)?.has(target) ?? false

export function ancestorPath(target: Uuid): { id: Uuid; name: string; depth: number }[] {
  const chain: ScopeNode[] = []
  let cur = nodes.find((n) => n.id === target)
  while (cur) {
    chain.unshift(cur)
    cur = cur.parent_id ? nodes.find((n) => n.id === cur!.parent_id) : undefined
  }
  return chain.map((n, i) => ({ id: n.id, name: n.name, depth: chain.length - 1 - i }))
}

/* ---------- permissions & roles ---------- */
const P = (app: string, res: string, act: string, risk: Permission['risk'],
  ial: Permission['min_assurance'], amr: string[] = [], age: string | null = null): Permission => ({
  id: uid('prm'), key: `${app}:${res}:${act}`, application_id: `app_${app}`, app_slug: app,
  resource: res, action: act, description: '', risk, requires_amr: amr,
  max_auth_age: age, min_assurance: ial, deprecated_at: null,
})

export const permissions: Permission[] = [
  P('billing', 'invoice', 'read', 'normal', 1),
  P('billing', 'invoice', 'create', 'normal', 2),
  P('billing', 'invoice', 'approve', 'sensitive', 3, ['otp'], '5m'),
  P('billing', 'invoice', 'void', 'critical', 3, ['otp'], '2m'),
  P('billing', 'payment', 'approve', 'critical', 3, ['otp'], '2m'),
  P('procure', 'purchase_order', 'read', 'normal', 2),
  P('procure', 'purchase_order', 'submit', 'normal', 2),
  P('hr', 'employee', 'read', 'normal', 2),
  P('hr', 'employee', 'update', 'sensitive', 3, ['otp'], '10m'),
  P('ats', 'application', 'read_own', 'normal', 1),
  P('ats', 'application', 'withdraw', 'normal', 1),
  P('ats', 'candidate', 'review', 'normal', 3),
]

export const roles: Role[] = [
  { id: 'rol_fin_view', name: 'finance.viewer', description: 'Read invoices',
    application_id: null, is_system: false, allowed_realm_kinds: ['internal'],
    assignable_at: ['office', 'department'], permission_count: 1 },
  { id: 'rol_fin_appr', name: 'finance.approver', description: 'Approve invoices and payments',
    application_id: null, is_system: true, allowed_realm_kinds: ['internal'],
    assignable_at: ['office', 'department'], permission_count: 4 },
  { id: 'rol_hr', name: 'hr.officer', description: 'Manage employee records',
    application_id: null, is_system: true, allowed_realm_kinds: ['internal'],
    assignable_at: ['office'], permission_count: 2 },
  { id: 'rol_partner', name: 'partner.portal', description: 'Supplier portal access',
    application_id: null, is_system: false, allowed_realm_kinds: ['partner'],
    assignable_at: ['partner_org'], permission_count: 2 },
  { id: 'rol_applicant', name: 'public.applicant', description: 'Job applicant self-service',
    application_id: null, is_system: true, allowed_realm_kinds: ['public'],
    assignable_at: [], permission_count: 2 },
  { id: 'rol_recruiter', name: 'hr.recruiter', description: 'Review candidates',
    application_id: null, is_system: false, allowed_realm_kinds: ['internal'],
    assignable_at: ['office', 'department'], permission_count: 1 },
]

const rolePerms: Record<string, string[]> = {
  rol_fin_view: ['billing:invoice:read'],
  rol_fin_appr: ['billing:invoice:read', 'billing:invoice:create',
    'billing:invoice:approve', 'billing:payment:approve'],
  rol_hr: ['hr:employee:read', 'hr:employee:update'],
  rol_partner: ['procure:purchase_order:read', 'procure:purchase_order:submit'],
  rol_applicant: ['ats:application:read_own', 'ats:application:withdraw'],
  rol_recruiter: ['ats:candidate:review'],
}
export const rolePermissions = rolePerms

/* ---------- identities & grants ---------- */
export const identities: Identity[] = []
export const grants: Grant[] = []

const nodeBy = (axis: string, name: string) =>
  nodes.find((n) => n.axis_code === axis && n.name === name)!

function mkIdentity(realm: Realm, username: string, email: string, ial: Identity['assurance_level'],
  status: Identity['status'] = 'active', category: string | null = null): Identity {
  const i: Identity = {
    id: uid('usr'), tenant_id: TENANT, realm_id: realm.id, username, email, status,
    category_id: category,
    assurance_level: ial, token_epoch: 1, external_ref: null,
    created_at: '2026-01-14T09:00:00Z', last_login_at: '2026-08-21T22:14:00Z',
    disabled_at: status === 'disabled' ? '2026-08-01T00:00:00Z' : null,
    retention_until: realm.default_retention ? '2028-08-22T00:00:00Z' : null,
    anonymized_at: null,
  }
  identities.push(i)
  return i
}

function mkGrant(id: Identity, roleId: string, scopes: Grant['scopes'], selfScoped = false): Grant {
  const g: Grant = {
    id: uid('grt'), identity_id: id.id, role_id: roleId,
    role_name: roles.find((r) => r.id === roleId)!.name,
    via_membership_id: null,
    self_scoped: selfScoped, valid_from: '2026-01-14T09:00:00Z', valid_until: null,
    revoked_at: null, granted_by: 'usr_000001', reason: null, scopes,
  }
  grants.push(g)
  return g
}

const [emp, part, pub] = realms as [Realm, Realm, Realm]

const alice = mkIdentity(emp, 'alice', 'alice@impack.co.id', 3, 'active', 'cat_employee')
const budi = mkIdentity(emp, 'budi', 'budi@impack.co.id', 3, 'active', 'cat_employee')
const citra = mkIdentity(emp, 'citra', 'citra@impack.co.id', 3, 'active', 'cat_employee')
const dedi = mkIdentity(emp, 'dedi', 'dedi@impack.co.id', 3, 'disabled')
// Same username in three realms -- uniqueness is per (tenant, realm).
const aliceSupplier = mkIdentity(part, 'alice', 'alice@acme-supplies.com', 2, 'active', 'cat_supplier')
const eka = mkIdentity(part, 'eka', 'eka@globex.com', 2, 'active', 'cat_contractor')
const aliceApplicant = mkIdentity(pub, 'alice', 'alice.p@gmail.com', 1, 'active', 'cat_applicant')
const fajar = mkIdentity(pub, 'fajar', 'fajar@gmail.com', 1, 'active', 'cat_customer')

mkGrant(alice, 'rol_fin_appr', [
  { axis_code: 'org', scope_node_id: nodeBy('org', 'Finance').id, inherit: true },
  { axis_code: 'product', scope_node_id: nodeBy('product', 'Rigid Packaging').id, inherit: true },
])
mkGrant(budi, 'rol_fin_view', [
  // Assigned to BOTH offices: any-of semantics on one axis.
  { axis_code: 'org', scope_node_id: nodeBy('org', 'Jakarta Office').id, inherit: true },
  { axis_code: 'org', scope_node_id: nodeBy('org', 'Surabaya Office').id, inherit: true },
])
mkGrant(citra, 'rol_hr', [
  { axis_code: 'org', scope_node_id: nodeBy('org', 'PT Impack Pratama').id, inherit: true },
])
mkGrant(citra, 'rol_recruiter', [
  { axis_code: 'org', scope_node_id: nodeBy('org', 'Jakarta Office').id, inherit: true },
])
mkGrant(dedi, 'rol_fin_appr', [
  { axis_code: 'org', scope_node_id: nodeBy('org', 'Surabaya Office').id, inherit: true },
])
mkGrant(aliceSupplier, 'rol_partner', [
  { axis_code: 'partner', scope_node_id: nodeBy('partner', 'Acme Supplies').id, inherit: true },
])
mkGrant(eka, 'rol_partner', [
  { axis_code: 'partner', scope_node_id: nodeBy('partner', 'Globex Materials').id, inherit: true },
])
mkGrant(aliceApplicant, 'rol_applicant', [], true)
mkGrant(fajar, 'rol_applicant', [], true)

/* ---------------------------------------------------------------------------
   authorize() -- mirrors migrations/0009_authorize_realms.sql

   Every branch below corresponds to a clause in the SQL. Kept deliberately
   literal rather than idiomatic so the two can be compared side by side.
   --------------------------------------------------------------------------- */
export function authorize(req: AuthorizeRequest): AuthorizeResponse {
  const t0 = performance.now()
  const identity = identities.find((i) => i.id === req.subject)
  const perm = permissions.find((p) => p.key === req.permission)
  const evaluations: GrantEvaluation[] = []

  const done = (allow: boolean, reason?: AuthorizeResponse['reason'],
    extra: Partial<AuthorizeResponse> = {}): AuthorizeResponse => ({
    allow, ...(reason ? { reason } : {}), evaluations,
    took_ms: Math.round((performance.now() - t0) * 1000) / 1000, ...extra,
  })

  if (!identity) return done(false, 'no_grant', { message: 'Unknown subject.' })
  if (!perm) return done(false, 'no_grant', { message: `Unknown permission "${req.permission}".` })

  // gate 1: identity state -- deprovisioning must not depend on revoking grants
  if (identity.status !== 'active' || identity.disabled_at || identity.anonymized_at) {
    return done(false, 'identity_inactive', {
      message: `Identity is ${identity.anonymized_at ? 'anonymised' : identity.status}. ` +
        'Denied regardless of grants.',
    })
  }

  // gate 2: assurance -- defence against grant misadministration
  if (perm.min_assurance > identity.assurance_level) {
    return done(false, 'assurance_too_low', {
      message: `Permission requires IAL${perm.min_assurance}; identity is IAL${identity.assurance_level}.`,
    })
  }

  const mine = grants.filter(
    (g) => g.identity_id === identity.id && !g.revoked_at &&
      (rolePerms[g.role_id] ?? []).includes(perm.key),
  )
  if (mine.length === 0) return done(false, 'no_grant', { message: 'No grant confers this permission.' })

  let allow = false
  let failingAxis: string | undefined
  let lastReason: AuthorizeResponse['reason']

  for (const g of mine) {
    const verdicts: AxisVerdict[] = []
    let survived = true
    let why: GrantEvaluation['failed_because'] | undefined

    // gate 3: self-scope. Fail-closed when _owner is absent.
    if (g.self_scoped) {
      const owner = req.scopes['_owner']
      const ok = owner !== undefined && owner === identity.id
      if (!ok) { survived = false; why = 'self_scope_mismatch' }
      verdicts.push({
        axis_code: '_owner', constrained: true, satisfied: ok,
        granted_node_id: identity.id, granted_node_name: 'the acting identity',
        target_node_id: owner ?? null,
        target_node_name: owner ? (owner === identity.id ? 'self' : 'another identity') : null,
        inherit: false, path: [],
        ...(owner === undefined
          ? { note: 'No _owner supplied. Self-scoped grants deny rather than ignore.' }
          : {}),
      })
    }

    // OR within an axis — mirror of migration 0013. Rows group per axis and
    // any granted node covering the target satisfies the whole axis. The
    // pre-0013 shape (one verdict per row) was an accidental AND: a grant for
    // Jakarta OR Surabaya denied both, since no target is under two siblings.
    const byAxis = new Map<string, typeof g.scopes>()
    for (const gs of g.scopes) {
      const list = byAxis.get(gs.axis_code) ?? []
      list.push(gs)
      byAxis.set(gs.axis_code, list)
    }
    for (const [axisCode, rows] of byAxis) {
      const target = req.scopes[axisCode]
      const tNode = target ? nodes.find((n) => n.id === target) : undefined
      const grantedNodes = rows.map((gs) => {
        const n = nodes.find((x) => x.id === gs.scope_node_id)
        const matched = target !== undefined && (gs.inherit
          ? isAncestorOrSelf(gs.scope_node_id, target)
          : gs.scope_node_id === target)
        return { id: gs.scope_node_id, name: n?.name ?? null, matched, inherit: gs.inherit }
      })
      const satisfied = target !== undefined && grantedNodes.some((n) => n.matched)
      let note: string | undefined
      if (target === undefined) {
        note = `No target supplied for "${axisCode}". Unresolved axes deny.`
        why ??= 'axis_unresolved'
      } else if (!satisfied) {
        note = rows.length > 1
          ? `Target is not under any of the ${rows.length} granted places.`
          : rows[0]!.inherit
            ? 'Target is not at or beneath the granted node.'
            : 'Grant does not inherit; only the granted node itself matches.'
        why ??= 'scope_mismatch'
      }
      if (!satisfied) { survived = false; failingAxis ??= axisCode }
      const shown = grantedNodes.find((n) => n.matched) ?? grantedNodes[0]
      verdicts.push({
        axis_code: axisCode, constrained: true, satisfied,
        granted_node_id: shown?.id ?? null, granted_node_name: shown?.name ?? null,
        granted_nodes: grantedNodes,
        target_node_id: target ?? null, target_node_name: tNode?.name ?? null,
        inherit: rows.every((r) => r.inherit),
        path: target ? ancestorPath(target) : [],
        ...(note ? { note } : {}),
      })
    }

    // strict axes the grant says nothing about
    for (const ax of axes) {
      if (ax.default_effect !== 'deny' || ax.status !== 'active') continue
      if (g.scopes.some((s) => s.axis_code === ax.code)) continue
      survived = false
      why ??= 'strict_axis_unaddressed'
      failingAxis ??= ax.code
      verdicts.push({
        axis_code: ax.code, constrained: false, satisfied: false,
        granted_node_id: null, granted_node_name: null,
        target_node_id: null, target_node_name: null, inherit: false, path: [],
        note: `Axis is strict (default_effect=deny) and this grant does not address it.`,
      })
    }

    // axes the grant leaves unconstrained -- shown for completeness so an
    // operator can see the axis was considered, not silently skipped
    for (const ax of axes) {
      if (g.scopes.some((s) => s.axis_code === ax.code)) continue
      if (ax.default_effect === 'deny') continue
      verdicts.push({
        axis_code: ax.code, constrained: false, satisfied: true,
        granted_node_id: null, granted_node_name: null,
        target_node_id: req.scopes[ax.code] ?? null,
        target_node_name: req.scopes[ax.code]
          ? nodes.find((n) => n.id === req.scopes[ax.code])?.name ?? null : null,
        inherit: false, path: [],
        note: 'Grant is silent on this axis; axis default is unconstrained.',
      })
    }

    evaluations.push({
      grant_id: g.id, role_name: g.role_name, self_scoped: g.self_scoped,
      survived, axes: verdicts, ...(why ? { failed_because: why } : {}),
    })
    if (survived) allow = true
    else lastReason ??= why
  }

  if (allow) return done(true)

  // step-up is reported separately: the grant is fine, the session is not
  if (perm.requires_amr.length && evaluations.some((e) => e.survived)) {
    return done(false, 'step_up_required', {
      required_amr: perm.requires_amr,
      ...(perm.max_auth_age ? { max_auth_age: perm.max_auth_age } : {}),
      current_amr: ['pwd'],
    })
  }
  return done(false, lastReason ?? 'no_grant', {
    ...(failingAxis ? { failing_axis: failingAxis } : {}),
  })
}

/* ---------- memberships ---------- */
export const memberships: Membership[] = [
  {
    id: 'mem_jkt_fin', name: 'Jakarta Finance Team',
    description: 'Everything a finance hire in Jakarta needs on day one.',
    entries: [
      { id: 'ent_1', role_id: 'rol_fin_view', role_name: 'finance.viewer',
        scopes: [{ axis_code: 'org', scope_node_id: nodeBy('org', 'Jakarta Office').id, inherit: true }] },
      { id: 'ent_2', role_id: 'rol_recruiter', role_name: 'hr.recruiter',
        scopes: [{ axis_code: 'org', scope_node_id: nodeBy('org', 'Jakarta Office').id, inherit: true }] },
      // Multi-structure entry: org AND product must both match (per-grant AND).
      { id: 'ent_3', role_id: 'rol_fin_view', role_name: 'finance.viewer',
        scopes: [
          { axis_code: 'org', scope_node_id: nodeBy('org', 'Finance').id, inherit: true },
          { axis_code: 'product', scope_node_id: nodeBy('product', 'Rigid Packaging').id, inherit: true },
        ] },
    ],
    member_ids: [],
  },
]

export function createMembership(input: {
  name: string; description: string
  entries: { role_id: string; scopes: Grant['scopes'] }[]
}): Membership {
  const name = input.name.trim()
  if (name.length < 2) throw new Error('Membership name is too short.')
  if (memberships.some((m) => m.name.toLowerCase() === name.toLowerCase())) {
    throw new Error(`Membership "${name}" already exists.`)
  }
  if (input.entries.length === 0) throw new Error('Add at least one role to the membership.')
  const entries = input.entries.map((e) => {
    const role = roles.find((r) => r.id === e.role_id)
    if (!role) throw new Error('Unknown role in membership.')
    for (const sc of e.scopes) {
      const n = nodes.find((x) => x.id === sc.scope_node_id)
      if (!n || n.axis_code !== sc.axis_code) throw new Error('Bad place on a membership entry.')
    }
    return { id: uid('ent'), role_id: role.id, role_name: role.name, scopes: e.scopes }
  })
  const m: Membership = { id: uid('mem'), name, description: input.description, entries, member_ids: [] }
  memberships.push(m)
  return m
}

export function assignMembership(identityId: string, membershipId: string): number {
  const m = memberships.find((x) => x.id === membershipId)
  if (!m) throw new Error('Unknown membership.')
  const identity = identities.find((i) => i.id === identityId)
  if (!identity) throw new Error('Unknown person.')
  if (m.member_ids.includes(identityId)) throw new Error(`Already a member of "${m.name}".`)
  // Pre-check EVERY entry against the realm guard so a failure cannot leave a
  // partial fan-out — mirrors the all-or-nothing transaction in the database.
  const kind = realms.find((r) => r.id === identity.realm_id)?.kind
  for (const e of m.entries) {
    const role = roles.find((r) => r.id === e.role_id)!
    if (kind && !role.allowed_realm_kinds.includes(kind)) {
      throw new Error(
        `Membership "${m.name}" includes role "${role.name}", which may not be granted to a "${kind}" identity.`)
    }
  }
  for (const e of m.entries) {
    createGrant({ identity_id: identityId, role_id: e.role_id, self_scoped: false,
      valid_until: null, scopes: e.scopes, via_membership_id: m.id })
  }
  m.member_ids.push(identityId)
  return m.entries.length
}

export function unassignMembership(identityId: string, membershipId: string): number {
  const m = memberships.find((x) => x.id === membershipId)
  if (!m) throw new Error('Unknown membership.')
  let n = 0
  for (let i = grants.length - 1; i >= 0; i--) {
    if (grants[i]!.identity_id === identityId && grants[i]!.via_membership_id === membershipId) {
      grants.splice(i, 1); n++
    }
  }
  m.member_ids = m.member_ids.filter((x) => x !== identityId)
  return n
}

// budi is a member out of the box, so provenance is visible without clicking.
// Seeded directly (not via assignMembership) — the write-ops helpers below are
// declared later in the module and are not initialised yet at this point.
{
  const m0 = memberships[0]!
  m0.member_ids.push(budi.id)
  for (const e of m0.entries) {
    const g = mkGrant(budi, e.role_id, [...e.scopes])
    g.via_membership_id = m0.id
  }
}

/* ---------- dashboard, audit, dry-run ---------- */
export function dashboard(): DashboardStats {
  return {
    identities_by_realm: realms.map((r) => ({
      realm: r.display_name, kind: r.kind,
      count: identities.filter((i) => i.realm_id === r.id).length,
    })),
    grants_total: grants.length,
    scope_nodes_total: nodes.length,
    decisions_24h: 184_302,
    deny_rate_24h: 0.061,
    p99_authorize_ms: 0.41,
    signals: [
      { kind: 'refresh_token_reuse', severity: 'page', count: 1,
        detail: 'Token family revoked for eka@globex.com. A refresh token was replayed.',
        since: '2026-08-22T04:12:00Z' },
      { kind: 'key_rotation_due', severity: 'alert', count: 1,
        detail: 'Signing key kid=k4-2026-05 is 94 days old (policy: 90).',
        since: '2026-08-20T00:00:00Z' },
      { kind: 'retention_overdue', severity: 'alert', count: 23,
        detail: '23 applicant records are past retention_until and not yet anonymised.',
        since: '2026-08-21T00:00:00Z' },
    ],
  }
}

export const audit: AuditEntry[] = [
  { id: uid('aud'), occurred_at: '2026-08-22T04:12:03Z', actor_id: eka.id,
    actor_label: 'eka@globex.com', action: 'auth.refresh.reuse_detected', result: 'deny',
    target_id: null, ip: '103.28.14.9',
    detail: { family_id: 'fam_9271', revoked_tokens: 4 }, chain_ok: true },
  { id: uid('aud'), occurred_at: '2026-08-22T03:58:41Z', actor_id: alice.id,
    actor_label: 'alice@impack.co.id', action: 'authz.decision', result: 'deny',
    target_id: null, ip: '10.4.2.88',
    detail: { permission: 'billing:payment:approve', failing_axis: 'product' }, chain_ok: true },
  { id: uid('aud'), occurred_at: '2026-08-22T03:41:12Z', actor_id: citra.id,
    actor_label: 'citra@impack.co.id', action: 'grant.create', result: 'allow',
    target_id: fajar.id, ip: '10.4.2.31',
    detail: { role: 'public.applicant', self_scoped: true }, chain_ok: true },
  { id: uid('aud'), occurred_at: '2026-08-22T02:07:55Z', actor_id: null,
    actor_label: 'system', action: 'scope.axis.created', result: 'allow',
    target_id: null, ip: null, detail: { axis: 'customer', nodes_synced: 66 }, chain_ok: true },
]

export function strictDryRun(axisCode: string): StrictDryRun {
  const relying = grants.filter((g) => !g.scopes.some((s) => s.axis_code === axisCode)).length
  const before = grants.length
  return {
    axis_code: axisCode, decisions_sampled: 2000,
    allowed_before: 800, allowed_after: Math.max(0, 800 - Math.round(800 * (relying / before))),
    grants_relying_on_absence: relying,
  }
}

/* ===========================================================================
   WRITE OPERATIONS
   ===========================================================================
   Every mutation validates the same invariants the database schema enforces
   (migrations 0008/0010), and throws with the same shape of message the SQL
   guards raise. The console therefore demonstrates the guards instead of
   hiding them behind a generic "something went wrong":

     - roles.allowed_realm_kinds     grant guard (trigger, migration 0010)
     - self_scoped XOR axis scopes   grant_scopes_self_guard
     - username unique per realm     identities_username index (0008)
     - node type legal under parent  scope_node_types.parent_types
     - sibling slug uniqueness       scope_nodes_sibling_slug
     - axis code charset + reserve   scope_axes.code CHECK
   =========================================================================== */

export const applications = [
  { id: 'app_billing', slug: 'billing', name: 'Billing' },
  { id: 'app_procure', slug: 'procure', name: 'Procurement' },
  { id: 'app_hr',      slug: 'hr',      name: 'HR' },
  { id: 'app_ats',     slug: 'ats',     name: 'Recruitment (ATS)' },
]

const nowIso = () => new Date().toISOString()

/* ---------- tenants (platform view) ---------- */
export const tenantsList: Tenant[] = [
  { id: TENANT, slug: 'impack', name: 'PT Impack Pratama', status: 'active',
    created_at: '2026-01-02T00:00:00Z' },
]

export function createTenant(input: { name: string }): Tenant {
  const name = input.name.trim()
  if (name.length < 2) throw new Error('Tenant name is too short.')
  const slug = name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 40)
  if (!/^[a-z0-9][a-z0-9-]{1,39}$/.test(slug)) throw new Error(`"${name}" does not reduce to a valid slug.`)
  if (tenantsList.some((t) => t.slug === slug)) throw new Error(`Tenant "${slug}" already exists.`)
  const t: Tenant = { id: uid('tnt'), slug, name, status: 'active', created_at: new Date().toISOString() }
  tenantsList.push(t)
  return t
}

export function setTenantStatus(id: string, status: Tenant['status']): Tenant {
  const t = tenantsList.find((x) => x.id === id)
  if (!t) throw new Error('Unknown tenant.')
  if (t.id === TENANT && status === 'suspended') {
    throw new Error('Refusing to suspend the tenant you are currently signed into.')
  }
  t.status = status
  return t
}

export function tenantStats(id: string): TenantStats {
  if (id !== TENANT) return { identities: 0, grants: 0, scope_nodes: 0, memberships: 0 }
  return {
    identities: identities.length, grants: grants.length,
    scope_nodes: nodes.filter((n) => n.status === 'active').length,
    memberships: memberships.length,
  }
}

/* ---------- sign-in page configs ---------- */
export const DEFAULT_SIGNIN: SignInConfig = {
  layout: 'centered', theme: 'light', brand_color: '#b8860b',
  logo_text: '', headline: 'Welcome back', subheadline: 'Sign in to continue',
  background: 'gradient', show_populations: true,
  footer_note: 'Trouble signing in? Contact your administrator.', language: 'en',
}
const signinConfigs: Record<string, SignInConfig> = {
  [TENANT]: { ...DEFAULT_SIGNIN, logo_text: 'Impack', brand_color: '#9a7412',
    headline: 'Welcome back', subheadline: 'Sign in to PT Impack Pratama' },
}

export function getSignin(tenantId: string): SignInConfig {
  const t = tenantsList.find((x) => x.id === tenantId)
  if (!t) throw new Error('Unknown tenant.')
  return signinConfigs[tenantId] ?? { ...DEFAULT_SIGNIN, logo_text: t.name.split(' ')[0] ?? t.slug }
}

export function saveSignin(tenantId: string, cfg: SignInConfig): SignInConfig {
  if (!tenantsList.some((x) => x.id === tenantId)) throw new Error('Unknown tenant.')
  if (!/^#[0-9a-fA-F]{6}$/.test(cfg.brand_color)) throw new Error('Brand colour must be a hex value.')
  if (cfg.headline.trim().length === 0) throw new Error('The headline cannot be empty.')
  signinConfigs[tenantId] = { ...cfg }
  return signinConfigs[tenantId]!
}

/* ---------- scope sync ----------
   The fetch (HTTP call / DB query) is the app tier's job; here a canned
   "remote" simulates the ERP drifting from the tree: one rename, one vanished
   supplier, one new one. Reconciliation is keyed on external_ref, archives
   rather than deletes, and never touches manual (ref-less) nodes. */
export const syncSources: SyncSource[] = [
  { id: 'src_partner', axis_code: 'partner', kind: 'http',
    target: 'https://erp.impack.co.id/api/suppliers',
    default_node_type: 'partner_org', last_run_at: null },
]
export const syncRuns: SyncRun[] = []

function remoteRows(source: SyncSource): { ref: string; name: string }[] {
  const current = nodes.filter((n) =>
    n.axis_code === source.axis_code && n.external_ref && n.status === 'active')
  const rows = current
    .filter((n) => n.external_ref !== 'ERP-SUP-4')            // Umbrella left the ERP
    .map((n) => ({
      ref: n.external_ref!,
      name: n.external_ref === 'ERP-SUP-2' ? 'Globex Materials Intl' : n.name,
    }))
  if (!current.some((n) => n.external_ref === 'ERP-NEW-1')) {
    rows.push({ ref: 'ERP-NEW-1', name: 'Wayne Polymers' })   // new in the ERP
  }
  return rows
}

export function syncPlan(sourceId: string): SyncPlan {
  const source = syncSources.find((s) => s.id === sourceId)
  if (!source) throw new Error('Unknown sync source.')
  const rows = remoteRows(source)
  const byRef = new Map(nodes
    .filter((n) => n.axis_code === source.axis_code && n.external_ref)
    .map((n) => [n.external_ref!, n]))
  const plan: SyncPlan = { added: [], renamed: [], archived: [], unchanged: 0 }
  for (const r of rows) {
    const n = byRef.get(r.ref)
    if (!n || n.status === 'archived') {
      if (n) plan.renamed.push({ ref: r.ref, from: `${n.name} (archived)`, to: r.name })
      else plan.added.push(r)
    } else if (n.name !== r.name) plan.renamed.push({ ref: r.ref, from: n.name, to: r.name })
    else plan.unchanged++
  }
  for (const [ref, n] of byRef) {
    if (n.status === 'active' && !rows.some((r) => r.ref === ref)) {
      plan.archived.push({ ref, name: n.name })
    }
  }
  return plan
}

export function syncApply(sourceId: string): SyncRun {
  const source = syncSources.find((s) => s.id === sourceId)!
  const plan = syncPlan(sourceId)
  const root = nodes.find((n) => n.axis_code === source.axis_code && n.is_axis_root)!
  for (const a of plan.added) {
    const n = addNode(source.axis_code, source.default_node_type, root.id,
      a.name.toLowerCase().replace(/[^a-z0-9]+/g, '-'), a.name)
    n.external_ref = a.ref
  }
  for (const r of plan.renamed) {
    const n = nodes.find((x) => x.axis_code === source.axis_code && x.external_ref === r.ref)!
    n.name = r.to; n.status = 'active'
  }
  for (const g of plan.archived) {
    const n = nodes.find((x) => x.axis_code === source.axis_code && x.external_ref === g.ref)!
    n.status = 'archived'
  }
  const run: SyncRun = {
    id: uid('run'), source_id: source.id, at: new Date().toISOString(), dry: false,
    added: plan.added.length, renamed: plan.renamed.length,
    archived: plan.archived.length, unchanged: plan.unchanged,
  }
  syncRuns.unshift(run)
  source.last_run_at = run.at
  return run
}

export function createSyncSource(input: {
  axis_code: string; kind: SyncSource['kind']; target: string; default_node_type: string
}): SyncSource {
  if (!axes.some((a) => a.code === input.axis_code)) throw new Error('Unknown structure.')
  if (syncSources.some((s) => s.axis_code === input.axis_code)) {
    throw new Error('This structure already has a source — one source of truth per structure.')
  }
  if (!input.target.trim()) throw new Error('Point the source at a URL, query or table.')
  const src: SyncSource = { id: uid('src'), axis_code: input.axis_code, kind: input.kind,
    target: input.target.trim(), default_node_type: input.default_node_type, last_run_at: null }
  syncSources.push(src)
  return src
}

export function createIdentity(input: {
  realm_id: string; username: string; email: string; assurance_level: 1 | 2 | 3
  category_id?: string | null
}): Identity {
  const realm = realms.find((r) => r.id === input.realm_id)
  if (!realm) throw new Error('Unknown realm.')
  const username = input.username.trim()
  if (!/^[a-z0-9][a-z0-9._-]{1,62}$/i.test(username)) {
    throw new Error('Username must be 2–63 characters: letters, digits, dot, dash, underscore.')
  }
  // uniqueness is per (tenant, realm) — the whole point of realms
  if (identities.some((i) => i.realm_id === realm.id
      && i.username.toLowerCase() === username.toLowerCase())) {
    throw new Error(`"${username}" already exists in the ${realm.display_name} realm.`)
  }
  if (input.assurance_level < realm.min_assurance) {
    throw new Error(
      `The ${realm.display_name} realm requires at least IAL${realm.min_assurance}; got IAL${input.assurance_level}.`)
  }
  // Mirror of identities_category_same_realm (migration 0011).
  if (input.category_id) {
    const cat = realmCategories.find((c) => c.id === input.category_id)
    if (!cat) throw new Error('Unknown category.')
    if (cat.realm_id !== realm.id) {
      throw new Error(
        `Category "${cat.display_name}" belongs to a different realm than ${realm.display_name}.`)
    }
  }
  const identity: Identity = {
    id: uid('usr'), tenant_id: TENANT, realm_id: realm.id, username,
    category_id: input.category_id ?? null,
    email: input.email.trim() || null, status: 'active',
    assurance_level: input.assurance_level, token_epoch: 1, external_ref: null,
    created_at: nowIso(), last_login_at: null, disabled_at: null,
    retention_until: realm.default_retention ? '2028-08-22T00:00:00Z' : null,
    anonymized_at: null,
  }
  identities.push(identity)
  return identity
}

export function createRealmCategory(input: {
  realm_id: string; display_name: string
}): RealmCategory {
  const realm = realms.find((r) => r.id === input.realm_id)
  if (!realm) throw new Error('Unknown realm.')
  const name = input.display_name.trim()
  if (name.length < 2) throw new Error('Category name is too short.')
  const code = name.toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_+|_+$/g, '')
  if (!/^[a-z][a-z0-9_]{1,30}$/.test(code)) {
    throw new Error(`"${name}" does not reduce to a valid code.`)
  }
  if (realmCategories.some((c) => c.realm_id === realm.id && c.code === code)) {
    throw new Error(`Category "${code}" already exists in ${realm.display_name}.`)
  }
  const cat: RealmCategory = {
    id: uid('cat'), realm_id: realm.id, code, display_name: name,
    sort_order: Math.max(0, ...realmCategories.filter((c) => c.realm_id === realm.id)
      .map((c) => c.sort_order)) + 10,
  }
  realmCategories.push(cat)
  return cat
}

export function setIdentityStatus(id: string, status: 'active' | 'disabled'): Identity {
  const identity = identities.find((i) => i.id === id)
  if (!identity) throw new Error('Unknown identity.')
  identity.status = status
  identity.disabled_at = status === 'disabled' ? nowIso() : null
  return identity
}

export function createPermission(input: {
  app_slug: string; resource: string; action: string; description: string
  risk: Permission['risk']; min_assurance: Permission['min_assurance']
  requires_amr: string[]; max_auth_age: string | null
}): Permission {
  const app = applications.find((a) => a.slug === input.app_slug)
  if (!app) throw new Error('Unknown application.')
  for (const [field, v] of [['resource', input.resource], ['action', input.action]] as const) {
    if (!/^[a-z][a-z0-9_]{1,30}$/.test(v)) {
      throw new Error(`${field} must be lowercase snake_case (got "${v}").`)
    }
  }
  const key = `${input.app_slug}:${input.resource}:${input.action}`
  if (permissions.some((p) => p.key === key)) {
    throw new Error(`Permission "${key}" already exists.`)
  }
  const p: Permission = {
    id: uid('prm'), key, application_id: app.id, app_slug: app.slug,
    resource: input.resource, action: input.action, description: input.description,
    risk: input.risk, requires_amr: input.requires_amr,
    max_auth_age: input.requires_amr.length ? input.max_auth_age : null,
    min_assurance: input.min_assurance, deprecated_at: null,
  }
  permissions.push(p)
  return p
}

export function createRole(input: {
  name: string; description: string
  allowed_realm_kinds: Role['allowed_realm_kinds']; permission_keys: string[]
}): Role {
  if (!/^[a-z][a-z0-9._-]{1,62}$/.test(input.name)) {
    throw new Error('Role name must be lowercase, e.g. "finance.approver".')
  }
  if (roles.some((r) => r.name === input.name)) {
    throw new Error(`Role "${input.name}" already exists.`)
  }
  if (input.allowed_realm_kinds.length === 0) {
    throw new Error('Pick at least one realm kind the role may be granted to.')
  }
  const missing = input.permission_keys.filter((k) => !permissions.some((p) => p.key === k))
  if (missing.length) throw new Error(`Unknown permissions: ${missing.join(', ')}`)
  const role: Role = {
    id: uid('rol'), name: input.name, description: input.description,
    application_id: null, is_system: false,
    allowed_realm_kinds: input.allowed_realm_kinds,
    assignable_at: [], permission_count: input.permission_keys.length,
  }
  roles.push(role)
  rolePerms[role.id] = [...input.permission_keys]
  return role
}

export function updateRole(input: {
  role_id: string; description: string
  allowed_realm_kinds: Role['allowed_realm_kinds']; permission_keys: string[]
}): Role {
  const role = roles.find((r) => r.id === input.role_id)
  if (!role) throw new Error('Unknown role.')
  // Manifest-declared roles are owned by the application that registered them.
  if (role.is_system) {
    throw new Error(
      `"${role.name}" is declared in an application manifest — change the manifest and redeploy, not the console.`)
  }
  if (input.permission_keys.length === 0) {
    throw new Error('A role needs at least one permission — delete the role instead of emptying it.')
  }
  const missing = input.permission_keys.filter((k) => !permissions.some((p) => p.key === k))
  if (missing.length) throw new Error(`Unknown permissions: ${missing.join(', ')}`)

  // Narrowing who may hold the role must not orphan existing holders: the
  // grants would instantly violate the realm guard the schema enforces.
  const removedKinds = role.allowed_realm_kinds.filter((k) => !input.allowed_realm_kinds.includes(k))
  if (removedKinds.length) {
    const holders = grants.filter((g) => g.role_id === role.id && !g.revoked_at)
      .map((g) => identities.find((i) => i.id === g.identity_id))
      .filter((i) => {
        const kind = i && realms.find((r) => r.id === i.realm_id)?.kind
        return kind && removedKinds.includes(kind)
      })
    if (holders.length) {
      throw new Error(
        `${holders.length} ${removedKinds.join('/')} member(s) still hold this role — revoke their access first.`)
    }
  }
  role.description = input.description
  role.allowed_realm_kinds = [...input.allowed_realm_kinds]
  role.permission_count = input.permission_keys.length
  rolePerms[role.id] = [...input.permission_keys]
  return role
}

export function deleteRole(roleId: string): Role {
  const role = roles.find((r) => r.id === roleId)
  if (!role) throw new Error('Unknown role.')
  if (role.is_system) {
    throw new Error(`"${role.name}" is declared in an application manifest — remove it there.`)
  }
  // Mirror of the RESTRICT FKs (migration 0016): consequences block deletion.
  const holders = grants.filter((g) => g.role_id === roleId && !g.revoked_at)
  if (holders.length) {
    throw new Error(
      `${holders.length} grant${holders.length > 1 ? 's' : ''} still reference "${role.name}" — revoke that access first.`)
  }
  const bundledIn = memberships.filter((m) => m.entries.some((e) => e.role_id === roleId))
  if (bundledIn.length) {
    throw new Error(
      `"${role.name}" is bundled in: ${bundledIn.map((m) => m.name).join(', ')} — remove it from the membership first.`)
  }
  roles.splice(roles.indexOf(role), 1)
  delete rolePerms[roleId]
  audit.unshift({
    id: uid('aud'), occurred_at: new Date().toISOString(), actor_id: null,
    actor_label: 'console', action: 'role.delete', result: 'allow',
    target_id: roleId, ip: null, detail: { role: role.name }, chain_ok: true,
  })
  return role
}

export function createGrant(input: {
  identity_id: string; role_id: string; self_scoped: boolean
  valid_until: string | null; scopes: Grant['scopes']
  via_membership_id?: string | null
}): Grant {
  const identity = identities.find((i) => i.id === input.identity_id)
  if (!identity) throw new Error('Unknown identity.')
  const role = roles.find((r) => r.id === input.role_id)
  if (!role) throw new Error('Unknown role.')

  // Mirror of trg_grant_realm_guard (migration 0010), message included.
  const kind = realms.find((r) => r.id === identity.realm_id)?.kind
  if (kind && !role.allowed_realm_kinds.includes(kind)) {
    throw new Error(
      `role "${role.name}" may not be granted to a "${kind}" identity (allowed: ${role.allowed_realm_kinds.join(', ')})`)
  }
  // Mirror of grant_scopes_self_guard.
  if (input.self_scoped && input.scopes.length > 0) {
    throw new Error('A self-scoped grant may not carry axis constraints — the two are orthogonal.')
  }
  const seen = new Set<string>()
  for (const s of input.scopes) {
    const k = `${s.axis_code}:${s.scope_node_id}`
    if (seen.has(k)) throw new Error('The same place is listed twice on one axis.')
    seen.add(k)
    const node = nodes.find((n) => n.id === s.scope_node_id)
    if (!node) throw new Error(`Unknown scope node for axis "${s.axis_code}".`)
    if (node.axis_code !== s.axis_code) {
      throw new Error(`Node "${node.name}" belongs to axis "${node.axis_code}", not "${s.axis_code}".`)
    }
  }
  const grant: Grant = {
    id: uid('grt'), identity_id: identity.id, role_id: role.id, role_name: role.name,
    via_membership_id: input.via_membership_id ?? null,
    self_scoped: input.self_scoped, valid_from: nowIso(),
    valid_until: input.valid_until, revoked_at: null,
    granted_by: 'usr_console', reason: null, scopes: input.scopes,
  }
  grants.push(grant)
  return grant
}

export function revokeGrant(id: string): Grant {
  const grant = grants.find((g) => g.id === id)
  if (!grant) throw new Error('Unknown grant.')
  if (grant.via_membership_id) {
    const m = memberships.find((x) => x.id === grant.via_membership_id)
    throw new Error(`This access is managed by the "${m?.name}" membership — remove the person there instead.`)
  }
  grant.revoked_at = nowIso()
  // authorize() filters on revoked_at, so removal from the live list keeps the
  // console's list view and the decision engine telling the same story.
  grants.splice(grants.indexOf(grant), 1)
  return grant
}

export function createScopeNode(input: {
  axis_code: string; parent_id: string; node_type: string
  name: string; slug: string; external_ref: string | null
}): ScopeNode {
  const axis = axes.find((a) => a.code === input.axis_code)
  if (!axis) throw new Error('Unknown axis.')
  const parent = nodes.find((n) => n.id === input.parent_id)
  if (!parent) throw new Error('Unknown parent node.')
  if (parent.axis_code !== input.axis_code) {
    throw new Error(`Parent belongs to axis "${parent.axis_code}", not "${input.axis_code}".`)
  }
  const type = nodeTypes.find((t) => t.code === input.node_type && t.axis_code === input.axis_code)
  if (!type) throw new Error(`Node type "${input.node_type}" does not exist on this axis.`)
  if (!type.parent_types.includes(parent.node_type)) {
    throw new Error(
      `A "${type.display_name}" cannot sit under a "${parent.node_type}" (legal parents: ${type.parent_types.join(', ') || 'none'}).`)
  }
  if (!/^[a-z0-9][a-z0-9-]{0,62}$/.test(input.slug)) {
    throw new Error('Slug must be lowercase letters, digits and dashes.')
  }
  if (nodes.some((n) => n.parent_id === parent.id && n.slug === input.slug)) {
    throw new Error(`A sibling with slug "${input.slug}" already exists under ${parent.name}.`)
  }
  const node = addNode(input.axis_code, input.node_type, parent.id, input.slug, input.name)
  node.external_ref = input.external_ref
  return node
}

/* Node types are rows too — "user can create it based on their needs" has to
   hold one level below axes. Legal-parent lists are validated against the same
   axis, mirroring scope_node_types.parent_types. */
export function createNodeType(input: {
  axis_code: string; display_name: string; parent_types: string[]
}): ScopeNodeType {
  const axis = axes.find((a) => a.code === input.axis_code)
  if (!axis) throw new Error('Unknown axis.')
  const name = input.display_name.trim()
  const code = name.toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_+|_+$/g, '')
  if (!/^[a-z][a-z0-9_]{1,30}$/.test(code)) throw new Error(`"${name}" does not reduce to a valid code.`)
  if (nodeTypes.some((t) => t.axis_code === axis.code && t.code === code)) {
    throw new Error(`Node type "${code}" already exists on this axis.`)
  }
  if (input.parent_types.length === 0) {
    throw new Error('Pick at least one legal parent — only the axis root has none.')
  }
  for (const pt of input.parent_types) {
    if (!nodeTypes.some((t) => t.axis_code === axis.code && t.code === pt)) {
      throw new Error(`Parent type "${pt}" does not exist on axis "${axis.code}".`)
    }
  }
  const t: ScopeNodeType = {
    code, axis_code: axis.code, display_name: name, parent_types: input.parent_types,
  }
  nodeTypes.push(t)
  return t
}

export function setNodeTypeParents(
  axis_code: string, code: string, parent_types: string[],
): ScopeNodeType {
  const t = nodeTypes.find((x) => x.axis_code === axis_code && x.code === code)
  if (!t) throw new Error('Unknown item kind.')
  if (t.parent_types.length === 0) {
    throw new Error('The root kind has no parents — that is what makes it the root.')
  }
  if (parent_types.length === 0) {
    throw new Error('A non-root kind needs at least one legal parent.')
  }
  for (const pt of parent_types) {
    if (!nodeTypes.some((x) => x.axis_code === axis_code && x.code === pt)) {
      throw new Error(`"${pt}" is not a kind on this structure.`)
    }
  }
  t.parent_types = [...parent_types]
  return t
}

export function createAxis(input: {
  code: string; display_name: string; default_effect: ScopeAxis['default_effect']
  resolution_from: 'token' | 'context'; resolution_key: string | null
  picker: 'tree' | 'select' | 'search'; icon: string
}): ScopeAxis {
  if (!/^[a-z][a-z0-9_]{1,30}$/.test(input.code)) {
    throw new Error('Axis code must be lowercase snake_case; the underscore prefix is reserved.')
  }
  if (axes.some((a) => a.code === input.code)) {
    throw new Error(`Axis "${input.code}" already exists.`)
  }
  const axis: ScopeAxis = {
    code: input.code, display_name: input.display_name,
    default_effect: input.default_effect, status: 'active',
    sort_order: Math.max(...axes.map((a) => a.sort_order)) + 10,
    resolution: input.resolution_from === 'token'
      ? { from: 'token' }
      : { from: 'context', key: input.resolution_key ?? `${input.code}_id` },
    ui_schema: { picker: input.picker, icon: input.icon, searchable: true },
  }
  axes.push(axis)
  // An axis without a root and at least one child type is unusable, so
  // registration provisions both — mirroring scope_ensure_root (migration 0003).
  nodeTypes.push(
    { code: `${input.code}_root`, axis_code: input.code,
      display_name: `All ${input.display_name}`, parent_types: [] },
    { code: `${input.code}_item`, axis_code: input.code,
      display_name: input.display_name, parent_types: [`${input.code}_root`, `${input.code}_item`] },
  )
  addNode(input.code, `${input.code}_root`, null, '_root', `All ${input.display_name}`)
  return axis
}
