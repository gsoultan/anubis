import { useQuery } from '@tanstack/react-query'
import { Popover, Tooltip, ActionIcon } from '@mantine/core'
import { IconX, IconChevronDown, IconAlertTriangle } from '@tabler/icons-react'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { AxisIcon } from './AxisIcon'
import { ScopeTree } from './ScopeTree'
import type { ScopeAxis, ScopeNode } from '@/lib/api/types'

/* ===========================================================================
   THE SCHEMA-DRIVEN SCOPE CONTROL
   ===========================================================================
   The backend's defining property is that a new access dimension is an INSERT,
   not a deploy (ADR-0003). A console that hardcoded axis names would destroy
   that at the presentation layer: the database would accept a `cost_center`
   axis instantly and the UI would need a release to show it.

   So this enumerates whatever GET /v1/admin/scope-axes returns and renders each
   axis from its own `ui_schema`. Axis codes are compared only against variables
   ("the axis being rendered"), never literals — no branch anywhere names a
   specific axis.
   =========================================================================== */

function AxisRow({ axis, value, onChange, warn }: {
  axis: ScopeAxis
  value: string | null
  onChange: (id: string | null) => void
  warn: boolean
}) {
  const { data: node } = useQuery({
    queryKey: qk.scopeNode(value ?? ''), queryFn: () => api.scopeNode(value!), enabled: !!value,
  })
  const { data: path } = useQuery({
    queryKey: qk.ancestorPath(value ?? ''), queryFn: () => api.ancestorPath(value!), enabled: !!value,
  })

  const strict = axis.default_effect === 'deny'
  const unset = !value
  const flag = unset && warn

  return (
    <div
      className="panel px-3 py-2.5"
      style={flag ? { borderColor: 'color-mix(in srgb, var(--warn) 24%, transparent)', background: 'linear-gradient(180deg, var(--warn-bg), var(--s-raised) 70%)' } : undefined}
    >
      <div className="mb-2 flex items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          <span style={{ color: 'var(--ink-3)', display: 'flex' }}>
            <AxisIcon name={axis.ui_schema.icon} size={14} />
          </span>
          <span className="t-body truncate" style={{ fontWeight: 550 }}>{axis.display_name}</span>
          {strict && (
            <Tooltip label="default_effect = deny. A grant that does not address this axis is denied.">
              <span className="chip" style={{ color: 'var(--deny)', borderColor: 'color-mix(in srgb, var(--deny) 20%, transparent)' }}>strict</span>
            </Tooltip>
          )}
        </div>
        <div className="flex shrink-0 items-center gap-1.5">
          <Tooltip label={axis.resolution.from === 'token'
            ? 'Resolved from the session token (active scope).'
            : `Supplied per request by the calling application as "${axis.resolution.key}".`}>
            <span className="chip" style={{ fontSize: 9.5 }}>
              {axis.resolution.from === 'token' ? 'token' : `ctx:${axis.resolution.key}`}
            </span>
          </Tooltip>
          {value && (
            <ActionIcon size="xs" variant="subtle" color="gray"
              onClick={() => onChange(null)} aria-label={`Clear ${axis.display_name}`}>
              <IconX size={12} />
            </ActionIcon>
          )}
        </div>
      </div>

      <Popover width={340} position="bottom-start">
        <Popover.Target>
          <button
            className="flex w-full items-center justify-between gap-2 rounded-md px-2.5 py-2 text-left"
            style={{
              background: 'var(--s-sunken)',
              border: `1px solid ${flag ? 'color-mix(in srgb, var(--warn) 33%, transparent)' : 'var(--line-soft)'}`,
              transition: 'border-color var(--t-fast)',
            }}
          >
            {node ? (
              <span className="min-w-0">
                <span className="t-body block truncate" style={{ fontWeight: 500 }}>{node.name}</span>
                {path && path.length > 1 && (
                  <span className="t-xs block truncate" style={{ fontSize: 10 }}>
                    {path.map((p) => p.name).join(' › ')}
                  </span>
                )}
              </span>
            ) : (
              <span className="t-sm" style={{ color: 'var(--ink-3)' }}>Not set</span>
            )}
            <IconChevronDown size={13} style={{ color: 'var(--ink-3)', flexShrink: 0 }} />
          </button>
        </Popover.Target>
        <Popover.Dropdown p="xs">
          {axis.ui_schema.help && <div className="t-xs mb-2">{axis.ui_schema.help}</div>}
          <ScopeTree axis={axis.code} selectedId={value}
            onSelect={(n: ScopeNode) => onChange(n.id)}
            searchable={axis.ui_schema.searchable ?? true} />
        </Popover.Dropdown>
      </Popover>

      {flag && (
        <div className="mt-2 flex items-start gap-1.5">
          <IconAlertTriangle size={11} style={{ color: 'var(--warn)', marginTop: 2, flexShrink: 0 }} />
          <span className="t-xs" style={{ color: 'var(--warn)' }}>
            Unresolved. If a grant constrains this axis the decision is <b>denied</b>, not ignored.
          </span>
        </div>
      )}
    </div>
  )
}

export function AxisTargetPicker({
  targets, onChange, showUnsetWarning = false, ownerValue, onOwnerChange,
}: {
  targets: Record<string, string>
  onChange: (axis: string, nodeId: string | null) => void
  showUnsetWarning?: boolean
  ownerValue?: string | null
  onOwnerChange?: (v: string | null) => void
}) {
  const { data: axes, isLoading } = useQuery({ queryKey: qk.axes(), queryFn: api.axes })

  if (isLoading) {
    return (
      <div className="flex flex-col gap-2">
        {[0, 1, 2].map((i) => (
          <div key={i} className="panel h-[86px] animate-pulse" style={{ opacity: 0.35 }} />
        ))}
      </div>
    )
  }
  if (!axes?.length) {
    return (
      <div className="panel px-4 py-6 text-center">
        <div className="t-sm">No active scope axes are registered.</div>
        <div className="t-xs mt-1">
          Register one under Scope and it appears here immediately — no deployment.
        </div>
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-2">
      {axes.map((a) => (
        <AxisRow key={a.code} axis={a} value={targets[a.code] ?? null}
          onChange={(v) => onChange(a.code, v)} warn={showUnsetWarning} />
      ))}
      {onOwnerChange && (
        <div className="panel px-3 py-2.5">
          <div className="mb-1.5 flex items-center gap-2">
            <span style={{ color: 'var(--ink-3)', display: 'flex' }}><AxisIcon name="tag" size={14} /></span>
            <span className="t-body" style={{ fontWeight: 550 }}>Record owner</span>
            <Tooltip label="Reserved target key. Axis codes may never begin with an underscore, so this cannot collide with a real axis.">
              <span className="chip">_owner</span>
            </Tooltip>
          </div>
          <div className="t-xs">Only consulted for self-scoped grants. Absent ⇒ denied.</div>
          <div className="mt-1.5 font-mono" style={{ fontSize: 11 }}>
            {ownerValue ?? <span className="t-xs">Not set</span>}
          </div>
        </div>
      )}
    </div>
  )
}
