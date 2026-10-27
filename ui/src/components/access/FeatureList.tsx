import { Tooltip } from '@mantine/core'
import { IconShieldLock } from '@tabler/icons-react'
import { groupFeatures } from '@/lib/access'
import type { Permission } from '@/lib/api/types'

const RISK_COLOR: Record<string, string> = {
  normal: 'var(--ink-2)', sensitive: 'var(--warn)', critical: 'var(--deny)',
}

function Action({ p }: { p: Permission }) {
  const extra = [
    p.min_assurance > 1 ? `needs assurance level ${p.min_assurance}` : '',
    p.requires_amr.length ? `asks for ${p.requires_amr.join(' or ')} again within ${p.max_auth_age}` : '',
  ].filter(Boolean)
  return (
    <Tooltip label={<span>{p.description || p.key}{extra.length > 0 && <><br />{extra.join(', ')}</>}</span>} multiline w={300}>
      <span className="chip inline-flex items-center gap-1" style={{ color: RISK_COLOR[p.risk], cursor: 'help' }}>
        {p.requires_amr.length > 0 && <IconShieldLock size={11} />}
        {p.action}
      </span>
    </Tooltip>
  )
}

/* One app's permissions the way people look for them: the part of the app,
   the feature, then the actions on it. Read-only; the app's manifest is
   where they are declared. */
export function FeatureList({ perms }: { perms: Permission[] }) {
  const mods = groupFeatures(perms)
  if (mods.length === 0) return <div className="t-sm">This app declares no permissions yet.</div>
  return (
    <div className="flex flex-col gap-3">
      {mods.map((m) => (
        <div key={m.module || 'other'} className="flex flex-col gap-1">
          <div className="t-label">
            {m.module || (mods.length > 1 ? 'Other' : 'Features')}
            <span className="ml-2 normal-case" style={{ fontWeight: 400, letterSpacing: 0 }}>
              {m.features.length} {m.features.length === 1 ? 'feature' : 'features'} · {m.features.reduce((n, f) => n + f.actions.length, 0)} actions
            </span>
          </div>
          <div className="panel-inset flex flex-col">
            {m.features.map((f) => (
              <div key={f.resource} className="flex flex-wrap items-center gap-x-3 gap-y-1.5 px-3 py-2"
                style={{ borderBottom: '1px solid var(--line-soft)' }}>
                <span className="t-body min-w-[140px] flex-1" style={{ fontWeight: 550 }}>{f.resource}</span>
                <span className="flex flex-wrap gap-1.5">{f.actions.map((p) => <Action key={p.id} p={p} />)}</span>
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}
