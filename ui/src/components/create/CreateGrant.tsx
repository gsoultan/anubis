import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Popover, Select, Switch, Tooltip } from '@mantine/core'
import { IconChevronDown, IconX } from '@tabler/icons-react'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/client'
import { useCreate } from '@/stores/create'
import { ScopeTree } from '@/components/scope/ScopeTree'
import { AxisIcon } from '@/components/scope/AxisIcon'
import { CreateShell, CancelSubmit, notifyCreated, notifyRejected } from './shell'
import type { GrantScope } from '@/lib/api/types'

/* The grant form carries the two ideas people get wrong, so the form itself
   teaches them:
   1. Role options are FILTERED by the subject's realm — the disabled entries
      stay visible with the reason, because an invisible option reads as a bug
      while a disabled one reads as a rule.
   2. Axis constraints and self-scoped are mutually exclusive, and the toggle
      physically disables the other half rather than letting submit fail. */

export type NodeSel = { id: string; name: string; inherit: boolean }

export function AxisConstraintRow({ axisCode, displayName, icon, values, onChange }: {
  axisCode: string
  displayName: string
  icon: string | undefined
  values: NodeSel[]
  onChange: (v: NodeSel[]) => void
}) {
  const toggle = (id: string, name: string) => {
    onChange(values.some((v) => v.id === id)
      ? values.filter((v) => v.id !== id)
      : [...values, { id, name, inherit: true }])
  }
  return (
    <div className="panel-inset px-3 py-2.5">
      <div className="mb-1.5 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span style={{ color: 'var(--ink-3)', display: 'flex' }}><AxisIcon name={icon} size={13} /></span>
          <span className="t-body" style={{ fontWeight: 530 }}>{displayName}</span>
          {values.length > 1 && (
            <Tooltip label="Any of these places is enough — they are OR, not AND.">
              <span className="chip">any of {values.length}</span>
            </Tooltip>
          )}
        </div>
        {values.length > 0 && (
          <button onClick={() => onChange([])} aria-label={`Clear ${displayName}`}
            style={{ color: 'var(--ink-3)', display: 'flex' }}>
            <IconX size={13} />
          </button>
        )}
      </div>

      <Popover width={330} position="left-start" closeOnClickOutside>
        <Popover.Target>
          <button className="flex w-full items-center justify-between gap-2 rounded-md px-2.5 py-1.5 text-left"
            style={{ background: 'var(--s-base)', border: '1px solid var(--line-soft)' }}>
            <span className="t-body truncate" style={{ color: values.length ? 'var(--ink)' : 'var(--ink-3)' }}>
              {values.length === 0 ? 'Anywhere'
                : values.length === 1 ? values[0]!.name
                : `${values.length} places — click to add more`}
            </span>
            <IconChevronDown size={12} style={{ color: 'var(--ink-3)', flexShrink: 0 }} />
          </button>
        </Popover.Target>
        <Popover.Dropdown p="xs">
          <div className="t-xs mb-1.5">Click to add or remove — the person gets access in <b>any</b> of them.</div>
          <ScopeTree axis={axisCode} selectedId={values[values.length - 1]?.id ?? null}
            onSelect={(n) => toggle(n.id, n.name)} />
        </Popover.Dropdown>
      </Popover>

      {values.length > 0 && (
        <div className="mt-2 flex flex-col gap-1">
          {values.map((v) => (
            <div key={v.id} className="flex items-center justify-between gap-2 rounded-md px-2 py-1"
              style={{ background: 'var(--s-base)', border: '1px solid var(--line-soft)' }}>
              <span className="t-body min-w-0 truncate">{v.name}</span>
              <div className="flex shrink-0 items-center gap-2">
                <Tooltip label="On: this place and everything inside it. Off: exactly this place.">
                  <Switch size="xs" label="include inside" checked={v.inherit}
                    onChange={(e) => onChange(values.map((x) =>
                      x.id === v.id ? { ...x, inherit: e.currentTarget.checked } : x))} />
                </Tooltip>
                <button onClick={() => onChange(values.filter((x) => x.id !== v.id))}
                  aria-label={`Remove ${v.name}`} style={{ color: 'var(--ink-3)', display: 'flex' }}>
                  <IconX size={12} />
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

const VALIDITY = [
  { value: '', label: 'No expiry' },
  { value: '30', label: '30 days — short engagement' },
  { value: '90', label: '90 days — contractor default' },
  { value: '365', label: '1 year — annual review cycle' },
]

export function CreateGrant({ opened }: { opened: boolean }) {
  const { close, ctx } = useCreate()
  const { data: identities } = useQuery({ queryKey: qk.identities(), queryFn: () => api.identities() })
  const { data: realms } = useQuery({ queryKey: qk.realms(), queryFn: api.realms })
  const { data: roles } = useQuery({ queryKey: qk.roles(), queryFn: api.roles })
  const { data: axes } = useQuery({ queryKey: qk.axes(), queryFn: api.axes })

  const [identityId, setIdentityId] = useState<string | null>(null)
  const [roleId, setRoleId] = useState<string | null>(null)
  const [selfScoped, setSelfScoped] = useState(false)
  const [validityDays, setValidityDays] = useState('')
  const [constraints, setConstraints] = useState<Record<string, NodeSel[]>>({})
  const [submitting, setSubmitting] = useState(false)

  // Preload from context ("grant a role to THIS identity" from a row action).
  useEffect(() => {
    if (opened && ctx.identityId) setIdentityId(ctx.identityId)
  }, [opened, ctx.identityId])

  const subject = identities?.find((i) => i.id === identityId)
  const subjectKind = realms?.find((r) => r.id === subject?.realm_id)?.kind

  const roleOptions = (roles ?? []).map((r) => {
    const blocked = !!subjectKind && !r.allowed_realm_kinds.includes(subjectKind)
    return {
      value: r.id,
      label: blocked ? `${r.name} — not grantable to ${subjectKind}` : r.name,
      disabled: blocked,
    }
  })

  // Switching subject to a realm the chosen role cannot serve must clear the
  // role, or the form submits into a guaranteed guard rejection.
  useEffect(() => {
    if (!roleId || !subjectKind) return
    const role = roles?.find((r) => r.id === roleId)
    if (role && !role.allowed_realm_kinds.includes(subjectKind)) setRoleId(null)
  }, [subjectKind, roleId, roles])

  const reset = () => {
    setIdentityId(null); setRoleId(null); setSelfScoped(false)
    setValidityDays(''); setConstraints({})
  }

  async function submit() {
    if (!identityId || !roleId) return
    setSubmitting(true)
    try {
      const scopes: GrantScope[] = selfScoped ? [] :
        Object.entries(constraints).flatMap(([axis_code, list]) =>
          list.map((c) => ({ axis_code, scope_node_id: c.id, inherit: c.inherit })))
      const validUntil = validityDays
        ? new Date(Date.now() + Number(validityDays) * 86_400_000).toISOString()
        : null
      const g = await api.createGrant({
        identity_id: identityId, role_id: roleId,
        self_scoped: selfScoped, valid_until: validUntil, scopes,
      })
      notifyCreated('Access given',
        `${subject?.username} → ${g.role_name}${scopes.length ? ` across ${scopes.length} axis constraint${scopes.length > 1 ? 's' : ''}` : selfScoped ? ' (own records only)' : ' (unconstrained)'}`)
      await queryClient.invalidateQueries({ queryKey: ['grants'] })
      await queryClient.invalidateQueries({ queryKey: qk.dashboard() })
      reset(); close()
    } catch (e) { notifyRejected(e) }
    setSubmitting(false)
  }

  const grouped = (realms ?? []).map((r) => ({
    group: r.display_name,
    items: (identities ?? []).filter((i) => i.realm_id === r.id)
      .map((i) => ({ value: i.id, label: `${i.username}${i.email ? ` · ${i.email}` : ''}` })),
  })).filter((g) => g.items.length > 0)

  return (
    <CreateShell
      opened={opened} onClose={close} title="Give access"
      description={<>A grant is an identity, a role, and one constraint per axis.
        Axes the grant is silent on are unconstrained. Constraints are AND across
        axes.</>}
      footer={
        <CancelSubmit onCancel={close} canSubmit={!!identityId && !!roleId}
          submitting={submitting} label="Give access" />
      }
    >
      <div className="flex flex-col gap-4">
        <Select label="Who" placeholder="Pick a person" searchable required
          data={grouped} value={identityId} onChange={setIdentityId} />

        <div>
          <Select label="Role" placeholder={subject ? 'Pick a role' : 'Pick a person first'}
            searchable required disabled={!subject}
            data={roleOptions} value={roleId} onChange={setRoleId} />
          {subjectKind && (
            <div className="t-xs mt-1.5">
              Greyed-out roles are blocked by <span className="chip">allowed_realm_kinds</span> for
              a <b>{subjectKind}</b> identity — the same guard the database enforces.
            </div>
          )}
        </div>

        <Select label="Validity" data={VALIDITY} value={validityDays}
          onChange={(v) => setValidityDays(v ?? '')}
          description="Time-boxed access expires on its own — nobody has to remember to revoke it." />

        <div className="panel-inset flex items-center justify-between px-3 py-2.5">
          <div>
            <div className="t-body" style={{ fontWeight: 530 }}>Own records only</div>
            <div className="t-xs mt-0.5">self_scoped — for applicants and customers</div>
          </div>
          <Switch checked={selfScoped}
            onChange={(e) => { setSelfScoped(e.currentTarget.checked); setConstraints({}) }} />
        </div>

        <div style={selfScoped ? { opacity: 0.4, pointerEvents: 'none' } : undefined}>
          <div className="mb-2 flex items-baseline justify-between">
            <span className="t-body" style={{ fontWeight: 500 }}>Axis constraints</span>
            <span className="t-xs">{selfScoped ? 'excluded by self-scoped' : 'optional'}</span>
          </div>
          <div className="flex flex-col gap-2">
            {(axes ?? []).map((a) => (
              <AxisConstraintRow key={a.code} axisCode={a.code}
                displayName={a.display_name} icon={a.ui_schema.icon}
                values={constraints[a.code] ?? []}
                onChange={(list) => setConstraints((prev) => {
                  const next = { ...prev }
                  if (list.length === 0) delete next[a.code]
                  else next[a.code] = list
                  return next
                })} />
            ))}
          </div>
        </div>
      </div>
    </CreateShell>
  )
}
