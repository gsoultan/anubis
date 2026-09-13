import { useState } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { ActionIcon, Button, Menu, Tooltip } from '@mantine/core'
import { notifications } from '@mantine/notifications'
import {
  IconCirclePlus, IconCopy, IconDots, IconKey, IconLock, IconPointFilled,
  IconRefreshAlert, IconUserCheck, IconUserOff, IconX,
} from '@tabler/icons-react'
import { Page } from '@/components/shell/Page'
import { Initial } from '@/components/ui/Initial'
import { GrantCard, GrantCardSkeleton } from '@/components/ui/GrantCard'
import { AttributesModal } from '@/components/ui/AttributesModal'
import { CredentialsModal } from '@/components/ui/CredentialsModal'
import { GrantFields, useGrantDraft } from '@/components/create/GrantFields'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/client'
import { realmKindColor } from '@/lib/realmKind'
import type { Ial } from '@/lib/api/types'

/* A person's own page.
 *
 * This used to be a 620px drawer over the People list. Everything an operator
 * comes here to do — read the access, understand why it is shaped that way,
 * add to it, take some away — was happening in a column narrower than the
 * table it was covering, and the one action they came for (give access) threw
 * a SECOND drawer on top of the first. A record you can act on is a page.
 *
 * The id is in the address, which the drawer deliberately avoided. That was
 * the right call for the encrypted attributes and the credentials — those
 * stay behind modals — but a username in a URL is what makes this page
 * linkable from a ticket, and an operator's browser history is not the threat
 * model that a ULID belongs to.
 */

export const Route = createFileRoute('/identities_/$id')({
  component: PersonPage,
  /* `?give=1` opens the grant panel already expanded, so "Give access…" from
     the People row menu lands on the form rather than next to it. */
  validateSearch: (s: Record<string, unknown>): { give?: true } =>
    s['give'] === true || s['give'] === '1' || s['give'] === 'true' ? { give: true } : {},
})

const IAL_HINT: Record<Ial, string> = {
  1: 'Self-asserted — email only, a self-registered applicant.',
  2: 'Remotely verified, typically through a contract or employer.',
  3: 'In-person verified with government ID on file.',
}
const IAL_COLOR: Record<Ial, string> = {
  1: 'var(--warn)', 2: 'var(--info)', 3: 'var(--allow)',
}

function Row({ label, children, hint }: {
  label: string; children: React.ReactNode; hint?: string
}) {
  return (
    <div className="flex items-baseline gap-3" style={{ minHeight: 24 }}>
      <span className="t-xs" style={{ width: 104, flexShrink: 0, opacity: 0.65 }}>{label}</span>
      {hint
        ? <Tooltip label={hint}><span className="t-body min-w-0">{children}</span></Tooltip>
        : <span className="t-body min-w-0">{children}</span>}
    </div>
  )
}

/** A number worth reading at a glance, sized so three of them fit a rail.
    Not `Stat` — that one is dashboard furniture, with a sparkline and a trend
    this page has nothing true to put in. */
function Metric({ label, value, sub, tone }: {
  label: string; value: string; sub?: string | undefined; tone?: string | undefined
}) {
  return (
    <div className="panel px-3.5 py-3">
      <div className="t-label">{label}</div>
      <div className="mt-1.5 tnum" style={{
        fontSize: 20, fontWeight: 600, letterSpacing: '-.02em', lineHeight: 1.1,
        color: tone ?? 'var(--ink)',
      }}>
        {value}
      </div>
      {sub && <div className="t-xs mt-1 truncate">{sub}</div>}
    </div>
  )
}

function PersonPage() {
  const { id } = Route.useParams()
  const { give } = Route.useSearch()
  const navigate = useNavigate()

  /* The panel's open/closed state is the URL, not component state. It has to
     survive a refresh (that is the whole point of `?give=1` from the row
     menu), it has to reset when the operator moves to a different person on
     the same route, and Back should close it rather than leave the page. */
  const giving = give === true
  const [attrsOpen, setAttrsOpen] = useState(false)
  const [credsOpen, setCredsOpen] = useState(false)

  const { data: person, isLoading, isError, error } = useQuery({
    queryKey: qk.identity(id), queryFn: () => api.identity(id),
  })

  /* Not paged: this is one person's access, and a person with more grants than
     a page can hold is itself the finding. 200 is the server's cap.

     Keyed under 'grants' rather than a key of its own, so the same
     invalidation that refreshes the Access screen refreshes this list — a
     grant given here used to stay invisible until a reload. */
  const { data: access, isLoading: accessLoading } = useQuery({
    queryKey: qk.grants(id),
    queryFn: () => api.searchGrants({ identityId: id, pageSize: 200 }),
  })
  const grants = access?.rows ?? []

  const { data: axes } = useQuery({ queryKey: qk.axes(), queryFn: api.axes })
  const { data: memberships } = useQuery({ queryKey: qk.memberships(), queryFn: api.memberships })
  const { data: realms } = useQuery({ queryKey: qk.realms(), queryFn: api.realms })
  const { data: creds, isLoading: credsLoading } = useQuery({
    queryKey: qk.credentials(id), queryFn: () => api.credentials(id),
  })
  /* Scoped to this person's population, not the tenant: category codes are
     unique per realm, and one realm's handful is all this page can show. */
  const realmId = person?.realm_id ?? ''
  const { data: categories } = useQuery({
    queryKey: qk.realmCategories(realmId),
    queryFn: () => api.realmCategories(realmId),
    enabled: !!realmId,
  })

  /* Same batch-resolve the Access screen uses: the names for the scopes on
     THIS page, not every node of every axis. */
  const scopeIds = [...new Set(grants.flatMap((g) => g.scopes.map((s) => s.scope_node_id)))].sort()
  const { data: nodes } = useQuery({
    queryKey: ['scope-names', scopeIds],
    queryFn: () => api.scopeNodesByIds(scopeIds),
    enabled: scopeIds.length > 0,
  })
  const nodeName = (nid: string) => nodes?.find((n) => n.id === nid)?.name ?? nid

  const openGive = () =>
    void navigate({ to: '/identities/$id', params: { id }, search: { give: true } })
  const closeGive = () =>
    void navigate({ to: '/identities/$id', params: { id }, search: {}, replace: true })
  const draft = useGrantDraft({ identityId: id, onCreated: closeGive })
  const cancelGive = () => { draft.reset(); closeGive() }

  if (isLoading) {
    return (
      <Page title="…" back={{ to: '/identities', label: 'People' }}>
        <div className="flex flex-col gap-3">
          <GrantCardSkeleton /><GrantCardSkeleton />
        </div>
      </Page>
    )
  }
  if (isError || !person) {
    return (
      <Page title="Person not found" back={{ to: '/identities', label: 'People' }}
        description="This identity does not exist in the tenant you are administering — it may have been deleted, or belong to another one.">
        <div className="panel px-6 py-10 text-center">
          <div className="t-sm" style={{ color: 'var(--ink-3)' }}>
            {error instanceof Error ? error.message : `No identity with id ${id}.`}
          </div>
        </div>
      </Page>
    )
  }

  const realm = realms?.find((r) => r.id === person.realm_id)
  const kindColour = realmKindColor(realm?.kind)
  const live = creds?.filter((c) => !c.revoked_at) ?? []
  const active = person.status === 'active'
  const expiring = grants.filter((g) =>
    !!g.valid_until && new Date(g.valid_until) > new Date()).length
  const derived = grants.filter((g) => g.via_membership_id).length

  async function toggleStatus() {
    if (!person) return
    const next = active ? 'disabled' : 'active'
    await api.setIdentityStatus(person.id, next)
    notifications.show({
      color: active ? 'orange' : 'teal',
      title: active ? 'Identity disabled' : 'Identity re-enabled',
      message: active
        ? 'authorize() gates on identity state — every grant is dead until re-enabled.'
        : 'Grants apply again immediately.',
    })
    // 'identities' is the prefix of both the list and this page's detail key.
    await queryClient.invalidateQueries({ queryKey: ['identities'] })
  }

  async function bumpEpoch() {
    if (!person) return
    const n = await api.bumpTokenEpoch(person.id)
    notifications.show({
      color: 'orange', title: 'Tokens invalidated',
      message: `Epoch is now ${n}. Every access token minted before this is refused.`,
    })
    await queryClient.invalidateQueries({ queryKey: ['identities'] })
  }

  return (
    <Page
      wide
      back={{ to: '/identities', label: 'People' }}
      lead={<Initial name={person.username} colour={kindColour} size={44} />}
      title={person.username}
      badge={
        <>
          <span className={`v-pill ${active ? 'v-pill-allow' : 'v-pill-deny'}`}>
            <IconPointFilled size={8} />{person.status}
          </span>
          <Tooltip label={IAL_HINT[person.assurance_level]} withArrow>
            <span className="chip" style={{ color: IAL_COLOR[person.assurance_level], cursor: 'help' }}>
              IAL{person.assurance_level}
            </span>
          </Tooltip>
        </>
      }
      description={
        <>
          {person.email || 'no email on file'}
          {realm && <> · {realm.display_name} <span className="chip" style={{ marginLeft: 2 }}>{realm.code}</span></>}
        </>
      }
      actions={
        <>
          <Button size="xs" leftSection={<IconCirclePlus size={14} />}
            onClick={openGive} disabled={giving}>
            Give access
          </Button>
          <Menu position="bottom-end" width={250} shadow="xl">
            <Menu.Target>
              <ActionIcon variant="default" size={30} aria-label={`More actions for ${person.username}`}>
                <IconDots size={15} />
              </ActionIcon>
            </Menu.Target>
            <Menu.Dropdown>
              <Menu.Item leftSection={<IconKey size={14} />} onClick={() => setCredsOpen(true)}>
                Credentials…
              </Menu.Item>
              <Menu.Item leftSection={<IconLock size={14} />} onClick={() => setAttrsOpen(true)}>
                Encrypted attributes
              </Menu.Item>
              <Menu.Item leftSection={<IconCopy size={14} />}
                onClick={() => { void navigator.clipboard.writeText(person.id) }}>
                Copy ID
              </Menu.Item>
              <Menu.Divider />
              <Menu.Item leftSection={<IconRefreshAlert size={14} />} onClick={() => void bumpEpoch()}>
                Invalidate issued tokens
              </Menu.Item>
              {active ? (
                <Menu.Item color="red" leftSection={<IconUserOff size={14} />}
                  onClick={() => void toggleStatus()}>
                  Disable — kills all access now
                </Menu.Item>
              ) : (
                <Menu.Item color="teal" leftSection={<IconUserCheck size={14} />}
                  onClick={() => void toggleStatus()}>
                  Re-enable
                </Menu.Item>
              )}
            </Menu.Dropdown>
          </Menu>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        {!active && (
          <div className="panel px-4 py-3"
            style={{ borderColor: 'color-mix(in srgb, var(--warn) 28%, transparent)',
                     background: 'var(--warn-bg)' }}>
            <div className="t-body">
              This identity is <strong>{person.status}</strong>.
            </div>
            <div className="t-xs mt-1" style={{ opacity: 0.85 }}>
              authorize() gates on identity state — every grant below is dead until
              it is re-enabled. Nothing here needs revoking to stop it.
            </div>
          </div>
        )}

        <div className="grid gap-3" style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))' }}>
          <Metric label="Grants" value={accessLoading ? '—' : String(grants.length)}
            sub={derived > 0 ? `${derived} via membership` : 'all granted directly'} />
          <Metric label="Time-boxed" value={accessLoading ? '—' : String(expiring)}
            sub={expiring > 0 ? 'expires without anyone acting' : 'nothing expires on its own'} />
          {/* "cannot authenticate" is the most alarming sentence on this page,
              so it must never be what an unfinished fetch looks like: an empty
              credential list and a loading one are the same `[]`. */}
          <Metric label="Sign-in methods" value={credsLoading ? '—' : String(live.length)}
            tone={!credsLoading && live.length === 0 ? 'var(--warn)' : undefined}
            sub={credsLoading ? 'checking…'
              : live.length === 0 ? 'cannot authenticate'
              : live.map((c) => c.kind).join(', ')} />
          <Metric label="Last sign-in" value={person.last_login_at?.slice(0, 10) ?? 'never'}
            tone={person.last_login_at ? undefined : 'var(--ink-3)'}
            sub={person.last_login_at ? undefined : 'an account nobody has used'} />
        </div>

        <div className="grid gap-4 items-start"
          style={{ gridTemplateColumns: 'minmax(0, 1.7fr) minmax(300px, 1fr)' }}>
          {/* ---- Access, and the way to change it ---- */}
          <div className="flex flex-col gap-3">
            <div className="flex items-baseline justify-between gap-3">
              <div className="t-h2">Access</div>
              <div className="t-xs">
                {accessLoading ? 'loading…' : `${grants.length} grant${grants.length === 1 ? '' : 's'}`}
              </div>
            </div>

            {giving && (
              <div className="panel rise p-4" style={{ borderColor: 'var(--line-strong)' }}>
                <div className="mb-1 flex items-start justify-between gap-3">
                  <div>
                    <div className="t-h2">Give {person.username} access</div>
                    <div className="t-sm mt-1" style={{ maxWidth: 460 }}>
                      A grant is a role plus one constraint per axis. Axes the grant is
                      silent on are unconstrained, and constraints are AND across axes.
                    </div>
                  </div>
                  <ActionIcon variant="subtle" color="gray" onClick={cancelGive}
                    aria-label="Cancel giving access">
                    <IconX size={15} />
                  </ActionIcon>
                </div>
                <div className="mt-3.5">
                  <GrantFields draft={draft} />
                </div>
                <div className="mt-4 flex items-center justify-end gap-2 pt-3.5"
                  style={{ borderTop: '1px solid var(--line)' }}>
                  <Button variant="default" size="sm" onClick={cancelGive}>Cancel</Button>
                  <Button size="sm" disabled={!draft.canSubmit} loading={draft.submitting}
                    onClick={() => void draft.submit()}>
                    Give access
                  </Button>
                </div>
              </div>
            )}

            {accessLoading ? (
              <div className="flex flex-col gap-3"><GrantCardSkeleton /><GrantCardSkeleton /></div>
            ) : grants.length === 0 ? (
              <div className="panel px-6 py-10 text-center">
                <div className="t-h2">No grants</div>
                <div className="t-sm mt-1.5" style={{ maxWidth: 420, margin: '6px auto 0' }}>
                  {person.username} can sign in and do nothing — authorize() denies
                  everything without a grant.
                </div>
                {!giving && (
                  <Button size="xs" variant="light" className="mt-4"
                    leftSection={<IconCirclePlus size={13} />} onClick={openGive}>
                    Give access
                  </Button>
                )}
              </div>
            ) : (
              <div className="flex flex-col gap-2.5">
                {grants.map((g) => (
                  <GrantCard key={g.id} grant={g} axes={axes} memberships={memberships}
                    nodeName={nodeName} username={person.username} />
                ))}
              </div>
            )}
          </div>

          {/* ---- The record itself ---- */}
          <div className="flex flex-col gap-4">
            <div className="panel p-4 flex flex-col gap-2">
              <div className="t-label mb-1">Identity</div>
              <Row label="Username">{person.username}</Row>
              <Row label="Email">{person.email || <span style={{ opacity: 0.5 }}>none</span>}</Row>
              <Row label="Population">
                {realm
                  ? `${realm.display_name} (${realm.code})`
                  : <span style={{ opacity: 0.5 }}>—</span>}
              </Row>
              {person.category && (
                <Row label="Category"
                  hint="Directory classification only. authorize() never reads it — access is roles and grants.">
                  {categories?.find((c) => c.code === person.category)?.display_name
                    ?? person.category}
                </Row>
              )}
              <Row label="Assurance" hint={IAL_HINT[person.assurance_level]}>
                IAL{person.assurance_level}
              </Row>
              <Row label="Created"><span className="tnum">{person.created_at.slice(0, 10)}</span></Row>
              {person.external_ref && (
                <Row label="External ref"><span className="chip">{person.external_ref}</span></Row>
              )}
              {/* Bumping this is how issued tokens are invalidated, so it is
                  worth seeing rather than inferring from an audit line. */}
              <Row label="Token epoch" hint="Access tokens minted before this number are refused.">
                <span className="tnum">{person.token_epoch}</span>
              </Row>
              <Row label="ID">
                <button
                  className="chip"
                  style={{ cursor: 'copy', maxWidth: '100%' }}
                  onClick={() => {
                    void navigator.clipboard.writeText(person.id)
                    notifications.show({ color: 'gray', title: 'Copied', message: person.id })
                  }}
                >
                  <span className="truncate">{person.id}</span>
                  <IconCopy size={10} style={{ marginLeft: 5, flexShrink: 0 }} />
                </button>
              </Row>
            </div>

            {(person.disabled_at || person.retention_until || person.anonymized_at) && (
              <div className="panel p-4 flex flex-col gap-2">
                <div className="t-label mb-1">Lifecycle</div>
                {person.disabled_at && (
                  <Row label="Disabled"><span className="tnum">{person.disabled_at.slice(0, 10)}</span></Row>
                )}
                {person.retention_until && (
                  <Row label="Retention"><span className="tnum">{person.retention_until.slice(0, 10)}</span></Row>
                )}
                {person.anonymized_at && (
                  <Row label="Anonymised"><span className="tnum">{person.anonymized_at.slice(0, 10)}</span></Row>
                )}
              </div>
            )}

            <div className="panel p-4">
              <div className="t-label mb-2">How they sign in</div>
              {credsLoading ? (
                <div className="t-xs" style={{ opacity: 0.6 }}>Loading…</div>
              ) : live.length === 0 ? (
                <div className="t-xs" style={{ color: 'var(--warn)' }}>
                  No usable credential — this identity cannot authenticate.
                </div>
              ) : (
                <div className="flex flex-wrap gap-1.5">
                  {live.map((c) => (
                    <span key={c.id} className="chip">
                      {c.kind === 'password'
                        ? <IconLock size={9} style={{ marginRight: 4 }} />
                        : <IconKey size={9} style={{ marginRight: 4 }} />}
                      {c.kind}{c.label ? ` · ${c.label}` : ''}
                    </span>
                  ))}
                </div>
              )}
              <div className="flex gap-2 mt-3">
                <Button size="xs" variant="default" onClick={() => setCredsOpen(true)}>
                  Manage credentials
                </Button>
                {/* Encrypted fields stay behind a modal on purpose: the drawer
                    kept them out of the address bar, and a page does not
                    change that. */}
                <Button size="xs" variant="default" onClick={() => setAttrsOpen(true)}>
                  Attributes
                </Button>
              </div>
            </div>
          </div>
        </div>
      </div>

      <AttributesModal id={attrsOpen ? person.id : null} label={person.username}
        onClose={() => setAttrsOpen(false)} />
      <CredentialsModal id={credsOpen ? person.id : null} label={person.username}
        onClose={() => setCredsOpen(false)} />
    </Page>
  )
}
