import { Button, Modal, PasswordInput } from '@mantine/core'
import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { api } from '@/lib/anubis'
import { signOut } from '@/stores/auth'
import { notifyCreated, notifyRejected } from '@/components/create/shell'

/** An operator changing their own password.
 *
 *  Until this existed the console could not do it at all: the only way an
 *  operator password ever changed was that somebody reinstalled. The RPC
 *  shipped first and reached nobody, because an operator lives in here.
 *
 *  Changing a password ends every session it opened, this one included —
 *  that is the point of it rather than a side effect, so the screen signs
 *  out on success instead of letting the next click fail with a 401 nobody
 *  can explain.
 */
export function ChangePassword({ opened, onClose }: { opened: boolean; onClose: () => void }) {
  const navigate = useNavigate()
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [again, setAgain] = useState('')
  const [busy, setBusy] = useState(false)

  const short = next.length > 0 && next.length < 12
  const mismatch = again.length > 0 && next !== again
  const same = next.length > 0 && next === current
  const ready = current.length > 0 && next.length >= 12 && next === again && !same

  const close = () => {
    setCurrent(''); setNext(''); setAgain('')
    onClose()
  }

  const submit = async () => {
    setBusy(true)
    try {
      await api.platformSession.changePlatformPassword({
        currentPassword: current, newPassword: next,
      })
      notifyCreated('Password changed',
        'Every session opened with the old password has ended, including this one.')
      close()
      await signOut()
      navigate({ to: '/signin', search: { next: '/' } })
    } catch (e) { notifyRejected(e) } finally { setBusy(false) }
  }

  return (
    <Modal opened={opened} onClose={close} title="Change your password" centered>
      <div className="flex flex-col gap-2.5">
        <p className="t-sm">
          You will be signed out here and anywhere else this account is open.
        </p>
        <PasswordInput label="Current password" value={current} autoFocus
          onChange={(e) => setCurrent(e.currentTarget.value)} />
        <PasswordInput label="New password" value={next}
          error={short ? 'At least 12 characters' : same ? 'Must differ from the current one' : null}
          onChange={(e) => setNext(e.currentTarget.value)} />
        <PasswordInput label="New password again" value={again}
          error={mismatch ? 'These do not match' : null}
          onChange={(e) => setAgain(e.currentTarget.value)} />
        <Button loading={busy} disabled={!ready} onClick={() => void submit()}>
          Change password
        </Button>
      </div>
    </Modal>
  )
}
