import type { GrantScope, Role, ScopeAxis } from '@/lib/api/types'

/* How access reads, in one place: which structures a grant must name, and the
   dates it runs between. The sheet that gives access and the list that shows
   it both answer "what can this person do, and until when" — computed twice,
   the two would disagree about the same grant.

   Pure on purpose: no React, no API client, so `bun test` can hold these to
   the rules authorize() applies without a browser or a server. */

/** One chosen place: included or carved out, with or without what is inside. */
export type NodeSel = { id: string; name: string; inherit: boolean; exclude: boolean }
/** Chosen places, by structure (axis code). */
export type Places = Record<string, NodeSel[]>

/** An exclusion with nothing to carve out of — the database refuses it (0046). */
export function axisNeedsAnInclude(values: NodeSel[]): boolean {
  return values.length > 0 && values.every((v) => v.exclude)
}

export function placesToScopes(places: Places): GrantScope[] {
  return Object.entries(places).flatMap(([axis_code, list]) =>
    list.map((c) => ({ axis_code, scope_node_id: c.id, inherit: c.inherit, exclude: c.exclude })))
}

/** Why a role cannot go to this person, or '' when it can. Retirement comes
    first because it is absolute: grants_role_live (0044) refuses the insert
    whoever the subject is. */
export function roleBlocked(r: Role, kind: string | undefined): string {
  if (r.deprecated) return 'Retired from the catalog'
  if (kind && !r.allowed_realm_kinds.includes(kind as Role['allowed_realm_kinds'][number]))
    return `Not available to ${kind} accounts`
  return ''
}

export type Until = '' | '30' | '90' | '365' | 'date'

/** The last moment of a calendar day, local time: "until 27 Dec" means all of it. */
function endOfDay(d: Date): Date {
  const e = new Date(d)
  e.setHours(23, 59, 59, 0)
  return e
}

/** When a grant ends: never (''), N days from `now`, or the picked
    `YYYY-MM-DD` — each to the end of that day. */
export function untilDate(until: Until, picked: string | null, now = Date.now()): Date | null {
  if (until === '') return null
  if (until === 'date') return picked ? endOfDay(new Date(`${picked}T00:00:00`)) : null
  return endOfDay(new Date(now + Number(until) * 86_400_000))
}

/** Structures every grant must name a place in (`default_effect = 'deny'`).
    authorize() drops a grant that is silent on any of them, so a grant without
    one does nothing at all, however broad it looks. */
export function requiredAxes(axes: ScopeAxis[] | undefined): ScopeAxis[] {
  return (axes ?? []).filter((a) => a.default_effect === 'deny' && a.status === 'active')
}

/** The required structures a grant names no place in. Non-empty means the
    grant never applies. A self-scoped grant carries no places at all, so any
    required structure silences it too — the same NOT EXISTS authorize() runs. */
export function missingRequired(scopes: GrantScope[], axes: ScopeAxis[] | undefined): ScopeAxis[] {
  return requiredAxes(axes).filter((a) => !scopes.some((s) => s.axis_code === a.code))
}

export function namesOf(axes: ScopeAxis[]): string {
  const n = axes.map((a) => a.display_name)
  return n.length <= 1 ? (n[0] ?? '') : `${n.slice(0, -1).join(', ')} and ${n[n.length - 1]}`
}

const DAY = 86_400_000
/* Day, short month, year: never ambiguous about which number is the month,
   which a bare 03/04/2026 is to half of any operations team. */
const DATE = new Intl.DateTimeFormat('en-GB', { day: 'numeric', month: 'short', year: 'numeric' })

export function fmtDate(at: string | Date): string {
  return DATE.format(new Date(at))
}

function midnight(t: number): number {
  const d = new Date(t)
  d.setHours(0, 0, 0, 0)
  return d.getTime()
}

/** Calendar days from today to `at`; negative in the past. Calendar days, not
    elapsed hours: a grant ending at 09:00 tomorrow ends "tomorrow", even at
    23:00 tonight. */
export function daysFrom(at: string | Date, now = Date.now()): number {
  return Math.round((midnight(new Date(at).getTime()) - midnight(now)) / DAY)
}

const plural = (n: number, unit: string) => `${n} ${unit}${n === 1 ? '' : 's'}`

export function relDays(at: string | Date, now = Date.now()): string {
  const d = daysFrom(at, now)
  if (d === 0) return 'today'
  if (d === 1) return 'tomorrow'
  if (d === -1) return 'yesterday'
  const n = Math.abs(d)
  const span = n < 60 ? plural(n, 'day')
    : n < 730 ? plural(Math.round(n / 30.44), 'month')
    : plural(Math.round(n / 365.25), 'year')
  return d > 0 ? `in ${span}` : `${span} ago`
}

/** Up to two letters for an avatar: `john.doe` is JD, `user1` is U. */
export function initials(name: string): string {
  const parts = name.split(/[^\p{L}\p{N}]+/u).filter(Boolean)
  if (parts.length >= 2) return `${parts[0]!.charAt(0)}${parts[1]!.charAt(0)}`.toUpperCase()
  return name.charAt(0).toUpperCase()
}

/* ---------------------------------------------------------------------------
   Everything, or only these (0057's console half)

   Every kind of place on a grant is one of two things, and the console says
   which: ∞ Everything, where no list is kept and a place added tomorrow is
   covered the moment it exists; or ☰ Only these, a list that never changes
   on its own. They look the same on the day access is given. They stop
   looking the same the day something new is added.
   --------------------------------------------------------------------------- */

export type AxisReach =
  /** No list kept: every place of this kind, new ones included. */
  | { kind: 'all' }
  /** A list: these places (each with or without what is inside), less any exceptions. */
  | { kind: 'list'; include: GrantScope[]; exclude: GrantScope[] }
  /** A required kind the grant names nothing in: the grant never applies. */
  | { kind: 'none' }

/** What a grant reaches on one kind of place. `rootId` is the top of that
    kind: a pick of the top "and everything inside" is Everything written the
    only way a required kind allows. */
export function axisReach(scopes: GrantScope[], axis: ScopeAxis, rootId?: string): AxisReach {
  const mine = scopes.filter((s) => s.axis_code === axis.code)
  if (mine.length === 0) return axis.default_effect === 'deny' ? { kind: 'none' } : { kind: 'all' }
  const include = mine.filter((s) => !s.exclude)
  const exclude = mine.filter((s) => s.exclude)
  if (exclude.length === 0 && include.length === 1 && include[0]!.inherit && include[0]!.scope_node_id === rootId)
    return { kind: 'all' }
  return { kind: 'list', include, exclude }
}

/** A place as the nudge needs it: where it sits, and how many it holds. */
export type PlaceFacts = { id: string; name: string; parent_id: string | null; child_count: number; is_axis_root: boolean }

/** Parents every one of whose children is in a hand-picked list. A list like
    that reads as "all of them" today and will miss the next one, so the sheet
    offers the version that grows. Only parents with two or more children:
    picking the one child of something is a choice, not an accident. */
export function fullParents(picked: NodeSel[], facts: (id: string) => PlaceFacts | undefined): PlaceFacts[] {
  const byParent = new Map<string, Set<string>>()
  for (const p of picked) {
    if (p.exclude) continue
    const f = facts(p.id)
    if (!f?.parent_id) continue
    byParent.set(f.parent_id, (byParent.get(f.parent_id) ?? new Set()).add(p.id))
  }
  const out: PlaceFacts[] = []
  for (const [parentId, kids] of byParent) {
    const parent = facts(parentId)
    if (parent && parent.child_count >= 2 && kids.size >= parent.child_count) out.push(parent)
  }
  return out
}

/** Replaces every pick under `parent` with the parent "and everything inside":
    the list that grows. Exceptions under it are kept. */
export function growInto(picked: NodeSel[], parent: PlaceFacts, facts: (id: string) => PlaceFacts | undefined): NodeSel[] {
  const kept = picked.filter((p) => p.exclude || facts(p.id)?.parent_id !== parent.id)
  return [...kept.filter((p) => p.id !== parent.id), { id: parent.id, name: parent.name, inherit: true, exclude: false }]
}

/** One kind of place, read back in words. `noun` is the plural of the kind,
    lower case: "workplaces". */
export function reachWords(r: AxisReach, noun: string, name: (id: string) => string): string {
  if (r.kind === 'all') return `every one of the ${noun}, including new ones`
  if (r.kind === 'none') return `no ${noun} (so this access never applies)`
  const names = r.include.map((s) => name(s.scope_node_id) + (s.inherit ? ' and everything inside' : ''))
  const list = names.length <= 2 ? names.join(' or ') : `${names.slice(0, -1).join(', ')} or ${names[names.length - 1]}`
  const except = r.exclude.length > 0 ? `, except ${r.exclude.map((s) => name(s.scope_node_id)).join(' and ')}` : ''
  return `exactly ${r.include.length === 1 ? 'this one' : `these ${r.include.length}`}: ${list}${except}`
}

/** The role, of those given, whose permissions are exactly these — so
    picking features reuses a role rather than minting a twin. */
export function roleWithExactly<T extends { id: string }>(keys: string[], roles: T[], permsOf: (r: T) => string[] | undefined): T | undefined {
  const want = [...new Set(keys)].sort().join('\n')
  return roles.find((r) => {
    const have = permsOf(r)
    return have !== undefined && [...new Set(have)].sort().join('\n') === want
  })
}

/** One feature of an app: a resource, and the actions on it. */
export type Feature<P> = { resource: string; actions: P[] }
/** A part of an app, as its manifest files it, with its features. */
export type Module<P> = { module: string; features: Feature<P>[] }

/** Permissions as people pick them: module, then feature, then action.
    Permissions the manifest files under no module come last, as "Other". */
export function groupFeatures<P extends { module: string; resource: string; action: string }>(perms: P[]): Module<P>[] {
  const mods = new Map<string, Map<string, P[]>>()
  for (const p of perms) {
    const m = mods.get(p.module) ?? new Map<string, P[]>()
    m.set(p.resource, [...(m.get(p.resource) ?? []), p])
    mods.set(p.module, m)
  }
  return [...mods]
    .sort(([a], [b]) => (a === '' ? 1 : b === '' ? -1 : a.localeCompare(b)))
    .map(([module, res]) => ({
      module,
      features: [...res].sort(([a], [b]) => a.localeCompare(b))
        .map(([resource, actions]) => ({ resource, actions: [...actions].sort((x, y) => x.action.localeCompare(y.action)) })),
    }))
}

/** A name for a role made from picked features, short enough for a list:
    "Invoices: invoice (read), customer (read, edit)". */
export function featureRoleName<P extends { resource: string; action: string }>(app: string, picked: P[]): string {
  const by = new Map<string, string[]>()
  for (const p of picked) by.set(p.resource, [...(by.get(p.resource) ?? []), p.action])
  const parts = [...by].sort(([a], [b]) => a.localeCompare(b)).map(([r, a]) => `${r} (${a.sort().join(', ')})`)
  const name = `${app}: ${parts.join(', ')}`
  return name.length <= 100 ? name : `${name.slice(0, 97)}…`
}

/** A role's name where its app is already plain: a manifest names its roles
    `<app>.<name>`, which reads as noise on that app's own page. */
export function shortRoleName(r: { name: string; application_id: string | null }): string {
  const prefix = r.application_id ? `${r.application_id}.` : ''
  return prefix && r.name.startsWith(prefix) ? r.name.slice(prefix.length) : r.name
}
