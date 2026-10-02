import { explain } from '@/lib/errors'
import { Button, Drawer } from '@mantine/core'
import { notifications } from '@mantine/notifications'
import { IconCheck, IconX } from '@tabler/icons-react'
import type { ReactNode } from 'react'

/* One shell for every create form: right-hand drawer, consistent header,
   sticky footer. Drawers beat modals here because the operator often needs to
   read the page underneath — "which department was it called?" — while filling
   the form in.

   `size` is for the one form that needs room for two panes (giving access);
   every other drawer keeps 460px. `status` sits at the start of the footer and
   says why the button is off — a disabled button with no sentence beside it
   is a form that has stopped explaining itself. */
export function CreateShell({
  opened, onClose, title, description, children, footer, size = 460, status,
}: {
  opened: boolean
  onClose: () => void
  title: string
  description: ReactNode
  children: ReactNode
  footer: ReactNode
  size?: number | string
  status?: ReactNode
}) {
  return (
    <Drawer
      opened={opened}
      onClose={onClose}
      position="right"
      size={size}
      overlayProps={{ blur: 2, backgroundOpacity: 0.45, color: 'var(--overlay-tint)' }}
      styles={{
        content: { background: 'var(--s-raised)', display: 'flex', flexDirection: 'column' },
        header: { background: 'var(--s-raised)', borderBottom: '1px solid var(--line)', padding: '14px 20px' },
        title: { fontSize: 15, fontWeight: 640, letterSpacing: '-.01em' },
        body: { padding: 0, display: 'flex', flexDirection: 'column', flex: 1, minHeight: 0 },
      }}
      title={title}
    >
      <div className="t-sm px-5 pt-3.5" style={{ maxWidth: 560 }}>{description}</div>
      <div className="@container flex-1 overflow-y-auto px-5 py-4">{children}</div>
      <div
        className="flex items-center justify-end gap-2 px-5 py-3.5"
        style={{ borderTop: '1px solid var(--line)', background: 'var(--s-raised)' }}
      >
        {status && <div className="t-xs mr-auto min-w-0 truncate">{status}</div>}
        {footer}
      </div>
    </Drawer>
  )
}

/* `onSubmit` is required, and the button is a plain button rather than a
   submit one. It used to be `type="submit"` — but CreateShell renders the
   footer as a SIBLING of the drawer body, so the button sat outside the
   <form> it was meant to submit, and half the drawers have no <form> at all.
   Every create drawer's primary action therefore did nothing when clicked;
   the forms could only be sent by pressing Enter inside a field, and the four
   drawers without a <form> could not be sent at all. Requiring the handler is
   what stops the next drawer from shipping with an inert button. */
export function CancelSubmit({
  onCancel, onSubmit, canSubmit, submitting, label,
}: {
  onCancel: () => void
  onSubmit: () => void
  canSubmit: boolean
  submitting: boolean
  label: string
}) {
  return (
    <>
      <Button variant="default" size="sm" onClick={onCancel}>Cancel</Button>
      <Button size="sm" onClick={onSubmit} disabled={!canSubmit} loading={submitting}>
        {label}
      </Button>
    </>
  )
}

export const notifyCreated = (title: string, message: string) =>
  notifications.show({ color: 'teal', icon: <IconCheck size={15} />, title, message })

/* A refusal says what was wrong: the field the server named, or the sentence
   the database guard raised — not the transport's string around them
   (lib/errors.ts). */
export const notifyRejected = (err: unknown) =>
  notifications.show({
    color: 'red', icon: <IconX size={15} />, title: 'Rejected',
    message: explain(err),
    autoClose: 8000,
  })
