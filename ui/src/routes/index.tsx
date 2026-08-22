import { createFileRoute, Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import {
  IconAlertTriangle, IconKey, IconTrash, IconActivity, IconUsers, IconSitemap,
  IconAffiliate, IconArrowRight, IconShieldExclamation, IconChevronRight,
} from '@tabler/icons-react'
import { IconX, IconUserPlus, IconCirclePlus, IconTestPipe } from '@tabler/icons-react'
import { Button, ActionIcon } from '@mantine/core'
import { Page } from '@/components/shell/Page'
import { useCreate } from '@/stores/create'
import { useSession } from '@/stores/session'
import { Stat } from '@/components/ui/Stat'
import { Sparkline } from '@/components/ui/Sparkline'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import type { SecuritySignal } from '@/lib/api/types'

export const Route = createFileRoute('/')({ component: Overview })

const SIGNAL: Record<SecuritySignal['kind'],
  { title: string; icon: typeof IconKey; why: string; action?: { label: string; to: string } }> = {
  refresh_token_reuse: {
    title: 'Refresh token reuse detected', icon: IconShieldExclamation,
    why: 'A consumed refresh token was replayed — a token was stolen. The family and session are already revoked; the remaining work is establishing blast radius.',
    action: { label: 'Inspect audit trail', to: '/audit' },
  },
  login_failure_spike: {
    title: 'Login failure spike', icon: IconActivity,
    why: 'Failures concentrated on one account indicate credential stuffing rather than a forgetful user.',
  },
  snapshot_stale: {
    title: 'Snapshot stale', icon: IconActivity,
    why: 'The gate is past its maximum snapshot age and failing closed. Protected paths are denying.',
  },
  key_rotation_due: {
    title: 'Key rotation overdue', icon: IconKey,
    why: 'Publish the new public key and wait for consumer caches before activating, or every token signed with it is rejected.',
    action: { label: 'Signing keys', to: '/keys' },
  },
  audit_chain_broken: {
    title: 'Audit hash chain broken', icon: IconAlertTriangle,
    why: 'Either tampering, or a bug destroying the log’s evidentiary value.',
  },
  retention_overdue: {
    title: 'Retention overdue', icon: IconTrash,
    why: 'Records past retention_until are not yet anonymised. This is statutory exposure, not housekeeping.',
    action: { label: 'Review identities', to: '/identities' },
  },
}

/* Deterministic pseudo-series so the sparklines are stable across renders
   instead of shimmering on every paint. */
const series = (seed: number, n = 24, base = 50, amp = 18) =>
  Array.from({ length: n }, (_, i) =>
    base + Math.sin(i * 0.7 + seed) * amp + Math.sin(i * 0.23 + seed * 2) * (amp * 0.5))

/* The guided path. Someone opening an IAM console for the first time has one
   question — "what do I do?" — and the answer is always the same three steps.
   Dismissable and persisted; experts see it once. */
function StartHere() {
  const { openCreate } = useCreate()
  const { gettingStartedDismissed, dismissGettingStarted } = useSession()
  if (gettingStartedDismissed) return null
  const steps = [
    { n: 1, title: 'Add a person', hint: 'An employee, a supplier contact, or an applicant.',
      action: <Button size="xs" variant="light" leftSection={<IconUserPlus size={13} />}
        onClick={() => openCreate('identity')}>Add person</Button> },
    { n: 2, title: 'Give them access', hint: 'A grant ties a person to a role, limited to a scope.',
      action: <Button size="xs" variant="light" leftSection={<IconCirclePlus size={13} />}
        onClick={() => openCreate('grant')}>Give access</Button> },
    { n: 3, title: 'Check it works', hint: 'Ask “can they do X?” and see exactly why yes or no.',
      action: <Link to="/playground" className="no-underline">
        <Button size="xs" variant="light" leftSection={<IconTestPipe size={13} />}>Test access</Button>
      </Link> },
  ]
  return (
    <div className="panel relative p-4">
      <ActionIcon variant="subtle" color="gray" size="sm" aria-label="Dismiss"
        className="!absolute right-2.5 top-2.5" onClick={dismissGettingStarted}>
        <IconX size={14} />
      </ActionIcon>
      <div className="t-h2 mb-3">New here? Three steps.</div>
      <div className="grid gap-3" style={{ gridTemplateColumns: 'repeat(3, minmax(0,1fr))' }}>
        {steps.map((st) => (
          <div key={st.n} className="panel-inset flex flex-col gap-2 px-3.5 py-3">
            <div className="flex items-center gap-2.5">
              <span className="tnum flex items-center justify-center rounded-full"
                style={{ width: 22, height: 22, fontSize: 11, fontWeight: 650,
                  background: 'var(--gold-glow)', color: 'var(--gold)' }}>{st.n}</span>
              <span className="t-body" style={{ fontWeight: 600 }}>{st.title}</span>
            </div>
            <span className="t-xs" style={{ minHeight: 30 }}>{st.hint}</span>
            <div>{st.action}</div>
          </div>
        ))}
      </div>
    </div>
  )
}

function Overview() {
  const { data, isLoading } = useQuery({ queryKey: qk.dashboard(), queryFn: api.dashboard })

  if (isLoading || !data) {
    return (
      <Page title="Overview">
        <div className="grid grid-cols-4 gap-4">
          {[0, 1, 2, 3].map((i) => (
            <div key={i} className="panel h-[108px] animate-pulse" style={{ opacity: 0.4 }} />
          ))}
        </div>
      </Page>
    )
  }

  const paging = data.signals.filter((s) => s.severity === 'page')
  const alerting = data.signals.filter((s) => s.severity !== 'page')
  const total = data.identities_by_realm.reduce((a, r) => a + r.count, 0)

  return (
    <Page
      title="Overview"
      description="Alerts first, numbers second — anything needing a human leads the page."
    >
      <div className="flex flex-col gap-5">
        <StartHere />
        {/* Paging signals lead. Burying the one event that means "a token was
            stolen" under vanity metrics inverts the console's priorities. */}
        {paging.map((s) => {
          const m = SIGNAL[s.kind]
          return (
            <div
              key={s.kind}
              className="panel rise relative overflow-hidden p-4"
              style={{ borderColor: 'color-mix(in srgb, var(--deny) 25%, transparent)',
                background: 'linear-gradient(100deg, var(--deny-bg), var(--s-raised) 55%)' }}
            >
              <div className="flex items-start gap-3.5">
                <div className="flex shrink-0 items-center justify-center rounded-lg"
                  style={{ width: 36, height: 36, background: 'color-mix(in srgb, var(--deny) 9%, transparent)', border: '1px solid color-mix(in srgb, var(--deny) 20%, transparent)' }}>
                  <m.icon size={18} style={{ color: 'var(--deny)' }} />
                </div>
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="t-h1">{m.title}</span>
                    <span className="v-pill v-pill-deny">pages on-call</span>
                  </div>
                  <div className="t-body mt-1.5" style={{ color: 'var(--ink-2)' }}>{s.detail}</div>
                  <div className="t-xs mt-1.5" style={{ maxWidth: 620 }}>{m.why}</div>
                  {m.action && (
                    <Link to={m.action.to}
                      className="mt-3 inline-flex items-center gap-1.5 no-underline"
                      style={{ color: 'var(--deny)', fontSize: 12, fontWeight: 600 }}>
                      {m.action.label}<IconArrowRight size={13} />
                    </Link>
                  )}
                </div>
              </div>
            </div>
          )
        })}

        <div className="grid gap-4" style={{ gridTemplateColumns: 'repeat(4, minmax(0,1fr))' }}>
          <Stat label="Identities" value={total.toLocaleString()} to="/identities"
            icon={<IconUsers size={11} />} series={series(1, 24, 55, 6)} trend={2.4}
            sub={`${data.identities_by_realm.length} populations`} />
          <Stat label="Grants" value={data.grants_total.toLocaleString()} to="/grants"
            icon={<IconAffiliate size={11} />} series={series(3, 24, 50, 8)} trend={0.8} />
          <Stat label="Scope nodes" value={data.scope_nodes_total.toLocaleString()} to="/scope"
            icon={<IconSitemap size={11} />} series={series(5, 24, 45, 12)} trend={6.1} />
          <Stat label="Decisions · 24h" value={data.decisions_24h.toLocaleString()}
            icon={<IconActivity size={11} />} series={series(7, 24, 60, 22)}
            accent="var(--info)"
            sub={`p99 ${data.p99_authorize_ms} ms · ${(data.deny_rate_24h * 100).toFixed(1)}% deny`} />
        </div>

        <div className="grid gap-4" style={{ gridTemplateColumns: 'minmax(0,7fr) minmax(0,5fr)' }}>
          <div className="panel p-4">
            <div className="mb-3.5 flex items-baseline justify-between">
              <div className="t-label">Identities by population</div>
              <div className="t-xs">one tenant, isolated by realm</div>
            </div>
            <div className="flex flex-col gap-3.5">
              {data.identities_by_realm.map((r, i) => {
                const pct = (r.count / total) * 100
                const colour = r.kind === 'internal' ? 'var(--gold)'
                  : r.kind === 'partner' ? 'var(--info)' : 'var(--grape)'
                return (
                  <div key={r.realm}>
                    <div className="mb-1.5 flex items-baseline justify-between gap-3">
                      <div className="flex items-center gap-2">
                        <span style={{ width: 7, height: 7, borderRadius: 99, background: colour }} />
                        <span className="t-body" style={{ fontWeight: 520 }}>{r.realm}</span>
                        <span className="chip">{r.kind}</span>
                      </div>
                      <div className="flex items-baseline gap-2">
                        <span className="tnum t-body" style={{ fontWeight: 600 }}>
                          {r.count.toLocaleString()}
                        </span>
                        <span className="t-xs tnum" style={{ width: 38, textAlign: 'right' }}>
                          {pct.toFixed(1)}%
                        </span>
                      </div>
                    </div>
                    <div style={{ height: 5, borderRadius: 99, background: 'var(--s-sunken)', overflow: 'hidden' }}>
                      <div style={{
                        width: `${pct}%`, height: '100%', borderRadius: 99, background: colour,
                        opacity: 0.85, transition: 'width 600ms var(--ease)',
                        transitionDelay: `${i * 80}ms`,
                      }} />
                    </div>
                  </div>
                )
              })}
            </div>
            <div className="t-xs mt-4" style={{ borderTop: '1px solid var(--line-soft)', paddingTop: 12 }}>
              Partners are not separate tenants — that would require cross-tenant grants, which
              every composite foreign key in the schema exists to forbid.
            </div>
          </div>

          <div className="flex flex-col gap-4">
            <div className="panel p-4">
              <div className="t-label mb-3">Open alerts</div>
              <div className="flex flex-col gap-2.5">
                {alerting.length === 0 && <div className="t-sm">Nothing outstanding.</div>}
                {alerting.map((s) => {
                  const m = SIGNAL[s.kind]
                  return (
                    <div key={s.kind} className="flex items-start gap-2.5">
                      <div className="flex shrink-0 items-center justify-center rounded"
                        style={{ width: 24, height: 24, background: 'var(--warn-bg)', border: '1px solid color-mix(in srgb, var(--warn) 18%, transparent)' }}>
                        <m.icon size={12} style={{ color: 'var(--warn)' }} />
                      </div>
                      <div className="min-w-0">
                        <div className="t-body" style={{ fontWeight: 530 }}>{m.title}</div>
                        <div className="t-xs mt-0.5">{s.detail}</div>
                      </div>
                    </div>
                  )
                })}
              </div>
            </div>

            <Link to="/playground" className="panel panel-hover group block p-4 no-underline">
              <div className="flex items-center justify-between gap-3">
                <div className="min-w-0">
                  <div className="t-h2">Debug a decision</div>
                  <div className="t-xs mt-1">
                    Trace every candidate grant and see which gate stopped it.
                  </div>
                </div>
                <div className="flex shrink-0 items-center justify-center rounded-lg"
                  style={{ width: 34, height: 34, background: 'var(--gold-glow)' }}>
                  <IconChevronRight size={16} style={{ color: 'var(--gold)' }} />
                </div>
              </div>
              <div className="mt-3 flex items-end justify-between gap-3"
                style={{ borderTop: '1px solid var(--line-soft)', paddingTop: 12 }}>
                <div>
                  <div className="t-label">deny rate · 24h</div>
                  <div className="tnum mt-1" style={{ fontSize: 18, fontWeight: 620 }}>
                    {(data.deny_rate_24h * 100).toFixed(1)}%
                  </div>
                </div>
                <Sparkline data={series(11, 24, 40, 14)} color="var(--deny)" w={110} h={30} />
              </div>
            </Link>
          </div>
        </div>
      </div>
    </Page>
  )
}
