import { useState } from 'react'
import { UnstyledButton } from '@mantine/core'
import { IconChevronRight, IconShieldLock } from '@tabler/icons-react'
import { groupFeatures } from '@/lib/access'
import type { Permission } from '@/lib/api/types'

const RISK: Record<string, string> = { sensitive: 'var(--warn)', critical: 'var(--deny)' }
const count = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`

/* An app's modules on the left, the chosen module's features and actions on
   the right. Each action shows the words people see and the key the app
   checks, since an admin talks in one and a developer in the other. */
export function AppModules({ perms, roleUse }: { perms: Permission[]; roleUse?: (module: string) => number }) {
  const mods = groupFeatures(perms)
  const [picked, setPicked] = useState(0)
  if (mods.length === 0) return <div className="panel px-4 py-6 text-center t-sm">This app declares no permissions yet.</div>
  const m = mods[Math.min(picked, mods.length - 1)]!
  const name = (mod: string) => mod || (mods.length > 1 ? 'Other' : 'Features')
  const actions = (mod: typeof m) => mod.features.reduce((n, f) => n + f.actions.length, 0)

  return (
    <div className="grid gap-4 md:grid-cols-[260px_minmax(0,1fr)]">
      <nav className="panel flex flex-col p-1.5" aria-label="Modules">
        <div className="t-label px-2 pb-1 pt-1.5">Modules</div>
        {mods.map((mod, i) => (
          <UnstyledButton key={mod.module || 'other'} onClick={() => setPicked(i)} aria-current={i === picked}
            className="flex items-center gap-2 rounded px-2.5 py-2"
            style={i === picked ? { background: 'var(--accent-bg)' } : undefined}>
            <span className="min-w-0 flex-1">
              <span className="t-body block" style={{ fontWeight: 550 }}>{name(mod.module)}</span>
              <span className="t-xs block">{count(mod.features.length, 'feature', 'features')} · {count(actions(mod), 'action', 'actions')}</span>
            </span>
            <IconChevronRight size={14} style={{ color: 'var(--ink-3)' }} />
          </UnstyledButton>
        ))}
      </nav>
      <section className="panel flex flex-col gap-3 p-4">
        <div>
          <h2 className="t-h2">{name(m.module)}</h2>
          <div className="t-xs">
            {count(m.features.length, 'feature', 'features')} · {count(actions(m), 'action', 'actions')}
            {roleUse && ` · used by ${count(roleUse(m.module), 'role', 'roles')}`}
          </div>
        </div>
        {m.features.map((f) => (
          <div key={f.resource} className="panel-inset flex flex-col gap-2 px-3 py-2.5">
            <div className="t-body" style={{ fontWeight: 570 }}>{f.resource}</div>
            <div className="flex flex-wrap gap-1.5">
              {f.actions.map((p) => (
                <span key={p.id} className="inline-flex items-baseline gap-1.5 rounded px-2 py-1"
                  style={{ background: 'var(--s-raised)', border: '1px solid var(--line-soft)' }}>
                  {p.requires_amr.length > 0 && <IconShieldLock size={11} style={{ alignSelf: 'center' }} />}
                  <span className="t-sm" style={{ color: RISK[p.risk] ?? 'var(--ink)' }}>{p.description || p.action}</span>
                  <code className="t-xs">{p.resource}:{p.action}</code>
                  {p.risk !== 'normal' && <span className="t-xs" style={{ color: RISK[p.risk] }}>{p.risk}</span>}
                </span>
              ))}
            </div>
          </div>
        ))}
      </section>
    </div>
  )
}
