import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Button, Modal } from '@mantine/core'
import { IconChevronRight } from '@tabler/icons-react'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { fmtDate } from '@/lib/access'
import { DataTable, type Column } from '@/components/ui/DataTable'
import { GrantWhere } from './GrantWhere'
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

  const open = (g: Holder) => {
    onClose()
    void navigate({ to: '/identities/$id', params: { id: g.identity_id } })
  }

  const columns: Column<Holder>[] = [
    { key: 'who', header: 'Person', width: 200, render: (g) => (
        <span className="row-title t-body truncate" style={{ fontWeight: 560 }}>{g.username || g.identity_id}</span>
      ) },
    { key: 'where', header: 'Where', width: 340, render: (g) => (
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
    { key: 'go', header: '', width: 36, render: () => <IconChevronRight className="row-go" size={14} aria-hidden /> },
  ]

  return (
    <Modal opened={!!role} onClose={onClose} size="min(960px, 96vw)"
      title={role ? `People with ${role.name}` : ''}>
      {role?.description && <div className="t-sm mb-3" style={{ maxWidth: 640 }}>{role.description}</div>}
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
