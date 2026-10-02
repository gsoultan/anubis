import { useState, useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Box, Group, Loader, Text, UnstyledButton, TextInput, ScrollArea } from '@mantine/core'
import { IconChevronRight, IconSearch, IconPoint, IconCircleCheckFilled, IconCircleMinus } from '@tabler/icons-react'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import type { ScopeNode } from '@/lib/api/types'

/* Lazy, virtualisable tree over one axis.

   Children load on expand rather than up front. The customer axis in the
   benchmark dataset holds ~20,000 nodes; fetching a whole axis to render a
   picker would move that cost to every screen that needs one.

   Two ways to show a choice. `selectedId` is one node — the Structure page and
   the access check. `picked` is many, each included or carved out — the place
   picker, where a row has to say which of the two it is before it is clicked
   again, not after. */

export type PickMark = 'include' | 'exclude'

interface RowProps {
  node: ScopeNode
  axis: string
  /** A level's name by its code. The code is a key nobody chose to read. */
  kind: (code: string) => string
  archived: boolean
  depth: number
  selectedId: string | null
  picked: ReadonlyMap<string, PickMark> | undefined
  onSelect: (n: ScopeNode) => void
}

function Mark({ mark }: { mark: PickMark }) {
  return mark === 'include'
    ? <IconCircleCheckFilled size={15} style={{ color: 'var(--accent)', flexShrink: 0 }} aria-label="included" />
    : <IconCircleMinus size={15} style={{ color: 'var(--deny)', flexShrink: 0 }} aria-label="excluded" />
}

function rowTint(mark: PickMark | undefined, selected: boolean): string | undefined {
  if (mark === 'include') return 'var(--accent-bg)'
  if (mark === 'exclude') return 'var(--deny-bg)'
  return selected ? 'var(--s-overlay)' : undefined
}

function TreeRow({ node, axis, kind, archived, depth, selectedId, picked, onSelect }: RowProps) {
  const [open, setOpen] = useState(depth < 1)
  const hasChildren = (node.child_count ?? 0) > 0
  const { data: children, isFetching } = useQuery({
    queryKey: qk.scopeChildren(axis, node.id, archived),
    queryFn: () => api.scopeChildren(axis, node.id, archived),
    enabled: open && hasChildren,
  })

  const mark = picked?.get(node.id)
  const selected = selectedId === node.id
  const gone = node.status === 'archived'
  return (
    <Box>
      <Group
        gap={4}
        wrap="nowrap"
        className="scope-row rounded-md"
        style={{
          paddingLeft: depth * 14 + 4,
          background: rowTint(mark, selected),
          transition: 'background var(--t-fast)',
        }}
      >
        <UnstyledButton
          onClick={() => setOpen((o) => !o)}
          aria-label={hasChildren ? (open ? `Collapse ${node.name}` : `Expand ${node.name}`) : undefined}
          disabled={!hasChildren}
          className="flex h-6 w-4 shrink-0 items-center justify-center"
        >
          {hasChildren ? (
            <IconChevronRight
              size={13}
              style={{ transform: open ? 'rotate(90deg)' : undefined, transition: 'transform 120ms' }}
            />
          ) : (
            <IconPoint size={8} opacity={0.35} />
          )}
        </UnstyledButton>

        <UnstyledButton
          onClick={() => onSelect(node)}
          className="min-w-0 flex-1 truncate py-1 text-left"
          aria-pressed={picked ? !!mark : undefined}
        >
          <Text size="sm" fw={selected || mark ? 600 : 430} truncate
            c={mark === 'exclude' ? 'var(--deny)' : gone ? 'var(--ink-3)' : selected || mark ? 'var(--ink)' : 'var(--ink-2)'}
            {...(gone ? { td: 'line-through' } : {})}>
            {node.name}
          </Text>
        </UnstyledButton>

        {mark
          ? <span className="flex pr-1"><Mark mark={mark} /></span>
          : <Text size="xs" className="shrink-0 pr-1.5" c="var(--ink-4)">{gone ? 'archived' : kind(node.node_type)}</Text>}
      </Group>

      {open && isFetching && (
        <Group gap={6} style={{ paddingLeft: (depth + 1) * 14 + 8 }} py={2}>
          <Loader size={10} />
          <Text size="10px" c="dimmed">loading…</Text>
        </Group>
      )}
      {open &&
        children?.map((c) => (
          <TreeRow key={c.id} node={c} axis={axis} kind={kind} archived={archived} depth={depth + 1}
            selectedId={selectedId} picked={picked} onSelect={onSelect} />
        ))}
    </Box>
  )
}

export function ScopeTree({
  axis, selectedId, onSelect, searchable = true, height = 320, picked, archived = false,
  emptyHint = 'Add the first one with “Add item” above.', placeholder = 'Search by name…',
}: {
  axis: string
  selectedId: string | null
  onSelect: (n: ScopeNode) => void
  searchable?: boolean
  /** Max height of the scroll area. Accepts a CSS length so the Structure
      page can hand it a viewport-relative height and let the tree fill the
      screen — as a fixed 430px it left an empty white rectangle wherever the
      panel beside it was taller. */
  height?: number | string
  /** Many chosen nodes, each marked included or carved out. */
  picked?: ReadonlyMap<string, PickMark>
  /** Also list archived items — the Structure page, to restore one. A picker
      never does: an archived place is not somewhere to give access. */
  archived?: boolean
  /** What to do about an empty structure — which depends on the screen:
      the Structure page can add an item, a picker cannot. */
  emptyHint?: string
  placeholder?: string
}) {
  const [q, setQ] = useState('')
  const { data: roots, isLoading } = useQuery({
    queryKey: qk.scopeChildren(axis, null, archived),
    queryFn: () => api.scopeChildren(axis, null, archived),
  })
  const { data: hits } = useQuery({
    queryKey: qk.scopeSearch(axis, q, archived),
    queryFn: () => api.scopeSearch(axis, q, archived),
    enabled: q.trim().length >= 2,
  })
  const { data: types } = useQuery({ queryKey: qk.nodeTypes(), queryFn: api.nodeTypes })
  const kind = useCallback(
    (code: string) => types?.find((t) => t.code === code)?.display_name ?? code, [types])

  const select = useCallback((n: ScopeNode) => onSelect(n), [onSelect])

  return (
    <Box>
      {searchable && (
        <TextInput
          size="xs"
          mb={6}
          placeholder={placeholder}
          leftSection={<IconSearch size={13} />}
          value={q}
          onChange={(e) => setQ(e.currentTarget.value)}
        />
      )}
      <ScrollArea.Autosize mah={height} type="hover">
        {isLoading && <Loader size="xs" />}

        {/* An axis with no items rendered as blank space — no rows, no
            message, nothing to say whether it was empty or still loading.
            Both ends of the search need the same courtesy. */}
        {!isLoading && q.trim().length < 2 && (roots?.length ?? 0) === 0 && (
          <div className="px-3 py-12 text-center">
            <div className="t-sm">No items in this structure yet</div>
            <div className="t-xs mt-1">{emptyHint}</div>
          </div>
        )}
        {q.trim().length >= 2 && hits !== undefined && hits.length === 0 && (
          <div className="px-3 py-12 text-center">
            <div className="t-sm">Nothing matches “{q.trim()}”</div>
            <div className="t-xs mt-1">Search covers this structure only.</div>
          </div>
        )}

        {q.trim().length >= 2
          ? hits?.map((n) => {
              const mark = picked?.get(n.id)
              return (
                <UnstyledButton key={n.id} onClick={() => select(n)}
                  aria-pressed={picked ? !!mark : undefined}
                  className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left hover:bg-[var(--s-overlay)]"
                  style={{ background: rowTint(mark, selectedId === n.id) }}>
                  <span className="min-w-0 flex-1">
                    <Text size="sm" fw={selectedId === n.id || mark ? 600 : 430} truncate
                      {...(mark === 'exclude' ? { c: 'var(--deny)' } : {})}>{n.name}</Text>
                    {/* Whose "Sales" this is. Two hits with one name were
                        indistinguishable, and the wrong one is a wrong grant. */}
                    <Text size="xs" c="var(--ink-3)" truncate>
                      {[kind(n.node_type), n.status === 'archived' && 'archived', n.path.join(' › ')]
                        .filter(Boolean).join(' · ')}
                    </Text>
                  </span>
                  {mark && <Mark mark={mark} />}
                </UnstyledButton>
              )
            })
          : roots?.map((r) => (
              <TreeRow key={r.id} node={r} axis={axis} kind={kind} archived={archived} depth={0}
                selectedId={selectedId} picked={picked} onSelect={select} />
            ))}
      </ScrollArea.Autosize>
    </Box>
  )
}
