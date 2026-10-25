import { createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Button, SegmentedControl, TextInput, Tooltip } from '@mantine/core'
import { IconLicense, IconSearch, IconShieldLock } from '@tabler/icons-react'
import { useState } from 'react'
import { useCreate } from '@/stores/create'
import { Page } from '@/components/shell/Page'
import { api } from '@/lib/api/client'
import { groupFeatures } from '@/lib/access'
import { qk } from '@/lib/query/keys'
import type { Permission } from '@/lib/api/types'

export const Route = createFileRoute('/permissions')({
  validateSearch: (s: Record<string, unknown>): { app?: string } => (typeof s.app === 'string' ? { app: s.app } : {}),
  component: Permissions,
})

const RISK_COLOR: Record<string, string> = {
  normal: 'var(--ink-2)', sensitive: 'var(--warn)', critical: 'var(--deny)',
}

/* Everything an app says can be done in it, the way people look for it: the
   app, the part of the app, the feature, and then the actions on it. A flat
   table of keys answered "what is the key"; this answers "can someone be
   allowed to read invoices but not edit them", which is the question asked. */
function Action({ p }: { p: Permission }) {
  const extra = [
    p.min_assurance > 1 ? `needs assurance level ${p.min_assurance}` : '',
    p.requires_amr.length ? `asks for ${p.requires_amr.join(' or ')} again within ${p.max_auth_age}` : '',
  ].filter(Boolean)
  return (
    <Tooltip label={<span>{p.description || p.key}{extra.length > 0 && <><br />{extra.join(', ')}</>}</span>} multiline w={300}>
      <span className="chip inline-flex items-center gap-1" style={{ color: RISK_COLOR[p.risk], cursor: 'help' }}>
        {p.requires_amr.length > 0 && <IconShieldLock size={11} />}
        {p.action}
      </span>
    </Tooltip>
  )
}

function Permissions() {
  const { openCreate } = useCreate()
  const { data: perms } = useQuery({ queryKey: qk.permissions(), queryFn: api.permissions })
  const { app } = Route.useSearch()
  const navigate = Route.useNavigate()
  const [q, setQ] = useState('')
  const [risk, setRisk] = useState('all')
  const needle = q.trim().toLowerCase()
  const shown = (perms ?? []).filter((p) => !p.deprecated_at && (!app || p.app_slug === app) &&
    (risk === 'all' || p.risk === risk) &&
    (!needle || p.key.toLowerCase().includes(needle) || p.module.toLowerCase().includes(needle) ||
      p.description.toLowerCase().includes(needle)))
  const apps = [...new Set(shown.map((p) => p.app_slug))].sort()

  return (
    <Page section="roles" title="Permissions"
      description="What each app says can be done in it, by part of the app and feature. Apps declare these in their manifest; roles bundle them."
      actions={
        <>
          <TextInput w={220} placeholder="Search permissions" leftSection={<IconSearch size={14} />}
            value={q} onChange={(e) => setQ(e.currentTarget.value)} />
          <Button size="xs" variant="default" leftSection={<IconLicense size={14} />} onClick={() => openCreate('permission')}>
            Add permission
          </Button>
        </>
      }>
      <div className="flex flex-col gap-5">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <SegmentedControl size="xs" value={risk} onChange={setRisk}
            data={[{ value: 'all', label: 'All' }, { value: 'normal', label: 'Normal' },
                   { value: 'sensitive', label: 'Sensitive' }, { value: 'critical', label: 'Critical' }]} />
          <span className="flex items-center gap-2">
            {app && <Button size="compact-xs" variant="light" onClick={() => void navigate({ search: {} })}>Only {app} · show all apps</Button>}
            <span className="t-xs tnum">{shown.length} of {perms?.length ?? 0}</span>
          </span>
        </div>
        {apps.length === 0 && <div className="panel px-4 py-6 text-center t-sm">No permissions match.</div>}
        {apps.map((slug) => {
          const mods = groupFeatures(shown.filter((p) => p.app_slug === slug))
          return (
            <section key={slug} className="panel flex flex-col gap-3 p-4">
              <h2 className="t-h2">{slug}</h2>
              {mods.map((m) => (
                <div key={m.module || 'other'} className="flex flex-col gap-1">
                  <div className="t-label">{m.module || (mods.length > 1 ? 'Other' : 'Features')}</div>
                  <div className="panel-inset flex flex-col">
                    {m.features.map((f) => (
                      <div key={f.resource} className="flex flex-wrap items-center gap-x-3 gap-y-1.5 px-3 py-2"
                        style={{ borderBottom: '1px solid var(--line-soft)' }}>
                        <span className="t-body min-w-[140px] flex-1" style={{ fontWeight: 550 }}>{f.resource}</span>
                        <span className="flex flex-wrap gap-1.5">{f.actions.map((p) => <Action key={p.id} p={p} />)}</span>
                      </div>
                    ))}
                  </div>
                </div>
              ))}
            </section>
          )
        })}
      </div>
    </Page>
  )
}
