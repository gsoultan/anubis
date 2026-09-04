import { createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import {
  ActionIcon, Button, Modal, MultiSelect, NumberInput, Switch, TextInput, Tooltip,
} from '@mantine/core'
import {
  IconInfoCircle, IconShieldLock, IconClock, IconTrash, IconUserPlus, IconLock,
  IconPlus,
  IconPencil,
} from '@tabler/icons-react'
import { useEffect, useState } from 'react'
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
  const [editing, setEditing] = useState(false)
  return (
    <div className="panel panel-hover overflow-hidden">
      <EditRealm r={r} opened={editing} onClose={() => setEditing(false)} />
      <div className="px-4 py-3.5" style={{ background: k.tint, borderBottom: '1px solid var(--line-soft)' }}>
        <div className="flex items-center justify-between gap-2">
          <span className="t-h1">{r.display_name}</span>
          <div className="flex items-center gap-1.5">
            <span className="chip" style={{ color: k.colour, borderColor: `${k.colour}33` }}>{r.kind}</span>
            <ActionIcon size="sm" variant="subtle" aria-label="Edit population"
              onClick={() => setEditing(true)}>
              <IconPencil size={14} />
            </ActionIcon>
          </div>
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
        <Row label="enforcement">
          {enforcementNote(r)}
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

/* Whether the required factors above actually bite.
   A realm can list `totp` as required and enforce nothing, which is the
   default and easy to mistake for protection — so say which it is. */
function enforcementNote(r: import('@/lib/api/types').Realm) {
  const secondFactors = r.required_factors.filter((f) => f !== 'password')
  if (secondFactors.length === 0) {
    return <span className="t-xs">password only</span>
  }
  if (!r.factor_enrolment_deadline) {
    return (
      <Tooltip label="Members who have not enrolled still sign in with a password alone. Set realms.factor_enrolment_deadline to start enforcing — read docs/enrolment-rollout.md first, because the date is a lockout for anyone who misses it.">
        <span className="chip" style={{ color: 'var(--warn)' }}>
          <IconShieldLock size={9} style={{ marginRight: 4 }} />not enforced
        </span>
      </Tooltip>
    )
  }
  const when = new Date(r.factor_enrolment_deadline * 1000)
  const started = when.getTime() <= Date.now()
  return (
    <Tooltip label={started
      ? 'Members without these factors are refused a session and handed an enrolment challenge instead.'
      : 'Members without these factors can still sign in, and are told the date.'}>
      <span className="chip" style={{ color: started ? 'var(--allow)' : 'var(--info)' }}>
        <IconShieldLock size={9} style={{ marginRight: 4 }} />
        {started ? 'enforced since ' : 'enforced from '}{when.toLocaleDateString()}
      </span>
    </Tooltip>
  )
}

/* Editing a population is mostly editing its MFA policy, which is why those
   fields lead. A realm can list `totp` as required and enforce nothing — the
   enrolment deadline is what makes it bite, and it is a lockout for anyone who
   has not enrolled by then, so it is set deliberately and never by default.

   code and kind are absent on purpose: the server corrects them only while a
   realm has no members, because kind decides which roles those members may
   hold. Offering an input that usually fails is worse than not offering one. */

/** Factor kinds the schema knows (credentials.kind), UNION whatever this realm
    already carries — the columns are free-form text[], and the seeded public
    realm uses `email_otp`, which is not a credential kind. Restricting the
    picker to the known set would silently drop it on the next save. */
const KNOWN_FACTORS = ['password', 'totp', 'device_key', 'recovery_code']

function EditRealm({ r, opened, onClose }: {
  r: Realm; opened: boolean; onClose: () => void
}) {
  const [draft, setDraft] = useState<Realm>(r)
  const [busy, setBusy] = useState(false)
  /* Reopening after a cancel must show what is stored, not the abandoned
     edit — the modal stays mounted, so the draft has to be reset explicitly. */
  useEffect(() => { if (opened) setDraft(r) }, [opened, r])
  const options = [...new Set([...KNOWN_FACTORS, ...r.allowed_factors, ...r.required_factors])]

  const set = <K extends keyof Realm>(k: K, v: Realm[K]) =>
    setDraft((d) => ({ ...d, [k]: v }))

  const secondFactors = draft.required_factors.filter((f) => f !== 'password')

  async function save() {
    setBusy(true)
    try {
      await api.updateRealm(draft)
      await queryClient.invalidateQueries({ queryKey: qk.realms() })
      notifyCreated(`${draft.display_name} updated`,
        secondFactors.length > 0 && !draft.factor_enrolment_deadline
          ? 'Required factors are listed but not enforced — set a deadline to make them bite.'
          : 'Applies to the next sign-in.')
      onClose()
    } catch (e) { notifyRejected(e) } finally { setBusy(false) }
  }

  return (
    <Modal opened={opened} onClose={onClose} centered title={`Edit ${r.display_name}`}>
      <div className="flex flex-col gap-3.5">
        <div>
          <div className="t-body mb-1.5" style={{ fontWeight: 550 }}>Name</div>
          <TextInput value={draft.display_name}
            onChange={(e) => set('display_name', e.currentTarget.value)} />
        </div>

        <div>
          <div className="t-body mb-1.5" style={{ fontWeight: 550 }}>Allowed factors</div>
          <MultiSelect
            data={options} value={draft.allowed_factors}
            onChange={(v) => set('allowed_factors', v)}
          />
          <div className="t-xs mt-1" style={{ opacity: 0.7 }}>
            What members of this population may enrol.
          </div>
        </div>

        <div>
          <div className="t-body mb-1.5" style={{ fontWeight: 550 }}>Required factors</div>
          <MultiSelect
            data={options} value={draft.required_factors}
            onChange={(v) => set('required_factors', v)}
          />
          {secondFactors.length > 0 && !draft.factor_enrolment_deadline && (
            <div className="t-xs mt-1" style={{ color: 'var(--warn)' }}>
              Listed but not enforced. Members who have not enrolled still sign in with a
              password alone until you set a deadline below.
            </div>
          )}
        </div>

        <div>
          <div className="t-body mb-1.5" style={{ fontWeight: 550 }}>Enforce required factors from</div>
          <TextInput
            type="date"
            value={draft.factor_enrolment_deadline
              ? new Date(draft.factor_enrolment_deadline * 1000).toISOString().slice(0, 10)
              : ''}
            onChange={(e) => set('factor_enrolment_deadline',
              e.currentTarget.value ? Math.floor(new Date(e.currentTarget.value).getTime() / 1000) : null)}
          />
          <div className="t-xs mt-1" style={{ opacity: 0.7 }}>
            A date, not a switch. Past it, a member without these factors is refused a session
            and handed an enrolment challenge — so it is a lockout for anyone who misses it.
            Leave empty to keep the requirement advisory.
          </div>
        </div>

        <div>
          <div className="t-body mb-1.5" style={{ fontWeight: 550 }}>Assurance floor</div>
          <NumberInput min={1} max={3} value={draft.min_assurance}
            onChange={(v) => set('min_assurance', (Number(v) || 1) as Realm['min_assurance'])} />
          <div className="t-xs mt-1" style={{ opacity: 0.7 }}>
            Identities below this level cannot be created here.
          </div>
        </div>

        <Switch
          size="xs" label="Self-registration"
          description="Anyone may create an account in this population."
          checked={draft.self_registration}
          onChange={(e) => set('self_registration', e.currentTarget.checked)}
        />
        <Switch
          size="xs" label="Require email verification"
          checked={draft.email_verification_required}
          onChange={(e) => set('email_verification_required', e.currentTarget.checked)}
        />

        <div className="t-xs" style={{ opacity: 0.7 }}>
          Session and token lifetimes, retention and the code and kind of this population
          are not editable here — the first are rarely changed and the last two are fixed
          once anyone belongs to it.
        </div>

        <div className="mt-1 flex justify-end gap-2">
          <Button variant="default" size="xs" onClick={onClose}>Cancel</Button>
          <Button size="xs" loading={busy} onClick={save}>Save</Button>
        </div>
      </div>
    </Modal>
  )
}
