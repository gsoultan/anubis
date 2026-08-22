import { createFileRoute } from '@tanstack/react-router'
import { IconAlertTriangle, IconCheck, IconPointFilled } from '@tabler/icons-react'
import { Page } from '@/components/shell/Page'
import { DataTable, Cell, type Column } from '@/components/ui/DataTable'

export const Route = createFileRoute('/keys')({ component: Keys })

type Key = { kid: string; alg: string; status: string; age: number; purpose: string }
const KEYS: Key[] = [
  { kid: 'k4-2026-05', alg: 'Ed25519',     status: 'active',   age: 94,  purpose: 'access' },
  { kid: 'k4-2026-02', alg: 'Ed25519',     status: 'retiring', age: 183, purpose: 'access' },
  { kid: 'lk-2026-05', alg: 'AES-256-GCM', status: 'active',   age: 94,  purpose: 'local' },
]

const STEPS = [
  { n: 'Generate', d: 'status = pending', done: true },
  { n: 'Publish', d: 'Appears in key discovery. Wait ≥ 2× cache TTL.', done: true },
  { n: 'Activate', d: 'Previous key becomes retiring', done: false },
  { n: 'Retire', d: 'Once the longest-lived token has expired', done: false },
]

function Keys() {
  const columns: Column<Key>[] = [
    { key: 'kid', header: 'Key ID', width: 170, render: (k) => <span className="chip chip-gold">{k.kid}</span> },
    { key: 'alg', header: 'Algorithm', width: 150, render: (k) => <span className="t-body">{k.alg}</span> },
    { key: 'purpose', header: 'Purpose', width: 110, render: (k) => <span className="t-xs">{k.purpose}</span> },
    { key: 'status', header: 'Status', width: 130, render: (k) => (
        <span className={`v-pill ${k.status === 'active' ? 'v-pill-allow' : 'v-pill-idle'}`}>
          <IconPointFilled size={8} />{k.status}
        </span>
      ) },
    { key: 'age', header: 'Age', width: 130, align: 'right', render: (k) => (
        <span className="inline-flex items-center justify-end gap-2">
          <span className="tnum t-body">{k.age}d</span>
          {k.age > 90 && <span className="chip" style={{ color: 'var(--warn)', borderColor: 'color-mix(in srgb, var(--warn) 20%, transparent)' }}>overdue</span>}
        </span>
      ) },
  ]

  return (
    <Page
      title="Signing keys"
      description="The keys that sign every token. If one leaks, an attacker can impersonate anyone — rotation discipline is what prevents that."
      wide
    >
      <div className="flex flex-col gap-5">
        <div className="panel p-4" style={{ borderColor: 'color-mix(in srgb, var(--warn) 24%, transparent)',
          background: 'linear-gradient(100deg, var(--warn-bg), var(--s-raised) 55%)' }}>
          <div className="flex items-start gap-3">
            <div className="flex shrink-0 items-center justify-center rounded-lg"
              style={{ width: 32, height: 32, background: 'color-mix(in srgb, var(--warn) 9%, transparent)', border: '1px solid color-mix(in srgb, var(--warn) 20%, transparent)' }}>
              <IconAlertTriangle size={16} style={{ color: 'var(--warn)' }} />
            </div>
            <div>
              <div className="t-h2">Rotation overdue</div>
              <div className="t-sm mt-1">
                <span className="chip chip-gold">k4-2026-05</span> is 94 days old against a 90-day policy.
              </div>
            </div>
          </div>
        </div>

        <div className="panel p-4">
          <div className="t-label mb-4">Rotation sequence</div>
          <div className="grid gap-0" style={{ gridTemplateColumns: 'repeat(4, minmax(0,1fr))' }}>
            {STEPS.map((s, i) => (
              <div key={s.n} className="relative pr-4">
                <div className="mb-2.5 flex items-center gap-2">
                  <div className="flex shrink-0 items-center justify-center rounded-full"
                    style={{
                      width: 20, height: 20,
                      background: s.done ? 'color-mix(in srgb, var(--allow) 9%, transparent)' : 'var(--s-sunken)',
                      border: `1px solid ${s.done ? 'color-mix(in srgb, var(--allow) 25%, transparent)' : 'var(--line)'}`,
                    }}>
                    {s.done
                      ? <IconCheck size={11} style={{ color: 'var(--allow)' }} />
                      : <span className="tnum" style={{ fontSize: 10, color: 'var(--ink-3)' }}>{i + 1}</span>}
                  </div>
                  {i < STEPS.length - 1 && (
                    <div style={{ flex: 1, height: 1, background: s.done ? 'color-mix(in srgb, var(--allow) 25%, transparent)' : 'var(--line)' }} />
                  )}
                </div>
                <div className="t-body" style={{ fontWeight: 550, color: s.done ? 'var(--ink)' : 'var(--ink-2)' }}>
                  {s.n}
                </div>
                <div className="t-xs mt-0.5" style={{ paddingRight: 8 }}>{s.d}</div>
              </div>
            ))}
          </div>
          <div className="t-xs mt-4" style={{ borderTop: '1px solid var(--line-soft)', paddingTop: 12 }}>
            Publishing before activating is the whole point: a consumer that has not yet seen the new
            <span className="chip" style={{ margin: '0 4px' }}>kid</span> rejects every token signed with
            it. At most one active key per purpose is enforced by a partial unique index.
          </div>
        </div>

        <DataTable columns={columns} rows={KEYS} rowKey={(k) => k.kid} />
      </div>
    </Page>
  )
}
