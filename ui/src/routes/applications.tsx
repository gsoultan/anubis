import { createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Button, Code, Modal, Select, TextInput, Textarea, Tooltip } from '@mantine/core'
import { IconFileCode, IconPlus, IconRefresh } from '@tabler/icons-react'
import { useState } from 'react'
import { Page } from '@/components/shell/Page'
import { DataTable, type Column } from '@/components/ui/DataTable'
import { notifyCreated, notifyRejected } from '@/components/create/shell'
import { queryClient } from '@/lib/query/client'
import * as live from '@/lib/api/live'

export const Route = createFileRoute('/applications')({ component: Applications })

const KINDS = [
  { value: 'spa', label: 'SPA — browser app, no secret' },
  { value: 'native', label: 'Native — mobile or desktop, no secret' },
  { value: 'server', label: 'Server — confidential, gets a secret' },
  { value: 'service', label: 'Service — machine to machine, gets a secret' },
]

/** A secret is shown exactly once. If it is lost, the only way back is to
    rotate — which is why this is a panel and not a toast. */
function SecretOnce({ secret, onDone }: { secret: string; onDone: () => void }) {
  return (
    <Modal opened onClose={onDone} title="Client secret" centered>
      <p className="t-sm mb-3">
        Copy this now. It is not stored anywhere readable and will never be
        shown again — losing it means rotating, which breaks whatever is
        already using the old one.
      </p>
      <Code block>{secret}</Code>
      <Button className="mt-3" onClick={onDone}>I have copied it</Button>
    </Modal>
  )
}

function ManifestDialog({ slug, onDone }: { slug: string; onDone: () => void }) {
  const [open, setOpen] = useState(false)
  const [json, setJson] = useState('')
  const [report, setReport] = useState('')
  const [busy, setBusy] = useState(false)

  const run = async (dry: boolean) => {
    setBusy(true)
    try {
      const resp = await live.applyManifest(slug, json, dry)
      setReport(resp.reportJson)
      if (!dry) {
        notifyCreated('Manifest applied', `${slug} is now at version ${resp.manifestVersion}.`)
        await queryClient.invalidateQueries({ queryKey: ['applications'] })
        onDone()
      }
    } catch (e) { notifyRejected(e) }
    setBusy(false)
  }

  return (
    <>
      <Button variant="default" size="compact-xs" leftSection={<IconFileCode size={13} />}
        onClick={() => setOpen(true)}>
        Manifest
      </Button>
      <Modal opened={open} onClose={() => setOpen(false)} title={`Manifest — ${slug}`} centered size="lg">
        <p className="t-sm mb-3">
          What this application declares: the permissions it defines, the roles
          that bundle them, and the routes a gateway should protect. Applying
          replaces the previous declaration, so check it first — the report
          says what would change.
        </p>
        <Textarea autosize minRows={8} maxRows={18} styles={{ input: { fontFamily: 'monospace' } }}
          placeholder='{"permissions":[{"resource":"invoice","action":"read","risk":"normal"}]}'
          value={json} onChange={(e) => setJson(e.currentTarget.value)} />
        <div className="mt-3 flex items-center gap-2">
          <Button variant="default" loading={busy} disabled={!json.trim()}
            onClick={() => void run(true)}>Check</Button>
          <Button loading={busy} disabled={!report}
            onClick={() => void run(false)}>Apply</Button>
          {!report && <span className="t-xs">Check before applying.</span>}
        </div>
        {report && (
          <div className="mt-3">
            <div className="t-label mb-1">What this would do</div>
            <Code block>{report}</Code>
          </div>
        )}
      </Modal>
    </>
  )
}

function AddDialog({ onSecret }: { onSecret: (s: string) => void }) {
  const [open, setOpen] = useState(false)
  const [slug, setSlug] = useState('')
  const [name, setName] = useState('')
  const [kind, setKind] = useState<string | null>('spa')
  const [redirects, setRedirects] = useState('')
  const [postLogout, setPostLogout] = useState('')
  const [busy, setBusy] = useState(false)

  const lines = (v: string) => v.split('\n').map((x) => x.trim()).filter(Boolean)

  const submit = async () => {
    if (!slug.trim() || !name.trim() || !kind) return
    setBusy(true)
    try {
      const { clientSecret } = await live.createApplication({
        slug: slug.trim(), name: name.trim(), kind,
        redirectUris: lines(redirects),
        postLogoutRedirectUris: lines(postLogout),
      })
      notifyCreated('Application created', `${name.trim()} can now own permissions.`)
      await queryClient.invalidateQueries({ queryKey: ['applications'] })
      setOpen(false); setSlug(''); setName(''); setRedirects(''); setPostLogout('')
      if (clientSecret) onSecret(clientSecret)
    } catch (e) { notifyRejected(e) }
    setBusy(false)
  }

  return (
    <>
      <Button leftSection={<IconPlus size={15} />} onClick={() => setOpen(true)}>
        Add application
      </Button>
      <Modal opened={open} onClose={() => setOpen(false)} title="Add an application" centered>
        <div className="flex flex-col gap-3">
          <TextInput label="Slug" required value={slug} autoFocus
            description="Namespaces every permission this app defines, and cannot be changed."
            onChange={(e) => setSlug(e.currentTarget.value)} />
          <TextInput label="Name" required value={name}
            onChange={(e) => setName(e.currentTarget.value)} />
          <Select label="Kind" data={KINDS} value={kind} onChange={setKind} />
          <Textarea label="Redirect URIs" autosize minRows={2}
            description="One per line. Matched exactly — a wildcard here is an open redirect."
            value={redirects} onChange={(e) => setRedirects(e.currentTarget.value)} />
          <Textarea label="Post-logout redirect URIs" autosize minRows={2}
            description="A separate allowlist: where a login lands is not where a logout should."
            value={postLogout} onChange={(e) => setPostLogout(e.currentTarget.value)} />
          <Button loading={busy} disabled={!slug.trim() || !name.trim()} onClick={() => void submit()}>
            Create
          </Button>
        </div>
      </Modal>
    </>
  )
}


/* The tenant's machine credentials, beside its relying parties: both are the
   integration surface. A key authenticates as the tenant's system — created
   by operators, tied to nobody, revocable without touching any person. */
function ApiKeysPanel({ onSecret }: { onSecret: (s: string) => void }) {
  const [label, setLabel] = useState('')
  const [busy, setBusy] = useState(false)
  const { data: keys, error, refetch } = useQuery({
    queryKey: ['api-keys'],
    queryFn: live.apiKeys,
    retry: false,
  })

  const create = async () => {
    if (!label.trim()) return
    setBusy(true)
    try {
      const { apiKey } = await live.createApiKey(label.trim())
      notifyCreated('API key created', 'Copy it now — it is shown exactly once.')
      onSecret(apiKey)
      setLabel('')
      await refetch()
    } catch (e) { notifyRejected(e) }
    setBusy(false)
  }

  const revoke = async (id: string, prefix: string) => {
    try {
      await live.revokeApiKey(id)
      notifyCreated('API key revoked', `${prefix} stopped working immediately.`)
      await refetch()
    } catch (e) { notifyRejected(e) }
  }

  const live_ = (keys ?? []).filter((k) => !k.revoked_at)
  return (
    <div className="panel mt-6 p-4">
      <div className="mb-1 flex items-center justify-between gap-3">
        <span className="t-h1">API keys</span>
        <div className="flex items-center gap-2">
          <TextInput size="xs" w={220} placeholder="Label, e.g. gateway-prod"
            value={label} onChange={(e) => setLabel(e.currentTarget.value)} />
          <Button size="compact-sm" disabled={!label.trim()} loading={busy}
            onClick={() => void create()}>Create key</Button>
        </div>
      </div>
      <p className="t-sm mb-3">
        Machine access for this tenant — a gateway asking authorize(), an
        integration reading the decision API. Keys belong to the tenant, not to
        any person: nobody leaving takes an integration down with them.
      </p>
      {error && <p className="t-sm">{(error as Error).message}</p>}
      {!error && live_.length === 0 && <p className="t-xs">No keys yet.</p>}
      <div className="flex flex-col gap-1">
        {live_.map((k) => (
          <div key={k.id} className="panel-inset flex items-center justify-between gap-3 px-2.5 py-1.5">
            <span className="flex items-baseline gap-2">
              <span className="t-body" style={{ fontWeight: 530 }}>{k.label || '(unlabelled)'}</span>
              <span className="chip">{k.prefix}</span>
              {k.created_by && <span className="t-xs">by {k.created_by}</span>}
              <span className="t-xs">
                {k.last_used_at ? `last used ${k.last_used_at.slice(0, 10)}` : 'never used'}
              </span>
            </span>
            <Button variant="subtle" size="compact-xs" color="red"
              onClick={() => void revoke(k.id, k.prefix)}>Revoke</Button>
          </div>
        ))}
      </div>
    </div>
  )
}

function Applications() {
  const [secret, setSecret] = useState('')
  const [q, setQ] = useState('')
  /* Keyset paging: a stack of cursors, because it can step forward from where
     it is and back to somewhere it has been, but cannot jump to page 7. */
  const [trail, setTrail] = useState<string[]>([''])
  const cursor = trail[trail.length - 1] ?? ''

  const { data: page, error, refetch } = useQuery({
    queryKey: ['applications', q, cursor],
    queryFn: () => live.applications({ query: q.trim(), cursor, pageSize: 25 }),
    placeholderData: (prev) => prev,
    retry: false,
  })

  const rotate = async (id: string, slug: string) => {
    try {
      const s = await live.rotateClientSecret(id)
      notifyCreated('Secret rotated', `${slug}'s previous secret stopped working immediately.`)
      setSecret(s)
      await refetch()
    } catch (e) { notifyRejected(e) }
  }

  const columns: Column<live.AppRecord>[] = [
    {
      key: 'app', header: 'Application', width: 260,
      render: (a) => (
        <div className="flex flex-col gap-1">
          <span className="t-body" style={{ fontWeight: 550 }}>{a.name}</span>
          <span className="chip">{a.slug}</span>
        </div>
      ),
    },
    {
      key: 'kind', header: 'Kind', width: 150,
      render: (a) => (
        <div className="flex flex-col gap-1">
          <span className="t-body">{a.kind}</span>
          {/* Whether a client can keep a secret decides half of how it is
              configured, so it belongs on the row rather than in a dialog. */}
          <span className="t-xs">
            {a.kind === 'server' || a.kind === 'service' ? 'confidential' : 'public — no secret'}
          </span>
        </div>
      ),
    },
    {
      key: 'redirects', header: 'Redirect URIs',
      render: (a) => a.redirect_uris.length === 0
        ? <span className="t-xs">none</span>
        : (
          <div className="flex flex-col gap-0.5">
            {a.redirect_uris.slice(0, 2).map((u) => (
              <span key={u} className="t-xs" style={{ wordBreak: 'break-all' }}>{u}</span>
            ))}
            {a.redirect_uris.length > 2 && (
              <span className="t-xs">+{a.redirect_uris.length - 2} more</span>
            )}
          </div>
        ),
    },
    {
      key: 'manifest', header: 'Manifest', width: 110, align: 'right',
      render: (a) => a.manifest_version > 0
        ? <span className="chip">v{a.manifest_version}</span>
        : <span className="t-xs">none yet</span>,
    },
    {
      key: 'actions', header: '', width: 210, align: 'right',
      render: (a) => (
        <div className="flex items-center justify-end gap-1">
          <ManifestDialog slug={a.slug} onDone={() => void refetch()} />
          {(a.kind === 'server' || a.kind === 'service') && (
            <Tooltip label="The old secret stops working immediately">
              <Button variant="default" size="compact-xs" leftSection={<IconRefresh size={13} />}
                onClick={() => void rotate(a.id, a.slug)}>
                Rotate
              </Button>
            </Tooltip>
          )}
        </div>
      ),
    },
  ]

  return (
    <Page
      title="Applications"
      description="The relying parties on this tenant — the things its people sign in to. Every permission is namespaced by the application that defines it, so a permission cannot exist until its application does. Anubis's own console and service are not listed: they are infrastructure, not somewhere anybody signs in."
      actions={<AddDialog onSecret={setSecret} />}
      wide
    >
      {secret && <SecretOnce secret={secret} onDone={() => setSecret('')} />}

      {error && (
        <div className="panel p-4">
          <div className="t-h1 mb-1">Cannot list applications</div>
          {/* A failed query must never read as "there are none" — that is how
              an empty dropdown looks like a missing feature. */}
          <p className="t-sm">{(error as Error).message}</p>
        </div>
      )}

      {!error && (
        <>
          <div className="mb-2 flex items-center justify-between gap-3">
            <span className="t-label">
              {page ? `${page.rows.length} of ${page.total}` : '…'}
            </span>
            <TextInput size="xs" w={240} placeholder="Search slug or name…"
              value={q} onChange={(e) => { setQ(e.currentTarget.value); setTrail(['']) }} />
          </div>

          <DataTable columns={columns} rows={page?.rows} rowKey={(a) => a.id}
            empty={{
              title: q.trim() ? 'No application matches' : 'No applications yet',
              hint: q.trim()
                ? 'Try another search.'
                : 'Add one before defining permissions — every permission key starts with the slug of the application that owns it.',
            }} />

          <ApiKeysPanel onSecret={setSecret} />

          {(trail.length > 1 || page?.next) && (
            <div className="mt-3 flex items-center gap-2">
              <Button variant="default" size="compact-sm" disabled={trail.length <= 1}
                onClick={() => setTrail((t) => t.slice(0, -1))}>Previous</Button>
              <Button variant="default" size="compact-sm" disabled={!page?.next}
                onClick={() => setTrail((t) => [...t, page?.next ?? ''])}>Next</Button>
            </div>
          )}
        </>
      )}
    </Page>
  )
}
