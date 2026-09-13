import { useEffect, useState } from 'react'
import { useCreate } from '@/stores/create'
import { IdentityPicker } from '@/components/ui/IdentityPicker'
import { CreateShell, CancelSubmit } from './shell'
import { GrantFields, useGrantDraft } from './GrantFields'

/* Giving access from anywhere: the one path that still has to ask WHO.
   Starting from a person — the People list, or their own page — the subject is
   already known, so that route grants inline on the page instead of throwing a
   drawer over the record you were reading. The fields themselves are shared
   (`GrantFields`), so both say the same thing. */
export function CreateGrant({ opened }: { opened: boolean }) {
  const { close, ctx } = useCreate()
  const [identityId, setIdentityId] = useState<string | null>(null)
  const draft = useGrantDraft({
    identityId,
    onCreated: () => { setIdentityId(null); close() },
  })

  // Preload from context ("grant a role to THIS identity" from a row action).
  useEffect(() => {
    if (opened && ctx.identityId) setIdentityId(ctx.identityId)
  }, [opened, ctx.identityId])

  return (
    <CreateShell
      opened={opened} onClose={close} title="Give access"
      description={<>A grant is an identity, a role, and one constraint per axis.
        Axes the grant is silent on are unconstrained. Constraints are AND across
        axes.</>}
      footer={
        <CancelSubmit onCancel={close} onSubmit={() => void draft.submit()}
          canSubmit={draft.canSubmit} submitting={draft.submitting} label="Give access" />
      }
    >
      <div className="flex flex-col gap-4">
        <IdentityPicker label="Who" placeholder="Search for a person…"
          value={identityId} onChange={(id) => setIdentityId(id)} />
        <GrantFields draft={draft} />
      </div>
    </CreateShell>
  )
}
