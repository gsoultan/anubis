import { Link } from '@tanstack/react-router'
import { IconArrowUpRight, IconTrendingUp, IconTrendingDown } from '@tabler/icons-react'
import type { ReactNode } from 'react'
import { Sparkline } from './Sparkline'

/* The number is the point, so it gets the size. Label above in muted small
   caps, trend and sparkline below — the eye lands on the value first and only
   then picks up context. */
export function Stat({
  label, value, sub, series, trend, to, accent = 'var(--gold)', icon,
}: {
  label: string
  value: string
  sub?: string
  series?: number[]
  trend?: number
  to?: string
  accent?: string
  icon?: ReactNode
}) {
  const body = (
    <div className="panel panel-hover group relative overflow-hidden p-4">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="t-label flex items-center gap-1.5">
            {icon}
            {label}
          </div>
          <div className="t-num mt-2" style={{ color: 'var(--ink)' }}>{value}</div>
        </div>
        {series && <Sparkline data={series} color={accent} />}
      </div>

      <div className="mt-2 flex items-center gap-2">
        {trend !== undefined && (
          <span
            className="inline-flex items-center gap-1 text-[11px] font-semibold tabular-nums"
            style={{ color: trend >= 0 ? 'var(--allow)' : 'var(--deny)' }}
          >
            {trend >= 0 ? <IconTrendingUp size={12} /> : <IconTrendingDown size={12} />}
            {trend >= 0 ? '+' : ''}{trend.toFixed(1)}%
          </span>
        )}
        {sub && <span className="t-xs truncate">{sub}</span>}
      </div>

      {to && (
        <IconArrowUpRight
          size={14}
          className="absolute right-3 top-3 opacity-0 transition-opacity group-hover:opacity-100"
          style={{ color: 'var(--ink-3)' }}
        />
      )}
    </div>
  )
  return to ? <Link to={to} className="block no-underline">{body}</Link> : body
}
