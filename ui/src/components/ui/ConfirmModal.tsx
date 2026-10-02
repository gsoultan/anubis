import { useState, type ReactNode } from 'react'
import { Button, Modal } from '@mantine/core'

/* One confirmation, so every irreversible action on a person asks the same
   way: what happens, to whom, and what does NOT happen. Revoking used to be a
   single menu click with no second look — on the one screen where an operator
   is reading somebody's whole access and clicking near it all day.

   `onConfirm` may reject; the dialog stays open so the error toast sits next
   to the thing that failed instead of replacing it. */
export function ConfirmModal({
  opened, onClose, title, children, confirmLabel, onConfirm, tone = 'danger',
}: {
  opened: boolean
  onClose: () => void
  title: string
  children: ReactNode
  confirmLabel: string
  onConfirm: () => Promise<void>
  tone?: 'danger' | 'default'
}) {
  const [busy, setBusy] = useState(false)
  async function run() {
    setBusy(true)
    try {
      await onConfirm()
      onClose()
    } catch {
      // The caller has already said why; staying open keeps the context.
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal opened={opened} onClose={busy ? () => {} : onClose} title={title} size={440}>
      <div className="t-sm flex flex-col gap-2 pt-1">{children}</div>
      <div className="mt-5 flex justify-end gap-2">
        <Button variant="default" size="sm" onClick={onClose} disabled={busy}>Cancel</Button>
        <Button size="sm" {...(tone === 'danger' ? { color: 'deny' } : {})} loading={busy}
          onClick={() => void run()}>
          {confirmLabel}
        </Button>
      </div>
    </Modal>
  )
}
