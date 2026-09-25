import { createFileRoute } from '@tanstack/react-router'
import { useQuery, useMutation } from '@tanstack/react-query'
import { Badge, Button, Modal, MultiSelect, Popover, SegmentedControl, Select, TextInput, Tooltip } from '@mantine/core'
import { IconPlus, IconFlask, IconAlertTriangle, IconSitemapFilled, IconRefresh, IconPlugConnected, IconClock } from '@tabler/icons-react'
import { queryClient } from '@/lib/query/client'
import { notifyCreated, notifyRejected } from '@/components/create/shell'
import { api } from '@/lib/api/client'
import { useCreate } from '@/stores/create'
import { useState } from 'react'
import { Page } from '@/components/shell/Page'
import { ScopeTree } from '@/components/scope/ScopeTree'
import { SYNC_INTERVALS } from '@/lib/syncIntervals'
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

/* Seconds as an operator said them. The drawer offers a fixed set, so this
   only has to name those; anything else falls back to minutes rather than
   pretending not to know. */
function everyLabel(secs: number): string {
  if (secs % 86400 === 0) return secs === 86400 ? 'day' : `${secs / 86400} days`
  if (secs % 3600 === 0) return secs === 3600 ? 'hour' : `${secs / 3600} hours`
  return `${Math.round(secs / 60)} minutes`
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
    <div className="p-4">
      <div className="mb-1 flex items-baseline justify-between">
        <div className="t-label">Item kinds</div>
        <div className="t-xs">what may sit under what</div>
      </div>
      <div className="t-xs mb-3">
        Levels are rules, not code — add “Division” between offices and departments and
        every picker follows. The database rejects illegal placements outright.
      </div>
      <div className="flex flex-col gap-1.5">
        {/* Name over parents, not beside them. Side by side the select was
            pinned at 220px and the level's own name was what gave way —
            "Department" and "Departments" both rendered as "Depart…", which
            is the one thing this panel exists to tell apart. */}
        {mine.map((t) => (
          <div key={t.code} className="panel-inset px-2.5 py-2">
            <div className="flex items-center justify-between gap-2">
              <div className="flex min-w-0 items-baseline gap-2">
                <span className="t-body truncate" style={{ fontWeight: 530 }}>{t.display_name}</span>
                <span className="chip">{t.code}</span>
              </div>
              {t.parent_types.length === 0 && (
                <Tooltip label="The root of this structure — nothing sits above it.">
                  <span className="chip chip-accent">root</span>
                </Tooltip>
              )}
            </div>
            {t.parent_types.length > 0 && (
              <MultiSelect
                size="xs" mt={6} value={t.parent_types}
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
  const [busy, setBusy] = useState<'plan' | 'apply' | 'schedule' | null>(null)

  const refresh = () => Promise.all([
    queryClient.invalidateQueries({ queryKey: qk.scope() }),
    queryClient.invalidateQueries({ queryKey: qk.syncSources() }),
    queryClient.invalidateQueries({ queryKey: qk.syncRuns(source?.id ?? '') }),
  ])
  const doSchedule = async (secs: number) => {
    if (!source || secs === source.interval_seconds) return
    setBusy('schedule')
    try {
      await api.setSyncSchedule(source.id, secs)
      notifyCreated(secs > 0 ? 'Refresh scheduled' : 'Refresh turned off',
        secs > 0
          ? `Anubis re-reads this structure every ${everyLabel(secs)}.`
          : 'This structure now only syncs when somebody asks.')
      await queryClient.invalidateQueries({ queryKey: qk.syncSources() })
    } catch (e) { notifyRejected(e) }
    setBusy(null)
  }
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
      <div className="p-4">
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
    <div className="p-4">
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
      <div className="t-xs mb-1">
        {source.last_run_at
          ? `Last synced ${source.last_run_at.slice(0, 16).replace('T', ' ')}`
          : 'Never synced — preview first.'}
      </div>
      {/* Whether a clock owns this feed, and the control to change it. A source
          that refreshes itself and one that waits for a button look identical
          otherwise, and "why is this tree stale?" has exactly two answers.
          Editable here because a schedule set only at connect time would mean
          every source that already exists is stuck manual forever. */}
      <div className="mb-3 flex items-center gap-2">
        <IconClock size={13}
          style={{ color: source.interval_seconds > 0 ? 'var(--allow)' : 'var(--ink-4)', flexShrink: 0 }} />
        <Select
          size="xs" aria-label="Refresh interval" className="flex-1"
          data={SYNC_INTERVALS}
          value={String(source.interval_seconds)}
          disabled={busy === 'schedule'}
          onChange={(v) => void doSchedule(Number(v ?? 0))}
        />
      </div>
      <div className="t-xs mb-3">
        {source.interval_seconds > 0
          ? `Next run ${source.next_run_at
              ? source.next_run_at.slice(0, 16).replace('T', ' ')
              : 'shortly'} · a failed run waits for the next one.`
          : 'Nothing refreshes this on its own.'}
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
            <div key={r.id} className="t-xs flex items-start gap-1.5">
              <span className="tnum" style={{ flexShrink: 0 }}>{r.at.slice(5, 16).replace('T', ' ')}</span>
              {/* A dry run and a real one leave the same counts behind, so
                  the panel has to say which it was. */}
              {r.dry && <span className="chip">dry</span>}
              {/* Two different failures. A run that reconciled and could not
                  place N rows is not the same event as one that never reached
                  the feed at all, and "0 unplaced" was what the second used to
                  say. */}
              {r.error
                ? <span style={{ color: 'var(--deny)' }}>{r.error}</span>
                : <>
                    {r.status === 'failed' && (
                      <span className="chip" style={{ color: 'var(--deny)', borderColor: 'var(--deny)' }}>
                        {r.errors} unplaced
                      </span>
                    )}
                    {r.status === 'running' && <span className="chip">running</span>}
                    <span className="tnum">+{r.added} ~{r.renamed} →{r.moved} −{r.archived} · {r.unchanged} unchanged</span>
                  </>}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

type Pane = 'settings' | 'levels' | 'source'

function Scope() {
  const { openCreate } = useCreate()
  const [selected, setSelected] = useState<ScopeNode | null>(null)
  const [pane, setPane] = useState<Pane>('settings')
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
        {/* Axis switcher. Axes are genuinely independent forests, so this
            reads as a set of peers — but it is still navigation, and as a
            wrapping grid of cards it took five rows and ~290px before the
            tree it selects. The code moved into Settings, where it is read
            once, rather than riding every tab. */}
        <div className="axis-rail-wrap">
        <div className="axis-rail" role="tablist" aria-label="Structure">
          {axes?.map((a) => {
            const on = a.code === active
            return (
              <button key={a.code} role="tab" aria-selected={on}
                onClick={() => { setTab(a.code); setSelected(null) }}
                className="axis-tab" {...(on ? { 'data-active': '' } : {})}>
                <span className="axis-tab-icon">
                  <AxisIcon name={a.ui_schema.icon} size={15} />
                </span>
                {a.display_name}
                {a.default_effect === 'deny' && (
                  <Tooltip label="Strict: a grant that does not name this structure is denied.">
                    <span className="axis-tab-strict" />
                  </Tooltip>
                )}
              </button>
            )
          })}
        </div>
        </div>

        {axis && (
          /* Tree left and wide, detail rail right and fixed. The split used to
             be 50/50, which gave half the screen to settings read once a
             quarter and half to the tree that is the actual work — and because
             a grid stretches its columns to equal height, the tree panel grew
             to 860px to match the settings stack and rendered one row of
             content inside an empty white rectangle. items-start stops that.
             Below lg the sidebar leaves no room for a 400px rail beside a
             usable tree — at phone width the tree was squeezed to 2px — so
             the rail goes underneath. */
          <div className="grid grid-cols-1 items-start gap-4 lg:grid-cols-[minmax(0,1fr)_400px]">
            <div className="panel overflow-clip">
              <div className="panel-head">
                <span className="t-label">{axis.display_name} tree</span>
                <span className="t-xs">children load on expand</span>
              </div>
              {/* A minimum as well as a maximum. Autosize shrinks to content,
                  so an axis holding one root collapsed the primary surface to
                  a single row beside a 400px rail. */}
              <div className="p-3" style={{ minHeight: 380 }}>
                <ScopeTree axis={axis.code} selectedId={selected?.id ?? null}
                  onSelect={setSelected} height="calc(100vh - 272px)" />
              </div>
            </div>

            {/* Sticky, and the selection comes FIRST. It used to be the fourth
                panel in the stack: you clicked a node at the top of the tree
                and the answer appeared ~900px away, below the fold, under
                three settings cards. A detail pane that is not on screen when
                the thing it describes is clicked is not a detail pane. */}
            <div className="sticky top-0 flex flex-col gap-4">
              {selected ? (
                <div className="panel rise p-4">
                  <div className="t-label mb-2.5">Selected item</div>
                  <div className="t-h1">{selected.name}</div>
                  <div className="mt-2 flex flex-wrap items-center gap-1.5">
                    <span className="chip">{selected.node_type}</span>
                    {selected.is_axis_root && (
                      <span className="chip chip-accent">axis root</span>
                    )}
                    <span className="chip">{selected.child_count ?? 0} children</span>
                  </div>
                  <div className="mt-2.5">
                    <span className="chip" style={{ maxWidth: '100%' }}>
                      <span className="truncate">{selected.id}</span>
                    </span>
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
                      <Button size="xs" variant="light" mt={12} fullWidth
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
                <div className="panel flex flex-col items-center justify-center px-4 py-8 text-center">
                  <div className="mb-2.5 flex items-center justify-center rounded-full"
                    style={{ width: 34, height: 34, background: 'var(--s-sunken)', border: '1px solid var(--line)' }}>
                    <AxisIcon name={axis.ui_schema.icon} size={15} />
                  </div>
                  <div className="t-sm">Select an item in the tree to inspect it</div>
                </div>
              )}

              {/* Configuration, levels and source were three stacked cards —
                  three borders and ~700px for settings nobody opens twice in a
                  day. One panel, three panes. */}
              <div className="panel overflow-clip">
                <div style={{ padding: 8, borderBottom: '1px solid var(--line-soft)' }}>
                  <SegmentedControl
                    size="xs" fullWidth value={pane}
                    onChange={(v) => setPane(v as Pane)}
                    data={[
                      { value: 'settings', label: 'Settings' },
                      { value: 'levels', label: 'Levels' },
                      { value: 'source', label: 'Source' },
                    ]}
                  />
                </div>

                {pane === 'settings' && (
                  <div className="p-4">
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

                    <div className="mt-3.5" style={{ borderTop: '1px solid var(--line-soft)', paddingTop: 12 }}>
                      <div className="t-xs mb-2">
                        Preview what would break if every access rule had to name this structure explicitly.
                      </div>
                      <Button size="xs" variant="default" fullWidth leftSection={<IconFlask size={13} />}
                        loading={dryRun.isPending} onClick={() => dryRun.mutate(axis.code)}>
                        Strict dry run
                      </Button>
                    </div>
                    {dryRun.data?.axis_code === axis.code && <DryRun r={dryRun.data} />}
                  </div>
                )}

                {pane === 'levels' && <ItemKinds axisCode={axis.code} />}
                {pane === 'source' && <SyncCard axisCode={axis.code} />}
              </div>
            </div>
          </div>
        )}
      </div>
    </Page>
  )
}
