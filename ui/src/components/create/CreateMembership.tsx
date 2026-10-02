import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Radio, Select, TextInput } from '@mantine/core'
import { IconMapPin, IconUsersGroup } from '@tabler/icons-react'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/client'
import { useCreate } from '@/stores/create'
import { AxisIcon } from '@/components/scope/AxisIcon'
import { MembershipEntries, type EntryDraft } from '@/components/access/MembershipEntries'
import { CreateShell, CancelSubmit, notifyCreated, notifyRejected } from './shell'

/* A membership is a named set of roles, held by people.

   The first question is whose places it uses, because it cannot be changed
   afterwards: every grant a membership has already given was placed by that
   answer.

   - Same places for everyone: each role names its places, and every member
     gets exactly those. "Jakarta Finance Team".
   - Where each member is assigned: the membership names a structure, and each
     person is placed somewhere in it when they are added. A seat on the
     Marketing Council in Company A gives nothing in Company B. */

type Mode = 'shared' | 'placed'

export function CreateMembership({ opened }: { opened: boolean }) {
  const { close } = useCreate()
  const { data: axes } = useQuery({ queryKey: qk.axes(), queryFn: api.axes })

  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [mode, setMode] = useState<Mode>('shared')
  const [anchor, setAnchor] = useState<string | null>(null)
  const [entries, setEntries] = useState<EntryDraft[]>([])
  const [busy, setBusy] = useState(false)

  const anchorAxis = mode === 'placed' ? anchor : null
  const anchorName = axes?.find((a) => a.code === anchor)?.display_name

  const reset = () => {
    setName(''); setDescription(''); setMode('shared'); setAnchor(null); setEntries([])
  }
  const cancel = () => { reset(); close() }

  const problem = name.trim().length < 2 ? 'Name the membership.'
    : mode === 'placed' && !anchor ? 'Choose the structure members are placed in.'
    : entries.length === 0 ? 'Add at least one role.'
    : undefined

  const submit = async () => {
    setBusy(true)
    try {
      await api.createMembership({ name: name.trim(), description, anchor_axis: anchorAxis, entries })
      notifyCreated('Membership created',
        `“${name.trim()}” — ${entries.length} role${entries.length > 1 ? 's' : ''}. Add people from the membership, or from their own page.`)
      await queryClient.invalidateQueries({ queryKey: qk.memberships() })
      reset(); close()
    } catch (e) { notifyRejected(e) }
    setBusy(false)
  }

  return (
    <CreateShell
      opened={opened} onClose={cancel} title="New membership"
      description="A named set of roles that people hold together — a team, a council, a committee. Add someone and they get all of it; remove them and it is all taken back."
      status={problem}
      footer={<CancelSubmit onCancel={cancel} onSubmit={() => void submit()}
        canSubmit={!problem} submitting={busy} label="Create membership" />}
    >
      <div className="flex flex-col gap-4">
        <TextInput label="Name" placeholder="Marketing Council" required
          value={name} onChange={(e) => setName(e.currentTarget.value)} />
        <TextInput label="Description" placeholder="Who is in it, and what it is for"
          value={description} onChange={(e) => setDescription(e.currentTarget.value)} />

        <div>
          <div className="t-label mb-2">Where it applies</div>
          {/* Changing this drops the roles added so far: a role's places mean
              different things under the two answers. */}
          <Radio.Group value={mode} aria-label="Where it applies"
            onChange={(v) => { setMode(v as Mode); setEntries([]) }}>
            <div className="grid grid-cols-1 gap-2 @lg:grid-cols-2">
              <Radio.Card value="shared" className="choice">
                <span className="flex items-center gap-2">
                  <span className="choice-icon"><IconUsersGroup size={16} /></span>
                  <span className="t-body" style={{ fontWeight: 600, color: 'var(--ink)' }}>Same places for everyone</span>
                </span>
                <span className="t-xs mt-1 block">Each role names its places. Every member gets exactly those.</span>
              </Radio.Card>
              <Radio.Card value="placed" className="choice">
                <span className="flex items-center gap-2">
                  <span className="choice-icon"><IconMapPin size={16} /></span>
                  <span className="t-body" style={{ fontWeight: 600, color: 'var(--ink)' }}>Where each member is assigned</span>
                </span>
                <span className="t-xs mt-1 block">Each person is placed when they are added — one company, one office — and holds it only there.</span>
              </Radio.Card>
            </div>
          </Radio.Group>
          {mode === 'placed' && (
            <Select mt="sm" label="Members are placed in" placeholder="Choose a structure" required
              data={(axes ?? []).map((a) => ({ value: a.code, label: a.display_name }))}
              value={anchor} onChange={(v) => { setAnchor(v); setEntries([]) }} />
          )}
          <div className="t-xs mt-2">
            {mode === 'placed'
              ? `A member placed at one ${anchorName ? anchorName.toLowerCase() + ' place' : 'place'} gets these roles there and in everything inside it, unless they are added for that place alone. This cannot be changed after the membership is created.`
              : 'This cannot be changed after the membership is created.'}
          </div>
        </div>

        {(mode === 'shared' || anchor) && (
          <MembershipEntries key={`${mode}:${anchor ?? ''}`} anchorAxis={anchorAxis} axes={axes}
            entries={entries} onChange={setEntries} />
        )}

        <div className="t-xs flex items-center gap-1.5">
          <AxisIcon name="users" size={12} />
          Memberships stay flat — one never contains another. That is what keeps them auditable.
        </div>
      </div>
    </CreateShell>
  )
}
