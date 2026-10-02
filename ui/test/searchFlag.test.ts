/* `?give=1` on a person's page must open the give-access sheet.

   Root cause of the bug this pins: TanStack Router parses search values as
   JSON, so a hand-typed `?give=1` reaches validateSearch as the NUMBER 1, and
   the check compared it against true and the STRING '1' only — the link
   opened the page and silently left the sheet shut. */
import { describe, expect, test } from 'bun:test'
import { searchFlag } from '@/lib/searchFlag'

describe('searchFlag', () => {
  test('a typed ?give=1 arrives as the number 1, and opens it', () => {
    expect(searchFlag(1)).toBe(true)
  })
  test('the forms links produce open it too', () => {
    expect(searchFlag(true)).toBe(true)
    expect(searchFlag('1')).toBe(true)
    expect(searchFlag('true')).toBe(true)
  })
  test('anything else leaves it shut', () => {
    for (const v of [0, false, '0', 'false', 'yes', '', undefined, null, {}]) {
      expect(searchFlag(v)).toBe(false)
    }
  })
})
