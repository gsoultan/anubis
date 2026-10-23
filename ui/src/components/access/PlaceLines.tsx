import { useQuery } from '@tanstack/react-query'
import { ActionIcon, Button, SegmentedControl, Select } from '@mantine/core'
import { IconInfinity, IconList, IconX } from '@tabler/icons-react'
import { api } from '@/lib/api/client'
import { fullParents, growInto, type NodeSel, type PlaceFacts, type Places } from '@/lib/access'
import { ScopeTree, type PickMark } from '@/components/scope/ScopeTree'
import { ChosenPlace } from './PlacePicker'
import type { ScopeAxis, ScopeNode } from '@/lib/api/types'

/* Where access applies, one line per kind of place.

   Each line is one of two things and says which: ∞ Everything, where no list
   is kept and a place added tomorrow is covered the day it exists; or
   ☰ Only these, a list that never changes on its own. The two look alike on
   the day access is given and stop looking alike the day a workplace is
   added, which is why the choice is made here, out loud, and not left to
   whichever one the form fell into.

   Lines appear for the kinds every grant must name, for kinds already
   picked from, and for any kind added with "Limit by another kind". A kind
   with no line is Everything: that is what leaving it out has always meant. */

export type LineMode = 'all' | 'only'

const byMode = (a: NodeSel, b: NodeSel) => Number(a.exclude) - Number(b.exclude) || a.name.localeCompare(b.name)

/** Picks that name every child of a parent, found from the places themselves. */
function useFullParents(list: NodeSel[]): { parents: PlaceFacts[]; facts: (id: string) => PlaceFacts | undefined } {
  const ids = list.filter((v) => !v.exclude).map((v) => v.id).sort()
  const { data: picked } = useQuery({
    queryKey: ['scope-names', ids], queryFn: () => api.scopeNodesByIds(ids), enabled: ids.length >= 2,
  })
  const parentIds = [...new Set((picked ?? []).map((n) => n.parent_id).filter((p): p is string => !!p))].sort()
  const { data: parents } = useQuery({
    queryKey: ['scope-names', parentIds], queryFn: () => api.scopeNodesByIds(parentIds), enabled: parentIds.length > 0,
  })
  const all = new Map<string, ScopeNode>([...(picked ?? []), ...(parents ?? [])].map((n) => [n.id, n]))
  const facts = (id: string): PlaceFacts | undefined => {
    const n = all.get(id)
    return n && { id: n.id, name: n.name, parent_id: n.parent_id, child_count: n.child_count, is_axis_root: n.is_axis_root }
  }
  return { parents: ids.length >= 2 ? fullParents(list, facts) : [], facts }
}

function Line({ axis, required, mode, rootId, list, onMode, onList, onRemove }: {
  axis: ScopeAxis
  required: boolean
  mode: LineMode | undefined
  rootId: string | undefined
  list: NodeSel[]
  onMode: (m: LineMode) => void
  onList: (next: NodeSel[]) => void
  onRemove?: (() => void) | undefined
}) {
  const noun = axis.display_name.toLowerCase()
  const picked = new Map<string, PickMark>(list.map((v) => [v.id, v.exclude ? 'exclude' : 'include']))
  const toggle = (n: ScopeNode) => onList(list.some((v) => v.id === n.id)
    ? list.filter((v) => v.id !== n.id)
    : [...list, { id: n.id, name: n.name, inherit: true, exclude: false }].sort(byMode))
  const { parents, facts } = useFullParents(mode === 'only' ? list : [])

  return (
    <div className="place-line panel-inset flex flex-col gap-2.5 p-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className="t-body min-w-0 flex-1 truncate" style={{ fontWeight: 600 }}>{axis.display_name}</span>
        {required && !mode && <span className="picker-req">required</span>}
        <SegmentedControl size="xs" value={mode ?? ''} onChange={(v) => onMode(v as LineMode)}
          aria-label={`${axis.display_name}: everything or only some`}
          data={[
            { value: 'all', label: <span className="inline-flex items-center gap-1"><IconInfinity size={13} />Everything</span> },
            { value: 'only', label: <span className="inline-flex items-center gap-1"><IconList size={13} />Only these</span> },
          ]} />
        {onRemove && (
          <ActionIcon variant="subtle" color="gray" size="sm" onClick={onRemove} aria-label={`No limit by ${noun}`}>
            <IconX size={13} />
          </ActionIcon>
        )}
      </div>

      {!mode && (
        <div className="t-xs">
          Every grant here has to say which {noun} it covers. Choose Everything, or only some.
        </div>
      )}
      {mode === 'all' && (
        <div className="t-xs">
          Every {noun}, including ones added later. No list is kept, so there is nothing to redo when one is added.
          {required && !rootId && <span style={{ color: 'var(--deny)' }}> This kind has no top place yet, so Everything cannot be written. Pick places instead.</span>}
        </div>
      )}
      {mode === 'only' && (
        <>
          <div className="t-xs">
            Exactly what you pick. One added later is not on this list unless it sits inside a place picked “with everything inside”.
          </div>
          <ScopeTree key={axis.code} axis={axis.code} selectedId={null} picked={picked} onSelect={toggle} height={220}
            placeholder={`Search ${noun}…`} searchable={axis.ui_schema.searchable ?? true}
            emptyHint="There is nothing to choose in it yet." />
          {list.length > 0 && (
            <div className="flex flex-col gap-1">
              {list.map((v) => (
                <ChosenPlace key={v.id} v={v}
                  onChange={(m) => onList(list.map((x) => (x.id === v.id ? { ...x, ...m } : x)).sort(byMode))}
                  onRemove={() => onList(list.filter((x) => x.id !== v.id))} />
              ))}
            </div>
          )}
          {parents.map((p) => (
            <div key={p.id} className="callout items-center">
              <span className="t-xs flex-1">
                You picked every place in <b>{p.name}</b>. As a list, the next one added there is left out.
                {p.is_axis_root ? ` Switch to Everything so new ${noun} are included?` : ` Use “${p.name} and everything inside” so new ones are included?`}
              </span>
              <Button size="compact-xs" variant="light"
                onClick={() => (p.is_axis_root ? onMode('all') : onList(growInto(list, p, facts).sort(byMode)))}>
                {p.is_axis_root ? 'Use Everything' : `Use ${p.name}`}
              </Button>
            </div>
          ))}
        </>
      )}
    </div>
  )
}

export function PlaceLines({ axes, required, rootOf, places, onPlaces, modes, onModes }: {
  axes: ScopeAxis[]
  required: ScopeAxis[]
  rootOf: (code: string) => string | undefined
  places: Places
  onPlaces: (next: Places) => void
  modes: Record<string, LineMode>
  onModes: (next: Record<string, LineMode>) => void
}) {
  const isRequired = (code: string) => required.some((r) => r.code === code)
  const lineCodes = axes.map((a) => a.code).filter((c) => isRequired(c) || (places[c]?.length ?? 0) > 0 || c in modes)
  const others = axes.filter((a) => !lineCodes.includes(a.code))

  const setList = (code: string, next: NodeSel[]) => {
    const out = { ...places }
    if (next.length === 0) delete out[code]
    else out[code] = next
    onPlaces(out)
  }
  const setMode = (code: string, m: LineMode) => {
    onModes({ ...modes, [code]: m })
    // Everything keeps no list: a list left behind would still be written.
    if (m === 'all') setList(code, [])
  }
  const drop = (code: string) => {
    const { [code]: _gone, ...rest } = modes
    onModes(rest)
    setList(code, [])
  }

  return (
    <div className="flex flex-col gap-2.5">
      {lineCodes.map((code) => {
        const axis = axes.find((a) => a.code === code)!
        return (
          <Line key={code} axis={axis} required={isRequired(code)}
            mode={modes[code] ?? ((places[code]?.length ?? 0) > 0 ? 'only' : undefined)}
            rootId={rootOf(code)} list={places[code] ?? []}
            onMode={(m) => setMode(code, m)} onList={(next) => setList(code, next)}
            onRemove={isRequired(code) ? undefined : () => drop(code)} />
        )
      })}
      {others.length > 0 && (
        <Select size="sm" placeholder={lineCodes.length === 0 ? 'Pick a kind of place to limit by' : 'Limit by another kind of place'}
          value={null} searchable aria-label="Limit by another kind of place"
          data={others.map((a) => ({ value: a.code, label: a.display_name }))}
          onChange={(v) => v && onModes({ ...modes, [v]: 'only' })} />
      )}
      {others.length > 0 && lineCodes.length > 0 && (
        <div className="t-xs">
          Kinds with no line here are not limited: every one, including new ones.
        </div>
      )}
    </div>
  )
}
