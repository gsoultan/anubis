import { Link, useNavigate, useRouterState } from '@tanstack/react-router'
import { ActionIcon, Button, Menu, Tooltip, useComputedColorScheme, useMantineColorScheme } from '@mantine/core'
import {
  IconBuildingBank, IconCheck, IconChevronDown, IconChevronRight, IconCirclePlus, IconKey,
  IconLicense, IconLogout, IconMoon, IconPlus, IconShieldPlus, IconSitemapFilled, IconAxisY,
  IconSun, IconUserCircle, IconUserPlus, IconUsersGroup,
} from '@tabler/icons-react'
import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useCreate } from '@/stores/create'
import { ChangePassword } from './ChangePassword'
import { signOut, useAuthed, useWho } from '@/stores/auth'
import { currentTenant, myTenants, setCurrentTenant } from '@/lib/anubis'
import { treeOrder } from '@/lib/tree'
import { queryClient } from '@/lib/query/client'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'

/* What sits in the bar along the top: which organisation, where inside it,
   and the actions that work from anywhere. Split out of the root route so the
   shell's layout reads as a layout. */

/* The global create menu is the single biggest usability fix: every object in
   the system is creatable from anywhere, in two clicks, without first finding
   the right page. */
export function NewMenu() {
  const { openCreate } = useCreate()
  const items = [
    { kind: 'identity' as const, label: 'Person', icon: <IconUserPlus size={15} />, hint: 'an employee, a partner contact or a customer' },
    { kind: 'grant' as const, label: 'Access for someone', icon: <IconCirclePlus size={15} />, hint: 'what they can do, and where' },
    { kind: 'membership' as const, label: 'Group', icon: <IconUsersGroup size={15} />, hint: 'several roles handed to everyone in it' },
    { kind: 'role' as const, label: 'Role', icon: <IconShieldPlus size={15} />, hint: 'a named set of permissions' },
    { kind: 'node' as const, label: 'Place', icon: <IconSitemapFilled size={15} />, hint: 'an office, unit, product line or customer' },
    { kind: 'axis' as const, label: 'Kind of place', icon: <IconAxisY size={15} />, hint: 'a new way to limit access, such as cost centres' },
    { kind: 'permission' as const, label: 'Permission', icon: <IconLicense size={15} />, hint: 'comes from its app: see how' },
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

export function ThemeToggle() {
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

/* The tenant this operator is administering.

   Defaults to the first one they are assigned to, so somebody with a single
   tenant never has to choose. The selection is context, not authority: it
   rides along with every request and the server checks it against their
   assignments each time. */
export function TenantPicker() {
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
    <Menu position="bottom-start" width={280}>
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
        {/* As the tree it is: a company under its parent, indented. A
            company someone covers because they run its parent says so, so
            nobody wonders where the access came from. */}
        {treeOrder(tenants, (t) => t.id, (t) => t.parentId, (t) => t.name).map(({ item: t, depth }) => (
          <Menu.Item key={t.slug}
            rightSection={t.slug === current ? <IconCheck size={13} /> : null}
            style={{ paddingLeft: 12 + depth * 14 }}
            onClick={() => {
              setCurrentTenant(t.slug)
              setCurrent(t.slug)
              // Everything on screen was fetched for the previous tenant.
              queryClient.clear()
            }}>
            {t.name}
            <span className="t-xs block">{t.inherited ? 'through the company above' : t.role}</span>
          </Menu.Item>
        ))}
      </Menu.Dropdown>
    </Menu>
  )
}

/** The account control. The tenant chip used to be hardcoded to "impack";
    once a screen could really sign in, a chip that names a tenant nobody is
    signed into is worse than no chip at all. */
export function Account() {
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

const TITLES: Record<string, string> = {
  '/': 'Home', '/playground': 'Check access', '/identities': 'People',
  '/realms': 'Settings', '/scope': 'Places', '/settings': 'Settings',
  '/roles': 'Roles', '/memberships': 'Roles', '/permissions': 'Roles',
  '/audit': 'Activity', '/keys': 'Settings', '/bulk': 'People',
  '/tenants': 'Organisations', '/signin-page': 'Settings',
  '/import': 'People', '/signin': 'Sign in', '/operators': 'Platform users',
  '/catalog': 'Apps',
  '/setup': 'Set up Anubis', '/applications': 'Apps',
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

export function Breadcrumb() {
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
        : <span className="t-body truncate" style={{ fontWeight: 560 }}>{TITLES[pathname] ?? TITLES[`/${pathname.split('/')[1]}`] ?? 'Not found'}</span>}
    </div>
  )
}
