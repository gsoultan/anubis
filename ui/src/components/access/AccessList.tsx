import { useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ActionIcon, Button, Menu, TextInput, Tooltip, UnstyledButton } from '@mantine/core'
import { notifications } from '@mantine/notifications'
import {
  IconChevronRight, IconCirclePlus, IconCopy, IconDots, IconShield, IconShieldOff,
  IconUserMinus, IconUsersGroup,
} from '@tabler/icons-react'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/client'
import { daysFrom, fmtDate, missingRequired, namesOf, relDays } from '@/lib/access'
import { notifyRejected } from '@/components/create/shell'
import { ConfirmModal } from '@/components/ui/ConfirmModal'
import { GrantWhere } from './GrantWhere'
import type { Grant, Membership, MembershipAssignment, Role, ScopeAxis } from '@/lib/api/types'

/* A person's access, where it can be changed.
 *
 * Grouped by where each grant came from, because that decides how it is taken
 * away: a grant given directly is revoked on its own; a grant a membership
 * gave is removed by leaving the membership, all of its grants at once, and
 * revoking just one would be written straight back. The old cards said
 * "managed by … — remove the person there" and sent the operator to a screen
 * whose only way to remove somebody was to search for them again.
 *
 * A membership is listed once per place it is held at. Each is its own
 * assignment with its own end date, and removing one leaves the others.
 */

function Permits({ roleId }: { roleId: string }) {
  const [open, setOpen] = useState(false)
  /* Fetched when asked: eight grants would otherwise be eight role lookups to
     render a list nobody has expanded. */
  const { data, isLoading } = useQuery({
    queryKey: qk.rolePermissions(roleId),
    queryFn: () => api.rolePermissions(roleId),
    enabled: open,
  })
  return (
    <div className="mt-2">
      <UnstyledButton className="t-xs inline-flex items-center gap-1" onClick={() => setOpen((o) => !o)}
        aria-expanded={open}>
        <IconChevronRight size={12}
          style={{ transform: open ? 'rotate(90deg)' : undefined, transition: 'transform var(--t-fast)' }} />
        What this role permits
      </UnstyledButton>
      {open && (
        <div className="mt-1.5 flex flex-wrap gap-1">
          {isLoading && <span className="t-xs">Loading…</span>}
          {data?.length === 0 && (
            <span className="t-xs" style={{ color: 'var(--warn)' }}>Nothing — this role permits nothing yet.</span>
          )}
          {data?.map((k) => <span key={k} className="chip">{k}</span>)}
        </div>
      )}
    </div>
  )
}

function Row({ g, role, axes, nodeName, onRevoke }: {
  g: Grant
  role: Role | undefined
  axes: ScopeAxis[] | undefined
  nodeName: (id: string) => string
  /** Absent for a grant a membership gave: that is taken away by leaving it. */
  onRevoke?: (() => void) | undefined
}) {
  /* An expired grant is not revoked and not active — authorize() simply will
     not match it. Saying so is the difference between an operator removing it
     and one wondering why it does nothing. */
  const days = g.valid_until ? daysFrom(g.valid_until) : null
  const expired = !!g.valid_until && new Date(g.valid_until) <= new Date()
  const soon = !expired && days !== null && days <= 14
  const dead = missingRequired(g.scopes, axes)
  const ends = g.valid_until
    ? `${expired ? 'Ended' : 'Ends'} ${fmtDate(g.valid_until)}`
    : 'No end date'
  const since = g.valid_until && !expired ? relDays(g.valid_until) : `since ${fmtDate(g.valid_from)}`
  const endsTone = { color: expired || soon ? 'var(--warn)' : 'var(--ink-2)', fontWeight: 540 }

  return (
    <div className="access-row" data-expired={expired ? '' : undefined}>
      <span className="access-icon" aria-hidden><IconShield size={15} /></span>
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <span className="t-body" style={{ fontWeight: 600 }}>{g.role_name}</span>
          {role?.application_id && <span className="chip">{role.application_id}</span>}
          {expired && <span className="v-pill v-pill-idle">expired</span>}
          {dead.length > 0 && !expired && (
            <Tooltip label={`authorize() drops any grant that names no place in ${namesOf(dead)}.`}>
              <span className="v-pill v-pill-warn" style={{ cursor: 'help' }}>has no effect</span>
            </Tooltip>
          )}
        </div>
        {role?.description && <div className="t-xs mt-0.5 truncate">{role.description}</div>}
        {/* In a narrow list the dates ride under the name; beside it they
            squeezed every place name onto two lines. */}
        <div className="access-when-inline t-xs tnum mt-1">
          <span style={endsTone}>{ends}</span> · {since}
        </div>
        <div className="mt-2.5">
          <GrantWhere scopes={g.scopes} selfScoped={g.self_scoped} axes={axes} nodeName={nodeName} />
        </div>
        {dead.length > 0 && !expired && (
          <div className="t-xs mt-2" style={{ color: 'var(--warn)' }}>
            Every grant must name a place in {namesOf(dead)}. This one does not, so it never applies —
            give the role again with a place there, then revoke this one.
          </div>
        )}
        {g.reason && <div className="t-xs mt-2" style={{ fontStyle: 'italic' }}>“{g.reason}”</div>}
        <Permits roleId={g.role_id} />
      </div>

      <div className="flex shrink-0 items-start gap-1.5">
        <div className="access-when text-right">
          <div className="t-xs tnum" style={endsTone}>{ends}</div>
          <div className="t-xs tnum">{since}</div>
        </div>
        <Menu position="bottom-end" width={240}>
          <Menu.Target>
            <ActionIcon variant="subtle" color="gray" aria-label={`Actions for ${g.role_name}`}>
              <IconDots size={15} />
            </ActionIcon>
          </Menu.Target>
          <Menu.Dropdown>
            <Menu.Item leftSection={<IconCopy size={14} />}
              onClick={() => {
                void navigator.clipboard.writeText(g.id)
                notifications.show({ color: 'gray', title: 'Grant ID copied', message: g.id })
              }}>
              Copy grant ID
            </Menu.Item>
            {onRevoke ? (
              <>
                <Menu.Divider />
                <Menu.Item color="deny" leftSection={<IconShieldOff size={14} />} onClick={onRevoke}>
                  Revoke access…
                </Menu.Item>
              </>
            ) : (
              <Menu.Label style={{ textTransform: 'none', letterSpacing: 0, fontWeight: 500 }}>
                Given by a membership — remove them from it to take this away.
              </Menu.Label>
            )}
          </Menu.Dropdown>
        </Menu>
      </div>
    </div>
  )
}

function Group({ icon, title, sub, count, action, children }: {
  icon: ReactNode
  title: ReactNode
  /** A second line under the title: where and until when it is held. */
  sub?: ReactNode
  count: number
  action?: ReactNode
  children: ReactNode
}) {
  return (
    <div className="access-section">
      <div className="access-group">
        <span className="min-w-0">
          <span className="flex min-w-0 items-center gap-2">
            <span className="flex" style={{ color: 'var(--ink-3)' }}>{icon}</span>
            <span className="t-body truncate" style={{ fontWeight: 580 }}>{title}</span>
            <span className="count-pill">{count}</span>
          </span>
          {sub && <span className="t-xs mt-0.5 block truncate" style={{ paddingLeft: 22 }}>{sub}</span>}
        </span>
        {/* The way out never gives way to a long name: the name truncates. */}
        {action && <span className="shrink-0">{action}</span>}
      </div>
      {children}
    </div>
  )
}

/* Where and until when one assignment is held, in a line. */
function heldLine(a: MembershipAssignment): string {
  const where = a.place_name
    ? (a.exact ? `Only ${a.place_name} itself` : `${a.place_name} and everything inside`)
    : 'Where the membership says'
  const until = a.valid_until ? `until ${fmtDate(a.valid_until)}` : 'no end date'
  return [where, until, a.reason && `“${a.reason}”`].filter(Boolean).join(' · ')
}

export function AccessList({ identityId, username, grants, loading, axes, roles, memberships, nodeName, onGive }: {
  identityId: string
  username: string
  grants: Grant[]
  loading: boolean
  axes: ScopeAxis[] | undefined
  roles: Role[] | undefined
  memberships: Membership[] | undefined
  nodeName: (id: string) => string
  onGive: () => void
}) {
  const [revoking, setRevoking] = useState<Grant | null>(null)
  /* Why it is being taken away — kept beside why it was given, not over it
     (0053). Optional: most revokes explain themselves. */
  const [revokeWhy, setRevokeWhy] = useState('')
  const [leaving, setLeaving] = useState<{ a: MembershipAssignment; roles: string[] } | null>(null)
  const [leaveWhy, setLeaveWhy] = useState('')
  /* Their assignments, not their grants: a membership that gives no roles yet
     leaves no grant to read it off, and they are in it all the same. */
  const { data: held } = useQuery({
    queryKey: qk.membershipAssignments({ identityId }),
    queryFn: () => api.membershipAssignments({ identityId, pageSize: 200 }),
  })
  const assignments = held?.rows ?? []

  /* Live grants first; an expired one is history, not access. */
  const order = (a: Grant, b: Grant) => {
    const ea = !!a.valid_until && new Date(a.valid_until) <= new Date()
    const eb = !!b.valid_until && new Date(b.valid_until) <= new Date()
    return Number(ea) - Number(eb) || a.role_name.localeCompare(b.role_name)
  }
  const direct = grants.filter((g) => !g.via_membership_id).sort(order)
  const grantsOf = (a: MembershipAssignment) => grants.filter((g) => g.via_assignment_id === a.id).sort(order)
  /* Grants a membership gave that match no current assignment: a page of
     assignments still loading, or one that ended a moment ago. Shown, so the
     count on the page never disagrees with the rows under it. */
  const known = new Set(assignments.map((a) => a.id))
  const unplaced = grants.filter((g) => g.via_membership_id && !(g.via_assignment_id && known.has(g.via_assignment_id)))
  const roleOf = (id: string) => roles?.find((r) => r.id === id)
  const membershipName = (id: string) => memberships?.find((m) => m.id === id)?.name ?? 'A membership'

  const refresh = async () => {
    await queryClient.invalidateQueries({ queryKey: ['grants'] })
    await queryClient.invalidateQueries({ queryKey: qk.memberships() })
    await queryClient.invalidateQueries({ queryKey: qk.dashboard() })
  }

  async function revoke(g: Grant, why: string) {
    try {
      await api.revokeGrant(g.id, why)
    } catch (e) {
      notifyRejected(e)
      throw e
    }
    notifications.show({
      color: 'orange', title: 'Access revoked',
      message: `${username} no longer holds ${g.role_name}. Tokens already issued lapse within 15 minutes.`,
    })
    await refresh()
  }

  async function leave(a: MembershipAssignment, why: string) {
    let n: number
    try {
      n = await api.removeAssignment(a.id, why)
    } catch (e) {
      notifyRejected(e)
      throw e
    }
    notifications.show({
      color: 'orange', title: 'Removed from membership',
      message: `${username} left “${a.membership_name}”${a.place_name ? ` at ${a.place_name}` : ''} — ${n} grant${n === 1 ? '' : 's'} revoked.`,
    })
    await refresh()
  }

  const live = grants.filter((g) => !g.valid_until || new Date(g.valid_until) > new Date())
  // Memberships, not places: one held at two offices is still one membership.
  const heldCount = new Set(assignments.map((a) => a.membership_id)).size
  const timeLimited = live.filter((g) => g.valid_until).length
  const summary = loading ? 'Loading…'
    : grants.length === 0 && assignments.length === 0 ? 'Nothing yet'
    : [
        `${live.length} active grant${live.length === 1 ? '' : 's'}`,
        heldCount > 0 && `${heldCount} membership${heldCount === 1 ? '' : 's'}`,
        timeLimited > 0 && `${timeLimited} with an end date`,
      ].filter(Boolean).join(' · ')

  return (
    <section className="@container panel overflow-clip" aria-labelledby="access-title">
      <div className="panel-head">
        <div className="min-w-0">
          <h2 id="access-title" className="t-h1">Access</h2>
          <div className="t-xs mt-0.5">What {username} can do, and where.</div>
        </div>
        <span className="t-xs tnum shrink-0">{summary}</span>
      </div>

      {loading ? (
        <div className="flex flex-col">
          {[0, 1].map((i) => (
            <div key={i} className="access-row">
              <span className="access-icon" />
              <div className="flex-1">
                <div className="animate-pulse rounded" style={{ height: 11, width: '34%', background: 'var(--line)' }} />
                <div className="animate-pulse mt-3 rounded" style={{ height: 9, width: '58%', background: 'var(--line)' }} />
              </div>
            </div>
          ))}
        </div>
      ) : grants.length === 0 && assignments.length === 0 ? (
        <div className="px-6 py-12 text-center">
          <span className="empty-icon"><IconShield size={20} /></span>
          <div className="t-h2 mt-3">No access yet</div>
          <div className="t-sm mx-auto mt-1.5" style={{ maxWidth: 380 }}>
            {username} can sign in and do nothing: every decision is denied until they hold a role.
          </div>
          <Button size="xs" className="mt-4" leftSection={<IconCirclePlus size={14} />} onClick={onGive}>
            Give access
          </Button>
        </div>
      ) : (
        <>
          {direct.length > 0 && (
            <Group icon={<IconShield size={14} />} title="Given directly" count={direct.length}>
              {direct.map((g) => (
                <Row key={g.id} g={g} role={roleOf(g.role_id)} axes={axes} nodeName={nodeName}
                  onRevoke={() => setRevoking(g)} />
              ))}
            </Group>
          )}
          {assignments.map((a) => {
            const list = grantsOf(a)
            return (
              <Group key={a.id} icon={<IconUsersGroup size={14} />}
                title={<>Through <span style={{ color: 'var(--ink)' }}>{a.membership_name}</span></>}
                sub={heldLine(a)} count={list.length}
                action={
                  <Button size="compact-xs" variant="subtle" color="deny"
                    leftSection={<IconUserMinus size={13} />}
                    onClick={() => setLeaving({ a, roles: list.map((g) => g.role_name) })}>
                    Remove
                  </Button>
                }>
                {list.length === 0 && (
                  <div className="access-row">
                    <span className="t-xs">This membership gives no roles yet. They are in it, and will get what it gives when it does.</span>
                  </div>
                )}
                {list.map((g) => (
                  <Row key={g.id} g={g} role={roleOf(g.role_id)} axes={axes} nodeName={nodeName} />
                ))}
              </Group>
            )
          })}
          {unplaced.length > 0 && (
            <Group icon={<IconUsersGroup size={14} />} title="Through a membership" count={unplaced.length}
              sub={[...new Set(unplaced.map((g) => membershipName(g.via_membership_id as string)))].join(', ')}>
              {unplaced.sort(order).map((g) => (
                <Row key={g.id} g={g} role={roleOf(g.role_id)} axes={axes} nodeName={nodeName} />
              ))}
            </Group>
          )}
        </>
      )}

      <ConfirmModal opened={!!revoking} onClose={() => { setRevoking(null); setRevokeWhy('') }}
        title="Revoke this access?" confirmLabel="Revoke access"
        onConfirm={() => (revoking ? revoke(revoking, revokeWhy) : Promise.resolve())}>
        <p>
          <b style={{ color: 'var(--ink)' }}>{username}</b> will no longer hold{' '}
          <b style={{ color: 'var(--ink)' }}>{revoking?.role_name}</b>. New decisions are denied at once;
          tokens already issued lapse within 15 minutes.
        </p>
        <TextInput label="Why" description="Optional. Kept with the grant, beside why it was given."
          placeholder="e.g. moved to another team" maxLength={200} data-autofocus
          value={revokeWhy} onChange={(e) => setRevokeWhy(e.currentTarget.value)} />
        <p className="t-xs">This revokes one grant. The person, and any other access they hold, stay as they are.</p>
      </ConfirmModal>

      <ConfirmModal opened={!!leaving} onClose={() => { setLeaving(null); setLeaveWhy('') }}
        title="Remove from this membership?" confirmLabel="Remove"
        onConfirm={() => (leaving ? leave(leaving.a, leaveWhy) : Promise.resolve())}>
        <p>
          <b style={{ color: 'var(--ink)' }}>{username}</b> will leave{' '}
          <b style={{ color: 'var(--ink)' }}>{leaving?.a.membership_name}</b>
          {leaving?.a.place_name && <> at <b style={{ color: 'var(--ink)' }}>{leaving.a.place_name}</b></>}
          {leaving && leaving.roles.length > 0
            ? <> and lose what it gave them there: {leaving.roles.join(', ')}.</>
            : '.'}
        </p>
        <TextInput label="Why" description="Optional. Kept with each grant this takes away."
          placeholder="e.g. rotated off the council" maxLength={200} data-autofocus
          value={leaveWhy} onChange={(e) => setLeaveWhy(e.currentTarget.value)} />
        <p className="t-xs">
          {leaving?.a.place_name
            ? `Only this place. Anywhere else ${username} holds this membership stays, and so do grants given directly.`
            : `Other members keep their access. Grants given to ${username} directly are not touched.`}
        </p>
      </ConfirmModal>
    </section>
  )
}
