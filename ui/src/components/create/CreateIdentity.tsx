import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { Button, Checkbox, PasswordInput, SegmentedControl, Select, Stepper, TextInput, UnstyledButton } from '@mantine/core'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/client'
import { realmKindColor } from '@/lib/realmKind'
import { useCreate } from '@/stores/create'
import { CreateShell, notifyCreated, notifyRejected } from './shell'
import type { Ial, Realm, RealmKind } from '@/lib/api/types'

const KIND: Record<RealmKind, { label: string; hint: string }> = {
  internal: { label: 'An employee', hint: 'Staff of this organisation.' },
  partner: { label: 'A partner contact', hint: 'Supplier or contractor staff.' },
  public: { label: 'A customer or applicant', hint: 'Sees only their own records.' },
  service: { label: 'A service account', hint: 'A program, not a person.' },
}

const IAL: { value: string; label: string }[] = [
  { value: '1', label: 'Only their email (they said who they are)' },
  { value: '2', label: 'Checked by their employer or a contract' },
  { value: '3', label: 'Checked in person, with ID on file' },
]

const USERNAME = /^[a-z0-9][a-z0-9._-]{1,62}$/i

/* Adding someone is three short steps in the order the answers are known:
   who they are, what they may do, and how they will sign in. Only the first
   is needed; "Add and finish later" saves at any step, and access is given on
   their page, where the sheet opens straight away if asked for. */
export function CreateIdentity({ opened }: { opened: boolean }) {
  const { close } = useCreate()
  const navigate = useNavigate()
  const { data: realms } = useQuery({ queryKey: qk.realms(), queryFn: api.realms, enabled: opened })
  const [step, setStep] = useState(0)
  const [realmId, setRealmId] = useState<string | null>(null)
  const [username, setUsername] = useState('')
  const [email, setEmail] = useState('')
  const [assurance, setAssurance] = useState<string | null>(null)
  const [categoryId, setCategoryId] = useState<string | null>(null)
  const [giveNext, setGiveNext] = useState(true)
  const [signIn, setSignIn] = useState<'later' | 'password'>('later')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)

  const realm = realms?.find((r) => r.id === realmId)
  const { data: categories } = useQuery({
    queryKey: qk.realmCategories(realm?.id ?? ''), queryFn: () => api.realmCategories(realm!.id), enabled: !!realm,
  })

  const reset = () => {
    setStep(0); setRealmId(null); setUsername(''); setEmail(''); setAssurance(null); setCategoryId(null)
    setGiveNext(true); setSignIn('later'); setPassword('')
  }
  const done = () => { reset(); close() }
  const pickRealm = (r: Realm) => {
    setRealmId(r.id); setCategoryId(null)
    setAssurance((a) => (a && Number(a) >= r.min_assurance ? a : String(Math.max(r.min_assurance, r.kind === 'internal' ? 2 : 1))))
  }
  const nameError = username && !USERNAME.test(username) ? 'Letters, digits, dot, dash or underscore, 2 to 63 long.' : undefined
  const whoOk = !!realm && USERNAME.test(username) && !!assurance
  const passwordOk = signIn === 'later' || password.length >= 12

  async function create() {
    if (!whoOk || !realm) return
    setBusy(true)
    try {
      const created = await api.createIdentity({
        realm_id: realm.id, username: username.trim(), email: email.trim(),
        assurance_level: Number(assurance) as Ial, category_id: categoryId,
      })
      /* The person exists now whatever happens to the password: a refusal
         is said, and their page offers to set one, rather than leaving the
         form open to add them a second time. */
      if (signIn === 'password' && password) {
        try { await api.setPassword(created.id, password) } catch (e) { notifyRejected(e) }
      }
      notifyCreated(`${created.username} added`, `In ${realm.display_name}.`)
      await queryClient.invalidateQueries({ queryKey: ['identities'] })
      await queryClient.invalidateQueries({ queryKey: qk.dashboard() })
      const give = giveNext && step >= 1
      done()
      void navigate({ to: '/identities/$id', params: { id: created.id }, search: give ? { give: true } : {} })
    } catch (e) { notifyRejected(e) } finally { setBusy(false) }
  }

  const kinds = new Map<RealmKind, number>()
  for (const r of realms ?? []) kinds.set(r.kind, (kinds.get(r.kind) ?? 0) + 1)

  return (
    <CreateShell
      opened={opened} onClose={done} title="Add a person" size={560}
      description="Who they are first. Access and sign-in can be done now or later."
      footer={
        <div className="flex w-full flex-wrap items-center gap-2">
          <Button variant="subtle" size="xs" disabled={!whoOk || !passwordOk} loading={busy} onClick={() => void create()}>
            Add and finish later
          </Button>
          <span className="ml-auto flex gap-2">
            {step === 0 ? <Button variant="default" size="xs" onClick={done}>Cancel</Button>
              : <Button variant="default" size="xs" onClick={() => setStep(step - 1)}>Back</Button>}
            {step < 2
              ? <Button size="xs" disabled={!whoOk} onClick={() => setStep(step + 1)}>{step === 0 ? 'Next: access' : 'Next: how they sign in'}</Button>
              : <Button size="xs" disabled={!whoOk || !passwordOk} loading={busy} onClick={() => void create()}>Add {username.trim() || 'person'}</Button>}
          </span>
        </div>
      }
    >
      <Stepper active={step} size="xs" mb="lg" allowNextStepsSelect={false}>
        <Stepper.Step label="Who they are" />
        <Stepper.Step label="Access" description="optional" />
        <Stepper.Step label="How they sign in" />
      </Stepper>

      {step === 0 && (
        <div className="flex flex-col gap-4">
          <div>
            <div className="t-body mb-1.5" style={{ fontWeight: 500 }}>Who is this?</div>
            <div className="grid gap-2 sm:grid-cols-2" role="radiogroup" aria-label="Population">
              {(realms ?? []).map((r) => (
                <UnstyledButton key={r.id} role="radio" aria-checked={realmId === r.id} onClick={() => pickRealm(r)}
                  className="panel-inset px-3 py-2.5"
                  style={realmId === r.id ? { outline: '2px solid var(--accent)', outlineOffset: -2 } : undefined}>
                  <div className="t-body flex items-center gap-2" style={{ fontWeight: 550 }}>
                    <span style={{ width: 7, height: 7, borderRadius: 99, background: realmKindColor(r.kind) }} />
                    {(kinds.get(r.kind) ?? 0) > 1 ? r.display_name : KIND[r.kind]?.label ?? r.display_name}
                  </div>
                  <div className="t-xs">{(kinds.get(r.kind) ?? 0) > 1 ? KIND[r.kind]?.label : KIND[r.kind]?.hint}</div>
                </UnstyledButton>
              ))}
            </div>
            <div className="t-xs mt-1.5">These are your populations. Each decides how its people sign in and how long their data is kept.</div>
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <TextInput label="Username" required value={username} placeholder="hartono" error={nameError}
              onChange={(e) => setUsername(e.currentTarget.value)} />
            <TextInput label="Email" type="email" value={email} placeholder="hartono@example.com"
              onChange={(e) => setEmail(e.currentTarget.value)} />
          </div>
          <Select label="How sure are you who they are?" value={assurance} onChange={setAssurance} allowDeselect={false}
            description={realm ? 'Some permissions need a higher level. Picked for you from who they are.' : 'Pick who they are first.'}
            data={IAL.map((o) => ({ ...o, disabled: realm ? Number(o.value) < realm.min_assurance : true }))} />
          {realm && (categories?.length ?? 0) > 0 && (
            <Select label="Department" description="Optional, for your directory only." clearable value={categoryId} onChange={setCategoryId}
              data={(categories ?? []).map((c) => ({ value: c.id, label: c.display_name }))} />
          )}
        </div>
      )}

      {step === 1 && (
        <div className="flex flex-col gap-3">
          <p className="t-sm">
            Access is given on {username.trim() || 'their'}'s page: pick what they can do, then where. Nothing is given until you do.
          </p>
          <Checkbox checked={giveNext} onChange={(e) => setGiveNext(e.currentTarget.checked)}
            label="Open “Give access” as soon as they are added" />
        </div>
      )}

      {step === 2 && (
        <div className="flex flex-col gap-3">
          <SegmentedControl fullWidth size="xs" value={signIn} onChange={(v) => setSignIn(v as 'later' | 'password')}
            data={[{ value: 'later', label: 'Set it up later' }, { value: 'password', label: 'Set a password now' }]} />
          {signIn === 'later' && (
            <p className="t-sm">
              They cannot sign in until they have a password or another way in. Their page says so, with a button to set it up.
              {realm && realm.required_factors.length > 0 && <> {realm.display_name} also asks for {realm.required_factors.join(' and ')}.</>}
            </p>
          )}
          {signIn === 'password' && (
            <PasswordInput label="Password" description="At least 12 characters. Give it to them some safe way; they can change it."
              value={password} onChange={(e) => setPassword(e.currentTarget.value)}
              error={password && password.length < 12 ? 'At least 12 characters.' : undefined} />
          )}
        </div>
      )}
    </CreateShell>
  )
}
