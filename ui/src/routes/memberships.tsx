import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { Button, Modal, TextInput, UnstyledButton } from '@mantine/core'
import { notifications } from '@mantine/notifications'
import {
  IconChevronRight, IconMapPin, IconPencil, IconPlus, IconSearch, IconUserMinus,
  IconUserPlus, IconUsersGroup,
} from '@tabler/icons-react'
import { useState } from 'react'
import { Page } from '@/components/shell/Page'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/client'
import { fmtDate } from '@/lib/access'
import { useCreate } from '@/stores/create'
import { notifyCreated, notifyRejected } from '@/components/create/shell'
import { ConfirmModal } from '@/components/ui/ConfirmModal'
import { Initial } from '@/components/ui/Initial'
import { GiveAccessSheet } from '@/components/access/GiveAccessSheet'
import { GrantWhere } from '@/components/access/GrantWhere'
import { MembershipEntries, type EntryDraft } from '@/components/access/MembershipEntries'
import type { Membership, MembershipAssignment, ScopeAxis } from '@/lib/api/types'

export const Route = createFileRoute('/memberships')({ component: Memberships })

const refresh = async () => {
  await queryClient.invalidateQueries({ queryKey: qk.memberships() })
  await queryClient.invalidateQueries({ queryKey: ['grants'] })
  await queryClient.invalidateQueries({ queryKey: qk.dashboard() })
}

/* Who holds it, a page at a time and only when asked. A membership can hold
   thousands, and a page of twenty memberships that each fetched a roster on
   arrival would be twenty requests nobody had read yet. */
function Roster({ m }: { m: Membership }) {
  const navigate = useNavigate()
  const [leaving, setLeaving] = useState<MembershipAssignment | null>(null)
  const [why, setWhy] = useState('')
  const q = useInfiniteQuery({
    queryKey: qk.membershipAssignments({ membershipId: m.id }),
    queryFn: ({ pageParam }) => api.membershipAssignments({ membershipId: m.id, cursor: pageParam, pageSize: 25 }),
    initialPageParam: '',
    getNextPageParam: (last) => last.next || undefined,
  })
  const rows = q.data?.pages.flatMap((p) => p.rows) ?? []

  async function remove(a: MembershipAssignment) {
    let n: number
    try {
      n = await api.removeAssignment(a.id, why)
    } catch (e) {
      notifyRejected(e)
      throw e
    }
    notifications.show({
      color: 'orange', title: 'Member removed',
      message: `${a.username} left “${m.name}”${a.place_name ? ` at ${a.place_name}` : ''} — ${n} grant${n === 1 ? '' : 's'} revoked.`,
    })
    await refresh()
  }

  return (
    <div>
      {q.isLoading && <div className="t-xs px-4 py-3">Loading…</div>}
      {!q.isLoading && rows.length === 0 && <div className="t-xs px-4 py-3">Nobody holds this yet.</div>}
      {rows.map((a) => (
        <div key={a.id} className="roster-row">
          <UnstyledButton className="flex min-w-0 flex-1 items-center gap-2.5 text-left"
            onClick={() => void navigate({ to: '/identities/$id', params: { id: a.identity_id } })}
            aria-label={`Open ${a.username}`}>
            <Initial name={a.username} colour="var(--ink-3)" size={24} />
            <span className="min-w-0">
              <span className="t-body block truncate" style={{ fontWeight: 540 }}>{a.username}</span>
              <span className="t-xs block truncate">
                {[
                  a.place_name && (a.exact ? `Only ${a.place_name}` : `${a.place_name} and inside`),
                  a.valid_until ? `until ${fmtDate(a.valid_until)}` : `since ${fmtDate(a.assigned_at)}`,
                  a.reason && `“${a.reason}”`,
                ].filter(Boolean).join(' · ')}
              </span>
            </span>
          </UnstyledButton>
          <Button size="compact-xs" variant="subtle" color="deny" leftSection={<IconUserMinus size={13} />}
            onClick={() => setLeaving(a)}>
            Remove
          </Button>
        </div>
      ))}
      {q.hasNextPage && (
        <div className="px-4 py-2">
          <Button size="compact-xs" variant="subtle" loading={q.isFetchingNextPage}
            onClick={() => void q.fetchNextPage()}>
            Show more
          </Button>
        </div>
      )}

      <ConfirmModal opened={!!leaving} onClose={() => { setLeaving(null); setWhy('') }}
        title="Remove this member?" confirmLabel="Remove"
        onConfirm={() => (leaving ? remove(leaving) : Promise.resolve())}>
        <p>
          <b style={{ color: 'var(--ink)' }}>{leaving?.username}</b> will leave{' '}
          <b style={{ color: 'var(--ink)' }}>{m.name}</b>
          {leaving?.place_name && <> at <b style={{ color: 'var(--ink)' }}>{leaving.place_name}</b></>}
          {' '}and lose what it gave them{leaving?.place_name ? ' there' : ''}. New decisions are denied at once.
        </p>
        <TextInput label="Why" description="Optional. Kept with each grant this takes away."
          maxLength={200} data-autofocus value={why} onChange={(e) => setWhy(e.currentTarget.value)} />
        <p className="t-xs">
          {leaving?.place_name
            ? 'Only this place. Anywhere else they hold this membership stays, and so does access given to them directly.'
            : 'Other members keep theirs. Access given to this person directly is not touched.'}
        </p>
      </ConfirmModal>
    </div>
  )
}

/* Changing what a membership gives changes it for everybody in it, at once.
   The dialog says so before the button does it. */
function EditGives({ m, axes, onClose }: { m: Membership; axes: ScopeAxis[] | undefined; onClose: () => void }) {
  const [entries, setEntries] = useState<EntryDraft[]>(
    () => m.entries.map((e) => ({ role_id: e.role_id, role_name: e.role_name, scopes: e.scopes })))
  const [busy, setBusy] = useState(false)
  const count = m.member_count ?? 0
  const save = async () => {
    setBusy(true)
    try {
      const n = await api.setMembershipEntries(m.id, entries)
      notifyCreated('Membership updated',
        n === 0 ? 'Nobody’s access changed.' : `${n} grant${n === 1 ? '' : 's'} changed across its members.`)
      await refresh()
      onClose()
    } catch (e) { notifyRejected(e) }
    setBusy(false)
  }
  return (
    <Modal opened onClose={busy ? () => {} : onClose} title={`What “${m.name}” gives`} size={520}>
      <div className="@container flex flex-col gap-4 pt-1">
        <div className="t-sm">
          {count === 0
            ? 'Nobody holds this yet, so nothing changes for anyone.'
            : `${count.toLocaleString()} ${count === 1 ? 'person holds' : 'people hold'} this. They gain every role you add and lose every role you remove, as soon as you save.`}
        </div>
        <MembershipEntries anchorAxis={m.anchor_axis} axes={axes} entries={entries} onChange={setEntries} />
        <div className="flex justify-end gap-2">
          <Button variant="default" size="sm" onClick={onClose} disabled={busy}>Cancel</Button>
          <Button size="sm" loading={busy} onClick={() => void save()}>Save</Button>
        </div>
      </div>
    </Modal>
  )
}

function MembershipCard({ m, axes }: { m: Membership; axes: ScopeAxis[] | undefined }) {
  /* Only the places this card shows. Pulling every node of every structure
     (32k here) to label a few chips was the previous approach. */
  const scopeIds = [...new Set(m.entries.flatMap((e) => e.scopes.map((s) => s.scope_node_id)))].sort()
  const { data: nodes } = useQuery({
    queryKey: ['scope-names', scopeIds],
    queryFn: () => api.scopeNodesByIds(scopeIds),
    enabled: scopeIds.length > 0,
  })
  const nodeName = (id: string) => nodes?.find((n) => n.id === id)?.name ?? '…'
  const [open, setOpen] = useState(false)
  const [adding, setAdding] = useState(false)
  const [editing, setEditing] = useState(false)
  const count = m.member_count ?? m.member_ids.length
  const anchor = axes?.find((a) => a.code === m.anchor_axis)?.display_name ?? m.anchor_axis

  return (
    <section className="@container panel overflow-clip">
      <div className="panel-head">
        <div className="min-w-0">
          <h2 className="t-h1 truncate">{m.name}</h2>
          {m.description && <div className="t-xs mt-0.5 truncate">{m.description}</div>}
        </div>
        <div className="flex shrink-0 items-center gap-1.5">
          <Button size="xs" variant="default" leftSection={<IconPencil size={13} />} onClick={() => setEditing(true)}>
            Edit
          </Button>
          <Button size="xs" leftSection={<IconUserPlus size={13} />} onClick={() => setAdding(true)}>
            Add member
          </Button>
        </div>
      </div>

      <div className="flex flex-col gap-3 px-4 py-3.5">
        <div className="t-xs flex items-center gap-1.5">
          {m.anchor_axis ? <IconMapPin size={13} /> : <IconUsersGroup size={13} />}
          {m.anchor_axis
            ? <>Applies where each member is placed in <b style={{ color: 'var(--ink-2)', fontWeight: 560 }}>{anchor}</b></>
            : 'Same places for every member'}
        </div>
        <div>
          <div className="t-label mb-1.5">Gives every member</div>
          {m.entries.length === 0 && <div className="t-xs">No roles yet — holding it gives nothing.</div>}
          <div className="flex flex-col gap-2">
            {m.entries.map((e) => (
              <div key={e.id} className="panel-inset flex flex-col gap-1 px-2.5 py-2">
                <span className="t-body" style={{ fontWeight: 560 }}>{e.role_name}</span>
                {/* A placed membership's entry that names no place means "at
                    the member's place", which GrantWhere would call everywhere. */}
                {m.anchor_axis && e.scopes.length === 0
                  ? <span className="t-xs">At the place each member holds it</span>
                  : <GrantWhere scopes={e.scopes} selfScoped={false} axes={axes} nodeName={nodeName} compact />}
              </div>
            ))}
          </div>
        </div>
      </div>

      <div style={{ borderTop: '1px solid var(--line-soft)' }}>
        <UnstyledButton className="access-group w-full" onClick={() => setOpen((o) => !o)} aria-expanded={open}>
          <span className="flex items-center gap-2">
            <IconChevronRight size={13}
              style={{ color: 'var(--ink-3)', transform: open ? 'rotate(90deg)' : undefined, transition: 'transform var(--t-fast)' }} />
            <span className="t-body" style={{ fontWeight: 580 }}>Members</span>
            <span className="count-pill">{count.toLocaleString()}</span>
          </span>
          <span className="t-xs">{open ? 'Hide' : 'Show'}</span>
        </UnstyledButton>
        {open && <Roster m={m} />}
      </div>

      <GiveAccessSheet opened={adding} onClose={() => setAdding(false)} pickSubject membershipId={m.id} />
      {editing && <EditGives m={m} axes={axes} onClose={() => setEditing(false)} />}
    </section>
  )
}

function Memberships() {
  const { openCreate } = useCreate()
  const { data: memberships } = useQuery({ queryKey: qk.memberships(), queryFn: api.memberships })
  const { data: axes } = useQuery({ queryKey: qk.axes(), queryFn: api.axes })
  const [q, setQ] = useState('')
  const needle = q.trim().toLowerCase()
  const shown = (memberships ?? []).filter((m) =>
    !needle || m.name.toLowerCase().includes(needle) ||
    m.description.toLowerCase().includes(needle))
  return (
    <Page
      title="Memberships"
      description="Named sets of roles that people hold together — a team, a council. Add someone and they get all of it; remove them and it is taken back at once."
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
            {needle ? `Nothing matching “${q}”.` : 'Put the roles a team or council shares in one place, then adding someone is one action.'}
          </div>
        </div>
      ) : (
        <div className="grid items-start gap-4" style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(min(380px, 100%), 1fr))' }}>
          {shown.map((m) => <MembershipCard key={m.id} m={m} axes={axes} />)}
        </div>
      )}
    </Page>
  )
}
