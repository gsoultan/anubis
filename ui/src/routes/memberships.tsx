import { createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Button, Select, TextInput } from '@mantine/core'
import { IconPlus, IconSearch, IconUsersGroup, IconX } from '@tabler/icons-react'
import { useState } from 'react'
import { Page } from '@/components/shell/Page'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/client'
import { useCreate } from '@/stores/create'
import { notifyCreated, notifyRejected } from '@/components/create/shell'
import type { Membership } from '@/lib/api/types'

export const Route = createFileRoute('/memberships')({ component: Memberships })

function MembershipCard({ m }: { m: Membership }) {
  const { data: identities } = useQuery({ queryKey: qk.identities(), queryFn: () => api.identities() })
  const { data: realms } = useQuery({ queryKey: qk.realms(), queryFn: api.realms })
  const { data: nodes } = useQuery({
    queryKey: ['all-nodes'],
    queryFn: async () => (await Promise.all((await api.axes()).map((a) => api.scopeSearch(a.code, '')))).flat(),
  })
  const [pick, setPick] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const nodeName = (id: string) => nodes?.find((n) => n.id === id)?.name ?? id
  const count = m.member_count ?? m.member_ids.length

  const refresh = async () => {
    await queryClient.invalidateQueries({ queryKey: qk.memberships() })
    await queryClient.invalidateQueries({ queryKey: ['grants'] })
  }
  const assign = async () => {
    if (!pick) return
    setBusy(true)
    try {
      const n = await api.assignMembership(pick, m.id)
      notifyCreated('Member added', `${identities?.find((i) => i.id === pick)?.username} received ${n} access grant${n > 1 ? 's' : ''}.`)
      await refresh(); setPick(null)
    } catch (e) { notifyRejected(e) }
    setBusy(false)
  }
  const unassign = async (id: string) => {
    try {
      const n = await api.unassignMembership(id, m.id)
      notifyCreated('Member removed', `${n} derived grant${n === 1 ? '' : 's'} revoked immediately.`)
      await refresh()
    } catch (e) { notifyRejected(e) }
  }

  const candidates = (realms ?? []).map((r) => ({
    group: r.display_name,
    items: (identities ?? [])
      .filter((i) => i.realm_id === r.id && !m.member_ids.includes(i.id))
      /* member_ids is empty when the server reported a count instead of a
         roster; adding somebody twice is a no-op, so nothing is lost. */
      .map((i) => ({ value: i.id, label: i.username })),
  })).filter((g) => g.items.length > 0)

  return (
    <div className="panel p-4">
      <div className="mb-1 flex items-baseline justify-between gap-2">
        <span className="t-h1">{m.name}</span>
        <span className="t-xs">{count} member{count === 1 ? '' : 's'}</span>
      </div>
      {m.description && <div className="t-sm mb-3">{m.description}</div>}

      <div className="t-label mb-1.5">Grants every member</div>
      <div className="mb-3 flex flex-col gap-1">
        {m.entries.map((e) => {
          /* Group per structure: "or" is only correct WITHIN one structure.
             Across structures the semantics are AND — joining everything with
             "or" would display the opposite of what the engine enforces. */
          const byAxis = new Map<string, typeof e.scopes>()
          for (const sc of e.scopes) {
            const list = byAxis.get(sc.axis_code) ?? []
            list.push(sc); byAxis.set(sc.axis_code, list)
          }
          return (
            <div key={e.id} className="panel-inset px-2.5 py-1.5">
              <span className="t-body" style={{ fontWeight: 530 }}>{e.role_name}</span>
              {e.scopes.length === 0 ? (
                <span className="t-xs" style={{ marginLeft: 6 }}>everywhere</span>
              ) : (
                <span className="t-xs" style={{ marginLeft: 6 }}>
                  {[...byAxis].map(([axis, list], i) => (
                    <span key={axis}>
                      {i > 0 && <span style={{ margin: '0 4px', color: 'var(--ink-4)' }}>·</span>}
                      <span className="chip" style={{ marginRight: 4, fontSize: 9.5 }}>{axis}</span>
                      {list.map((sc) => nodeName(sc.scope_node_id)).join(' or ')}
                    </span>
                  ))}
                </span>
              )}
            </div>
          )
        })}
      </div>

      <div className="t-label mb-1.5">Members</div>
      <div className="mb-2.5 flex flex-wrap gap-1.5">
        {m.member_ids.length === 0 && <span className="t-xs">Nobody yet.</span>}
        {m.member_ids.map((id) => (
          <span key={id} className="chip">
            {identities?.find((i) => i.id === id)?.username ?? id}
            <button onClick={() => void unassign(id)} aria-label="Remove member"
              style={{ marginLeft: 5, display: 'inline-flex', color: 'var(--ink-4)' }}>
              <IconX size={10} />
            </button>
          </span>
        ))}
      </div>
      <div className="flex items-center gap-1.5">
        <Select size="xs" searchable placeholder="Add a person…" data={candidates}
          value={pick} onChange={setPick} style={{ flex: 1 }} />
        <Button size="xs" variant="light" loading={busy} disabled={!pick} onClick={() => void assign()}>
          Assign
        </Button>
      </div>
    </div>
  )
}

function Memberships() {
  const { openCreate } = useCreate()
  const { data: memberships } = useQuery({ queryKey: qk.memberships(), queryFn: api.memberships })
  const [q, setQ] = useState('')
  const needle = q.trim().toLowerCase()
  const shown = (memberships ?? []).filter((m) =>
    !needle || m.name.toLowerCase().includes(needle) ||
    m.description.toLowerCase().includes(needle))
  return (
    <Page
      title="Memberships"
      description="Bundles of roles, each pinned to any mix of structures — organisation, product lines, customers — assigned as one unit. Removing a member revokes everything the membership gave them, instantly."
      actions={
        <>
          <TextInput w={200} placeholder="Search memberships"
            leftSection={<IconSearch size={14} />}
            value={q} onChange={(e) => setQ(e.currentTarget.value)} />
          <Button size="xs" leftSection={<IconPlus size={13} />} onClick={() => openCreate('membership')}>
            New membership
          </Button>
        </>
      }
    >
      {shown.length === 0 ? (
        <div className="panel px-6 py-14 text-center">
          <IconUsersGroup size={26} style={{ color: 'var(--ink-3)', margin: '0 auto 10px' }} />
          <div className="t-h2">{needle ? 'No memberships match' : 'No memberships yet'}</div>
          <div className="t-sm mt-1.5">
            {needle ? `Nothing matching “${q}”.` : 'Bundle the roles a typical hire needs, then onboarding is one action.'}
          </div>
        </div>
      ) : (
        <div className="grid gap-4" style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(360px, 1fr))' }}>
          {shown.map((m) => <MembershipCard key={m.id} m={m} />)}
        </div>
      )}
    </Page>
  )
}
