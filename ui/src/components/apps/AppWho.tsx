import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button, UnstyledButton } from '@mantine/core'
import { IconAlertTriangle, IconChevronRight, IconUserPlus, IconUsersGroup } from '@tabler/icons-react'
import { Link } from '@tanstack/react-router'
import { GiveAccessSheet } from '@/components/access/GiveAccessSheet'
import { RoleHolders } from '@/components/access/RoleHolders'
import { notifyCreated, notifyRejected } from '@/components/create/shell'
import { SignInChoice, type SignIn } from './SignInChoice'
import { api } from '@/lib/api/client'
import { shortRoleName } from '@/lib/access'
import { qk } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/client'
import * as live from '@/lib/api/live'
import type { Role } from '@/lib/api/types'

const people = (n: number) => (n === 0 ? 'Nobody' : `${n} ${n === 1 ? 'person' : 'people'}`)

/** The roles an app owns that can still be given. */
export function useAppRoles(appSlug: string) {
  const { data: roles } = useQuery({ queryKey: qk.roles(), queryFn: api.roles })
  const own = useMemo(() => (roles ?? []).filter((r) => r.application_id === appSlug && !r.deprecated), [roles, appSlug])
  const { data: usage } = useQuery({
    queryKey: ['grants', 'role-usage', own.map((r) => r.id).join()],
    queryFn: () => live.roleUsage(own.map((r) => r.id)),
    enabled: own.length > 0,
  })
  const count = (id: string) => usage?.find((u) => u.role_id === id)?.people ?? 0
  return { roles: own, loaded: roles !== undefined, count }
}

/* The app's front door: who is let in at all, the ways in, and how many
   people that comes to today. Anything odd about it goes in Needs a look,
   so it is seen without opening each role. */
export function AppWho({ app }: { app: live.AppRecord }) {
  const { roles, count } = useAppRoles(app.slug)
  const { data: groups } = useQuery({ queryKey: qk.memberships(), queryFn: api.memberships })
  const { data: usage } = useQuery({ queryKey: ['grants', 'app-usage', app.id], queryFn: () => live.appUsage(app.id) })
  const [holdersOf, setHoldersOf] = useState<Role | null>(null)
  const [giving, setGiving] = useState(false)
  const [busy, setBusy] = useState(false)

  const ids = new Set(roles.map((r) => r.id))
  const appGroups = (groups ?? []).filter((g) => g.entries.some((e) => ids.has(e.role_id)))

  async function setSignIn(v: SignIn) {
    setBusy(true)
    try {
      await live.updateApplication({ ...app, sign_in: v })
      notifyCreated('Saved', v === 'anyone' ? `Anyone with an account can sign in to ${app.name}.` : `Only people with a role in ${app.name} can sign in.`)
      await queryClient.invalidateQueries({ queryKey: ['applications'] })
    } catch (e) { notifyRejected(e) } finally { setBusy(false) }
  }

  const looks: string[] = [
    ...roles.filter((r) => r.permission_count === 0).map((r) => `${shortRoleName(r)} gives nothing yet: it has no permissions`),
    ...roles.filter((r) => r.permission_count > 0 && count(r.id) === 0).map((r) => `Nobody has ${shortRoleName(r)} yet`),
    ...(app.sign_in === 'anyone' && roles.length > 0 ? [`Anyone with an account can sign in, even without a role in ${app.name}`] : []),
  ]

  return (
    <div className="grid min-w-0 gap-4 lg:grid-cols-[minmax(0,1fr)_320px]">
      <div className="flex min-w-0 flex-col gap-4">
        <section className="panel p-4" aria-busy={busy}>
          <SignInChoice value={app.sign_in} onChange={(v) => void setSignIn(v)} appName={app.name} />
        </section>

        <section className="panel p-4">
          <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
            <h2 className="t-h2">How people get in</h2>
            <Button size="xs" leftSection={<IconUserPlus size={14} />} onClick={() => setGiving(true)}>Give someone a role</Button>
          </div>
          {roles.length === 0 && (
            <div className="t-sm">
              {app.name} has no roles yet. Roles come from its manifest, or <Link to="/roles" style={{ color: 'var(--accent)' }}>add one</Link>.
            </div>
          )}
          <div className="flex flex-col">
            {roles.map((r) => (
              <UnstyledButton key={r.id} onClick={() => setHoldersOf(r)}
                className="flex items-center gap-3 px-1 py-2.5" style={{ borderBottom: '1px solid var(--line-soft)' }}>
                <span className="min-w-0 flex-1">
                  <span className="t-body block" style={{ fontWeight: 550 }}>{shortRoleName(r)}</span>
                  <span className="t-xs block">
                    {r.description ? `${r.description} · ` : ''}
                    {r.permission_count === 0 ? 'No permissions yet' : `${r.permission_count} ${r.permission_count === 1 ? 'permission' : 'permissions'}`}
                  </span>
                </span>
                <span className="t-sm tnum shrink-0">{people(count(r.id))}</span>
                <IconChevronRight size={14} style={{ color: 'var(--ink-3)' }} />
              </UnstyledButton>
            ))}
            {appGroups.map((g) => (
              <Link key={g.id} to="/memberships" className="flex items-center gap-3 px-1 py-2.5 no-underline"
                style={{ borderBottom: '1px solid var(--line-soft)', color: 'inherit' }}>
                <IconUsersGroup size={16} style={{ color: 'var(--ink-3)' }} />
                <span className="min-w-0 flex-1">
                  <span className="t-body block" style={{ fontWeight: 550 }}>{g.name} <span className="t-xs">(group)</span></span>
                  <span className="t-xs block">
                    Gives {[...new Set(g.entries.filter((e) => ids.has(e.role_id)).map((e) => shortRoleName({ name: e.role_name, application_id: app.slug })))].join(' and ')}
                  </span>
                </span>
                <span className="t-sm tnum shrink-0">{people(g.member_count ?? g.member_ids.length)}</span>
                <IconChevronRight size={14} style={{ color: 'var(--ink-3)' }} />
              </Link>
            ))}
          </div>
        </section>
      </div>

      <aside className="flex flex-col gap-4">
        <section className="panel p-4">
          <div className="t-label mb-1">Can use {app.name} today</div>
          <div className="t-display tnum">{usage ? people(usage.people) : '…'}</div>
          {usage && usage.people > 0 && (
            <div className="t-xs mt-1">
              {usage.direct} through a role{usage.via_groups > 0 ? `, ${usage.via_groups} through a group` : ''}
            </div>
          )}
          {app.sign_in === 'anyone' && <div className="t-xs mt-2">Everyone else with an account can also sign in, with nothing to do inside.</div>}
        </section>
        {looks.length > 0 && (
          <section className="panel p-4">
            <div className="t-label mb-2">Needs a look</div>
            <ul className="flex flex-col gap-2">
              {looks.map((l) => (
                <li key={l} className="t-sm flex items-start gap-2">
                  <IconAlertTriangle size={14} style={{ color: 'var(--warn)', marginTop: 2, flexShrink: 0 }} />{l}
                </li>
              ))}
            </ul>
          </section>
        )}
      </aside>

      <RoleHolders role={holdersOf} onClose={() => setHoldersOf(null)} />
      <GiveAccessSheet opened={giving} onClose={() => setGiving(false)} pickSubject />
    </div>
  )
}
