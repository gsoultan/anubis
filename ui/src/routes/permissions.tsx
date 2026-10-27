import { createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Button, SegmentedControl, TextInput } from '@mantine/core'
import { IconLicense, IconSearch } from '@tabler/icons-react'
import { useState } from 'react'
import { useCreate } from '@/stores/create'
import { Page } from '@/components/shell/Page'
import { api } from '@/lib/api/client'
import { FeatureList } from '@/components/access/FeatureList'
import { qk } from '@/lib/query/keys'

export const Route = createFileRoute('/permissions')({
  validateSearch: (s: Record<string, unknown>): { app?: string } => (typeof s.app === 'string' ? { app: s.app } : {}),
  component: Permissions,
})

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
          return (
            <section key={slug} className="panel flex flex-col gap-3 p-4">
              <h2 className="t-h2">{slug}</h2>
              <FeatureList perms={shown.filter((p) => p.app_slug === slug)} />
            </section>
          )
        })}
      </div>
    </Page>
  )
}
