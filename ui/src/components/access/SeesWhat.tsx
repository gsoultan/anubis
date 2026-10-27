import { useMemo, useState } from 'react'
import { useQueries, useQuery } from '@tanstack/react-query'
import { Button, Select } from '@mantine/core'
import { IconCheck, IconX } from '@tabler/icons-react'
import { IdentityPicker } from '@/components/ui/IdentityPicker'
import { AxisTargetPicker } from '@/components/scope/AxisTargetPicker'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { groupFeatures, shortRoleName } from '@/lib/access'
import { GrantWhere } from './GrantWhere'
import type { AuthorizeResponse, Grant, Permission } from '@/lib/api/types'

const label = (p: Permission) => p.description || `${p.action} ${p.resource}`
const live = (g: Grant) => !g.revoked_at && (!g.valid_until || new Date(g.valid_until) > new Date())

/* One person in one app: what they can do there, feature by feature, which
   data each feature reaches and which role it comes from, and a record to
   try against all of it. The question people bring, not the API's. */
export function SeesWhat({ subject, onSubject }: { subject: string | null; onSubject: (id: string | null) => void }) {
  const [app, setApp] = useState<string | null>(null)
  const [action, setAction] = useState<string | null>(null)
  const [targets, setTargets] = useState<Record<string, string>>({})
  const [answer, setAnswer] = useState<AuthorizeResponse | null>(null)
  const [busy, setBusy] = useState(false)

  const { data: who } = useQuery({ queryKey: qk.identity(subject ?? ''), queryFn: () => api.identity(subject!), enabled: !!subject })
  const { data: grants } = useQuery({ queryKey: qk.grants(subject ?? ''), queryFn: () => api.grants(subject!), enabled: !!subject })
  const { data: perms } = useQuery({ queryKey: qk.permissions(), queryFn: api.permissions })
  const { data: axes } = useQuery({ queryKey: qk.axes(), queryFn: api.axes })
  const held = (grants ?? []).filter(live)
  const roleIds = [...new Set(held.map((g) => g.role_id))]
  const keyLists = useQueries({
    queries: roleIds.map((id) => ({ queryKey: qk.rolePermissions(id), queryFn: () => api.rolePermissions(id) })),
  })
  const keysOf = (roleId: string) => keyLists[roleIds.indexOf(roleId)]?.data ?? []
  const ids = [...new Set(held.flatMap((g) => g.scopes.map((s) => s.scope_node_id)))].sort()
  const { data: nodes } = useQuery({ queryKey: ['scope-names', ids], queryFn: () => api.scopeNodesByIds(ids), enabled: ids.length > 0 })
  const nodeName = (id: string) => nodes?.find((n) => n.id === id)?.name ?? '…'

  /* The apps they can do anything in, so the picker never offers an empty answer. */
  const reachable = useMemo(() => {
    const keys = new Set(roleIds.flatMap(keysOf))
    return [...new Set((perms ?? []).filter((p) => keys.has(p.key)).map((p) => p.app_slug))].sort()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [perms, roleIds.join(), keyLists.map((q) => q.dataUpdatedAt).join()])
  const appPick = app && reachable.includes(app) ? app : reachable[0] ?? null
  const appPerms = (perms ?? []).filter((p) => p.app_slug === appPick && !p.deprecated_at)
  const grantsFor = (key: string) => held.filter((g) => keysOf(g.role_id).includes(key))
  const name = who?.username ?? 'They'

  const mods = groupFeatures(appPerms)
  const features = mods.flatMap((m) => m.features.map((f) => ({ module: m.module, ...f })))
    .filter((f) => f.actions.some((a) => grantsFor(a.key).length > 0))

  async function tryIt() {
    if (!subject || !action) return
    setBusy(true)
    try { setAnswer(await api.authorize({ subject, permission: action, scopes: targets })) } finally { setBusy(false) }
  }
  const best = answer?.evaluations.find((e) => e.survived) ?? answer?.evaluations[0]
  const actionPerm = appPerms.find((p) => p.key === action)

  return (
    <div className="flex flex-col gap-4">
      <div className="panel flex flex-wrap items-end gap-3 p-4">
        <div className="min-w-[240px] flex-1">
          <div className="t-xs mb-1">Person</div>
          <IdentityPicker placeholder="Search for a person…" value={subject} onChange={(v) => { onSubject(v); setAnswer(null) }} />
        </div>
        <Select w={220} label="App" value={appPick} onChange={(v) => { setApp(v); setAction(null); setAnswer(null) }}
          data={reachable} disabled={reachable.length === 0}
          placeholder={subject ? 'Nothing in any app' : 'Pick a person first'} />
      </div>

      {subject && appPick && (
        <div className="grid items-start gap-4 lg:grid-cols-[minmax(0,1fr)_380px]">
          <div className="flex flex-col gap-4">
            <section className="panel p-4">
              <h2 className="t-h2 mb-3">What {name} can do in {appPick}</h2>
              <div className="flex flex-col gap-3">
                {mods.map((m) => (
                  <div key={m.module || 'x'}>
                    {m.module && <div className="t-label mb-1">{m.module}</div>}
                    {m.features.map((f) => (
                      <div key={f.resource} className="flex flex-wrap items-baseline gap-x-3 gap-y-1 py-1.5" style={{ borderBottom: '1px solid var(--line-soft)' }}>
                        <span className="t-body min-w-[120px]" style={{ fontWeight: 550 }}>{f.resource}</span>
                        <span className="flex flex-wrap gap-x-3 gap-y-1">
                          {f.actions.map((a) => {
                            const yes = grantsFor(a.key).length > 0
                            return (
                              <span key={a.id} className="t-sm inline-flex items-center gap-1" style={{ color: yes ? 'var(--ink)' : 'var(--ink-4)' }}>
                                {yes ? <IconCheck size={13} style={{ color: 'var(--allow)' }} /> : <IconX size={12} />}{label(a)}
                              </span>
                            )
                          })}
                        </span>
                      </div>
                    ))}
                  </div>
                ))}
              </div>
            </section>

            <section className="panel p-4">
              <h2 className="t-h2 mb-3">Which data, per feature</h2>
              {features.length === 0 && <div className="t-sm">Nothing in this app.</div>}
              <div className="flex flex-col gap-4">
                {features.map((f) => {
                  const fromGrants = [...new Map(f.actions.flatMap((a) => grantsFor(a.key)).map((g) => [g.id, g])).values()]
                  return (
                    <div key={f.resource} className="flex flex-col gap-2">
                      {fromGrants.map((g) => {
                        const can = f.actions.filter((a) => keysOf(g.role_id).includes(a.key)).map((a) => a.action)
                        return (
                          <div key={g.id} className="panel-inset flex flex-col gap-1.5 px-3 py-2.5">
                            <div className="t-body"><b>{f.resource}</b>: {can.join(', ')}</div>
                            <GrantWhere scopes={g.scopes} selfScoped={g.self_scoped} axes={axes} nodeName={nodeName} compact />
                            <div className="t-xs">From the role {shortRoleName({ name: g.role_name, application_id: appPick })}{g.via_membership_id ? ', through a group' : ''}</div>
                          </div>
                        )
                      })}
                    </div>
                  )
                })}
              </div>
            </section>
          </div>

          <section className="panel flex flex-col gap-3 p-4 lg:sticky lg:top-4">
            <h2 className="t-h2">Try a record</h2>
            <Select label="Action" searchable value={action} onChange={(v) => { setAction(v); setAnswer(null) }}
              data={appPerms.map((p) => ({ value: p.key, label: label(p) }))} />
            <AxisTargetPicker targets={targets} onChange={(a, v) => {
              setAnswer(null)
              setTargets((t) => { const n = { ...t }; if (v) n[a] = v; else delete n[a]; return n })
            }} />
            <Button size="xs" disabled={!action} loading={busy} onClick={() => void tryIt()}>Check</Button>
            {answer && (
              <div className="flex flex-col gap-1.5" role="status">
                <div className="t-body" style={{ fontWeight: 600, color: answer.allow ? 'var(--allow)' : 'var(--deny)' }}>
                  {answer.allow ? `Yes, ${name} can ${actionPerm ? label(actionPerm).toLowerCase() : 'do this'} here` : `No, ${name} cannot ${actionPerm ? label(actionPerm).toLowerCase() : 'do this'} here`}
                </div>
                {answer.evaluations.length === 0 && <div className="t-sm">No role of theirs allows this action at all.</div>}
                {best && (
                  <>
                    <div className="t-sm inline-flex items-center gap-1.5">
                      <IconCheck size={13} style={{ color: 'var(--allow)' }} />The role {best.role_name} allows it
                    </div>
                    {best.axes.filter((v) => v.constrained).map((v) => (
                      <div key={v.axis_code} className="t-sm inline-flex items-start gap-1.5">
                        {v.satisfied ? <IconCheck size={13} style={{ color: 'var(--allow)', marginTop: 3 }} /> : <IconX size={13} style={{ color: 'var(--deny)', marginTop: 3 }} />}
                        <span>
                          {(axes?.find((a) => a.code === v.axis_code)?.display_name ?? v.axis_code)}:{' '}
                          {v.target_node_name ?? 'not given'} {v.satisfied ? 'is in their limit' : `is not in ${(v.granted_nodes ?? []).filter((n) => !n.exclude).map((n) => n.name).join(' or ') || 'their limit'}`}
                        </span>
                      </div>
                    ))}
                  </>
                )}
              </div>
            )}
          </section>
        </div>
      )}
      {subject && !appPick && grants && <div className="panel px-4 py-6 text-center t-sm">{name} cannot do anything in any app yet.</div>}
    </div>
  )
}
