import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Button, Menu, ActionIcon, TextInput, Tooltip } from '@mantine/core'
import {
  IconBuildingBank, IconDots, IconBrush, IconPlayerPause, IconPlayerPlay,
  IconArrowRight, IconPlus,
} from '@tabler/icons-react'
import { useState } from 'react'
import { Page } from '@/components/shell/Page'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/client'
import { useSession } from '@/stores/session'
import { notifyCreated, notifyRejected } from '@/components/create/shell'
import type { Tenant } from '@/lib/api/types'

export const Route = createFileRoute('/tenants')({ component: Tenants })

function TenantCard({ t }: { t: Tenant }) {
  const { setCurrentTenant } = useSession()
  const navigate = useNavigate()
  const { data: stats } = useQuery({
    queryKey: qk.tenantStats(t.id), queryFn: () => api.tenantStats(t.id),
  })
  const suspended = t.status === 'suspended'

  const toggle = async () => {
    try {
      await api.setTenantStatus(t.id, suspended ? 'active' : 'suspended')
      notifyCreated(suspended ? 'Tenant reactivated' : 'Tenant suspended',
        suspended ? `${t.name} can sign in again.` : `${t.name}: sign-ins refuse from now; data is intact.`)
      await queryClient.invalidateQueries({ queryKey: qk.tenants() })
    } catch (e) { notifyRejected(e) }
  }

  return (
    <div className="panel p-4" style={suspended ? { opacity: 0.65 } : undefined}>
      <div className="mb-1 flex items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2.5">
          <div className="flex shrink-0 items-center justify-center rounded-lg"
            style={{ width: 32, height: 32, background: 'var(--gold-glow)' }}>
            <IconBuildingBank size={16} style={{ color: 'var(--gold)' }} />
          </div>
          <div className="min-w-0">
            <div className="t-h2 truncate">{t.name}</div>
            <span className="chip">{t.slug}</span>
          </div>
        </div>
        <div className="flex items-center gap-1.5">
          {suspended && <span className="v-pill v-pill-deny">suspended</span>}
          <Menu position="bottom-end" width={240} shadow="xl">
            <Menu.Target>
              <ActionIcon variant="subtle" color="gray" aria-label={`Actions for ${t.name}`}>
                <IconDots size={15} />
              </ActionIcon>
            </Menu.Target>
            <Menu.Dropdown>
              <Menu.Item leftSection={<IconBrush size={14} />}
                onClick={() => void navigate({ to: '/signin-page', search: { tenant: t.id } })}>
                Design sign-in page
              </Menu.Item>
              {suspended ? (
                <Menu.Item color="teal" leftSection={<IconPlayerPlay size={14} />} onClick={() => void toggle()}>
                  Reactivate
                </Menu.Item>
              ) : (
                <Menu.Item color="red" leftSection={<IconPlayerPause size={14} />} onClick={() => void toggle()}>
                  Suspend — blocks sign-in, keeps data
                </Menu.Item>
              )}
            </Menu.Dropdown>
          </Menu>
        </div>
      </div>

      <div className="mt-3 grid grid-cols-4 gap-2">
        {([['people', stats?.identities], ['access', stats?.grants],
           ['structure', stats?.scope_nodes], ['bundles', stats?.memberships]] as const)
          .map(([label, v]) => (
          <div key={label} className="panel-inset px-2.5 py-2 text-center">
            <div className="tnum" style={{ fontSize: 16, fontWeight: 620 }}>{v ?? '—'}</div>
            <div className="t-label" style={{ fontSize: 9 }}>{label}</div>
          </div>
        ))}
      </div>

      <button
        className="t-body mt-3 inline-flex items-center gap-1.5"
        style={{ color: 'var(--gold)', fontWeight: 570 }}
        disabled={suspended}
        onClick={() => setCurrentTenant(t.id)}
      >
        Manage this tenant <IconArrowRight size={13} />
      </button>
    </div>
  )
}

/** Mirrors the slug the seam derives, so what is shown is what gets created. */
function slugify(nameInput: string): string {
  return nameInput.toLowerCase().trim().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '').slice(0, 63)
}

function Tenants() {
  const { data: tenants } = useQuery({ queryKey: qk.tenants(), queryFn: api.tenants })
  const [name, setName] = useState('')
  const [busy, setBusy] = useState(false)
  const slug = slugify(name)
  const ready = name.trim().length >= 2 && slug.length >= 2

  const add = async () => {
    if (name.trim().length < 2) return
    setBusy(true)
    try {
      const t = await api.createTenant({ name: name.trim() })
      notifyCreated('Tenant created',
        `${t.name} is live and empty — design its sign-in page, then add its first admin.`)
      await queryClient.invalidateQueries({ queryKey: qk.tenants() })
      setName('')
    } catch (e) { notifyRejected(e) }
    setBusy(false)
  }

  return (
    <Page
      title="Tenants"
      description="Every organisation on this Anubis. Each is a sealed universe — people, structures and access in one tenant can never reference another; the schema makes it impossible, not just impolite."
      actions={
        <>
          <div className="flex flex-col">
            <TextInput w={240} placeholder="New tenant name…" value={name}
              onChange={(e) => setName(e.currentTarget.value)}
              onKeyDown={(e) => { if (e.key === 'Enter') void add() }} />
            {/* The slug is derived once and never editable: it appears in
                URLs, tokens and every hosted page path, so showing it before
                the tenant exists beats discovering it afterwards. */}
            {slug && (
              <span className="t-xs" style={{ marginTop: 3 }}>
                URL slug: <span className="chip">{slug}</span>
              </span>
            )}
          </div>
          <Tooltip label="Enter a name of at least two characters" disabled={ready}>
            <div>
              <Button size="xs" leftSection={<IconPlus size={13} />} loading={busy}
                disabled={!ready} onClick={() => void add()}>
                Create tenant
              </Button>
            </div>
          </Tooltip>
        </>
      }
    >
      <div className="grid gap-4" style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(340px, 1fr))' }}>
        {tenants?.map((t) => <TenantCard key={t.id} t={t} />)}
      </div>
    </Page>
  )
}
