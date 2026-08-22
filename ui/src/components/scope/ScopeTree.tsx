import { useState, useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Box, Group, Loader, Text, UnstyledButton, TextInput, ScrollArea } from '@mantine/core'
import { IconChevronRight, IconSearch, IconPoint } from '@tabler/icons-react'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import type { ScopeNode } from '@/lib/api/types'

/* Lazy, virtualisable tree over one axis.

   Children load on expand rather than up front. The customer axis in the
   benchmark dataset holds ~20,000 nodes; fetching a whole axis to render a
   picker would move that cost to every screen that needs one. */

interface RowProps {
  node: ScopeNode
  axis: string
  depth: number
  selectedId: string | null
  onSelect: (n: ScopeNode) => void
}

function TreeRow({ node, axis, depth, selectedId, onSelect }: RowProps) {
  const [open, setOpen] = useState(depth < 1)
  const hasChildren = (node.child_count ?? 0) > 0
  const { data: children, isFetching } = useQuery({
    queryKey: qk.scopeChildren(axis, node.id),
    queryFn: () => api.scopeChildren(axis, node.id),
    enabled: open && hasChildren,
  })

  const selected = selectedId === node.id
  return (
    <Box>
      <Group
        gap={4}
        wrap="nowrap"
        className="rounded-md"
        style={{
          paddingLeft: depth * 14 + 4,
          background: selected ? 'var(--s-overlay)' : undefined,
          transition: 'background var(--t-fast)',
        }}
      >
        <UnstyledButton
          onClick={() => setOpen((o) => !o)}
          aria-label={hasChildren ? (open ? 'Collapse' : 'Expand') : undefined}
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
        >
          <Text size="sm" fw={selected ? 600 : 430} truncate
            c={selected ? 'var(--ink)' : 'var(--ink-2)'}>
            {node.name}
          </Text>
        </UnstyledButton>

        <Text size="10px" className="shrink-0 pr-1 font-mono" c="var(--ink-4)">
          {node.node_type}
        </Text>
      </Group>

      {open && isFetching && (
        <Group gap={6} style={{ paddingLeft: (depth + 1) * 14 + 8 }} py={2}>
          <Loader size={10} />
          <Text size="10px" c="dimmed">loading…</Text>
        </Group>
      )}
      {open &&
        children?.map((c) => (
          <TreeRow key={c.id} node={c} axis={axis} depth={depth + 1}
            selectedId={selectedId} onSelect={onSelect} />
        ))}
    </Box>
  )
}

export function ScopeTree({
  axis, selectedId, onSelect, searchable = true, height = 320,
}: {
  axis: string
  selectedId: string | null
  onSelect: (n: ScopeNode) => void
  searchable?: boolean
  height?: number
}) {
  const [q, setQ] = useState('')
  const { data: roots, isLoading } = useQuery({
    queryKey: qk.scopeChildren(axis, null),
    queryFn: () => api.scopeChildren(axis, null),
  })
  const { data: hits } = useQuery({
    queryKey: qk.scopeSearch(axis, q),
    queryFn: () => api.scopeSearch(axis, q),
    enabled: q.trim().length >= 2,
  })

  const select = useCallback((n: ScopeNode) => onSelect(n), [onSelect])

  return (
    <Box>
      {searchable && (
        <TextInput
          size="xs"
          mb={6}
          placeholder="Filter nodes…"
          leftSection={<IconSearch size={13} />}
          value={q}
          onChange={(e) => setQ(e.currentTarget.value)}
        />
      )}
      <ScrollArea.Autosize mah={height} type="hover">
        {isLoading && <Loader size="xs" />}
        {q.trim().length >= 2
          ? hits?.map((n) => (
              <UnstyledButton key={n.id} onClick={() => select(n)}
                className="block w-full rounded-md px-2 py-1.5 text-left hover:bg-[var(--s-overlay)]">
                <Text size="sm" fw={selectedId === n.id ? 600 : 430}>{n.name}</Text>
                <Text size="10px" c="var(--ink-4)" className="font-mono">{n.node_type}</Text>
              </UnstyledButton>
            ))
          : roots?.map((r) => (
              <TreeRow key={r.id} node={r} axis={axis} depth={0}
                selectedId={selectedId} onSelect={select} />
            ))}
      </ScrollArea.Autosize>
    </Box>
  )
}
