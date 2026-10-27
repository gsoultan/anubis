import { useState } from 'react'
import { Popover, TextInput, UnstyledButton } from '@mantine/core'
import { IconBuildingBank, IconCheck, IconChevronDown, IconSearch } from '@tabler/icons-react'
import { treeOrder } from '@/lib/tree'

type Company = { id: string; slug: string; name: string; parentId: string; role: string; inherited: boolean }

const RECENT_KEY = 'anubis.recent-companies'
function readRecent(): string[] {
  try { return JSON.parse(localStorage.getItem(RECENT_KEY) ?? '[]') as string[] } catch { return [] }
}
function remember(slug: string) {
  try { localStorage.setItem(RECENT_KEY, JSON.stringify([slug, ...readRecent().filter((s) => s !== slug)].slice(0, 4))) } catch { /* private window */ }
}

/* The company being administered, for someone who runs more than one. A
   search first, because a group can hold hundreds; the last few visited
   next; then every company they run, as the tree it is. */
export function CompanySwitcher({ companies, current, onPick }: {
  companies: Company[]; current: string; onPick: (slug: string) => void
}) {
  const [open, setOpen] = useState(false)
  const [q, setQ] = useState('')
  const active = companies.find((t) => t.slug === current)
  const byId = new Map(companies.map((c) => [c.id, c]))
  const parentName = (c: Company) => byId.get(c.parentId)?.name
  const needle = q.trim().toLowerCase()
  const found = needle ? companies.filter((c) => c.name.toLowerCase().includes(needle) || c.slug.includes(needle)) : []
  const recent = readRecent().filter((s) => s !== current).map((s) => companies.find((c) => c.slug === s)).filter((c): c is Company => !!c)
  const tops = companies.filter((c) => !c.inherited)
  const pick = (slug: string) => { remember(slug); setOpen(false); setQ(''); onPick(slug) }

  const row = (c: Company, depth = 0, sub?: string) => (
    <UnstyledButton key={`${c.slug}-${depth}-${sub ?? ''}`} onClick={() => pick(c.slug)}
      className="flex w-full items-center gap-2 rounded px-2 py-1.5"
      style={{ paddingLeft: 8 + depth * 14, background: c.slug === current ? 'var(--accent-bg)' : undefined }}>
      <span className="min-w-0 flex-1">
        <span className="t-body block truncate" style={{ fontWeight: c.slug === current ? 600 : 450 }}>{c.name}</span>
        {sub && <span className="t-xs block truncate">{sub}</span>}
      </span>
      {c.slug === current && <span className="t-xs inline-flex items-center gap-1" style={{ color: 'var(--accent)' }}><IconCheck size={12} />current</span>}
    </UnstyledButton>
  )

  return (
    <Popover opened={open} onChange={setOpen} position="bottom-start" width={320} shadow="xl" trapFocus>
      <Popover.Target>
        <button type="button" onClick={() => setOpen((o) => !o)} aria-expanded={open} aria-haspopup="dialog"
          className="flex items-center gap-2 px-2.5"
          style={{ cursor: 'pointer', height: 32, borderRadius: 'var(--r-sm)', border: '1px solid var(--line)', background: 'var(--s-raised)' }}>
          <IconBuildingBank size={14} style={{ color: 'var(--accent)', flex: 'none' }} />
          {active && parentName(active) && <span className="t-xs hidden truncate md:inline md:max-w-[110px]">{parentName(active)} ›</span>}
          <span className="t-body truncate max-w-[112px] md:max-w-[180px]" style={{ fontWeight: 560 }}>{active?.name ?? current}</span>
          <IconChevronDown size={13} style={{ color: 'var(--ink-3)', flex: 'none' }} />
        </button>
      </Popover.Target>
      <Popover.Dropdown p={6}>
        <TextInput size="xs" m={4} placeholder="Find a company" leftSection={<IconSearch size={13} />} data-autofocus
          value={q} onChange={(e) => setQ(e.currentTarget.value)} aria-label="Find a company" />
        <div className="flex max-h-[60vh] flex-col overflow-y-auto">
          {needle ? (
            found.length === 0
              ? <div className="t-xs px-2 py-3">No company you run matches.</div>
              : found.map((c) => row(c, 0, parentName(c) ? `in ${parentName(c)}` : undefined))
          ) : (
            <>
              {recent.length > 0 && (
                <>
                  <div className="t-label px-2 pb-1 pt-2">Recent</div>
                  {recent.map((c) => row(c, 0, parentName(c) ? `in ${parentName(c)}` : undefined))}
                </>
              )}
              <div className="t-label px-2 pb-1 pt-2">All companies you run</div>
              {treeOrder(companies, (t) => t.id, (t) => t.parentId, (t) => t.name).map(({ item, depth }) => row(item, depth))}
            </>
          )}
        </div>
        {!needle && tops.length > 0 && tops.length < companies.length && (
          <div className="t-xs px-2 pb-1 pt-2" style={{ borderTop: '1px solid var(--line-soft)' }}>
            You run {tops.map((t) => t.name).join(' and ')}, so every company below {tops.length === 1 ? 'it' : 'them'} is listed.
          </div>
        )}
      </Popover.Dropdown>
    </Popover>
  )
}
