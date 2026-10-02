import { useEffect, useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Chip, Radio, SegmentedControl, Select, Textarea, UnstyledButton } from '@mantine/core'
import { DateInput } from '@mantine/dates'
import {
  IconCheck, IconChevronRight, IconInfoCircle, IconMapPin, IconShield, IconUser,
  IconUsersGroup, IconWorld,
} from '@tabler/icons-react'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { realmKindColor } from '@/lib/realmKind'
import { fmtDate, namesOf, placesToScopes, relDays, roleBlocked, type Until } from '@/lib/access'
import { CreateShell, CancelSubmit } from '@/components/create/shell'
import { IdentityPicker } from '@/components/ui/IdentityPicker'
import { Initial } from '@/components/ui/Initial'
import { GrantWhere } from './GrantWhere'
import { PlacePicker } from './PlacePicker'
import { ScopeTree } from '@/components/scope/ScopeTree'
import { useGrantDraft, type GrantDraft, type Reach } from './useGrantDraft'
import type { Membership, Role, ScopeAxis } from '@/lib/api/types'

/* Giving somebody access — the one form for it, wherever it is opened from.
 *
 * It used to exist twice: a panel on a person's page and a drawer everywhere
 * else, sharing fields but not a shape. Both asked for a role, then a
 * validity, then a self-scope switch, then one "Anywhere" row per structure —
 * the question of WHERE was answered by not touching forty-seven controls.
 *
 * Now it is four questions in the order a person asks them — what, where,
 * how long, why — and a review that reads the answer back before anything is
 * written. "Where" is an explicit choice between everywhere, specific places
 * and their own records: the broadest grant there is can no longer be the one
 * you get by default.
 */

function Step({ n, title, hint, done, children }: {
  n: number
  title: string
  hint?: ReactNode
  done: boolean
  children: ReactNode
}) {
  return (
    <section className="step">
      <div className="flex items-start gap-2.5">
        <span className="step-n" data-done={done ? '' : undefined} aria-hidden>
          {done ? <IconCheck size={12} stroke={2.6} /> : n}
        </span>
        <div className="min-w-0 pt-px">
          <h3 className="t-h2">{title}</h3>
          {hint && <div className="t-xs mt-0.5">{hint}</div>}
        </div>
      </div>
      <div className="step-body">{children}</div>
    </section>
  )
}

function RoleDetail({ role, held }: { role: Role; held: number }) {
  const [open, setOpen] = useState(false)
  const { data: perms, isLoading } = useQuery({
    queryKey: qk.rolePermissions(role.id),
    queryFn: () => api.rolePermissions(role.id),
    enabled: open,
  })
  return (
    <div className="panel-inset mt-2 px-3 py-2.5">
      <div className="t-sm">{role.description || 'No description for this role.'}</div>
      <div className="mt-2 flex flex-wrap items-center gap-2">
        {role.application_id && <span className="chip">{role.application_id}</span>}
        <UnstyledButton className="t-xs inline-flex items-center gap-1" style={{ color: 'var(--accent)' }}
          onClick={() => setOpen((o) => !o)} aria-expanded={open}>
          <IconChevronRight size={12}
            style={{ transform: open ? 'rotate(90deg)' : undefined, transition: 'transform var(--t-fast)' }} />
          {open ? 'Hide what it permits' : 'What it permits'}
        </UnstyledButton>
      </div>
      {open && (
        <div className="mt-2 flex max-h-[168px] flex-wrap gap-1 overflow-y-auto">
          {isLoading && <span className="t-xs">Loading…</span>}
          {perms?.length === 0 && (
            <span className="t-xs" style={{ color: 'var(--warn)' }}>
              This role permits nothing yet — giving it changes nothing.
            </span>
          )}
          {perms?.map((k) => <span key={k} className="chip">{k}</span>)}
        </div>
      )}
      {held > 0 && (
        <div className="t-xs mt-2" style={{ color: 'var(--info)' }}>
          They already hold this role{held > 1 ? ` in ${held} grants` : ''}. This adds another grant beside it.
        </div>
      )}
    </div>
  )
}

function MembershipDetail({ m, axes, member }: {
  m: Membership
  axes: ScopeAxis[] | undefined
  member: boolean
}) {
  const ids = [...new Set(m.entries.flatMap((e) => e.scopes.map((s) => s.scope_node_id)))].sort()
  const { data: nodes } = useQuery({
    queryKey: ['scope-names', ids],
    queryFn: () => api.scopeNodesByIds(ids),
    enabled: ids.length > 0,
  })
  const nodeName = (id: string) => nodes?.find((n) => n.id === id)?.name ?? '…'
  const anchor = axes?.find((a) => a.code === m.anchor_axis)
  return (
    <div className="panel-inset mt-2 flex flex-col gap-2.5 px-3 py-2.5">
      {m.description && <div className="t-sm">{m.description}</div>}
      <div className="t-label">
        {m.anchor_axis
          ? `Gives every member, at the ${(anchor?.display_name ?? m.anchor_axis).toLowerCase()} place they hold it`
          : 'Gives every member'}
      </div>
      {m.entries.length === 0 && <div className="t-xs">No roles yet — joining gives nothing.</div>}
      {m.entries.map((e) => (
        <div key={e.id} className="flex flex-col gap-1">
          <span className="t-body" style={{ fontWeight: 560 }}>{e.role_name}</span>
          {/* Where-assigned: an entry naming no place means "at their place",
              which GrantWhere would read as everywhere. */}
          {m.anchor_axis && e.scopes.length === 0
            ? <span className="t-xs">At the place they hold it</span>
            : <GrantWhere scopes={e.scopes} selfScoped={false} axes={axes} nodeName={nodeName} compact />}
        </div>
      ))}
      <div className="t-xs" style={member ? { color: 'var(--info)' } : undefined}>
        {m.anchor_axis
          ? member
            ? 'They already hold this somewhere. Another place gives the same roles there too.'
            : 'One person can hold it at several places. Removing one place later leaves the others.'
          : member
            ? 'They already hold this membership.'
            : 'Places come from the membership. Removing them from it later takes all of this away at once.'}
      </div>
    </div>
  )
}

/* Where a where-assigned membership is held: one place in its structure, and
   whether it reaches what sits inside that place. */
function Seat({ d }: { d: GrantDraft }) {
  const axis = d.membership?.anchor_axis
  if (!axis) return null
  const name = d.axes?.find((a) => a.code === axis)?.display_name ?? axis
  return (
    <>
      <div className="panel-inset p-2">
        <ScopeTree axis={axis} selectedId={d.seat?.id ?? null} height={260}
          placeholder={`Search ${name}…`} emptyHint="This structure has no places yet."
          onSelect={(n) => d.setSeat({ id: n.id, name: n.name })} />
      </div>
      {d.seat && (
        <SegmentedControl fullWidth size="sm" mt="sm" value={d.seatExact ? 'exact' : 'inside'}
          onChange={(v) => d.setSeatExact(v === 'exact')} aria-label="Reach"
          data={[
            { value: 'inside', label: `${d.seat.name} and everything inside` },
            { value: 'exact', label: `Only ${d.seat.name} itself` },
          ]} />
      )}
      {d.heldHere.length > 0 && (
        <div className="t-xs mt-2">
          Already held at {d.heldHere.map((a) => a.place_name ?? '—').join(', ')}.
        </div>
      )}
    </>
  )
}

const REACH: { value: Reach; title: string; body: string; icon: ReactNode }[] = [
  { value: 'everywhere', title: 'Everywhere', icon: <IconWorld size={16} />,
    body: 'No limits — every office, product and customer, including ones added later.' },
  { value: 'places', title: 'Specific places', icon: <IconMapPin size={16} />,
    body: 'Only the places you choose, in the structures you choose from.' },
  { value: 'own', title: 'Their own records', icon: <IconUser size={16} />,
    body: 'Only records they own — their application, their orders. For customers and applicants.' },
]

const UNTIL: { value: Until; label: string }[] = [
  { value: '', label: 'No end date' },
  { value: '30', label: '30 days' },
  { value: '90', label: '90 days' },
  { value: '365', label: '1 year' },
  { value: 'date', label: 'Pick a date' },
]

function Review({ d }: { d: GrantDraft }) {
  const names = new Map(Object.values(d.places).flat().map((v) => [v.id, v.name]))
  const row = (label: string, value: ReactNode) => (
    <>
      <dt className="t-xs pt-px">{label}</dt>
      <dd className="t-body min-w-0">{value}</dd>
    </>
  )
  const none = <span style={{ color: 'var(--ink-3)' }}>—</span>
  return (
    <div className="review">
      <div className="t-label mb-2.5">Review</div>
      <dl className="review-grid">
        {row('Person', d.subject ? <b style={{ fontWeight: 600 }}>{d.subject.username}</b> : none)}
        {d.what === 'membership' ? (
          <>
            {row('Membership', d.membership?.name ?? none)}
            {row('Gives', d.membership
              ? `${d.membership.entries.length} role${d.membership.entries.length === 1 ? '' : 's'}`
              : none)}
            {row('Where', !d.membership ? none
              : !d.membership.anchor_axis ? 'Where the membership says, the same for everyone'
              : d.seat ? (d.seatExact ? `Only ${d.seat.name} itself` : `${d.seat.name} and everything inside it`)
              : <span style={{ color: 'var(--ink-3)' }}>No place chosen yet</span>)}
            {row('Ends', d.ends
              ? <span className="tnum">{fmtDate(d.ends)} <span className="t-xs">· {relDays(d.ends)}</span></span>
              : d.until === 'date' ? none : 'No end date — until someone removes it')}
            {d.reason.trim() && row('Note', <span style={{ whiteSpace: 'pre-wrap' }}>{d.reason.trim()}</span>)}
          </>
        ) : (
          <>
            {row('Role', d.role ? <b style={{ fontWeight: 600 }}>{d.role.name}</b> : none)}
            {/* "Specific places" with none chosen is NOT everywhere, though it
                has the same empty list of scopes. Rendered through GrantWhere
                it read "Everywhere — no limits": the review promising the
                widest grant there is for a form that is not finished. */}
            {row('Where', d.reach === null ? none
              : d.reach === 'places' && !placesToScopes(d.places).some((s) => !s.exclude)
                ? <span style={{ color: 'var(--ink-3)' }}>No places chosen yet</span>
              : <GrantWhere scopes={d.reach === 'places' ? placesToScopes(d.places) : []}
                  selfScoped={d.reach === 'own'} axes={d.axes}
                  nodeName={(id) => names.get(id) ?? id} />)}
            {row('Ends', d.ends
              ? <span className="tnum">{fmtDate(d.ends)} <span className="t-xs">· {relDays(d.ends)}</span></span>
              : d.until === 'date' ? none : 'No end date — until someone revokes it')}
            {d.reason.trim() && row('Note', <span style={{ whiteSpace: 'pre-wrap' }}>{d.reason.trim()}</span>)}
          </>
        )}
      </dl>
    </div>
  )
}

export function GiveAccessSheet({ opened, onClose, identityId = null, pickSubject = false, membershipId = null }: {
  opened: boolean
  onClose: () => void
  /** Opened from a membership: it is already chosen, the person is not. */
  membershipId?: string | null
  /** The person, when the sheet is opened from them. */
  identityId?: string | null
  /** Opened from nowhere in particular: ask who first. */
  pickSubject?: boolean
}) {
  const [picked, setPicked] = useState<string | null>(null)
  const subjectId = pickSubject ? picked : identityId
  const d = useGrantDraft({ identityId: subjectId, onDone: () => { setPicked(null); onClose() } })
  const close = () => { d.reset(); setPicked(null); onClose() }
  /* Every opening starts empty. The sheet stays mounted, and on a person's
     page it closes without onClose too — Back simply drops ?give — so a
     half-filled draft (a role picked for somebody else, even) would otherwise
     be waiting the next time it opened. */
  const { reset, setWhat, setMembershipId } = d
  useEffect(() => {
    if (opened) {
      reset(); setPicked(null)
      if (membershipId) { setWhat('membership'); setMembershipId(membershipId) }
    }
    // Only on opening: `reset` is a new function every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [opened])

  const blocked = new Map((d.roles ?? []).map((r) => [r.id, roleBlocked(r, d.subjectKind)]))
  const open = (d.roles ?? []).filter((r) => !blocked.get(r.id))
  const shut = (d.roles ?? []).filter((r) => blocked.get(r.id))
  const opt = (r: Role) => ({ value: r.id, label: r.name, disabled: !!blocked.get(r.id) })
  const roleData = shut.length === 0 ? open.map(opt) : [
    { group: 'Available', items: open.map(opt) },
    { group: 'Not available for this person', items: shut.map(opt) },
  ]
  const hasMemberships = (d.memberships?.length ?? 0) > 0

  let n = 0
  const who = d.subject
  const kindColour = realmKindColor(d.subjectKind)

  return (
    <CreateShell
      opened={opened} onClose={close} size="min(680px, 100vw)"
      title={membershipId ? 'Add a member' : 'Give access'}
      description={who ? (
        <span className="flex min-w-0 items-center gap-2.5">
          <Initial name={who.username} colour={kindColour} size={28} />
          <span className="min-w-0">
            <span className="t-body block truncate" style={{ fontWeight: 600, color: 'var(--ink)' }}>{who.username}</span>
            <span className="t-xs block truncate">{who.email || 'no email on file'}</span>
          </span>
        </span>
      ) : 'Choose a person, what they can do, and where.'}
      /* Neutral even when ready: --allow is a verdict about access, and a
         finished form is not one. */
      status={d.problems[0] ?? <span style={{ color: 'var(--ink-2)' }}>Ready — check the review, then confirm.</span>}
      footer={
        <CancelSubmit onCancel={close} onSubmit={() => void d.submit()}
          canSubmit={d.canSubmit} submitting={d.submitting}
          label={d.what === 'membership' ? 'Add to membership' : 'Give access'} />
      }
    >
      <div className="flex flex-col gap-7">
        {pickSubject && (
          <Step n={++n} title="Who needs access?" done={!!subjectId}>
            <IdentityPicker value={picked} onChange={(id) => setPicked(id)}
              placeholder="Search by username or email…" />
          </Step>
        )}

        <Step n={++n} title={membershipId ? 'The membership' : 'What should they be able to do?'} done={d.ready.what}
          hint={membershipId ? undefined : hasMemberships ? 'A role on its own, or a membership that bundles several.' : undefined}>
          {hasMemberships && !membershipId && (
            <SegmentedControl fullWidth size="sm" mb="sm" value={d.what}
              onChange={(v) => d.setWhat(v as GrantDraft['what'])}
              data={[
                { value: 'role', label: <span className="inline-flex items-center gap-1.5"><IconShield size={14} />A role</span> },
                { value: 'membership', label: <span className="inline-flex items-center gap-1.5"><IconUsersGroup size={14} />A membership</span> },
              ]} />
          )}
          {d.what === 'role' ? (
            <>
              <Select searchable placeholder={d.subject ? 'Search roles…' : 'Choose a person first'}
                disabled={!d.subject} data={roleData} value={d.roleId} onChange={d.setRoleId}
                maxDropdownHeight={340} nothingFoundMessage="No role matches"
                aria-label="Role"
                renderOption={({ option, checked }) => {
                  const r = d.roles?.find((x) => x.id === option.value)
                  if (!r) return option.label
                  const why = blocked.get(r.id)
                  return (
                    <div className="flex w-full min-w-0 items-start gap-2.5 py-0.5">
                      <IconShield size={15} style={{ marginTop: 2, flexShrink: 0, color: why ? 'var(--ink-4)' : 'var(--ink-3)' }} />
                      <div className="min-w-0 flex-1">
                        <div className="flex min-w-0 items-center gap-2">
                          <span className="t-body truncate" style={{ fontWeight: 540 }}>{r.name}</span>
                          {r.application_id && <span className="chip shrink-0" style={{ fontSize: 10 }}>{r.application_id}</span>}
                        </div>
                        {(why || r.description) && (
                          <div className="t-xs truncate">{why || r.description}</div>
                        )}
                      </div>
                      {checked && <IconCheck size={14} style={{ marginTop: 3, color: 'var(--accent)' }} />}
                    </div>
                  )
                }} />
              {d.subjectKind && shut.length > 0 && !d.role && (
                <div className="t-xs mt-1.5">
                  {shut.length} role{shut.length === 1 ? ' is' : 's are'} not available to {d.subjectKind} accounts
                  — listed last and greyed out, by the same rule the server enforces.
                </div>
              )}
              {d.role && <RoleDetail role={d.role} held={d.alreadyHeld.length} />}
            </>
          ) : (
            <>
              {/* Opened from a membership: it is the one thing already
                  decided, so it is stated rather than offered. */}
              {membershipId && d.membership && (
                <div className="t-body" style={{ fontWeight: 600 }}>{d.membership.name}</div>
              )}
              {!membershipId && <Select searchable placeholder={d.subject ? 'Search memberships…' : 'Choose a person first'}
                disabled={!d.subject} aria-label="Membership" value={d.membershipId}
                onChange={d.setMembershipId} nothingFoundMessage="No membership matches"
                data={(d.memberships ?? []).map((m) => ({ value: m.id, label: m.name }))}
                renderOption={({ option, checked }) => {
                  const m = d.memberships?.find((x) => x.id === option.value)
                  if (!m) return option.label
                  const count = m.member_count ?? m.member_ids.length
                  return (
                    <div className="flex w-full min-w-0 items-start gap-2.5 py-0.5">
                      <IconUsersGroup size={15} style={{ marginTop: 2, flexShrink: 0, color: 'var(--ink-3)' }} />
                      <div className="min-w-0 flex-1">
                        <div className="t-body truncate" style={{ fontWeight: 540 }}>{m.name}</div>
                        <div className="t-xs truncate">
                          {m.entries.length} role{m.entries.length === 1 ? '' : 's'} · {count.toLocaleString()} member{count === 1 ? '' : 's'}
                        </div>
                      </div>
                      {checked && <IconCheck size={14} style={{ marginTop: 3, color: 'var(--accent)' }} />}
                    </div>
                  )
                }} />}
              {d.membership && (
                <MembershipDetail m={d.membership} axes={d.axes}
                  member={d.heldMemberships.has(d.membership.id)} />
              )}
            </>
          )}
        </Step>

        {d.what === 'role' && (
          <>
            <Step n={++n} title="Where does it apply?" done={d.ready.where}
              hint="Choose deliberately — the widest option is not the default.">
              <Radio.Group value={d.reach ?? ''} onChange={(v) => d.setReach(v as Reach)} aria-label="Where it applies">
                <div className="grid grid-cols-1 gap-2 @lg:grid-cols-3">
                  {REACH.map((c) => {
                    const off = d.required.length > 0 && c.value !== 'places'
                    return (
                      <Radio.Card key={c.value} value={c.value} className="choice" disabled={off}>
                        <span className="flex items-center gap-2">
                          <span className="choice-icon">{c.icon}</span>
                          <span className="t-body" style={{ fontWeight: 600, color: 'var(--ink)' }}>{c.title}</span>
                        </span>
                        <span className="t-xs mt-1 block">{c.body}</span>
                      </Radio.Card>
                    )
                  })}
                </div>
              </Radio.Group>
              {d.required.length > 0 && (
                <div className="t-xs mt-2">
                  Every grant here must name a place in {namesOf(d.required)} — a grant that does not
                  never applies — so only specific places are possible.
                </div>
              )}
              {d.reach === 'everywhere' && (
                <div className="callout mt-3">
                  <IconInfoCircle size={14} style={{ color: 'var(--ink-3)', flexShrink: 0, marginTop: 2 }} />
                  <span className="t-xs">
                    <b>{d.role?.name ?? 'The role'}</b> will apply in every structure, and in every place
                    added to one later. Pick specific places if that is more than they need.
                  </span>
                </div>
              )}
              {d.reach === 'own' && (
                <div className="callout mt-3">
                  <IconInfoCircle size={14} style={{ color: 'var(--ink-3)', flexShrink: 0, marginTop: 2 }} />
                  <span className="t-xs">
                    The application has to say who owns the record it is asking about. If it does not,
                    or the owner is someone else, the answer is no.
                  </span>
                </div>
              )}
              {d.reach === 'places' && d.axes && (
                <div className="mt-3">
                  <PlacePicker axes={d.axes} value={d.places} onChange={d.setPlaces} required={d.required} />
                </div>
              )}
            </Step>
          </>
        )}

        {d.what === 'membership' && d.membership?.anchor_axis && (
          <Step n={++n} title="Where do they hold it?" done={d.ready.where}
            hint="This membership applies where each member is placed — choose the place for this person.">
            <Seat d={d} />
          </Step>
        )}

        <Step n={++n} title="For how long?" done={d.ready.when}
          hint="Access with an end date expires on its own — nobody has to remember to revoke it.">
          <Chip.Group value={d.until} onChange={(v) => d.setUntil(v as Until)}>
            <div className="flex flex-wrap gap-1.5">
              {UNTIL.map((u) => (
                <Chip key={u.value || 'none'} value={u.value} size="sm" variant="outline">{u.label}</Chip>
              ))}
            </div>
          </Chip.Group>
          {d.until === 'date' && (
            <DateInput mt="sm" maw={260} placeholder="Choose the last day" valueFormat="D MMM YYYY"
              value={d.pickedDate} onChange={d.setPickedDate} aria-label="End date"
              minDate={new Date(Date.now() + 86_400_000)} clearable />
          )}
          {d.ends && (
            <div className="t-xs mt-2">
              Ends <b className="tnum" style={{ color: 'var(--ink-2)' }}>{fmtDate(d.ends)}</b>, {relDays(d.ends)}, at the end of that day.
            </div>
          )}
        </Step>

        <Step n={++n} title="Note" done={d.reason.trim().length > 0}
          hint={d.what === 'membership'
            ? 'Optional. Why they hold it — kept with the membership and every grant it gives.'
            : 'Optional. Why this is needed — a ticket or request. Kept with the grant.'}>
          <Textarea autosize minRows={2} maxRows={5} maxLength={500} value={d.reason}
            onChange={(e) => d.setReason(e.currentTarget.value)} aria-label="Note"
            placeholder="e.g. INC-1042 — covering month-end close for Jakarta" />
        </Step>

        <Review d={d} />
      </div>
    </CreateShell>
  )
}
