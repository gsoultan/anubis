/* Domain types mirroring the Anubis database schema (migrations/0001-0010).
   Field names match the SQL columns so the two can be diffed by eye. */

export type Uuid = string
export type RealmKind = 'internal' | 'partner' | 'public' | 'service'
export type AxisDefaultEffect = 'unconstrained' | 'deny'
export type IdentityStatus = 'active' | 'disabled' | 'locked' | 'pending'
export type Risk = 'normal' | 'sensitive' | 'critical'
/** NIST 800-63 Identity Assurance Level. Ordered, so it compares numerically. */
export type Ial = 1 | 2 | 3

export interface Realm {
  id: Uuid
  code: string
  kind: RealmKind
  display_name: string
  min_assurance: Ial
  self_registration: boolean
  email_verification_required: boolean
  allowed_factors: string[]
  required_factors: string[]
  session_ttl: string
  access_token_ttl: string
  refresh_token_ttl: string
  /** null = no statutory retention limit (employees). */
  default_retention: string | null
  pii_encryption: boolean
}

/* ---------------------------------------------------------------------------
   Scope axes.

   `ui_schema` is the contract that keeps this console schema-driven. Every
   scope control renders from these fields. There is no switch on `code`
   anywhere in the codebase -- adding an axis server-side makes it appear in
   every picker, filter, grant builder and explain view with no frontend change.
   --------------------------------------------------------------------------- */
export interface AxisUiSchema {
  picker?: 'tree' | 'select' | 'search'
  icon?: string
  searchable?: boolean
  /** Optional operator-facing hint shown beside the control. */
  help?: string
}

export interface ScopeAxis {
  code: string
  display_name: string
  default_effect: AxisDefaultEffect
  status: 'active' | 'deprecated'
  sort_order: number
  /** Where the target value comes from when a decision is evaluated. */
  resolution: { from: 'token' | 'context'; key?: string }
  ui_schema: AxisUiSchema
}

export interface ScopeNodeType {
  code: string
  axis_code: string
  display_name: string
  parent_types: string[]
}

export interface ScopeNode {
  id: Uuid
  tenant_id: Uuid
  axis_code: string
  node_type: string
  parent_id: Uuid | null
  slug: string
  name: string
  external_ref: string | null
  is_axis_root: boolean
  status: 'active' | 'archived'
  /** Display metadata only. Never a decision input -- see ADR-0003. */
  attributes: Record<string, unknown>
  /** Denormalised for tree rendering; not authoritative. */
  child_count?: number
}

export interface RealmCategory {
  id: Uuid
  realm_id: Uuid
  code: string
  display_name: string
  sort_order: number
}

export interface Identity {
  id: Uuid
  tenant_id: Uuid
  realm_id: Uuid
  /** Directory classification within the realm (supplier, applicant, …).
      Never an authorization input. */
  category_id: Uuid | null
  username: string
  email: string | null
  status: IdentityStatus
  assurance_level: Ial
  token_epoch: number
  external_ref: string | null
  created_at: string
  last_login_at: string | null
  disabled_at: string | null
  retention_until: string | null
  anonymized_at: string | null
}

export interface Permission {
  id: Uuid
  key: string
  application_id: Uuid
  app_slug: string
  resource: string
  action: string
  description: string
  risk: Risk
  requires_amr: string[]
  max_auth_age: string | null
  min_assurance: Ial
  deprecated_at: string | null
}

export interface Role {
  id: Uuid
  name: string
  description: string
  application_id: Uuid | null
  is_system: boolean
  allowed_realm_kinds: RealmKind[]
  assignable_at: string[]
  permission_count: number
}

/** One axis constraint on a grant. `inherit` is per-axis, not per-grant. */
export interface GrantScope {
  axis_code: string
  scope_node_id: Uuid
  inherit: boolean
}

export interface MembershipEntry {
  id: Uuid
  role_id: Uuid
  role_name: string
  scopes: GrantScope[]
}

/** A named bundle of (role, place) entries — assign the bundle, not N grants.
    Flat by design: no membership-in-membership. */
export interface Membership {
  id: Uuid
  name: string
  description: string
  entries: MembershipEntry[]
  /** Empty when the server reported a count instead of a roster. */
  member_ids: Uuid[]
  /** How many people are in this membership. Falls back to member_ids.length
      for the sample data, which carries a roster rather than a count. */
  member_count?: number
}

export interface Grant {
  id: Uuid
  identity_id: Uuid
  role_id: Uuid
  role_name: string
  /** Set when this grant derives from a membership. Managed there, not here. */
  via_membership_id: Uuid | null
  /** "only your own record" -- mutually exclusive with axis constraints. */
  self_scoped: boolean
  valid_from: string
  valid_until: string | null
  revoked_at: string | null
  granted_by: Uuid
  reason: string | null
  scopes: GrantScope[]
}

/* ---------------------------------------------------------------------------
   Authorization
   --------------------------------------------------------------------------- */

/** Axis code -> scope node id. `_owner` is reserved for self-scoped grants;
    reserved keys start with '_', which axis codes may never do. */
export type TargetMap = Record<string, Uuid>

export interface AuthorizeRequest {
  subject: Uuid
  permission: string
  scopes: TargetMap
}

export type DenyReason =
  | 'no_grant'
  | 'scope_mismatch'
  | 'axis_unresolved'
  | 'strict_axis_unaddressed'
  | 'assurance_too_low'
  | 'identity_inactive'
  | 'self_scope_mismatch'
  | 'step_up_required'

/** Per-axis verdict, so the UI can name exactly which constraint failed
    rather than showing an unhelpful boolean. */
export interface AxisVerdict {
  axis_code: string
  constrained: boolean
  satisfied: boolean
  granted_node_id: Uuid | null
  granted_node_name: string | null
  /** All nodes granted on this axis (OR semantics): the matched one flagged. */
  granted_nodes?: { id: Uuid; name: string | null; matched: boolean; inherit: boolean }[]
  target_node_id: Uuid | null
  target_node_name: string | null
  inherit: boolean
  /** Ancestor chain proving (or disproving) reachability. */
  path: { id: Uuid; name: string; depth: number }[]
  note?: string
}

export interface GrantEvaluation {
  grant_id: Uuid
  role_name: string
  self_scoped: boolean
  survived: boolean
  axes: AxisVerdict[]
  failed_because?: DenyReason
}

export interface AuthorizeResponse {
  allow: boolean
  reason?: DenyReason
  failing_axis?: string
  message?: string
  required_amr?: string[]
  max_auth_age?: string
  current_amr?: string[]
  /** Every candidate grant considered, with per-axis detail. This is what
      makes a denial debuggable instead of mysterious. */
  evaluations: GrantEvaluation[]
  took_ms: number
}

export interface AuditEntry {
  id: Uuid
  occurred_at: string
  actor_id: Uuid | null
  actor_label: string
  action: string
  result: 'allow' | 'deny' | 'error'
  target_id: Uuid | null
  ip: string | null
  detail: Record<string, unknown>
  /** Hash-chain link. A break means tampering or a bug destroying evidence. */
  chain_ok: boolean
}

export interface SecuritySignal {
  kind:
    | 'refresh_token_reuse'
    | 'login_failure_spike'
    | 'snapshot_stale'
    | 'key_rotation_due'
    | 'audit_chain_broken'
    | 'retention_overdue'
  severity: 'page' | 'alert' | 'info'
  count: number
  detail: string
  since: string
}

export interface Tenant {
  id: Uuid
  slug: string
  name: string
  status: 'active' | 'suspended'
  created_at: string
}

export interface TenantStats {
  identities: number
  grants: number
  scope_nodes: number
  memberships: number
}

/** Constrained sign-in page config — knobs that cannot break the page. */
export interface SignInConfig {
  layout: 'centered' | 'split'
  theme: 'light' | 'dark'
  brand_color: string
  logo_text: string
  headline: string
  subheadline: string
  background: 'solid' | 'gradient'
  show_populations: boolean
  footer_note: string
  language: 'en' | 'id'
}

export interface DashboardStats {
  identities_by_realm: { realm: string; kind: RealmKind; count: number }[]
  grants_total: number
  scope_nodes_total: number
  decisions_24h: number
  deny_rate_24h: number
  p99_authorize_ms: number
  signals: SecuritySignal[]
}

/** Result of previewing a flip of an axis to default_effect='deny'. */
export type SyncKind = 'http' | 'db_query' | 'db_table'

export interface SyncSource {
  id: Uuid
  axis_code: string
  kind: SyncKind
  /** URL, SQL text, or table name — what the kind points at. */
  target: string
  default_node_type: string
  last_run_at: string | null
}

/** The reconciler's report, as the server actually emits it: counts plus the
    rows it could not place. Per-row before/after listings were the sample
    data's invention — the server does not narrate them. */
export interface SyncPlan {
  dry: boolean
  added: number
  renamed: number
  moved: number
  archived: number
  unchanged: number
  errors: { ref: string; error: string }[]
}

export interface SyncRun {
  id: Uuid
  source_id: Uuid
  at: string
  dry: boolean
  added: number
  renamed: number
  archived: number
  unchanged: number
}

export interface StrictDryRun {
  axis_code: string
  /** Real authorize decisions replayed with the axis forced strict. */
  sampled: number
  would_deny: number
  examples: unknown[]
}

/* ---------------------------------------------------------------------------
   Applications & write operations
   --------------------------------------------------------------------------- */
export interface Application {
  id: Uuid
  slug: string
  name: string
}

export interface NewIdentityInput {
  realm_id: Uuid
  username: string
  email: string
  assurance_level: Ial
  category_id: Uuid | null
}

export interface NewPermissionInput {
  app_slug: string
  resource: string
  action: string
  description: string
  risk: Risk
  min_assurance: Ial
  requires_amr: string[]
  max_auth_age: string | null
}

export interface NewRoleInput {
  name: string
  description: string
  allowed_realm_kinds: RealmKind[]
  permission_keys: string[]
}

export interface NewGrantInput {
  identity_id: Uuid
  role_id: Uuid
  self_scoped: boolean
  valid_until: string | null
  scopes: GrantScope[]
}

export interface NewNodeInput {
  axis_code: string
  parent_id: Uuid
  node_type: string
  name: string
  slug: string
  external_ref: string | null
}

export interface NewAxisInput {
  code: string
  display_name: string
  default_effect: AxisDefaultEffect
  resolution_from: 'token' | 'context'
  resolution_key: string | null
  picker: 'tree' | 'select' | 'search'
  icon: string
}
