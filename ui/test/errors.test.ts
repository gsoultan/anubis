import { describe, expect, test } from 'bun:test'
import { sayRejection } from '../src/lib/errors'

describe('sayRejection', () => {
  test('a guard’s own sentence wins over everything around it', () => {
    expect(sayRejection(
      'invalid_argument: Invalid argument (reason=x): storm: check constraint violated',
      { reason: 'items of level "org_division" already sit under a "org_company"; move them before removing that rule' },
    )).toBe('Items of level "org_division" already sit under a "org_company"; move them before removing that rule.')
  })
  test('a named field is said as the field', () => {
    expect(sayRejection('invalid_argument: Invalid argument (valid_until=must be in the future)',
      { valid_until: 'must be in the future' })).toBe('valid until: must be in the future')
  })
  test('with no details, the message loses its code and its wrapped cause', () => {
    expect(sayRejection('not_found: Not found: storm: no rows', {})).toBe('Not found.')
    expect(sayRejection('', {})).toBe('The request was refused.')
  })
})
