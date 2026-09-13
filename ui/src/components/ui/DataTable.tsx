import type { ReactNode } from 'react'
import { Tooltip } from '@mantine/core'

/* Sticky header, hover affordance, right-aligned numerics, and an empty state
   that teaches rather than saying "no data". Wrapping in .panel keeps the
   header flush with the border instead of floating a rounded corner over it.

   The panel also owns two optional rails: a toolbar above the header and a
   footer below the body. Filters belong on the table they filter, not in the
   page header next to the global search box — and a footer that is always
   rendered means the row count does not vanish the moment a list fits on one
   page. The toolbar survives the empty state for the same reason: the control
   that produced "no matches" has to still be there to undo it. */
export interface Column<T> {
  key: string
  header: string
  /** Explains the column where the cells cannot — what a bare em dash means. */
  headerHint?: string
  width?: number | string
  align?: 'left' | 'right'
  render: (row: T) => ReactNode
}

export function DataTable<T>({
  columns, rows, empty, rowKey, maxHeight, toolbar, footer, onRowClick, stale,
}: {
  columns: Column<T>[]
  rows: T[] | undefined
  empty?: { title: string; hint?: string; action?: ReactNode }
  rowKey: (row: T) => string
  maxHeight?: number
  toolbar?: ReactNode
  footer?: ReactNode
  onRowClick?: (row: T) => void
  /** Showing the previous page while the next one loads. Say so. */
  stale?: boolean
}) {
  const head = (
    <thead>
      <tr>
        {columns.map((c) => (
          <th key={c.key}
            style={{ width: c.width, textAlign: c.align === 'right' ? 'right' : 'left' }}>
            {c.headerHint
              ? <Tooltip label={c.headerHint} withArrow>
                  <span style={{ cursor: 'help', borderBottom: '1px dotted var(--line-strong)' }}>
                    {c.header}
                  </span>
                </Tooltip>
              : c.header}
          </th>
        ))}
      </tr>
    </thead>
  )

  const body = rows && rows.length === 0 ? (
    <div className="px-6 py-14 text-center">
      <div className="t-h2">{empty?.title ?? 'Nothing here'}</div>
      {empty?.hint && <div className="t-sm" style={{ maxWidth: 420, margin: '6px auto 0' }}>{empty.hint}</div>}
      {empty?.action && <div className="mt-4 flex justify-center">{empty.action}</div>}
    </div>
  ) : (
    <div style={maxHeight ? { maxHeight, overflowY: 'auto' } : undefined}>
      <table className={`tbl${toolbar ? ' tbl-offset' : ''}`}>
        {head}
        <tbody>
          {!rows &&
            Array.from({ length: 6 }).map((_, i) => (
              <tr key={i}>
                {columns.map((c) => (
                  <td key={c.key}>
                    <div className="animate-pulse rounded"
                      style={{ height: 11, width: `${40 + ((i * 17 + c.key.length * 9) % 45)}%`,
                        background: 'var(--line)' }} />
                  </td>
                ))}
              </tr>
            ))}
          {rows?.map((r) => (
            <tr key={rowKey(r)}
              data-clickable={onRowClick ? '' : undefined}
              tabIndex={onRowClick ? 0 : undefined}
              onClick={onRowClick ? () => onRowClick(r) : undefined}
              /* Enter/Space, because a row that only answers to a mouse is a
                 row a keyboard operator cannot open. */
              onKeyDown={onRowClick
                ? (e) => {
                    if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onRowClick(r) }
                  }
                : undefined}>
              {columns.map((c) => (
                <td key={c.key} className={c.align === 'right' ? 'num' : undefined}>
                  {c.render(r)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )

  /* `overflow: clip`, not `hidden`. Both round the corners off the table, but
     `hidden` makes the panel a scroll container, and a sticky child resolves
     against the nearest scroll container — so the sticky column header had
     nothing to stick to and scrolled away with the rows. `clip` clips without
     creating one, which hands the header back to <main>. */
  return (
    <div className="panel overflow-clip">
      {toolbar && <div className="tbl-rail tbl-rail-top">{toolbar}</div>}
      <div data-stale={stale ? '' : undefined} className="tbl-body">{body}</div>
      {footer && <div className="tbl-rail tbl-rail-bottom">{footer}</div>}
    </div>
  )
}

/** Two-line cell: primary label with muted secondary beneath. */
export function Cell({ top, bottom }: { top: ReactNode; bottom?: ReactNode }) {
  return (
    <div className="min-w-0">
      <div className="t-body truncate" style={{ fontWeight: 500 }}>{top}</div>
      {bottom && <div className="t-xs mt-0.5 truncate">{bottom}</div>}
    </div>
  )
}

export function Dot({ color, label }: { color: string; label: ReactNode }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <span style={{ width: 6, height: 6, borderRadius: 99, background: color, flexShrink: 0 }} />
      <span className="t-body">{label}</span>
    </span>
  )
}
