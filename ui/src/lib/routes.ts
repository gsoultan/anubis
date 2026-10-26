import type { RouteRule } from '@/lib/api/types'

/* The gate's matcher (internal/gate/routepath), in the console, so "Try an
   address" answers the way the gate will. Segment-wise: {name} and * take
   exactly one segment, a terminal ** takes one or more. Rules are tried top
   to bottom; the first whose host, method and path all accept wins. */

const segs = (p: string) => p.replace(/^\/+|\/+$/g, '').split('/').filter((s, i, a) => !(a.length === 1 && s === ''))

export function pathMatches(pattern: string, path: string): boolean {
  const ps = segs(pattern)
  const xs = segs(path)
  for (let i = 0; i < ps.length; i++) {
    const seg = ps[i]!
    if (seg === '**') return i === ps.length - 1 && xs.length > i
    if (i >= xs.length) return false
    if (seg === '*' || (seg.length > 1 && seg.startsWith('{') && seg.endsWith('}'))) continue
    if (seg !== xs[i]) return false
  }
  return xs.length === ps.length
}

export function methodMatches(methods: string[], m: string): boolean {
  return methods.length === 0 || methods.some((a) => a === '*' || a.toUpperCase() === m.toUpperCase())
}

/** The rule that decides a request, by its place in the list, or -1 when none does. */
export function firstMatch(rules: RouteRule[], method: string, path: string, host = ''): number {
  return rules.findIndex((r) =>
    (!r.host_pattern || r.host_pattern.toLowerCase() === host.split(':')[0]!.toLowerCase()) &&
    methodMatches(r.methods, method) && pathMatches(r.path_pattern, path))
}

/** What a rule's methods mean, in words. */
export function methodWords(methods: string[]): string {
  const m = methods.map((x) => x.toUpperCase())
  if (m.length === 0 || m.includes('*')) return 'any request'
  if (m.every((x) => x === 'GET' || x === 'HEAD')) return 'viewing'
  if (m.every((x) => x !== 'GET' && x !== 'HEAD')) return 'changes only'
  return m.join(', ')
}
