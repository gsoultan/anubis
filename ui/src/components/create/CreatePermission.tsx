import { useQuery } from '@tanstack/react-query'
import { useForm } from '@tanstack/react-form'
import { MultiSelect, Select, TextInput } from '@mantine/core'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/client'
import { useCreate } from '@/stores/create'
import { CreateShell, CancelSubmit, notifyCreated, notifyRejected } from './shell'
import type { Ial, Risk } from '@/lib/api/types'

const snake = ({ value }: { value: string }) =>
  value && !/^[a-z][a-z0-9_]{1,30}$/.test(value)
    ? 'Lowercase snake_case, starting with a letter.' : undefined

export function CreatePermission({ opened }: { opened: boolean }) {
  const { close } = useCreate()
  const { data: apps } = useQuery({ queryKey: ['applications'], queryFn: api.applications })

  const form = useForm({
    defaultValues: {
      app_slug: '', resource: '', action: '', description: '',
      risk: 'normal', min_assurance: '1', requires_amr: [] as string[], max_auth_age: '5m',
    },
    onSubmit: async ({ value }) => {
      try {
        const created = await api.createPermission({
          app_slug: value.app_slug, resource: value.resource, action: value.action,
          description: value.description, risk: value.risk as Risk,
          min_assurance: Number(value.min_assurance) as Ial,
          requires_amr: value.requires_amr,
          max_auth_age: value.requires_amr.length ? value.max_auth_age : null,
        })
        notifyCreated('Permission registered', created.key)
        await queryClient.invalidateQueries({ queryKey: qk.permissions() })
        form.reset(); close()
      } catch (e) { notifyRejected(e) }
    },
  })

  const v = form.state.values
  const preview = v.app_slug && v.resource && v.action
    ? `${v.app_slug}:${v.resource}:${v.action}` : null

  return (
    <CreateShell
      opened={opened} onClose={close} title="Add a permission"
      description={<>Permissions are owned by applications and namespaced by them, so
        <b> billing:invoice:approve</b> and <b>procure:invoice:approve</b> can coexist.
        In production these arrive via the application manifest.</>}
      footer={
        <form.Subscribe selector={(s) => [s.canSubmit, s.isSubmitting] as const}>
          {([canSubmit, isSubmitting]) => (
            <CancelSubmit onCancel={close} canSubmit={!!canSubmit && !!preview}
              submitting={!!isSubmitting} label="Add permission" />
          )}
        </form.Subscribe>
      }
    >
      <form onSubmit={(e) => { e.preventDefault(); void form.handleSubmit() }}
        className="flex flex-col gap-4">
        <form.Field name="app_slug">
          {(f) => (
            <Select label="Application" required placeholder="Which app owns this?"
              data={(apps ?? []).map((a) => ({ value: a.slug, label: `${a.name} (${a.slug})` }))}
              value={f.state.value} onChange={(x) => f.handleChange(x ?? '')} />
          )}
        </form.Field>

        <div className="grid grid-cols-2 gap-3">
          <form.Field name="resource" validators={{ onChange: snake }}>
            {(f) => (
              <TextInput label="Resource" placeholder="invoice" required
                value={f.state.value} onChange={(e) => f.handleChange(e.currentTarget.value)}
                error={f.state.meta.errors[0]} />
            )}
          </form.Field>
          <form.Field name="action" validators={{ onChange: snake }}>
            {(f) => (
              <TextInput label="Action" placeholder="approve" required
                value={f.state.value} onChange={(e) => f.handleChange(e.currentTarget.value)}
                error={f.state.meta.errors[0]} />
            )}
          </form.Field>
        </div>

        {preview && (
          <div className="panel-inset flex items-center justify-between px-3 py-2.5">
            <span className="t-xs">key (generated)</span>
            <span className="chip chip-gold">{preview}</span>
          </div>
        )}

        <form.Field name="description">
          {(f) => (
            <TextInput label="Description" placeholder="What does holding this allow?"
              value={f.state.value} onChange={(e) => f.handleChange(e.currentTarget.value)} />
          )}
        </form.Field>

        <div className="grid grid-cols-2 gap-3">
          <form.Field name="risk">
            {(f) => (
              <Select label="Risk"
                data={[
                  { value: 'normal', label: 'normal' },
                  { value: 'sensitive', label: 'sensitive' },
                  { value: 'critical', label: 'critical' },
                ]}
                value={f.state.value} onChange={(x) => f.handleChange(x ?? 'normal')} />
            )}
          </form.Field>
          <form.Field name="min_assurance">
            {(f) => (
              <Select label="Minimum assurance"
                description="Denied below this even with a grant"
                data={[{ value: '1', label: 'IAL1' }, { value: '2', label: 'IAL2' }, { value: '3', label: 'IAL3' }]}
                value={f.state.value} onChange={(x) => f.handleChange(x ?? '1')} />
            )}
          </form.Field>
        </div>

        <form.Field name="requires_amr">
          {(f) => (
            <MultiSelect label="Step-up factors" placeholder="None — no step-up"
              description="If set, the session must have authenticated with these recently."
              data={[{ value: 'otp', label: 'otp — TOTP code' }, { value: 'device_key', label: 'device_key — biometric' }]}
              value={f.state.value} onChange={(x) => f.handleChange(x)} />
          )}
        </form.Field>

        {form.state.values.requires_amr.length > 0 && (
          <form.Field name="max_auth_age">
            {(f) => (
              <Select label="Maximum authentication age"
                data={['2m', '5m', '10m', '30m'].map((x) => ({ value: x, label: `within ${x}` }))}
                value={f.state.value} onChange={(x) => f.handleChange(x ?? '5m')} />
            )}
          </form.Field>
        )}
      </form>
    </CreateShell>
  )
}
