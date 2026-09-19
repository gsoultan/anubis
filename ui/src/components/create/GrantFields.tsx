import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Popover, SegmentedControl, Select, Switch, Tooltip } from '@mantine/core'
import { IconChevronDown, IconX } from '@tabler/icons-react'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/client'
import { ScopeTree } from '@/components/scope/ScopeTree'
import { AxisIcon } from '@/components/scope/AxisIcon'
import { notifyCreated, notifyRejected } from './shell'
import type { GrantScope } from '@/lib/api/types'

/* The grant form, in one place.
 *
 * It is built in two: the drawer that gives access from anywhere, and the
 * panel on a person's own page. Both have to teach the two things operators
 * get wrong, so both render the same fields:
 *
 * 1. Role options are FILTERED by the subject's population — the blocked
 *    entries stay visible with the reason, because an invisible option reads
 *    as a bug while a disabled one reads as a rule.
 * 2. Axis constraints and self-scoped are mutually exclusive, and the toggle
 *    physically disables the other half rather than letting submit fail.
 * 3. An exclusion is a carve-out of the places beside it, and it narrows THIS
 *    grant only. Both halves of that get said on screen, because the second
 *    half is the one operators assume the other way round: they expect
 *    "exclude Surabaya" to mean nobody reaches Surabaya, and it does not.
 *
 * Written twice they would drift, and the same form would teach two different
 * rules depending on which one you happened to open — this console's recurring
 * failure, in miniature.
 */

export type NodeSel = { id: string; name: string; inherit: boolean; exclude: boolean }

/* Includes first, then exclusions: the order the sentence is read in (here,
   except there) and the order ListGrantScopes returns them in, so the form and
   the grant it produces list the same places the same way. */
const byMode = (a: NodeSel, b: NodeSel) =>
  Number(a.exclude) - Number(b.exclude) || a.name.localeCompare(b.name)

/** An exclusion with nothing to exclude from is refused by the database
    (migration 0046). Saying so here means a sentence instead of a constraint
    error, and it is the same rule either way. */
export function axisNeedsAnInclude(values: NodeSel[]): boolean {
  return values.length > 0 && values.every((v) => v.exclude)
}

export function AxisConstraintRow({ axisCode, displayName, icon, values, onChange }: {
  axisCode: string
  displayName: string
  icon: string | undefined
  values: NodeSel[]
  onChange: (v: NodeSel[]) => void
}) {
  /* A node arrives as an include. Nobody opens a tree meaning to carve
     something out of nothing, and the one shape the database refuses is an
     axis that holds only exclusions — so the default can never create one. */
  const toggle = (id: string, name: string) => {
    onChange(values.some((v) => v.id === id)
      ? values.filter((v) => v.id !== id)
      : [...values, { id, name, inherit: true, exclude: false }].sort(byMode))
  }
  const update = (id: string, patch: Partial<NodeSel>) =>
    onChange(values.map((x) => (x.id === id ? { ...x, ...patch } : x)).sort(byMode))

  const includes = values.filter((v) => !v.exclude)
  const excludes = values.filter((v) => v.exclude)
  const orphanExclusions = axisNeedsAnInclude(values)

  return (
    <div className="panel-inset px-3 py-2.5">
      <div className="mb-1.5 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span style={{ color: 'var(--ink-3)', display: 'flex' }}><AxisIcon name={icon} size={13} /></span>
          <span className="t-body" style={{ fontWeight: 530 }}>{displayName}</span>
          {includes.length > 1 && (
            <Tooltip label="Any of these places is enough — they are OR, not AND.">
              <span className="chip">any of {includes.length}</span>
            </Tooltip>
          )}
          {excludes.length > 0 && (
            <Tooltip label="Carved out of the places above, for this grant only.">
              <span className="chip" style={{
                color: 'var(--deny)',
                borderColor: 'color-mix(in srgb, var(--deny) 24%, transparent)',
              }}>except {excludes.length}</span>
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
                : includes.length === 1 && excludes.length === 0 ? includes[0]!.name
                : `${includes.length} place${includes.length === 1 ? '' : 's'}`
                  + (excludes.length ? `, except ${excludes.length}` : '')}
            </span>
            <IconChevronDown size={12} style={{ color: 'var(--ink-3)', flexShrink: 0 }} />
          </button>
        </Popover.Target>
        <Popover.Dropdown p="xs">
          <div className="t-xs mb-1.5">
            Click to add or remove — the person gets access in <b>any</b> of them.
            Switch one to <b>Exclude</b> to carve it back out.
          </div>
          <ScopeTree axis={axisCode} selectedId={values[values.length - 1]?.id ?? null}
            onSelect={(n) => toggle(n.id, n.name)} />
        </Popover.Dropdown>
      </Popover>

      {values.length > 0 && (
        <div className="mt-2 flex flex-col gap-1">
          {values.map((v) => (
            <div key={v.id} className="rounded-md px-2 py-1.5"
              style={v.exclude ? {
                background: 'var(--deny-bg)',
                border: '1px solid color-mix(in srgb, var(--deny) 24%, transparent)',
              } : {
                background: 'var(--s-base)', border: '1px solid var(--line-soft)',
              }}>
              <div className="flex items-center justify-between gap-2">
                <span className="t-body min-w-0 truncate"
                  style={v.exclude ? { color: 'var(--deny)' } : undefined}>{v.name}</span>
                <button onClick={() => onChange(values.filter((x) => x.id !== v.id))}
                  aria-label={`Remove ${v.name}`}
                  style={{ color: 'var(--ink-3)', display: 'flex', flexShrink: 0 }}>
                  <IconX size={12} />
                </button>
              </div>
              <div className="mt-1.5 flex items-center justify-between gap-2">
                <SegmentedControl size="xs" value={v.exclude ? 'exclude' : 'include'}
                  onChange={(m) => update(v.id, { exclude: m === 'exclude' })}
                  aria-label={`Include or exclude ${v.name}`}
                  data={[{ label: 'Include', value: 'include' }, { label: 'Exclude', value: 'exclude' }]} />
                <Tooltip label={v.exclude
                  ? 'On: this place and everything inside it is carved out. Off: exactly this place, its children stay.'
                  : 'On: this place and everything inside it. Off: exactly this place.'}>
                  <Switch size="xs" label="and inside" checked={v.inherit}
                    onChange={(e) => update(v.id, { inherit: e.currentTarget.checked })} />
                </Tooltip>
              </div>
            </div>
          ))}
        </div>
      )}

      {orphanExclusions && (
        <div className="t-xs mt-2" style={{ color: 'var(--deny)' }}>
          An exclusion carves out of somewhere. Add a place to <b>Include</b> on
          this axis, or remove the exclusion — the database refuses this either way.
        </div>
      )}

      {excludes.length > 0 && !orphanExclusions && (
        <div className="t-xs mt-2">
          Carved out of <b>this grant</b> only. Another grant covering the same
          place still applies there — this is not a deny rule.
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

/* State, validation and the write. The caller owns layout — a drawer with a
   sticky footer, or a panel on a page — so the submit button lives out there
   and reads `canSubmit`/`submitting` from here. */
export function useGrantDraft({ identityId, onCreated }: {
  identityId: string | null
  onCreated?: (() => void) | undefined
}) {
  const { data: realms } = useQuery({ queryKey: qk.realms(), queryFn: api.realms })
  const { data: roles } = useQuery({ queryKey: qk.roles(), queryFn: api.roles })
  const { data: axes } = useQuery({ queryKey: qk.axes(), queryFn: api.axes })

  const [roleId, setRoleId] = useState<string | null>(null)
  const [selfScoped, setSelfScoped] = useState(false)
  const [validityDays, setValidityDays] = useState('')
  const [constraints, setConstraints] = useState<Record<string, NodeSel[]>>({})
  const [submitting, setSubmitting] = useState(false)

  /* One person, fetched when picked — not the whole directory to find them.
     Keyed the way every other screen keys an identity, so a person's own page
     and this form share the single fetch instead of making two. */
  const { data: subject } = useQuery({
    queryKey: qk.identity(identityId ?? ''),
    queryFn: () => api.identity(identityId as string),
    enabled: !!identityId,
  })
  const subjectKind = realms?.find((r) => r.id === subject?.realm_id)?.kind

  /* Both reasons a role cannot be granted, said in the option rather than
     discovered on submit. Retirement comes first because it is absolute: the
     grants_role_live trigger (0044) refuses the insert whoever the subject is. */
  const roleOptions = (roles ?? []).map((r) => {
    const why = r.deprecated
      ? 'retired from the catalog'
      : (!!subjectKind && !r.allowed_realm_kinds.includes(subjectKind))
          ? `not grantable to ${subjectKind}`
          : ''
    return { value: r.id, label: why ? `${r.name} — ${why}` : r.name, disabled: why !== '' }
  })

  // Switching subject to a population the chosen role cannot serve must clear
  // the role, or the form submits into a guaranteed guard rejection.
  useEffect(() => {
    if (!roleId) return
    const role = roles?.find((r) => r.id === roleId)
    if (!role) return
    if (role.deprecated) { setRoleId(null); return }
    if (subjectKind && !role.allowed_realm_kinds.includes(subjectKind)) setRoleId(null)
  }, [subjectKind, roleId, roles])

  const reset = () => {
    setRoleId(null); setSelfScoped(false); setValidityDays(''); setConstraints({})
  }

  async function submit() {
    if (!identityId || !roleId) return
    setSubmitting(true)
    try {
      const scopes: GrantScope[] = selfScoped ? [] :
        Object.entries(constraints).flatMap(([axis_code, list]) =>
          list.map((c) => ({
            axis_code, scope_node_id: c.id, inherit: c.inherit, exclude: c.exclude,
          })))
      const validUntil = validityDays
        ? new Date(Date.now() + Number(validityDays) * 86_400_000).toISOString()
        : null
      await api.createGrant({
        identity_id: identityId, role_id: roleId,
        self_scoped: selfScoped, valid_until: validUntil, scopes,
      })
      /* The carve-outs get their own clause. "3 axis constraints" counts an
         exclusion as if it widened the grant, and the one thing an operator
         wants confirmed after writing a carve-out is that it was written. */
      const carved = scopes.filter((sc) => sc.exclude).length
      const pinned = scopes.length - carved
      notifyCreated('Access given',
        `${subject?.username} → ${roles?.find((r) => r.id === roleId)?.name ?? 'role'}${
          pinned ? ` across ${pinned} axis constraint${pinned > 1 ? 's' : ''}` +
                   (carved ? `, ${carved} place${carved > 1 ? 's' : ''} carved out` : '')
          : selfScoped ? ' (own records only)' : ' (unconstrained)'}`)
      /* One prefix covers the Access screen and a person's own access list,
         which is why both are keyed under 'grants'. Two key shapes is how a
         page ends up showing the grant it just wrote as still absent. */
      await queryClient.invalidateQueries({ queryKey: ['grants'] })
      await queryClient.invalidateQueries({ queryKey: qk.dashboard() })
      reset()
      onCreated?.()
    } catch (e) { notifyRejected(e) }
    setSubmitting(false)
  }

  return {
    subject: subject ?? null, subjectKind, axes, roleOptions,
    roleId, setRoleId,
    selfScoped, setSelfScoped,
    validityDays, setValidityDays,
    constraints, setConstraints,
    /* An axis holding only exclusions is refused by the database, so the
       button refuses it first — the drawer says which axis and why, where the
       constraint error would only name a trigger. */
    submitting,
    canSubmit: !!identityId && !!roleId
      && !Object.values(constraints).some(axisNeedsAnInclude),
    submit, reset,
  }
}

export type GrantDraft = ReturnType<typeof useGrantDraft>

/** Role, validity, self-scoped, axis constraints. The subject is the caller's
    problem: the drawer picks one, a person's page already is one. */
export function GrantFields({ draft }: { draft: GrantDraft }) {
  const {
    subject, subjectKind, axes, roleOptions,
    roleId, setRoleId, selfScoped, setSelfScoped,
    validityDays, setValidityDays, constraints, setConstraints,
  } = draft

  return (
    <div className="flex flex-col gap-4">
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
  )
}
