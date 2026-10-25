import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { Button, Loader } from '@mantine/core'
import { api } from '@/lib/api/client'
import type { ScopeNode } from '@/lib/api/types'

/* Who has access here, and who might expect to.

   Opened on a place, and above all on a new one: people whose access grows
   by itself have it already and need nothing, and people whose hand-picked
   list names every place beside it probably meant "all of them". Those are
   offered it in one reviewed change, never handed it silently. */
export function PlaceReachPanel({ node }: { node: ScopeNode }) {
  const navigate = useNavigate()
  const { data: r, isLoading } = useQuery({
    queryKey: ['grant-change', 'reach', node.id],
    queryFn: () => api.placeReach(node.id),
    enabled: !node.is_axis_root && node.status !== 'archived',
  })
  if (node.is_axis_root || node.status === 'archived') return null
  const parent = node.parent_id
  return (
    <div className="panel p-4">
      <div className="t-label mb-2">Who has access here</div>
      {isLoading && <Loader size="xs" />}
      {r && (
        <div className="flex flex-col gap-2.5">
          <div className="t-sm flex flex-col gap-1">
            <span><b className="tnum">{r.have_all}</b> {r.have_all === 1 ? 'grant covers' : 'grants cover'} every place of this kind, so {r.have_all === 1 ? 'it has' : 'they have'} it already.</span>
            <span><b className="tnum">{r.have_branch}</b> {r.have_branch === 1 ? 'grant reaches' : 'grants reach'} it through a place above it, picked with everything inside.</span>
            {r.other_fixed > 0 && <span className="t-xs">{r.other_fixed} more hold a list that names some places here but not this one. They are left as they are.</span>}
          </div>

          {(r.might_expect.length > 0 || r.group_entries.length > 0) && (
            <>
              <div className="t-label mt-1">Might need it too</div>
              <div className="t-xs">Their list names every place beside {node.name}, but not {node.name}.</div>
              {r.group_entries.map((e) => (
                <div key={e.membership_id} className="panel-inset flex flex-wrap items-center gap-2 px-3 py-2">
                  <span className="t-body min-w-0 flex-1">
                    Group <b>{e.membership_name}</b> ({e.role_name}), {e.members} {e.members === 1 ? 'member' : 'members'}
                  </span>
                  <Link to="/memberships" className="t-xs" style={{ color: 'var(--accent)' }}>Change the group</Link>
                </div>
              ))}
              {r.might_expect.map((g) => (
                <div key={g.role_id} className="panel-inset flex flex-wrap items-center gap-2 px-3 py-2">
                  <span className="t-body min-w-0 flex-1">
                    <b>{g.role_name}</b>, {g.people} {g.people === 1 ? 'person' : 'people'}
                    {g.risk !== 'normal' && <span className="chip ml-1.5" style={{ color: g.risk === 'critical' ? 'var(--deny)' : 'var(--warn)' }}>{g.risk}</span>}
                  </span>
                  <Button size="compact-xs" variant="light"
                    onClick={() => void navigate({ to: '/bulk', search: { role: g.role_id, axis: node.axis_code, node: node.id, kind: 'add_place', ...(parent ? { siblings: parent } : {}) } })}>
                    Add it to them
                  </Button>
                </div>
              ))}
            </>
          )}
        </div>
      )}
    </div>
  )
}
