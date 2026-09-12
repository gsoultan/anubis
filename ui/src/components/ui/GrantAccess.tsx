import { Tooltip } from '@mantine/core'
import { IconLock } from '@tabler/icons-react'
import type { Grant, ScopeAxis, Membership } from '@/lib/api/types'

/* How a grant reads, in one place.
 *
 * These two cells encode authorize() semantics, and getting them wrong is not
 * a cosmetic bug — it is telling an operator someone has narrower access than
 * they do. Within an axis the scopes are OR'd; across axes they are AND'ed, so
 * an axis the grant never mentions is UNLIMITED rather than empty, which is
 * why silent axes are spelled out instead of omitted. `inherit` false means
 * exactly that node and nothing beneath it. No scopes at all, with no
 * self-scope, means everywhere.
 *
 * They live here because the Access screen and a person's detail answer the
 * same question about the same row. Rendered separately they would drift, and
 * the same grant would read two ways depending on which screen you opened —
 * this console's recurring failure, in miniature.
 */

/** The role a grant confers, and where the grant itself came from. */
export function GrantRole({ grant: g, memberships }: {
  grant: Grant
  memberships?: Membership[] | undefined
}) {
  return (
    <div className="flex flex-col gap-1">
      <span className="t-body" style={{ fontWeight: 550 }}>{g.role_name}</span>
      {g.via_membership_id && (
        <Tooltip label="Derived from a membership — manage it there, not here.">
          <span className="chip w-fit" style={{ color: 'var(--gold)', borderColor: 'var(--gold-chip-line)', background: 'var(--gold-chip-bg)' }}>
            via {memberships?.find((x) => x.id === g.via_membership_id)?.name ?? 'membership'}
          </span>
        </Tooltip>
      )}
      {g.self_scoped && (
        <Tooltip label="Applies only to records this identity owns. The caller must supply _owner or the decision is denied.">
          <span className="chip w-fit" style={{ color: 'var(--info)', borderColor: 'color-mix(in srgb, var(--info) 20%, transparent)' }}>
            <IconLock size={9} style={{ marginRight: 4 }} />self-scoped
          </span>
        </Tooltip>
      )}
    </div>
  )
}

/** Where the grant applies. `nodeName` resolves an id the caller has already
    batch-fetched — never a lookup per chip. */
export function GrantScopes({ grant: g, axes, nodeName, maxSilent }: {
  grant: Grant
  axes?: ScopeAxis[] | undefined
  nodeName: (id: string) => string
  /** Cap the "no limit on" list. Unset means list them all, which is what the
      Access table does — it has a wide column and one row per grant. In a
      narrow card the same list runs to three lines per grant and buries the
      scopes that ARE set, so the drawer caps it and keeps the rest in the
      tooltip. The count is always shown: "no limit on" is the sentence where
      omitting something would understate someone's access. */
  maxSilent?: number | undefined
}) {
  const silent = (axes ?? []).filter((a) => !g.scopes.some((s) => s.axis_code === a.code))
  const byAxis = new Map<string, Grant['scopes']>()
  for (const s of g.scopes) {
    const list = byAxis.get(s.axis_code) ?? []
    list.push(s); byAxis.set(s.axis_code, list)
  }

  return (
    <div className="flex flex-col gap-1.5">
      {g.scopes.length === 0 && !g.self_scoped && (
        <span className="t-xs">everywhere — no limits</span>
      )}
      {[...byAxis].map(([axisCode, list]) => (
        <div key={axisCode} className="flex items-start gap-2">
          <span className="chip" style={{ minWidth: 62, justifyContent: 'center', marginTop: 1 }}>
            {axisCode}
          </span>
          <span className="t-body min-w-0">
            {list.map((s, i) => (
              <span key={s.scope_node_id}>
                {i > 0 && <span className="t-xs" style={{ margin: '0 5px', fontStyle: 'italic' }}>or</span>}
                {nodeName(s.scope_node_id)}
                {!s.inherit && (
                  <Tooltip label="Exactly this place — nothing inside it.">
                    <span className="chip" style={{ marginLeft: 4, color: 'var(--warn)',
                      borderColor: 'color-mix(in srgb, var(--warn) 20%, transparent)' }}>exact</span>
                  </Tooltip>
                )}
              </span>
            ))}
          </span>
        </div>
      ))}
      {silent.length > 0 && g.scopes.length > 0 && (
        maxSilent !== undefined && silent.length > maxSilent ? (
          <Tooltip label={silent.map((a) => a.code).join(', ')} multiline w={280}>
            <span className="t-xs" style={{ fontSize: 10 }}>
              no limit on {silent.slice(0, maxSilent).map((a) => a.code).join(', ')}
              {' '}+{silent.length - maxSilent} more
            </span>
          </Tooltip>
        ) : (
          <span className="t-xs" style={{ fontSize: 10 }}>
            no limit on {silent.map((a) => a.code).join(', ')}
          </span>
        )
      )}
    </div>
  )
}
