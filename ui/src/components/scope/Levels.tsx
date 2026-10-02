import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button, Checkbox, MultiSelect, TextInput } from '@mantine/core'
import { IconAlertTriangle, IconPencil, IconPlus } from '@tabler/icons-react'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/client'
import { notifyCreated, notifyRejected } from '@/components/create/shell'
import { topDown } from '@/lib/levels'
import type { ScopeNodeType } from '@/lib/api/types'

/* A structure's levels: what kinds of item it has, and what may sit under
   what. "A department sits under an office or a division" is data — this is
   where it is read and changed, and the database holds every item to it.

   Three things this could not do before. Edit: the list was a row of editable
   selects wired to a call that always failed. Nest: a level could not sit
   inside itself, so a company under a company — every group with
   subsidiaries — needed one level per depth. Start: a new structure had no
   top level and no way to add one, so it could not hold its first item. */

function LevelForm({ axisCode, levels, level, onDone }: {
  axisCode: string
  levels: ScopeNodeType[]
  /** The level being edited; absent when adding one. */
  level?: ScopeNodeType
  onDone: () => void
}) {
  const isTop = !!level && level.parent_types.length === 0
  const [name, setName] = useState(level?.display_name ?? '')
  const [parents, setParents] = useState<string[]>(level?.parent_types.filter((p) => p !== level.code) ?? [])
  const [nests, setNests] = useState(level?.parent_types.includes(level.code) ?? false)
  const [busy, setBusy] = useState(false)
  const { data: all } = useQuery({ queryKey: qk.nodeTypes(), queryFn: api.nodeTypes })

  const label = name.trim() || 'this level'
  const problem = name.trim().length < 2 ? 'Name the level.'
    : !isTop && parents.length === 0 ? 'Choose what it sits under.'
    : ''

  const save = async () => {
    setBusy(true)
    try {
      if (level) {
        await api.updateNodeType({
          code: level.code, axis_code: axisCode, display_name: name,
          parent_types: isTop ? [] : nests ? [...parents, level.code] : parents,
        })
        notifyCreated('Level updated', `“${name.trim()}” — pickers and the tree follow at once.`)
      } else {
        await api.createNodeType({
          axis_code: axisCode, display_name: name, parent_types: parents, nests,
          taken: new Set((all ?? []).map((t) => t.code)),
        })
        notifyCreated('Level added', `“${name.trim()}” can now be added to the tree.`)
      }
      await queryClient.invalidateQueries({ queryKey: qk.nodeTypes() })
      onDone()
    } catch (e) { notifyRejected(e) }
    setBusy(false)
  }

  return (
    <div className="flex flex-col gap-2.5">
      <TextInput size="xs" label="Name" placeholder="Division" maxLength={80} data-autofocus
        value={name} onChange={(e) => setName(e.currentTarget.value)} />
      {isTop ? (
        <div className="t-xs">The top level sits under nothing. Only its name can change.</div>
      ) : (
        <>
          <MultiSelect size="xs" label="Sits under" placeholder={parents.length ? undefined : 'Choose one or more levels'}
            data={levels.filter((x) => x.code !== level?.code).map((x) => ({ value: x.code, label: x.display_name }))}
            value={parents} onChange={setParents} comboboxProps={{ withinPortal: true }} />
          <Checkbox size="xs" checked={nests} onChange={(e) => setNests(e.currentTarget.checked)}
            label={`A ${label.toLowerCase()} can sit inside another ${label.toLowerCase()}`}
            description="For shapes with no fixed depth — a company owned by a company, a folder in a folder." />
        </>
      )}
      {level && !isTop && (
        <div className="t-xs">
          A rule that items already rely on cannot be removed — move those items first.
        </div>
      )}
      <div className="flex items-center justify-between gap-2">
        <span className="t-xs">{problem}</span>
        <span className="flex gap-1.5">
          <Button size="compact-xs" variant="default" onClick={onDone} disabled={busy}>Cancel</Button>
          <Button size="compact-xs" loading={busy} disabled={!!problem} onClick={() => void save()}>
            {level ? 'Save' : 'Add level'}
          </Button>
        </span>
      </div>
    </div>
  )
}

/* A structure made before top levels were created with it (or through the
   API without one) holds nothing until it has one. Said, with the fix beside
   it, instead of left for "Add item" to fail on. */
function AddTop({ axisCode, axisName }: { axisCode: string; axisName: string }) {
  const [name, setName] = useState(`All ${axisName}`)
  const [busy, setBusy] = useState(false)
  const { data: all } = useQuery({ queryKey: qk.nodeTypes(), queryFn: api.nodeTypes })
  const add = async () => {
    setBusy(true)
    try {
      await api.createNodeType({
        axis_code: axisCode, display_name: name, parent_types: [],
        taken: new Set((all ?? []).map((t) => t.code)),
      })
      notifyCreated('Top level added', 'The structure can hold items now.')
      await queryClient.invalidateQueries({ queryKey: qk.nodeTypes() })
    } catch (e) { notifyRejected(e) }
    setBusy(false)
  }
  return (
    <div className="callout mb-3" style={{ alignItems: 'flex-start' }}>
      <IconAlertTriangle size={14} style={{ color: 'var(--warn)', flexShrink: 0, marginTop: 2 }} />
      <div className="min-w-0 flex-1">
        <div className="t-body" style={{ fontWeight: 560 }}>This structure has no top level</div>
        <div className="t-xs mt-0.5 mb-2">
          Every structure has one item at the top that the rest sits under. Until it has a level for it, nothing can be added.
        </div>
        <div className="flex items-end gap-1.5">
          <TextInput size="xs" className="flex-1" aria-label="Top level name" maxLength={80}
            value={name} onChange={(e) => setName(e.currentTarget.value)} />
          <Button size="xs" loading={busy} disabled={name.trim().length < 2} onClick={() => void add()}>
            Add top level
          </Button>
        </div>
      </div>
    </div>
  )
}

export function Levels({ axisCode, axisName }: { axisCode: string; axisName: string }) {
  const { data: types, isLoading } = useQuery({ queryKey: qk.nodeTypes(), queryFn: api.nodeTypes })
  const levels = topDown((types ?? []).filter((t) => t.axis_code === axisCode))
  const name = (code: string) => levels.find((t) => t.code === code)?.display_name ?? code
  /* One form open at a time: a level's code, or 'new'. */
  const [editing, setEditing] = useState<string | null>(null)
  const hasTop = levels.some((t) => t.parent_types.length === 0)

  return (
    <div className="p-4">
      <div className="t-label mb-1">Levels</div>
      <div className="t-xs mb-3">
        The kinds of item this structure has, and what sits under what. The database refuses
        an item placed anywhere else. Levels belong to the structure, so every tenant that
        uses it shares them.
      </div>

      {!isLoading && !hasTop && <AddTop axisCode={axisCode} axisName={axisName} />}

      <div className="flex flex-col gap-1.5">
        {levels.map((t) => {
          const top = t.parent_types.length === 0
          const under = t.parent_types.filter((p) => p !== t.code).map(name)
          const nests = t.parent_types.includes(t.code)
          return (
            <div key={t.code} className="panel-inset px-2.5 py-2">
              {editing === t.code ? (
                <LevelForm axisCode={axisCode} levels={levels} level={t} onDone={() => setEditing(null)} />
              ) : (
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0">
                    <div className="flex min-w-0 items-center gap-2">
                      <span className="t-body truncate" style={{ fontWeight: 560 }}>{t.display_name}</span>
                      {top && <span className="chip chip-accent shrink-0">top level</span>}
                    </div>
                    <div className="t-xs mt-0.5">
                      {top ? 'Everything else sits under it.'
                        : [
                            under.length > 0 && `Sits under ${under.join(' or ')}`,
                            nests && (under.length > 0 ? 'or another of its own' : 'Sits inside another of its own'),
                          ].filter(Boolean).join(', ')}
                    </div>
                  </div>
                  <Button size="compact-xs" variant="subtle" color="gray" className="shrink-0"
                    leftSection={<IconPencil size={12} />} aria-label={`Edit ${t.display_name}`}
                    onClick={() => setEditing(t.code)}>
                    Edit
                  </Button>
                </div>
              )}
            </div>
          )
        })}
      </div>

      {editing === 'new' ? (
        <div className="panel-inset mt-2 px-2.5 py-2.5">
          <LevelForm axisCode={axisCode} levels={levels} onDone={() => setEditing(null)} />
        </div>
      ) : (
        <Button size="xs" variant="light" mt={10} leftSection={<IconPlus size={13} />}
          disabled={!hasTop} onClick={() => setEditing('new')}>
          Add level
        </Button>
      )}
    </div>
  )
}
