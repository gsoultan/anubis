import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Checkbox, Select, TextInput } from '@mantine/core'
import { FeaturePicker } from '@/components/access/FeaturePicker'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/client'
import { useCreate } from '@/stores/create'
import { CreateShell, CancelSubmit, notifyCreated, notifyRejected } from './shell'
import type { RealmKind } from '@/lib/api/types'

export const KINDS: { value: RealmKind; label: string; hint: string }[] = [
  { value: 'internal', label: 'Employees', hint: 'staff, interns, board members' },
  { value: 'partner', label: 'Partner contacts', hint: 'suppliers, contractors' },
  { value: 'public', label: 'Customers and applicants', hint: 'anyone who signed themselves up' },
  { value: 'service', label: 'Service accounts', hint: 'programs, not people' },
]

/* A new role in the order people think of one: its name and app, who it is
   for, then what it allows. Who can get it is enforced by the database: a
   role for employees can never be attached to a self-registered account,
   even by a script that bypasses this console. */
export function CreateRole({ opened }: { opened: boolean }) {
  const { close } = useCreate()
  const { data: perms } = useQuery({ queryKey: qk.permissions(), queryFn: api.permissions, enabled: opened })
  const { data: apps } = useQuery({ queryKey: ['app-choices'], queryFn: () => api.applications(), enabled: opened })
  const [name, setName] = useState('')
  const [app, setApp] = useState<string | null>(null)
  const [description, setDescription] = useState('')
  const [kinds, setKinds] = useState<RealmKind[]>(['internal'])
  const [keys, setKeys] = useState<string[]>([])
  const [busy, setBusy] = useState(false)

  const reset = () => { setName(''); setApp(null); setDescription(''); setKinds(['internal']); setKeys([]) }
  const done = () => { reset(); close() }
  const ok = name.trim().length >= 2 && kinds.length > 0 && keys.length > 0

  async function submit() {
    setBusy(true)
    try {
      await api.createRole({
        name: name.trim(), description: description.trim(), allowed_realm_kinds: kinds, permission_keys: keys,
        ...(app ? { application_slug: app } : {}),
      })
      notifyCreated('Role created', `${name.trim()} · ${keys.length} permissions. Nobody has it yet.`)
      await queryClient.invalidateQueries({ queryKey: qk.roles() })
      done()
    } catch (e) { notifyRejected(e) } finally { setBusy(false) }
  }

  return (
    <CreateShell
      opened={opened} onClose={done} title="New role" size={620}
      description="Name it, say who it is for, then tick what it allows."
      footer={<CancelSubmit onCancel={done} onSubmit={() => void submit()} canSubmit={ok} submitting={busy} label="Create role" />}
    >
      <div className="flex flex-col gap-4">
        <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_200px]">
          <TextInput label="Name" placeholder="Payables clerk" required data-autofocus
            value={name} onChange={(e) => setName(e.currentTarget.value)} />
          <Select label="App" placeholder="Across apps" clearable searchable value={app}
            onChange={(v) => { setApp(v); setKeys([]) }}
            data={(apps ?? []).map((a) => ({ value: a.slug, label: a.name }))} />
        </div>
        <TextInput label="What is it for?" description="Shown when someone gives it." placeholder="Enters supplier invoices and keeps suppliers up to date"
          value={description} onChange={(e) => setDescription(e.currentTarget.value)} />
        <div>
          <div className="t-body mb-1.5" style={{ fontWeight: 500 }}>Who can get it</div>
          <div className="grid gap-2 sm:grid-cols-2">
            {KINDS.map((k) => (
              <Checkbox key={k.value} label={<span>{k.label} <span className="t-xs">· {k.hint}</span></span>}
                checked={kinds.includes(k.value)}
                onChange={(e) => setKinds(e.currentTarget.checked ? [...kinds, k.value] : kinds.filter((x) => x !== k.value))} />
            ))}
          </div>
        </div>
        <div>
          <div className="t-body mb-0.5" style={{ fontWeight: 500 }}>What it allows</div>
          <div className="t-xs mb-2">
            Tick a whole module, a whole feature, or single actions. Permissions come from each app; to add one, change
            the app on its page under Apps.
          </div>
          <FeaturePicker perms={perms ?? []} apps={apps ?? []} appId={app} value={keys} onChange={setKeys} />
        </div>
      </div>
    </CreateShell>
  )
}
