/* The editor for the one column Anubis encrypts (ADR-0013).

   It is a separate screen rather than another block of fields on the person
   form, and that is deliberate: everything on the person form is a lookup key
   stored in the clear, and everything here is sealed. An operator who cannot
   tell the two apart is an operator who eventually types a home address into
   `username`. */
import { useEffect, useState } from 'react'
import { Alert, Button, Modal, TextInput } from '@mantine/core'
import { IconLock, IconPlus, IconTrash, IconAlertTriangle } from '@tabler/icons-react'
import { notifications } from '@mantine/notifications'
import { api } from '@/lib/api/client'

type Row = { key: string; value: string }

export function AttributesModal(
  { id, label, onClose }: { id: string | null; label: string; onClose: () => void },
) {
  const [rows, setRows] = useState<Row[]>([])
  const [erased, setErased] = useState(false)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!id) return
    setLoading(true); setError(''); setErased(false)
    api.identityAttributes(id)
      .then((r) => {
        setErased(r.erased)
        setRows(Object.entries(r.attributes).map(([key, value]) => ({ key, value })))
      })
      .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)))
      .finally(() => setLoading(false))
  }, [id])

  /* A blank key is how a half-typed row looks, not something to store; the
     server would reject it anyway. Later rows win on a duplicate key, which
     matches what the list looks like on screen. */
  const attributes = Object.fromEntries(
    rows.filter((r) => r.key.trim() !== '').map((r) => [r.key.trim(), r.value]),
  )

  async function save() {
    if (!id) return
    setSaving(true); setError('')
    try {
      await api.setIdentityAttributes(id, attributes)
      notifications.show({
        color: 'teal', title: 'Attributes sealed',
        message: Object.keys(attributes).length === 0
          ? 'Cleared. Nothing is left in the column.'
          : 'Encrypted under this person’s own key before it reached the database.',
      })
      onClose()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Modal opened={id !== null} onClose={onClose} title={`Encrypted attributes — ${label}`} size="lg">
      {erased ? (
        <Alert color="orange" icon={<IconAlertTriangle size={16} />}>
          This person’s key has been shredded, so their attributes are
          unrecoverable. That is a completed erasure, not a fault — and it
          cannot be undone by writing new ones.
        </Alert>
      ) : (
        <div className="flex flex-col gap-3">
          <Alert color="gray" icon={<IconLock size={16} />}>
            Sealed before it reaches the database — the values <b>and</b> the
            field names. Saving replaces the whole set; removing every row
            clears it.
          </Alert>

          {error && <Alert color="red">{error}</Alert>}

          {rows.map((r, i) => (
            <div key={i} className="flex items-center gap-2">
              <TextInput
                placeholder="Field, e.g. employee_id" value={r.key} w={220}
                onChange={(e) => {
                  const v = e.currentTarget.value
                  setRows((rs) => rs.map((x, j) => (j === i ? { ...x, key: v } : x)))
                }} />
              <TextInput
                placeholder="Value" value={r.value} style={{ flex: 1 }}
                onChange={(e) => {
                  const v = e.currentTarget.value
                  setRows((rs) => rs.map((x, j) => (j === i ? { ...x, value: v } : x)))
                }} />
              <Button variant="subtle" color="red" size="compact-sm"
                onClick={() => setRows((rs) => rs.filter((_, j) => j !== i))}>
                <IconTrash size={14} />
              </Button>
            </div>
          ))}

          <div className="flex items-center justify-between">
            <Button variant="default" size="compact-sm" leftSection={<IconPlus size={14} />}
              onClick={() => setRows((rs) => [...rs, { key: '', value: '' }])}>
              Add field
            </Button>
            <span className="t-xs">{Object.keys(attributes).length} of 64 fields</span>
          </div>

          <div className="flex justify-end gap-2">
            <Button variant="default" onClick={onClose}>Cancel</Button>
            <Button loading={saving || loading} onClick={() => void save()}>Save</Button>
          </div>
        </div>
      )}
    </Modal>
  )
}
