import { createFileRoute, Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Button, TextInput } from '@mantine/core'
import { useState } from 'react'
import { Page } from '@/components/shell/Page'
import { DataTable, type Column } from '@/components/ui/DataTable'
import { notifyCreated, notifyRejected } from '@/components/create/shell'
import { SecretOnce } from '@/components/apps/SecretOnce'
import { ConnectApp } from '@/components/apps/ConnectApp'
import { hasSecret, kindLabel } from '@/components/apps/AppConnection'
import * as live from '@/lib/api/live'

export const Route = createFileRoute('/applications')({ component: Applications })


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
      {/* Title, a 220px label field and the button need ~390px on one line;
          a phone's panel has ~317, so below sm the field takes its own row. */}
      <div className="mb-1 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between sm:gap-3">
        <span className="t-h1">API keys</span>
        <div className="flex items-center gap-2">
          <TextInput size="xs" className="min-w-0 flex-1 sm:w-[220px] sm:flex-none"
            placeholder="Label, e.g. gateway-prod"
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
            <span className="flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-0.5">
              <span className="t-body" style={{ fontWeight: 530 }}>{k.label || '(unlabelled)'}</span>
              <span className="chip">{k.prefix}</span>
              {k.created_by && <span className="t-xs">by {k.created_by}</span>}
              <span className="t-xs">
                {k.last_used_at ? `last used ${k.last_used_at.slice(0, 10)}` : 'never used'}
              </span>
            </span>
            <Button variant="subtle" size="compact-xs" color="red" className="shrink-0"
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

  const columns: Column<live.AppRecord>[] = [
    {
      key: 'app', header: 'Application', width: 260,
      render: (a) => (
        <Link to="/applications/$slug" params={{ slug: a.slug }} className="flex flex-col gap-1 no-underline" style={{ color: 'inherit' }}>
          <span className="t-body" style={{ fontWeight: 550, color: 'var(--accent)' }}>{a.name}</span>
          <span className="chip w-fit">{a.slug}</span>
        </Link>
      ),
    },
    {
      key: 'kind', header: 'Kind', width: 150,
      render: (a) => (
        <div className="flex flex-col gap-1">
          <span className="t-body">{kindLabel(a.kind)}</span>
          {/* Whether a client can keep a secret decides half of how it is
              configured, so it belongs on the row rather than in a dialog. */}
          <span className="t-xs">{hasSecret(a.kind) ? 'has a secret' : 'no secret'}</span>
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
      key: 'signin', header: 'Who may sign in', width: 170,
      render: (a) => <span className="t-body">{a.sign_in === 'role_holders' ? 'People with a role in it' : 'Anyone'}</span>,
    },
    {
      key: 'manifest', header: 'Features', width: 120, align: 'right',
      render: (a) => (
        <Link to="/applications/$slug" params={{ slug: a.slug }} search={{ tab: 'features' }} className="t-xs" style={{ color: 'var(--accent)' }}>
          See features{a.manifest_version > 0 && <span className="chip ml-1">v{a.manifest_version}</span>}
        </Link>
      ),
    },
    {
      key: 'actions', header: '', width: 90, align: 'right',
      render: (a) => (
        <Link to="/applications/$slug" params={{ slug: a.slug }} className="t-sm" style={{ color: 'var(--accent)' }}>Open</Link>
      ),
    },
  ]

  return (
    <Page
      section="apps"
      title="Apps"
      description="The apps people sign in to. Open one to see who can use it, its roles and features, and the rules for its pages."
      actions={<ConnectApp />}
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
