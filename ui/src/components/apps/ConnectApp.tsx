import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { Button, Code, Modal, Stepper, TextInput, UnstyledButton } from '@mantine/core'
import { IconPlus, IconX } from '@tabler/icons-react'
import { notifyRejected } from '@/components/create/shell'
import { SignInChoice, type SignIn } from './SignInChoice'
import { KINDS, hasSecret } from './AppConnection'
import { queryClient } from '@/lib/query/client'
import * as live from '@/lib/api/live'

const slugOf = (name: string) => name.toLowerCase().normalize('NFKD').replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 40)

function Addresses({ label, hint, value, onChange }: { label: string; hint: string; value: string[]; onChange: (v: string[]) => void }) {
  return (
    <div className="flex flex-col gap-1.5">
      <div className="t-body" style={{ fontWeight: 500 }}>{label}</div>
      {value.map((v, i) => (
        <div key={i} className="flex items-center gap-1.5">
          <TextInput className="flex-1" size="sm" value={v} placeholder="https://finance.example.com/callback"
            aria-label={`${label} ${i + 1}`}
            onChange={(e) => onChange(value.map((x, j) => (j === i ? e.currentTarget.value : x)))} />
          {value.length > 1 && (
            <UnstyledButton aria-label="Remove" onClick={() => onChange(value.filter((_, j) => j !== i))}><IconX size={14} /></UnstyledButton>
          )}
        </div>
      ))}
      <div className="flex items-center justify-between">
        <span className="t-xs">{hint}</span>
        <Button size="compact-xs" variant="subtle" leftSection={<IconPlus size={12} />} onClick={() => onChange([...value, ''])}>Add another</Button>
      </div>
    </div>
  )
}

/* Connecting an app is three questions in the order people know the answers:
   what it is, who may sign in to it, and then the keys to hand the developers.
   The keys come last because the secret exists only once the app does, and is
   shown exactly once. */
export function ConnectApp() {
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const [step, setStep] = useState(0)
  const [name, setName] = useState('')
  const [slug, setSlug] = useState('')
  const [slugTouched, setSlugTouched] = useState(false)
  const [kind, setKind] = useState('server')
  const [redirects, setRedirects] = useState([''])
  const [logouts, setLogouts] = useState([''])
  const [signIn, setSignIn] = useState<SignIn>('role_holders')
  const [secret, setSecret] = useState('')
  const [busy, setBusy] = useState(false)

  const reset = () => {
    setStep(0); setName(''); setSlug(''); setSlugTouched(false); setKind('server')
    setRedirects(['']); setLogouts(['']); setSignIn('role_holders'); setSecret('')
  }
  const close = () => { setOpen(false); reset() }
  const short = slugTouched ? slug : slugOf(name)
  const people = kind !== 'service'
  const stepOneOk = name.trim() !== '' && /^[a-z0-9][a-z0-9-]*$/.test(short) && (!people || redirects.some((r) => r.trim()))

  async function create() {
    setBusy(true)
    try {
      const clean = (v: string[]) => v.map((x) => x.trim()).filter(Boolean)
      const { clientSecret } = await live.createApplication({
        slug: short, name: name.trim(), kind,
        redirectUris: people ? clean(redirects) : [],
        postLogoutRedirectUris: people ? clean(logouts) : [],
        signIn: people ? signIn : 'anyone',
      })
      setSecret(clientSecret)
      setStep(2)
      await queryClient.invalidateQueries({ queryKey: ['applications'] })
    } catch (e) { notifyRejected(e) } finally { setBusy(false) }
  }

  return (
    <>
      <Button leftSection={<IconPlus size={15} />} onClick={() => setOpen(true)}>Connect an app</Button>
      <Modal opened={open} onClose={close} title="Connect an app" centered size="lg"
        closeOnClickOutside={step !== 2}>
        <Stepper active={step} size="xs" mb="md" allowNextStepsSelect={false}>
          <Stepper.Step label="What it is" />
          <Stepper.Step label="Who may sign in" />
          <Stepper.Step label="Keys for your developers" />
        </Stepper>

        {step === 0 && (
          <div className="flex flex-col gap-3.5">
            <div className="grid gap-3 sm:grid-cols-2">
              <TextInput label="Name" required value={name} autoFocus placeholder="Finance"
                onChange={(e) => setName(e.currentTarget.value)} />
              <TextInput label="Short name" required value={short} placeholder="finance"
                description="Starts its permissions. Cannot be changed later." inputWrapperOrder={['label', 'input', 'description', 'error']}
                onChange={(e) => { setSlug(e.currentTarget.value); setSlugTouched(true) }} />
            </div>
            <div>
              <div className="t-body mb-1.5" style={{ fontWeight: 500 }}>What kind of app?</div>
              <div className="grid gap-2 sm:grid-cols-2" role="radiogroup">
                {KINDS.map((k) => (
                  <UnstyledButton key={k.value} role="radio" aria-checked={kind === k.value} onClick={() => setKind(k.value)}
                    className="panel-inset px-3 py-2.5"
                    style={kind === k.value ? { outline: '2px solid var(--accent)', outlineOffset: -2 } : undefined}>
                    <div className="t-body" style={{ fontWeight: 550 }}>{k.label}</div>
                    <div className="t-xs">{k.hint}</div>
                  </UnstyledButton>
                ))}
              </div>
            </div>
            {people && (
              <>
                <Addresses label="Where people come back after signing in" hint="Exact address." value={redirects} onChange={setRedirects} />
                <Addresses label="Where people land after signing out" hint="Optional. Usually the app's home page." value={logouts} onChange={setLogouts} />
              </>
            )}
            <div className="flex justify-end gap-2">
              <Button variant="default" size="xs" onClick={close}>Cancel</Button>
              {people
                ? <Button size="xs" disabled={!stepOneOk} onClick={() => setStep(1)}>Next: who may sign in</Button>
                : <Button size="xs" disabled={!stepOneOk} loading={busy} onClick={() => void create()}>Connect it</Button>}
            </div>
          </div>
        )}

        {step === 1 && (
          <div className="flex flex-col gap-4">
            <SignInChoice value={signIn} onChange={setSignIn} appName={name.trim()} />
            <div className="t-xs">You give people roles in {name.trim()} once it is connected, from its page.</div>
            <div className="flex justify-end gap-2">
              <Button variant="default" size="xs" onClick={() => setStep(0)}>Back</Button>
              <Button size="xs" loading={busy} onClick={() => void create()}>Connect it</Button>
            </div>
          </div>
        )}

        {step === 2 && (
          <div className="flex flex-col gap-3">
            <p className="t-sm">{name.trim()} is connected. Give these to whoever sets it up.</p>
            <div className="t-xs">Client id</div>
            <Code block>{short}</Code>
            {hasSecret(kind) && secret && (
              <>
                <div className="t-xs">Client secret</div>
                <Code block>{secret}</Code>
                <div className="t-xs" style={{ color: 'var(--warn)' }}>
                  Copy the secret now. It is shown only this once; if it is lost you make a new one, which stops the old one.
                </div>
              </>
            )}
            <div className="flex justify-end gap-2">
              <Button variant="default" size="xs" onClick={close}>Done</Button>
              <Button size="xs" onClick={() => { close(); void navigate({ to: '/applications/$slug', params: { slug: short } }) }}>
                Open {name.trim()}
              </Button>
            </div>
          </div>
        )}
      </Modal>
    </>
  )
}
