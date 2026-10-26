import { useState } from 'react'
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Button, Tabs, Tooltip } from '@mantine/core'
import { IconPencil, IconShieldPlus, IconTestPipe, IconUsers } from '@tabler/icons-react'
import { Page } from '@/components/shell/Page'
import { Initial } from '@/components/ui/Initial'
import { RoleHolders } from '@/components/access/RoleHolders'
import { AppModules } from '@/components/apps/AppModules'
import { AppWho, useAppRoles } from '@/components/apps/AppWho'
import { PageRules } from '@/components/apps/PageRules'
import { AppConnection, kindLabel } from '@/components/apps/AppConnection'
import { ManifestDialog } from '@/components/apps/ManifestDialog'
import { useCreate } from '@/stores/create'
import { api } from '@/lib/api/client'
import { shortRoleName } from '@/lib/access'
import { qk } from '@/lib/query/keys'
import * as live from '@/lib/api/live'
import type { Role } from '@/lib/api/types'

/* One app on one page: who can use it, the roles and features it brings, the
   rules for its addresses, and how it is wired up. The list of apps only
   points here, so everything about an app is changed in one place. */

const TABS = ['who', 'roles', 'features', 'rules', 'connection', 'sync'] as const
type Tab = (typeof TABS)[number]

export const Route = createFileRoute('/applications_/$slug')({
  component: AppPage,
  validateSearch: (s: Record<string, unknown>): { tab?: Tab } =>
    TABS.includes(s['tab'] as Tab) && s['tab'] !== 'who' ? { tab: s['tab'] as Tab } : {},
})

function AppRoles({ app }: { app: live.AppRecord }) {
  const { openCreate } = useCreate()
  const { roles, count } = useAppRoles(app.slug)
  const [holdersOf, setHoldersOf] = useState<Role | null>(null)
  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center justify-between gap-2">
        <p className="t-sm">Each role is a set of things people can do in {app.name}. People get a role, never single permissions.</p>
        <Button size="xs" leftSection={<IconShieldPlus size={14} />} onClick={() => openCreate('role')}>Add role</Button>
      </div>
      {roles.length === 0 && <div className="panel px-4 py-6 text-center t-sm">{app.name} has no roles yet.</div>}
      <div className="flex flex-col gap-2">
        {roles.map((r) => (
          <div key={r.id} className="panel flex flex-wrap items-center gap-x-4 gap-y-2 px-4 py-3">
            <span className="min-w-[200px] flex-1">
              <span className="t-body block" style={{ fontWeight: 570 }}>{shortRoleName(r)}</span>
              <span className="t-xs block">{r.description || 'No description'}</span>
            </span>
            <span className="t-sm tnum">{r.permission_count} permissions</span>
            <span className="t-sm tnum">{count(r.id)} people</span>
            <span className="flex gap-1">
              <Button size="compact-xs" variant="default" leftSection={<IconUsers size={13} />} onClick={() => setHoldersOf(r)}>People</Button>
              {r.is_system
                ? <Tooltip label="Comes from the app's manifest. Change it there."><span className="chip">from manifest</span></Tooltip>
                : <Button size="compact-xs" variant="default" leftSection={<IconPencil size={13} />}
                    onClick={() => openCreate('editRole', { roleId: r.id })}>Edit</Button>}
            </span>
          </div>
        ))}
      </div>
      <RoleHolders role={holdersOf} onClose={() => setHoldersOf(null)} />
    </div>
  )
}

function AppFeatures({ app }: { app: live.AppRecord }) {
  const { data: perms } = useQuery({ queryKey: qk.permissions(), queryFn: api.permissions })
  const mine = (perms ?? []).filter((p) => p.app_slug === app.slug && !p.deprecated_at)
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
        <p className="t-sm" style={{ maxWidth: 620 }}>
          {app.name} is split into modules. Each module has features, and each feature has the things people can do
          in it. Those are the permissions you give out through roles. The app declares them in its manifest.
        </p>
        <ManifestDialog slug={app.slug} onDone={() => undefined} />
      </div>
      <AppModules perms={mine} />
    </div>
  )
}

function AppSync({ app }: { app: live.AppRecord }) {
  return (
    <section className="panel flex flex-col gap-2 p-4">
      <h2 className="t-h2">Keep {app.name} in step with its own list</h2>
      <p className="t-sm" style={{ maxWidth: 620 }}>
        Instead of pasting the manifest by hand, Anubis can read it from an address on a schedule, check what would
        change, and apply it. Roles and permissions the app stops naming are retired, never deleted, so nobody loses
        access mid-day.
      </p>
      <div className="flex gap-2">
        <Button component={Link} to="/catalog" size="xs" variant="default">Set up a scheduled read</Button>
        <ManifestDialog slug={app.slug} onDone={() => undefined} />
      </div>
      <div className="t-xs">Last applied manifest: {app.manifest_version > 0 ? `version ${app.manifest_version}` : 'none yet'}</div>
    </section>
  )
}

function AppPage() {
  const { slug } = Route.useParams()
  const { tab = 'who' } = Route.useSearch()
  const navigate = useNavigate({ from: Route.fullPath })
  const { data: app, isLoading } = useQuery({
    queryKey: ['applications', 'slug', slug],
    queryFn: async () => (await live.applications({ query: slug, pageSize: 50 })).rows.find((a) => a.slug === slug) ?? null,
  })
  const { roles } = useAppRoles(app?.slug ?? '')
  const { data: perms } = useQuery({ queryKey: qk.permissions(), queryFn: api.permissions })
  const { data: rules } = useQuery({
    queryKey: ['route-rules', slug], queryFn: () => live.routeRules(slug), enabled: !!app, retry: false,
  })
  const { data: usage } = useQuery({
    queryKey: ['grants', 'app-usage', app?.id], queryFn: () => live.appUsage(app!.id), enabled: !!app,
  })

  if (!app) {
    return (
      <Page title={isLoading ? 'Loading…' : 'No such app'} back={{ to: '/applications', label: 'Apps' }}>
        {!isLoading && <div className="panel p-4 t-sm">There is no app called {slug} in this organisation.</div>}
      </Page>
    )
  }

  const nPerms = (perms ?? []).filter((p) => p.app_slug === app.slug && !p.deprecated_at).length
  const modules = new Set((perms ?? []).filter((p) => p.app_slug === app.slug).map((p) => p.module || '')).size
  const live_ = app.status === 'active'
  const count = (n: number | undefined) => n !== undefined && <span className="chip ml-1.5 tnum">{n}</span>

  return (
    <Page
      title={app.name}
      back={{ to: '/applications', label: 'Apps' }}
      lead={<Initial name={app.name} colour="var(--accent)" size={40} />}
      badge={<span className={`v-pill ${live_ ? 'v-pill-allow' : 'v-pill-deny'}`}>{live_ ? 'Live' : 'Switched off'}</span>}
      description={<>{kindLabel(app.kind)}{app.redirect_uris[0] ? ` · ${new URL(app.redirect_uris[0], 'https://x').host}` : ''}
        {usage ? ` · ${usage.people} people can use it` : ''}</>}
      actions={
        <Button size="xs" variant="default" leftSection={<IconTestPipe size={14} />}
          onClick={() => void navigate({ to: '/playground' })}>
          Check someone's access
        </Button>
      }
      wide
    >
      <Tabs value={tab} onChange={(v) => void navigate({ search: v && v !== 'who' ? { tab: v as Tab } : {} })} keepMounted={false} className="min-w-0">
        <Tabs.List mb="md" style={{ overflowX: 'auto', flexWrap: 'nowrap' }}>
          <Tabs.Tab value="who">Who can use it</Tabs.Tab>
          <Tabs.Tab value="roles">Roles{count(roles.length)}</Tabs.Tab>
          <Tabs.Tab value="features">Modules and features{nPerms > 0 && <span className="chip ml-1.5 tnum">{modules} · {nPerms}</span>}</Tabs.Tab>
          <Tabs.Tab value="rules">Page rules{count(rules?.length)}</Tabs.Tab>
          <Tabs.Tab value="connection">Connection</Tabs.Tab>
          <Tabs.Tab value="sync">Sync</Tabs.Tab>
        </Tabs.List>
        <Tabs.Panel value="who"><AppWho app={app} /></Tabs.Panel>
        <Tabs.Panel value="roles"><AppRoles app={app} /></Tabs.Panel>
        <Tabs.Panel value="features"><AppFeatures app={app} /></Tabs.Panel>
        <Tabs.Panel value="rules"><PageRules app={app} /></Tabs.Panel>
        <Tabs.Panel value="connection"><AppConnection app={app} /></Tabs.Panel>
        <Tabs.Panel value="sync"><AppSync app={app} /></Tabs.Panel>
      </Tabs>
    </Page>
  )
}
