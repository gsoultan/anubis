/* The rules the give-access sheet and a person's access list apply before the
   server does. Each of these is a sentence an operator reads as the truth
   about somebody's access, so each is held to what authorize() actually does
   (migrations/0046_scope_exclusions.sql) rather than to what reads well.

   Run: bun test (in ui/). No browser, no server — lib/access.ts is pure. */
import { describe, expect, test } from 'bun:test'
import {
  axisNeedsAnInclude, axisReach, daysFrom, fullParents, growInto, initials, missingRequired,
  namesOf, placesToScopes, reachWords, relDays, requiredAxes, roleBlocked, roleWithExactly,
  untilDate, groupFeatures, featureRoleName, type NodeSel, type PlaceFacts,
} from '@/lib/access'
import type { GrantScope, Role, ScopeAxis } from '@/lib/api/types'

const axis = (code: string, strict = false, status: ScopeAxis['status'] = 'active'): ScopeAxis => ({
  code,
  display_name: code.charAt(0).toUpperCase() + code.slice(1),
  default_effect: strict ? 'deny' : 'unconstrained',
  status,
  sort_order: 0,
  resolution: { from: 'context', key: code },
  ui_schema: {},
})
const at = (axis_code: string, id: string, exclude = false, inherit = true): GrantScope =>
  ({ axis_code, scope_node_id: id, inherit, exclude })

describe('required structures mirror authorize()', () => {
  const axes = [axis('org'), axis('region', true), axis('legacy', true, 'deprecated')]

  test('only active structures with default_effect = deny are required', () => {
    expect(requiredAxes(axes).map((a) => a.code)).toEqual(['region'])
  })
  test('a grant naming a place in every required structure applies', () => {
    expect(missingRequired([at('region', 'jkt')], axes)).toEqual([])
    expect(missingRequired([at('org', 'fin'), at('region', 'jkt')], axes)).toEqual([])
  })
  test('a grant silent on a required structure never applies, however narrow it looks', () => {
    expect(missingRequired([at('org', 'fin')], axes).map((a) => a.code)).toEqual(['region'])
  })
  test('"everywhere" and self-scoped grants carry no places, so any required structure silences them', () => {
    expect(missingRequired([], axes).map((a) => a.code)).toEqual(['region'])
  })
  test('with no required structure, nothing is missing', () => {
    expect(missingRequired([], [axis('org'), axis('product')])).toEqual([])
    expect(requiredAxes(undefined)).toEqual([])
  })
})

describe('chosen places become scopes', () => {
  test('flattened per structure, each keeping its mode', () => {
    expect(placesToScopes({
      org: [
        { id: 'jkt', name: 'Jakarta', inherit: true, exclude: false },
        { id: 'sby', name: 'Surabaya', inherit: false, exclude: true },
      ],
      product: [{ id: 'pkg', name: 'Packaging', inherit: false, exclude: false }],
    })).toEqual([
      at('org', 'jkt'), at('org', 'sby', true, false), at('product', 'pkg', false, false),
    ])
  })
  test('an exception with nothing to carve out of is refused (0046)', () => {
    const except = { id: 'sby', name: 'Surabaya', inherit: true, exclude: true }
    const include = { id: 'jkt', name: 'Jakarta', inherit: true, exclude: false }
    expect(axisNeedsAnInclude([except])).toBe(true)
    expect(axisNeedsAnInclude([include, except])).toBe(false)
    expect(axisNeedsAnInclude([])).toBe(false)
  })
})

describe('roles a person cannot be given', () => {
  const role = (over: Partial<Role>): Role => ({
    id: 'r', name: 'Finance Approver', description: '', application_id: null, is_system: false,
    allowed_realm_kinds: ['internal'], assignable_at: [], permission_count: 0, deprecated: false,
    ...over,
  })
  test('retired comes first — it is refused whoever the person is', () => {
    expect(roleBlocked(role({ deprecated: true, allowed_realm_kinds: [] }), 'partner'))
      .toBe('Retired from the catalog')
  })
  test('a population the role does not serve is named', () => {
    expect(roleBlocked(role({}), 'partner')).toBe('Not available to partner accounts')
  })
  test('otherwise it can be given', () => {
    expect(roleBlocked(role({}), 'internal')).toBe('')
    expect(roleBlocked(role({}), undefined)).toBe('')
  })
})

describe('end dates run to the end of the day', () => {
  const now = new Date(2026, 8, 28, 10, 30).getTime() // 28 Sep 2026, 10:30 local

  test('no end date is no date', () => {
    expect(untilDate('', null, now)).toBeNull()
    expect(untilDate('date', null, now)).toBeNull()
  })
  test('a preset ends at 23:59:59 on the Nth day', () => {
    const d = untilDate('30', null, now)!
    expect([d.getFullYear(), d.getMonth(), d.getDate()]).toEqual([2026, 9, 28])
    expect([d.getHours(), d.getMinutes(), d.getSeconds()]).toEqual([23, 59, 59])
  })
  test('a picked day ends at 23:59:59 on that day', () => {
    const d = untilDate('date', '2026-12-27', now)!
    expect([d.getFullYear(), d.getMonth(), d.getDate(), d.getHours()]).toEqual([2026, 11, 27, 23])
  })
})

describe('relative days are calendar days, not elapsed hours', () => {
  const late = new Date(2026, 8, 28, 23, 0).getTime() // 23:00

  test('09:00 tomorrow is "tomorrow", even at 23:00 tonight', () => {
    expect(daysFrom(new Date(2026, 8, 29, 9, 0), late)).toBe(1)
    expect(relDays(new Date(2026, 8, 29, 9, 0), late)).toBe('tomorrow')
  })
  test('both directions, in the unit a person would say', () => {
    expect(relDays(new Date(2026, 8, 28, 1, 0), late)).toBe('today')
    expect(relDays(new Date(2026, 8, 27, 12, 0), late)).toBe('yesterday')
    expect(relDays(new Date(2026, 9, 12, 12, 0), late)).toBe('in 14 days')
    expect(relDays(new Date(2026, 11, 27, 12, 0), late)).toBe('in 3 months')
    expect(relDays(new Date(2023, 8, 28, 12, 0), late)).toBe('3 years ago')
  })
})

describe('small words', () => {
  test('initials: two when the name has two parts, one otherwise', () => {
    expect(initials('john.doe')).toBe('JD')
    expect(initials('user1')).toBe('U')
    expect(initials('élodie_durand')).toBe('ÉD')
  })
  test('a list of structures reads as a sentence', () => {
    expect(namesOf([axis('region')])).toBe('Region')
    expect(namesOf([axis('region'), axis('org'), axis('product')])).toBe('Region, Org and Product')
    expect(namesOf([])).toBe('')
  })
})

describe('Everything, or only these', () => {
  const org = axis('org')
  const region = axis('region', true)
  test('no picks on a kind is Everything, unless the kind is required', () => {
    expect(axisReach([], org).kind).toBe('all')
    expect(axisReach([], region).kind).toBe('none')
  })
  test('the top of a kind "and everything inside" is Everything too', () => {
    expect(axisReach([at('region', 'top')], region, 'top').kind).toBe('all')
    expect(axisReach([at('region', 'top', false, false)], region, 'top').kind).toBe('list')
  })
  test('anything else is a list, with its exceptions apart', () => {
    const r = axisReach([at('org', 'a'), at('org', 'b', true)], org, 'top')
    expect(r.kind).toBe('list')
    if (r.kind === 'list') {
      expect(r.include.map((s) => s.scope_node_id)).toEqual(['a'])
      expect(r.exclude.map((s) => s.scope_node_id)).toEqual(['b'])
    }
  })

  const facts: Record<string, PlaceFacts> = {
    top: { id: 'top', name: 'Top', parent_id: null, child_count: 2, is_axis_root: true },
    sum: { id: 'sum', name: 'Sumatra', parent_id: 'top', child_count: 3, is_axis_root: false },
    jav: { id: 'jav', name: 'Java', parent_id: 'top', child_count: 1, is_axis_root: false },
    med: { id: 'med', name: 'Medan', parent_id: 'sum', child_count: 0, is_axis_root: false },
    pad: { id: 'pad', name: 'Padang', parent_id: 'sum', child_count: 0, is_axis_root: false },
    pek: { id: 'pek', name: 'Pekanbaru', parent_id: 'sum', child_count: 0, is_axis_root: false },
    jkt: { id: 'jkt', name: 'Jakarta', parent_id: 'jav', child_count: 0, is_axis_root: false },
  }
  const f = (id: string) => facts[id]
  const sel = (id: string, exclude = false): NodeSel => ({ id, name: facts[id]!.name, inherit: true, exclude })

  test('a list naming every child of a parent is found', () => {
    expect(fullParents([sel('med'), sel('pad'), sel('pek'), sel('jkt')], f).map((p) => p.id)).toEqual(['sum'])
  })
  test('the only child of something is a choice, not a full list', () => {
    expect(fullParents([sel('jkt')], f)).toEqual([])
  })
  test('a partial list is not full, and exceptions do not count', () => {
    expect(fullParents([sel('med'), sel('pad')], f)).toEqual([])
    expect(fullParents([sel('med'), sel('pad'), sel('pek', true)], f)).toEqual([])
  })
  test('growing into the parent replaces its children and keeps the rest', () => {
    const next = growInto([sel('med'), sel('pad'), sel('pek'), sel('jkt')], facts['sum']!, f)
    expect(next.map((p) => p.id).sort()).toEqual(['jkt', 'sum'])
    expect(next.find((p) => p.id === 'sum')?.inherit).toBe(true)
  })
  test('the read-back says which kind it is', () => {
    const name = (id: string) => facts[id]?.name ?? id
    expect(reachWords({ kind: 'all' }, 'workplaces', name)).toContain('including new ones')
    const list = axisReach([at('org', 'jkt', false, false), at('org', 'med', false, false)], org)
    expect(reachWords(list, 'workplaces', name)).toBe('exactly these 2: Jakarta or Medan')
  })
  test('picking features reuses the role with exactly those permissions', () => {
    const roles = [{ id: 'a', p: ['x', 'y'] }, { id: 'b', p: ['x'] }]
    expect(roleWithExactly(['x'], roles, (r) => r.p)?.id).toBe('b')
    expect(roleWithExactly(['y', 'x'], roles, (r) => r.p)?.id).toBe('a')
    expect(roleWithExactly(['z'], roles, (r) => r.p)).toBeUndefined()
  })
})

describe('features', () => {
  const p = (module: string, resource: string, action: string) => ({ module, resource, action })
  test('grouped by module, then feature, with unfiled ones last', () => {
    const g = groupFeatures([p('', 'misc', 'read'), p('Sales', 'invoice', 'read'), p('Sales', 'invoice', 'edit'), p('CRM', 'lead', 'read')])
    expect(g.map((m) => m.module)).toEqual(['CRM', 'Sales', ''])
    expect(g[1]!.features[0]!.actions.map((a) => a.action)).toEqual(['edit', 'read'])
  })
  test('a role made from features is named after them', () => {
    expect(featureRoleName('Invoices', [p('', 'invoice', 'read'), p('', 'customer', 'read'), p('', 'customer', 'edit')]))
      .toBe('Invoices: customer (edit, read), invoice (read)')
  })
})
