import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { ActionIcon, Button, Menu, TextInput, Tooltip, UnstyledButton } from '@mantine/core'
import { IconDots, IconPencil, IconSearch, IconShieldPlus, IconStack2, IconUserPlus, IconUsers } from '@tabler/icons-react'
import { useMemo, useState } from 'react'
import { RoleHolders } from '@/components/access/RoleHolders'
import { GiveAccessSheet } from '@/components/access/GiveAccessSheet'
import { useCreate } from '@/stores/create'
import { Page } from '@/components/shell/Page'
import { api } from '@/lib/api/client'
import * as live from '@/lib/api/live'
import { qk } from '@/lib/query/keys'
import { groupFeatures, shortRoleName } from '@/lib/access'
import { REALM_KIND_COLOR } from '@/lib/realmKind'
import type { Permission, RealmKind, Role } from '@/lib/api/types'

/* Roles as a list with the chosen one open beside it: what it allows, who
   may be given it and how many have it, without opening a menu to find out.
   On a phone the detail follows the list. */

export const Route = createFileRoute('/roles')({
  component: Roles,
  validateSearch: (s: Record<string, unknown>): { role?: string } =>
    typeof s['role'] === 'string' && s['role'] ? { role: s['role'] } : {},
})

const KIND_WORDS: Record<RealmKind, string> = {
  internal: 'Employees', partner: 'Partner contacts', public: 'Customers and applicants', service: 'Service accounts',
}
const RISK: Record<string, { fg: string; label: string }> = {
  sensitive: { fg: 'var(--warn)', label: 'Sensitive' }, critical: { fg: 'var(--deny)', label: 'Critical' },
}
const people = (n: number | undefined) => (n === undefined ? '…' : n === 0 ? 'Nobody' : `${n} ${n === 1 ? 'person' : 'people'}`)

function appWords(r: Role) {
  return r.application_id ? `${r.application_id} app` : 'Across apps'
}

function Allows({ role, perms }: { role: Role; perms: Permission[] }) {
  const { data: keys } = useQuery({ queryKey: qk.rolePermissions(role.id), queryFn: () => api.rolePermissions(role.id) })
  if (!keys) return <div className="t-xs">Loading…</div>
  const held = perms.filter((p) => keys.includes(p.key))
  const patterns = keys.filter((k) => !perms.some((p) => p.key === k))
  if (held.length === 0 && patterns.length === 0) {
    return <div className="t-sm" style={{ color: 'var(--warn)' }}>Nothing yet. Edit the role to tick what it allows.</div>
  }
  const apps = [...new Set(held.map((p) => p.app_slug))].sort()
  return (
    <div className="flex flex-col gap-3">
      {apps.map((app) => (
        <div key={app} className="flex flex-col gap-1">
          {apps.length > 1 && <div className="t-label">{app} app</div>}
          {groupFeatures(held.filter((p) => p.app_slug === app)).flatMap((m) => m.features.flatMap((f) => f.actions)).map((p) => (
            <div key={p.id} className="flex flex-wrap items-baseline gap-x-3 gap-y-0.5 py-1" style={{ borderBottom: '1px solid var(--line-soft)' }}>
              <span className="t-body min-w-0 flex-1">{p.description || `${p.action} ${p.resource}`}</span>
              <code className="t-xs">{p.key}</code>
              <span className="t-xs w-16 text-right" style={{ color: RISK[p.risk]?.fg ?? 'var(--ink-3)' }}>{RISK[p.risk]?.label ?? 'Normal'}</span>
            </div>
          ))}
        </div>
      ))}
      {patterns.length > 0 && (
        <div className="t-xs">Also everything matching {patterns.map((k) => <code key={k} className="chip ml-1">{k}</code>)}</div>
      )}
    </div>
  )
}

function Detail({ role, count, perms, onHolders, onGive }: {
  role: Role; count: number | undefined; perms: Permission[]; onHolders: () => void; onGive: () => void
}) {
  const { openCreate } = useCreate()
  const navigate = useNavigate()
  const { data: types } = useQuery({ queryKey: qk.nodeTypes(), queryFn: api.nodeTypes })
  const limited = role.assignable_at.map((c) => types?.find((t) => t.code === c)?.display_name ?? c)
  return (
    <section className="panel flex min-w-0 flex-col gap-4 p-4 lg:sticky lg:top-4" aria-label={`Role ${role.name}`}>
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <h2 className="t-h1">{shortRoleName(role)}</h2>
          <div className="t-sm mt-0.5">{role.description || 'No description yet.'}</div>
        </div>
        <span className="flex gap-1.5">
          <Button size="xs" leftSection={<IconUserPlus size={14} />} onClick={onGive} disabled={role.deprecated}>Give to someone</Button>
          {role.is_system
            ? <Tooltip label="Comes from the app's manifest. Change it there."><Button size="xs" variant="default" disabled>Edit</Button></Tooltip>
            : <Button size="xs" variant="default" leftSection={<IconPencil size={14} />} onClick={() => openCreate('editRole', { roleId: role.id })}>Edit</Button>}
          <Menu position="bottom-end" width={240}>
            <Menu.Target><ActionIcon variant="default" size="md" aria-label="More"><IconDots size={15} /></ActionIcon></Menu.Target>
            <Menu.Dropdown>
              <Menu.Item leftSection={<IconStack2 size={14} />} onClick={() => void navigate({ to: '/bulk', search: { role: role.id } })}>
                Change their access in bulk
              </Menu.Item>
              {role.application_id && (
                <Menu.Item leftSection={<IconShieldPlus size={14} />}
                  onClick={() => void navigate({ to: '/applications/$slug', params: { slug: role.application_id! } })}>
                  Open the {role.application_id} app
                </Menu.Item>
              )}
            </Menu.Dropdown>
          </Menu>
        </span>
      </div>
      {role.deprecated && (
        <div className="t-xs" style={{ color: 'var(--warn)' }}>
          Retired: its app stopped naming it. Nobody new can be given it; everyone who has it keeps it.
        </div>
      )}
      <dl className="grid gap-x-6 gap-y-3 sm:grid-cols-3">
        <div>
          <dt className="t-xs">Who can get it</dt>
          <dd className="mt-1 flex flex-wrap gap-1">
            {role.allowed_realm_kinds.map((k) => (
              <span key={k} className="chip" style={{ color: REALM_KIND_COLOR[k] }}>{KIND_WORDS[k] ?? k}</span>
            ))}
          </dd>
        </div>
        <div>
          <dt className="t-xs">Can be limited to</dt>
          <dd className="t-body mt-1">{limited.length ? limited.join(', ') : 'Any kind of place'}</dd>
        </div>
        <div>
          <dt className="t-xs">People with it</dt>
          <dd className="mt-1">
            <UnstyledButton onClick={onHolders} className="t-body" style={{ color: 'var(--accent)' }}>{people(count)}</UnstyledButton>
          </dd>
        </div>
      </dl>
      <div>
        <div className="t-label mb-1.5">
          What it allows · {appWords(role)} · {role.permission_count} {role.permission_count === 1 ? 'permission' : 'permissions'}
        </div>
        <Allows role={role} perms={perms} />
      </div>
    </section>
  )
}

function Roles() {
  const { openCreate } = useCreate()
  const navigate = useNavigate({ from: Route.fullPath })
  const { role: picked } = Route.useSearch()
  const { data: roles } = useQuery({ queryKey: qk.roles(), queryFn: api.roles })
  const { data: perms } = useQuery({ queryKey: qk.permissions(), queryFn: api.permissions })
  const { data: groups } = useQuery({ queryKey: qk.memberships(), queryFn: api.memberships })
  const { data: usage } = useQuery({ queryKey: ['grants', 'role-usage', 'all'], queryFn: () => live.roleUsage() })
  const [q, setQ] = useState('')
  const [holdersOf, setHoldersOf] = useState<Role | null>(null)
  const [giving, setGiving] = useState<Role | null>(null)
  const needle = q.trim().toLowerCase()
  const shown = useMemo(() => (roles ?? [])
    .filter((r) => !needle || r.name.toLowerCase().includes(needle) || r.description.toLowerCase().includes(needle))
    .sort((a, b) => Number(a.deprecated) - Number(b.deprecated) || shortRoleName(a).localeCompare(shortRoleName(b))), [roles, needle])
  const count = (id: string) => (usage ? usage.find((u) => u.role_id === id)?.people ?? 0 : undefined)
  const current = shown.find((r) => r.id === picked) ?? shown[0]

  return (
    <Page
      section="roles"
      title="Roles"
      description="A role is a set of permissions. A group hands several roles to everyone in it."
      help={<>
        People are never given single permissions, always a role, and the role says where it applies when it is
        given. Changing a role changes it for everyone who has it, at once. Roles that come from an app's manifest
        are changed in the manifest.
      </>}
      wide
      actions={
        <Button size="xs" leftSection={<IconShieldPlus size={14} />} onClick={() => openCreate('role')}>New role</Button>
      }
    >
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <span className="t-sm"><b className="tnum">{roles?.length ?? '…'}</b> roles</span>
        <span className="t-xs">·</span>
        <Link to="/memberships" className="t-sm" style={{ color: 'var(--accent)' }}><span className="tnum">{groups?.length ?? '…'}</span> groups</Link>
        <span className="t-xs">·</span>
        <Link to="/permissions" className="t-sm" style={{ color: 'var(--accent)' }}><span className="tnum">{perms?.length ?? '…'}</span> permissions</Link>
      </div>
      <div className="grid items-start gap-4 lg:grid-cols-[minmax(300px,420px)_minmax(0,1fr)]">
        <div className="panel flex min-w-0 flex-col p-1.5">
          <TextInput m={6} size="xs" placeholder="Search roles" leftSection={<IconSearch size={14} />}
            value={q} onChange={(e) => setQ(e.currentTarget.value)} aria-label="Search roles" />
          {shown.length === 0 && <div className="t-sm px-3 py-6 text-center">{needle ? `No role matches “${q}”.` : 'No roles yet.'}</div>}
          <div className="flex flex-col" role="listbox" aria-label="Roles">
            {shown.map((r) => (
              <UnstyledButton key={r.id} role="option" aria-selected={current?.id === r.id}
                onClick={() => void navigate({ search: { role: r.id }, replace: true })}
                className="flex w-full min-w-0 items-center gap-3 rounded px-2.5 py-2"
                style={current?.id === r.id ? { background: 'var(--accent-bg)' } : undefined}>
                <span className="min-w-0 flex-1">
                  <span className="t-body flex items-center gap-1.5" style={{ fontWeight: 560 }}>
                    <span className="truncate">{shortRoleName(r)}</span>
                    {r.deprecated && <span className="v-pill v-pill-deny">retired</span>}
                  </span>
                  <span className="t-xs block truncate">
                    {appWords(r)} · {r.permission_count === 0 ? 'no permissions yet' : `${r.permission_count} ${r.permission_count === 1 ? 'permission' : 'permissions'}`}
                  </span>
                </span>
                <span className="t-xs tnum shrink-0 inline-flex items-center gap-1"><IconUsers size={12} />{count(r.id) ?? '…'}</span>
              </UnstyledButton>
            ))}
          </div>
        </div>
        {current && (
          <Detail role={current} count={count(current.id)} perms={perms ?? []}
            onHolders={() => setHoldersOf(current)} onGive={() => setGiving(current)} />
        )}
      </div>
      <RoleHolders role={holdersOf} onClose={() => setHoldersOf(null)} />
      <GiveAccessSheet opened={!!giving} onClose={() => setGiving(null)} pickSubject roleId={giving?.id ?? null} />
    </Page>
  )
}
