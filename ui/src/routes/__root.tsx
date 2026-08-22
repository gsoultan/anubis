import { createRootRoute, Link, Outlet, useRouterState } from '@tanstack/react-router'
import { Tooltip } from '@mantine/core'
import {
  IconLayoutDashboard, IconUsers, IconSitemap, IconKey, IconShieldCheck,
  IconFileDescription, IconAffiliate, IconWorld, IconTestPipe, IconSearch,
  IconPointFilled,
} from '@tabler/icons-react'
import { ActionIcon, Button, Menu, useComputedColorScheme, useMantineColorScheme } from '@mantine/core'
import {
  IconPlus, IconUserPlus, IconLicense, IconShieldPlus, IconCirclePlus,
  IconSitemapFilled, IconAxisY, IconSun, IconMoon, IconUsersGroup,
} from '@tabler/icons-react'
import type { ReactNode } from 'react'
import { CommandPalette } from '@/components/shell/CommandPalette'
import { CreateDrawers } from '@/components/create'
import { useCreate } from '@/stores/create'
import { useSession } from '@/stores/session'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { IconBuildingBank, IconChevronDown, IconBrush, IconCheck } from '@tabler/icons-react'

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
      { to: '/signin-page', label: 'Sign-in pages', icon: <IconBrush size={15} />,
        hint: 'Brand each tenant’s login page' },
    ],
  },
  {
    title: 'System',
    items: [
      { to: '/keys', label: 'Signing keys', icon: <IconKey size={15} /> },
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
      <span style={{ color: active ? 'var(--gold)' : 'var(--ink-3)', display: 'flex' }}>{icon}</span>
      {label}
    </Link>
  )
  return hint ? <Tooltip label={hint} position="right" openDelay={600}>{el}</Tooltip> : el
}

function WorkspaceSwitcher() {
  const { currentTenantId, setCurrentTenant } = useSession()
  const { data: tenants } = useQuery({ queryKey: qk.tenants(), queryFn: api.tenants })
  const current = tenants?.find((t) => t.id === currentTenantId)
  return (
    <Menu position="bottom-start" width={230} shadow="xl">
      <Menu.Target>
        <button className="panel-inset mx-3 mb-1 flex items-center justify-between gap-2 px-2.5 py-2 text-left"
          style={{ width: 'calc(100% - 24px)' }}>
          <span className="flex min-w-0 items-center gap-2">
            <IconBuildingBank size={14} style={{ color: 'var(--gold)', flexShrink: 0 }} />
            <span className="t-body truncate" style={{ fontWeight: 570 }}>
              {currentTenantId === null ? 'Platform' : current?.name ?? '…'}
            </span>
          </span>
          <IconChevronDown size={13} style={{ color: 'var(--ink-3)', flexShrink: 0 }} />
        </button>
      </Menu.Target>
      <Menu.Dropdown>
        <Menu.Label>Workspace</Menu.Label>
        <Menu.Item leftSection={<IconBuildingBank size={14} />}
          rightSection={currentTenantId === null ? <IconCheck size={13} /> : null}
          onClick={() => setCurrentTenant(null)}>
          Platform <span className="t-xs block">tenants, keys — the super-admin view</span>
        </Menu.Item>
        <Menu.Divider />
        {tenants?.map((t) => (
          <Menu.Item key={t.id}
            rightSection={currentTenantId === t.id ? <IconCheck size={13} /> : null}
            disabled={t.status === 'suspended'}
            onClick={() => setCurrentTenant(t.id)}>
            {t.name}
            <span className="t-xs block">{t.status === 'suspended' ? 'suspended' : `manage ${t.slug}`}</span>
          </Menu.Item>
        ))}
      </Menu.Dropdown>
    </Menu>
  )
}

function Jackal() {
  return (
    <svg viewBox="0 0 24 24" width={20} height={20} aria-hidden>
      <defs>
        <linearGradient id="jk" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="#e8bc55" /><stop offset="100%" stopColor="#b6801d" />
        </linearGradient>
      </defs>
      <path d="M4 3l3.2 5.4A8.6 8.6 0 0 1 12 7c1.7 0 3.3.5 4.8 1.4L20 3l.9 7.4c.3 2.6-.6 5.2-2.5 7L12 22l-6.4-4.6c-1.9-1.8-2.8-4.4-2.5-7L4 3z"
        fill="url(#jk)" />
      <circle cx="9.4" cy="12.6" r="1.05" fill="#0a0b0e" />
      <circle cx="14.6" cy="12.6" r="1.05" fill="#0a0b0e" />
    </svg>
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
    { kind: 'permission' as const, label: 'Permission', icon: <IconLicense size={15} />, hint: 'one action in one app' },
    { kind: 'node' as const, label: 'Structure item', icon: <IconSitemapFilled size={15} />, hint: 'an office, product line, customer…' },
    { kind: 'axis' as const, label: 'Structure', icon: <IconAxisY size={15} />, hint: 'a whole new way to limit access' },
  ]
  return (
    <Menu position="bottom-end" width={280} shadow="xl">
      <Menu.Target>
        <Button size="xs" leftSection={<IconPlus size={14} />}>Add</Button>
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
      variant="default" size={30} aria-label={`Switch to ${next} theme`}
      onClick={() => setColorScheme(next)}
    >
      {computed === 'dark' ? <IconSun size={15} /> : <IconMoon size={15} />}
    </ActionIcon>
  )
}

function SideNav() {
  const currentTenantId = useSession((st) => st.currentTenantId)
  const groups = currentTenantId === null ? PLATFORM_GROUPS : GROUPS
  return (
    <nav className="flex-1 overflow-y-auto px-3 pb-3">
      {groups.map((g, gi) => (
        <div key={g.title ?? gi} className={gi === 0 ? '' : 'mt-4'}>
          {g.title && <div className="t-label px-2.5 pb-1.5">{g.title}</div>}
          <div className="flex flex-col gap-0.5">
            {g.items.map((n) => <NavItem key={n.to} {...n} />)}
          </div>
        </div>
      ))}
    </nav>
  )
}

export const Route = createRootRoute({
  component: () => (
    <div className="flex h-full" style={{ background: 'var(--s-base)' }}>
      <CommandPalette />
      <CreateDrawers />

      {/* Sidebar. Fixed 216px: wide enough that no label truncates, narrow
          enough that the content column keeps a comfortable measure. */}
      <aside
        className="flex w-[216px] shrink-0 flex-col"
        style={{ borderRight: '1px solid var(--line)', background: 'var(--s-sunken)' }}
      >
        <div className="flex items-center gap-2.5 px-4" style={{ height: 52 }}>
          <Jackal />
          <div className="leading-none">
            <div style={{ fontSize: 14, fontWeight: 650, letterSpacing: '-.02em' }}>Anubis</div>
            <div className="t-xs" style={{ marginTop: 2 }}>console</div>
          </div>
        </div>

        <WorkspaceSwitcher />
        <SideNav />
        <div className="px-3 pb-3">
          <div className="panel-inset flex items-center gap-2 px-2.5 py-2">
            <IconPointFilled size={11} style={{ color: 'var(--warn)' }} />
            <div className="min-w-0">
              <div className="t-xs" style={{ color: 'var(--ink-2)', fontWeight: 550 }}>
                Mock backend
              </div>
              <div className="t-xs" style={{ fontSize: 10 }}>API not built yet</div>
            </div>
          </div>
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header
          className="flex shrink-0 items-center justify-between gap-4 px-6"
          style={{ height: 52, borderBottom: '1px solid var(--line)' }}
        >
          <Breadcrumb />
          <div className="flex items-center gap-3">
            <button
              onClick={() => window.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', metaKey: true }))}
              className="panel-inset flex items-center gap-2 px-2.5 py-1.5"
              style={{ transition: 'border-color var(--t-fast)' }}
            >
              <IconSearch size={13} style={{ color: 'var(--ink-3)' }} />
              <span className="t-xs" style={{ minWidth: 92, textAlign: 'left' }}>Search…</span>
              <kbd className="chip" style={{ fontSize: 9.5 }}>⌘K</kbd>
            </button>
            <NewMenu />
            <ThemeToggle />
            <div className="chip chip-gold">impack</div>
          </div>
        </header>

        <main className="min-h-0 flex-1 overflow-y-auto">
          <Outlet />
        </main>
      </div>
    </div>
  ),
})

const TITLES: Record<string, string> = {
  '/': 'Overview', '/playground': 'Access check', '/identities': 'People',
  '/realms': 'Populations', '/scope': 'Structure', '/grants': 'Access',
  '/roles': 'Roles & permissions', '/memberships': 'Memberships',
  '/audit': 'Audit', '/keys': 'Signing keys',
  '/tenants': 'Tenants', '/signin-page': 'Sign-in pages',
}

function Breadcrumb() {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  return (
    <div className="flex items-center gap-2">
      <span className="t-xs">Anubis</span>
      <span style={{ color: 'var(--ink-4)' }}>/</span>
      <span className="t-body" style={{ fontWeight: 550 }}>{TITLES[pathname] ?? 'Not found'}</span>
    </div>
  )
}
