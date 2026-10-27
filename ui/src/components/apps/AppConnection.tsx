import { useEffect, useState } from 'react'
import { Button, Select, TextInput, Textarea, Tooltip } from '@mantine/core'
import { IconRefresh } from '@tabler/icons-react'
import { notifyCreated, notifyRejected } from '@/components/create/shell'
import { SecretOnce } from './SecretOnce'
import { queryClient } from '@/lib/query/client'
import * as live from '@/lib/api/live'

export const KINDS = [
  { value: 'server', label: 'Website with its own server', hint: 'Most internal tools. Gets a secret.' },
  { value: 'spa', label: 'Browser-only app', hint: 'Runs fully in the browser. No secret.' },
  { value: 'native', label: 'Phone or desktop app', hint: 'Installed on a device. No secret.' },
  { value: 'service', label: 'Program talking to a program', hint: 'No person signs in. Gets a secret.' },
]

export const kindLabel = (k: string) => KINDS.find((x) => x.value === k)?.label ?? k
export const hasSecret = (k: string) => k === 'server' || k === 'service'
const lines = (v: string) => v.split('\n').map((l) => l.trim()).filter(Boolean)

/* slug and kind are not editable: UpdateApplication does not write them. The
   slug IS the client_id, so changing it breaks every configured client, and
   the kind decides the auth model. An input that always fails is worse than
   none.

   The redirect lists are separate for a reason worth repeating where they are
   edited: a login callback is not a safe place to land after signing out, and
   an open redirect on the logout side is a phishing primitive. */
export function AppConnection({ app }: { app: live.AppRecord }) {
  const [name, setName] = useState(app.name)
  const [status, setStatus] = useState(app.status)
  const [redirects, setRedirects] = useState('')
  const [logouts, setLogouts] = useState('')
  const [backchannel, setBackchannel] = useState('')
  const [busy, setBusy] = useState(false)
  const [secret, setSecret] = useState('')

  const reset = () => {
    setName(app.name); setStatus(app.status)
    setRedirects(app.redirect_uris.join('\n'))
    setLogouts(app.post_logout_redirect_uris.join('\n'))
    setBackchannel(app.backchannel_logout_uri)
  }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(reset, [app])

  const dirty = name !== app.name || status !== app.status || redirects !== app.redirect_uris.join('\n')
    || logouts !== app.post_logout_redirect_uris.join('\n') || backchannel !== app.backchannel_logout_uri

  async function save() {
    setBusy(true)
    try {
      await live.updateApplication({
        ...app, name: name.trim(), status,
        redirect_uris: lines(redirects), post_logout_redirect_uris: lines(logouts),
        backchannel_logout_uri: backchannel.trim(),
      })
      notifyCreated(`${app.name} saved`, 'Applies to the next sign-in.')
      await queryClient.invalidateQueries({ queryKey: ['applications'] })
    } catch (e) { notifyRejected(e) } finally { setBusy(false) }
  }

  async function rotate() {
    try {
      setSecret(await live.rotateClientSecret(app.id))
      notifyCreated('New secret made', 'The previous secret stopped working immediately.')
    } catch (e) { notifyRejected(e) }
  }

  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_300px]">
      <section className="panel flex flex-col gap-3.5 p-4">
        <TextInput label="Name" value={name} onChange={(e) => setName(e.currentTarget.value)} />
        <Select label="Status" value={status} allowDeselect={false} onChange={(v) => setStatus(v ?? 'active')}
          data={[{ value: 'active', label: 'Live' }, { value: 'disabled', label: 'Switched off: nobody can sign in' }]} />
        <Textarea label="Where people come back after signing in" autosize minRows={2} value={redirects}
          description="One address per line, matched exactly."
          onChange={(e) => setRedirects(e.currentTarget.value)} />
        <Textarea label="Where people land after signing out" autosize minRows={2} value={logouts}
          description="A separate list on purpose: an open address here would let anyone send people to a fake site."
          onChange={(e) => setLogouts(e.currentTarget.value)} />
        <TextInput label="Sign-out notice address" value={backchannel} placeholder="none"
          description="Anubis calls it server to server when someone's session ends."
          onChange={(e) => setBackchannel(e.currentTarget.value)} />
        <div className="flex justify-end gap-2">
          <Button variant="default" size="xs" disabled={!dirty} onClick={reset}>Undo</Button>
          <Button size="xs" loading={busy} disabled={!dirty || !name.trim()} onClick={() => void save()}>Save</Button>
        </div>
      </section>
      <aside className="flex flex-col gap-4">
        <section className="panel flex flex-col gap-2 p-4">
          <div className="t-label">Keys for your developers</div>
          <div className="t-xs">Client id</div>
          <code className="chip w-fit">{app.slug}</code>
          <div className="t-xs mt-1">Kind</div>
          <div className="t-body">{kindLabel(app.kind)}</div>
          {hasSecret(app.kind) ? (
            <Tooltip label="The old secret stops working immediately">
              <Button mt="xs" size="xs" variant="default" leftSection={<IconRefresh size={14} />} onClick={() => void rotate()}>
                Make a new secret
              </Button>
            </Tooltip>
          ) : <div className="t-xs mt-1">This kind of app keeps no secret.</div>}
          <div className="t-xs mt-1">The client id and kind are fixed when the app is connected.</div>
        </section>
      </aside>
      {secret && <SecretOnce secret={secret} onDone={() => setSecret('')} />}
    </div>
  )
}
