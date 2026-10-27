import { Button, Code, Modal } from '@mantine/core'

/** A secret is shown exactly once. If it is lost, the only way back is to
    rotate — which is why this is a panel and not a toast. */
export function SecretOnce({ secret, onDone }: { secret: string; onDone: () => void }) {
  return (
    <Modal opened onClose={onDone} title="Client secret" centered>
      <p className="t-sm mb-3">
        Copy this now. It is not stored anywhere readable and will never be
        shown again — losing it means rotating, which breaks whatever is
        already using the old one.
      </p>
      <Code block>{secret}</Code>
      <Button className="mt-3" onClick={onDone}>I have copied it</Button>
    </Modal>
  )
}
