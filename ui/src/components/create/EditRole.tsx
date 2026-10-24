import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button, Checkbox, TextInput } from '@mantine/core'
import { FeaturePicker } from '@/components/access/FeaturePicker'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/client'
import { useCreate } from '@/stores/create'
import { KINDS } from './CreateRole'
import { CreateShell, CancelSubmit, notifyCreated, notifyRejected } from './shell'
import type { Permission, RealmKind } from '@/lib/api/types'

/** Keys in the role that name no single permission: patterns such as
    invoices:invoice:*. Kept as they are; the picker only lists real ones. */
export function Patterns({ keys, perms }: { keys: string[]; perms: Permission[] }) {
  const odd = keys.filter((k) => !perms.some((p) => p.key === k))
  if (odd.length === 0) return null
  return (
    <div className="t-xs mt-2 flex flex-wrap items-center gap-1.5">
      Also matches every permission like:
      {odd.map((k) => <span key={k} className="chip font-mono">{k}</span>)}
    </div>
  )
}

/* Where "remove a permission from a role" lives: deselect it here and save.
   Changes bite immediately — every holder's next access check uses the new
   bundle, which is exactly why the two guards exist (manifest roles locked,
   narrowing holders blocked). */
export function EditRole({ opened }: { opened: boolean }) {
  const { close, ctx } = useCreate()
  const { data: roles } = useQuery({ queryKey: qk.roles(), queryFn: api.roles })
  const { data: perms } = useQuery({ queryKey: qk.permissions(), queryFn: api.permissions })
  const role = roles?.find((r) => r.id === ctx.roleId)
  const { data: currentKeys } = useQuery({
    queryKey: qk.rolePermissions(ctx.roleId ?? ''),
    queryFn: () => api.rolePermissions(ctx.roleId!),
    enabled: opened && !!ctx.roleId,
  })

  const [description, setDescription] = useState('')
  const [kinds, setKinds] = useState<RealmKind[]>([])
  const [keys, setKeys] = useState<string[]>([])
  const [busy, setBusy] = useState(false)
  const [armed, setArmed] = useState(false)

  useEffect(() => {
    if (!opened || !role) return
    setDescription(role.description)
    setKinds(role.allowed_realm_kinds)
    setArmed(false)
  }, [opened, role])
  useEffect(() => { if (currentKeys) setKeys(currentKeys) }, [currentKeys])

  const removed = (currentKeys ?? []).filter((k) => !keys.includes(k))
  const added = keys.filter((k) => !(currentKeys ?? []).includes(k))

  const save = async () => {
    if (!role) return
    setBusy(true)
    try {
      await api.updateRole({ role_id: role.id, description,
        allowed_realm_kinds: kinds, permission_keys: keys })
      notifyCreated('Role updated',
        `${role.name}: ${added.length} added, ${removed.length} removed. Every holder's next check uses the new bundle.`)
      await queryClient.invalidateQueries({ queryKey: qk.roles() })
      await queryClient.invalidateQueries({ queryKey: qk.rolePermissions(role.id) })
      close()
    } catch (e) { notifyRejected(e) }
    setBusy(false)
  }

  const remove = async () => {
    if (!role) return
    if (!armed) { setArmed(true); return }
    setBusy(true)
    try {
      await api.deleteRole(role.id)
      notifyCreated('Role deleted', `“${role.name}” is gone; the deletion is in the audit log.`)
      await queryClient.invalidateQueries({ queryKey: qk.roles() })
      await queryClient.invalidateQueries({ queryKey: qk.audit() })
      close()
    } catch (e) { notifyRejected(e); setArmed(false) }
    setBusy(false)
  }

  return (
    <CreateShell
      opened={opened} onClose={close} title={`Edit ${role?.name ?? 'role'}`}
      description={<>Add or remove permissions and change who may hold it. Edits apply to
        <b> every holder immediately</b> — there is no per-person copy to drift.</>}
      footer={<CancelSubmit onCancel={close} onSubmit={() => void save()}
        canSubmit={keys.length > 0 && kinds.length > 0 && !!role && !role.is_system}
        submitting={busy} label="Save changes" />}
    >
      <div className="flex flex-col gap-4">
        <TextInput label="Description" value={description}
          onChange={(e) => setDescription(e.currentTarget.value)} />
        <div>
          <div className="t-body mb-1.5" style={{ fontWeight: 500 }}>Grantable to</div>
          <div className="flex flex-col gap-2">
            {KINDS.map((k) => (
              <Checkbox key={k.value}
                label={<span>{k.label} <span className="t-xs">— {k.hint}</span></span>}
                checked={kinds.includes(k.value)}
                onChange={(e) => setKinds(e.currentTarget.checked
                  ? [...kinds, k.value] : kinds.filter((x) => x !== k.value))} />
            ))}
          </div>
        </div>
        <div>
          <div className="t-body mb-0.5" style={{ fontWeight: 500 }}>What it permits</div>
          <div className="t-xs mb-2">By app, part of the app and feature. Untick to remove; the role must keep at least one.</div>
          <FeaturePicker perms={perms ?? []} apps={[]} appId={null} value={keys} onChange={setKeys} />
          <Patterns keys={keys} perms={perms ?? []} />
        </div>
        {(removed.length > 0 || added.length > 0) && (
          <div className="panel-inset px-3 py-2.5">
            <div className="t-label mb-1.5">Pending changes</div>
            {added.map((k) => <div key={k} className="t-xs" style={{ color: 'var(--allow)' }}>+ {k}</div>)}
            {removed.map((k) => <div key={k} className="t-xs" style={{ color: 'var(--deny)' }}>− {k}</div>)}
          </div>
        )}

        <div className="panel-inset px-3 py-3"
          style={{ borderColor: 'color-mix(in srgb, var(--deny) 25%, transparent)' }}>
          <div className="t-label mb-1" style={{ color: 'var(--deny)' }}>Danger zone</div>
          <div className="t-xs mb-2.5">
            Deletion is blocked while anyone holds this role or a membership bundles it —
            revoke and unbundle first. The schema enforces the same rule.
          </div>
          <Button size="xs" color="red" variant={armed ? 'filled' : 'light'}
            loading={busy && armed} onClick={() => void remove()}>
            {armed ? 'Click again to permanently delete' : 'Delete role…'}
          </Button>
        </div>
      </div>
    </CreateShell>
  )
}
