import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { CloseButton, Menu, Select, TextInput, UnstyledButton } from '@mantine/core'
import {
  IconCheck, IconChevronDown, IconCircleCheckFilled, IconCircleMinus, IconInfoCircle, IconSearch,
} from '@tabler/icons-react'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { AxisIcon } from '@/components/scope/AxisIcon'
import { ScopeTree, type PickMark } from '@/components/scope/ScopeTree'
import { axisNeedsAnInclude, namesOf, type NodeSel, type Places } from '@/lib/access'
import type { ScopeAxis, ScopeNode } from '@/lib/api/types'

/* Choosing where access applies.
 *
 * The form this replaces listed every structure as its own row with an
 * "Anywhere" button — forty-seven of them in this installation — and each
 * place, once picked, grew a segmented control and a switch. An operator had
 * to read the whole list to find the one structure they meant, and then
 * decode "Include / Exclude" and "and inside" into what the grant would do.
 *
 * Now it reads like a file dialog: structures on the left, that structure's
 * tree on the right, and what you have chosen underneath, in words. The
 * structure list is navigation, the tree is the choice, and the chosen list
 * is the only place a choice is refined — which is the order the question is
 * actually asked in.
 *
 * Laid out by container, not viewport: the same picker sits in a 640px sheet
 * (two panes) and a 460px drawer or a phone (one column). */

const byMode = (a: NodeSel, b: NodeSel) =>
  Number(a.exclude) - Number(b.exclude) || a.name.localeCompare(b.name)

type Mode = { exclude: boolean; inherit: boolean }
const MODES: { mode: Mode; label: string; hint: string }[] = [
  { mode: { exclude: false, inherit: true }, label: 'Include, with everything inside',
    hint: 'This place and every place within it.' },
  { mode: { exclude: false, inherit: false }, label: 'Include this place only',
    hint: 'Not the places within it.' },
  { mode: { exclude: true, inherit: true }, label: 'Except, with everything inside',
    hint: 'Carve this place and all within it out of this grant.' },
  { mode: { exclude: true, inherit: false }, label: 'Except this place only',
    hint: 'Carve out just this place; what is within it stays included.' },
]

function modeLabel(v: NodeSel): string {
  if (!v.exclude) return v.inherit ? 'Everything inside' : 'This place only'
  return v.inherit ? 'Except, and inside' : 'Except this place'
}

function ChosenPlace({ v, onChange, onRemove }: {
  v: NodeSel
  onChange: (m: Mode) => void
  onRemove: () => void
}) {
  /* Where it sits, so two places both called "Finance" can be told apart.
     One small lookup per CHOSEN place — a handful — never per tree row. */
  const { data: path } = useQuery({
    queryKey: qk.ancestorPath(v.id), queryFn: () => api.ancestorPath(v.id),
  })
  const trail = (path ?? []).filter((p) => p.id !== v.id && !p.is_axis_root).map((p) => p.name)

  return (
    <div className="chosen-row" data-exclude={v.exclude ? '' : undefined}>
      {v.exclude
        ? <IconCircleMinus size={16} style={{ color: 'var(--deny)', flexShrink: 0 }} aria-hidden />
        : <IconCircleCheckFilled size={16} style={{ color: 'var(--accent)', flexShrink: 0 }} aria-hidden />}
      <div className="min-w-0 flex-1">
        <div className="t-body truncate" style={{ fontWeight: 540, color: v.exclude ? 'var(--deny)' : undefined }}>
          {v.exclude && <span className="t-xs" style={{ color: 'var(--deny)', marginRight: 5 }}>except</span>}
          {v.name}
        </div>
        {trail.length > 0 && <div className="t-xs truncate">{trail.join(' › ')}</div>}
      </div>
      <Menu position="bottom-end" width={280}>
        <Menu.Target>
          <UnstyledButton className="mode-btn" aria-label={`How ${v.name} applies: ${modeLabel(v)}`}>
            <span className="truncate">{modeLabel(v)}</span>
            <IconChevronDown size={12} style={{ flexShrink: 0 }} />
          </UnstyledButton>
        </Menu.Target>
        <Menu.Dropdown>
          {MODES.map((m, i) => {
            const on = m.mode.exclude === v.exclude && m.mode.inherit === v.inherit
            return (
              <div key={m.label}>
                {i === 2 && <Menu.Divider />}
                <Menu.Item onClick={() => onChange(m.mode)}
                  {...(m.mode.exclude ? { color: 'deny' } : {})}
                  rightSection={on ? <IconCheck size={14} /> : null}>
                  <span className="t-body block" style={{ fontWeight: on ? 600 : 500 }}>{m.label}</span>
                  <span className="t-xs block">{m.hint}</span>
                </Menu.Item>
              </div>
            )
          })}
        </Menu.Dropdown>
      </Menu>
      <CloseButton size="sm" onClick={onRemove} aria-label={`Remove ${v.name}`} />
    </div>
  )
}

export function PlacePicker({ axes, value, onChange, required }: {
  axes: ScopeAxis[]
  value: Places
  onChange: (next: Places) => void
  /** Structures every grant must name — marked, and opened first. */
  required: ScopeAxis[]
}) {
  const [active, setActive] = useState<string | null>(null)
  const [filter, setFilter] = useState('')
  const firstChosen = axes.find((a) => (value[a.code]?.length ?? 0) > 0)?.code
  const current = active ?? firstChosen ?? required[0]?.code ?? axes[0]?.code ?? ''
  const axis = axes.find((a) => a.code === current)
  const list = value[current] ?? []
  const picked = new Map<string, PickMark>(list.map((v) => [v.id, v.exclude ? 'exclude' : 'include']))
  const isRequired = (code: string) => required.some((r) => r.code === code)
  /* Icons only when the installation has given its structures some. The
     fallback glyph on forty-eight rows is forty-eight copies of "no icon". */
  const withIcons = axes.some((a) => !!a.ui_schema.icon)

  const setAxis = (code: string, next: NodeSel[]) => {
    const out = { ...value }
    if (next.length === 0) delete out[code]
    else out[code] = [...next].sort(byMode)
    onChange(out)
  }
  /* A place arrives included, with everything inside it. Nobody opens a tree
     meaning to carve something out of nothing, and the one shape the database
     refuses is a structure that holds only exceptions. */
  const toggle = (n: ScopeNode) =>
    setAxis(current, list.some((v) => v.id === n.id)
      ? list.filter((v) => v.id !== n.id)
      : [...list, { id: n.id, name: n.name, inherit: true, exclude: false }])

  const needle = filter.trim().toLowerCase()
  const shownAxes = needle
    ? axes.filter((a) => a.display_name.toLowerCase().includes(needle) || a.code.includes(needle))
    : axes
  const chosenAxes = axes.filter((a) => (value[a.code]?.length ?? 0) > 0)
  const withIncludes = chosenAxes.filter((a) => value[a.code]!.some((v) => !v.exclude))
  const anyExcept = chosenAxes.some((a) => value[a.code]!.some((v) => v.exclude))
  /* Only the optional ones are unlimited when left alone. A required
     structure left alone makes the grant do nothing, and the sheet says that
     next to its button instead. */
  const silent = axes.filter((a) => !(value[a.code]?.length) && !isRequired(a.code)).length

  return (
    <div className="@container flex flex-col gap-3">
      <div className="picker">
        <div className="grid grid-cols-1 @xl:grid-cols-[212px_minmax(0,1fr)]">
          {/* Structures. A list from 576px of room up, a select below it. */}
          <div className="picker-axes hidden @xl:flex">
            {axes.length > 8 && (
              <div className="p-2" style={{ borderBottom: '1px solid var(--line-soft)' }}>
                <TextInput size="xs" placeholder={`Find among ${axes.length} structures`}
                  leftSection={<IconSearch size={13} />} value={filter}
                  onChange={(e) => setFilter(e.currentTarget.value)} />
              </div>
            )}
            <div className="flex flex-col gap-px overflow-y-auto p-1.5" style={{ maxHeight: 322 }}
              role="group" aria-label="Structures">
              {shownAxes.map((a) => {
                const n = value[a.code]?.length ?? 0
                return (
                  <button key={a.code} type="button" aria-pressed={a.code === current}
                    className="picker-axis" data-active={a.code === current ? '' : undefined}
                    onClick={() => setActive(a.code)}>
                    {withIcons && <span className="picker-axis-icon"><AxisIcon name={a.ui_schema.icon} size={14} /></span>}
                    <span className="min-w-0 flex-1 truncate">{a.display_name}</span>
                    {isRequired(a.code) && n === 0 && <span className="picker-req" title="Required">required</span>}
                    {n > 0 && <span className="picker-count">{n}</span>}
                  </button>
                )
              })}
              {shownAxes.length === 0 && <div className="t-xs px-2 py-3">No structure matches.</div>}
            </div>
          </div>
          <div className="p-2.5 @xl:hidden" style={{ borderBottom: '1px solid var(--line-soft)' }}>
            <Select size="sm" label="Structure" searchable allowDeselect={false} value={current}
              onChange={(v) => v && setActive(v)}
              data={axes.map((a) => {
                const n = value[a.code]?.length ?? 0
                return {
                  value: a.code,
                  label: a.display_name + (n ? ` · ${n} chosen` : isRequired(a.code) ? ' · required' : ''),
                }
              })} />
          </div>

          {/* The chosen structure's tree. */}
          <div className="min-w-0 p-2.5">
            {axis && (
              <>
                <div className="mb-2 flex items-center gap-2 px-0.5">
                  {axis.ui_schema.icon && (
                    <span style={{ color: 'var(--ink-3)', display: 'flex' }}><AxisIcon name={axis.ui_schema.icon} size={14} /></span>
                  )}
                  <span className="t-h2 truncate">{axis.display_name}</span>
                  {isRequired(axis.code) && (
                    <span className="chip" style={{ color: 'var(--warn)', borderColor: 'color-mix(in srgb, var(--warn) 30%, transparent)' }}>
                      required
                    </span>
                  )}
                  <span className="t-xs ml-auto hidden shrink-0 @md:inline">Click to add · again to remove</span>
                </div>
                {axis.ui_schema.help && <div className="t-xs mb-2 px-0.5">{axis.ui_schema.help}</div>}
                <ScopeTree key={axis.code} axis={axis.code} selectedId={null} picked={picked}
                  onSelect={toggle} height={258} placeholder={`Search ${axis.display_name}…`}
                  searchable={axis.ui_schema.searchable ?? true}
                  emptyHint="There is nothing to choose in it yet." />
              </>
            )}
          </div>
        </div>
      </div>

      {/* What has been chosen, in words, and the only place it is refined. */}
      <div>
        <div className="mb-1.5 flex items-baseline justify-between gap-2">
          <span className="t-label">Chosen places</span>
          {chosenAxes.length > 0 && (
            <UnstyledButton className="t-xs" style={{ color: 'var(--ink-3)' }} onClick={() => onChange({})}>
              Clear all
            </UnstyledButton>
          )}
        </div>
        {chosenAxes.length === 0 ? (
          <div className="chosen-empty t-xs">
            Nothing chosen yet. Pick a structure, then click the places this access should cover.
          </div>
        ) : (
          <div className="flex flex-col gap-2.5">
            {chosenAxes.map((a) => {
              const vs = value[a.code]!
              const inc = vs.filter((v) => !v.exclude).length
              return (
                <div key={a.code}>
                  <UnstyledButton className="mb-1 flex items-center gap-1.5 t-xs" onClick={() => setActive(a.code)}
                    style={{ color: 'var(--ink-2)', fontWeight: 560 }}>
                    {a.ui_schema.icon && <AxisIcon name={a.ui_schema.icon} size={13} />}
                    {a.display_name}
                    {inc > 1 && <span style={{ color: 'var(--ink-3)', fontWeight: 400 }}>· any of these {inc}</span>}
                  </UnstyledButton>
                  <div className="flex flex-col gap-1">
                    {vs.map((v) => (
                      <ChosenPlace key={v.id} v={v}
                        onChange={(m) => setAxis(a.code, vs.map((x) => (x.id === v.id ? { ...x, ...m } : x)))}
                        onRemove={() => setAxis(a.code, vs.filter((x) => x.id !== v.id))} />
                    ))}
                  </div>
                  {axisNeedsAnInclude(vs) && (
                    <div className="t-xs mt-1.5" style={{ color: 'var(--deny)' }}>
                      An exception carves out of somewhere: include a place in {a.display_name} too,
                      or remove the exception.
                    </div>
                  )}
                </div>
              )
            })}
          </div>
        )}
      </div>

      {/* The rules, said once, and only the ones this choice touches. */}
      {withIncludes.length > 0 && (
        <div className="callout">
          <IconInfoCircle size={14} style={{ color: 'var(--ink-3)', flexShrink: 0, marginTop: 2 }} />
          <div className="t-xs flex flex-col gap-1">
            {withIncludes.length > 1 && (
              <span>
                They need a match in <b>each</b> structure — {namesOf(withIncludes)}. Within one
                structure, any chosen place is enough.
              </span>
            )}
            {anyExcept && (
              <span>
                An exception narrows <b>this grant</b> only. Another grant that covers the same place
                still applies there — it is not a deny rule.
              </span>
            )}
            {silent > 0 && (
              <span>
                The {silent === 1 ? 'structure' : `${silent} structures`} you choose nothing from
                {silent === 1 ? ' stays' : ' stay'} unlimited.
              </span>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
