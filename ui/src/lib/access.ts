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
