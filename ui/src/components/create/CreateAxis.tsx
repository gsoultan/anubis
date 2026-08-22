import { useForm } from '@tanstack/react-form'
import { Select, TextInput } from '@mantine/core'
import { IconInfoCircle } from '@tabler/icons-react'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/client'
import { useCreate } from '@/stores/create'
import { CreateShell, CancelSubmit, notifyCreated, notifyRejected } from './shell'
import type { AxisDefaultEffect } from '@/lib/api/types'

const ICONS = ['tag', 'building', 'truck', 'box', 'users', 'wallet', 'location']

export function CreateAxis({ opened }: { opened: boolean }) {
  const { close } = useCreate()

  const form = useForm({
    defaultValues: {
      code: '', display_name: '', default_effect: 'unconstrained',
      resolution_from: 'context', picker: 'tree', icon: 'tag',
    },
    onSubmit: async ({ value }) => {
      try {
        const axis = await api.createAxis({
          code: value.code, display_name: value.display_name,
          default_effect: value.default_effect as AxisDefaultEffect,
          resolution_from: value.resolution_from as 'token' | 'context',
          resolution_key: `${value.code}_id`,
          picker: value.picker as 'tree' | 'select' | 'search',
          icon: value.icon,
        })
        notifyCreated(`Structure "${axis.display_name}" added`,
          'It now appears in every picker — add items to it next.')
        await queryClient.invalidateQueries({ queryKey: qk.axes() })
        await queryClient.invalidateQueries({ queryKey: qk.nodeTypes() })
        await queryClient.invalidateQueries({ queryKey: qk.scope() })
        form.reset(); close()
      } catch (e) { notifyRejected(e) }
    },
  })

  return (
    <CreateShell
      opened={opened} onClose={close} title="Add a structure"
      description={<>A whole new way to limit access — by cost centre, by project, by region. Ready the moment you add it; nothing existing changes until you start using it.</>}
      footer={
        <form.Subscribe selector={(s) => [s.canSubmit, s.isSubmitting] as const}>
          {([canSubmit, isSubmitting]) => (
            <CancelSubmit onCancel={close}
              canSubmit={!!canSubmit && !!form.state.values.code && !!form.state.values.display_name}
              submitting={!!isSubmitting} label="Add structure" />
          )}
        </form.Subscribe>
      }
    >
      <form onSubmit={(e) => { e.preventDefault(); void form.handleSubmit() }}
        className="flex flex-col gap-4">
        <form.Field name="code" validators={{
          onChange: ({ value }) =>
            value && !/^[a-z][a-z0-9_]{1,30}$/.test(value)
              ? 'Lowercase snake_case. The underscore prefix is reserved for keys like _owner.'
              : undefined,
        }}>
          {(f) => (
            <TextInput label="Code" placeholder="cost_center" required
              description="Immutable — used as the target-map key in tokens and API calls."
              value={f.state.value} onChange={(e) => f.handleChange(e.currentTarget.value)}
              error={f.state.meta.errors[0]} />
          )}
        </form.Field>

        <form.Field name="display_name">
          {(f) => (
            <TextInput label="Display name" placeholder="Cost Centre" required
              value={f.state.value} onChange={(e) => f.handleChange(e.currentTarget.value)} />
          )}
        </form.Field>

        <form.Field name="default_effect">
          {(f) => (
            <Select label="Default effect"
              description="Start unconstrained; flip to strict only after the dry run."
              data={[
                { value: 'unconstrained', label: 'unconstrained — grants silent on this axis still pass' },
                { value: 'deny', label: 'deny — every grant must address this axis' },
              ]}
              value={f.state.value} onChange={(v) => f.handleChange(v ?? 'unconstrained')} />
          )}
        </form.Field>

        <div className="grid grid-cols-2 gap-3">
          <form.Field name="resolution_from">
            {(f) => (
              <Select label="Target resolved from"
                data={[
                  { value: 'token', label: 'token — session scope' },
                  { value: 'context', label: 'context — per request' },
                ]}
                value={f.state.value} onChange={(v) => f.handleChange(v ?? 'context')} />
            )}
          </form.Field>
          <form.Field name="icon">
            {(f) => (
              <Select label="Icon" data={ICONS}
                value={f.state.value} onChange={(v) => f.handleChange(v ?? 'tag')} />
            )}
          </form.Field>
        </div>

        <div className="panel-inset flex items-start gap-2.5 px-3 py-2.5">
          <IconInfoCircle size={14} style={{ color: 'var(--ink-3)', marginTop: 1, flexShrink: 0 }} />
          <span className="t-xs">
            Registration provisions the axis root (“All …”) and an item type, so you can add
            nodes immediately after — or point your ERP sync at it.
          </span>
        </div>
      </form>
    </CreateShell>
  )
}
