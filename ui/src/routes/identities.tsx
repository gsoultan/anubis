import { createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { ActionIcon, Button, Menu, TextInput, Tooltip } from '@mantine/core'
import {
  IconSearch, IconInfoCircle, IconDots, IconUserPlus, IconCirclePlus,
  IconUserOff, IconUserCheck, IconCopy,
} from '@tabler/icons-react'
import { notifications } from '@mantine/notifications'
import { queryClient } from '@/lib/query/client'
import { useCreate } from '@/stores/create'
import { useState } from 'react'
import { Page } from '@/components/shell/Page'
import { DataTable, Cell, type Column } from '@/components/ui/DataTable'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { useSession } from '@/stores/session'
import type { Ial, Identity } from '@/lib/api/types'

export const Route = createFileRoute('/identities')({ component: Identities })

const IAL_HINT: Record<Ial, string> = {
  1: 'Self-asserted — email only, a self-registered applicant.',
  2: 'Remotely verified, typically through a contract or employer.',
  3: 'In-person verified with government ID on file.',
}
const IAL_COLOR: Record<Ial, string> = {
  1: 'var(--warn)', 2: 'var(--info)', 3: 'var(--allow)',
}

function Identities() {
  const { realmFilter, setRealmFilter } = useSession()
  const { openCreate } = useCreate()

  async function toggleStatus(id: string, current: string) {
    await api.setIdentityStatus(id, current === 'active' ? 'disabled' : 'active')
    notifications.show({
      color: current === 'active' ? 'orange' : 'teal',
      title: current === 'active' ? 'Identity disabled' : 'Identity re-enabled',
      message: current === 'active'
        ? 'authorize() gates on identity state — every grant is dead until re-enabled.'
        : 'Grants apply again immediately.',
    })
    await queryClient.invalidateQueries({ queryKey: ['identities'] })
  }
  const [q, setQ] = useState('')
  const { data: realms } = useQuery({ queryKey: qk.realms(), queryFn: api.realms })
  const { data: rows } = useQuery({
    queryKey: qk.identities(realmFilter, q),
    queryFn: () => api.identities(realmFilter ?? undefined, q || undefined),
  })
  const { data: categories } = useQuery({
    queryKey: qk.realmCategories(), queryFn: () => api.realmCategories(),
  })

  const realmOf = (id: string) => realms?.find((r) => r.id === id)
  const realmColour = (kind?: string) =>
    kind === 'internal' ? 'var(--gold)' : kind === 'partner' ? 'var(--info)' : 'var(--grape)'

  const columns: Column<Identity>[] = [
    { key: 'user', header: 'Identity', render: (i) => <Cell top={i.username} bottom={i.email} /> },
    { key: 'realm', header: 'Realm', width: 150, render: (i) => {
        const r = realmOf(i.realm_id)
        return (
          <span className="inline-flex items-center gap-1.5">
            <span style={{ width: 6, height: 6, borderRadius: 99, background: realmColour(r?.kind) }} />
            <span className="t-body">{r?.code}</span>
          </span>
        )
      } },
    { key: 'category', header: 'Category', width: 110, render: (i) => {
        const c = categories?.find((x) => x.id === i.category_id)
        return c
          ? <span className="chip">{c.display_name}</span>
          : <span className="t-xs">—</span>
      } },
    { key: 'ial', header: 'ID verification', width: 110, render: (i) => (
        <Tooltip label={IAL_HINT[i.assurance_level]}>
          <span className="chip" style={{ color: IAL_COLOR[i.assurance_level] }}>
            IAL{i.assurance_level}
          </span>
        </Tooltip>
      ) },
    { key: 'status', header: 'Status', width: 150, render: (i) =>
        i.status === 'active'
          ? <span className="v-pill v-pill-idle">active</span>
          : (
            <Tooltip label="authorize() gates on identity state, so this is denied regardless of grants.">
              <span className="v-pill v-pill-deny">{i.status}</span>
            </Tooltip>
          ) },
    { key: 'retention', header: 'Retention', width: 140, render: (i) =>
        i.retention_until
          ? <span className="tnum t-body">{i.retention_until.slice(0, 10)}</span>
          : <span className="t-xs">no statutory limit</span> },
    { key: 'id', header: 'ID', width: 170, render: (i) => <span className="chip">{i.id}</span> },
    { key: 'actions', header: '', width: 46, render: (i) => (
        <Menu position="bottom-end" width={230} shadow="xl">
          <Menu.Target>
            <ActionIcon variant="subtle" color="gray" aria-label={`Actions for ${i.username}`}>
              <IconDots size={15} />
            </ActionIcon>
          </Menu.Target>
          <Menu.Dropdown>
            <Menu.Item leftSection={<IconCirclePlus size={14} />}
              onClick={() => openCreate('grant', { identityId: i.id })}>
              Give access…
            </Menu.Item>
            <Menu.Item leftSection={<IconCopy size={14} />}
              onClick={() => { void navigator.clipboard.writeText(i.id) }}>
              Copy ID
            </Menu.Item>
            <Menu.Divider />
            {i.status === 'active' ? (
              <Menu.Item color="red" leftSection={<IconUserOff size={14} />}
                onClick={() => void toggleStatus(i.id, i.status)}>
                Disable — kills all access now
              </Menu.Item>
            ) : (
              <Menu.Item color="teal" leftSection={<IconUserCheck size={14} />}
                onClick={() => void toggleStatus(i.id, i.status)}>
                Re-enable
              </Menu.Item>
            )}
          </Menu.Dropdown>
        </Menu>
      ) },
  ]

  return (
    <Page
      title="People"
      description="Everyone who can sign in — employees, supplier contacts, applicants. Each lives in a realm, and usernames only need to be unique within their realm."
      wide
      actions={
        <>
          <TextInput w={230} placeholder="Search username or email"
            leftSection={<IconSearch size={14} />}
            value={q} onChange={(e) => setQ(e.currentTarget.value)} />
          <Button size="xs" leftSection={<IconUserPlus size={14} />}
            onClick={() => openCreate('identity')}>
            Add person
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <div className="flex items-center gap-1">
          {[{ id: null, label: 'All', kind: undefined }, ...(realms ?? []).map((r) => ({
            id: r.id, label: r.display_name, kind: r.kind,
          }))].map((t) => {
            const active = realmFilter === t.id
            return (
              <button key={t.id ?? 'all'} onClick={() => setRealmFilter(t.id)}
                className="flex items-center gap-1.5 rounded-md px-2.5 py-1.5"
                style={{
                  background: active ? 'var(--s-overlay)' : 'transparent',
                  border: `1px solid ${active ? 'var(--line)' : 'transparent'}`,
                  color: active ? 'var(--ink)' : 'var(--ink-3)',
                  fontSize: 12.5, fontWeight: active ? 550 : 450,
                  transition: 'all var(--t-fast)',
                }}>
                {t.kind && <span style={{ width: 6, height: 6, borderRadius: 99, background: realmColour(t.kind) }} />}
                {t.label}
              </button>
            )
          })}
        </div>

        <div className="panel-inset flex items-start gap-2.5 px-3.5 py-2.5">
          <IconInfoCircle size={14} style={{ color: 'var(--ink-3)', marginTop: 1, flexShrink: 0 }} />
          <div className="t-xs">
            <b style={{ color: 'var(--ink-2)' }}>alice</b> appears three times — once per realm.
            These are different people; linking them is explicit and never merges grants.
          </div>
        </div>

        <DataTable columns={columns} rows={rows} rowKey={(i) => i.id}
          empty={{
            title: 'No identities match',
            hint: 'Try a different realm filter or clear the search.',
            action: <Button size="xs" variant="light" onClick={() => openCreate('identity')}>Add person</Button>,
          }} />
      </div>
    </Page>
  )
}
