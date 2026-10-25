import { useState } from 'react'
import { Checkbox, Select, TextInput } from '@mantine/core'
import { IconSearch } from '@tabler/icons-react'
import { groupFeatures } from '@/lib/access'
import type { Application, Permission } from '@/lib/api/types'

/* Picking what someone may do without a role that already says it: an app,
   then the parts of it, then for each feature the actions. Ana may read
   invoices and nothing else there; in the CRM she may read, edit and delete.
   The same picker edits a role's permissions, so a role and a one-off pick
   are built the same way. */
export function FeaturePicker({ perms, apps, appId, onApp, value, onChange }: {
  perms: Permission[]
  apps: Pick<Application, 'id' | 'name'>[]
  /** The one app to pick from. Left out with no `onApp`, every app is listed
      under its own heading: a role may span apps, a one-off pick does not. */
  appId: string | null
  onApp?: ((id: string | null) => void) | undefined
  value: string[]
  onChange: (keys: string[]) => void
}) {
  const [q, setQ] = useState('')
  const needle = q.trim().toLowerCase()
  const live = perms.filter((p) => (!p.deprecated_at || value.includes(p.key)) && (!appId || p.application_id === appId) &&
    (!needle || p.key.toLowerCase().includes(needle) || p.module.toLowerCase().includes(needle)))
  const appIds = appId ? [appId] : onApp ? [] : [...new Set(live.map((p) => p.application_id))]
  const appName = (id: string) => apps.find((a) => a.id === id)?.name ?? live.find((p) => p.application_id === id)?.app_slug ?? id
  const on = new Set(value)
  const set = (keys: string[], yes: boolean) => {
    const next = new Set(on)
    for (const k of keys) { if (yes) next.add(k); else next.delete(k) }
    onChange([...next].sort())
  }
  return (
    <div className="flex flex-col gap-3">
      {onApp && (
        <Select label="App" placeholder="Pick an app" searchable value={appId}
          onChange={(v) => { onApp(v); onChange([]) }}
          data={apps.filter((a) => perms.some((p) => p.application_id === a.id)).map((a) => ({ value: a.id, label: a.name }))} />
      )}
      {(appIds.length > 1 || live.length > 12 || needle) && (
        <TextInput size="xs" placeholder="Find a feature" leftSection={<IconSearch size={13} />}
          value={q} onChange={(e) => setQ(e.currentTarget.value)} aria-label="Find a feature" />
      )}
      {appIds.length > 0 && live.length === 0 && <div className="t-sm">{needle ? 'Nothing matches.' : 'This app declares no permissions yet.'}</div>}
      {appIds.map((id) => {
        const mods = groupFeatures(live.filter((p) => p.application_id === id))
        return (
          <div key={id} className="flex flex-col gap-2">
            {appIds.length > 1 && <div className="t-h3">{appName(id)}</div>}
            {mods.map((m) => (
              <div key={m.module || 'other'} className="flex flex-col gap-1">
                <div className="t-label">{m.module || (mods.length > 1 ? 'Other' : 'Features')}</div>
                <div className="panel-inset flex flex-col">
                  {m.features.map((f) => {
                    const keys = f.actions.map((a) => a.key)
                    const n = keys.filter((k) => on.has(k)).length
                    return (
                      <div key={f.resource} className="flex flex-wrap items-center gap-x-3 gap-y-1.5 px-3 py-2"
                        style={{ borderBottom: '1px solid var(--line-soft)' }}>
                        <Checkbox size="xs" checked={n === keys.length} indeterminate={n > 0 && n < keys.length}
                          onChange={(e) => set(keys, e.currentTarget.checked)}
                          label={<span className="t-body" style={{ fontWeight: 550 }}>{f.resource}</span>}
                          aria-label={`Every action on ${f.resource}`} />
                        <span className="ml-auto flex flex-wrap gap-1.5">
                          {f.actions.map((a) => (
                            <Checkbox key={a.key} size="xs" checked={on.has(a.key)} label={a.action}
                              onChange={(e) => set([a.key], e.currentTarget.checked)}
                              {...(a.description ? { title: a.description } : {})}
                              {...(a.risk === 'normal' ? {} : { styles: { label: { color: a.risk === 'critical' ? 'var(--deny)' : 'var(--warn)' } } })} />
                          ))}
                        </span>
                      </div>
                    )
                  })}
                </div>
              </div>
            ))}
          </div>
        )
      })}
    </div>
  )
}
