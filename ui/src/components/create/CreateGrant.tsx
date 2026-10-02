import { useCreate } from '@/stores/create'
import { GiveAccessSheet } from '@/components/access/GiveAccessSheet'

/* Giving access from anywhere — the header's Add menu, ⌘K, the overview's
   first steps. It is the same sheet a person's own page opens, so the form
   cannot teach two different rules depending on where it was started; the
   only difference is that from here it has to ask WHO first, unless the
   caller already said (`openCreate('grant', { identityId })`). */
export function CreateGrant({ opened }: { opened: boolean }) {
  const { close, ctx } = useCreate()
  return (
    <GiveAccessSheet opened={opened} onClose={close}
      identityId={ctx.identityId ?? null} pickSubject={!ctx.identityId} />
  )
}
