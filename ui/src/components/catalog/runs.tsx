import type { CatalogRun } from '@/lib/api/types'

/* How a run reads at a glance, in one place because the table and the drawer
   must not describe the same row two different ways. */

const TONE: Record<CatalogRun['status'], { color: string; pill: boolean; label: string }> = {
  ok: { color: 'var(--allow)', pill: false, label: 'applied' },
  // Not a failure and not an apply: the document was byte-identical to the one
  // already installed, so nothing was written and no manifest version burned.
  skipped: { color: 'var(--ink-3)', pill: false, label: 'unchanged' },
  dry_run: { color: 'var(--info)', pill: false, label: 'dry run' },
  failed: { color: 'var(--deny)', pill: true, label: 'failed' },
  // A row that stays here is a feed that hangs — the only way anybody notices.
  running: { color: 'var(--warn)', pill: true, label: 'running' },
}

export function RunStatus({ status }: { status: CatalogRun['status'] }) {
  const t = TONE[status] ?? TONE.failed
  if (t.pill) {
    return <span className="v-pill v-pill-deny">{t.label}</span>
  }
  return (
    <span className="t-body inline-flex items-center gap-1.5" style={{ color: 'var(--ink-2)' }}>
      <span style={{ width: 6, height: 6, borderRadius: 99, background: t.color, flexShrink: 0 }} />
      {t.label}
    </span>
  )
}

/** The apply report in a sentence. The raw JSON is still shown underneath —
    this is for the row you are scanning past, not the one you are reading. */
export function describeRun(run: CatalogRun): string {
  if (run.status === 'running') return 'Started, no result yet.'
  if (run.status === 'failed') return run.error || 'Failed.'
  if (run.status === 'skipped') {
    return 'The document had not changed, so nothing was written.'
  }
  const report = safeReport(run.report_json)
  if (!report) return run.dry ? 'Validated, nothing written.' : 'Applied.'

  const parts: string[] = []
  const perms = report['permissions'] as { applied?: number; deprecated?: string[] } | undefined
  if (perms) {
    const retired = perms.deprecated?.length ?? 0
    parts.push(`${perms.applied ?? 0} permissions` + (retired ? `, ${retired} retired` : ''))
  }
  const roles = report['roles'] as { applied?: number; deprecated?: string[] } | undefined
  if (roles) {
    const retired = roles.deprecated?.length ?? 0
    parts.push(`${roles.applied ?? 0} roles` + (retired ? `, ${retired} retired` : ''))
  }
  const routes = report['routes'] as { replaced?: number } | undefined
  if (routes) parts.push(`${routes.replaced ?? 0} routes`)

  if (parts.length === 0) return run.dry ? 'Validated, nothing written.' : 'Applied.'
  const head = run.dry ? 'Would apply' : 'Applied'
  const version = report['manifest_version']
  const tail = !run.dry && typeof version === 'number' ? ` — version ${version}` : ''
  return `${head} ${parts.join('; ')}${tail}.`
}

function safeReport(raw: string): Record<string, unknown> | null {
  if (!raw) return null
  try {
    const v = JSON.parse(raw) as unknown
    return typeof v === 'object' && v !== null ? (v as Record<string, unknown>) : null
  } catch {
    return null
  }
}

/** Relative for anything recent, the date once it stops being "today-ish".
    An operator reading a run history wants "12 minutes ago", not a timestamp
    they have to subtract. */
export function when(iso: string | null): string {
  if (!iso) return 'never'
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return 'never'
  const diff = Date.now() - then
  const future = diff < 0
  const mins = Math.round(Math.abs(diff) / 60000)
  if (mins < 1) return future ? 'in under a minute' : 'just now'
  if (mins < 60) return future ? `in ${mins} min` : `${mins} min ago`
  const hours = Math.round(mins / 60)
  if (hours < 24) return future ? `in ${hours}h` : `${hours}h ago`
  return iso.slice(0, 10)
}

/** A source's schedule as a phrase. */
export function schedule(intervalSeconds: number): string {
  if (intervalSeconds <= 0) return 'manual'
  const mins = Math.round(intervalSeconds / 60)
  if (mins < 60) return `every ${mins} min`
  const hours = mins / 60
  if (Number.isInteger(hours) && hours < 48) return `every ${hours}h`
  return `every ${Math.round(hours)}h`
}
