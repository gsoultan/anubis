import { createFileRoute, useNavigate, useSearch } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Button, Select, Tooltip, Loader } from '@mantine/core'
import {
  IconCheck, IconX, IconPlayerPlayFilled, IconRefresh, IconShieldLock,
  IconPointFilled, IconBolt, IconChevronRight,
} from '@tabler/icons-react'
import { useEffect, useState } from 'react'
import { Page } from '@/components/shell/Page'
import { AxisTargetPicker } from '@/components/scope/AxisTargetPicker'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { usePlayground } from '@/stores/session'
import type { AuthorizeResponse, AxisVerdict, GrantEvaluation } from '@/lib/api/types'

/* Decision state lives in the URL as well as the store.
   An operator debugging a denial almost always ends up sending it to someone
   else — "why is this denied for Alice?" — and a screenshot loses the inputs.
   Reserved keys keep their underscore, and axis codes cannot start with one,
   so subject/permission can never collide with an axis. */
type Search = Record<string, string | undefined>

export const Route = createFileRoute('/playground')({
  component: Playground,
  validateSearch: (raw: Record<string, unknown>): Search => {
    const out: Search = {}
    for (const [k, v] of Object.entries(raw)) {
      if (typeof v === 'string' && v.length > 0 && v.length < 128) out[k] = v
    }
    return out
  },
})

const REASON: Record<string, string> = {
  no_grant: 'No live grant confers this permission on this identity.',
  scope_mismatch: 'A grant exists, but the target sits outside its scope on at least one axis.',
  axis_unresolved: 'A grant constrains an axis for which no target was supplied. Unresolved axes deny.',
  strict_axis_unaddressed: 'An axis is strict and this grant does not address it.',
  assurance_too_low: 'The identity’s assurance level is below what this permission requires.',
  identity_inactive: 'The identity is disabled or anonymised. Denied regardless of grants.',
  self_scope_mismatch: 'This is a self-scoped grant and the record owner is missing or different.',
  step_up_required: 'The grant is sufficient; the session is not. Re-authentication required.',
}

/* One segment of the trace. Reading left to right, the first red gate is where
   the evaluation stopped — which is the question an operator actually has. */
function Gate({ v }: { v: AxisVerdict }) {
  const skipped = !v.constrained && v.satisfied
  const cls = skipped ? 'gate gate-skip' : v.satisfied ? 'gate gate-pass' : 'gate gate-fail'
  return (
    <Tooltip
      label={
        <div className="flex flex-col gap-1">
          <div style={{ fontWeight: 600 }}>{v.axis_code}</div>
          {v.constrained ? (
            (v.granted_nodes?.length ?? 0) > 1 ? (
              <div>
                <div>granted <b>any of</b>:</div>
                {v.granted_nodes!.map((n) => (
                  <div key={n.id} style={{ paddingLeft: 8 }}>
                    {n.matched ? '✓' : '·'} {n.name}{!n.inherit ? ' (exact)' : ''}
                  </div>
                ))}
                <div>target <b>{v.target_node_name ?? '(not supplied)'}</b></div>
              </div>
            ) : (
              <div>
                granted <b>{v.granted_node_name ?? '—'}</b>
                {' → '}target <b>{v.target_node_name ?? '(not supplied)'}</b>
              </div>
            )
          ) : (
            <div>Grant is silent on this axis.</div>
          )}
          {v.note && <div style={{ color: 'var(--ink-2)' }}>{v.note}</div>}
        </div>
      }
    >
      <div className={cls}>
        <div className="flex items-center gap-1.5">
          {skipped
            ? <IconPointFilled size={9} style={{ color: 'var(--ink-4)' }} />
            : v.satisfied
              ? <IconCheck size={11} style={{ color: 'var(--allow)' }} />
              : <IconX size={11} style={{ color: 'var(--deny)' }} />}
          <span
            className="truncate font-mono"
            style={{ fontSize: 10.5, color: skipped ? 'var(--ink-4)' : 'var(--ink)' }}
          >
            {v.axis_code}
          </span>
        </div>
        <div className="truncate" style={{ fontSize: 10, color: 'var(--ink-3)' }}>
          {!v.constrained ? 'unconstrained'
            : (v.granted_nodes?.length ?? 0) > 1
              ? `any of ${v.granted_nodes!.length} · ${v.target_node_name ?? 'not supplied'}`
              : (v.target_node_name ?? 'not supplied')}
        </div>
      </div>
    </Tooltip>
  )
}

function Trace({ e }: { e: GrantEvaluation }) {
  const failing = e.axes.find((a) => !a.satisfied && a.constrained)
  return (
    <div className="panel rise overflow-hidden">
      <div
        className="flex items-center justify-between gap-3 px-4 py-2.5"
        style={{ borderBottom: '1px solid var(--line-soft)' }}
      >
        <div className="flex min-w-0 items-center gap-2">
          <span className="t-h2 truncate">{e.role_name}</span>
          {e.self_scoped && (
            <Tooltip label="Applies only to records the subject owns. Cannot carry axis constraints.">
              <span className="chip" style={{ color: 'var(--info)', borderColor: 'color-mix(in srgb, var(--info) 20%, transparent)' }}>
                self-scoped
              </span>
            </Tooltip>
          )}
        </div>
        <span className={`v-pill ${e.survived ? 'v-pill-allow' : 'v-pill-deny'}`}>
          {e.survived ? <IconCheck size={11} /> : <IconX size={11} />}
          {e.survived ? 'survives' : 'fails'}
        </span>
      </div>

      <div className="flex flex-wrap items-stretch gap-2.5 px-4 py-3">
        {e.axes.map((v) => <Gate key={v.axis_code} v={v} />)}
      </div>

      {failing?.note && (
        <div
          className="px-4 py-2.5"
          style={{ borderTop: '1px solid var(--line-soft)', background: 'var(--deny-bg)' }}
        >
          <div className="flex items-start gap-2">
            <IconX size={13} style={{ color: 'var(--deny)', marginTop: 1, flexShrink: 0 }} />
            <div>
              <div className="t-sm" style={{ color: 'var(--ink)' }}>
                Stopped at <span className="font-mono" style={{ color: 'var(--deny)' }}>{failing.axis_code}</span>
              </div>
              <div className="t-xs mt-0.5">{failing.note}</div>
              {failing.path.length > 1 && (
                <div className="t-xs mt-1 font-mono" style={{ fontSize: 10 }}>
                  target path: {failing.path.map((p) => p.name).join(' › ')}
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

/* Scenarios that teach the model. An empty state that only says "nothing here"
   wastes the one moment the operator is looking for guidance. */
type PresetCtx = {
  setSubject: (v: string | null) => void
  setPermission: (v: string | null) => void
  setTarget: (axis: string, id: string | null) => void
  identities: { id: string; username: string; assurance_level: number }[] | undefined
  permissions: { key: string; min_assurance: number }[] | undefined
}
const PRESETS: { label: string; hint: string; apply: (c: PresetCtx) => void }[] = [
  {
    label: 'Inherited scope',
    hint: 'A grant at a department covering a team beneath it',
    apply: ({ setSubject, setPermission, identities }) => {
      const a = identities?.find((i) => i.username === 'alice' && i.assurance_level === 3)
      setSubject(a?.id ?? null); setPermission('billing:invoice:read')
    },
  },
  {
    label: 'Unresolved axis',
    hint: 'Leave an axis unset to see the fail-closed rule deny',
    apply: ({ setSubject, setPermission, setTarget, identities }) => {
      const a = identities?.find((i) => i.username === 'alice' && i.assurance_level === 3)
      setSubject(a?.id ?? null); setPermission('billing:invoice:approve')
      setTarget('org', null); setTarget('product', null)
    },
  },
  {
    label: 'Assurance floor',
    hint: 'An IAL1 applicant against an IAL3 permission',
    apply: ({ setSubject, setPermission, identities }) => {
      const p = identities?.find((i) => i.assurance_level === 1)
      setSubject(p?.id ?? null); setPermission('billing:payment:approve')
    },
  },
]

function Playground() {
  const { subject, permission, targets, setSubject, setPermission, setTarget, reset } = usePlayground()
  const search = useSearch({ from: '/playground' })
  const navigate = useNavigate({ from: '/playground' })
  const [hydrated, setHydrated] = useState(false)

  // Hydrate once from the URL, then let the store own it. A link arriving with
  // a full scenario evaluates immediately: a shared decision link that shows an
  // empty form until you click has lost the thing being shared.
  useEffect(() => {
    if (hydrated) return
    setHydrated(true)
    const { subject: s, permission: p, ...axisTargets } = search
    if (s) setSubject(s)
    if (p) setPermission(p)
    for (const [k, v] of Object.entries(axisTargets)) if (v) setTarget(k, v)
    if (s && p) {
      setTouched(true)
      void api.authorize({
        subject: s, permission: p,
        scopes: Object.fromEntries(Object.entries(axisTargets).filter(([, v]) => !!v)) as Record<string, string>,
      }).then(setResult)
    }
  }, [hydrated, search, setSubject, setPermission, setTarget])

  // Mirror state back so the address bar is always a shareable link.
  useEffect(() => {
    if (!hydrated) return
    const next: Search = { ...targets }
    if (subject) next['subject'] = subject
    if (permission) next['permission'] = permission
    void navigate({ search: next, replace: true })
  }, [hydrated, subject, permission, targets, navigate])
  const [result, setResult] = useState<AuthorizeResponse | null>(null)
  const [running, setRunning] = useState(false)
  /* Unset-axis warnings only after a first attempt. Four yellow blocks on an
     untouched form is noise, and noise trains operators to ignore the colour
     that later has to mean something. */
  const [touched, setTouched] = useState(false)

  const { data: identities } = useQuery({ queryKey: qk.identities(), queryFn: () => api.identities() })
  const { data: realms } = useQuery({ queryKey: qk.realms(), queryFn: api.realms })
  const { data: permissions } = useQuery({ queryKey: qk.permissions(), queryFn: api.permissions })

  const subj = identities?.find((i) => i.id === subject)
  const realm = realms?.find((r) => r.id === subj?.realm_id)
  const perm = permissions?.find((p) => p.key === permission)
  const ready = !!subject && !!permission

  async function run() {
    if (!ready) return
    setRunning(true); setTouched(true)
    setResult(await api.authorize({ subject: subject!, permission: permission!, scopes: { ...targets } }))
    setRunning(false)
  }

  // ⌘↵ evaluates. An operator iterating on a denial changes one field and
  // re-runs dozens of times; reaching for the mouse each cycle is friction.
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') { e.preventDefault(); void run() }
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  })

  return (
    <Page
      title="Access check"
      description="Ask “can this person do this, here?” and see exactly why the answer is yes or no. An axis left unset counts against access, never for it."
      wide
      actions={
        <>
          <Button size="xs" variant="default" leftSection={<IconRefresh size={13} />}
            onClick={() => { reset(); setResult(null) }}>Reset</Button>
          <Button size="xs" disabled={!ready} loading={running} onClick={run}
            leftSection={<IconPlayerPlayFilled size={11} />}
            rightSection={<kbd className="chip" style={{ fontSize: 9, borderColor: 'transparent', background: '#0003' }}>⌘↵</kbd>}>
            Evaluate
          </Button>
        </>
      }
    >
      <div className="grid gap-5" style={{ gridTemplateColumns: 'minmax(320px, 400px) minmax(0, 1fr)' }}>
        {/* ---------------- composer ---------------- */}
        <div className="flex flex-col gap-4">
          <div className="panel p-4">
            <div className="t-label mb-2.5">Subject</div>
            <Select
              searchable clearable placeholder="Select an identity"
              data={(identities ?? []).map((i) => ({
                value: i.id,
                label: `${i.username} · ${realms?.find((r) => r.id === i.realm_id)?.code ?? ''}`,
              }))}
              value={subject}
              onChange={(v) => { setSubject(v); setResult(null) }}
            />
            {subj && realm && (
              <div className="mt-2.5 flex flex-wrap items-center gap-1.5">
                <span className="chip">{realm.display_name}</span>
                <Tooltip label="NIST 800-63 Identity Assurance Level, gated against each permission's minimum.">
                  <span className="chip" style={{
                    color: subj.assurance_level >= 3 ? 'var(--allow)' : 'var(--warn)',
                    borderColor: subj.assurance_level >= 3 ? 'color-mix(in srgb, var(--allow) 20%, transparent)' : 'color-mix(in srgb, var(--warn) 20%, transparent)',
                  }}>IAL{subj.assurance_level}</span>
                </Tooltip>
                <span className="chip" style={{
                  color: subj.status === 'active' ? 'var(--ink-2)' : 'var(--deny)',
                  borderColor: subj.status === 'active' ? 'var(--line-soft)' : 'color-mix(in srgb, var(--deny) 20%, transparent)',
                }}>{subj.status}</span>
              </div>
            )}
          </div>

          <div className="panel p-4">
            <div className="t-label mb-2.5">Permission</div>
            <Select
              searchable clearable placeholder="Select a permission"
              data={(permissions ?? []).map((p) => ({ value: p.key, label: p.key }))}
              value={permission}
              onChange={(v) => { setPermission(v); setResult(null) }}
            />
            {perm && (
              <div className="mt-2.5 flex flex-wrap items-center gap-1.5">
                <span className="chip" style={{
                  color: perm.risk === 'critical' ? 'var(--deny)'
                    : perm.risk === 'sensitive' ? 'var(--warn)' : 'var(--ink-2)',
                }}>{perm.risk}</span>
                <span className="chip">min IAL{perm.min_assurance}</span>
                {perm.requires_amr.length > 0 && (
                  <Tooltip label={`Requires ${perm.requires_amr.join(', ')} within ${perm.max_auth_age}`}>
                    <span className="chip" style={{ color: 'var(--info)', borderColor: 'color-mix(in srgb, var(--info) 20%, transparent)' }}>
                      <IconShieldLock size={9} style={{ marginRight: 4 }} />step-up
                    </span>
                  </Tooltip>
                )}
              </div>
            )}
          </div>

          <div>
            <div className="mb-2 flex items-baseline justify-between">
              <div className="t-label">Target scope</div>
              <div className="t-xs" style={{ fontSize: 10 }}>from the axis registry</div>
            </div>
            <AxisTargetPicker
              targets={targets}
              onChange={(a, v) => { setTarget(a, v); setResult(null) }}
              showUnsetWarning={touched}
              ownerValue={targets['_owner'] ?? null}
              onOwnerChange={(v) => setTarget('_owner', v)}
            />
            {subject && (
              <button
                onClick={() => setTarget('_owner', targets['_owner'] ? null : subject)}
                className="t-xs mt-2 underline underline-offset-2"
                style={{ color: 'var(--ink-2)' }}
              >
                {targets['_owner'] ? 'Clear _owner' : 'Set _owner to subject'}
              </button>
            )}
          </div>
        </div>

        {/* ---------------- verdict ---------------- */}
        <div className="flex flex-col gap-4">
          {!result && !running && (
            <div className="panel flex flex-col items-center justify-center px-6 py-16 text-center">
              <div
                className="mb-4 flex items-center justify-center rounded-full"
                style={{ width: 46, height: 46, background: 'var(--gold-glow)' }}
              >
                <IconBolt size={20} style={{ color: 'var(--gold)' }} />
              </div>
              <div className="t-h1">Nothing evaluated yet</div>
              <p className="t-sm mt-1.5" style={{ maxWidth: 380 }}>
                Pick a subject and a permission, then press Evaluate. The trace shows every
                candidate grant and the gate that stopped it.
              </p>
              <div className="mt-5 flex w-full max-w-[400px] flex-col gap-1.5">
                {PRESETS.map((p) => (
                  <button
                    key={p.label}
                    onClick={() => p.apply({ setSubject, setPermission, setTarget, identities, permissions })}
                    className="panel-inset panel-hover flex items-center gap-2.5 px-3 py-2.5 text-left"
                  >
                    <IconChevronRight size={12} style={{ color: 'var(--gold)', flexShrink: 0 }} />
                    <div className="min-w-0">
                      <div className="t-sm" style={{ color: 'var(--ink)' }}>{p.label}</div>
                      <div className="t-xs">{p.hint}</div>
                    </div>
                  </button>
                ))}
              </div>
            </div>
          )}

          {running && (
            <div className="panel flex items-center justify-center py-20">
              <Loader size="sm" color="gold" />
            </div>
          )}

          {result && !running && (
            <>
              <div
                className="panel rise relative overflow-hidden p-5"
                style={{
                  borderColor: result.allow ? 'color-mix(in srgb, var(--allow) 25%, transparent)' : 'color-mix(in srgb, var(--deny) 25%, transparent)',
                  background: result.allow
                    ? 'linear-gradient(180deg, var(--allow-bg), var(--s-raised) 60%)'
                    : 'linear-gradient(180deg, var(--deny-bg), var(--s-raised) 60%)',
                }}
              >
                <div className="flex items-start justify-between gap-4">
                  <div className="flex items-center gap-3.5">
                    <div
                      className="flex items-center justify-center rounded-lg"
                      style={{
                        width: 44, height: 44,
                        background: result.allow ? 'color-mix(in srgb, var(--allow) 9%, transparent)' : 'color-mix(in srgb, var(--deny) 9%, transparent)',
                        border: `1px solid ${result.allow ? 'color-mix(in srgb, var(--allow) 25%, transparent)' : 'color-mix(in srgb, var(--deny) 25%, transparent)'}`,
                      }}
                    >
                      {result.allow
                        ? <IconCheck size={22} style={{ color: 'var(--allow)' }} />
                        : <IconX size={22} style={{ color: 'var(--deny)' }} />}
                    </div>
                    <div>
                      <div
                        style={{
                          fontSize: 24, fontWeight: 680, letterSpacing: '-.02em', lineHeight: 1.1,
                          color: result.allow ? 'var(--allow)' : 'var(--deny)',
                        }}
                      >
                        {result.allow ? 'ALLOW' : 'DENY'}
                      </div>
                      {result.reason && (
                        <div className="t-sm mt-1" style={{ maxWidth: 460 }}>
                          {REASON[result.reason] ?? result.reason}
                        </div>
                      )}
                    </div>
                  </div>
                  <div className="text-right">
                    <div className="t-label">decided in</div>
                    {/* A literal "0 ms" reads as a broken timer rather than a
                        fast one. Floor the display instead of rounding to zero. */}
                    <div className="tnum mt-1" style={{ fontSize: 15, fontWeight: 600 }}>
                      {result.took_ms < 0.01
                        ? <>&lt;0.01<span className="t-xs"> ms</span></>
                        : <>{result.took_ms.toFixed(2)}<span className="t-xs"> ms</span></>}
                    </div>
                  </div>
                </div>

                {result.failing_axis && (
                  <div className="mt-3.5 flex items-center gap-2">
                    <span className="t-xs">failing axis</span>
                    <span className="chip" style={{ color: 'var(--deny)', borderColor: 'color-mix(in srgb, var(--deny) 20%, transparent)' }}>
                      {result.failing_axis}
                    </span>
                  </div>
                )}
                {result.reason === 'step_up_required' && (
                  <div className="panel-inset mt-3.5 px-3 py-2.5">
                    <div className="t-sm">
                      Requires <span className="chip">{result.required_amr?.join(', ')}</span> within{' '}
                      <span className="chip">{result.max_auth_age}</span>; the session has{' '}
                      <span className="chip">{result.current_amr?.join(', ')}</span>.
                    </div>
                  </div>
                )}
                {result.message && !result.failing_axis && (
                  <div className="t-sm mt-3">{result.message}</div>
                )}
              </div>

              <div>
                <div className="mb-2.5 flex items-baseline justify-between">
                  <div className="t-label">Decision trace · {result.evaluations.length} candidate grant{result.evaluations.length === 1 ? '' : 's'}</div>
                  <div className="t-xs">access is granted if <b>any</b> grant clears every gate</div>
                </div>
                <div className="flex flex-col gap-2.5">
                  {result.evaluations.length === 0 ? (
                    <div className="panel px-4 py-8 text-center">
                      <div className="t-sm">No grant confers this permission, so no axis was evaluated.</div>
                    </div>
                  ) : (
                    result.evaluations.map((e) => <Trace key={e.grant_id} e={e} />)
                  )}
                </div>
              </div>
            </>
          )}
        </div>
      </div>
    </Page>
  )
}
