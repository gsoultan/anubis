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
  /** Unix seconds when required_factors starts being enforced against members
      who have not enrolled. null = never, which is the default — and means a
      required factor is currently decorative. */
  factor_enrolment_deadline: number | null
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
  /** Counted in the database, not by tallying a fetched page. */
  identity_count: number
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
/* Page configuration, mirroring internal/tenancy/domain/pagecfg exactly.
 *
 * It has to mirror it exactly. The console previously edited a FLAT shape
 * (logo_text, brand_color, theme, background) against the legacy
 * signin_pages table, while the server rendered a NESTED one
 * (brand.logo_url, brand.primary_color) out of auth_pages. Nothing
 * translated between them, so half of what the builder wrote was never read
 * and brand.logo_url — a field the template renders and validates — had no
 * input at all. Renaming or flattening anything here reopens that gap.
 */
export type PageKind = 'signin' | 'signout'
export type PageLayout = 'centered' | 'split' | 'minimal'
export type CornerRadius = 'none' | 'sm' | 'md' | 'lg' | 'full'
export type PageFont = 'system' | 'serif' | 'mono'

export interface PageBrand {
  title: string
  /** Rendered as <img src>. The server rejects javascript: and data: URLs. */
  logo_url?: string
  primary_color: string
  background_color: string
  text_color: string
  corner_radius: CornerRadius
  font: PageFont
}

export interface PageCopy {
  heading: string
  subheading?: string
  username_label: string
  password_label: string
  submit_label: string
  /* sign-out only */
  confirm_heading?: string
  confirm_body?: string
  body?: string
  return_label?: string
}

export interface PageFeatures {
  show_realm_picker?: boolean
  show_registration?: boolean
  show_forgot_password?: boolean
  remember_me?: boolean
}

export interface PageBehavior {
  confirm?: boolean
  auto_redirect_seconds?: number
  default_return_url?: string
}

/* One field, three values — see internal/tenancy/domain/pagecfg/motion.go.
   The template emits it only inside prefers-reduced-motion: no-preference. */
export type PageEntrance = 'none' | 'fade' | 'rise'

export interface PageMotion {
  entrance?: PageEntrance
}

export interface PageLink {
  label: string
  url: string
}

export interface PageConfig {
  brand: PageBrand
  layout: PageLayout
  copy: PageCopy
  links?: PageLink[]
  features?: PageFeatures
  behavior?: PageBehavior
  motion?: PageMotion
}

export interface AuthPage {
  id: string
  kind: PageKind
  /** URL segment: /p/{tenant}/{kind}/{slug}. Immutable once published. */
  slug: string
  name: string
  status: 'active' | 'disabled'
  is_default: boolean
  /* Bound to an application OR a realm, never both — the server refuses a
     row carrying both. Resolution is slug -> application -> realm -> default,
     so a realm binding is the door that population sees. */
  application_id: string | null
  application_slug: string | null
  realm_id: string | null
  realm_code: string | null
  config: PageConfig
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
  /** running | ok | failed | dry_run — a row stuck at running is a run
      whose process died, which is worth seeing rather than hiding. */
  status: string
  added: number
  renamed: number
  moved: number
  archived: number
  unchanged: number
  /** Rows the reconciler could not place. */
  errors: number
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
