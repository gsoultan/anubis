import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Button, Menu, ActionIcon, Modal, Select, TextInput, Tooltip } from '@mantine/core'
import {
  IconBuildingBank, IconDots, IconBrush, IconPlayerPause, IconPlayerPlay,
  IconArrowRight, IconArrowsMove, IconPlus,
} from '@tabler/icons-react'
import { descendants, treeOrder } from '@/lib/tree'
import { useState } from 'react'
import { Page } from '@/components/shell/Page'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/client'
import { useSession } from '@/stores/session'
import { notifyCreated, notifyRejected } from '@/components/create/shell'
import type { Tenant } from '@/lib/api/types'

export const Route = createFileRoute('/tenants')({ component: Tenants })

function TenantCard({ t, onAddInside, onMove }: { t: Tenant; onAddInside: () => void; onMove: () => void }) {
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
            style={{ width: 32, height: 32, background: 'var(--accent-bg)' }}>
            <IconBuildingBank size={16} style={{ color: 'var(--accent)' }} />
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
              <Menu.Item leftSection={<IconPlus size={14} />} onClick={onAddInside}>
                Add a company inside
              </Menu.Item>
              <Menu.Item leftSection={<IconArrowsMove size={14} />} onClick={onMove}>
                Move under another company
              </Menu.Item>
              <Menu.Divider />
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
        style={{ color: 'var(--accent)', fontWeight: 570 }}
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

/* Moving a company: under any company that is not itself or inside it, or
   to the top. Who runs the new parent runs it from then on; access given
   inside the company does not change. */
function MoveTenant({ t, tenants, onClose }: { t: Tenant; tenants: Tenant[]; onClose: () => void }) {
  const below = descendants(tenants, (x) => x.id, (x) => x.parent_id ?? '', t.id)
  const [to, setTo] = useState<string>(t.parent_id ?? '')
  const [busy, setBusy] = useState(false)
  const options = [{ value: '', label: 'Nothing: a company of its own at the top' },
    ...treeOrder(tenants.filter((x) => x.id !== t.id && !below.has(x.id)), (x) => x.id, (x) => x.parent_id ?? '', (x) => x.name)
      .map(({ item, depth }) => ({ value: item.id, label: `${'  '.repeat(depth)}${depth ? '└ ' : ''}${item.name}` }))]
  const move = async () => {
    setBusy(true)
    try {
      await api.moveTenant(t.id, to)
      notifyCreated('Moved', to ? `${t.name} now sits under ${tenants.find((x) => x.id === to)?.name}.` : `${t.name} is now at the top.`)
      await queryClient.invalidateQueries({ queryKey: qk.tenants() })
      await queryClient.invalidateQueries({ queryKey: ['my-tenants'] })
      onClose()
    } catch (e) { notifyRejected(e) }
    setBusy(false)
  }
  return (
    <Modal opened onClose={onClose} title={`Move ${t.name}`} centered>
      <div className="flex flex-col gap-3">
        <Select label="Under" data={options} value={to} onChange={(v) => setTo(v ?? '')} searchable allowDeselect={false} />
        <div className="t-xs">
          Whoever runs the company above will run {t.name} too. People, places and access inside {t.name} stay as they are,
          and nobody gets access across companies by this.
        </div>
        <div className="flex justify-end gap-2">
          <Button variant="default" size="xs" onClick={onClose}>Cancel</Button>
          <Button size="xs" loading={busy} disabled={to === (t.parent_id ?? '')} onClick={() => void move()}>Move</Button>
        </div>
      </div>
    </Modal>
  )
}

function Tenants() {
  const { data: tenants } = useQuery({ queryKey: qk.tenants(), queryFn: api.tenants })
  const [name, setName] = useState('')
  const [inside, setInside] = useState<Tenant | null>(null)
  const [moving, setMoving] = useState<Tenant | null>(null)
  const [busy, setBusy] = useState(false)
  const slug = slugify(name)
  const ready = name.trim().length >= 2 && slug.length >= 2

  const add = async () => {
    if (name.trim().length < 2) return
    setBusy(true)
    try {
      const t = await api.createTenant({ name: name.trim(), ...(inside ? { parentId: inside.id } : {}) })
      notifyCreated('Company created', inside
        ? `${t.name} sits under ${inside.name}. Whoever runs ${inside.name} runs it too.`
        : `${t.name} is live and empty. Design its sign-in page, then add its first admin.`)
      await queryClient.invalidateQueries({ queryKey: qk.tenants() })
      setName(''); setInside(null)
    } catch (e) { notifyRejected(e) }
    setBusy(false)
  }

  return (
    <Page
      title="Organisations"
      description="Every company on this Anubis, with the companies each one owns. Whoever runs a company runs the ones inside it; people, places and access never cross from one company to another."
      actions={
        <>
          <div className="flex flex-col">
            {inside && (
              <span className="t-xs mb-1 flex items-center gap-1.5">
                Inside <b>{inside.name}</b>
                <button type="button" className="t-xs" style={{ color: 'var(--accent)' }} onClick={() => setInside(null)}>at the top instead</button>
              </span>
            )}
            <TextInput w={240} placeholder={inside ? `New company in ${inside.name}…` : 'New company name…'} value={name}
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
                {inside ? 'Create inside' : 'Create company'}
              </Button>
            </div>
          </Tooltip>
        </>
      }
    >
      {/* As a tree: a company directly under the one that owns it. A grid of
          equal cards made a subsidiary look like a stranger. */}
      <div className="flex max-w-[860px] flex-col gap-3">
        {treeOrder(tenants ?? [], (t) => t.id, (t) => t.parent_id ?? '', (t) => t.name).map(({ item: t, depth }) => (
          <div key={t.id} style={{ marginLeft: Math.min(depth, 4) * 20, borderLeft: depth ? '2px solid var(--line-soft)' : undefined, paddingLeft: depth ? 12 : 0 }}>
            <TenantCard t={t} onAddInside={() => setInside(t)} onMove={() => setMoving(t)} />
          </div>
        ))}
      </div>
      {moving && <MoveTenant t={moving} tenants={tenants ?? []} onClose={() => setMoving(null)} />}
    </Page>
  )
}
