import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button, Modal, TextInput } from '@mantine/core'
import { notifications } from '@mantine/notifications'
import {
  IconArchive, IconArrowsMove, IconCopy, IconPencil, IconPlus, IconRestore,
} from '@tabler/icons-react'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/client'
import { useCreate } from '@/stores/create'
import { notifyCreated, notifyRejected } from '@/components/create/shell'
import { ConfirmModal } from '@/components/ui/ConfirmModal'
import { ScopeTree } from './ScopeTree'
import type { ScopeNode, ScopeNodeType } from '@/lib/api/types'

/* One item of a structure, and everything that can be done to it.
 *
 * It used to be a read-only card with one button. Renaming a department after
 * a reorganisation, moving it, or retiring it had calls on the server and no
 * way to reach them — the tree could only grow.
 *
 * Each action says what it does to ACCESS, because that is the part nobody can
 * see from the tree: a rename changes nothing, a move changes who can reach
 * the item, and archiving takes it out of the pickers but leaves the access
 * already given there working.
 */

function Rename({ node, onClose, onDone }: { node: ScopeNode; onClose: () => void; onDone: () => void }) {
  const [name, setName] = useState(node.name)
  const [busy, setBusy] = useState(false)
  const save = async () => {
    setBusy(true)
    try {
      await api.renameScopeNode(node.id, name)
      notifyCreated('Renamed', `“${node.name}” is now “${name.trim()}”. Nobody’s access changed.`)
      onDone(); onClose()
    } catch (e) { notifyRejected(e) }
    setBusy(false)
  }
  const same = name.trim() === node.name || name.trim().length === 0
  return (
    <Modal opened onClose={busy ? () => {} : onClose} title="Rename this item" size={440}>
      <form onSubmit={(e) => { e.preventDefault(); if (!same) void save() }} className="flex flex-col gap-3 pt-1">
        <TextInput label="Name" maxLength={200} data-autofocus
          value={name} onChange={(e) => setName(e.currentTarget.value)} />
        <div className="t-xs">
          Only the name changes. Access given here, and everything inside, stays exactly as it is.
          {node.external_ref && ' This item comes from a connected source — the next sync puts the source’s name back.'}
        </div>
        <div className="flex justify-end gap-2">
          <Button variant="default" size="sm" onClick={onClose} disabled={busy}>Cancel</Button>
          <Button size="sm" type="submit" loading={busy} disabled={same}>Rename</Button>
        </div>
      </form>
    </Modal>
  )
}

function Move({ node, levels, levelName, onClose, onDone }: {
  node: ScopeNode
  levels: ScopeNodeType[]
  levelName: (code: string) => string
  onClose: () => void
  onDone: () => void
}) {
  const [to, setTo] = useState<ScopeNode | null>(null)
  const [busy, setBusy] = useState(false)
  const legal = levels.find((t) => t.code === node.node_type)?.parent_types ?? []
  /* The rule the database enforces, said before it has to refuse. The one it
     alone can know — that the target sits inside the item being moved — comes
     back as its own message. */
  const problem = !to ? 'Choose where it should sit.'
    : to.id === node.id ? 'An item cannot sit under itself.'
    : to.id === node.parent_id ? 'It already sits there.'
    : !legal.includes(to.node_type)
      ? `A ${levelName(node.node_type).toLowerCase()} cannot sit under a ${levelName(to.node_type).toLowerCase()} — only under ${legal.map(levelName).join(' or ') || 'nothing'}.`
    : ''
  const move = async () => {
    if (!to || problem) return
    setBusy(true)
    try {
      await api.moveScopeNode(node.id, to.id)
      notifyCreated('Moved', `“${node.name}” now sits under “${to.name}”, with everything inside it.`)
      onDone(); onClose()
    } catch (e) { notifyRejected(e) }
    setBusy(false)
  }
  return (
    <Modal opened onClose={busy ? () => {} : onClose} title={`Move “${node.name}”`} size={520}>
      <div className="flex flex-col gap-3 pt-1">
        <div className="t-sm">
          Moving changes who can reach it. People with access to where it sits now lose it here;
          people with access to the new place gain it — for this item and everything inside.
          Access given on the item itself moves with it.
        </div>
        <div className="panel-inset p-2">
          <ScopeTree axis={node.axis_code} selectedId={to?.id ?? null} onSelect={setTo} height={280}
            placeholder="Search for the new place…" emptyHint="Nothing to move it under." />
        </div>
        {node.external_ref && (
          <div className="t-xs">This item comes from a connected source — the next sync moves it back to where the source says.</div>
        )}
        <div className="flex items-center justify-between gap-3">
          <span className="t-xs min-w-0">{problem || `Will sit under “${to?.name}”.`}</span>
          <span className="flex shrink-0 gap-2">
            <Button variant="default" size="sm" onClick={onClose} disabled={busy}>Cancel</Button>
            <Button size="sm" loading={busy} disabled={!!problem} onClick={() => void move()}>Move</Button>
          </span>
        </div>
      </div>
    </Modal>
  )
}

export function ItemInspector({ node, levels, onChanged }: {
  node: ScopeNode
  /** This structure's levels. */
  levels: ScopeNodeType[]
  /** After a write: the fresh item, or null when it can no longer be shown. */
  onChanged: (next: ScopeNode | null) => void
}) {
  const { openCreate } = useCreate()
  const [doing, setDoing] = useState<'rename' | 'move' | 'archive' | null>(null)
  const [busy, setBusy] = useState(false)
  const { data: chain } = useQuery({
    queryKey: qk.ancestorPath(node.id),
    queryFn: () => api.ancestorPath(node.id),
  })
  const levelName = (code: string) => levels.find((t) => t.code === code)?.display_name ?? code
  const above = (chain ?? []).filter((a) => a.id !== node.id && !a.is_axis_root)
  const archived = node.status === 'archived'
  const under = levels.filter((t) => t.parent_types.includes(node.node_type))

  const reload = async () => {
    await queryClient.invalidateQueries({ queryKey: qk.scope() })
    onChanged(await api.scopeNode(node.id))
  }
  const archive = async () => {
    try {
      await api.archiveScopeNode(node.id)
    } catch (e) {
      notifyRejected(e)
      throw e
    }
    notifications.show({
      color: 'orange', title: 'Archived',
      message: `“${node.name}” no longer appears in pickers. Access already given there still works.`,
    })
    await reload()
  }
  const restore = async () => {
    setBusy(true)
    try {
      await api.restoreScopeNode(node.id)
      notifyCreated('Restored', `“${node.name}” is back in the pickers.`)
      await reload()
    } catch (e) { notifyRejected(e) }
    setBusy(false)
  }

  return (
    <div className="panel rise p-4">
      <div className="t-label mb-2">Selected item</div>
      <div className="t-h1" style={archived ? { color: 'var(--ink-3)' } : undefined}>{node.name}</div>
      {above.length > 0 && (
        <div className="t-xs mt-1">In {above.map((a) => a.name).join(' › ')}</div>
      )}
      <div className="mt-2 flex flex-wrap items-center gap-1.5">
        <span className="chip">{levelName(node.node_type)}</span>
        {node.is_axis_root && <span className="chip chip-accent">top of the structure</span>}
        {archived && <span className="v-pill v-pill-idle">archived</span>}
        <span className="chip">{node.child_count} inside</span>
        {node.external_ref && <span className="chip">from source · {node.external_ref}</span>}
      </div>

      {archived ? (
        <>
          <div className="t-xs mt-3">
            Archived: hidden from pickers, so no new access can be given here. Access given before
            still works until it is revoked.
          </div>
          <Button size="xs" variant="light" mt={10} fullWidth loading={busy}
            leftSection={<IconRestore size={13} />} onClick={() => void restore()}>
            Restore
          </Button>
        </>
      ) : (
        <>
          {under.length === 0 ? (
            <div className="t-xs mt-3">
              Nothing can be added under a {levelName(node.node_type).toLowerCase()} — add a level that sits under it first.
            </div>
          ) : (
            <Button size="xs" variant="light" mt={12} fullWidth leftSection={<IconPlus size={13} />}
              onClick={() => openCreate('node', { axisCode: node.axis_code, parentId: node.id })}>
              {/* The levels know what may live here, so the button says it. */}
              Add {under.length === 1 ? under[0]!.display_name.toLowerCase() : 'item'} under “{node.name}”
            </Button>
          )}
          <div className="mt-2 grid grid-cols-3 gap-1.5">
            <Button size="xs" variant="default" leftSection={<IconPencil size={13} />}
              onClick={() => setDoing('rename')}>Rename</Button>
            <Button size="xs" variant="default" leftSection={<IconArrowsMove size={13} />}
              disabled={node.is_axis_root} onClick={() => setDoing('move')}>Move</Button>
            <Button size="xs" variant="default" leftSection={<IconArchive size={13} />}
              disabled={node.is_axis_root} onClick={() => setDoing('archive')}>Archive</Button>
          </div>
          {node.is_axis_root && (
            <div className="t-xs mt-3" style={{ borderTop: '1px solid var(--line-soft)', paddingTop: 10 }}>
              The top of the structure cannot be moved or archived. Access given here covers
              <b style={{ color: 'var(--ink-2)' }}> everything in the structure</b> — said outright,
              which matters once the structure is made required.
            </div>
          )}
        </>
      )}

      <button type="button" className="t-xs mt-3 inline-flex items-center gap-1" style={{ color: 'var(--ink-3)' }}
        onClick={() => {
          void navigator.clipboard.writeText(node.id)
          notifications.show({ color: 'gray', title: 'Item ID copied', message: node.id })
        }}>
        <IconCopy size={12} /> Copy ID
      </button>

      {doing === 'rename' && <Rename node={node} onClose={() => setDoing(null)} onDone={() => void reload()} />}
      {doing === 'move' && (
        <Move node={node} levels={levels} levelName={levelName}
          onClose={() => setDoing(null)} onDone={() => void reload()} />
      )}
      <ConfirmModal opened={doing === 'archive'} onClose={() => setDoing(null)}
        title="Archive this item?" confirmLabel="Archive" onConfirm={archive}>
        <p>
          <b style={{ color: 'var(--ink)' }}>{node.name}</b> will stop appearing in pickers, so no new
          access can be given there.
        </p>
        <p>
          <b style={{ color: 'var(--ink)' }}>It does not take access away.</b> Everyone who has access
          there keeps it — revoke that from the people who hold it.
        </p>
        <p className="t-xs">
          {node.child_count > 0 ? `The ${node.child_count} item${node.child_count === 1 ? '' : 's'} inside stay as they are. ` : ''}
          Nothing is deleted, and it can be restored.
          {node.external_ref && ' It comes from a connected source — the next sync restores it if the source still has it.'}
        </p>
      </ConfirmModal>
    </div>
  )
}
