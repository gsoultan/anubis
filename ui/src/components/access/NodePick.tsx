import { useQuery } from '@tanstack/react-query'
import { Popover } from '@mantine/core'
import { IconChevronDown } from '@tabler/icons-react'
import { useState } from 'react'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { ScopeTree } from '@/components/scope/ScopeTree'

/* One place of one kind, picked from its tree. The bulk change names a
   single place at a time ("add Medan", "swap Jakarta for Bandung"), so it
   needs the plain picker rather than the many-place one on Give access. */
export function NodePick({ axis, value, onChange, placeholder = 'Pick a place', label }: {
  axis: string
  value: string | null
  onChange: (id: string, name: string) => void
  placeholder?: string
  label?: string
}) {
  const [opened, setOpened] = useState(false)
  const { data: path } = useQuery({
    queryKey: qk.ancestorPath(value ?? ''), queryFn: () => api.ancestorPath(value!), enabled: !!value,
  })
  const here = path?.[path.length - 1]
  return (
    <Popover width={340} position="bottom-start" opened={opened} onChange={setOpened}>
      <Popover.Target>
        <button type="button" aria-label={label ?? placeholder} onClick={() => setOpened((o) => !o)}
          className="flex w-full min-w-0 items-center justify-between gap-2 rounded-md px-2.5 py-2 text-left"
          style={{ background: 'var(--s-sunken)', border: '1px solid var(--line-soft)' }}>
          {here ? (
            <span className="min-w-0">
              <span className="t-body block truncate" style={{ fontWeight: 500 }}>{here.name}</span>
              {path!.length > 1 && (
                <span className="t-xs block truncate">{path!.slice(0, -1).map((p) => p.name).join(' › ')}</span>
              )}
            </span>
          ) : <span className="t-sm" style={{ color: 'var(--ink-3)' }}>{placeholder}</span>}
          <IconChevronDown size={13} style={{ color: 'var(--ink-3)', flexShrink: 0 }} />
        </button>
      </Popover.Target>
      <Popover.Dropdown p="xs">
        <ScopeTree axis={axis} selectedId={value} emptyHint="Nothing of this kind yet."
          onSelect={(n) => { onChange(n.id, n.name); setOpened(false) }} />
      </Popover.Dropdown>
    </Popover>
  )
}
