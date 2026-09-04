import { createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Button } from '@mantine/core'
import { IconAlertTriangle, IconPlus, IconPointFilled } from '@tabler/icons-react'
import { Page } from '@/components/shell/Page'
import { DataTable, Cell, type Column } from '@/components/ui/DataTable'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/client'
import { notifyCreated, notifyRejected } from '@/components/create/shell'
import type { SigningKeyRecord } from '@/lib/api/types'

export const Route = createFileRoute('/keys')({ component: Keys })

/* This page used to render three invented keys, a hardcoded "rotation
   overdue" banner naming a key that did not exist, and a four-step checklist
   with its progress written into the source. Signing keys decide whether
   every token in the system verifies, so a page that describes them
   inaccurately is worse than one that does not exist: an operator reads
   "active, 94 days" and stops looking.
   Everything below comes from ListSigningKeys. */

/** A key can be `active` and still unusable: verifiers reject a token signed
    outside the key's published window, so issuance must refuse it too. That
    is the state a hardcoded table could never show, and the one worth
    shouting about. */
function windowProblem(k: SigningKeyRecord, now: Date): string | null {
  if (k.status !== 'active') return null
  if (k.not_before && new Date(k.not_before) > now) return 'not valid yet'
  if (k.not_after && new Date(k.not_after) <= now) return 'past its window'
  return null
}

function Keys() {
  const { data: keys, isLoading } = useQuery({
    queryKey: qk.signingKeys(),
    queryFn: () => api.signingKeys(),
  })
  const now = new Date()
  const rows = keys ?? []

  /* Warnings are derived, never asserted. Each one is a real question an
     operator would otherwise have to answer by reading the database. */
  const unusable = rows.filter((k) => windowProblem(k, now))
  const purposes: SigningKeyRecord['purpose'][] = ['access', 'local']
  const missing = purposes.filter(
    (p) => rows.some((k) => k.purpose === p) && !rows.some((k) => k.purpose === p && k.status === 'active'),
  )
  const pending = rows.filter((k) => k.status === 'pending')

  async function prepare(purpose: SigningKeyRecord['purpose']) {
    try {
      const kid = await api.prepareSigningKey(purpose)
      await queryClient.invalidateQueries({ queryKey: qk.signingKeys() })
      notifyCreated(`Prepared ${kid}`,
        'It is pending. Publish it, wait out the discovery cache, then promote it.')
    } catch (e) { notifyRejected(e) }
  }

  const columns: Column<SigningKeyRecord>[] = [
    { key: 'kid', header: 'Key ID', width: 190, render: (k) => <span className="chip chip-gold">{k.kid}</span> },
    { key: 'alg', header: 'Algorithm', width: 140, render: (k) => <span className="t-body">{k.alg}</span> },
    { key: 'purpose', header: 'Purpose', width: 100, render: (k) => <span className="t-xs">{k.purpose}</span> },
    {
      key: 'status', header: 'Status', width: 150, render: (k) => {
        const bad = windowProblem(k, now)
        return (
          <span className={`v-pill ${bad ? 'v-pill-deny' : k.status === 'active' ? 'v-pill-allow' : 'v-pill-idle'}`}>
            <IconPointFilled size={8} />{bad ?? k.status}
          </span>
        )
      },
    },
    {
      key: 'window', header: 'Valid', width: 190, render: (k) => (
        <Cell
          top={<span className="tnum">{k.not_before ? k.not_before.slice(0, 10) : 'always'}</span>}
          bottom={k.not_after ? `until ${k.not_after.slice(0, 10)}` : 'no end date'}
        />
      ),
    },
  ]

  return (
    <Page
      title="Signing keys"
      description="The keys that sign every token. If one leaks, an attacker can impersonate anyone — rotation discipline is what prevents that."
      wide
      actions={
        <>
          <Button variant="default" size="xs" leftSection={<IconPlus size={14} />}
            onClick={() => prepare('access')}>
            Prepare access key
          </Button>
          <Button variant="default" size="xs" leftSection={<IconPlus size={14} />}
            onClick={() => prepare('local')}>
            Prepare local key
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-5">
        {unusable.length > 0 && (
          <Banner tone="deny" title="A key in use is outside its published window">
            {unusable.map((k) => (
              <div key={k.kid}>
                <span className="chip chip-gold">{k.kid}</span> is {windowProblem(k, now)}.
                Verifiers reject tokens signed with it, so every token it mints is refused
                — promote a prepared key.
              </div>
            ))}
          </Banner>
        )}

        {missing.length > 0 && (
          <Banner tone="deny" title="No active key">
            {missing.map((p) => (
              <div key={p}>
                Nothing can be signed for <strong>{p}</strong>. Prepare a key, publish it,
                then <code>anubisd keys promote {p}</code>.
              </div>
            ))}
          </Banner>
        )}

        {pending.length > 0 && unusable.length === 0 && missing.length === 0 && (
          <Banner tone="warn" title="A prepared key is waiting">
            {pending.map((k) => (
              <div key={k.kid}>
                <span className="chip chip-gold">{k.kid}</span> is published but not active.
                Once consumers have had at least 2× the discovery cache TTL to see it,
                run <code>anubisd keys promote {k.purpose}</code>.
              </div>
            ))}
          </Banner>
        )}

        <div className="panel p-4">
          <div className="t-label mb-2">How rotation works</div>
          <div className="t-body" style={{ opacity: 0.85 }}>
            <strong>Prepare</strong> mints a pending key and publishes it to key discovery.
            <strong> Promote</strong> makes it the one that signs, and the previous key becomes
            retiring. <strong>Retire</strong> drops it once the longest-lived token it signed has
            expired.
          </div>
          <div className="t-xs mt-3" style={{ borderTop: '1px solid var(--line-soft)', paddingTop: 12 }}>
            Publishing before activating is the whole point: a consumer that has not yet seen the
            new <span className="chip" style={{ margin: '0 4px' }}>kid</span> rejects every token
            signed with it. At most one active key per purpose is enforced by a partial unique index.
            Promotion is deliberately not a button — it is <code>anubisd keys promote</code>, run
            when you know consumers have caught up.
          </div>
        </div>

        {!isLoading && rows.length === 0 ? (
          <div className="panel p-6 text-center">
            <div className="t-h2">No signing keys</div>
            <div className="t-sm mt-1">
              Nothing can be issued until one exists. Prepare one above, or run
              {' '}<code>anubisd keys init access</code>.
            </div>
          </div>
        ) : (
          <DataTable columns={columns} rows={rows} rowKey={(k) => k.kid} />
        )}
      </div>
    </Page>
  )
}

function Banner({ tone, title, children }: {
  tone: 'warn' | 'deny'; title: string; children: React.ReactNode
}) {
  const c = tone === 'deny' ? 'var(--deny)' : 'var(--warn)'
  return (
    <div
      className="panel p-4"
      style={{ borderColor: `color-mix(in srgb, ${c} 24%, transparent)` }}
    >
      <div className="flex items-start gap-3">
        <div
          className="flex shrink-0 items-center justify-center rounded-lg"
          style={{
            width: 32, height: 32,
            background: `color-mix(in srgb, ${c} 9%, transparent)`,
            border: `1px solid color-mix(in srgb, ${c} 20%, transparent)`,
          }}
        >
          <IconAlertTriangle size={16} style={{ color: c }} />
        </div>
        <div>
          <div className="t-h2">{title}</div>
          <div className="t-sm mt-1 flex flex-col gap-1">{children}</div>
        </div>
      </div>
    </div>
  )
}
