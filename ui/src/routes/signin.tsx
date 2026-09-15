import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { Button, PasswordInput, TextInput } from '@mantine/core'
import { IconArrowRight, IconShieldLock } from '@tabler/icons-react'
import { useEffect, useState } from 'react'
import { completeMfa, signIn } from '@/stores/auth'
import { notifyRejected } from '@/components/create/shell'
import { useQuery } from '@tanstack/react-query'

/* Which tenant this console signs in to, asked of the server before anybody
   has signed in. A username is unique only within (tenant, realm), so the
   server does need one — but almost every installation has exactly one, and
   making a person recall its slug is friction with nothing behind it.

   Unauthenticated by necessity: it is read to render this form. */
type ConsoleConfig = { issuer: string; setup_required: boolean }

async function consoleConfig(): Promise<ConsoleConfig> {
  const resp = await fetch('/v1/console-config', { headers: { accept: 'application/json' } })
  if (!resp.ok) throw new Error(`console-config: ${resp.status}`)
  return resp.json() as Promise<ConsoleConfig>
}

/** next is where to land after signing in. It comes out of the URL, so it is
    only ever honoured as a same-origin path: a value like //evil.example or
    https://evil.example would otherwise turn this form into an open redirect
    that arrives wearing our own domain. */
function safePath(next: unknown): string {
  if (typeof next !== 'string') return '/'
  if (!next.startsWith('/') || next.startsWith('//')) return '/'
  return next
}

export const Route = createFileRoute('/signin')({
  validateSearch: (search: Record<string, unknown>): { next: string } => ({
    next: safePath(search['next']),
  }),
  component: SignIn,
})

/* Same boxed mark as the shell's, so the product the operator signs into looks
   like the product they arrive in. The old one hardcoded two hex golds and a
   near-black eye, which meant the wordmark was the one element on the page
   that did not follow the colour scheme. */
function Jackal() {
  return (
    <span
      className="flex items-center justify-center"
      style={{
        width: 40, height: 40, borderRadius: 11, flex: 'none',
        background: 'color-mix(in srgb, var(--brand) 14%, transparent)',
        border: '1px solid color-mix(in srgb, var(--brand) 26%, transparent)',
      }}
    >
      <svg viewBox="0 0 24 24" width={23} height={23} aria-hidden>
        <path d="M4 3l3.2 5.4A8.6 8.6 0 0 1 12 7c1.7 0 3.3.5 4.8 1.4L20 3l.9 7.4c.3 2.6-.6 5.2-2.5 7L12 22l-6.4-4.6c-1.9-1.8-2.8-4.4-2.5-7L4 3z"
          fill="var(--brand)" />
        <circle cx="9.4" cy="12.6" r="1.15" fill="var(--s-base)" />
        <circle cx="14.6" cy="12.6" r="1.15" fill="var(--s-base)" />
      </svg>
    </span>
  )
}


function SignIn() {
  const { next } = Route.useSearch()
  const navigate = useNavigate()

  const { data: cfg } = useQuery({
    queryKey: ['console-config'],
    queryFn: consoleConfig,
    staleTime: 5 * 60_000,
    retry: false,
  })

  useEffect(() => {
    if (cfg?.setup_required) navigate({ to: '/setup' })
  }, [cfg, navigate])

  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  // Set when the operator has a second factor. Holding it here and nowhere
  // else means a half-finished sign-in leaves no trace if they walk away.
  const [challenge, setChallenge] = useState('')
  const [code, setCode] = useState('')

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      const res = await signIn(username.trim(), password)
      if (res.mfa) {
        setChallenge(res.mfaToken)
        // The password is spent and a factor is still outstanding.
        setPassword('')
      } else {
        navigate({ to: next })
      }
    } catch (err) { notifyRejected(err) }
    setBusy(false)
  }

  const submitCode = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      await completeMfa(challenge, code.trim())
      navigate({ to: next })
    } catch (err) {
      notifyRejected(err)
      // A TOTP step is single-use whether or not it was right, so the same
      // code will not work twice.
      setCode('')
    }
    setBusy(false)
  }

  return (
    /* min-h-dvh, not h-full: on mobile the browser chrome is inside 100vh, so a
       centred card sits partly under it. The padding is what stops a 360px card
       from touching both edges of a 360px phone. */
    <div className="relative flex min-h-dvh items-center justify-center p-4"
      style={{ background: 'var(--s-base)' }}>
      {/* A card centred in a flat field reads as an unstyled page. Two very
          soft washes give the plane some depth without putting anything on it
          that has to be read. */}
      <div aria-hidden className="pointer-events-none absolute inset-0" style={{
        background:
          'radial-gradient(60rem 30rem at 50% -10%, var(--accent-bg), transparent 70%),' +
          'radial-gradient(40rem 20rem at 50% 110%, color-mix(in srgb, var(--brand) 7%, transparent), transparent 70%)',
      }} />

      <div className="fade relative" style={{ width: 'min(384px, 100%)' }}>
        <div className="mb-5 flex items-center gap-3">
          <Jackal />
          <div className="leading-none">
            <div style={{ fontSize: 19, fontWeight: 650, letterSpacing: '-.02em' }}>Anubis</div>
            <div className="t-xs" style={{ marginTop: 4 }}>platform console</div>
          </div>
        </div>

        <div className="panel p-6" style={{ boxShadow: 'var(--shadow-lg)' }}>
          {challenge ? (
            <form onSubmit={submitCode}>
              <div className="mb-1 flex items-center gap-2">
                <IconShieldLock size={17} style={{ color: 'var(--accent)' }} />
                <h1 className="t-h1">Second factor</h1>
              </div>
              <p className="t-sm mb-4">
                Enter the six-digit code from your authenticator.
              </p>
              <div className="flex flex-col gap-3">
                <TextInput label="Authenticator code" value={code} required autoFocus
                  inputMode="numeric" autoComplete="one-time-code" placeholder="000000"
                  onChange={(e) => setCode(e.currentTarget.value)} />
                <Button type="submit" loading={busy} rightSection={<IconArrowRight size={15} />}>
                  Verify
                </Button>
                <Button variant="subtle" size="compact-sm"
                  onClick={() => { setChallenge(''); setCode('') }}>
                  Start over
                </Button>
              </div>
            </form>
          ) : (
          <form onSubmit={submit}>
            <h1 className="t-h1 mb-1">Sign in</h1>
            <p className="t-sm mb-4">
              For people who operate this installation. A tenant’s own users sign
              in through their organisation’s page, not here.
            </p>
            <div className="flex flex-col gap-3">
              <TextInput label="Username" value={username} required autoFocus
                autoComplete="username" onChange={(e) => setUsername(e.currentTarget.value)} />
              <PasswordInput label="Password" value={password} required
                autoComplete="current-password" onChange={(e) => setPassword(e.currentTarget.value)} />
              <Button type="submit" loading={busy} disabled={cfg?.setup_required === true}
                rightSection={<IconArrowRight size={15} />}>
                Continue
              </Button>
              {cfg?.setup_required && (
                <p className="t-xs" style={{ color: 'var(--warn)' }}>
                  This installation has not been set up yet — no platform user exists.
                </p>
              )}
            </div>
          </form>
          )}
        </div>

        {/* Which installation this is. consoleConfig already returns the
            issuer and the form already fetches it; an operator who runs a
            staging and a production Anubis had no way to tell the two sign-in
            pages apart. */}
        {cfg?.issuer && (
          <div className="mt-4 text-center">
            <span className="chip">{cfg.issuer}</span>
          </div>
        )}
      </div>
    </div>
  )
}
