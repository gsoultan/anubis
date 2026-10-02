import { useState, type ReactNode } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { ActionIcon, Button, Menu, Tooltip } from '@mantine/core'
import { notifications } from '@mantine/notifications'
import {
  IconAlertTriangle, IconCirclePlus, IconCopy, IconDots, IconKey, IconLock, IconPointFilled,
  IconRefreshAlert, IconTestPipe, IconUserCheck, IconUserOff,
} from '@tabler/icons-react'
import { Page } from '@/components/shell/Page'
import { Initial } from '@/components/ui/Initial'
import { AttributesModal } from '@/components/ui/AttributesModal'
import { CredentialsModal } from '@/components/ui/CredentialsModal'
import { ConfirmModal } from '@/components/ui/ConfirmModal'
import { notifyRejected } from '@/components/create/shell'
import { AccessList } from '@/components/access/AccessList'
import { GiveAccessSheet } from '@/components/access/GiveAccessSheet'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/client'
import { realmKindColor } from '@/lib/realmKind'
import { fmtDate, relDays } from '@/lib/access'
import { searchFlag } from '@/lib/searchFlag'
import type { Ial } from '@/lib/api/types'

/* A person's own page — and, since the Access screen went, the one place a
 * person's access is read and changed.
 *
 * It used to be a 620px drawer over the People list, where giving access
 * threw a second drawer on top of the first. Then it was a page that opened
 * with four metric tiles and put the grant form inline, above the list it was
 * adding to. Now: who they are in the header, anything wrong with the account
 * straight under it, their access as the main column, and the record itself in
 * a rail. Giving access is one sheet with its own room.
 *
 * The id is in the address. That is what makes the page linkable from a
 * ticket; the encrypted attributes and the credentials stay behind modals
 * opened from component state, never a route param.
 */

export const Route = createFileRoute('/identities_/$id')({
  component: PersonPage,
  /* `?give=true` opens the sheet, so "Give access" from the People list lands
     on the form — and so does a hand-typed `?give=1` (see searchFlag). */
  validateSearch: (s: Record<string, unknown>): { give?: true } =>
    searchFlag(s['give']) ? { give: true } : {},
})

const IAL: Record<Ial, string> = {
  1: 'Self-asserted',
  2: 'Remotely verified',
  3: 'Verified in person',
}
const IAL_HINT: Record<Ial, string> = {
  1: 'Self-asserted — email only, a self-registered applicant.',
  2: 'Remotely verified, typically through a contract or employer.',
  3: 'In-person verified with government ID on file.',
}

function Fact({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
  return (
    <>
      <dt className="t-xs">
        {hint
          ? <Tooltip label={hint}><span className="dotted">{label}</span></Tooltip>
          : label}
      </dt>
      <dd className="t-body">{children}</dd>
    </>
  )
}

function Banner({ children, action }: { children: ReactNode; action?: ReactNode }) {
  return (
    <div className="banner banner-warn" role="status">
      <IconAlertTriangle size={16} className="banner-icon" />
      <div className="min-w-0 flex-1">{children}</div>
      {action}
    </div>
  )
}

function PersonPage() {
  const { id } = Route.useParams()
  const { give } = Route.useSearch()
  const navigate = useNavigate()

  /* Whether the sheet is open is the URL, not component state: it survives a
     refresh, resets when the operator moves to another person on the same
     route, and Back closes it rather than leaving the page. */
  const giving = give === true
  const openGive = () => void navigate({ to: '/identities/$id', params: { id }, search: { give: true } })
  const closeGive = () => void navigate({ to: '/identities/$id', params: { id }, search: {}, replace: true })

  const [attrsOpen, setAttrsOpen] = useState(false)
  const [credsOpen, setCredsOpen] = useState(false)
  const [confirmDisable, setConfirmDisable] = useState(false)

  const { data: person, isLoading, isError, error } = useQuery({
    queryKey: qk.identity(id), queryFn: () => api.identity(id),
  })
  /* Not paged: this is one person's access, and a person with more grants than
     a page holds is itself the finding. 200 is the server's cap. Keyed under
     'grants' so every write that invalidates grants refreshes this list. */
  const { data: access, isLoading: accessLoading } = useQuery({
    queryKey: qk.grants(id),
    queryFn: () => api.searchGrants({ identityId: id, pageSize: 200 }),
  })
  const grants = access?.rows ?? []
  const { data: axes } = useQuery({ queryKey: qk.axes(), queryFn: api.axes })
  const { data: roles } = useQuery({ queryKey: qk.roles(), queryFn: api.roles })
  const { data: memberships } = useQuery({ queryKey: qk.memberships(), queryFn: api.memberships })
  const { data: realms } = useQuery({ queryKey: qk.realms(), queryFn: api.realms })
  const { data: creds, isLoading: credsLoading } = useQuery({
    queryKey: qk.credentials(id), queryFn: () => api.credentials(id),
  })
  /* Scoped to this person's population: category codes are unique per realm. */
  const realmId = person?.realm_id ?? ''
  const { data: categories } = useQuery({
    queryKey: qk.realmCategories(realmId),
    queryFn: () => api.realmCategories(realmId),
    enabled: !!realmId,
  })
  /* The names for the places on THIS page, batch-resolved — not every node of
     every structure. */
  const scopeIds = [...new Set(grants.flatMap((g) => g.scopes.map((s) => s.scope_node_id)))].sort()
  const { data: nodes } = useQuery({
    queryKey: ['scope-names', scopeIds],
    queryFn: () => api.scopeNodesByIds(scopeIds),
    enabled: scopeIds.length > 0,
  })
  const nodeName = (nid: string) => nodes?.find((n) => n.id === nid)?.name ?? '…'

  if (isLoading) {
    return (
      <Page title="…" back={{ to: '/identities', label: 'People' }}>
        <div className="panel h-48 animate-pulse" />
      </Page>
    )
  }
  if (isError || !person) {
    return (
      <Page title="Person not found" back={{ to: '/identities', label: 'People' }}
        description="This person does not exist in the tenant you are administering — they may have been deleted, or belong to another one.">
        <div className="panel px-6 py-10 text-center">
          <div className="t-sm" style={{ color: 'var(--ink-3)' }}>
            {error instanceof Error ? error.message : `No identity with id ${id}.`}
          </div>
        </div>
      </Page>
    )
  }

  const realm = realms?.find((r) => r.id === person.realm_id)
  const category = person.category
    ? categories?.find((c) => c.code === person.category)?.display_name ?? person.category
    : null
  const usable = creds?.filter((c) => !c.revoked_at) ?? []
  const active = person.status === 'active'

  async function setStatus(next: 'active' | 'disabled') {
    try {
      await api.setIdentityStatus(id, next)
    } catch (e) {
      notifyRejected(e)
      throw e
    }
    notifications.show({
      color: next === 'disabled' ? 'orange' : 'teal',
      title: next === 'disabled' ? 'Disabled' : 'Re-enabled',
      message: next === 'disabled'
        ? `${person?.username} cannot sign in, and none of their access applies until re-enabled.`
        : 'Their access applies again immediately.',
    })
    // 'identities' prefixes both the list and this page's own key.
    await queryClient.invalidateQueries({ queryKey: ['identities'] })
  }

  async function bumpEpoch() {
    let n: number
    try {
      n = await api.bumpTokenEpoch(id)
    } catch (e) {
      notifyRejected(e)
      return
    }
    notifications.show({
      color: 'orange', title: 'Tokens invalidated',
      message: `Epoch is now ${n}. Every access token minted before this is refused.`,
    })
    await queryClient.invalidateQueries({ queryKey: ['identities'] })
  }

  const copyId = () => {
    void navigator.clipboard.writeText(person.id)
    notifications.show({ color: 'gray', title: 'Copied', message: person.id })
  }

  return (
    <Page
      wide
      back={{ to: '/identities', label: 'People' }}
      lead={<Initial name={person.username} colour={realmKindColor(realm?.kind)} size={44} />}
      title={person.username}
      badge={
        <>
          <span className={`v-pill ${active ? 'v-pill-allow' : 'v-pill-deny'}`}>
            <IconPointFilled size={8} />{person.status}
          </span>
          <Tooltip label={IAL_HINT[person.assurance_level]}>
            <span className="chip" style={{ cursor: 'help' }}>IAL{person.assurance_level}</span>
          </Tooltip>
        </>
      }
      description={[person.email || 'no email on file', realm?.display_name, category]
        .filter(Boolean).join(' · ')}
      actions={
        <>
          <Button variant="default" size="xs" leftSection={<IconTestPipe size={14} />}
            onClick={() => void navigate({ to: '/playground', search: { subject: id } })}>
            Test access
          </Button>
          <Button size="xs" leftSection={<IconCirclePlus size={14} />} onClick={openGive}>
            Give access
          </Button>
          <Menu position="bottom-end" width={250}>
            <Menu.Target>
              <ActionIcon variant="default" size={30} aria-label={`More actions for ${person.username}`}>
                <IconDots size={15} />
              </ActionIcon>
            </Menu.Target>
            <Menu.Dropdown>
              <Menu.Item leftSection={<IconKey size={14} />} onClick={() => setCredsOpen(true)}>
                Sign-in methods…
              </Menu.Item>
              <Menu.Item leftSection={<IconLock size={14} />} onClick={() => setAttrsOpen(true)}>
                Encrypted attributes…
              </Menu.Item>
              <Menu.Item leftSection={<IconCopy size={14} />} onClick={copyId}>Copy ID</Menu.Item>
              <Menu.Divider />
              <Menu.Item leftSection={<IconRefreshAlert size={14} />} onClick={() => void bumpEpoch()}>
                Invalidate issued tokens
              </Menu.Item>
              {active ? (
                <Menu.Item color="deny" leftSection={<IconUserOff size={14} />}
                  onClick={() => setConfirmDisable(true)}>
                  Disable…
                </Menu.Item>
              ) : (
                <Menu.Item leftSection={<IconUserCheck size={14} />}
                  onClick={() => void setStatus('active').catch(() => {})}>
                  Re-enable
                </Menu.Item>
              )}
            </Menu.Dropdown>
          </Menu>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        {/* What is wrong with the account, before what it can do: none of the
            access below matters to somebody who is disabled or cannot sign in. */}
        {!active && (
          <Banner action={
            <Button size="xs" variant="default" leftSection={<IconUserCheck size={14} />}
              onClick={() => void setStatus('active').catch(() => {})}>
              Re-enable
            </Button>
          }>
            <div className="t-body" style={{ fontWeight: 560 }}>This person is {person.status}.</div>
            <div className="t-xs mt-0.5">
              None of the access below applies until they are re-enabled — nothing needs revoking to stop it.
            </div>
          </Banner>
        )}
        {active && !credsLoading && usable.length === 0 && (
          <Banner action={
            <Button size="xs" variant="default" leftSection={<IconKey size={14} />}
              onClick={() => setCredsOpen(true)}>
              Set up sign-in
            </Button>
          }>
            <div className="t-body" style={{ fontWeight: 560 }}>{person.username} cannot sign in yet.</div>
            <div className="t-xs mt-0.5">
              There is no password or authenticator on this account, so their access cannot be used.
            </div>
          </Banner>
        )}

        <div className="grid grid-cols-1 items-start gap-4 lg:grid-cols-[minmax(0,1fr)_300px] xl:grid-cols-[minmax(0,1fr)_340px]">
          <AccessList identityId={id} username={person.username} grants={grants}
            loading={accessLoading} axes={axes} roles={roles} memberships={memberships}
            nodeName={nodeName} onGive={openGive} />

          <aside className="flex flex-col gap-4">
            <section className="panel" aria-labelledby="profile-title">
              <div className="panel-head">
                <h2 id="profile-title" className="t-h2">Profile</h2>
              </div>
              <dl className="facts panel-body">
                <Fact label="Username">{person.username}</Fact>
                <Fact label="Email">{person.email || <span className="t-xs">none</span>}</Fact>
                <Fact label="Population">
                  {realm ? (
                    <span className="inline-flex min-w-0 items-center gap-1.5">
                      <span style={{ width: 6, height: 6, borderRadius: 99, flexShrink: 0,
                        background: realmKindColor(realm.kind) }} />
                      <span className="truncate">{realm.display_name}</span>
                    </span>
                  ) : <span className="t-xs">—</span>}
                </Fact>
                {category && (
                  <Fact label="Category"
                    hint="Directory classification only. authorize() never reads it — access is roles and grants.">
                    {category}
                  </Fact>
                )}
                <Fact label="Assurance" hint={IAL_HINT[person.assurance_level]}>
                  IAL{person.assurance_level} <span className="t-xs">· {IAL[person.assurance_level]}</span>
                </Fact>
                <Fact label="Last sign-in">
                  {person.last_login_at
                    ? <span className="tnum">{fmtDate(person.last_login_at)} <span className="t-xs">· {relDays(person.last_login_at)}</span></span>
                    : <span className="t-xs">Never</span>}
                </Fact>
                <Fact label="Created"><span className="tnum">{fmtDate(person.created_at)}</span></Fact>
                {person.external_ref && (
                  <Fact label="External ref"><span className="chip">{person.external_ref}</span></Fact>
                )}
                <Fact label="ID">
                  <button className="chip max-w-full" style={{ cursor: 'copy' }} onClick={copyId}
                    aria-label="Copy ID">
                    <span className="truncate">{person.id}</span>
                    <IconCopy size={10} style={{ marginLeft: 5, flexShrink: 0 }} />
                  </button>
                </Fact>
              </dl>
            </section>

            <section className="panel" aria-labelledby="signin-title">
              <div className="panel-head">
                <h2 id="signin-title" className="t-h2">Sign-in</h2>
                <Button size="compact-xs" variant="subtle" onClick={() => setCredsOpen(true)}>Manage</Button>
              </div>
              <div className="panel-body flex flex-col gap-3">
                {credsLoading ? (
                  <div className="t-xs">Checking…</div>
                ) : usable.length === 0 ? (
                  <div className="t-xs">No sign-in methods.</div>
                ) : (
                  <ul className="flex flex-col gap-2">
                    {usable.map((c) => (
                      <li key={c.id} className="flex items-center gap-2.5">
                        <span className="cred-icon">
                          {c.kind === 'password' ? <IconLock size={13} /> : <IconKey size={13} />}
                        </span>
                        <span className="min-w-0 flex-1">
                          <span className="t-body block truncate">{c.label || c.kind}</span>
                          <span className="t-xs block truncate">
                            {c.kind}{c.last_used_at ? ` · used ${relDays(c.last_used_at)}` : ' · never used'}
                          </span>
                        </span>
                      </li>
                    ))}
                  </ul>
                )}
                <dl className="facts" style={{ paddingTop: 10, borderTop: '1px solid var(--line-soft)' }}>
                  <Fact label="Token epoch" hint="Access tokens minted before this number are refused. Invalidate issued tokens to raise it.">
                    <span className="tnum">{person.token_epoch}</span>
                  </Fact>
                  <Fact label="Attributes">
                    <button className="link-btn" onClick={() => setAttrsOpen(true)}>View encrypted…</button>
                  </Fact>
                </dl>
              </div>
            </section>

            {(person.disabled_at || person.retention_until || person.anonymized_at) && (
              <section className="panel" aria-labelledby="lifecycle-title">
                <div className="panel-head">
                  <h2 id="lifecycle-title" className="t-h2">Lifecycle</h2>
                </div>
                <dl className="facts panel-body">
                  {person.disabled_at && <Fact label="Disabled"><span className="tnum">{fmtDate(person.disabled_at)}</span></Fact>}
                  {person.retention_until && (
                    <Fact label="Keep until" hint="The population's statutory retention limit for this record.">
                      <span className="tnum">{fmtDate(person.retention_until)} <span className="t-xs">· {relDays(person.retention_until)}</span></span>
                    </Fact>
                  )}
                  {person.anonymized_at && <Fact label="Anonymised"><span className="tnum">{fmtDate(person.anonymized_at)}</span></Fact>}
                </dl>
              </section>
            )}
          </aside>
        </div>
      </div>

      <GiveAccessSheet opened={giving} onClose={closeGive} identityId={id} />
      <AttributesModal id={attrsOpen ? person.id : null} label={person.username}
        onClose={() => setAttrsOpen(false)} />
      <CredentialsModal id={credsOpen ? person.id : null} label={person.username}
        onClose={() => setCredsOpen(false)} />
      <ConfirmModal opened={confirmDisable} onClose={() => setConfirmDisable(false)}
        title={`Disable ${person.username}?`} confirmLabel="Disable"
        onConfirm={() => setStatus('disabled')}>
        <p>
          Every session they have ends now and the tokens already issued to them stop working. They
          cannot sign in, and none of their access applies while disabled.
        </p>
        <p className="t-xs">Their grants are kept: re-enabling gives their access back without re-granting anything.</p>
      </ConfirmModal>
    </Page>
  )
}
