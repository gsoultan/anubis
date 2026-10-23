import { Link, useRouterState } from '@tanstack/react-router'
import { SECTIONS, type SectionKey } from './nav'

/* The tabs along the top of a page that absorbed its old neighbours —
   Roles / Groups / Permissions, Apps / Sync, and the three Settings pages.
   Links, not a tab widget: each tab is its own address, so bookmarks, Back
   and "open in a new tab" all keep working. */
export function SectionTabs({ section }: { section: SectionKey }) {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  return (
    <div role="navigation" aria-label="Sections" className="section-tabs">
      {SECTIONS[section].map((t) => {
        const on = pathname === t.to || pathname.startsWith(`${t.to}/`)
        return (
          <Link key={t.to} to={t.to} className="section-tab no-underline"
            {...(on ? { 'data-active': '', 'aria-current': 'page' as const } : {})}>
            {t.label}
          </Link>
        )
      })}
    </div>
  )
}
