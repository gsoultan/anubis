import { Link, useRouterState } from '@tanstack/react-router'
import { Tooltip } from '@mantine/core'
import { IconLayoutSidebarLeftCollapse, IconLayoutSidebarLeftExpand } from '@tabler/icons-react'
import { useIsOwner } from '@/stores/auth'
import { DAILY, PLATFORM, SYSTEM, isActive, type NavItem } from './nav'

/* The one place gold survives. It is the mark, not a UI colour — everything
   interactive is --accent so that nothing on screen can be mistaken for a
   status. */
export function Jackal() {
  return (
    <span
      className="flex items-center justify-center"
      style={{
        width: 28, height: 28, borderRadius: 8, flex: 'none',
        background: 'color-mix(in srgb, var(--brand) 14%, transparent)',
        border: '1px solid color-mix(in srgb, var(--brand) 26%, transparent)',
      }}
    >
      <svg viewBox="0 0 24 24" width={16} height={16} aria-hidden>
        <path d="M4 3l3.2 5.4A8.6 8.6 0 0 1 12 7c1.7 0 3.3.5 4.8 1.4L20 3l.9 7.4c.3 2.6-.6 5.2-2.5 7L12 22l-6.4-4.6c-1.9-1.8-2.8-4.4-2.5-7L4 3z"
          fill="var(--brand)" />
        <circle cx="9.4" cy="12.6" r="1.15" fill="var(--s-nav)" />
        <circle cx="14.6" cy="12.6" r="1.15" fill="var(--s-nav)" />
      </svg>
    </span>
  )
}

function Item({ item, folded }: { item: NavItem; folded: boolean }) {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const active = isActive(item, pathname)
  const link = (
    <Link to={item.to} className="nav-item no-underline" aria-label={folded ? item.label : undefined}
      {...(active ? { 'data-active': '', 'aria-current': 'page' as const } : {})}>
      <span className="nav-icon">{item.icon(18)}</span>
      {!folded && <span className="truncate">{item.label}</span>}
    </Link>
  )
  /* Folded, the label IS the tooltip — without it the rail is a row of
     pictures. Unfolded the label is on screen and the hint is extra. */
  const tip = folded ? item.label : item.hint
  return tip
    ? <Tooltip label={tip} position="right" openDelay={folded ? 0 : 700} withArrow={folded}>{link}</Tooltip>
    : link
}

function Group({ items, folded, title }: { items: NavItem[]; folded: boolean; title?: string }) {
  return (
    <div className="flex flex-col gap-0.5">
      {title && !folded && <div className="t-label px-2.5 pb-1.5 pt-1">{title}</div>}
      {items.map((it) => <Item key={it.key} item={it} folded={folded} />)}
    </div>
  )
}

/* The sidebar, and the same sidebar folded to a 64px rail. One component so
   the two cannot drift: folding drops the labels and nothing else. */
export function Sidebar({ folded, onFold, inDrawer = false }: {
  folded: boolean
  onFold?: (folded: boolean) => void
  /** Rendered in a drawer (none today, kept for narrow embedded hosts). */
  inDrawer?: boolean
}) {
  const owner = useIsOwner()
  return (
    <nav aria-label="Main" className="flex h-full flex-col" data-folded={folded ? '' : undefined}>
      <div className={`flex items-center gap-2.5 ${folded ? 'justify-center' : 'px-4'}`}
        style={{ height: 'var(--header-h)', flex: 'none' }}>
        <Jackal />
        {!folded && (
          <span style={{ fontSize: 15, fontWeight: 660, letterSpacing: '-.022em' }}>Anubis</span>
        )}
      </div>

      <div className={`flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto pb-3 ${folded ? 'px-2' : 'px-2.5'}`}>
        <Group items={DAILY} folded={folded} />
        <div style={{ height: 1, background: 'var(--line-soft)', margin: '0 6px' }} />
        <Group items={SYSTEM} folded={folded} />
        {owner && (
          <>
            <div style={{ height: 1, background: 'var(--line-soft)', margin: '0 6px' }} />
            <Group items={PLATFORM} folded={folded} title="Platform" />
          </>
        )}
      </div>

      {onFold && !inDrawer && (
        <div className={`flex-none pb-3 pt-2 ${folded ? 'px-2' : 'px-2.5'}`}
          style={{ borderTop: '1px solid var(--line-soft)' }}>
          <Tooltip label={folded ? 'Unfold the sidebar' : 'Fold the sidebar'} position="right"
            disabled={!folded}>
            <button type="button" className="nav-item w-full" onClick={() => onFold(!folded)}
              aria-label={folded ? 'Unfold the sidebar' : 'Fold the sidebar'}
              style={{ border: 0, background: 'transparent', cursor: 'pointer' }}>
              <span className="nav-icon">
                {folded ? <IconLayoutSidebarLeftExpand size={18} /> : <IconLayoutSidebarLeftCollapse size={18} />}
              </span>
              {!folded && <span>Fold sidebar</span>}
            </button>
          </Tooltip>
        </div>
      )}
    </nav>
  )
}
