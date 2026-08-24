import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { Button, PasswordInput, Select, TextInput } from '@mantine/core'
import { IconArrowRight, IconCheck, IconDatabase, IconKey, IconUserShield } from '@tabler/icons-react'
import { useState } from 'react'
import { notifyRejected } from '@/components/create/shell'

export const Route = createFileRoute('/setup')({ component: Setup })

const SSL_MODES = ['disable', 'allow', 'prefer', 'require', 'verify-ca', 'verify-full']

type Fields = Record<string, string>

/* The installer is the only part of Anubis that answers before there is a
   database, so it talks plain HTTP rather than Connect: there is no schema,
   no keyring and no interceptor chain yet. */
async function post(path: string, body: unknown) {
  const resp = await fetch(path, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(body),
  })
  const json = (await resp.json()) as { ok?: boolean; error?: string; fields?: Fields; message?: string }
  if (!resp.ok && !json.fields) throw new Error(json.message ?? `setup failed (${resp.status})`)
  return json
}

function Setup() {
  const navigate = useNavigate()
  const [step, setStep] = useState<'key' | 'database' | 'owner' | 'done'>('key')
  const [fields, setFields] = useState<Fields>({})
  const [busy, setBusy] = useState(false)
  const [tested, setTested] = useState(false)

  const [token, setToken] = useState('')
  const [db, setDb] = useState({
    host: 'localhost', port: '5432', name: 'anubis', user: 'anubis',
    password: '', sslmode: 'require',
  })
  const [owner, setOwner] = useState({ username: '', email: '', password: '' })
  const [firstTenant, setFirstTenant] = useState({ slug: '', name: '' })

  const payload = () => ({
    token: token.trim(),
    db_host: db.host.trim(), db_port: Number(db.port) || 0, db_name: db.name.trim(),
    db_user: db.user.trim(), db_password: db.password, db_sslmode: db.sslmode,
    first_tenant_slug: firstTenant.slug.trim(), first_tenant_name: firstTenant.name.trim(),
    owner_username: owner.username.trim(), owner_email: owner.email.trim(),
    owner_password: owner.password,
  })

  const testConnection = async () => {
    setBusy(true); setFields({})
    try {
      const r = await post('/v1/setup/test-connection', payload())
      if (r.fields) { setFields(r.fields); setTested(false) }
      else if (r.ok) { setTested(true) }
      else { setTested(false); notifyRejected(new Error(r.error ?? 'could not connect')) }
    } catch (e) { notifyRejected(e) }
    setBusy(false)
  }

  const install = async () => {
    setBusy(true); setFields({})
    try {
      const r = await post('/v1/setup', payload())
      if (r.fields) { setFields(r.fields); setStep('database') }
      else if (r.ok) { setStep('done') }
      else notifyRejected(new Error(r.error ?? 'setup failed'))
    } catch (e) { notifyRejected(e) }
    setBusy(false)
  }

  const err = (k: string) => fields[k]

  return (
    <div className="flex h-full items-center justify-center" style={{ background: 'var(--s-base)' }}>
      <div className="fade" style={{ width: 460 }}>
        <div className="mb-5">
          <div style={{ fontSize: 19, fontWeight: 650, letterSpacing: '-.02em' }}>Set up Anubis</div>
          <div className="t-sm mt-1">
            This runs once. When it finishes, this page is gone for good.
          </div>
        </div>

        <div className="panel p-5">
          {step === 'key' && (
            <form onSubmit={(e) => { e.preventDefault(); setStep('database') }}>
              <div className="mb-1 flex items-center gap-2">
                <IconKey size={17} style={{ color: 'var(--gold)' }} />
                <h1 className="t-h1">Setup key</h1>
              </div>
              <p className="t-sm mb-4">
                Printed to the server’s console when it started. Without it, whoever
                reaches this page first would choose the database this installation
                trusts.
              </p>
              <div className="flex flex-col gap-3">
                <TextInput label="Setup key" required autoFocus value={token}
                  error={err('token')} onChange={(e) => setToken(e.currentTarget.value)} />
                <Button type="submit" disabled={!token.trim()} rightSection={<IconArrowRight size={15} />}>
                  Continue
                </Button>
              </div>
            </form>
          )}

          {step === 'database' && (
            <form onSubmit={(e) => { e.preventDefault(); setStep('owner') }}>
              <div className="mb-1 flex items-center gap-2">
                <IconDatabase size={17} style={{ color: 'var(--gold)' }} />
                <h1 className="t-h1">Database</h1>
              </div>
              <p className="t-sm mb-4">
                Anubis will create its schema here. The password is sealed into the
                config file, never written in the clear.
              </p>
              <div className="flex flex-col gap-3">
                <div className="flex gap-2">
                  <TextInput label="Host" required style={{ flex: 2 }} value={db.host}
                    error={err('db_host')} onChange={(e) => { setDb({ ...db, host: e.currentTarget.value }); setTested(false) }} />
                  <TextInput label="Port" required style={{ flex: 1 }} value={db.port}
                    error={err('db_port')} onChange={(e) => { setDb({ ...db, port: e.currentTarget.value }); setTested(false) }} />
                </div>
                <TextInput label="Database" required value={db.name}
                  error={err('db_name')} onChange={(e) => { setDb({ ...db, name: e.currentTarget.value }); setTested(false) }} />
                <TextInput label="User" required value={db.user}
                  error={err('db_user')} onChange={(e) => { setDb({ ...db, user: e.currentTarget.value }); setTested(false) }} />
                <PasswordInput label="Password" value={db.password}
                  onChange={(e) => { setDb({ ...db, password: e.currentTarget.value }); setTested(false) }} />
                <Select label="TLS" data={SSL_MODES} value={db.sslmode}
                  error={err('db_sslmode')}
                  onChange={(v) => { setDb({ ...db, sslmode: v ?? 'require' }); setTested(false) }} />

                <div className="flex items-center gap-2">
                  <Button variant="default" loading={busy} onClick={testConnection}>
                    Test connection
                  </Button>
                  {tested && (
                    <span className="t-xs flex items-center gap-1" style={{ color: 'var(--gold)' }}>
                      <IconCheck size={13} /> reachable
                    </span>
                  )}
                </div>
                {/* Testing first is not required, but going on without it means
                    finding out at the point where the schema is being written. */}
                <Button type="submit" rightSection={<IconArrowRight size={15} />}>
                  Continue
                </Button>
              </div>
            </form>
          )}

          {step === 'owner' && (
            <form onSubmit={(e) => { e.preventDefault(); install() }}>
              <div className="mb-1 flex items-center gap-2">
                <IconUserShield size={17} style={{ color: 'var(--gold)' }} />
                <h1 className="t-h1">Owner account</h1>
              </div>
              <p className="t-sm mb-4">
                The person who runs this installation. They belong to no tenant —
                a tenant’s own users are a separate population entirely.
              </p>
              <div className="flex flex-col gap-3">
                <TextInput label="Username" required autoFocus value={owner.username}
                  error={err('owner_username')}
                  onChange={(e) => setOwner({ ...owner, username: e.currentTarget.value })} />
                <TextInput label="Email" value={owner.email}
                  onChange={(e) => setOwner({ ...owner, email: e.currentTarget.value })} />
                <PasswordInput label="Password" required value={owner.password}
                  description="At least 12 characters."
                  error={err('owner_password')}
                  onChange={(e) => setOwner({ ...owner, password: e.currentTarget.value })} />

                <div className="t-label mt-2">First tenant (optional)</div>
                <div className="flex gap-2">
                  <TextInput label="Slug" style={{ flex: 1 }} value={firstTenant.slug}
                    error={err('first_tenant_slug')}
                    onChange={(e) => setFirstTenant({ ...firstTenant, slug: e.currentTarget.value })} />
                  <TextInput label="Name" style={{ flex: 2 }} value={firstTenant.name}
                    error={err('first_tenant_name')}
                    onChange={(e) => setFirstTenant({ ...firstTenant, name: e.currentTarget.value })} />
                </div>

                <Button type="submit" loading={busy} rightSection={<IconArrowRight size={15} />}>
                  Install
                </Button>
              </div>
            </form>
          )}

          {step === 'done' && (
            <div>
              <div className="mb-1 flex items-center gap-2">
                <IconCheck size={17} style={{ color: 'var(--gold)' }} />
                <h1 className="t-h1">Installed</h1>
              </div>
              <p className="t-sm mb-4">
                The schema is in place, the owner account exists, and the
                configuration has been written. This page will not open again.
              </p>
              <Button onClick={() => navigate({ to: '/signin', search: { next: '/' } })}
                rightSection={<IconArrowRight size={15} />}>
                Sign in
              </Button>
            </div>
          )}
        </div>

        <p className="t-xs mt-3" style={{ textAlign: 'center' }}>
          Step {step === 'key' ? 1 : step === 'database' ? 2 : step === 'owner' ? 3 : 3} of 3
        </p>
      </div>
    </div>
  )
}
