import { createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { ActionIcon, TextInput, Tooltip } from '@mantine/core'
import {
  IconInfoCircle, IconShieldLock, IconClock, IconTrash, IconUserPlus, IconLock,
  IconPlus,
} from '@tabler/icons-react'
import { useState } from 'react'
import { queryClient } from '@/lib/query/client'
import { notifyCreated, notifyRejected } from '@/components/create/shell'
import { Page } from '@/components/shell/Page'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import type { Realm } from '@/lib/api/types'

export const Route = createFileRoute('/realms')({ component: Realms })

const KIND = {
  internal: { colour: 'var(--gold)', tint: 'color-mix(in srgb, var(--gold) 8%, transparent)' },
  partner:  { colour: 'var(--info)', tint: 'color-mix(in srgb, var(--info) 8%, transparent)' },
  public:   { colour: 'var(--grape)',     tint: 'color-mix(in srgb, var(--grape) 8%, transparent)' },
  service:  { colour: 'var(--ink-3)', tint: '#ffffff08' },
} as const

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-3 py-1.5">
      <span className="t-xs" style={{ paddingTop: 1 }}>{label}</span>
      <div className="flex flex-wrap justify-end gap-1">{children}</div>
    </div>
  )
}

/* Categories are rows the tenant extends at runtime — "Public can be anything"
   is the requirement, so the add affordance lives right on the card. */
function Categories({ r }: { r: Realm }) {
  const { data: cats } = useQuery({
    queryKey: qk.realmCategories(r.id),
    queryFn: () => api.realmCategories(r.id),
  })
  const [name, setName] = useState('')
  const [busy, setBusy] = useState(false)

  async function add() {
    if (name.trim().length < 2) return
    setBusy(true)
    try {
      const c = await api.createRealmCategory({ realm_id: r.id, display_name: name.trim() })
      notifyCreated('Category added', `${c.display_name} in ${r.display_name} — usable immediately.`)
      await queryClient.invalidateQueries({ queryKey: ['realm-categories'] })
      setName('')
    } catch (e) { notifyRejected(e) }
    setBusy(false)
  }

  return (
    <div style={{ borderTop: '1px solid var(--line-soft)' }} className="px-4 py-3">
      <div className="t-label mb-2">Categories</div>
      <div className="flex flex-wrap gap-1.5">
        {(cats ?? []).map((c) => (
          <span key={c.id} className="chip">
            {c.display_name}
            <span style={{ marginLeft: 5, color: 'var(--ink-4)' }} className="tnum">
              {c.identity_count.toLocaleString()}
            </span>
          </span>
        ))}
        {(cats?.length ?? 0) === 0 && (
          <span className="t-xs">None yet — add one to group this population.</span>
        )}
      </div>
      <div className="mt-2.5 flex items-center gap-1.5">
        <TextInput
          size="xs" placeholder="Add a category…" value={name}
          onChange={(e) => setName(e.currentTarget.value)}
          onKeyDown={(e) => { if (e.key === 'Enter') void add() }}
          style={{ flex: 1 }}
        />
        <ActionIcon size={30} variant="default" loading={busy} aria-label="Add category"
          onClick={() => void add()} disabled={name.trim().length < 2}>
          <IconPlus size={14} />
        </ActionIcon>
      </div>
    </div>
  )
}

function RealmCard({ r }: { r: Realm }) {
  const k = KIND[r.kind]
  return (
    <div className="panel panel-hover overflow-hidden">
      <div className="px-4 py-3.5" style={{ background: k.tint, borderBottom: '1px solid var(--line-soft)' }}>
        <div className="flex items-center justify-between gap-2">
          <span className="t-h1">{r.display_name}</span>
          <span className="chip" style={{ color: k.colour, borderColor: `${k.colour}33` }}>{r.kind}</span>
        </div>
        <div className="t-xs mt-1 font-mono">{r.code}</div>
      </div>

      <div className="px-4 pb-1.5 pt-2.5">
        <Row label="assurance floor">
          <Tooltip label="Identities below this level cannot be created in this realm.">
            <span className="chip" style={{ color: k.colour }}>IAL{r.min_assurance}</span>
          </Tooltip>
        </Row>
        <Row label="registration">
          <span className="inline-flex items-center gap-1.5 t-body">
            {r.self_registration
              ? <><IconUserPlus size={11} style={{ color: 'var(--warn)' }} />self-service</>
              : <><IconLock size={11} style={{ color: 'var(--ink-3)' }} />provisioned</>}
          </span>
        </Row>
        <Row label="required factors">
          {r.required_factors.map((f) => (
            <span key={f} className="chip">
              <IconShieldLock size={9} style={{ marginRight: 4 }} />{f}
            </span>
          ))}
        </Row>
        <Row label="also allowed">
          {r.allowed_factors.filter((f) => !r.required_factors.includes(f)).length === 0
            ? <span className="t-xs">—</span>
            : r.allowed_factors.filter((f) => !r.required_factors.includes(f))
                .map((f) => <span key={f} className="chip">{f}</span>)}
        </Row>
        <Row label="session">
          <span className="inline-flex items-center gap-1.5 t-body tnum">
            <IconClock size={11} style={{ color: 'var(--ink-3)' }} />{r.session_ttl}
          </span>
        </Row>
        <Row label="retention">
          {r.default_retention ? (
            <Tooltip label="Statutory limit. Records past this are anonymised by the retention sweeper — failing to run it is regulatory exposure, not housekeeping.">
              <span className="chip" style={{ color: 'var(--warn)', borderColor: 'color-mix(in srgb, var(--warn) 20%, transparent)' }}>
                <IconTrash size={9} style={{ marginRight: 4 }} />{r.default_retention}
              </span>
            </Tooltip>
          ) : <span className="t-xs">no statutory limit</span>}
        </Row>
      </div>
      <Categories r={r} />
    </div>
  )
}

function Realms() {
  const { data: realms } = useQuery({ queryKey: qk.realms(), queryFn: api.realms })
  return (
    <Page
      title="Populations"
      description="Who can sign in, and under what rules. Populations are data — rename them or add more, even several of the same kind. Only the kind itself (internal, partner, public, service) is fixed by the schema."
    >
      <div className="flex flex-col gap-4">
        <div className="panel-inset flex items-start gap-2.5 px-3.5 py-2.5">
          <IconInfoCircle size={14} style={{ color: 'var(--ink-3)', marginTop: 1, flexShrink: 0 }} />
          <div className="t-xs">
            Partners are <b style={{ color: 'var(--ink-2)' }}>not</b> separate tenants. A supplier
            needs access to our purchase-order data, which would require cross-tenant grants — and
            every scope foreign key is composite on tenant precisely to make those impossible to insert.
          </div>
        </div>
        <div className="grid gap-4" style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(300px, 1fr))' }}>
          {realms?.map((r) => <RealmCard key={r.id} r={r} />)}
        </div>
      </div>
    </Page>
  )
}
