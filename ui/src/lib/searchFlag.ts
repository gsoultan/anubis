/* A yes/no flag in the query string.

   TanStack Router parses search values as JSON, so `?give=1` arrives as the
   NUMBER 1 and `?give=true` as the boolean — while a link written by hand, a
   runbook, or another tool may carry either as a string. The person page
   compared against true and the STRING '1' only, so a hand-typed `?give=1`
   opened nothing and said nothing. */
export function searchFlag(v: unknown): boolean {
  return v === true || v === 1 || v === '1' || v === 'true'
}
