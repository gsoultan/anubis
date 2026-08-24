import { createFileRoute } from '@tanstack/react-router'
import { useQuery, useMutation } from '@tanstack/react-query'
import { Badge, Button, Modal, MultiSelect, Popover, TextInput, Tooltip } from '@mantine/core'
import { IconPlus, IconFlask, IconAlertTriangle, IconSitemapFilled, IconRefresh, IconPlugConnected } from '@tabler/icons-react'
import { queryClient } from '@/lib/query/client'
import { notifyCreated, notifyRejected } from '@/components/create/shell'
import { api } from '@/lib/api/client'
import { useCreate } from '@/stores/create'
import { useState } from 'react'
import { Page } from '@/components/shell/Page'
import { ScopeTree } from '@/components/scope/ScopeTree'
import { AxisIcon } from '@/components/scope/AxisIcon'
import { qk } from '@/lib/query/keys'
import type { ScopeNode, StrictDryRun } from '@/lib/api/types'

export const Route = createFileRoute('/scope')({ component: Scope })

function DryRun({ r }: { r: StrictDryRun }) {
  /* The server replays REAL recent decisions with the axis forced strict and
     counts how many flip to deny. That is the whole honest answer: no
     before/after totals are narrated, so none are shown. */
  const severe = r.sampled > 0 && r.would_deny > r.sampled * 0.2
  return (
    <div className="panel rise mt-3 overflow-hidden"
      style={{ borderColor: severe ? 'color-mix(in srgb, var(--deny) 24%, transparent)' : 'color-mix(in srgb, var(--warn) 24%, transparent)' }}>
      <div className="flex items-start gap-2.5 px-3.5 py-3"
        style={{ background: severe ? 'var(--deny-bg)' : 'var(--warn-bg)' }}>
        <IconAlertTriangle size={15}
          style={{ color: severe ? 'var(--deny)' : 'var(--warn)', marginTop: 1, flexShrink: 0 }} />
        <div className="t-body">
          Flipping <span className="chip">{r.axis_code}</span> to strict would deny{' '}
          <b className="tnum">{r.would_deny.toLocaleString()}</b> of{' '}
          {r.sampled.toLocaleString()} recently sampled decisions.
        </div>
      </div>
      {r.would_deny > 0 && r.examples.length > 0 && (
        <div className="px-3.5 py-2.5">
          <div className="t-label mb-1">examples that would break</div>
          <pre className="t-xs" style={{ margin: 0, maxHeight: 160, overflow: 'auto' }}>
            {JSON.stringify(r.examples.slice(0, 5), null, 1)}
          </pre>
        </div>
      )}
    </div>
  )
}

/* The level rules, visible and editable. "Departments can sit under offices
   or divisions" is data — this panel is where an operator adds a Division
   level between existing ones, without a deploy. The same rules drive the
   contextual add-button and are enforced by a schema trigger (0014). */
function ItemKinds({ axisCode }: { axisCode: string }) {
  const { data: types } = useQuery({ queryKey: qk.nodeTypes(), queryFn: api.nodeTypes })
  const mine = (types ?? []).filter((t) => t.axis_code === axisCode)
  const [name, setName] = useState('')
  const [parents, setParents] = useState<string[]>([])
  const [busy, setBusy] = useState(false)

  const save = async (code: string, next: string[]) => {
    try {
      await api.setNodeTypeParents(axisCode, code, next)
      await queryClient.invalidateQueries({ queryKey: qk.nodeTypes() })
      notifyCreated('Level rules updated', `“${code}” placement changed — pickers follow immediately.`)
    } catch (e) { notifyRejected(e) }
  }
  const add = async () => {
    setBusy(true)
    try {
      await api.createNodeType({ axis_code: axisCode, display_name: name, parent_types: parents })
      notifyCreated('Kind added', `“${name}” can now be created under: ${parents.join(', ')}.`)
      await queryClient.invalidateQueries({ queryKey: qk.nodeTypes() })
      setName(''); setParents([])
    } catch (e) { notifyRejected(e) }
    setBusy(false)
  }

  return (
    <div className="panel p-4">
      <div className="mb-1 flex items-baseline justify-between">
        <div className="t-label">Item kinds</div>
        <div className="t-xs">what may sit under what</div>
      </div>
      <div className="t-xs mb-3">
        Levels are rules, not code — add “Division” between offices and departments and
        every picker follows. The database rejects illegal placements outright.
      </div>
      <div className="flex flex-col gap-1.5">
        {mine.map((t) => (
          <div key={t.code} className="panel-inset flex items-center justify-between gap-2 px-2.5 py-1.5">
            <div className="flex min-w-0 items-center gap-2">
              <span className="t-body truncate" style={{ fontWeight: 530 }}>{t.display_name}</span>
              <span className="chip">{t.code}</span>
            </div>
            {t.parent_types.length === 0 ? (
              <Tooltip label="The root of this structure — nothing sits above it.">
                <span className="chip chip-gold">root</span>
              </Tooltip>
            ) : (
              <MultiSelect
                size="xs" w={220} value={t.parent_types}
                aria-label={`Legal parents of ${t.display_name}`}
                data={mine.filter((x) => x.code !== t.code).map((x) => ({ value: x.code, label: x.display_name }))}
                onChange={(v) => void save(t.code, v)}
                comboboxProps={{ withinPortal: true }}
              />
            )}
          </div>
        ))}
      </div>
      <Popover width={300} position="bottom-start">
        <Popover.Target>
          <Button size="xs" variant="light" mt={10} leftSection={<IconPlus size={13} />}>
            Add kind
          </Button>
        </Popover.Target>
        <Popover.Dropdown p="sm">
          <div className="flex flex-col gap-2.5">
            <TextInput size="xs" label="Name" placeholder="Division"
              value={name} onChange={(e) => setName(e.currentTarget.value)} />
            <MultiSelect size="xs" label="May sit under" placeholder="Pick parents"
              data={mine.map((x) => ({ value: x.code, label: x.display_name }))}
              value={parents} onChange={setParents} />
            <Button size="xs" loading={busy} disabled={name.trim().length < 2 || parents.length === 0}
              onClick={() => void add()}>
              Add kind
            </Button>
          </div>
        </Popover.Dropdown>
      </Popover>
    </div>
  )
}

/* Where the tree meets its source of truth. Preview is the default action —
   the same discipline as the strict-mode dry run: see the diff, then apply. */
function SyncCard({ axisCode }: { axisCode: string }) {
  const { openCreate } = useCreate()
  const { data: sources } = useQuery({ queryKey: qk.syncSources(), queryFn: api.syncSources })
  const source = sources?.find((s) => s.axis_code === axisCode)
  const { data: runs } = useQuery({
    queryKey: qk.syncRuns(source?.id ?? ''),
    queryFn: () => api.syncRuns(source!.id),
    enabled: !!source,
  })
  const [preview, setPreview] = useState<import('@/lib/api/types').SyncPlan | null>(null)
  const [busy, setBusy] = useState<'plan' | 'apply' | null>(null)

  const refresh = () => Promise.all([
    queryClient.invalidateQueries({ queryKey: qk.scope() }),
    queryClient.invalidateQueries({ queryKey: qk.syncSources() }),
    queryClient.invalidateQueries({ queryKey: qk.syncRuns(source?.id ?? '') }),
  ])
  const doPlan = async () => {
    if (!source) return
    setBusy('plan')
    try { setPreview(await api.syncPlan(source.id)) } catch (e) { notifyRejected(e) }
    setBusy(null)
  }
  const doApply = async () => {
    if (!source) return
    setBusy('apply')
    try {
      const run = await api.syncApply(source.id)
      notifyCreated('Sync applied',
        `+${run.added} added · ${run.renamed} renamed · ${run.archived} archived · ${run.unchanged} unchanged.`)
      setPreview(null); await refresh()
    } catch (e) { notifyRejected(e) }
    setBusy(null)
  }

  if (!source) {
    return (
      <div className="panel p-4" style={{ borderStyle: 'dashed' }}>
        <div className="t-label mb-1">Sync</div>
        <div className="t-xs mb-2.5">
          This structure is maintained by hand. Connect the system that owns it — matched by
          reference, vanished rows archived (never deleted), manual items untouched.
        </div>
        <Button size="xs" variant="light" leftSection={<IconPlugConnected size={13} />}
          onClick={() => openCreate('syncSource', { axisCode })}>
          Connect source
        </Button>
      </div>
    )
  }

  return (
    <div className="panel p-4">
      <Modal opened={!!preview} onClose={() => setPreview(null)} title="Preview — nothing applied yet" size={440}>
        {preview && (
          <div className="flex flex-col gap-3">
            {preview.added + preview.renamed + preview.moved + preview.archived === 0 ? (
              <div className="t-sm">In sync — the source and the tree already agree.</div>
            ) : (
              <div className="grid grid-cols-2 gap-2">
                {([
                  ['would add', preview.added, 'var(--allow)'],
                  ['would rename', preview.renamed, 'var(--warn)'],
                  ['would move', preview.moved, 'var(--warn)'],
                  ['would archive', preview.archived, 'var(--deny)'],
                ] as const).map(([label, n, colour]) => (
                  <div key={label} className="panel-inset px-3 py-2">
                    <div className="t-h1 tnum" style={{ color: n > 0 ? colour : 'var(--ink-3)' }}>{n}</div>
                    <div className="t-xs">{label}</div>
                  </div>
                ))}
              </div>
            )}
            <div className="t-xs">{preview.unchanged} unchanged.</div>
            {preview.errors.length > 0 && (
              <div>
                <div className="t-label mb-1">{preview.errors.length} row{preview.errors.length === 1 ? '' : 's'} the feed could not place</div>
                {preview.errors.slice(0, 6).map((e) => (
                  <div key={e.ref} className="t-xs"><span className="chip">{e.ref}</span> {e.error}</div>
                ))}
              </div>
            )}
          </div>
        )}
      </Modal>

      <div className="mb-1 flex items-center justify-between">
        <div className="t-label">Sync</div>
        <Badge size="xs" variant="light" color="slate">{source.kind}</Badge>
      </div>
      <div className="chip mb-2 w-fit" style={{ maxWidth: '100%' }}>
        <span className="truncate">{source.target}</span>
      </div>
      <div className="t-xs mb-3">
        {source.last_run_at
          ? `Last synced ${source.last_run_at.slice(0, 16).replace('T', ' ')}`
          : 'Never synced — preview first.'}
      </div>
      <div className="flex items-center gap-2">
        <Button size="xs" variant="default" loading={busy === 'plan'} onClick={() => void doPlan()}>
          Preview changes
        </Button>
        <Button size="xs" variant="light" leftSection={<IconRefresh size={13} />}
          loading={busy === 'apply'} onClick={() => void doApply()}>
          Sync now
        </Button>
      </div>
      {(runs?.length ?? 0) > 0 && (
        <div className="mt-3 flex flex-col gap-1" style={{ borderTop: '1px solid var(--line-soft)', paddingTop: 10 }}>
          {runs!.map((r) => (
            <div key={r.id} className="t-xs tnum">
              {r.at.slice(5, 16).replace('T', ' ')} · +{r.added} ~{r.renamed} −{r.archived} · {r.unchanged} unchanged
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

function Scope() {
  const { openCreate } = useCreate()
  const [selected, setSelected] = useState<ScopeNode | null>(null)
  const { data: axes } = useQuery({ queryKey: qk.axes(), queryFn: api.axes })
  const { data: nodeTypes } = useQuery({ queryKey: qk.nodeTypes(), queryFn: api.nodeTypes })
  // ?axis= makes a structure deep-linkable (docs, and the screenshot harness)
  const [tab, setTab] = useState<string | null>(
    () => new URLSearchParams(window.location.search).get('axis'))
  const active = tab ?? axes?.[0]?.code ?? null
  const axis = axes?.find((a) => a.code === active)
  const dryRun = useMutation({ mutationFn: (a: string) => api.strictDryRun(a) })

  return (
    <Page
      title="Structure"
      description="The places and things access can be limited to — offices, product lines, customers. Each structure is its own tree, and new kinds are added here, never deployed."
      wide
      actions={
        <>
          <Button size="xs" variant="default" leftSection={<IconSitemapFilled size={13} />}
            onClick={() => openCreate('node', active ? { axisCode: active } : undefined)}>
            Add item
          </Button>
          <Button size="xs" leftSection={<IconPlus size={13} />}
            onClick={() => openCreate('axis')}>
            Add structure
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        {/* Axis switcher. Reads as a set of peers rather than browser tabs,
            which matters because axes are genuinely independent forests. */}
        <div className="flex flex-wrap gap-2">
          {axes?.map((a) => {
            const on = a.code === active
            return (
              <button key={a.code} onClick={() => { setTab(a.code); setSelected(null) }}
                className="panel flex items-center gap-2.5 px-3 py-2 text-left"
                style={{
                  borderColor: on ? 'color-mix(in srgb, var(--gold) 30%, transparent)' : 'var(--line)',
                  background: on ? 'linear-gradient(180deg, var(--gold-glow), var(--s-raised) 70%)' : 'var(--s-raised)',
                  transition: 'all var(--t-fast)',
                }}>
                <span style={{ color: on ? 'var(--gold)' : 'var(--ink-3)', display: 'flex' }}>
                  <AxisIcon name={a.ui_schema.icon} size={15} />
                </span>
                <span>
                  <span className="t-body block" style={{ fontWeight: on ? 570 : 460 }}>{a.display_name}</span>
                  <span className="t-xs block font-mono" style={{ fontSize: 10 }}>{a.code}</span>
                </span>
                {a.default_effect === 'deny' && (
                  <span className="chip" style={{ color: 'var(--deny)', borderColor: 'color-mix(in srgb, var(--deny) 20%, transparent)' }}>strict</span>
                )}
              </button>
            )
          })}
        </div>

        {axis && (
          <div className="grid gap-4" style={{ gridTemplateColumns: 'minmax(0,1fr) minmax(0,1fr)' }}>
            <div className="panel overflow-hidden">
              <div className="flex items-baseline justify-between px-4 py-3"
                style={{ borderBottom: '1px solid var(--line-soft)' }}>
                <span className="t-label">{axis.display_name} tree</span>
                <span className="t-xs">children load on expand</span>
              </div>
              <div className="p-3">
                <ScopeTree axis={axis.code} selectedId={selected?.id ?? null}
                  onSelect={setSelected} height={430} />
              </div>
            </div>

            <div className="flex flex-col gap-4">
              <div className="panel p-4">
                <div className="t-label mb-3">Configuration</div>
                <div className="flex flex-col gap-2">
                  {[
                    ['code', <span key="c" className="chip">{axis.code}</span>],
                    ['default effect', <span key="d" className="chip" style={{
                      color: axis.default_effect === 'deny' ? 'var(--deny)' : 'var(--ink-2)',
                    }}>{axis.default_effect}</span>],
                    ['resolution', <span key="r" className="chip">{axis.resolution.from === 'token'
                      ? 'token' : `context.${axis.resolution.key}`}</span>],
                    ['picker', <span key="p" className="chip">{axis.ui_schema.picker}</span>],
                  ].map(([label, node]) => (
                    <div key={String(label)} className="flex items-center justify-between gap-3">
                      <span className="t-xs">{label as string}</span>
                      {node as React.ReactNode}
                    </div>
                  ))}
                </div>

                {axis.ui_schema.help && (
                  <div className="t-xs mt-3" style={{ borderTop: '1px solid var(--line-soft)', paddingTop: 10 }}>
                    {axis.ui_schema.help}
                  </div>
                )}

                <div className="mt-3.5 flex items-center justify-between gap-3"
                  style={{ borderTop: '1px solid var(--line-soft)', paddingTop: 12 }}>
                  <span className="t-xs" style={{ maxWidth: 230 }}>
                    Preview what would break if every access rule had to name this structure explicitly.
                  </span>
                  <Button size="xs" variant="default" leftSection={<IconFlask size={13} />}
                    loading={dryRun.isPending} onClick={() => dryRun.mutate(axis.code)}>
                    Strict dry run
                  </Button>
                </div>
                {dryRun.data?.axis_code === axis.code && <DryRun r={dryRun.data} />}
              </div>

              <SyncCard axisCode={axis.code} />
              <ItemKinds axisCode={axis.code} />

              {selected ? (
                <div className="panel rise p-4">
                  <div className="t-label mb-2.5">Selected item</div>
                  <div className="t-h1">{selected.name}</div>
                  <div className="mt-2 flex flex-wrap items-center gap-1.5">
                    <span className="chip">{selected.node_type}</span>
                    {selected.is_axis_root && (
                      <span className="chip chip-gold">axis root</span>
                    )}
                    <span className="chip">{selected.child_count ?? 0} children</span>
                  </div>
                  <div className="mt-2.5">
                    <span className="chip">{selected.id}</span>
                  </div>
                  {(() => {
                    /* Contextual verb: the schema knows what may live under
                       this node, so the button should say it. "Add department"
                       teaches the hierarchy; "Add child node" teaches nothing. */
                    const legal = (nodeTypes ?? []).filter((t) =>
                      t.axis_code === axis.code && t.parent_types.includes(selected.node_type))
                    if (legal.length === 0) return (
                      <div className="t-xs mt-3">Nothing can be added under a {selected.node_type}.</div>
                    )
                    const label = legal.length === 1
                      ? `Add ${legal[0]!.display_name.toLowerCase()}`
                      : 'Add item'
                    return (
                      <Button size="xs" variant="light" mt={12}
                        leftSection={<IconPlus size={13} />}
                        onClick={() => openCreate('node', { axisCode: axis.code, parentId: selected.id })}>
                        {label} under “{selected.name}”
                      </Button>
                    )
                  })()}
                  {selected.is_axis_root && (
                    <div className="t-xs mt-3" style={{ borderTop: '1px solid var(--line-soft)', paddingTop: 10 }}>
                      Granting here means <b style={{ color: 'var(--ink-2)' }}>deliberately unrestricted</b> on
                      this axis — distinct from a grant that is merely silent about it, which matters the
                      moment the axis is flipped to strict.
                    </div>
                  )}
                </div>
              ) : (
                <div className="panel flex flex-col items-center justify-center px-4 py-10 text-center">
                  <Tooltip label="Select any node in the tree">
                    <div className="mb-2.5 flex items-center justify-center rounded-full"
                      style={{ width: 34, height: 34, background: 'var(--s-sunken)', border: '1px solid var(--line)' }}>
                      <AxisIcon name={axis.ui_schema.icon} size={15} />
                    </div>
                  </Tooltip>
                  <div className="t-sm">Select an item to inspect it</div>
                </div>
              )}
            </div>
          </div>
        )}
      </div>
    </Page>
  )
}
