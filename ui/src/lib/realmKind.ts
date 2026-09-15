import type { RealmKind } from '@/lib/api/types'

/* One mapping, because three screens were each carrying their own and two of
   them had only three of the four kinds — a `service` population came out the
   same colour as a `public` one, which is the difference between a robot and
   the general public. */
export const REALM_KIND_COLOR: Record<RealmKind, string> = {
  internal: 'var(--kind-internal)',
  partner:  'var(--kind-partner)',
  public:   'var(--kind-public)',
  service:  'var(--kind-service)',
}

export function realmKindColor(kind?: string): string {
  return REALM_KIND_COLOR[kind as RealmKind] ?? 'var(--ink-3)'
}
