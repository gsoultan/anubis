import { describe, expect, test } from 'bun:test'
import { levelCode, topDown } from '../src/lib/levels'
import type { ScopeNodeType } from '../src/lib/api/types'

const CODE = /^[a-z][a-z0-9_]{1,30}$/ // scope_node_types_code_check

describe('levelCode', () => {
  test('is prefixed with the structure, so one name can exist in two', () => {
    expect(levelCode('org', 'Division', new Set())).toBe('org_division')
    expect(levelCode('cost_center', 'Division', new Set())).toBe('cost_center_division')
  })
  test('always satisfies the database check, whatever the name', () => {
    for (const name of ['Division', '  Sous-unité №3 ', '東京', '', 'x'.repeat(80), '9 to 5']) {
      for (const axis of ['org', 'a_structure_with_a_31_char_code']) {
        expect(levelCode(axis, name, new Set())).toMatch(CODE)
      }
    }
  })
  test('never returns a code that is taken, and stays within the limit', () => {
    const taken = new Set<string>()
    for (let i = 0; i < 12; i++) {
      const code = levelCode('a_structure_with_a_31_char_code', 'Department', taken)
      expect(taken.has(code)).toBe(false)
      expect(code).toMatch(CODE)
      taken.add(code)
    }
  })
})

describe('topDown', () => {
  const lv = (code: string, parents: string[]): ScopeNodeType =>
    ({ code, axis_code: 'org', display_name: code, parent_types: parents })
  test('lists the top level first and each level after what it sits under', () => {
    const order = topDown([lv('unit', ['company', 'unit']), lv('office', ['unit']), lv('company', ['top', 'company']), lv('top', [])])
      .map((t) => t.code)
    expect(order).toEqual(['top', 'company', 'unit', 'office'])
  })
  test('returns every level even when two name each other', () => {
    expect(topDown([lv('a', ['b']), lv('b', ['a'])])).toHaveLength(2)
  })
})
