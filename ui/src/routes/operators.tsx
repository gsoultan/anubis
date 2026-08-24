import { createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Button, Modal, PasswordInput, Select, TextInput } from '@mantine/core'
import { IconCrown, IconPlus, IconShieldCog, IconShieldLock, IconTrash, IconUserPlus } from '@tabler/icons-react'
import { useState } from 'react'
import { Page } from '@/components/shell/Page'
import { notifyCreated, notifyRejected } from '@/components/create/shell'
import { api } from '@/lib/anubis'

export const Route = createFileRoute('/operators')({ component: Operators })

const ROLES = [
  { value: 'support', label: 'Support — read config, administer people' },
  { value: 'admin', label: 'Admin — everything an in-tenant administrator may do' },
  { value: 'owner', label: 'Owner — admin, plus assigning other operators' },
]

type Assignment = { id: string; tenantSlug: string; role: string; reason: string; createdAt: bigint }
type Operator = {
  identityId: string; username: string; email: string; status: string
  owner: boolean; mfaEnrolled: boolean; assignments: Assignment[]
}

function useOperators(query: string, cursor: string) {
  return useQuery({
    queryKey: ['operators', query, cursor],
    queryFn: async () => {
      const resp = await api.platformAdmin.listOperators({
        query, pageToken: cursor, pageSize: 50,
      })
      return {
        next: resp.nextPageToken,
        total: resp.total,
        operators: resp.operators.map((o): Operator => ({
          identityId: o.identityId, username: o.username, email: o.email,
          status: o.status, owner: o.owner, mfaEnrolled: o.mfaEnrolled,
          assignments: o.assignments.map((a) => ({
            id: a.id, tenantSlug: a.tenantSlug, role: a.role,
            reason: a.reason, createdAt: a.createdAt,
          })),
        })),
      }
    },
  })
}

/* An assignment with no tenant is authority over all of them. Showing that as
   an empty cell would read as "none", which is the opposite of what it is. */
function scopeLabel(a: Assignment) {
  return a.tenantSlug === '' ? 'every tenant' : a.tenantSlug
}

/* Creating a platform administrator is NOT the same action as adding a person
   to a tenant. These accounts live in the platform tenant, a separate
   population: a tenant's own users never appear here, and nothing on the
   People screen can turn one into an operator. */
function CreateDialog({ onDone }: { onDone: () => void }) {
  const [open, setOpen] = useState(false)
  const [username, setUsername] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [tenant, setTenant] = useState('')
  const [role, setRole] = useState<string | null>('admin')
  const [reason, setReason] = useState('')
  const [busy, setBusy] = useState(false)

  const reset = () => {
    setUsername(''); setEmail(''); setPassword(''); setTenant(''); setReason('')
  }

  const submit = async () => {
    if (!username.trim() || !password || !role) return
    setBusy(true)
    try {
      await api.platformAdmin.createOperator({
        username: username.trim(), email: email.trim(), password,
        tenantSlug: tenant.trim(), role, reason: reason.trim(),
      })
      notifyCreated('Platform user created',
        `${username.trim()} can administer ${tenant.trim() || 'every tenant'}.`)
      setOpen(false); reset(); onDone()
    } catch (e) { notifyRejected(e) }
    setBusy(false)
  }

  return (
    <>
      <Button leftSection={<IconUserPlus size={15} />} onClick={() => setOpen(true)}>
        Add platform user
      </Button>
      <Modal opened={open} onClose={() => setOpen(false)} title="Add a platform user" centered>
        <p className="t-sm mb-3">
          This creates an account in the platform tenant. It is not a user of any
          tenant, and a tenant’s own people cannot be made into one here.
        </p>
        <div className="flex flex-col gap-3">
          <TextInput label="Username" required value={username} autoFocus
            onChange={(e) => setUsername(e.currentTarget.value)} />
          <TextInput label="Email" value={email} onChange={(e) => setEmail(e.currentTarget.value)} />
          <PasswordInput label="Password" required value={password}
            description="Held to the platform realm’s own password policy."
            onChange={(e) => setPassword(e.currentTarget.value)} />
          <TextInput label="Tenant" placeholder="Leave blank for every tenant"
            description="Which tenant they may administer. Blank makes them an installation owner."
            value={tenant} onChange={(e) => setTenant(e.currentTarget.value)} />
          <Select label="Role" data={ROLES} value={role} onChange={setRole} />
          <TextInput label="Reason" placeholder="Recorded in the audit log"
            value={reason} onChange={(e) => setReason(e.currentTarget.value)} />
          <Button loading={busy} disabled={!username.trim() || !password || !role} onClick={submit}>
            Create
          </Button>
        </div>
      </Modal>
    </>
  )
}

function AssignDialog({ onDone }: { onDone: () => void }) {
  const [open, setOpen] = useState(false)
  const [username, setUsername] = useState('')
  const [tenant, setTenant] = useState('')
  const [role, setRole] = useState<string | null>('admin')
  const [reason, setReason] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async () => {
    if (!username.trim() || !role) return
    setBusy(true)
    try {
      await api.platformAdmin.assignOperator({
        operatorUsername: username.trim(), tenantSlug: tenant.trim(), role, reason: reason.trim(),
      })
      notifyCreated('Operator assigned',
        `${username.trim()} can now administer ${tenant.trim() || 'every tenant'}.`)
      setOpen(false); setUsername(''); setTenant(''); setReason(''); onDone()
    } catch (e) { notifyRejected(e) }
    setBusy(false)
  }

  return (
    <>
      <Button variant="default" leftSection={<IconPlus size={15} />} onClick={() => setOpen(true)}>
        Assign existing
      </Button>
      <Modal opened={open} onClose={() => setOpen(false)} title="Assign an operator" centered>
        <div className="flex flex-col gap-3">
          {/* Somebody with no assignment yet is not on the list above, so this
              takes a username rather than only offering existing operators. */}
          <TextInput label="Operator" placeholder="username in the platform tenant"
            description="They must already have an account in the platform tenant."
            value={username} onChange={(e) => setUsername(e.currentTarget.value)} />
          <TextInput label="Tenant" placeholder="Leave blank for every tenant"
            description="The tenant they may administer. Blank makes them an installation owner."
            value={tenant} onChange={(e) => setTenant(e.currentTarget.value)} />
          <Select label="Role" data={ROLES} value={role} onChange={setRole} />
          <TextInput label="Reason" placeholder="Recorded in the audit log"
            value={reason} onChange={(e) => setReason(e.currentTarget.value)} />
          <Button loading={busy} disabled={!username.trim() || !role} onClick={submit}>Assign</Button>
        </div>
      </Modal>
    </>
  )
}

function Operators() {
  const [query, setQuery] = useState('')
  /* A stack, not a page number: keyset paging can go forward from a cursor
     and back to one it has already seen, but it cannot jump to page 7. */
  const [trail, setTrail] = useState<string[]>([''])
  const cursor = trail[trail.length - 1] ?? ''
  const { data, isPending, error, refetch } = useOperators(query, cursor)

  const revoke = async (id: string, who: string) => {
    try {
      await api.platformAdmin.revokeAssignment({ assignmentId: id })
      notifyCreated('Assignment revoked', `${who} no longer has that authority.`)
      await refetch()
    } catch (e) { notifyRejected(e) }
  }

  return (
    <Page
      title="Platform users"
      description="The people who operate this installation, and which tenants each of them may administer. These accounts live in the platform tenant — a separate population from any tenant's own people. The owner created during setup is here too."
      actions={
        <>
          <AssignDialog onDone={() => refetch()} />
          <CreateDialog onDone={() => refetch()} />
        </>
      }
    >
      {error && (
        <div className="panel p-4">
          <div className="t-h1 mb-1">Cannot list operators</div>
          <p className="t-sm">{(error as Error).message}</p>
        </div>
      )}

      {!error && !isPending && data && data.operators.length === 0 && (
        <div className="panel p-4">
          <div className="t-h1 mb-1">Nobody holds platform authority yet</div>
          <p className="t-sm">
            Add a platform user to give somebody authority over this installation.
          </p>
        </div>
      )}

      {data && data.operators.length > 0 && (
        <>
          <div className="mb-2 flex items-center justify-between gap-3">
            <span className="t-label">
              {data.operators.length} of {data.total} platform user{data.total === 1 ? '' : 's'}
            </span>
            <TextInput size="xs" placeholder="Search username…" value={query}
              onChange={(e) => { setQuery(e.currentTarget.value); setTrail(['']) }} />
          </div>
          <div className="flex flex-col gap-2">
            {data.operators.map((o) => (
              <div key={o.identityId} className="panel p-4">
                <div className="mb-2 flex items-baseline justify-between gap-3">
                  <span className="flex items-center gap-2">
                    <span className="t-h1">{o.username}</span>
                    {o.owner && (
                      <span className="chip chip-gold" title="Authority over every tenant">
                        <IconCrown size={11} style={{ marginRight: 4 }} />owner
                      </span>
                    )}
                    {o.status !== 'active' && <span className="chip">{o.status}</span>}
                    {/* Who has a second factor is the owner's business:
                        these accounts run the installation. */}
                    {o.mfaEnrolled
                      ? <span className="chip" title="Second factor enrolled">
                          <IconShieldLock size={11} style={{ marginRight: 4 }} />2FA
                        </span>
                      : <span className="chip" style={{ color: 'var(--warn)' }}
                          title="Password only — this account runs the installation">
                          no 2FA
                        </span>}
                  </span>
                  <span className="t-xs">{o.email}</span>
                </div>

                {o.assignments.length === 0 ? (
                  <div className="t-sm">
                    No assignments — this account can sign in but administers nothing.
                  </div>
                ) : (
                  <div className="flex flex-col gap-1">
                    {o.assignments.map((a) => (
                      <div key={a.id} className="panel-inset flex items-center justify-between gap-3 px-2.5 py-1.5">
                        <span className="flex items-center gap-2">
                          <IconShieldCog size={14} style={{ color: 'var(--ink-3)' }} />
                          <span className="t-body" style={{ fontWeight: 530 }}>{a.role}</span>
                          <span className="t-xs">on {scopeLabel(a)}</span>
                          {a.reason && <span className="t-xs">· {a.reason}</span>}
                        </span>
                        <Button variant="subtle" size="compact-xs" color="red"
                          leftSection={<IconTrash size={13} />}
                          onClick={() => revoke(a.id, o.username)}>
                          Revoke
                        </Button>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            ))}
          </div>

          {(trail.length > 1 || data.next) && (
            <div className="mt-3 flex items-center gap-2">
              <Button variant="default" size="compact-sm" disabled={trail.length <= 1}
                onClick={() => setTrail((t) => t.slice(0, -1))}>
                Previous
              </Button>
              <Button variant="default" size="compact-sm" disabled={!data.next}
                onClick={() => setTrail((t) => [...t, data.next])}>
                Next
              </Button>
            </div>
          )}
        </>
      )}
    </Page>
  )
}
