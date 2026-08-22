import { createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { ActionIcon, Button, Loader, Menu, Popover, SegmentedControl, TextInput, Tooltip, UnstyledButton } from '@mantine/core'
import { IconDots, IconInfoCircle, IconPencil, IconSearch, IconShieldLock, IconShieldPlus, IconLicense } from '@tabler/icons-react'
import { useState } from 'react'
import { useCreate } from '@/stores/create'
import { Page } from '@/components/shell/Page'
import { DataTable, Cell, type Column } from '@/components/ui/DataTable'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import type { Permission, Role } from '@/lib/api/types'

export const Route = createFileRoute('/roles')({ component: Roles })

const KIND_COLOR: Record<string, string> = {
  internal: 'var(--gold)', partner: 'var(--info)', public: 'var(--grape)', service: 'var(--ink-3)',
}
/* The count alone answered "how many" but not "which" — the question every
   reviewer actually has. Click to see the bundle, loaded on demand. */
function RolePermissions({ roleId, count }: { roleId: string; count: number }) {
  const [opened, setOpened] = useState(false)
  const { data: keys, isLoading } = useQuery({
    queryKey: qk.rolePermissions(roleId),
    queryFn: () => api.rolePermissions(roleId),
    enabled: opened,
  })
  return (
    <Popover width={300} position="left-start" opened={opened} onChange={setOpened}>
      <Popover.Target>
        <UnstyledButton onClick={() => setOpened((o) => !o)}
          className="tnum t-body underline decoration-dotted underline-offset-4"
          aria-label={`Show the ${count} permissions of this role`}>
          {count}
        </UnstyledButton>
      </Popover.Target>
      <Popover.Dropdown p="sm">
        <div className="t-label mb-2">Bundled permissions</div>
        {isLoading && <Loader size="xs" />}
        <div className="flex max-h-[260px] flex-col gap-1 overflow-y-auto">
          {keys?.map((k) => <span key={k} className="chip w-fit">{k}</span>)}
          {keys?.length === 0 && <span className="t-xs">None yet.</span>}
        </div>
      </Popover.Dropdown>
    </Popover>
  )
}

const RISK_COLOR: Record<string, string> = {
  normal: 'var(--ink-2)', sensitive: 'var(--warn)', critical: 'var(--deny)',
}

function Roles() {
  const { openCreate } = useCreate()
  const { data: roles } = useQuery({ queryKey: qk.roles(), queryFn: api.roles })
  const { data: perms } = useQuery({ queryKey: qk.permissions(), queryFn: api.permissions })
  const [q, setQ] = useState('')
  const [risk, setRisk] = useState('all')
  const needle = q.trim().toLowerCase()
  const shownRoles = (roles ?? []).filter((r) =>
    !needle || r.name.toLowerCase().includes(needle) || r.description.toLowerCase().includes(needle))
  const shownPerms = (perms ?? []).filter((p) =>
    (risk === 'all' || p.risk === risk) &&
    (!needle || p.key.toLowerCase().includes(needle)))

  const roleCols: Column<Role>[] = [
    { key: 'name', header: 'Role', render: (r) => (
        <div className="flex flex-col gap-0.5">
          <div className="flex items-center gap-2">
            <span className="t-body" style={{ fontWeight: 570 }}>{r.name}</span>
            {r.is_system && (
              <Tooltip label="Declared in an application manifest. Immutable here.">
                <span className="chip">system</span>
              </Tooltip>
            )}
          </div>
          <span className="t-xs">{r.description}</span>
        </div>
      ) },
    { key: 'realms', header: 'Grantable to', width: 210, render: (r) => (
        <div className="flex flex-wrap gap-1">
          {r.allowed_realm_kinds.map((k) => (
            <span key={k} className="chip" style={{ color: KIND_COLOR[k] }}>{k}</span>
          ))}
        </div>
      ) },
    { key: 'at', header: 'Assignable at', width: 190, render: (r) => (
        <span className="t-xs">{r.assignable_at.length ? r.assignable_at.join(', ') : 'any / none'}</span>
      ) },
    { key: 'n', header: 'Permissions', width: 100, align: 'right', render: (r) => (
        <RolePermissions roleId={r.id} count={r.permission_count} />
      ) },
    { key: 'act', header: '', width: 46, render: (r) => (
        <Menu position="bottom-end" width={260} shadow="xl">
          <Menu.Target>
            <ActionIcon variant="subtle" color="gray" aria-label={`Actions for ${r.name}`}>
              <IconDots size={15} />
            </ActionIcon>
          </Menu.Target>
          <Menu.Dropdown>
            {r.is_system ? (
              <Menu.Item disabled leftSection={<IconPencil size={14} />}>
                Declared in an application manifest — edit the manifest, not the console
              </Menu.Item>
            ) : (
              <Menu.Item leftSection={<IconPencil size={14} />}
                onClick={() => openCreate('editRole', { roleId: r.id })}>
                Edit role — add or remove permissions
              </Menu.Item>
            )}
          </Menu.Dropdown>
        </Menu>
      ) },
  ]

  const permCols: Column<Permission>[] = [
    { key: 'key', header: 'Permission', render: (p) => (
        <Cell top={<span className="font-mono" style={{ fontSize: 12 }}>{p.key}</span>}
          bottom={p.description || undefined} />
      ) },
    { key: 'risk', header: 'Risk', width: 110, render: (p) => (
        <span className="chip" style={{ color: RISK_COLOR[p.risk] }}>{p.risk}</span>
      ) },
    { key: 'ial', header: 'Min assurance', width: 130, render: (p) => (
        <span className="chip" style={{
          color: p.min_assurance >= 3 ? 'var(--allow)' : p.min_assurance === 2 ? 'var(--info)' : 'var(--warn)',
        }}>IAL{p.min_assurance}</span>
      ) },
    { key: 'stepup', header: 'Step-up', width: 210, render: (p) =>
        p.requires_amr.length ? (
          <span className="inline-flex items-center gap-1.5">
            <IconShieldLock size={11} style={{ color: 'var(--info)' }} />
            <span className="t-body">{p.requires_amr.join(', ')} within {p.max_auth_age}</span>
          </span>
        ) : <span className="t-xs">—</span> },
  ]

  return (
    <Page
      title="Roles & permissions"
      description="Roles bundle permissions; permissions belong to the application that registered them. A new permission never silently widens an existing role."
      wide
      actions={
        <>
          <TextInput w={220} placeholder="Search roles & permissions"
            leftSection={<IconSearch size={14} />}
            value={q} onChange={(e) => setQ(e.currentTarget.value)} />
          <Button size="xs" variant="default" leftSection={<IconLicense size={14} />}
            onClick={() => openCreate('permission')}>
            Add permission
          </Button>
          <Button size="xs" leftSection={<IconShieldPlus size={14} />}
            onClick={() => openCreate('role')}>
            Add role
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-5">
        <div className="panel-inset flex items-start gap-2.5 px-3.5 py-2.5">
          <IconInfoCircle size={14} style={{ color: 'var(--ink-3)', marginTop: 1, flexShrink: 0 }} />
          <div className="t-xs">
            <b style={{ color: 'var(--ink-2)' }}>allowed_realm_kinds</b> is schema-enforced: an
            employee-only role cannot be attached to a self-registered public account, even by a
            script that bypasses this console.
          </div>
        </div>

        <div>
          <div className="mb-2.5 flex items-baseline justify-between">
            <div className="t-label">Roles</div>
            <span className="t-xs tnum">{shownRoles.length} of {roles?.length ?? 0}</span>
          </div>
          <DataTable columns={roleCols} rows={shownRoles} rowKey={(r) => r.id}
            empty={{ title: needle ? 'No roles match' : 'No roles defined',
              ...(needle ? { hint: `Nothing matching “${q}”.` } : {}) }} />
        </div>

        <div>
          <div className="mb-2.5 flex items-baseline justify-between">
            <div className="flex items-center gap-3">
              <div className="t-label">Permission catalog</div>
              <SegmentedControl size="xs" value={risk} onChange={setRisk}
                data={[{ value: 'all', label: 'All' }, { value: 'normal', label: 'Normal' },
                       { value: 'sensitive', label: 'Sensitive' }, { value: 'critical', label: 'Critical' }]} />
            </div>
            <span className="t-xs tnum">{shownPerms.length} of {perms?.length ?? 0}</span>
          </div>
          <DataTable columns={permCols} rows={shownPerms} rowKey={(p) => p.id}
            empty={{ title: 'No permissions match',
              hint: 'Try another search or risk level.' }} />
        </div>
      </div>
    </Page>
  )
}
