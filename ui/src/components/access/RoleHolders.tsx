import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { ActionIcon, Button, Menu, Modal, SegmentedControl } from '@mantine/core'
import { IconChevronRight, IconDots, IconInfinity, IconStack2, IconUser } from '@tabler/icons-react'
import { api } from '@/lib/api/client'
import * as live from '@/lib/api/live'
import { qk } from '@/lib/query/keys'
import { axisReach, fmtDate, shortRoleName } from '@/lib/access'
import { DataTable, type Column } from '@/components/ui/DataTable'
import { GrantWhere, useAxisRoots } from './GrantWhere'
import type { Grant, Role } from '@/lib/api/types'

type Holder = Grant & { username: string }

/* Who holds a role — the one question the old Access list answered that a
   person's page cannot, because it starts from the role. It lives on the role
   now. Paged on the server like every list here that can be long: a role in
   this installation can be held tens of thousands of times. */
export function RoleHolders({ role, onClose }: { role: Role | null; onClose: () => void }) {
  const navigate = useNavigate()
  /* Keyed by role, so switching roles starts again at page one rather than
     asking the new role for the old role's page-three cursor. */
  const [trails, setTrails] = useState<Record<string, string[]>>({})
  const roleId = role?.id ?? ''
  const trail = trails[roleId] ?? ['']
  const setTrail = (t: string[]) => setTrails((all) => ({ ...all, [roleId]: t }))
  const cursor = trail[trail.length - 1] ?? ''

  const { data: page, isFetching } = useQuery({
    /* Under 'grants', so giving or revoking anywhere refreshes it. */
    queryKey: ['grants', 'role', roleId, cursor],
    queryFn: () => api.searchGrants({ roleId, cursor, pageSize: 25 }),
    enabled: !!role,
    /* Keep the previous page on screen while the next loads — but never
       another role's holders under this role's name. */
    placeholderData: (prev, prevQuery) => (prevQuery?.queryKey[2] === roleId ? prev : undefined),
  })
  const rows = page?.rows
  const { data: axes } = useQuery({ queryKey: qk.axes(), queryFn: api.axes, enabled: !!role })
  const { data: memberships } = useQuery({
    queryKey: qk.memberships(), queryFn: api.memberships, enabled: !!role,
  })
  const ids = [...new Set((rows ?? []).flatMap((g) => g.scopes.map((s) => s.scope_node_id)))].sort()
  const { data: nodes } = useQuery({
    queryKey: ['scope-names', ids],
    queryFn: () => api.scopeNodesByIds(ids),
    enabled: ids.length > 0,
  })
  const nodeName = (id: string) => nodes?.find((n) => n.id === id)?.name ?? '…'

  /* One kind of place at a time: how many holders' access on it grows by
     itself, grows inside a branch, or is a fixed list someone has to keep. */
  const [axisPick, setAxisPick] = useState<string | null>(null)
  const axis = (axes ?? []).find((a) => a.code === axisPick) ?? axes?.[0]
  const rootOf = useAxisRoots(axes)
  const { data: kinds } = useQuery({
    queryKey: ['grants', 'holder-kinds', roleId, axis?.code],
    queryFn: () => live.holderKinds(roleId, axis!.code),
    enabled: !!role && !!axis,
  })
  const noun = (axis?.display_name ?? 'place').toLowerCase()
  const badge = (g: Holder) => {
    if (!axis || g.self_scoped) return null
    const r = axisReach(g.scopes, axis, rootOf(axis.code))
    if (r.kind === 'all') return <span className="v-pill v-pill-allow">↻ Every {noun}</span>
    if (r.kind === 'none') return <span className="v-pill v-pill-deny">Nothing, never applies</span>
    const grows = r.include.every((x) => x.inherit)
    return grows
      ? <span className="v-pill" style={{ color: 'var(--info)', background: 'var(--info-bg)' }}>↻ {r.include.length === 1 ? `${nodeName(r.include[0]!.scope_node_id)} and inside` : `${r.include.length} branches`}</span>
      : <span className="v-pill v-pill-warn">▣ {r.include.length} picked</span>
  }
  const toBulk = (kind?: 'grow') => {
    onClose()
    void navigate({ to: '/bulk', search: { role: roleId, ...(axis ? { axis: axis.code } : {}), ...(kind ? { kind } : {}) } })
  }

  const open = (g: Holder) => {
    onClose()
    void navigate({ to: '/identities/$id', params: { id: g.identity_id } })
  }

  const columns: Column<Holder>[] = [
    { key: 'who', header: 'Person', width: 200, render: (g) => (
        <span className="row-title t-body truncate" style={{ fontWeight: 560 }}>{g.username || g.identity_id}</span>
      ) },
    { key: 'kind', header: axis?.display_name ?? 'Kind', width: 170, render: (g) => badge(g) },
    { key: 'where', header: 'Where', width: 320, render: (g) => (
        <GrantWhere scopes={g.scopes} selfScoped={g.self_scoped} axes={axes} nodeName={nodeName} compact />
      ) },
    { key: 'from', header: 'Given', width: 170, render: (g) => (
        <span className="t-xs">
          {g.via_membership_id
            ? `Via ${memberships?.find((m) => m.id === g.via_membership_id)?.name ?? 'a membership'}`
            : 'Directly'}
        </span>
      ) },
    { key: 'ends', header: 'Ends', width: 120, render: (g) => (
        <span className="t-xs tnum">{g.valid_until ? fmtDate(g.valid_until) : 'No end date'}</span>
      ) },
    { key: 'act', header: '', width: 48, render: (g) => (
        <span onClick={(e) => e.stopPropagation()} onKeyDown={(e) => e.stopPropagation()}>
          <Menu position="bottom-end" width={220}>
            <Menu.Target>
              <ActionIcon variant="subtle" color="gray" aria-label={`Actions for ${g.username}`}><IconDots size={15} /></ActionIcon>
            </Menu.Target>
            <Menu.Dropdown>
              <Menu.Item leftSection={<IconUser size={14} />} onClick={() => open(g)}>Open {g.username}</Menu.Item>
              <Menu.Item leftSection={<IconInfinity size={14} />} onClick={() => toBulk('grow')}>Make it grow</Menu.Item>
            </Menu.Dropdown>
          </Menu>
        </span>
      ) },
    { key: 'go', header: '', width: 30, render: () => <IconChevronRight className="row-go" size={14} aria-hidden /> },
  ]

  return (
    <Modal opened={!!role} onClose={onClose} size="min(960px, 96vw)"
      title={role ? `People with ${shortRoleName(role)}` : ''}>
      <div className="mb-3 flex flex-wrap items-start justify-between gap-2">
        <div className="t-sm" style={{ maxWidth: 600 }}>
          {kinds ? `Given ${kinds.total.toLocaleString()} ${kinds.total === 1 ? 'time' : 'times'}. ` : ''}
          Each row shows whether their {noun} limit grows by itself or is a fixed list someone has to keep.
        </div>
        <Button size="xs" variant="default" leftSection={<IconStack2 size={14} />} onClick={() => toBulk()}>Change these in bulk</Button>
      </div>
      {(axes?.length ?? 0) > 1 && (
        <SegmentedControl size="xs" mb="sm" value={axis?.code ?? ''} onChange={setAxisPick}
          data={(axes ?? []).map((a) => ({ value: a.code, label: a.display_name }))} />
      )}
      {kinds && kinds.total > 0 && (
        <div className="mb-3 flex flex-wrap gap-1.5" aria-label={`By ${noun}`}>
          <span className="chip">All · {kinds.total}</span>
          {kinds.everything > 0 && <span className="v-pill v-pill-allow">↻ Every {noun} · {kinds.everything}</span>}
          {kinds.branch > 0 && <span className="v-pill" style={{ color: 'var(--info)', background: 'var(--info-bg)' }}>↻ A branch · {kinds.branch}</span>}
          {kinds.fixed > 0 && <span className="v-pill v-pill-warn">▣ Fixed list · {kinds.fixed}</span>}
          {kinds.full > 0 && (
            <button type="button" className="v-pill v-pill-warn" onClick={() => toBulk('grow')} style={{ cursor: 'pointer' }}>
              ⚠ Fixed list holding every place · {kinds.full} · make it grow
            </button>
          )}
          {kinds.nothing > 0 && <span className="v-pill v-pill-deny">Never applies · {kinds.nothing}</span>}
        </div>
      )}
      <DataTable columns={columns} rows={rows} rowKey={(g) => g.id} onRowClick={open}
        stale={isFetching && rows !== undefined}
        footer={
          <>
            <span className="t-xs tnum">
              {rows?.length ?? 0} on this page{trail.length > 1 && ` · page ${trail.length}`}
            </span>
            <div className="ml-auto flex items-center gap-2">
              <Button variant="default" size="compact-sm" disabled={trail.length <= 1}
                onClick={() => setTrail(trail.slice(0, -1))}>Previous</Button>
              <Button variant="default" size="compact-sm" disabled={!page?.next}
                onClick={() => setTrail([...trail, page?.next ?? ''])}>Next</Button>
            </div>
          </>
        }
        empty={{
          title: 'Nobody holds this role',
          hint: 'Give it from a person’s own page: People → the person → Give access.',
        }} />
    </Modal>
  )
}
