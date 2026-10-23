import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { ActionIcon, Button, Loader, Menu, Popover, TextInput, Tooltip, UnstyledButton } from '@mantine/core'
import { IconDots, IconInfoCircle, IconPencil, IconSearch, IconShieldPlus, IconStack2, IconUsers } from '@tabler/icons-react'
import { RoleHolders } from '@/components/access/RoleHolders'
import { useState } from 'react'
import { useCreate } from '@/stores/create'
import { Page } from '@/components/shell/Page'
import { DataTable, type Column } from '@/components/ui/DataTable'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { REALM_KIND_COLOR } from '@/lib/realmKind'
import type { Role } from '@/lib/api/types'

export const Route = createFileRoute('/roles')({ component: Roles })

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

function Roles() {
  const { openCreate } = useCreate()
  const navigate = useNavigate()
  const { data: roles } = useQuery({ queryKey: qk.roles(), queryFn: api.roles })
  const [q, setQ] = useState('')
  const [holdersOf, setHoldersOf] = useState<Role | null>(null)
  const needle = q.trim().toLowerCase()
  const shownRoles = (roles ?? []).filter((r) =>
    !needle || r.name.toLowerCase().includes(needle) || r.description.toLowerCase().includes(needle))
  /* Every column carries an explicit width. Leave one without and it becomes
     the only column the browser can grow, so all the slack on a wide display
     pools into it — the role column ran 670px for 120px of name while
     "grantable to" and "permissions" were squeezed against the right edge. */
  const roleCols: Column<Role>[] = [
    { key: 'name', header: 'Role', width: 420, render: (r) => (
        <div className="flex flex-col gap-0.5">
          <div className="flex items-center gap-2">
            <span className="t-body" style={{ fontWeight: 570 }}>{r.name}</span>
            {r.is_system && (
              <Tooltip label="Declared in an application manifest. Immutable here.">
                <span className="chip">system</span>
              </Tooltip>
            )}
            {/* The pill is spent on the exception: almost every role is live,
                and the one that is not is what somebody is scanning for. */}
            {r.deprecated && (
              <Tooltip
                label="Retired: its manifest stopped naming it. It cannot be granted to anybody new — every grant that already has it keeps working."
                withArrow
              >
                <span className="v-pill v-pill-deny" style={{ cursor: 'help' }}>retired</span>
              </Tooltip>
            )}
          </div>
          <span className="t-xs">{r.description}</span>
        </div>
      ) },
    { key: 'realms', header: 'Grantable to', width: 210, render: (r) => (
        <div className="flex flex-wrap gap-1">
          {r.allowed_realm_kinds.map((k) => (
            <span key={k} className="chip" style={{ color: REALM_KIND_COLOR[k] }}>{k}</span>
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
            <Menu.Item leftSection={<IconUsers size={14} />} onClick={() => setHoldersOf(r)}>
              People with this role
            </Menu.Item>
            <Menu.Item leftSection={<IconStack2 size={14} />}
              onClick={() => void navigate({ to: '/bulk', search: { role: r.id } })}>
              Change their access in bulk
            </Menu.Item>
            <Menu.Divider />
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

  return (
    <Page
      section="roles"
      title="Roles"
      description="A role is a bundle of permissions with a name. A new permission never silently widens an existing role."
      wide
      actions={
        <>
          <TextInput w={220} placeholder="Search roles"
            leftSection={<IconSearch size={14} />}
            value={q} onChange={(e) => setQ(e.currentTarget.value)} />
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

      </div>
      <RoleHolders role={holdersOf} onClose={() => setHoldersOf(null)} />
    </Page>
  )
}
