import { useEffect, useMemo, useState } from 'react'
import { Modal, TextInput } from '@mantine/core'
import { useNavigate } from '@tanstack/react-router'
import { useCreate, type CreateKind } from '@/stores/create'
import {
  IconSearch, IconCornerDownLeft, IconLayoutDashboard, IconTestPipe, IconUsers,
  IconSitemap, IconAffiliate, IconShieldCheck, IconWorld, IconFileDescription,
  IconKey, IconArrowRight,
} from '@tabler/icons-react'

type Cmd = {
  id: string; label: string; group: string; icon: React.ReactNode; keywords?: string
} & ({ to: string; create?: never } | { create: CreateKind; to?: never })

const COMMANDS: Cmd[] = [
  { id: 'overview', label: 'Overview', group: 'Navigate', to: '/', icon: <IconLayoutDashboard size={15} />, keywords: 'dashboard home signals' },
  { id: 'authz', label: 'Access check', group: 'Navigate', to: '/playground', icon: <IconTestPipe size={15} />, keywords: 'authorization decide evaluate explain deny debug playground' },
  { id: 'identities', label: 'People', group: 'Navigate', to: '/identities', icon: <IconUsers size={15} />, keywords: 'users identities accounts realm' },
  { id: 'scope', label: 'Structure', group: 'Navigate', to: '/scope', icon: <IconSitemap size={15} />, keywords: 'scope axes tree org offices product customer' },
  { id: 'grants', label: 'Access', group: 'Navigate', to: '/grants', icon: <IconAffiliate size={15} />, keywords: 'grants assignments who can' },
  { id: 'roles', label: 'Roles & permissions', group: 'Navigate', to: '/roles', icon: <IconShieldCheck size={15} />, keywords: 'rbac permission' },
  { id: 'memberships', label: 'Memberships', group: 'Navigate', to: '/memberships', icon: <IconAffiliate size={15} />, keywords: 'groups bundles teams onboarding' },
  { id: 'c-membership', label: 'New membership…', group: 'Create', create: 'membership', icon: <IconArrowRight size={15} />, keywords: 'group bundle team' },
  { id: 'realms', label: 'Populations', group: 'Navigate', to: '/realms', icon: <IconWorld size={15} />, keywords: 'realms internal partner public sign-in rules' },
  { id: 'audit', label: 'Audit', group: 'Navigate', to: '/audit', icon: <IconFileDescription size={15} />, keywords: 'log history chain' },
  { id: 'keys', label: 'Signing keys', group: 'Navigate', to: '/keys', icon: <IconKey size={15} />, keywords: 'rotation kid paseto' },
  { id: 'debug-deny', label: 'Debug a denied decision', group: 'Actions', to: '/playground', icon: <IconArrowRight size={15} />, keywords: 'why denied troubleshoot' },
  { id: 'c-identity', label: 'Add person…', group: 'Create', create: 'identity', icon: <IconArrowRight size={15} />, keywords: 'new identity user employee applicant supplier' },
  { id: 'c-grant', label: 'Give access…', group: 'Create', create: 'grant', icon: <IconArrowRight size={15} />, keywords: 'new grant assign role' },
  { id: 'c-role', label: 'Add role…', group: 'Create', create: 'role', icon: <IconArrowRight size={15} />, keywords: 'new rbac' },
  { id: 'c-permission', label: 'Add permission…', group: 'Create', create: 'permission', icon: <IconArrowRight size={15} />, keywords: 'new resource action' },
  { id: 'c-node', label: 'Add structure item…', group: 'Create', create: 'node', icon: <IconArrowRight size={15} />, keywords: 'new scope node office department product customer' },
  { id: 'c-axis', label: 'Add structure…', group: 'Create', create: 'axis', icon: <IconArrowRight size={15} />, keywords: 'new scope axis dimension cost centre project' },
]

export function CommandPalette() {
  const [open, setOpen] = useState(false)
  const [q, setQ] = useState('')
  const [idx, setIdx] = useState(0)
  const navigate = useNavigate()
  const { openCreate } = useCreate()

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault(); setOpen((o) => !o); setQ(''); setIdx(0)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  const results = useMemo(() => {
    const needle = q.trim().toLowerCase()
    if (!needle) return COMMANDS
    return COMMANDS.filter((c) =>
      c.label.toLowerCase().includes(needle) || (c.keywords ?? '').includes(needle))
  }, [q])

  const run = (c: Cmd) => {
    setOpen(false)
    if (c.create) openCreate(c.create)
    else void navigate({ to: c.to })
  }

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown') { e.preventDefault(); setIdx((i) => Math.min(i + 1, results.length - 1)) }
    if (e.key === 'ArrowUp')   { e.preventDefault(); setIdx((i) => Math.max(i - 1, 0)) }
    if (e.key === 'Enter' && results[idx]) { e.preventDefault(); run(results[idx]!) }
  }

  let lastGroup = ''
  return (
    <Modal opened={open} onClose={() => setOpen(false)} withCloseButton={false}
      size={520} padding={0} radius="lg" styles={{ body: { padding: 0 } }}>
      <div style={{ borderBottom: '1px solid var(--line)' }}>
        <TextInput
          autoFocus variant="unstyled" placeholder="Search or jump to…"
          value={q}
          onChange={(e) => { setQ(e.currentTarget.value); setIdx(0) }}
          onKeyDown={onKeyDown}
          leftSection={<IconSearch size={15} style={{ color: 'var(--ink-3)' }} />}
          styles={{ input: { fontSize: 14, height: 46, paddingLeft: 40 } }}
        />
      </div>

      <div className="max-h-[340px] overflow-y-auto p-1.5">
        {results.length === 0 && (
          <div className="px-3 py-8 text-center">
            <div className="t-sm">No matches for “{q}”</div>
          </div>
        )}
        {results.map((c, i) => {
          const header = c.group !== lastGroup ? (lastGroup = c.group) : null
          return (
            <div key={c.id}>
              {header && <div className="t-label px-2.5 pb-1 pt-2.5">{header}</div>}
              <button
                onClick={() => run(c)}
                onMouseEnter={() => setIdx(i)}
                className="flex w-full items-center gap-2.5 rounded-md px-2.5 py-2 text-left"
                style={{
                  background: i === idx ? 'var(--s-overlay)' : 'transparent',
                  color: i === idx ? 'var(--ink)' : 'var(--ink-2)',
                }}
              >
                <span style={{ color: i === idx ? 'var(--gold)' : 'var(--ink-3)' }}>{c.icon}</span>
                <span className="flex-1 text-[13px]">{c.label}</span>
                {i === idx && <IconCornerDownLeft size={12} style={{ color: 'var(--ink-3)' }} />}
              </button>
            </div>
          )
        })}
      </div>

      <div className="flex items-center gap-3 px-3 py-2"
        style={{ borderTop: '1px solid var(--line)' }}>
        {[['↑↓', 'navigate'], ['↵', 'open'], ['esc', 'close']].map(([k, l]) => (
          <span key={k} className="flex items-center gap-1.5">
            <kbd className="chip">{k}</kbd><span className="t-xs">{l}</span>
          </span>
        ))}
      </div>
    </Modal>
  )
}
