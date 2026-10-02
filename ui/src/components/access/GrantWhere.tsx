import { Fragment } from 'react'
import { Tooltip } from '@mantine/core'
import { IconUser, IconWorld } from '@tabler/icons-react'
import type { GrantScope, ScopeAxis } from '@/lib/api/types'

/* Where a grant applies, in words.
 *
 * This encodes authorize() semantics, and getting it wrong is not cosmetic —
 * it tells an operator someone has narrower access than they do:
 *
 *  - within one structure the places are OR'd ("Jakarta or Bandung");
 *  - across structures they are AND'ed, so each structure gets its own line;
 *  - an excluded place is carved back OUT of the includes beside it, so it
 *    reads "except", never as one more place in the OR;
 *  - `inherit` false means exactly that place — marked, because the default
 *    (the place and everything inside it) is what people assume;
 *  - a structure the grant never names is UNLIMITED, not empty, which is why
 *    the silent ones are counted instead of left out;
 *  - no places and not self-scoped means everywhere.
 *
 * One component for every screen that shows a grant or a membership entry,
 * so the same grant cannot read two ways depending on where it was opened.
 */

function Exact({ exclude }: { exclude: boolean }) {
  return (
    <Tooltip label={exclude
      ? 'Only this place is carved out — what is inside it stays included.'
      : 'This place only — nothing inside it.'}>
      <span className="chip" style={{ marginLeft: 5, padding: '2px 5px', fontSize: 10, cursor: 'help' }}>
        exact
      </span>
    </Tooltip>
  )
}

function Places({ list, nodeName }: { list: GrantScope[]; nodeName: (id: string) => string }) {
  const inc = list.filter((s) => !s.exclude)
  const exc = list.filter((s) => s.exclude)
  return (
    <span className="t-body min-w-0" style={{ color: 'var(--ink)' }}>
      {inc.map((s, i) => (
        <Fragment key={s.scope_node_id}>
          {i > 0 && <span className="t-xs" style={{ margin: '0 5px' }}>or</span>}
          <span style={{ fontWeight: 520 }}>{nodeName(s.scope_node_id)}</span>
          {!s.inherit && <Exact exclude={false} />}
        </Fragment>
      ))}
      {exc.length > 0 && (
        <span style={{ color: 'var(--deny)' }}>
          <span className="t-xs" style={{ margin: '0 5px', color: 'var(--deny)' }}>except</span>
          {exc.map((s, i) => (
            <Fragment key={s.scope_node_id}>
              {i > 0 && <span className="t-xs" style={{ margin: '0 5px', color: 'var(--deny)' }}>and</span>}
              {nodeName(s.scope_node_id)}
              {!s.inherit && <Exact exclude />}
            </Fragment>
          ))}
        </span>
      )}
    </span>
  )
}

export function GrantWhere({ scopes, selfScoped, axes, nodeName, compact = false }: {
  scopes: GrantScope[]
  selfScoped: boolean
  axes: ScopeAxis[] | undefined
  /** Resolves an id the caller has already batch-fetched — never a lookup per place. */
  nodeName: (id: string) => string
  /** One line per structure without the icon column — for tables. */
  compact?: boolean
}) {
  if (selfScoped) {
    return (
      <div className="flex items-center gap-2">
        <IconUser size={14} style={{ color: 'var(--ink-3)', flexShrink: 0 }} />
        <span className="t-body">Only records they own</span>
      </div>
    )
  }
  if (scopes.length === 0) {
    return (
      <div className="flex items-center gap-2">
        <IconWorld size={14} style={{ color: 'var(--ink-3)', flexShrink: 0 }} />
        <span className="t-body">Everywhere — no limits</span>
      </div>
    )
  }

  const byAxis = new Map<string, GrantScope[]>()
  for (const s of scopes) byAxis.set(s.axis_code, [...(byAxis.get(s.axis_code) ?? []), s])
  const axisOf = (code: string) => axes?.find((a) => a.code === code)
  const silent = (axes ?? []).filter((a) => !byAxis.has(a.code))

  /* A two-column list — structure, then places — rather than icons: most
     structures carry no icon of their own, and the fallback glyph repeated on
     every line read as something still loading. */
  return (
    <div className={compact ? 'flex flex-col gap-0.5' : 'where-grid'}>
      {[...byAxis].map(([code, list]) => (
        <div key={code} className={compact ? 'flex min-w-0 items-baseline gap-2' : 'contents'}>
          <span className="where-label t-xs shrink-0 truncate" title={axisOf(code)?.display_name ?? code}>
            {axisOf(code)?.display_name ?? code}
          </span>
          <Places list={list} nodeName={nodeName} />
        </div>
      ))}
      {silent.length > 0 && (
        <Tooltip label={`No limit in: ${silent.map((a) => a.display_name).join(', ')}`} multiline w={320}>
          <span className="t-xs w-fit" style={{ cursor: 'help', gridColumn: compact ? undefined : '1 / -1' }}>
            <span className="dotted">
              No limit in {silent.length === 1 ? silent[0]!.display_name : `${silent.length} other structures`}
            </span>
          </span>
        </Tooltip>
      )}
    </div>
  )
}
