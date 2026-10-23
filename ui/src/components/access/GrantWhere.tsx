import { Fragment } from 'react'
import { useQueries } from '@tanstack/react-query'
import { Tooltip } from '@mantine/core'
import { IconUser, IconWorld } from '@tabler/icons-react'
import { api } from '@/lib/api/client'
import { axisReach } from '@/lib/access'
import { qk } from '@/lib/query/keys'
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

/** The top place of each kind, so a pick of it "and everything inside" reads
    as Everything. Shares the root listing every tree picker already caches. */
export function useAxisRoots(axes: ScopeAxis[] | undefined): (code: string) => string | undefined {
  const results = useQueries({
    queries: (axes ?? []).map((a) => ({
      queryKey: qk.scopeChildren(a.code, null),
      queryFn: () => api.scopeChildren(a.code, null),
      staleTime: 60_000,
    })),
  })
  return (code) => {
    const i = (axes ?? []).findIndex((a) => a.code === code)
    const roots = i >= 0 ? results[i]?.data : undefined
    return roots?.find((n) => n.is_axis_root)?.id ?? (roots?.length === 1 ? roots[0]!.id : undefined)
  }
}

function Mark({ kind }: { kind: 'all' | 'list' | 'none' }) {
  const label = kind === 'all'
    ? 'Everything: no list is kept, so a place added later is covered the moment it exists.'
    : kind === 'list'
      ? 'Only these: a list. A place added later is not on it until someone adds it.'
      : 'This kind is required and the access names nothing in it, so it never applies.'
  return (
    <Tooltip label={label} multiline w={280}>
      <span aria-label={kind === 'all' ? 'Everything' : kind === 'list' ? 'Only these' : 'Nothing'}
        style={{
          display: 'inline-flex', width: 16, justifyContent: 'center', flexShrink: 0, cursor: 'help',
          fontWeight: 600, color: kind === 'all' ? 'var(--allow)' : kind === 'none' ? 'var(--deny)' : 'var(--ink-3)',
        }}>
        {kind === 'all' ? '∞' : kind === 'list' ? '☰' : '∅'}
      </span>
    </Tooltip>
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
  const rootOf = useAxisRoots(axes)
  if (selfScoped) {
    return (
      <div className="flex items-center gap-2">
        <IconUser size={14} style={{ color: 'var(--ink-3)', flexShrink: 0 }} />
        <span className="t-body">Only records they own</span>
      </div>
    )
  }

  /* Every kind of place gets a line, because the line that is NOT a list is
     the one people misread: "Workplace: every one, including new ones" is a
     different promise from twelve workplaces ticked by hand. Kinds the grant
     never names are folded into one closing line, so a console with seven
     kinds does not print five lines of "everything". */
  const known = axes ?? []
  const reaches = known.map((a) => ({ axis: a, reach: axisReach(scopes, a, rootOf(a.code)) }))
  const strangers = [...new Set(scopes.map((s) => s.axis_code))].filter((c) => !known.some((a) => a.code === c))
  const named = reaches.filter((r) => r.reach.kind !== 'all' || scopes.some((s) => s.axis_code === r.axis.code))
  const silent = reaches.filter((r) => !named.includes(r))

  if (named.length === 0 && strangers.length === 0) {
    return (
      <div className="flex items-center gap-2">
        <IconWorld size={14} style={{ color: 'var(--ink-3)', flexShrink: 0 }} />
        <span className="t-body">Everywhere, including places added later</span>
      </div>
    )
  }

  return (
    <div className={compact ? 'flex flex-col gap-0.5' : 'where-grid'}>
      {named.map(({ axis, reach }) => (
        <div key={axis.code} className={compact ? 'flex min-w-0 items-baseline gap-2' : 'contents'}>
          <span className="where-label t-xs shrink-0 truncate" title={axis.display_name}>{axis.display_name}</span>
          <span className="flex min-w-0 items-baseline gap-1.5">
            <Mark kind={reach.kind} />
            {reach.kind === 'all' && <span className="t-body">Every one, including new ones</span>}
            {reach.kind === 'none' && <span className="t-body" style={{ color: 'var(--deny)' }}>Nothing, so this never applies</span>}
            {reach.kind === 'list' && <Places list={[...reach.include, ...reach.exclude]} nodeName={nodeName} />}
          </span>
        </div>
      ))}
      {strangers.map((code) => (
        <div key={code} className={compact ? 'flex min-w-0 items-baseline gap-2' : 'contents'}>
          <span className="where-label t-xs shrink-0 truncate">{code}</span>
          <span className="flex min-w-0 items-baseline gap-1.5">
            <Mark kind="list" />
            <Places list={scopes.filter((s) => s.axis_code === code)} nodeName={nodeName} />
          </span>
        </div>
      ))}
      {silent.length > 0 && (
        <Tooltip label={`No list kept for: ${silent.map((r) => r.axis.display_name).join(', ')}. New places of these kinds are covered straight away.`} multiline w={320}>
          <span className="t-xs w-fit" style={{ cursor: 'help', gridColumn: compact ? undefined : '1 / -1' }}>
            <span style={{ color: 'var(--allow)', fontWeight: 600, marginRight: 6 }}>∞</span>
            <span className="dotted">
              Every {silent.length === 1 ? silent[0]!.axis.display_name.toLowerCase() : `${silent.length} other kinds of place`}, including new ones
            </span>
          </span>
        </Tooltip>
      )}
    </div>
  )
}
