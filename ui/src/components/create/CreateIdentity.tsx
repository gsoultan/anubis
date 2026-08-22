import { useQuery } from '@tanstack/react-query'
import { useForm } from '@tanstack/react-form'
import { Select, TextInput } from '@mantine/core'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/client'
import { useCreate } from '@/stores/create'
import { CreateShell, CancelSubmit, notifyCreated, notifyRejected } from './shell'
import type { Ial } from '@/lib/api/types'

const IAL_OPTIONS = [
  { value: '1', label: 'IAL1 — self-asserted (email only)' },
  { value: '2', label: 'IAL2 — remotely verified (contract, employer)' },
  { value: '3', label: 'IAL3 — in-person verified, ID on file' },
]

export function CreateIdentity({ opened }: { opened: boolean }) {
  const { close } = useCreate()
  const { data: realms } = useQuery({ queryKey: qk.realms(), queryFn: api.realms })

  const form = useForm({
    defaultValues: { realm_id: '', username: '', email: '', assurance: '', category_id: '' },
    onSubmit: async ({ value }) => {
      try {
        const created = await api.createIdentity({
          realm_id: value.realm_id,
          username: value.username,
          email: value.email,
          assurance_level: Number(value.assurance) as Ial,
          category_id: value.category_id || null,
        })
        notifyCreated(`${created.username} added`,
          `Identity ${created.id} in the ${realms?.find((r) => r.id === value.realm_id)?.display_name} realm.`)
        await queryClient.invalidateQueries({ queryKey: ['identities'] })
        await queryClient.invalidateQueries({ queryKey: qk.dashboard() })
        form.reset()
        close()
      } catch (e) { notifyRejected(e) }
    },
  })

  const realm = realms?.find((r) => r.id === form.state.values.realm_id)
  const { data: categories } = useQuery({
    queryKey: qk.realmCategories(realm?.id ?? ''),
    queryFn: () => api.realmCategories(realm!.id),
    enabled: !!realm,
  })

  return (
    <CreateShell
      opened={opened} onClose={close} title="Add a person"
      description={<>The realm decides everything downstream: which factors are required,
        how long sessions live, and whether data has a statutory retention limit.</>}
      footer={
        <form.Subscribe selector={(s) => [s.canSubmit, s.isSubmitting] as const}>
          {([canSubmit, isSubmitting]) => (
            <CancelSubmit onCancel={close} canSubmit={!!canSubmit && !!form.state.values.realm_id}
              submitting={!!isSubmitting} label="Add person" />
          )}
        </form.Subscribe>
      }
    >
      <form id="create-identity" onSubmit={(e) => { e.preventDefault(); void form.handleSubmit() }}
        className="flex flex-col gap-4">
        <form.Field name="realm_id">
          {(f) => (
            <Select
              label="Population" placeholder="Who are they to us?" required
              data={(realms ?? []).map((r) => ({
                value: r.id,
                label: `${r.display_name} (${r.kind})`,
              }))}
              value={f.state.value}
              onChange={(v) => {
                f.handleChange(v ?? '')
                form.setFieldValue('category_id', '')   // categories are per-realm
                const min = realms?.find((r) => r.id === v)?.min_assurance
                if (min) form.setFieldValue('assurance', String(min))
              }}
            />
          )}
        </form.Field>

        {realm && (
          <div className="panel-inset px-3 py-2.5">
            <div className="t-xs flex flex-col gap-1">
              <span>Required factors: <b style={{ color: 'var(--ink-2)' }}>{realm.required_factors.join(', ')}</b></span>
              <span>Session TTL: <b style={{ color: 'var(--ink-2)' }}>{realm.session_ttl}</b></span>
              <span>Retention: <b style={{ color: 'var(--ink-2)' }}>{realm.default_retention ?? 'no statutory limit'}</b></span>
            </div>
          </div>
        )}

        <form.Field name="username" validators={{
          onChange: ({ value }) =>
            value && !/^[a-z0-9][a-z0-9._-]{1,62}$/i.test(value)
              ? 'Letters, digits, dot, dash, underscore. 2–63 characters.' : undefined,
        }}>
          {(f) => (
            <TextInput label="Username" placeholder="e.g. hartono" required
              description="Unique within the realm — the same username may exist in another realm."
              value={f.state.value} onChange={(e) => f.handleChange(e.currentTarget.value)}
              error={f.state.meta.errors[0]} />
          )}
        </form.Field>

        {realm && (categories?.length ?? 0) > 0 && (
          <form.Field name="category_id">
            {(f) => (
              <Select
                label="Category" clearable placeholder="Optional — e.g. supplier, applicant"
                description={`Directory grouping within ${realm.display_name}. Add more on the Realms page.`}
                data={(categories ?? []).map((c) => ({ value: c.id, label: c.display_name }))}
                value={f.state.value || null} onChange={(v) => f.handleChange(v ?? '')}
              />
            )}
          </form.Field>
        )}

        <form.Field name="email">
          {(f) => (
            <TextInput label="Email" placeholder="hartono@example.com" type="email"
              value={f.state.value} onChange={(e) => f.handleChange(e.currentTarget.value)} />
          )}
        </form.Field>

        <form.Field name="assurance">
          {(f) => (
            <Select
              label="Identity assurance" required
              description={realm ? `This realm's floor is IAL${realm.min_assurance}.` : undefined}
              data={IAL_OPTIONS.map((o) => ({
                ...o,
                disabled: realm ? Number(o.value) < realm.min_assurance : false,
              }))}
              value={f.state.value} onChange={(v) => f.handleChange(v ?? '')}
            />
          )}
        </form.Field>
      </form>
    </CreateShell>
  )
}
