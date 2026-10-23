import { createRootRoute, Outlet, redirect, useRouterState } from '@tanstack/react-router'
import { IconSearch } from '@tabler/icons-react'
import { CommandPalette } from '@/components/shell/CommandPalette'
import { CreateDrawers } from '@/components/create'
import { Sidebar } from '@/components/shell/Sidebar'
import { PhoneNav } from '@/components/shell/PhoneNav'
import { useFold } from '@/components/shell/useFold'
import { Account, Breadcrumb, NewMenu, TenantPicker, ThemeToggle } from '@/components/shell/Header'
import { currentTenant, isAuthenticated, myTenants, setCurrentTenant } from '@/lib/anubis'

function RootLayout() {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const [folded, setFolded] = useFold()

  /* Sign-in renders on its own. Wrapping it in the shell would put a nav
     full of links behind a form whose whole point is that you are not
     through yet. */
  if (pathname === '/signin' || pathname === '/setup') return <Outlet />

  return (
    <div className="flex h-full" style={{ background: 'var(--s-base)' }}>
      <CommandPalette />
      <CreateDrawers />

      {/* From md up the nav is a column: 216px, or a 64px rail when folded.
          It sits on --s-nav, a different plane from the page in both
          schemes. Below md it is the bar along the bottom (PhoneNav). */}
      <aside
        className="hidden shrink-0 flex-col md:flex"
        style={{ width: folded ? 'var(--rail-w)' : 'var(--nav-w)', borderRight: '1px solid var(--line)',
          background: 'var(--s-nav)', transition: 'width var(--t-mid)' }}
      >
        <Sidebar folded={folded} onFold={setFolded} />
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        {/* Left is context: which organisation, and where inside it. Right
            is action. In a console where every write lands in exactly one
            organisation, "which one am I changing?" leads the line. */}
        <header
          className="flex shrink-0 items-center justify-between gap-2 px-3 md:gap-4 md:px-5"
          style={{ height: 'var(--header-h)', borderBottom: '1px solid var(--line)',
            background: 'var(--s-base)' }}
        >
          <div className="flex min-w-0 items-center gap-2 md:gap-3">
            <TenantPicker />
            {/* Each page opens with its own title, so on a phone the crumb is
                the thing to drop; the organisation it sits beside is not. */}
            <div className="hidden min-w-0 sm:flex"><Breadcrumb /></div>
          </div>
          <div className="flex shrink-0 items-center gap-1.5 md:gap-2">
            <button
              onClick={() => window.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', metaKey: true }))}
              className="flex items-center gap-2 px-2.5" aria-label="Search"
              style={{ height: 32, borderRadius: 'var(--r-sm)',
                border: '1px solid var(--line)', background: 'var(--s-sunken)',
                transition: 'border-color var(--t-fast), background var(--t-fast)' }}
            >
              <IconSearch size={14} style={{ color: 'var(--ink-3)' }} />
              <span className="t-xs hidden md:inline" style={{ minWidth: 88, textAlign: 'left' }}>Search…</span>
              <kbd className="chip hidden md:inline" style={{ fontSize: 9.5, padding: '3px 5px' }}>⌘K</kbd>
            </button>
            <NewMenu />
            <div className="hidden md:block" style={{ width: 1, height: 22, background: 'var(--line)' }} />
            <span className="hidden md:inline-flex"><ThemeToggle /></span>
            <Account />
          </div>
        </header>

        {/* The bottom padding below md is the phone bar's height, so the
            last row of a page is never hidden under it. */}
        <main className="min-h-0 flex-1 overflow-y-auto pb-[calc(var(--phone-bar-h)+env(safe-area-inset-bottom))] md:pb-0">
          <Outlet />
        </main>
        <PhoneNav />
      </div>
    </div>
  )
}

/* One gate for the whole console.

   This started per-route, because most screens read the built-in sample data
   and worked with no server at all. That stopped being true the moment
   realms, identities, roles and permissions began reaching the API: a signed
   out visitor would now get a wall of failed requests instead of a sign-in
   form. One gate is also one place to get right. */
export const Route = createRootRoute({
  beforeLoad: async ({ location }) => {
    // The installer and the sign-in form are the two pages that exist before
    // anybody is signed in.
    if (location.pathname === '/signin' || location.pathname === '/setup') return
    if (!isAuthenticated()) {
      throw redirect({ to: '/signin', search: { next: location.href } })
    }
    // Resolve the working tenant BEFORE any screen mounts. Every admin query
    // carries X-Anubis-Tenant; on a fresh session the picker used to choose
    // the default in an effect, and any screen that rendered first fired
    // tenant-scoped requests with no tenant — a wall of failed calls right
    // after sign-in. (The server now refuses those cleanly as
    // no_tenant_selected, but the console should not make them at all.)
    if (!currentTenant()) {
      const tenants = await myTenants().catch(() => [])
      const first = tenants[0]
      if (first) setCurrentTenant(first.slug)
    }
  },
  component: RootLayout,
})

