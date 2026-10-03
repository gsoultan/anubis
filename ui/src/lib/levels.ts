import type { ScopeNodeType } from '@/lib/api/types'

/* The rules about a structure's levels that the console applies before the
   server does. Plain functions, so ui/test can hold them to their word. */

/** A level's code. The code is unique across every structure of a tenant,
    so "Division" in two of them cannot both be `division` — the second one
    used to fail with a conflict nobody could explain. Prefixed with the
    structure, and made unique against every level the tenant has. Nobody
    reads it: pickers and rows show the level's name. */
export function levelCode(axis: string, name: string, taken: ReadonlySet<string>): string {
  const slug = name.toLowerCase().trim().replace(/[^a-z0-9]+/g, '_').replace(/^_|_$/g, '') || 'level'
  const base = `${axis}_${slug}`.slice(0, 31).replace(/_+$/, '')
  let code = base
  for (let n = 2; taken.has(code); n++) {
    const tail = `_${n}`
    code = base.slice(0, 31 - tail.length).replace(/_+$/, '') + tail
  }
  return code
}

/** Top first, then each level after the levels it sits under. A level that
    sits inside itself does not wait for itself. */
export function topDown(levels: ScopeNodeType[]): ScopeNodeType[] {
  const out: ScopeNodeType[] = []
  const left = [...levels].sort((a, b) => a.display_name.localeCompare(b.display_name))
  while (left.length > 0) {
    const i = left.findIndex((t) =>
      t.parent_types.every((p) => p === t.code || out.some((o) => o.code === p)))
    out.push(...left.splice(i < 0 ? 0 : i, 1)) // levels that name each other: take them as they come
  }
  return out
}
