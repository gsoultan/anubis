import { createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { ActionIcon, Button, Menu, SegmentedControl, TextInput, Tooltip } from '@mantine/core'
import { notifications } from '@mantine/notifications'
import { IconInfoCircle, IconLock, IconDots, IconCirclePlus, IconSearch, IconTrash } from '@tabler/icons-react'
import { useState } from 'react'
import { queryClient } from '@/lib/query/client'
import { useCreate } from '@/stores/create'
import { Page } from '@/components/shell/Page'
import { DataTable, Cell, type Column } from '@/components/ui/DataTable'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import type { Grant } from '@/lib/api/types'

/** A search hit carries the holder's name, so the table needs no per-row lookup. */
type GrantRow = Grant & { username: string }

export const Route = createFileRoute('/grants')({ component: Grants })

function Grants() {
  const { openCreate } = useCreate()
  const [q, setQ] = useState('')
  const [source, setSource] = useState('all')
  /* Search-first, not a listing: this tenant holds 150,000 grants, so the
     screen narrows on the server and pages through what is left rather than
     pulling a list it could never render. */
  const [trail, setTrail] = useState<string[]>([''])
  const cursor = trail[trail.length - 1] ?? ''
  const { data: page } = useQuery({
    queryKey: ['grant-search', q, source, cursor],
    queryFn: () => api.searchGrants({
      query: q.trim(),
      source: source === 'all' ? '' : source,
      cursor,
      pageSize: 50,
    }),
    placeholderData: (prev) => prev,
  })
  const grants = page?.rows
  const { data: memberships } = useQuery({ queryKey: qk.memberships(), queryFn: api.memberships })
  // The server applied the filters; re-doing it here would only hide rows
  // it deliberately returned.
  const shown = grants ?? []

  async function revoke(id: string, who: string, role: string) {
    await api.revokeGrant(id)
    notifications.show({
      color: 'orange', title: 'Grant revoked',
      message: `${who} no longer holds ${role}. Access tokens die at their TTL (≤15 min).`,
    })
    await queryClient.invalidateQueries({ queryKey: ['grants'] })
    await queryClient.invalidateQueries({ queryKey: qk.dashboard() })
  }
  const { data: identities } = useQuery({ queryKey: qk.identities(), queryFn: () => api.identities() })
  const { data: axes } = useQuery({ queryKey: qk.axes(), queryFn: api.axes })
  const { data: nodes } = useQuery({
    queryKey: ['all-nodes'],
    queryFn: async () => {
      const all = await Promise.all((await api.axes()).map((a) => api.scopeSearch(a.code, '')))
      return all.flat()
    },
  })
  const nodeName = (id: string) => nodes?.find((n) => n.id === id)?.name ?? id

  const columns: Column<GrantRow>[] = [
    { key: 'who', header: 'Identity', width: 190, render: (g) => (
        <Cell top={g.username || '—'}
          bottom={<span className="chip">{g.identity_id}</span>} />
      ) },
    { key: 'role', header: 'Role', width: 190, render: (g) => (
        <div className="flex flex-col gap-1">
          <span className="t-body" style={{ fontWeight: 550 }}>{g.role_name}</span>
          {g.via_membership_id && (
            <Tooltip label="Derived from a membership — manage it there, not here.">
              <span className="chip w-fit" style={{ color: 'var(--gold)', borderColor: 'var(--gold-chip-line)', background: 'var(--gold-chip-bg)' }}>
                via {memberships?.find((x) => x.id === g.via_membership_id)?.name ?? 'membership'}
              </span>
            </Tooltip>
          )}
          {g.self_scoped && (
            <Tooltip label="Applies only to records this identity owns. The caller must supply _owner or the decision is denied.">
              <span className="chip w-fit" style={{ color: 'var(--info)', borderColor: 'color-mix(in srgb, var(--info) 20%, transparent)' }}>
                <IconLock size={9} style={{ marginRight: 4 }} />self-scoped
              </span>
            </Tooltip>
          )}
        </div>
      ) },
    { key: 'scopes', header: 'Where', render: (g) => {
        const silent = (axes ?? []).filter((a) => !g.scopes.some((s) => s.axis_code === a.code))
        const byAxis = new Map<string, typeof g.scopes>()
        for (const s of g.scopes) {
          const list = byAxis.get(s.axis_code) ?? []
          list.push(s); byAxis.set(s.axis_code, list)
        }
        return (
          <div className="flex flex-col gap-1.5">
            {g.scopes.length === 0 && !g.self_scoped && (
              <span className="t-xs">everywhere — no limits</span>
            )}
            {[...byAxis].map(([axisCode, list]) => (
              <div key={axisCode} className="flex items-start gap-2">
                <span className="chip" style={{ minWidth: 62, justifyContent: 'center', marginTop: 1 }}>
                  {axisCode}
                </span>
                <span className="t-body min-w-0">
                  {list.map((s, i) => (
                    <span key={s.scope_node_id}>
                      {i > 0 && <span className="t-xs" style={{ margin: '0 5px', fontStyle: 'italic' }}>or</span>}
                      {nodeName(s.scope_node_id)}
                      {!s.inherit && (
                        <Tooltip label="Exactly this place — nothing inside it.">
                          <span className="chip" style={{ marginLeft: 4, color: 'var(--warn)',
                            borderColor: 'color-mix(in srgb, var(--warn) 20%, transparent)' }}>exact</span>
                        </Tooltip>
                      )}
                    </span>
                  ))}
                </span>
              </div>
            ))}
            {silent.length > 0 && g.scopes.length > 0 && (
              <span className="t-xs" style={{ fontSize: 10 }}>
                no limit on {silent.map((a) => a.code).join(', ')}
              </span>
            )}
          </div>
        )
      } },
    { key: 'validity', header: 'Validity', width: 130, render: (g) => (
        <Cell top={<span className="tnum">{g.valid_from.slice(0, 10)}</span>}
          bottom={g.valid_until ? `until ${g.valid_until.slice(0, 10)}` : 'no expiry'} />
      ) },
    { key: 'actions', header: '', width: 46, render: (g) => (
        <Menu position="bottom-end" width={250} shadow="xl">
          <Menu.Target>
            <ActionIcon variant="subtle" color="gray" aria-label="Grant actions">
              <IconDots size={15} />
            </ActionIcon>
          </Menu.Target>
          <Menu.Dropdown>
            {g.via_membership_id ? (
              <Menu.Item disabled leftSection={<IconTrash size={14} />}>
                Managed by “{memberships?.find((x) => x.id === g.via_membership_id)?.name}” —
                remove the person there
              </Menu.Item>
            ) : (
              <>
                <Menu.Label>This revokes access, not the identity</Menu.Label>
                <Menu.Item color="red" leftSection={<IconTrash size={14} />}
                  onClick={() => void revoke(g.id,
                    identities?.find((i) => i.id === g.identity_id)?.username ?? 'identity',
                    g.role_name)}>
                  Revoke grant
                </Menu.Item>
              </>
            )}
          </Menu.Dropdown>
        </Menu>
      ) },
  ]

  return (
    <Page
      title="Access"
      description="Who can do what, and where. A grant ties a person to a role, optionally limited per axis — all limits must match at once."
      wide
      actions={
        <>
          <TextInput w={210} placeholder="Search person or role"
            leftSection={<IconSearch size={14} />}
            value={q} onChange={(e) => { setQ(e.currentTarget.value); setTrail(['']) }} />
          <SegmentedControl size="xs" value={source}
            onChange={(v) => { setSource(v); setTrail(['']) }}
            data={[{ value: 'all', label: 'All' }, { value: 'direct', label: 'Direct' },
                   { value: 'membership', label: 'Via membership' }]} />
          <Button size="xs" leftSection={<IconCirclePlus size={14} />}
            onClick={() => openCreate('grant')}>
            Give access
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <div className="panel-inset flex items-start gap-2.5 px-3.5 py-2.5">
          <IconInfoCircle size={14} style={{ color: 'var(--ink-3)', marginTop: 1, flexShrink: 0 }} />
          <div className="t-xs">
            <b style={{ color: 'var(--ink-2)' }}>inherit</b> is per-axis, not per-grant — “everything
            under Jakarta, but only Rigid Packaging itself and not its SKUs” is expressible.
            Self-scoped grants cannot carry axis constraints; the database rejects the combination.
          </div>
        </div>
        <div className="t-xs tnum" style={{ alignSelf: 'flex-end' }}>
          {shown.length} on this page
        </div>
        <DataTable columns={columns} rows={shown} rowKey={(g) => g.id}
          empty={{
            title: q.trim() || source !== 'all' ? 'No access matches' : 'Nobody has access yet',
            hint: q.trim() || source !== 'all'
              ? 'Try another search or source filter.'
              : 'Grants connect an identity to a role within a scope.',
            action: <Button size="xs" variant="light" onClick={() => openCreate('grant')}>Give access</Button>,
          }} />
        {(trail.length > 1 || page?.next) && (
          <div className="mt-3 flex items-center gap-2">
            <Button variant="default" size="compact-sm" disabled={trail.length <= 1}
              onClick={() => setTrail((t) => t.slice(0, -1))}>Previous</Button>
            <Button variant="default" size="compact-sm" disabled={!page?.next}
              onClick={() => setTrail((t) => [...t, page?.next ?? ''])}>Next</Button>
          </div>
        )}
      </div>
    </Page>
  )
}
