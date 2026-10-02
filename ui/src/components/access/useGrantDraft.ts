import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/client'
import { notifyCreated, notifyRejected } from '@/components/create/shell'
import {
  axisNeedsAnInclude, daysFrom, fmtDate, namesOf, placesToScopes, requiredAxes, roleBlocked,
  untilDate, type Places, type Until,
} from '@/lib/access'

/* State, validation and the write for giving somebody access. The sheet owns
   layout; this owns every rule the form has to teach before the database has
   to refuse it:

   1. Roles are filtered by the person's population — blocked ones stay
      visible with the reason, because an invisible option reads as a bug and
      a disabled one reads as a rule.
   2. Where a grant applies is an explicit choice, never a default. The widest
      grant there is used to be what you got by not touching a section.
   3. Places and "own records only" are mutually exclusive, and so the choice
      between them is one control, not a switch that disables a list.
   4. A structure holding only exclusions is refused (migration 0046).
   5. A required structure (default_effect = deny) that the grant names no
      place in makes the grant do nothing at all — so the form will not
      write one.
   6. A membership that applies where each member is assigned needs the one
      place this person holds it — and the same place twice is refused here,
      not left to come back as "nothing changed". */

export type Reach = 'everywhere' | 'places' | 'own'
export type What = 'role' | 'membership'

export function useGrantDraft({ identityId, onDone }: {
  identityId: string | null
  onDone?: (() => void) | undefined
}) {
  const { data: realms } = useQuery({ queryKey: qk.realms(), queryFn: api.realms })
  const { data: roles } = useQuery({ queryKey: qk.roles(), queryFn: api.roles })
  const { data: axes } = useQuery({ queryKey: qk.axes(), queryFn: api.axes })
  const { data: memberships } = useQuery({ queryKey: qk.memberships(), queryFn: api.memberships })
  /* One person, fetched when picked — keyed the way every screen keys an
     identity, so their own page and this sheet share one fetch. */
  const { data: subject } = useQuery({
    queryKey: qk.identity(identityId ?? ''),
    queryFn: () => api.identity(identityId as string),
    enabled: !!identityId,
  })
  /* What they already hold, so "they already have this role" is said before
     a duplicate is written. Same key and call as the person's own page. */
  const { data: held } = useQuery({
    queryKey: qk.grants(identityId ?? ''),
    queryFn: () => api.searchGrants({ identityId: identityId as string, pageSize: 200 }),
    enabled: !!identityId,
  })
  const subjectKind = realms?.find((r) => r.id === subject?.realm_id)?.kind
  const required = requiredAxes(axes)

  const [what, setWhat] = useState<What>('role')
  const [roleId, setRoleId] = useState<string | null>(null)
  const [membershipId, setMembershipId] = useState<string | null>(null)
  /* The place a where-assigned membership is held at, and whether it reaches
     what sits inside that place (the default) or the place alone. */
  const [seat, setSeat] = useState<{ id: string; name: string } | null>(null)
  const [seatExact, setSeatExact] = useState(false)
  const [reach, setReach] = useState<Reach | null>(null)
  const [places, setPlaces] = useState<Places>({})
  const [until, setUntil] = useState<Until>('')
  const [pickedDate, setPickedDate] = useState<string | null>(null)
  const [reason, setReason] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const role = roles?.find((r) => r.id === roleId)
  const membership = memberships?.find((m) => m.id === membershipId)
  /* Their current assignments, read from the assignments themselves. Grants
     were the old proxy, and a membership that gives no roles yet leaves none. */
  const { data: assignments } = useQuery({
    queryKey: qk.membershipAssignments({ identityId: identityId ?? '' }),
    queryFn: () => api.membershipAssignments({ identityId: identityId as string, pageSize: 200 }),
    enabled: !!identityId,
  })
  const heldHere = (assignments?.rows ?? []).filter((a) => a.membership_id === membershipId)

  // A person from a population the chosen role cannot serve clears the role,
  // or the form submits straight into the guard.
  useEffect(() => {
    if (role && roleBlocked(role, subjectKind)) setRoleId(null)
  }, [role, subjectKind])

  // A place belongs to one membership's structure; choosing another clears it.
  useEffect(() => { setSeat(null); setSeatExact(false) }, [membershipId])

  const reset = () => {
    setWhat('role'); setRoleId(null); setMembershipId(null); setReach(null)
    setPlaces({}); setUntil(''); setPickedDate(null); setReason('')
    setSeat(null); setSeatExact(false)
  }

  const ends = untilDate(until, pickedDate)
  const scopes = reach === 'places' ? placesToScopes(places) : []
  const includes = scopes.filter((s) => !s.exclude)
  const alreadyHeld = (held?.rows ?? []).filter((g) => g.role_id === roleId)
  const heldMemberships = new Set((assignments?.rows ?? []).map((a) => a.membership_id))

  /* Every reason the button is off, per step and in the order the form is
     read. The first one is said next to the button: a disabled button with no
     sentence beside it is the console's oldest failure. */
  const who: string[] = identityId ? [] : ['Choose who needs access.']
  const whatP: string[] = []
  const where: string[] = []
  const when: string[] = []
  if (what === 'membership') {
    if (!membershipId) whatP.push('Choose a membership.')
    else if (membership && !membership.anchor_axis && heldHere.length > 0)
      whatP.push('They already hold this membership.')
    if (membership?.anchor_axis) {
      if (!seat) where.push('Choose where they hold this membership.')
      else if (heldHere.some((a) => a.place_id === seat.id && a.exact === seatExact))
        where.push(`They already hold it at ${seat.name}.`)
    }
  } else {
    if (!roleId) whatP.push('Choose a role.')
    if (!reach) where.push('Choose where this access applies.')
    /* The cards for these are disabled when a structure is required — but a
       choice made before the structures loaded would survive that, and write
       a grant authorize() drops on every decision. Refused here, not only in
       the layout. */
    if ((reach === 'everywhere' || reach === 'own') && required.length > 0)
      where.push(`Every grant must name a place in ${namesOf(required)} — choose specific places.`)
    if (reach === 'places') {
      if (includes.length === 0) where.push('Choose at least one place.')
      for (const [code, list] of Object.entries(places)) {
        if (axisNeedsAnInclude(list)) {
          const name = axes?.find((a) => a.code === code)?.display_name ?? code
          where.push(`${name}: an exception needs a place to be carved out of.`)
        }
      }
      for (const a of required) {
        if (!(places[a.code] ?? []).some((v) => !v.exclude))
          where.push(`${a.display_name} is required — choose a place in it.`)
      }
    }
  }
  // Both paths end the same way: a membership assignment has an end date too.
  if (until === 'date') {
    if (!pickedDate) when.push('Pick an end date.')
    else if (daysFrom(`${pickedDate}T12:00:00`) < 1) when.push('The end date has to be after today.')
  }
  const problems = [...who, ...whatP, ...where, ...when]
  const ready = {
    who: who.length === 0, what: whatP.length === 0,
    where: where.length === 0, when: when.length === 0,
  }

  async function submit() {
    if (!identityId || problems.length > 0) return
    setSubmitting(true)
    try {
      const name = subject?.username ?? 'They'
      if (what === 'membership' && membership) {
        const out = await api.assignMembership({
          identity_id: identityId, membership_id: membership.id,
          place_id: membership.anchor_axis ? seat?.id ?? null : null, exact: seatExact,
          valid_until: ends?.toISOString() ?? null, reason: reason.trim(),
        })
        const at = membership.anchor_axis && seat ? ` at ${seat.name}` : ''
        notifyCreated(out.assignment_id ? 'Added to membership' : 'Nothing changed',
          out.assignment_id
            ? `${name} joined “${membership.name}”${at} and received ${out.grants_created} grant${out.grants_created === 1 ? '' : 's'}.`
            : `${name} already holds “${membership.name}”${at}.`)
        await queryClient.invalidateQueries({ queryKey: qk.memberships() })
      } else if (role) {
        await api.createGrant({
          identity_id: identityId, role_id: role.id,
          self_scoped: reach === 'own', valid_until: ends?.toISOString() ?? null,
          scopes, reason: reason.trim(),
        })
        /* Carve-outs get their own clause: "3 places" for one office with two
           departments taken out of it says the grant reaches somewhere it
           does not. */
        const carved = scopes.length - includes.length
        const reachWords = reach === 'own' ? 'own records only'
          : reach === 'everywhere' ? 'everywhere'
          : `${includes.length} place${includes.length === 1 ? '' : 's'}`
            + (carved ? `, ${carved} excepted` : '')
        notifyCreated('Access given',
          `${name} → ${role.name} · ${reachWords} · ${ends ? `until ${fmtDate(ends)}` : 'no end date'}`)
      }
      /* One prefix covers every list of grants — a person's own page and the
         holders of a role — which is why they are all keyed under 'grants'. */
      await queryClient.invalidateQueries({ queryKey: ['grants'] })
      await queryClient.invalidateQueries({ queryKey: qk.dashboard() })
      reset()
      onDone?.()
    } catch (e) {
      notifyRejected(e)
    }
    setSubmitting(false)
  }

  return {
    subject: subject ?? null, subjectKind, roles, memberships, axes, required,
    what, setWhat, roleId, setRoleId, role, membershipId, setMembershipId, membership,
    seat, setSeat, seatExact, setSeatExact, heldHere,
    reach, setReach, places, setPlaces,
    until, setUntil, pickedDate, setPickedDate, ends,
    reason, setReason,
    alreadyHeld, heldMemberships, problems, ready,
    submitting, canSubmit: problems.length === 0 && !submitting,
    submit, reset,
  }
}

export type GrantDraft = ReturnType<typeof useGrantDraft>
