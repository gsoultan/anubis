import { useQuery } from '@tanstack/react-query'
import { useForm } from '@tanstack/react-form'
import { Checkbox, MultiSelect, TextInput } from '@mantine/core'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/client'
import { useCreate } from '@/stores/create'
import { CreateShell, CancelSubmit, notifyCreated, notifyRejected } from './shell'
import type { RealmKind } from '@/lib/api/types'

export const KINDS: { value: RealmKind; label: string; hint: string }[] = [
  { value: 'internal', label: 'Internal', hint: 'employees, interns, board members' },
  { value: 'partner', label: 'Partners', hint: 'suppliers, contractors' },
  { value: 'public', label: 'Public', hint: 'applicants, customers, any external user' },
]

export function CreateRole({ opened }: { opened: boolean }) {
  const { close } = useCreate()
  const { data: perms } = useQuery({ queryKey: qk.permissions(), queryFn: api.permissions })

  const form = useForm({
    defaultValues: {
      name: '', description: '',
      kinds: ['internal'] as RealmKind[], permission_keys: [] as string[],
    },
    onSubmit: async ({ value }) => {
      try {
        const created = await api.createRole({
          name: value.name, description: value.description,
          allowed_realm_kinds: value.kinds, permission_keys: value.permission_keys,
        })
        notifyCreated('Role created', `${created.name} · ${created.permission_count} permissions`)
        await queryClient.invalidateQueries({ queryKey: qk.roles() })
        form.reset(); close()
      } catch (e) { notifyRejected(e) }
    },
  })

  return (
    <CreateShell
      opened={opened} onClose={close} title="Add a role"
      description={<>A role is a named permission set. <b>Grantable to</b> is the escalation
        guard: an employee-only role can never be attached to a self-registered public
        account, even by a script that bypasses this console.</>}
      footer={
        <form.Subscribe selector={(s) => [s.canSubmit, s.isSubmitting] as const}>
          {([canSubmit, isSubmitting]) => (
            <CancelSubmit onCancel={close}
              canSubmit={!!canSubmit && form.state.values.permission_keys.length > 0}
              submitting={!!isSubmitting} label="Add role" />
          )}
        </form.Subscribe>
      }
    >
      <form onSubmit={(e) => { e.preventDefault(); void form.handleSubmit() }}
        className="flex flex-col gap-4">
        <form.Field name="name" validators={{
          onChange: ({ value }) =>
            value && !/^[a-z][a-z0-9._-]{1,62}$/.test(value)
              ? 'Lowercase, dot-separated by convention — e.g. finance.approver' : undefined,
        }}>
          {(f) => (
            <TextInput label="Name" placeholder="finance.approver" required
              value={f.state.value} onChange={(e) => f.handleChange(e.currentTarget.value)}
              error={f.state.meta.errors[0]} />
          )}
        </form.Field>

        <form.Field name="description">
          {(f) => (
            <TextInput label="Description" placeholder="Approve invoices and payments"
              value={f.state.value} onChange={(e) => f.handleChange(e.currentTarget.value)} />
          )}
        </form.Field>

        <form.Field name="kinds">
          {(f) => (
            <div>
              <div className="t-body mb-1.5" style={{ fontWeight: 500 }}>Grantable to</div>
              <div className="flex flex-col gap-2">
                {KINDS.map((k) => (
                  <Checkbox
                    key={k.value}
                    label={<span>{k.label} <span className="t-xs">— {k.hint}</span></span>}
                    checked={f.state.value.includes(k.value)}
                    onChange={(e) => f.handleChange(
                      e.currentTarget.checked
                        ? [...f.state.value, k.value]
                        : f.state.value.filter((x) => x !== k.value))}
                  />
                ))}
              </div>
            </div>
          )}
        </form.Field>

        <form.Field name="permission_keys">
          {(f) => (
            <MultiSelect
              label="Permissions" placeholder="Search the catalog…" searchable required
              description="Wildcards expand at write time in production; here you pick concrete keys."
              data={(perms ?? []).map((p) => p.key)}
              value={f.state.value} onChange={(x) => f.handleChange(x)}
              maxDropdownHeight={240}
            />
          )}
        </form.Field>
      </form>
    </CreateShell>
  )
}
