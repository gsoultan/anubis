import { Button, Drawer } from '@mantine/core'
import { notifications } from '@mantine/notifications'
import { IconCheck, IconX } from '@tabler/icons-react'
import type { ReactNode } from 'react'

/* One shell for every create form: right-hand drawer, consistent header,
   sticky footer. Drawers beat modals here because the operator often needs to
   read the page underneath — "which department was it called?" — while filling
   the form in. */
export function CreateShell({
  opened, onClose, title, description, children, footer,
}: {
  opened: boolean
  onClose: () => void
  title: string
  description: ReactNode
  children: ReactNode
  footer: ReactNode
}) {
  return (
    <Drawer
      opened={opened}
      onClose={onClose}
      position="right"
      size={460}
      overlayProps={{ blur: 2, backgroundOpacity: 0.45, color: 'var(--overlay-tint)' }}
      styles={{
        content: { background: 'var(--s-raised)', display: 'flex', flexDirection: 'column' },
        header: { background: 'var(--s-raised)', borderBottom: '1px solid var(--line)', padding: '14px 20px' },
        title: { fontSize: 15, fontWeight: 640, letterSpacing: '-.01em' },
        body: { padding: 0, display: 'flex', flexDirection: 'column', flex: 1, minHeight: 0 },
      }}
      title={title}
    >
      <div className="t-sm px-5 pt-3.5" style={{ maxWidth: 400 }}>{description}</div>
      <div className="flex-1 overflow-y-auto px-5 py-4">{children}</div>
      <div
        className="flex items-center justify-end gap-2 px-5 py-3.5"
        style={{ borderTop: '1px solid var(--line)', background: 'var(--s-raised)' }}
      >
        {footer}
      </div>
    </Drawer>
  )
}

export function CancelSubmit({
  onCancel, canSubmit, submitting, label,
}: {
  onCancel: () => void
  canSubmit: boolean
  submitting: boolean
  label: string
}) {
  return (
    <>
      <Button variant="default" size="sm" onClick={onCancel}>Cancel</Button>
      <Button type="submit" size="sm" disabled={!canSubmit} loading={submitting}>{label}</Button>
    </>
  )
}

export const notifyCreated = (title: string, message: string) =>
  notifications.show({ color: 'teal', icon: <IconCheck size={15} />, title, message })

/* Guard violations from the backend arrive as thrown Errors with the same
   message the SQL trigger raises. Showing them verbatim is the point — the
   console demonstrates the schema guard instead of translating it into a
   generic "something went wrong". */
export const notifyRejected = (err: unknown) =>
  notifications.show({
    color: 'red', icon: <IconX size={15} />, title: 'Rejected',
    message: err instanceof Error ? err.message : String(err),
    autoClose: 8000,
  })
