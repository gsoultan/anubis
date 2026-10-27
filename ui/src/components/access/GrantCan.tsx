import { useQuery } from '@tanstack/react-query'
import { IconCheck } from '@tabler/icons-react'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { axisReach, groupFeatures } from '@/lib/access'
import { useAxisRoots } from './GrantWhere'
import type { GrantScope, Permission, ScopeAxis } from '@/lib/api/types'

/** What one role lets someone do, feature by feature, with the feature's
    other actions shown greyed so "can see but not edit" reads at a glance. */
export function useRoleCan(roleId: string) {
  const { data: keys } = useQuery({ queryKey: qk.rolePermissions(roleId), queryFn: () => api.rolePermissions(roleId) })
  const { data: perms } = useQuery({ queryKey: qk.permissions(), queryFn: api.permissions })
  const held = new Set(keys ?? [])
  const mine = (perms ?? []).filter((p) => held.has(p.key))
  const features = new Set(mine.map((p) => `${p.app_slug}|${p.resource}`))
  /* The whole of each feature the role touches, so what it leaves out shows. */
  const shown = (perms ?? []).filter((p) => features.has(`${p.app_slug}|${p.resource}`) && (!p.deprecated_at || held.has(p.key)))
  const apps = [...new Set(shown.map((p) => p.app_slug))].sort()
  return {
    loading: keys === undefined || perms === undefined, held, mine,
    apps: apps.map((app) => ({ app, modules: groupFeatures(shown.filter((p) => p.app_slug === app)) })),
  }
}

const label = (p: Permission) => p.description || p.action

export function GrantCan({ roleId }: { roleId: string }) {
  const { loading, held, apps } = useRoleCan(roleId)
  if (loading) return <div className="t-xs">Loading…</div>
  if (apps.length === 0) return <div className="t-xs" style={{ color: 'var(--warn)' }}>Nothing: this role permits nothing yet.</div>
  return (
    <div className="flex flex-col gap-1.5">
      {apps.flatMap(({ app, modules }) => modules.flatMap((m) => m.features.map((f) => (
        <div key={`${app}|${m.module}|${f.resource}`} className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
          <span className="t-xs min-w-[110px]">{[apps.length > 1 && app, m.module, f.resource].filter(Boolean).join(' › ')}</span>
          <span className="flex flex-wrap gap-1">
            {f.actions.map((p) => {
              const yes = held.has(p.key)
              return (
                <span key={p.id} className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 t-xs"
                  aria-label={`${label(p)}: ${yes ? 'yes' : 'no'}`}
                  style={yes
                    ? { background: 'var(--allow-bg)', color: 'var(--ink)', fontWeight: 540 }
                    : { color: 'var(--ink-4)', textDecoration: 'line-through' }}>
                  {yes && <IconCheck size={11} style={{ color: 'var(--allow)' }} />}{label(p)}
                </span>
              )
            })}
          </span>
        </div>
      ))))}
    </div>
  )
}

const joinOr = (xs: string[]) => (xs.length <= 2 ? xs.join(' or ') : `${xs.slice(0, -1).join(', ')} or ${xs[xs.length - 1]}`)

/** The grant in one sentence: what, and the places that limit it, each
    limit AND'ed with the others because that is how authorize() reads them. */
export function GrantSentence({ who, roleId, scopes, selfScoped, axes, nodeName }: {
  who: string; roleId: string; scopes: GrantScope[]; selfScoped: boolean
  axes: ScopeAxis[] | undefined; nodeName: (id: string) => string
}) {
  const { mine } = useRoleCan(roleId)
  const rootOf = useAxisRoots(axes)
  if (mine.length === 0) return null
  const multi = new Set(mine.map((p) => p.app_slug)).size > 1
  const byFeature = new Map<string, Set<string>>()
  for (const p of mine) {
    const k = multi ? `${p.resource} in ${p.app_slug}` : p.resource
    byFeature.set(k, (byFeature.get(k) ?? new Set()).add(p.action))
  }
  const and = (xs: string[]) => (xs.length <= 1 ? xs.join('') : `${xs.slice(0, -1).join(', ')} and ${xs[xs.length - 1]}`)
  const what = [...byFeature].map(([r, acts]) => `${and([...acts].sort())} ${r}`)
  const limits = (axes ?? []).flatMap((a) => {
    const r = axisReach(scopes, a, rootOf(a.code))
    if (r.kind === 'all') return []
    if (r.kind === 'none') return [`never, because nothing is picked under ${a.display_name}`]
    const names = r.include.map((s) => nodeName(s.scope_node_id))
    const except = r.exclude.length ? ` (not ${r.exclude.map((s) => nodeName(s.scope_node_id)).join(' or ')})` : ''
    return [`in ${joinOr(names)}${except}`]
  })
  const where = selfScoped ? 'only on records they own'
    : limits.length === 0 ? 'everywhere, including places added later'
    : `only when the record is ${limits.join(' and ')}`
  return (
    <p className="t-sm" style={{ color: 'var(--ink-2)' }}>
      {who} can {what.join('; ')}, {where}.
    </p>
  )
}
