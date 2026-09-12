import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button, Drawer, Tooltip } from '@mantine/core'
import { IconChevronRight, IconKey, IconLock, IconPointFilled } from '@tabler/icons-react'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { GrantRole, GrantScopes } from '@/components/ui/GrantAccess'
import type { Identity, Ial, Grant } from '@/lib/api/types'

/* Everything about one person, on one surface.
 *
 * The console could answer "who is this" (the identities row) and "who holds
 * this role" (the Access screen), but never "what can THIS person do" without
 * an operator filtering Access by hand and reading grants one at a time.
 * During an incident that is the only question being asked.
 *
 * A drawer rather than a route, and that is deliberate: identities.tsx already
 * keeps the id out of the URL because "an id in the URL is an id in someone's
 * browser history" — and a person page is exactly the URL you would otherwise
 * paste into a ticket. The cost is that it cannot be deep-linked; the benefit
 * is that opening it leaves no trace in history, the referer, or a proxy log.
 */

const IAL_HINT: Record<Ial, string> = {
  1: 'Self-asserted — email only, a self-registered applicant.',
  2: 'Remotely verified, typically through a contract or employer.',
  3: 'In-person verified with government ID on file.',
}

function Row({ label, children, hint }: {
  label: string; children: React.ReactNode; hint?: string
}) {
  return (
    <div className="flex items-baseline gap-3" style={{ minHeight: 24 }}>
      <span className="t-xs" style={{ width: 108, flexShrink: 0, opacity: 0.65 }}>{label}</span>
      {hint
        ? <Tooltip label={hint}><span className="t-body min-w-0">{children}</span></Tooltip>
        : <span className="t-body min-w-0">{children}</span>}
    </div>
  )
}

/** What the role actually permits, fetched only when asked. A person with
    eight grants would otherwise fire eight role lookups to render a drawer
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

function GrantCard({ grant, axes, memberships, nodeName }: {
  grant: Grant
  axes: Parameters<typeof GrantScopes>[0]['axes']
  memberships: Parameters<typeof GrantRole>[0]['memberships']
  nodeName: (id: string) => string
}) {
  const [open, setOpen] = useState(false)
  /* An expired grant is not revoked and not active — authorize() simply will
     not match it. Saying "expired" is the difference between an operator
     removing it and an operator wondering why it does nothing. */
  const expired = !!grant.valid_until && new Date(grant.valid_until) <= new Date()

  return (
    <div className="panel p-3" style={expired ? { opacity: 0.6 } : undefined}>
      <div className="flex items-start justify-between gap-3">
        <GrantRole grant={grant} memberships={memberships} />
        <div className="t-xs" style={{ textAlign: 'right', flexShrink: 0, opacity: 0.7 }}>
          <div className="tnum">from {grant.valid_from.slice(0, 10)}</div>
          <div className="tnum" style={expired ? { color: 'var(--warn)' } : undefined}>
            {grant.valid_until ? `${expired ? 'expired' : 'until'} ${grant.valid_until.slice(0, 10)}` : 'no expiry'}
          </div>
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
        <IconChevronRight size={12} style={{ transform: open ? 'rotate(90deg)' : undefined, transition: 'transform .12s' }} />
        what this role permits
      </button>
      {open && <div className="mt-2"><RolePermissions roleId={grant.role_id} /></div>}
    </div>
  )
}

export function PersonDetail({ person, onClose, onAttributes, onCredentials }: {
  person: Identity | null
  onClose: () => void
  onAttributes: () => void
  onCredentials: () => void
}) {
  const id = person?.id ?? ''

  /* Not paged: this is one person's access, and a person with more grants
     than a drawer can hold is itself the finding. 200 is the server's cap. */
  const { data: access, isLoading: accessLoading } = useQuery({
    queryKey: ['person-access', id],
    queryFn: () => api.searchGrants({ identityId: id, pageSize: 200 }),
    enabled: !!id,
  })
  const grants = access?.rows ?? []

  const { data: axes } = useQuery({ queryKey: qk.axes(), queryFn: api.axes, enabled: !!id })
  const { data: memberships } = useQuery({ queryKey: qk.memberships(), queryFn: api.memberships, enabled: !!id })
  const { data: realms } = useQuery({ queryKey: qk.realms(), queryFn: api.realms, enabled: !!id })
  const { data: creds } = useQuery({
    queryKey: qk.credentials(id), queryFn: () => api.credentials(id), enabled: !!id,
  })

  /* Same batch-resolve the Access screen uses: the names for the scopes on
     THIS drawer, not every node of every axis. */
  const scopeIds = [...new Set(grants.flatMap((g) => g.scopes.map((s) => s.scope_node_id)))].sort()
  const { data: nodes } = useQuery({
    queryKey: ['scope-names', scopeIds],
    queryFn: () => api.scopeNodesByIds(scopeIds),
    enabled: scopeIds.length > 0,
  })
  const nodeName = (nid: string) => nodes?.find((n) => n.id === nid)?.name ?? nid

  const realm = realms?.find((r) => r.id === person?.realm_id)
  const live = creds?.filter((c) => !c.revoked_at) ?? []

  return (
    <Drawer
      opened={person !== null}
      onClose={onClose}
      position="right"
      size={620}
      overlayProps={{ blur: 2, backgroundOpacity: 0.45, color: 'var(--overlay-tint)' }}
      styles={{
        content: { background: 'var(--s-raised)' },
        header: { background: 'var(--s-raised)', borderBottom: '1px solid var(--line)', padding: '14px 20px' },
        title: { fontSize: 15, fontWeight: 640, letterSpacing: '-.01em' },
      }}
      title={person?.username ?? ''}
    >
      {person && (
        <div className="flex flex-col gap-4">
          {person.status !== 'active' && (
            <div className="panel p-3" style={{ borderColor: 'color-mix(in srgb, var(--warn) 24%, transparent)' }}>
              <div className="t-body">
                This identity is <strong>{person.status}</strong>.
              </div>
              {/* The list page says the same thing when toggling status, and it
                  is the single most load-bearing fact on this screen: the
                  grants below are all inert. */}
              <div className="t-xs mt-1" style={{ opacity: 0.75 }}>
                authorize() gates on identity state — every grant below is dead
                until it is re-enabled.
              </div>
            </div>
          )}

          <div className="panel p-4 flex flex-col gap-2">
            <div className="t-label mb-1">Identity</div>
            <Row label="Username">{person.username}</Row>
            <Row label="Email">{person.email || <span style={{ opacity: 0.5 }}>none</span>}</Row>
            <Row label="Status">
              <span className={`v-pill ${person.status === 'active' ? 'v-pill-allow' : 'v-pill-idle'}`}>
                <IconPointFilled size={8} />{person.status}
              </span>
            </Row>
            <Row label="Population">
              {realm ? `${realm.display_name} (${realm.code})` : <span style={{ opacity: 0.5 }}>—</span>}
            </Row>
            <Row label="Assurance" hint={IAL_HINT[person.assurance_level]}>
              IAL{person.assurance_level}
            </Row>
            <Row label="Created"><span className="tnum">{person.created_at.slice(0, 10)}</span></Row>
            <Row label="Last sign-in">
              {person.last_login_at
                ? <span className="tnum">{person.last_login_at.slice(0, 10)}</span>
                : <span style={{ opacity: 0.5 }}>never</span>}
            </Row>
            {person.external_ref && <Row label="External ref"><span className="chip">{person.external_ref}</span></Row>}
            {/* Bumping this is how issued tokens are invalidated, so it is
                worth seeing rather than inferring from an audit line. */}
            <Row label="Token epoch" hint="Access tokens minted before this number are refused.">
              <span className="tnum">{person.token_epoch}</span>
            </Row>
          </div>

          {(person.disabled_at || person.retention_until || person.anonymized_at) && (
            <div className="panel p-4 flex flex-col gap-2">
              <div className="t-label mb-1">Lifecycle</div>
              {person.disabled_at && <Row label="Disabled"><span className="tnum">{person.disabled_at.slice(0, 10)}</span></Row>}
              {person.retention_until && <Row label="Retention"><span className="tnum">{person.retention_until.slice(0, 10)}</span></Row>}
              {person.anonymized_at && <Row label="Anonymised"><span className="tnum">{person.anonymized_at.slice(0, 10)}</span></Row>}
            </div>
          )}

          <div>
            <div className="flex items-baseline justify-between mb-2">
              <div className="t-label">Access</div>
              <div className="t-xs" style={{ opacity: 0.6 }}>
                {accessLoading ? 'loading…' : `${grants.length} grant${grants.length === 1 ? '' : 's'}`}
              </div>
            </div>

            {!accessLoading && grants.length === 0 ? (
              <div className="panel p-4 text-center">
                <div className="t-body">No grants.</div>
                <div className="t-xs mt-1" style={{ opacity: 0.7 }}>
                  This person can sign in and do nothing — authorize() denies
                  everything without a grant.
                </div>
              </div>
            ) : (
              <div className="flex flex-col gap-2">
                {grants.map((g) => (
                  <GrantCard key={g.id} grant={g} axes={axes}
                    memberships={memberships} nodeName={nodeName} />
                ))}
              </div>
            )}
          </div>

          <div className="panel p-4">
            <div className="t-label mb-2">How they sign in</div>
            {live.length === 0 ? (
              <div className="t-xs" style={{ color: 'var(--warn)' }}>
                No usable credential — this identity cannot authenticate.
              </div>
            ) : (
              <div className="flex flex-wrap gap-1.5">
                {live.map((c) => (
                  <span key={c.id} className="chip">
                    {c.kind === 'password' ? <IconLock size={9} style={{ marginRight: 4 }} />
                      : <IconKey size={9} style={{ marginRight: 4 }} />}
                    {c.kind}{c.label ? ` · ${c.label}` : ''}
                  </span>
                ))}
              </div>
            )}
            <div className="flex gap-2 mt-3">
              <Button size="xs" variant="default" onClick={onCredentials}>Manage credentials</Button>
              <Button size="xs" variant="default" onClick={onAttributes}>Attributes</Button>
            </div>
          </div>
        </div>
      )}
    </Drawer>
  )
}
