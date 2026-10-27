import { describe, expect, test } from 'bun:test'
import { firstMatch, methodWords, pathMatches } from '@/lib/routes'
import type { RouteRule } from '@/lib/api/types'

const rule = (path_pattern: string, methods: string[], effect: RouteRule['effect']): RouteRule =>
  ({ priority: 0, path_pattern, host_pattern: '', methods, effect, permission: '', scope_bindings: {} })

describe('page rules match like the gate', () => {
  test('{name} and * take one segment, a terminal ** one or more', () => {
    expect(pathMatches('/invoices/{id}', '/invoices/42')).toBe(true)
    expect(pathMatches('/invoices/{id}', '/invoices/42/approve')).toBe(false)
    expect(pathMatches('/invoices/*/approve', '/invoices/42/approve')).toBe(true)
    expect(pathMatches('/static/**', '/static/a/b.css')).toBe(true)
    expect(pathMatches('/static/**', '/static')).toBe(false)
    expect(pathMatches('/health', '/health')).toBe(true)
  })
  test('the first rule that accepts decides', () => {
    const rules = [rule('/invoices/*/approve', ['POST'], 'require_permission'), rule('/invoices/**', [], 'require_auth')]
    expect(firstMatch(rules, 'POST', '/invoices/1/approve')).toBe(0)
    expect(firstMatch(rules, 'GET', '/invoices/1/approve')).toBe(1)
    expect(firstMatch(rules, 'GET', '/payments')).toBe(-1)
  })
  test('methods read as what they are for', () => {
    expect(methodWords(['GET'])).toBe('viewing')
    expect(methodWords(['POST', 'PUT'])).toBe('changes only')
    expect(methodWords([])).toBe('any request')
  })
})
