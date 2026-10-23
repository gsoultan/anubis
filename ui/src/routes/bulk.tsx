import { createFileRoute } from '@tanstack/react-router'
import { useMutation, useQuery } from '@tanstack/react-query'
import { Button, Checkbox, Loader, Select, TextInput, Textarea, UnstyledButton } from '@mantine/core'
import { DateInput } from '@mantine/dates'
import {
  IconArrowBackUp, IconArrowsExchange, IconCalendarTime, IconMapPinMinus, IconMapPinPlus,
  IconInfinity, IconUserMinus,
} from '@tabler/icons-react'
import { notifications } from '@mantine/notifications'
import { useState, type ReactNode } from 'react'
import { Page } from '@/components/shell/Page'
import { GrantWhere } from '@/components/access/GrantWhere'
import { NodePick } from '@/components/access/NodePick'
import { notifyRejected } from '@/components/create/shell'
import { api } from '@/lib/api/client'
import { fmtDate, relDays } from '@/lib/access'
import { queryClient } from '@/lib/query/client'
import { qk } from '@/lib/query/keys'
import type { GrantChangeKind, GrantChangeRecord, GrantChangeRow, GrantChangeSpec } from '@/lib/api/types'

export const Route = createFileRoute('/bulk')({
  validateSearch: (s: Record<string, unknown>): { role?: string; axis?: string; node?: string; siblings?: string; kind?: GrantChangeKind } => ({
    ...(typeof s.siblings === 'string' ? { siblings: s.siblings } : {}),
    ...(typeof s.role === 'string' ? { role: s.role } : {}),
    ...(typeof s.axis === 'string' ? { axis: s.axis } : {}),
    ...(typeof s.node === 'string' ? { node: s.node } : {}),
    ...(typeof s.kind === 'string' && s.kind in KINDS ? { kind: s.kind as GrantChangeKind } : {}),
  }),
  component: Bulk,
})

/* One change, many people.

   At two hundred people nobody edits grants one page at a time, and at two
   thousand nobody should. The page asks three things in order: which access
   (always one role, so the change can be read back in a sentence), what
   should change, and then shows exactly what will happen before anything
   does. What is applied is saved as one change with one reason, and undone
   as one. */

const KINDS: Record<GrantChangeKind, { label: string; icon: ReactNode; hint: string }> = {
  add_place: { label: 'Add a place', icon: <IconMapPinPlus size={15} />, hint: 'Put one more place on their list.' },
  remove_place: { label: 'Remove a place', icon: <IconMapPinMinus size={15} />, hint: 'Take one place off their list. Nobody is left with nothing.' },
  swap_place: { label: 'Swap one place for another', icon: <IconArrowsExchange size={15} />, hint: 'Where the list names the first place, name the second instead.' },
  grow: { label: 'Make it grow by itself', icon: <IconInfinity size={15} />, hint: 'Replace a hand-picked list with one that covers new places as they are added.' },
  set_until: { label: 'Change the end date', icon: <IconCalendarTime size={15} />, hint: 'Set when the access ends, or make it last until taken away.' },
  revoke: { label: 'Take the access away', icon: <IconUserMinus size={15} />, hint: 'End the access now. It can be undone from the list below.' },
}

const SKIP_WORDS: Record<string, string> = {
  'from a group': 'have it from a group, so change the group instead',
  'self-scoped': 'only see records they own, which has no places',
}

function Step({ n, title, children }: { n: number; title: string; children: ReactNode }) {
  return (
    <section className="panel flex flex-col gap-3 p-4">
      <h2 className="t-h3 flex items-center gap-2">
        <span className="chip tnum">{n}</span>{title}
      </h2>
      {children}
    </section>
  )
}

function Bulk() {
  const search = Route.useSearch()
  const { data: roles } = useQuery({ queryKey: qk.roles(), queryFn: api.roles })
  const { data: axes } = useQuery({ queryKey: qk.axes(), queryFn: api.axes })

  const [roleId, setRoleId] = useState<string | null>(search.role ?? null)
  const [axis, setAxis] = useState<string | null>(search.axis ?? null)
  const [covers, setCovers] = useState<string | null>(null)
  /* From "who might need it": only lists naming every place under this one. */
  const [siblingsOf, setSiblingsOf] = useState<string | null>(search.siblings ?? null)
  const [kind, setKind] = useState<GrantChangeKind>(search.kind ?? 'add_place')
  const [node, setNode] = useState<string | null>(search.node ?? null)
  const [toNode, setToNode] = useState<string | null>(null)
  const [inherit, setInherit] = useState(true)
  const [growAll, setGrowAll] = useState(!(search.kind === 'grow' && search.node))
  const [until, setUntil] = useState<Date | null>(null)
  const [reason, setReason] = useState('')

  const needsAxis = kind !== 'set_until' && kind !== 'revoke'
  const axisDef = axes?.find((a) => a.code === axis)
  const noun = axisDef?.display_name.toLowerCase() ?? 'place'

  /* The spec as the server reads it, or null while it is not yet complete
     enough to preview: a change that cannot be read back is not shown. */
  const spec: GrantChangeSpec | null = (() => {
    if (!roleId) return null
    if ((needsAxis || covers || siblingsOf) && !axis) return null
    const s: GrantChangeSpec = { role_id: roleId, kind }
    if (axis && (needsAxis || covers || siblingsOf)) s.axis = axis
    if (covers) s.covers_node_ids = [covers]
    if (siblingsOf) s.siblings_of = siblingsOf
    if (kind === 'add_place' || kind === 'remove_place' || kind === 'swap_place') {
      if (!node) return null
      s.node_id = node
    }
    if (kind === 'swap_place') {
      if (!toNode || toNode === node) return null
      s.to_node_id = toNode
    }
    if (kind === 'add_place' || kind === 'swap_place') s.inherit = inherit
    if (kind === 'grow' && !growAll) {
      if (!node) return null
      s.node_id = node
    }
    if (kind === 'set_until') s.valid_until = until ? until.toISOString() : null
    if (kind === 'revoke' && !reason.trim()) return null
    if (reason.trim()) s.reason = reason.trim()
    return s
  })()

  const preview = useQuery({
    queryKey: ['grant-change', 'preview', spec],
    queryFn: () => api.previewGrantChange(spec!),
    enabled: spec !== null,
    retry: false,
  })
  const p = spec ? preview.data : undefined

  const ids = [...new Set((p?.sample ?? []).flatMap((r) => [...r.before, ...r.after].map((s) => s.scope_node_id)))].sort()
  const { data: nodes } = useQuery({
    queryKey: ['scope-names', ids], queryFn: () => api.scopeNodesByIds(ids), enabled: ids.length > 0,
  })
  const nodeName = (id: string) => nodes?.find((n) => n.id === id)?.name ?? '…'

  const apply = useMutation({
    mutationFn: (s: GrantChangeSpec) => api.applyGrantChange(s),
    onSuccess: (r) => {
      notifications.show({ color: 'teal', message: `Changed ${r.changed} ${r.changed === 1 ? 'grant' : 'grants'}. You can undo it below.` })
      void queryClient.invalidateQueries({ queryKey: ['grant-change'] })
      void queryClient.invalidateQueries({ queryKey: qk.grants() })
      void queryClient.invalidateQueries({ queryKey: ['identities'] })
    },
    onError: (e) => notifyRejected(e),
  })

  const role = roles?.find((r) => r.id === roleId)
  const roleOptions = (roles ?? []).filter((r) => !r.deprecated).map((r) => ({ value: r.id, label: r.name }))
  const axisOptions = (axes ?? []).map((a) => ({ value: a.code, label: a.display_name }))

  return (
    <Page title="Change access in bulk" back={{ to: '/identities', label: 'People' }}
      description="Find the access to change, pick one change, check the preview, apply. It is saved as one change you can undo.">
      <div className="flex max-w-[860px] flex-col gap-4">
        <Step n={1} title="Which access?">
          <div className="grid gap-3 sm:grid-cols-2">
            <Select label="Role is" placeholder="Pick a role" searchable data={roleOptions}
              value={roleId} onChange={setRoleId} />
            <Select label="Kind of place" placeholder="Pick a kind" data={axisOptions} value={axis}
              onChange={(v) => { setAxis(v); setCovers(null); setSiblingsOf(null); setNode(null); setToNode(null) }}
              description={needsAxis ? 'The change is about this kind of place.' : 'Only needed to narrow who is changed.'} />
          </div>
          {axis && (
            <div className="flex flex-col gap-1">
              <span className="t-label">Only people whose {noun} list reaches (optional)</span>
              <div className="flex items-center gap-2">
                <div className="min-w-0 flex-1">
                  <NodePick axis={axis} value={covers} onChange={(id) => setCovers(id)} placeholder={`Anyone with ${role?.name ?? 'the role'}`} />
                </div>
                {covers && <Button variant="subtle" color="gray" size="xs" onClick={() => setCovers(null)}>Clear</Button>}
              </div>
            </div>
          )}
          {siblingsOf && (
            <div className="t-sm flex flex-wrap items-center gap-2">
              Only people whose list names every place beside the one being added.
              <Button variant="subtle" color="gray" size="compact-xs" onClick={() => setSiblingsOf(null)}>Everyone with the role</Button>
            </div>
          )}
          {p && (
            <div className="t-body">
              <b className="tnum">{p.matched}</b> {p.matched === 1 ? 'grant matches' : 'grants match'}
              {role && <> for <b>{role.name}</b></>}.
            </div>
          )}
        </Step>

        <Step n={2} title="What should change?">
          <div className="grid gap-2 sm:grid-cols-2">
            {(Object.keys(KINDS) as GrantChangeKind[]).map((k) => (
              <UnstyledButton key={k} onClick={() => setKind(k)} aria-pressed={kind === k}
                className="flex items-start gap-2 rounded-md p-2.5"
                style={{
                  border: `1px solid ${kind === k ? 'var(--accent)' : 'var(--line-soft)'}`,
                  background: kind === k ? 'var(--accent-bg, var(--s-sunken))' : undefined,
                }}>
                <span style={{ color: kind === k ? 'var(--accent)' : 'var(--ink-3)', marginTop: 2 }}>{KINDS[k].icon}</span>
                <span className="min-w-0">
                  <span className="t-body block" style={{ fontWeight: 550 }}>{KINDS[k].label}</span>
                  <span className="t-xs block">{KINDS[k].hint}</span>
                </span>
              </UnstyledButton>
            ))}
          </div>

          {needsAxis && !axis && <div className="t-sm" style={{ color: 'var(--warn)' }}>Pick the kind of place above first.</div>}

          {axis && (kind === 'add_place' || kind === 'remove_place' || kind === 'swap_place') && (
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="flex flex-col gap-1">
                <span className="t-label">{kind === 'add_place' ? 'Add' : kind === 'remove_place' ? 'Remove' : 'Swap'}</span>
                <NodePick axis={axis} value={node} onChange={(id) => setNode(id)} />
              </div>
              {kind === 'swap_place' && (
                <div className="flex flex-col gap-1">
                  <span className="t-label">For</span>
                  <NodePick axis={axis} value={toNode} onChange={(id) => setToNode(id)} />
                </div>
              )}
            </div>
          )}
          {axis && (kind === 'add_place' || kind === 'swap_place') && (
            <Checkbox checked={inherit} onChange={(e) => setInherit(e.currentTarget.checked)}
              label="And everything inside it, including what is added later" />
          )}
          {axis && kind === 'grow' && (
            <div className="flex flex-col gap-2">
              <Checkbox checked={growAll} onChange={(e) => setGrowAll(e.currentTarget.checked)}
                label={`Every ${noun}, including new ones`} />
              {!growAll && (
                <div className="flex flex-col gap-1">
                  <span className="t-label">Replace the picks inside this branch with the branch itself</span>
                  <NodePick axis={axis} value={node} onChange={(id) => setNode(id)} />
                </div>
              )}
            </div>
          )}
          {kind === 'set_until' && (
            <DateInput label="Ends on" placeholder="Never: lasts until taken away" clearable
              minDate={new Date(Date.now() + 86_400_000)} value={until}
              onChange={(v) => setUntil(v ? new Date(v) : null)} />
          )}
          <Textarea label={kind === 'revoke' ? 'Why is it being taken away?' : 'Why (optional)'} autosize minRows={1}
            value={reason} onChange={(e) => setReason(e.currentTarget.value)} required={kind === 'revoke'}
            description="Saved with the change, so whoever reads the activity log later knows." />
        </Step>

        <Step n={3} title="Preview">
          {!spec && <div className="t-sm">Fill in the steps above to see exactly who changes.</div>}
          {spec && preview.isLoading && <Loader size="sm" />}
          {spec && preview.isError && (
            <div className="t-sm" style={{ color: 'var(--deny)' }}>{(preview.error as Error).message}</div>
          )}
          {p && (
            <>
              <div className="t-body">
                <b className="tnum">{p.changes}</b> {p.changes === 1 ? 'grant changes' : 'grants change'} for{' '}
                <b className="tnum">{p.people}</b> {p.people === 1 ? 'person' : 'people'}.
              </div>
              {p.sample.length > 0 && <PreviewTable rows={p.sample} more={p.changes - p.sample.length} axes={axes} nodeName={nodeName} />}
              {p.skipped.length > 0 && (
                <ul className="t-sm flex flex-col gap-0.5" style={{ color: 'var(--ink-2)' }}>
                  {p.skipped.map((s) => (
                    <li key={s.reason}>
                      <span className="tnum">{s.count}</span> skipped: {SKIP_WORDS[s.reason] ?? s.reason}
                    </li>
                  ))}
                </ul>
              )}
              <div>
                <Button disabled={p.changes === 0} loading={apply.isPending} onClick={() => spec && apply.mutate(spec)}
                  {...(kind === 'revoke' ? { color: 'red' } : {})}>
                  {p.changes === 0 ? 'Nothing to change' : `Apply to ${p.people} ${p.people === 1 ? 'person' : 'people'}`}
                </Button>
              </div>
            </>
          )}
        </Step>

        <RecentChanges />
      </div>
    </Page>
  )
}

function Until({ at, revoked }: { at: string | null; revoked: boolean }) {
  if (revoked) return <span className="t-xs" style={{ color: 'var(--deny)' }}>Taken away</span>
  return at ? <span className="t-xs">until {fmtDate(at)}</span> : null
}

function PreviewTable({ rows, more, axes, nodeName }: {
  rows: GrantChangeRow[]
  more: number
  axes: Parameters<typeof GrantWhere>[0]['axes']
  nodeName: (id: string) => string
}) {
  return (
    <div className="flex flex-col">
      <div className="t-label grid grid-cols-[minmax(0,1fr)_minmax(0,2fr)_minmax(0,2fr)] gap-3 border-b pb-1.5" style={{ borderColor: 'var(--line-soft)' }}>
        <span>Person</span><span>Now</span><span>After</span>
      </div>
      {rows.map((r) => (
        <div key={r.grant_id} className="grid grid-cols-[minmax(0,1fr)_minmax(0,2fr)_minmax(0,2fr)] gap-3 border-b py-2" style={{ borderColor: 'var(--line-soft)' }}>
          <span className="t-body truncate" style={{ fontWeight: 520 }}>{r.username}</span>
          <div className="min-w-0">
            <GrantWhere scopes={r.before} selfScoped={false} axes={axes} nodeName={nodeName} compact />
            <Until at={r.before_until} revoked={false} />
          </div>
          <div className="min-w-0">
            <GrantWhere scopes={r.after} selfScoped={false} axes={axes} nodeName={nodeName} compact />
            <Until at={r.after_until} revoked={r.revoked} />
          </div>
        </div>
      ))}
      {more > 0 && <div className="t-xs pt-2">and {more} more</div>}
    </div>
  )
}

function RecentChanges() {
  const { data: changes } = useQuery({ queryKey: ['grant-change', 'list'], queryFn: () => api.grantChanges(20) })
  const { data: roles } = useQuery({ queryKey: qk.roles(), queryFn: api.roles })
  const [undoing, setUndoing] = useState<GrantChangeRecord | null>(null)
  const [why, setWhy] = useState('')
  const undo = useMutation({
    mutationFn: (c: GrantChangeRecord) => api.undoGrantChange(c.id, why.trim() || undefined),
    onSuccess: () => {
      notifications.show({ color: 'teal', message: 'Undone. Everyone has the access they had before.' })
      setUndoing(null); setWhy('')
      void queryClient.invalidateQueries({ queryKey: ['grant-change'] })
      void queryClient.invalidateQueries({ queryKey: qk.grants() })
      void queryClient.invalidateQueries({ queryKey: ['identities'] })
    },
    onError: (e) => notifyRejected(e),
  })
  if (!changes?.length) return null
  const roleName = (id?: string) => roles?.find((r) => r.id === id)?.name ?? 'a role'
  return (
    <section className="panel flex flex-col gap-2 p-4">
      <h2 className="t-h3">Recent bulk changes</h2>
      {changes.map((c) => (
        <div key={c.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 border-b py-2" style={{ borderColor: 'var(--line-soft)' }}>
          <span className="min-w-0 flex-1">
            <span className="t-body block">
              {KINDS[c.kind]?.label ?? c.kind} on <b>{roleName(c.spec.role_id)}</b>, {c.grants} {c.grants === 1 ? 'grant' : 'grants'}
            </span>
            <span className="t-xs block">
              {relDays(c.created_at)}{c.reason && ` · ${c.reason}`}{c.undone_at && ` · undone ${relDays(c.undone_at)}`}
            </span>
          </span>
          {!c.undone_at && (undoing?.id === c.id ? (
            <span className="flex w-full items-end gap-2 sm:w-auto">
              <TextInput size="xs" placeholder="Why (optional)" value={why} onChange={(e) => setWhy(e.currentTarget.value)} />
              <Button size="xs" color="red" loading={undo.isPending} onClick={() => undo.mutate(c)}>Undo</Button>
              <Button size="xs" variant="subtle" color="gray" onClick={() => setUndoing(null)}>Keep</Button>
            </span>
          ) : (
            <Button size="xs" variant="default" leftSection={<IconArrowBackUp size={13} />} onClick={() => setUndoing(c)}>Undo</Button>
          ))}
        </div>
      ))}
    </section>
  )
}
