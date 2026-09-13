import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ActionIcon, Menu } from '@mantine/core'
import { IconChevronRight, IconDots, IconTrash } from '@tabler/icons-react'
import { notifications } from '@mantine/notifications'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/client'
import { GrantRole, GrantScopes } from '@/components/ui/GrantAccess'
import type { Grant, Membership, ScopeAxis } from '@/lib/api/types'

/** What the role actually permits, fetched only when asked. A person with
    eight grants would otherwise fire eight role lookups to render a list
    nobody has scrolled yet. */
function RolePermissions({ roleId }: { roleId: string }) {
  const { data, isLoading } = useQuery({
    queryKey: qk.rolePermissions(roleId),
    queryFn: () => api.rolePermissions(roleId),
  })
  if (isLoading) return <div className="t-xs" style={{ opacity: 0.6 }}>Loading…</div>
  if (!data || data.length === 0) {
    return (
      <div className="t-xs" style={{ color: 'var(--warn)' }}>
        This role grants no permissions — the grant confers nothing.
      </div>
    )
  }
  return (
    <div className="flex flex-wrap gap-1">
      {data.map((k) => <span key={k} className="chip">{k}</span>)}
    </div>
  )
}

/* One grant, as a card: the role, where it applies, when it runs out, and the
   way to take it back. Revoking used to be reachable only from the Access
   table — so the one screen that showed a person's whole access could not
   change any of it, and an operator had to go and find the row again
   somewhere else. */
export function GrantCard({ grant, axes, memberships, nodeName, username }: {
  grant: Grant
  axes: ScopeAxis[] | undefined
  memberships: Membership[] | undefined
  nodeName: (id: string) => string
  username: string
}) {
  const [open, setOpen] = useState(false)
  const [revoking, setRevoking] = useState(false)
  /* An expired grant is not revoked and not active — authorize() simply will
     not match it. Saying "expired" is the difference between an operator
     removing it and an operator wondering why it does nothing. */
  const expired = !!grant.valid_until && new Date(grant.valid_until) <= new Date()
  const viaMembership = memberships?.find((x) => x.id === grant.via_membership_id)

  async function revoke() {
    setRevoking(true)
    try {
      await api.revokeGrant(grant.id)
      notifications.show({
        color: 'orange', title: 'Grant revoked',
        message: `${username} no longer holds ${grant.role_name}. Access tokens die at their TTL (≤15 min).`,
      })
      await queryClient.invalidateQueries({ queryKey: ['grants'] })
      await queryClient.invalidateQueries({ queryKey: qk.dashboard() })
    } finally {
      setRevoking(false)
    }
  }

  return (
    <div className="panel panel-hover p-3.5"
      style={{ opacity: expired || revoking ? 0.55 : undefined }}>
      <div className="flex items-start justify-between gap-3">
        <GrantRole grant={grant} memberships={memberships} />
        <div className="flex shrink-0 items-start gap-2">
          <div className="t-xs" style={{ textAlign: 'right', opacity: 0.7 }}>
            <div className="tnum">from {grant.valid_from.slice(0, 10)}</div>
            <div className="tnum" style={expired ? { color: 'var(--warn)' } : undefined}>
              {grant.valid_until
                ? `${expired ? 'expired' : 'until'} ${grant.valid_until.slice(0, 10)}`
                : 'no expiry'}
            </div>
          </div>
          <Menu position="bottom-end" width={260} shadow="xl">
            <Menu.Target>
              <ActionIcon variant="subtle" color="gray"
                aria-label={`Actions for the ${grant.role_name} grant`}>
                <IconDots size={15} />
              </ActionIcon>
            </Menu.Target>
            <Menu.Dropdown>
              {/* A derived grant cannot be revoked on its own: the membership
                  would write it straight back. Say where it is managed rather
                  than offering an action that loses. */}
              {grant.via_membership_id ? (
                <Menu.Item disabled leftSection={<IconTrash size={14} />}>
                  Managed by “{viaMembership?.name ?? 'a membership'}” — remove the person there
                </Menu.Item>
              ) : (
                <>
                  <Menu.Label>This revokes access, not the identity</Menu.Label>
                  <Menu.Item color="red" leftSection={<IconTrash size={14} />}
                    onClick={() => void revoke()}>
                    Revoke this grant
                  </Menu.Item>
                </>
              )}
            </Menu.Dropdown>
          </Menu>
        </div>
      </div>

      <div className="mt-2.5">
        <GrantScopes grant={grant} axes={axes} nodeName={nodeName} maxSilent={3} />
      </div>

      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="t-xs mt-2.5 flex items-center gap-1"
        style={{ opacity: 0.7, background: 'none', border: 'none', padding: 0, cursor: 'pointer' }}
      >
        <IconChevronRight size={12}
          style={{ transform: open ? 'rotate(90deg)' : undefined, transition: 'transform .12s' }} />
        what this role permits
      </button>
      {open && <div className="mt-2"><RolePermissions roleId={grant.role_id} /></div>}
    </div>
  )
}

/** The same card's shape while the list is still loading, so the section does
    not jump a hundred pixels the moment the grants arrive. */
export function GrantCardSkeleton() {
  return (
    <div className="panel p-3.5">
      <div className="animate-pulse rounded" style={{ height: 11, width: '38%', background: 'var(--line)' }} />
      <div className="animate-pulse mt-3 rounded" style={{ height: 9, width: '62%', background: 'var(--line)' }} />
    </div>
  )
}

