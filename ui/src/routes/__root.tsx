import { createRootRoute, Link, Outlet, redirect, useNavigate, useRouterState } from '@tanstack/react-router'
import { Tooltip } from '@mantine/core'
import {
  IconLayoutDashboard, IconUsers, IconSitemap, IconKey, IconShieldCheck,
  IconFileDescription, IconAffiliate, IconWorld, IconTestPipe, IconSearch,
  IconPointFilled,
} from '@tabler/icons-react'
import { ActionIcon, Burger, Button, Drawer, Menu, useComputedColorScheme, useMantineColorScheme } from '@mantine/core'
import { useMediaQuery } from '@mantine/hooks'
import {
  IconPlus, IconUserPlus, IconLicense, IconShieldPlus, IconCirclePlus,
  IconSitemapFilled, IconAxisY, IconSun, IconMoon, IconUsersGroup, IconTableImport,
  IconLogout, IconUserCircle, IconShieldCog, IconAppWindow, IconRefreshDot,
} from '@tabler/icons-react'
import type { ReactNode } from 'react'
import { CommandPalette } from '@/components/shell/CommandPalette'
import { CreateDrawers } from '@/components/create'
import { useCreate } from '@/stores/create'
import { ChangePassword } from '@/components/shell/ChangePassword'
import { signOut, useAuthed, useIsOwner, useWho } from '@/stores/auth'
import { currentTenant, myTenants, setCurrentTenant } from '@/lib/anubis'
import { queryClient } from '@/lib/query/client'
import { useEffect, useState } from 'react'
import { isAuthenticated } from '@/lib/anubis'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { IconBuildingBank, IconChevronDown, IconChevronRight, IconBrush, IconCheck } from '@tabler/icons-react'

type Item = { to: string; label: string; icon: ReactNode; hint?: string }
/* Platform view: what the SUPER ADMIN sees — the tenants themselves, and the
   deployment-level machinery. Tenant view is everything that already existed:
   the tenant admin managing their own people. Same shell, two vocabularies of
   responsibility. */
const PLATFORM_GROUPS: { title: string | null; items: Item[] }[] = [
  {
    title: 'Platform',
    items: [
      { to: '/tenants', label: 'Tenants', icon: <IconBuildingBank size={15} />,
        hint: 'Every organisation on this Anubis' },
      { to: '/operators', label: 'Platform users', icon: <IconShieldCog size={15} />,
        hint: 'Who operates this installation, and which tenants they may administer' },
    ],
  },
]

const GROUPS: { title: string | null; items: Item[] }[] = [
  {
    title: null,
    items: [
      { to: '/', label: 'Overview', icon: <IconLayoutDashboard size={15} /> },
      { to: '/playground', label: 'Access check', icon: <IconTestPipe size={15} />,
        hint: 'Ask “can this person do this?” and see exactly why' },
    ],
  },
  {
    title: 'People',
    items: [
      { to: '/identities', label: 'People', icon: <IconUsers size={15} /> },
      { to: '/realms', label: 'Populations', icon: <IconWorld size={15} />,
        hint: 'Internal, partners and public — and their sign-in rules' },
      { to: '/signin-page', label: 'Sign-in & sign-out', icon: <IconBrush size={15} />,
        hint: 'Brand the login page this tenant’s people see' },
      { to: '/import', label: 'Import', icon: <IconTableImport size={15} />,
        hint: 'Bring people and their access in from a spreadsheet' },
    ],
  },
  {
    title: 'Access',
    items: [
      { to: '/grants', label: 'Access', icon: <IconAffiliate size={15} />,
        hint: 'Who can do what, and where' },
      { to: '/memberships', label: 'Memberships', icon: <IconUsersGroup size={15} />,
        hint: 'Role bundles — onboard a hire in one action' },
      { to: '/roles', label: 'Roles & permissions', icon: <IconShieldCheck size={15} /> },
      { to: '/applications', label: 'Applications', icon: <IconAppWindow size={15} />,
        hint: 'The relying parties that own permissions' },
      { to: '/catalog', label: 'Catalog sync', icon: <IconRefreshDot size={15} />,
        hint: 'Read an application’s permissions and roles from where they are maintained' },
    ],
  },
  {
    title: 'Structure',
    items: [
      { to: '/scope', label: 'Structure', icon: <IconSitemap size={15} />,
        hint: 'Offices, product lines, customers — what access is limited to' },
    ],
  },
  {
    title: 'System',
    items: [
      { to: '/audit', label: 'Audit', icon: <IconFileDescription size={15} /> },
      { to: '/keys', label: 'Signing keys', icon: <IconKey size={15} /> },
    ],
  },
]

function NavItem({ to, label, icon, hint }: Item) {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const active = to === '/' ? pathname === '/' : pathname.startsWith(to)
  const el = (
    <Link to={to} className="nav-item no-underline" {...(active ? { 'data-active': '' } : {})}>
      <span className="nav-icon">{icon}</span>
      <span className="truncate">{label}</span>
    </Link>
  )
  return hint ? <Tooltip label={hint} position="right" openDelay={600}>{el}</Tooltip> : el
}

/* The one place gold survives. It is the mark, not a UI colour — everything
   interactive moved to --accent so that nothing on screen can be mistaken for
   a status. */
function Jackal() {
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

/* The global create menu is the single biggest usability fix: every object in
   the system is creatable from anywhere, in two clicks, without first finding
   the right page. */
function NewMenu() {
  const { openCreate } = useCreate()
  const items = [
    { kind: 'identity' as const, label: 'Person', icon: <IconUserPlus size={15} />, hint: 'someone internal, a partner contact, or a public user' },
    { kind: 'grant' as const, label: 'Access', icon: <IconCirclePlus size={15} />, hint: 'give a person a role, limited to a place' },
    { kind: 'membership' as const, label: 'Membership', icon: <IconUsersGroup size={15} />, hint: 'a role bundle for easy onboarding' },
    { kind: 'role' as const, label: 'Role', icon: <IconShieldPlus size={15} />, hint: 'a named bundle of permissions' },
    { kind: 'permission' as const, label: 'Permission', icon: <IconLicense size={15} />, hint: 'declared in its app’s manifest' },
    { kind: 'node' as const, label: 'Structure item', icon: <IconSitemapFilled size={15} />, hint: 'an office, product line, customer…' },
    { kind: 'axis' as const, label: 'Structure', icon: <IconAxisY size={15} />, hint: 'a whole new way to limit access' },
  ]
  return (
    <Menu position="bottom-end" width={280} shadow="xl">
      <Menu.Target>
        <Button size="xs" h={32} leftSection={<IconPlus size={14} />}>Add</Button>
      </Menu.Target>
      <Menu.Dropdown>
        {items.map((it) => (
          <Menu.Item key={it.kind} leftSection={it.icon} onClick={() => openCreate(it.kind)}>
            <span className="t-body" style={{ fontWeight: 530 }}>{it.label}</span>
            <span className="t-xs block">{it.hint}</span>
          </Menu.Item>
        ))}
      </Menu.Dropdown>
    </Menu>
  )
}

function ThemeToggle() {
  const { setColorScheme } = useMantineColorScheme()
  const computed = useComputedColorScheme('light')
  const next = computed === 'dark' ? 'light' : 'dark'
  return (
    <ActionIcon
      variant="default" size={32} radius="sm" aria-label={`Switch to ${next} theme`}
      onClick={() => setColorScheme(next)}
    >
      {computed === 'dark' ? <IconSun size={15} /> : <IconMoon size={15} />}
    </ActionIcon>
  )
}

function SideNav() {
  /* An owner runs the installation AND can administer any tenant, so they see
     both. An operator sees only the tenant work they were assigned. */
  const owner = useIsOwner()
  const groups = owner ? [...PLATFORM_GROUPS, ...GROUPS] : GROUPS
  return (
    <nav className="flex-1 overflow-y-auto px-2.5 pb-3">
      {groups.map((g, gi) => (
        <div
          key={g.title ?? gi}
          className={gi === 0 ? '' : 'mt-3 pt-3'}
          /* A hairline above each titled group. Five sections separated only by
             whitespace read as one long list — the eye needs an edge to count
             groups against. */
          style={gi === 0 ? undefined : { borderTop: '1px solid var(--line-soft)' }}
        >
          {g.title && <div className="t-label px-2.5 pb-2">{g.title}</div>}
          <div className="flex flex-col gap-px">
            {g.items.map((n) => <NavItem key={n.to} {...n} />)}
          </div>
        </div>
      ))}
    </nav>
  )
}

/* The tenant this operator is administering.

   Defaults to the first one they are assigned to, so somebody with a single
   tenant never has to choose. The selection is context, not authority: it
   rides along with every request and the server checks it against their
   assignments each time. */
function TenantPicker() {
  const authed = useAuthed()
  const { data: tenants, error } = useQuery({
    queryKey: ['my-tenants'],
    queryFn: myTenants,
    enabled: authed,
    staleTime: 60_000,
    retry: false,
  })
  const [current, setCurrent] = useState(currentTenant())

  useEffect(() => {
    if (!tenants?.length) return
    const known = tenants.some((t) => t.slug === current)
    if (!current || !known) {
      const first = tenants[0]
      if (first) {
        setCurrentTenant(first.slug)
        setCurrent(first.slug)
      }
    }
  }, [tenants, current])

  if (!authed) return null
  /* A failed lookup is not "no tenants". Rendering nothing for both is how
     this went unnoticed: the picker simply was not there, and nothing said
     why. */
  if (error) {
    return (
      <Tooltip label={(error as Error).message}>
        <div className="chip" style={{ color: 'var(--warn)' }}>tenants unavailable</div>
      </Tooltip>
    )
  }
  if (!tenants?.length) return null

  const active = tenants.find((t) => t.slug === current)
  // One tenant is not a choice; showing a menu implies there is somewhere
  // else to go.
  if (tenants.length === 1) {
    return (
      <Tooltip label={`Administering ${active?.name ?? current} as ${active?.role ?? ''}`}>
        <div className="flex items-center gap-2 px-2.5"
          style={{ height: 32, borderRadius: 'var(--r-sm)',
            border: '1px solid var(--line)', background: 'var(--s-raised)' }}>
          <IconBuildingBank size={14} style={{ color: 'var(--accent)', flex: 'none' }} />
          <span className="t-body truncate max-w-[112px] md:max-w-[180px]" style={{ fontWeight: 560 }}>
            {active?.name ?? current}
          </span>
        </div>
      </Tooltip>
    )
  }
  return (
    <Menu position="bottom-start" width={260}>
      <Menu.Target>
        <button
          className="flex items-center gap-2 px-2.5"
          style={{
            cursor: 'pointer', height: 32, borderRadius: 'var(--r-sm)',
            border: '1px solid var(--line)', background: 'var(--s-raised)',
            transition: 'border-color var(--t-fast), background var(--t-fast)',
          }}
        >
          <IconBuildingBank size={14} style={{ color: 'var(--accent)', flex: 'none' }} />
          <span className="t-body truncate max-w-[112px] md:max-w-[180px]" style={{ fontWeight: 560 }}>
            {active?.name ?? current}
          </span>
          <IconChevronDown size={13} style={{ color: 'var(--ink-3)', flex: 'none' }} />
        </button>
      </Menu.Target>
      <Menu.Dropdown>
        <Menu.Label>Administering</Menu.Label>
        {tenants.map((t) => (
          <Menu.Item key={t.slug}
            rightSection={t.slug === current ? <IconCheck size={13} /> : null}
            onClick={() => {
              setCurrentTenant(t.slug)
              setCurrent(t.slug)
              // Everything on screen was fetched for the previous tenant.
              queryClient.clear()
            }}>
            {t.name}
            <span className="t-xs block">{t.role}</span>
          </Menu.Item>
        ))}
      </Menu.Dropdown>
    </Menu>
  )
}

/** The account control. The tenant chip used to be hardcoded to "impack";
    once a screen could really sign in, a chip that names a tenant nobody is
    signed into is worse than no chip at all. */
function Account() {
  const authed = useAuthed()
  const who = useWho()
  const navigate = useNavigate()
  const [changing, setChanging] = useState(false)

  if (!authed) {
    return (
      <Button variant="default" size="compact-sm" leftSection={<IconUserCircle size={15} />}
        onClick={() => navigate({ to: '/signin', search: { next: '/' } })}>
        Sign in
      </Button>
    )
  }
  return (
    <>
    <Menu position="bottom-end" width={200}>
      <Menu.Target>
        <button className="flex items-center gap-2 pl-1 pr-2"
          style={{ cursor: 'pointer', height: 32, borderRadius: 'var(--r-sm)',
            border: '1px solid var(--line)', background: 'var(--s-sunken)' }}>
          <span className="avatar" style={{ width: 22, height: 22, fontSize: 10,
            background: 'var(--accent-bg)', color: 'var(--accent)',
            borderColor: 'var(--accent-line)' }}>
            {(who?.username ?? '?').slice(0, 1)}
          </span>
          <span className="t-xs hidden truncate md:block" style={{ color: 'var(--ink-2)', fontWeight: 560, maxWidth: 110 }}>
            {who?.username ?? 'signed in'}
          </span>
        </button>
      </Menu.Target>
      <Menu.Dropdown>
        <Menu.Label>platform operator</Menu.Label>
        <Menu.Item leftSection={<IconKey size={15} />} onClick={() => setChanging(true)}>
          Change password
        </Menu.Item>
        <Menu.Item
          leftSection={<IconLogout size={15} />}
          onClick={async () => { await signOut(); navigate({ to: '/signin', search: { next: '/' } }) }}
        >
          Sign out
        </Menu.Item>
      </Menu.Dropdown>
    </Menu>
    <ChangePassword opened={changing} onClose={() => setChanging(false)} />
    </>
  )
}

/* The nav's contents, rendered twice — as the fixed column from md up and
   inside the drawer below it. One component, so the two cannot drift. */
function NavContent({ authed }: { authed: boolean }) {
  return (
    <>
        <div className="flex items-center gap-2.5 px-4"
          style={{ height: 'var(--header-h)' }}>
          <Jackal />
          <div className="leading-none">
            <div style={{ fontSize: 14.5, fontWeight: 660, letterSpacing: '-.022em' }}>Anubis</div>
            <div className="t-xs" style={{ marginTop: 3, fontSize: 10.5 }}>console</div>
          </div>
        </div>

        <SideNav />

        {/* A green dot when the console is live. This used to be --warn, so a
            perfectly healthy installation announced itself in the colour the
            rest of the UI uses for "something needs attention". */}
        <div className="px-2.5 pb-3 pt-2" style={{ borderTop: '1px solid var(--line-soft)' }}>
          <div className="flex items-center gap-2 px-2.5 py-1.5">
            <IconPointFilled size={10} style={{ color: authed ? 'var(--allow)' : 'var(--warn)', flex: 'none' }} />
            <div className="min-w-0">
              <div className="t-xs truncate" style={{ color: 'var(--ink-2)', fontWeight: 560 }}>
                {authed ? 'Platform console' : 'Sample data'}
              </div>
              <div className="t-xs truncate" style={{ fontSize: 10.5 }}>
                {authed ? 'Operating this installation' : 'Most screens use built-in data'}
              </div>
            </div>
          </div>
        </div>
    </>
  )
}

function RootLayout() {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const authed = useAuthed()
  /* Below md the nav is a drawer rather than a column. It was a fixed 232px
     column at every width, so at 375px it kept two thirds of the screen and
     the page underneath got about 145px — one word per line, and a document
     662px wide inside a 375px viewport. */
  const [navOpen, setNavOpen] = useState(false)
  const wide = useMediaQuery('(min-width: 48em)')
  // Choosing a destination is the end of the drawer's job.
  useEffect(() => { setNavOpen(false) }, [pathname])
  // And a drawer left open while the window grows past md would sit on top
  // of the sidebar it duplicates.
  useEffect(() => { if (wide) setNavOpen(false) }, [wide])

  /* Sign-in renders on its own. Wrapping it in the shell would put a nav
     full of links behind a form whose whole point is that you are not
     through yet. */
  if (pathname === '/signin' || pathname === '/setup') return <Outlet />

  return (
    <div className="flex h-full" style={{ background: 'var(--s-base)' }}>
      <CommandPalette />
      <CreateDrawers />

      {/* Sidebar. --nav-w wide: enough that no label truncates, narrow enough
          that the content column keeps a comfortable measure. It sits on
          --s-nav, which is below the page in dark and above it in light — in
          both schemes the nav and the content are visibly different planes,
          which the old two-point gap between surfaces never achieved. */}
      <aside
        className="hidden shrink-0 flex-col md:flex"
        style={{ width: 'var(--nav-w)', borderRight: '1px solid var(--line)',
          background: 'var(--s-nav)' }}
      >
        <NavContent authed={authed} />
      </aside>

      {/* The same nav below md, as a drawer over the page. Same --s-nav plane
          and same --nav-w width as the column it replaces, and the create
          drawers' scrim, so it reads as the sidebar slid in rather than a new
          surface. Dismissed by the scrim, by Escape, or by going somewhere. */}
      <Drawer
        opened={navOpen}
        onClose={() => setNavOpen(false)}
        position="left"
        size="var(--nav-w)"
        withCloseButton={false}
        overlayProps={{ blur: 2, backgroundOpacity: 0.45, color: 'var(--overlay-tint)' }}
        styles={{
          content: { background: 'var(--s-nav)', display: 'flex', flexDirection: 'column' },
          body: { padding: 0, display: 'flex', flexDirection: 'column', flex: 1, minHeight: 0 },
        }}
      >
        <NavContent authed={authed} />
      </Drawer>

      <div className="flex min-w-0 flex-1 flex-col">
        {/* Left is context — which tenant, and where inside it. Right is
            action. The tenant used to be a small chip fourth from the right,
            among the theme toggle and the account menu; in a console where
            every write lands in exactly one organisation, the answer to
            "which one am I changing?" belongs at the start of the line, not
            filed with the preferences. */}
        <header
          className="flex shrink-0 items-center justify-between gap-2 px-3 md:gap-4 md:px-5"
          style={{ height: 'var(--header-h)', borderBottom: '1px solid var(--line)',
            background: 'var(--s-base)' }}
        >
          <div className="flex min-w-0 items-center gap-2 md:gap-3">
            <Burger opened={navOpen} onClick={() => setNavOpen((o) => !o)}
              size="sm" className="md:hidden" aria-label="Navigation" />
            <TenantPicker />
            {/* Each page opens with its own title, so on a phone the crumb is
                the thing to drop — the tenant it sits beside is not. */}
            <div className="hidden min-w-0 sm:flex"><Breadcrumb /></div>
          </div>
          <div className="flex shrink-0 items-center gap-1.5 md:gap-2">
            <button
              onClick={() => window.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', metaKey: true }))}
              className="flex items-center gap-2 px-2.5"
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
            <ThemeToggle />
            <Account />
          </div>
        </header>

        <main className="min-h-0 flex-1 overflow-y-auto">
          <Outlet />
        </main>
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

const TITLES: Record<string, string> = {
  '/': 'Overview', '/playground': 'Access check', '/identities': 'People',
  '/realms': 'Populations', '/scope': 'Structure', '/grants': 'Access',
  '/roles': 'Roles & permissions', '/memberships': 'Memberships',
  '/audit': 'Audit', '/keys': 'Signing keys',
  '/tenants': 'Tenants', '/signin-page': 'Sign-in & sign-out',
  '/import': 'Import', '/signin': 'Sign in', '/operators': 'Platform users',
  '/catalog': 'Catalog sync',
  '/setup': 'Set up Anubis', '/applications': 'Applications',
}

/* A detail page is two crumbs, not one. TITLES is keyed on the exact
   pathname, so `/identities/01J…` resolved to "Not found" in the header of a
   page that had loaded perfectly well. The name comes out of the query cache
   the page itself fills, which means no second request and no store to keep
   in sync — and it is still right on a cold reload, because the crumb waits
   for the same fetch the page is waiting for. */
function PersonCrumb({ id }: { id: string }) {
  const { data } = useQuery({ queryKey: qk.identity(id), queryFn: () => api.identity(id) })
  return (
    <>
      <Link to="/identities" className="t-xs no-underline shrink-0">People</Link>
      <IconChevronRight size={13} style={{ color: 'var(--ink-4)', flex: 'none' }} />
      <span className="t-body truncate" style={{ fontWeight: 560, maxWidth: 260 }}>
        {data?.username ?? '…'}
      </span>
    </>
  )
}

function Breadcrumb() {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const personId = pathname.startsWith('/identities/')
    ? pathname.slice('/identities/'.length)
    : ''
  /* No "Anubis /" root any more. The wordmark is in the sidebar and the tenant
     sits immediately to the left of this; a crumb whose first segment is the
     product name is a tautology that pushed the only useful segment along. */
  return (
    <div className="flex min-w-0 items-center gap-2">
      <IconChevronRight size={13} style={{ color: 'var(--ink-4)', flex: 'none' }} />
      {personId
        ? <PersonCrumb id={personId} />
        : <span className="t-body truncate" style={{ fontWeight: 560 }}>{TITLES[pathname] ?? 'Not found'}</span>}
    </div>
  )
}
