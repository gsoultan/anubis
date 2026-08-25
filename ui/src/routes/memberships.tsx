import { createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Button, Select, TextInput } from '@mantine/core'
import { IconPlus, IconSearch, IconUsersGroup, IconX } from '@tabler/icons-react'
import { useState } from 'react'
import { useDebouncedValue } from '@mantine/hooks'
import { Page } from '@/components/shell/Page'
import { api } from '@/lib/api/client'
import * as live from '@/lib/api/live'
import { qk } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/client'
import { useCreate } from '@/stores/create'
import { notifyCreated, notifyRejected } from '@/components/create/shell'
import type { Membership } from '@/lib/api/types'

export const Route = createFileRoute('/memberships')({ component: Memberships })

function MembershipCard({ m }: { m: Membership }) {
  /* The picker SEARCHES the server rather than loading the directory. It
     used to pull up to 2000 identities to populate a dropdown over 57,000
     people — so the person you wanted was often simply not in the list. */
  const [search, setSearch] = useState('')
  const [debounced] = useDebouncedValue(search, 250)
  const { data: found } = useQuery({
    queryKey: ['member-search', debounced],
    queryFn: () => live.identitiesPage(undefined, debounced || undefined, '', 20),
    placeholderData: (prev) => prev,
  })
  const { data: realms } = useQuery({ queryKey: qk.realms(), queryFn: api.realms })
  /* Only the scopes this card actually shows. Pulling every node of every
     axis (32k here) to label a few chips was the previous approach. */
  const scopeIds = [...new Set(m.entries.flatMap((e) => e.scopes.map((s) => s.scope_node_id)))].sort()
  const { data: nodes } = useQuery({
    queryKey: ['scope-names', scopeIds],
    queryFn: () => api.scopeNodesByIds(scopeIds),
    enabled: scopeIds.length > 0,
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
      notifyCreated('Member added', `${found?.rows.find((i) => i.id === pick)?.username ?? 'The person'} received ${n} access grant${n > 1 ? 's' : ''}.`)
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
    /* No exclusion of existing members: the server reports a COUNT, not a
       roster, and AssignMembership is idempotent — so adding somebody twice
       costs nothing, while hiding them would need a roster nobody sends. */
    items: (found?.rows ?? [])
      .filter((i) => i.realm_id === r.id)
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
        {/* The API reports a COUNT, not a roster: a membership can hold
            thousands, and no screen needs the list to say "412 members".
            This used to render the empty roster as "Nobody yet." — which
            read as a fact and was one for no membership at all. */}
        {count === 0
          ? <span className="t-xs">Nobody yet.</span>
          : <span className="t-xs">{count.toLocaleString()} {count === 1 ? 'person holds' : 'people hold'} this membership. Find them on Access, filtered by membership.</span>}
      </div>
      <div className="flex items-center gap-1.5">
        <Select size="xs" searchable placeholder="Search for a person…" data={candidates}
          value={pick} onChange={setPick} style={{ flex: 1 }}
          searchValue={search} onSearchChange={setSearch}
          /* The server already filtered; filtering again would hide matches
             it deliberately returned. */
          filter={({ options }) => options}
          nothingFoundMessage={debounced ? 'Nobody matches' : 'Type to search'} />
        <Button size="xs" variant="light" loading={busy} disabled={!pick} onClick={() => void assign()}>
          Assign
        </Button>
        {/* Removing works through the same search. Without a roster from the
            server there is nobody to click on, and the chips this replaced
            were never populated. */}
        <Button size="xs" variant="subtle" color="red" loading={busy} disabled={!pick}
          leftSection={<IconX size={12} />} onClick={() => pick && void unassign(pick)}>
          Remove
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
