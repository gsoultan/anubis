import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button, Checkbox, Modal, Select, TextInput } from '@mantine/core'
import { notifyCreated, notifyRejected } from '@/components/create/shell'
import { api } from '@/lib/anubis'
import * as live from '@/lib/api/live'
import { treeOrder } from '@/lib/tree'
import type { Tenant } from '@/lib/api/types'

type Assignment = { id: string; tenantSlug: string; role: string }
type Pick = Record<string, string>

const LEVELS = [
  { value: 'admin', label: 'Full admin' },
  { value: 'support', label: 'Support: people only' },
]
const levelWord = (r: string) => (r === 'admin' ? 'full admin' : r === 'support' ? 'support' : r)

/* Which companies someone runs, as a tick per company. Ticking a parent
   covers every company below it, including ones added later, so the rows
   under a ticked one say so instead of offering a second tick. Saved as the
   difference: new ticks are assigned, cleared ones revoked, a changed level
   is revoked and assigned again. */
export function CompanyTicks({ username, assignments, onClose, onSaved }: {
  username: string | null; assignments: Assignment[]; onClose: () => void; onSaved: () => void
}) {
  const { data: tenants } = useQuery({ queryKey: ['tenants'], queryFn: live.tenants, enabled: !!username })
  const start = useMemo(() => Object.fromEntries(assignments.filter((a) => a.tenantSlug && a.role !== 'owner').map((a) => [a.tenantSlug, a.role])) as Pick,
    [assignments])
  const [pick, setPick] = useState<Pick>(start)
  const [reason, setReason] = useState('')
  const [busy, setBusy] = useState(false)
  useEffect(() => { setPick(start); setReason('') }, [start, username])

  const all = tenants ?? []
  const byId = new Map(all.map((t) => [t.id, t]))
  const coveredBy = (t: Tenant): Tenant | undefined => {
    for (let p = t.parent_id ? byId.get(t.parent_id) : undefined; p; p = p.parent_id ? byId.get(p.parent_id) : undefined) {
      if (pick[p.slug]) return p
    }
    return undefined
  }
  const below = (t: Tenant) => all.filter((x) => { for (let p = x.parent_id ? byId.get(x.parent_id) : undefined; p; p = p.parent_id ? byId.get(p.parent_id) : undefined) if (p.id === t.id) return true; return false }).length
  const everyTenant = assignments.some((a) => !a.tenantSlug)
  const owner = assignments.some((a) => a.role === 'owner')

  const ticked = all.filter((t) => pick[t.slug] && !coveredBy(t))
  const sentence = ticked.length === 0 ? `${username} will run no company.`
    : `${username} will run ${ticked.map((t) => `${t.name}${below(t) ? ` and the ${below(t)} ${below(t) === 1 ? 'company' : 'companies'} below it` : ''} as ${levelWord(pick[t.slug]!)}`).join(', and ')}.`

  async function save() {
    if (!username) return
    setBusy(true)
    try {
      const keep = Object.fromEntries(ticked.map((t) => [t.slug, pick[t.slug]!]))
      for (const a of assignments) {
        if (!a.tenantSlug || a.role === 'owner') continue
        if (keep[a.tenantSlug] !== a.role) await api.platformAdmin.revokeAssignment({ assignmentId: a.id })
      }
      for (const [slug, role] of Object.entries(keep)) {
        if (start[slug] === role) continue
        await api.platformAdmin.assignOperator({ operatorUsername: username, tenantSlug: slug, role, reason: reason.trim() })
      }
      notifyCreated('Saved', sentence)
      onSaved()
    } catch (e) { notifyRejected(e) } finally { setBusy(false) }
  }

  return (
    <Modal opened={!!username} onClose={onClose} centered size="lg" title={`Which companies can ${username ?? ''} run?`}>
      <p className="t-sm mb-3">Tick a company. Ticking a parent covers every company below it, including ones added later.</p>
      {(everyTenant || owner) && (
        <div className="t-xs mb-2" style={{ color: 'var(--warn)' }}>
          {username} also has authority over every company{owner ? ' as an owner' : ''}. That is not changed here.
        </div>
      )}
      <div className="panel-inset flex max-h-[50vh] flex-col overflow-y-auto">
        {treeOrder(all, (t) => t.id, (t) => t.parent_id ?? '', (t) => t.name).map(({ item: t, depth }) => {
          const cover = coveredBy(t)
          const on = !!pick[t.slug] && !cover
          return (
            <div key={t.id} className="flex flex-wrap items-center gap-2 px-3 py-1.5" style={{ paddingLeft: 12 + depth * 20, borderBottom: '1px solid var(--line-soft)' }}>
              <Checkbox size="xs" checked={on || !!cover} disabled={!!cover} aria-label={t.name}
                onChange={(e) => setPick((p) => { const n = { ...p }; if (e.currentTarget.checked) n[t.slug] = 'admin'; else delete n[t.slug]; return n })} />
              <span className="min-w-0 flex-1">
                <span className="t-body" style={{ fontWeight: on ? 560 : 450 }}>{t.name}</span>
                {cover && <span className="t-xs ml-2">covered by {cover.name}</span>}
              </span>
              {on && (
                <Select size="xs" w={190} value={pick[t.slug] ?? 'admin'} allowDeselect={false} data={LEVELS}
                  aria-label={`Level at ${t.name}`} onChange={(v) => setPick((p) => ({ ...p, [t.slug]: v ?? 'admin' }))} />
              )}
            </div>
          )
        })}
      </div>
      <p className="t-sm mt-3">{sentence}</p>
      <TextInput mt="sm" size="xs" label="Reason" placeholder="Kept in the activity log" value={reason} onChange={(e) => setReason(e.currentTarget.value)} />
      <div className="mt-3 flex justify-end gap-2">
        <Button size="xs" variant="default" onClick={onClose}>Cancel</Button>
        <Button size="xs" loading={busy} onClick={() => void save()}>Save</Button>
      </div>
    </Modal>
  )
}
