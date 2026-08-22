import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button, Select, TextInput } from '@mantine/core'
import { IconPlus, IconX } from '@tabler/icons-react'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/client'
import { useCreate } from '@/stores/create'
import { AxisIcon } from '@/components/scope/AxisIcon'
import { AxisConstraintRow, type NodeSel } from './CreateGrant'
import { CreateShell, CancelSubmit, notifyCreated, notifyRejected } from './shell'
import type { GrantScope } from '@/lib/api/types'

/* A membership is built entry by entry: pick a role, limit it to places, add.
   The entry list IS the definition — what every member receives. */
type Draft = { role_id: string; role_name: string; scopes: GrantScope[] }

export function CreateMembership({ opened }: { opened: boolean }) {
  const { close } = useCreate()
  const { data: roles } = useQuery({ queryKey: qk.roles(), queryFn: api.roles })
  const { data: axes } = useQuery({ queryKey: qk.axes(), queryFn: api.axes })

  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [entries, setEntries] = useState<Draft[]>([])
  const [roleId, setRoleId] = useState<string | null>(null)
  const [constraints, setConstraints] = useState<Record<string, NodeSel[]>>({})
  const [busy, setBusy] = useState(false)

  const addEntry = () => {
    const role = roles?.find((r) => r.id === roleId)
    if (!role) return
    const scopes: GrantScope[] = Object.entries(constraints).flatMap(([axis_code, list]) =>
      list.map((c) => ({ axis_code, scope_node_id: c.id, inherit: c.inherit })))
    setEntries((e) => [...e, { role_id: role.id, role_name: role.name, scopes }])
    setRoleId(null); setConstraints({})
  }

  const submit = async () => {
    setBusy(true)
    try {
      const m = await api.createMembership({ name, description, entries })
      notifyCreated('Membership created',
        `“${m.name}” — ${m.entries.length} role${m.entries.length > 1 ? 's' : ''}. Assign people on the Memberships page.`)
      await queryClient.invalidateQueries({ queryKey: qk.memberships() })
      setName(''); setDescription(''); setEntries([]); close()
    } catch (e) { notifyRejected(e) }
    setBusy(false)
  }

  return (
    <CreateShell
      opened={opened} onClose={close} title="New membership"
      description={<>A bundle of roles assigned as one unit. Each role can be pinned to any
        mix of structures — an office <i>and</i> a product line <i>and</i> a customer
        segment. “New finance hire in Jakarta” becomes a single action.</>}
      footer={<CancelSubmit onCancel={close} canSubmit={name.trim().length >= 2 && entries.length > 0}
        submitting={busy} label="Create membership" />}
    >
      <div className="flex flex-col gap-4">
        <TextInput label="Name" placeholder="Jakarta Finance Team" required
          value={name} onChange={(e) => setName(e.currentTarget.value)} />
        <TextInput label="Description" placeholder="Everything a finance hire needs on day one"
          value={description} onChange={(e) => setDescription(e.currentTarget.value)} />

        {entries.length > 0 && (
          <div>
            <div className="t-label mb-2">In this membership</div>
            <div className="flex flex-col gap-1.5">
              {entries.map((e, i) => (
                <div key={i} className="panel-inset flex items-center justify-between gap-2 px-2.5 py-2">
                  <span className="t-body min-w-0 truncate" style={{ fontWeight: 530 }}>
                    {e.role_name}
                    <span className="t-xs" style={{ marginLeft: 6 }}>
                      {e.scopes.length ? `· ${e.scopes.length} place${e.scopes.length > 1 ? 's' : ''}` : '· everywhere'}
                    </span>
                  </span>
                  <button onClick={() => setEntries((x) => x.filter((_, j) => j !== i))}
                    aria-label={`Remove ${e.role_name}`} style={{ color: 'var(--ink-3)', display: 'flex' }}>
                    <IconX size={13} />
                  </button>
                </div>
              ))}
            </div>
          </div>
        )}

        <div className="panel-inset flex flex-col gap-2.5 px-3 py-3">
          <div className="t-label">Add a role to the bundle</div>
          <Select size="sm" searchable placeholder="Pick a role" data={(roles ?? []).map((r) => ({ value: r.id, label: r.name }))}
            value={roleId} onChange={setRoleId} />
          {roleId && (axes ?? []).map((a) => (
            <AxisConstraintRow key={a.code} axisCode={a.code} displayName={a.display_name}
              icon={a.ui_schema.icon} values={constraints[a.code] ?? []}
              onChange={(list) => setConstraints((prev) => {
                const next = { ...prev }
                if (list.length === 0) delete next[a.code]; else next[a.code] = list
                return next
              })} />
          ))}
          <Button size="xs" variant="light" leftSection={<IconPlus size={13} />}
            disabled={!roleId} onClick={addEntry}>
            Add to bundle
          </Button>
        </div>
        <div className="t-xs flex items-center gap-1.5">
          <AxisIcon name="users" size={12} />
          Memberships stay flat — no bundles of bundles. That is what keeps them auditable.
        </div>
      </div>
    </CreateShell>
  )
}
