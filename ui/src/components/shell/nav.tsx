import type { ReactNode } from 'react'
import {
  IconActivity, IconAppWindow, IconBuildingBank, IconHome, IconMapPin, IconSettings,
  IconShieldCheck, IconShieldCog, IconUserSearch, IconUsers,
} from '@tabler/icons-react'

/* The whole map of the console, in one place.

   Fifteen links in six groups became eight in two. Every daily job starts at
   one of six places; the rest moved into Settings or into the page it belongs
   to, as a tab. Nothing was removed: each old screen keeps its address, and
   `match` is what lights up the right link when one of them is open — the
   Groups tab lives at /memberships, so Roles has to answer for that path too.

   The sidebar, the folded rail, the phone bar and its More sheet all render
   from these lists, so the four cannot disagree about what exists. */

export type NavItem = {
  key: string
  to: string
  label: string
  icon: (size: number) => ReactNode
  /** Paths this link answers for, besides its own. */
  match?: string[]
  hint?: string
}

export const DAILY: NavItem[] = [
  { key: 'home', to: '/', label: 'Home', icon: (s) => <IconHome size={s} /> },
  { key: 'people', to: '/identities', label: 'People', icon: (s) => <IconUsers size={s} />,
    match: ['/import', '/bulk'], hint: 'Everyone who can sign in. Open a person to change what they can do' },
  { key: 'roles', to: '/roles', label: 'Roles', icon: (s) => <IconShieldCheck size={s} />,
    match: ['/memberships', '/permissions'], hint: 'Roles, groups and the permissions inside them' },
  { key: 'places', to: '/scope', label: 'Places', icon: (s) => <IconMapPin size={s} />,
    hint: 'Offices, units, product lines and customers that access can be limited to' },
  { key: 'apps', to: '/applications', label: 'Apps', icon: (s) => <IconAppWindow size={s} />,
    match: ['/catalog'], hint: 'The applications people sign in to, and what is inside them' },
  { key: 'check', to: '/playground', label: 'Check access', icon: (s) => <IconUserSearch size={s} />,
    hint: 'Ask "can this person do this?" and see exactly why' },
]

export const SYSTEM: NavItem[] = [
  { key: 'activity', to: '/audit', label: 'Activity', icon: (s) => <IconActivity size={s} />,
    hint: 'Everything that was changed, by whom, and when' },
  { key: 'settings', to: '/settings', label: 'Settings', icon: (s) => <IconSettings size={s} />,
    match: ['/realms', '/signin-page', '/keys'], hint: 'Populations, the sign-in page and signing keys' },
]

/** Only an installation owner sees these: the organisations that exist and who operates them. */
export const PLATFORM: NavItem[] = [
  { key: 'orgs', to: '/tenants', label: 'Organisations', icon: (s) => <IconBuildingBank size={s} />,
    hint: 'Every organisation on this Anubis, with the companies each one owns' },
  { key: 'operators', to: '/operators', label: 'Platform users', icon: (s) => <IconShieldCog size={s} />,
    hint: 'Who operates this installation, and which organisations they may run' },
]

/** The phone bar holds the four most used places; everything else is under More. */
export const PHONE_BAR = ['home', 'people', 'roles', 'places']

export function isActive(item: NavItem, pathname: string): boolean {
  const under = (p: string) => p === '/' ? pathname === '/' : pathname === p || pathname.startsWith(`${p}/`)
  return under(item.to) || (item.match ?? []).some(under)
}

/* Tabs that sit at the top of a page whose old neighbours folded into it.
   Each tab is a real address, so a bookmark to /memberships still opens the
   Groups tab, and Back walks between tabs the way it walks between pages. */
export type SectionTab = { to: string; label: string }

export const SECTIONS = {
  roles: [
    { to: '/roles', label: 'Roles' },
    { to: '/memberships', label: 'Groups' },
    { to: '/permissions', label: 'Permissions' },
  ],
  apps: [
    { to: '/applications', label: 'Apps' },
    { to: '/catalog', label: 'Sync' },
  ],
  settings: [
    { to: '/realms', label: 'Populations' },
    { to: '/signin-page', label: 'Sign-in page' },
    { to: '/keys', label: 'Signing keys' },
  ],
} satisfies Record<string, SectionTab[]>

export type SectionKey = keyof typeof SECTIONS
