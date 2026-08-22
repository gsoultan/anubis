import type { ReactNode } from 'react'

/* Sticky header, hover affordance, right-aligned numerics, and an empty state
   that teaches rather than saying "no data". Wrapping in .panel keeps the
   header flush with the border instead of floating a rounded corner over it. */
export interface Column<T> {
  key: string
  header: string
  width?: number | string
  align?: 'left' | 'right'
  render: (row: T) => ReactNode
}

export function DataTable<T>({
  columns, rows, empty, rowKey, maxHeight,
}: {
  columns: Column<T>[]
  rows: T[] | undefined
  empty?: { title: string; hint?: string; action?: ReactNode }
  rowKey: (row: T) => string
  maxHeight?: number
}) {
  if (rows && rows.length === 0) {
    return (
      <div className="panel px-6 py-14 text-center">
        <div className="t-h2">{empty?.title ?? 'Nothing here'}</div>
        {empty?.hint && <div className="t-sm mt-1.5" style={{ maxWidth: 420, margin: '6px auto 0' }}>{empty.hint}</div>}
        {empty?.action && <div className="mt-4 flex justify-center">{empty.action}</div>}
      </div>
    )
  }
  return (
    <div className="panel overflow-hidden">
      <div style={maxHeight ? { maxHeight, overflowY: 'auto' } : undefined}>
        <table className="tbl">
          <thead>
            <tr>
              {columns.map((c) => (
                <th key={c.key}
                  style={{ width: c.width, textAlign: c.align === 'right' ? 'right' : 'left' }}>
                  {c.header}
                </th>
              ))}
            </tr>
          </thead>
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
              <tr key={rowKey(r)}>
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
