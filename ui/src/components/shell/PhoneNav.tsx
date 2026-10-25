import { useEffect, useState } from 'react'
import { Link, useNavigate, useRouterState } from '@tanstack/react-router'
import { Drawer, UnstyledButton, useComputedColorScheme, useMantineColorScheme } from '@mantine/core'
import { IconChevronRight, IconDots, IconLogout, IconMoon, IconSun } from '@tabler/icons-react'
import { signOut, useIsOwner } from '@/stores/auth'
import { DAILY, PHONE_BAR, PLATFORM, SYSTEM, isActive, type NavItem } from './nav'

/* Below md the sidebar becomes a bar along the bottom: Home, People, Roles,
   Places, and More for the rest. A thumb reaches the bottom of a phone; it
   does not reach a hamburger in the top corner, and a drawer that has to be
   opened before every move is two taps for what should be one. */

function MoreRow({ item, onGo }: { item: NavItem; onGo: () => void }) {
  return (
    <Link to={item.to} onClick={onGo} className="more-row no-underline">
      <span className="nav-icon">{item.icon(18)}</span>
      <span className="flex-1">{item.label}</span>
      <IconChevronRight size={16} style={{ color: 'var(--ink-4)' }} />
    </Link>
  )
}

export function PhoneNav() {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const owner = useIsOwner()
  const navigate = useNavigate()
  const [more, setMore] = useState(false)
  const { setColorScheme } = useMantineColorScheme()
  const scheme = useComputedColorScheme('light')
  useEffect(() => { setMore(false) }, [pathname])

  const bar = DAILY.filter((i) => PHONE_BAR.includes(i.key))
  const rest = [...DAILY.filter((i) => !PHONE_BAR.includes(i.key)), ...SYSTEM]
  const inMore = [...rest, ...(owner ? PLATFORM : [])].some((i) => isActive(i, pathname))

  return (
    <>
      <nav aria-label="Main" className="phone-bar md:hidden">
        {bar.map((it) => {
          const on = isActive(it, pathname)
          return (
            <Link key={it.key} to={it.to} className="phone-tab no-underline"
              {...(on ? { 'data-active': '', 'aria-current': 'page' as const } : {})}>
              {it.icon(20)}
              <span>{it.label}</span>
            </Link>
          )
        })}
        <UnstyledButton className="phone-tab" onClick={() => setMore(true)} aria-haspopup="dialog"
          {...(inMore ? { 'data-active': '' } : {})}>
          <IconDots size={20} />
          <span>More</span>
        </UnstyledButton>
      </nav>

      <Drawer opened={more} onClose={() => setMore(false)} position="bottom" size="auto"
        title="More" radius="lg"
        overlayProps={{ blur: 2, backgroundOpacity: 0.45, color: 'var(--overlay-tint)' }}
        styles={{ content: { background: 'var(--s-raised)' }, header: { background: 'var(--s-raised)' } }}>
        <div className="flex flex-col gap-4 pb-4">
          <div className="more-group">
            {rest.map((it) => <MoreRow key={it.key} item={it} onGo={() => setMore(false)} />)}
          </div>
          {owner && (
            <div>
              <div className="t-label px-1 pb-2">Platform</div>
              <div className="more-group">
                {PLATFORM.map((it) => <MoreRow key={it.key} item={it} onGo={() => setMore(false)} />)}
              </div>
            </div>
          )}
          <div className="more-group">
            <UnstyledButton className="more-row" onClick={() => setColorScheme(scheme === 'dark' ? 'light' : 'dark')}>
              <span className="nav-icon">{scheme === 'dark' ? <IconSun size={18} /> : <IconMoon size={18} />}</span>
              <span className="flex-1">{scheme === 'dark' ? 'Light theme' : 'Dark theme'}</span>
            </UnstyledButton>
            <UnstyledButton className="more-row" style={{ color: 'var(--deny)' }}
              onClick={async () => { await signOut(); navigate({ to: '/signin', search: { next: '/' } }) }}>
              <span className="nav-icon" style={{ color: 'var(--deny)' }}><IconLogout size={18} /></span>
              <span className="flex-1">Sign out</span>
            </UnstyledButton>
          </div>
        </div>
      </Drawer>
    </>
  )
}
