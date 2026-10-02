import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button, SegmentedControl, Select } from '@mantine/core'
import { IconPlus, IconX } from '@tabler/icons-react'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { axisNeedsAnInclude, namesOf, placesToScopes, requiredAxes, type Places } from '@/lib/access'
import { PlacePicker } from './PlacePicker'
import type { GrantScope, ScopeAxis } from '@/lib/api/types'

/* What a membership gives: a list of roles, each with where it applies. One
   editor for creating a membership and for changing one later, so the two
   cannot drift into different rules.

   Two kinds of membership read differently here. When the places are the same
   for everyone, each role names them. When the membership applies where each
   member is assigned, the member's own place is the answer and a role may only
   add limits in OTHER structures — naming a place in the membership's own
   structure is refused by the server, so that structure is not offered. */

export type EntryDraft = { role_id: string; role_name: string; scopes: GrantScope[] }

/** One line for where an entry applies, for a list of entries. */
export function entryReach(e: EntryDraft, anchored: boolean): string {
  const carved = e.scopes.filter((sc) => sc.exclude).length
  const pinned = e.scopes.length - carved
  if (!e.scopes.length) return anchored ? 'at the member’s place' : 'everywhere'
  /* Carve-outs counted apart from places: a bare count reads "2 places" for
     one office with a department taken out. */
  return `${anchored ? 'at the member’s place, within ' : ''}${pinned} place${pinned === 1 ? '' : 's'}`
    + (carved ? `, except ${carved}` : '')
}

export function MembershipEntries({ anchorAxis, axes, entries, onChange }: {
  /** The structure members are placed in, or null when places are shared. */
  anchorAxis: string | null
  axes: ScopeAxis[] | undefined
  entries: EntryDraft[]
  onChange: (next: EntryDraft[]) => void
}) {
  const { data: roles } = useQuery({ queryKey: qk.roles(), queryFn: api.roles })
  const anchored = !!anchorAxis
  /* The member's place answers for the membership's own structure, so it is
     neither offered nor required of an entry. */
  const offered = (axes ?? []).filter((a) => a.code !== anchorAxis)
  const required = requiredAxes(offered)

  const [roleId, setRoleId] = useState<string | null>(null)
  const [reach, setReach] = useState<'open' | 'places'>(anchored ? 'open' : 'places')
  const [places, setPlaces] = useState<Places>({})

  /* "Open" names no place: everywhere for a shared membership, the member's
     place alone for a placed one. Not on offer when another structure is
     required — the grant would never apply. */
  const open = reach === 'open' && required.length === 0
  const scopes = open ? [] : placesToScopes(places)
  const problem = !roleId ? 'Pick a role.'
    : open ? ''
    : scopes.every((s) => s.exclude) ? 'Choose at least one place.'
    : Object.values(places).some(axisNeedsAnInclude) ? 'An exception needs a place to be carved out of.'
    : required.some((a) => !scopes.some((s) => s.axis_code === a.code && !s.exclude))
      ? `Choose a place in ${namesOf(required)} — every grant must name one.`
    : ''

  const add = () => {
    const role = roles?.find((r) => r.id === roleId)
    if (!role || problem) return
    onChange([...entries, { role_id: role.id, role_name: role.name, scopes }])
    setRoleId(null); setPlaces({})
  }

  return (
    <>
      {entries.length > 0 && (
        <div>
          <div className="t-label mb-2">Gives every member</div>
          <div className="flex flex-col gap-1.5">
            {entries.map((e, i) => (
              <div key={i} className="panel-inset flex items-center justify-between gap-2 px-2.5 py-2">
                <span className="t-body min-w-0 truncate" style={{ fontWeight: 530 }}>
                  {e.role_name}
                  <span className="t-xs" style={{ marginLeft: 6 }}>· {entryReach(e, anchored)}</span>
                </span>
                <button type="button" onClick={() => onChange(entries.filter((_, j) => j !== i))}
                  aria-label={`Remove ${e.role_name}`} style={{ color: 'var(--ink-3)', display: 'flex' }}>
                  <IconX size={13} />
                </button>
              </div>
            ))}
          </div>
        </div>
      )}

      <div className="panel-inset flex flex-col gap-3 px-3 py-3">
        <div className="t-label">Add a role</div>
        <Select size="sm" searchable placeholder="Pick a role" aria-label="Role"
          /* An entry becomes a grant the moment somebody is assigned, so a
             retired role would fail at assignment rather than here. */
          data={(roles ?? []).map((r) => ({
            value: r.id,
            label: r.deprecated ? `${r.name} — retired from the catalog` : r.name,
            disabled: r.deprecated,
          }))}
          value={roleId} onChange={setRoleId} />
        {roleId && (
          <>
            <SegmentedControl fullWidth size="xs" value={open ? 'open' : 'places'}
              onChange={(v) => setReach(v as 'open' | 'places')}
              aria-label="Where this role applies"
              data={anchored ? [
                { value: 'open', label: 'At the member’s place', disabled: required.length > 0 },
                { value: 'places', label: 'Also limit elsewhere', disabled: offered.length === 0 },
              ] : [
                { value: 'places', label: 'Specific places' },
                { value: 'open', label: 'Everywhere', disabled: required.length > 0 },
              ]} />
            {anchored && open && (
              <div className="t-xs">
                Applies at the place each member holds this membership, and nowhere else in that structure.
              </div>
            )}
            {!open && offered.length > 0 && (
              <PlacePicker axes={offered} value={places} onChange={setPlaces} required={required} />
            )}
          </>
        )}
        {roleId && problem && <div className="t-xs">{problem}</div>}
        <Button size="xs" variant="light" leftSection={<IconPlus size={13} />}
          disabled={!!problem} onClick={add}>
          Add role
        </Button>
      </div>
    </>
  )
}
