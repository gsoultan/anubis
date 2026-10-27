import { useState } from 'react'
import { Checkbox, Select, Switch, TextInput, UnstyledButton } from '@mantine/core'
import { IconChevronRight, IconSearch } from '@tabler/icons-react'
import { groupFeatures } from '@/lib/access'
import type { Permission } from '@/lib/api/types'

/* Picking what someone may do without a role that already says it: an app,
   then the parts of it, then for each feature the actions. Ana may read
   invoices and nothing else there; in the CRM she may read, edit and delete.
   The same picker edits a role's permissions, so a role and a one-off pick
   are built the same way.

   Each module folds, with "6 of 11" on its line, so a long app reads as its
   modules first. A module, a feature or one action can be ticked. Ticking a
   module takes the actions it has today: later ones are not added by
   themselves, which the footnote says. */

const label = (p: Permission) => p.description || p.action
const RISK: Record<string, string> = { sensitive: 'var(--warn)', critical: 'var(--deny)' }

export function FeaturePicker({ perms, apps, appId, onApp, value, onChange }: {
  perms: Permission[]
  /** Apps by slug, the name a permission is filed under. */
  apps: { slug: string; name: string }[]
  /** The one app to pick from, by slug. Left out with no `onApp`, every app
      is listed under its own heading: a role may span apps, a one-off pick
      does not. */
  appId: string | null
  onApp?: ((slug: string | null) => void) | undefined
  value: string[]
  onChange: (keys: string[]) => void
}) {
  const [q, setQ] = useState('')
  const [onlyTicked, setOnlyTicked] = useState(false)
  const [openMods, setOpenMods] = useState<Record<string, boolean>>({})
  const on = new Set(value)
  const needle = q.trim().toLowerCase()
  const inApp = perms.filter((p) => (!p.deprecated_at || on.has(p.key)) && (!appId || p.app_slug === appId))
  const live = inApp.filter((p) => (!onlyTicked || on.has(p.key)) &&
    (!needle || p.key.toLowerCase().includes(needle) || p.module.toLowerCase().includes(needle) || p.description.toLowerCase().includes(needle)))
  const appIds = appId ? [appId] : onApp ? [] : [...new Set(live.map((p) => p.app_slug))].sort()
  const appName = (slug: string) => apps.find((a) => a.slug === slug)?.name ?? slug
  const set = (keys: string[], yes: boolean) => {
    const next = new Set(on)
    for (const k of keys) { if (yes) next.add(k); else next.delete(k) }
    onChange([...next].sort())
  }

  const ticked = inApp.filter((p) => on.has(p.key))
  const tickedMods = new Set(ticked.map((p) => `${p.app_slug}|${p.module}`)).size
  const tickedRisky = ticked.filter((p) => p.risk !== 'normal').length

  return (
    <div className="flex flex-col gap-3">
      {onApp && (
        <Select label="App" placeholder="Pick an app" searchable value={appId}
          onChange={(v) => { onApp(v); onChange([]) }}
          data={apps.filter((a) => perms.some((p) => p.app_slug === a.slug)).map((a) => ({ value: a.slug, label: a.name }))} />
      )}
      {(appIds.length > 0 || needle || onlyTicked) && (
        <div className="flex flex-wrap items-center gap-3">
          <TextInput size="xs" className="min-w-[180px] flex-1" placeholder="Search modules, features or actions"
            leftSection={<IconSearch size={13} />} value={q} onChange={(e) => setQ(e.currentTarget.value)} aria-label="Find a feature" />
          <Switch size="xs" label="Show only ticked" checked={onlyTicked} onChange={(e) => setOnlyTicked(e.currentTarget.checked)} />
        </div>
      )}
      {appIds.length > 0 && live.length === 0 && (
        <div className="t-sm">{needle || onlyTicked ? 'Nothing matches.' : 'This app declares no permissions yet.'}</div>
      )}
      {appIds.map((app) => {
        const mods = groupFeatures(live.filter((p) => p.app_slug === app))
        return (
          <div key={app} className="flex flex-col gap-1.5">
            {appIds.length > 1 && <div className="t-h3">{appName(app)}</div>}
            {mods.map((m) => {
              const id = `${app}|${m.module}`
              const all = m.features.flatMap((f) => f.actions.map((a) => a.key))
              const n = all.filter((k) => on.has(k)).length
              /* Open when searching or showing ticks, or when it holds a tick:
                 an edit should land on what the role already allows. */
              const open = openMods[id] ?? (!!needle || onlyTicked || n > 0 || (appIds.length === 1 && mods.length === 1))
              return (
                <div key={id} className="panel-inset flex flex-col">
                  <div className="flex items-center gap-2 px-3 py-2">
                    <Checkbox size="xs" checked={n === all.length && n > 0} indeterminate={n > 0 && n < all.length}
                      onChange={(e) => set(all, e.currentTarget.checked)}
                      aria-label={`Every action in ${m.module || 'this app'}`} />
                    <UnstyledButton className="flex flex-1 items-center gap-1.5" aria-expanded={open}
                      onClick={() => setOpenMods((o) => ({ ...o, [id]: !open }))}>
                      <IconChevronRight size={13} style={{ transform: open ? 'rotate(90deg)' : undefined, transition: 'transform var(--t-fast)' }} />
                      <span className="t-body" style={{ fontWeight: 580 }}>{m.module || (mods.length > 1 ? 'Other' : 'Features')}</span>
                      <span className="t-xs tnum ml-auto">{n} of {all.length}</span>
                    </UnstyledButton>
                  </div>
                  {open && m.features.map((f) => {
                    const keys = f.actions.map((a) => a.key)
                    const k = keys.filter((x) => on.has(x)).length
                    return (
                      <div key={f.resource} className="flex flex-wrap items-center gap-x-3 gap-y-1.5 py-2 pl-9 pr-3"
                        style={{ borderTop: '1px solid var(--line-soft)' }}>
                        <Checkbox size="xs" checked={k === keys.length} indeterminate={k > 0 && k < keys.length}
                          onChange={(e) => set(keys, e.currentTarget.checked)}
                          label={<span className="t-body" style={{ fontWeight: 550 }}>{f.resource}</span>}
                          aria-label={`Every action on ${f.resource}`} />
                        <span className="ml-auto flex flex-wrap gap-x-3 gap-y-1.5">
                          {f.actions.map((a) => (
                            <Checkbox key={a.key} size="xs" checked={on.has(a.key)}
                              label={<span>{label(a)}{a.risk !== 'normal' && <span className="t-xs ml-1" style={{ color: RISK[a.risk] }}>{a.risk}</span>}</span>}
                              onChange={(e) => set([a.key], e.currentTarget.checked)} title={a.key} />
                          ))}
                        </span>
                      </div>
                    )
                  })}
                </div>
              )
            })}
          </div>
        )
      })}
      {appIds.length > 0 && (
        <div className="flex flex-col gap-1">
          <div className="t-sm" style={{ fontWeight: 520 }}>
            {ticked.length === 0 ? 'Nothing ticked yet.'
              : `${ticked.length} ${ticked.length === 1 ? 'action' : 'actions'} in ${tickedMods} ${tickedMods === 1 ? 'module' : 'modules'}, ${tickedRisky === 0 ? 'none sensitive' : `${tickedRisky} sensitive`}.`}
          </div>
          <div className="t-xs">Ticking a whole module gives only the actions it has today. Actions added to it later are not given automatically.</div>
        </div>
      )}
    </div>
  )
}
