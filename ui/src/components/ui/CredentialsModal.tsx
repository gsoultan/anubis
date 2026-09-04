import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button, Modal, PasswordInput, Tooltip } from '@mantine/core'
import { IconAlertTriangle } from '@tabler/icons-react'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/client'
import { notifyCreated, notifyRejected } from '@/components/create/shell'
import type { CredentialInfo } from '@/lib/api/types'

/* Incident response, which is the whole reason this exists: at three in the
   morning you want to revoke one credential, reset a password, or invalidate
   issued tokens — not read a CLI reference. ListCredentials, RevokeCredential,
   SetPassword and BumpTokenEpoch have been in the API the whole time.
   No secret is ever shown: the API returns metadata and, for API keys, the
   public lookup prefix. */

export function CredentialsModal(
  { id, label, onClose }: { id: string | null; label: string; onClose: () => void },
) {
  const { data: creds, isLoading } = useQuery({
    queryKey: qk.credentials(id ?? ''),
    queryFn: () => api.credentials(id!),
    enabled: id !== null,
  })
  const [pw, setPw] = useState('')
  const [busy, setBusy] = useState(false)

  async function refresh() {
    await queryClient.invalidateQueries({ queryKey: qk.credentials(id ?? '') })
  }

  async function revoke(c: CredentialInfo) {
    setBusy(true)
    try {
      await api.revokeCredential(c.id)
      await refresh()
      notifyCreated(`${c.kind} revoked`, 'It can no longer be used to authenticate.')
    } catch (e) { notifyRejected(e) } finally { setBusy(false) }
  }

  async function resetPassword() {
    setBusy(true)
    try {
      await api.setPassword(id!, pw)
      setPw('')
      await refresh()
      notifyCreated('Password set', 'Existing sessions are unaffected — end them separately if this is a compromise.')
    } catch (e) {
      /* The realm's password policy is checked server-side, so the rejection
         names the rule that failed rather than saying "invalid". */
      notifyRejected(e)
    } finally { setBusy(false) }
  }

  async function bump() {
    setBusy(true)
    try {
      const epoch = await api.bumpTokenEpoch(id!)
      notifyCreated(`Token epoch is now ${epoch}`,
        'Access tokens already issued are refused. Sessions and refresh tokens are not affected.')
    } catch (e) { notifyRejected(e) } finally { setBusy(false) }
  }

  const rows = creds ?? []

  return (
    <Modal opened={id !== null} onClose={onClose} title={`Credentials — ${label}`} size="lg">
      <div className="flex flex-col gap-4">
        <div>
          <div className="t-label mb-2">Enrolled</div>
          {isLoading && <div className="t-xs">Loading…</div>}
          {!isLoading && rows.length === 0 && (
            <div className="t-xs">None. This identity cannot authenticate.</div>
          )}
          <div className="flex flex-col gap-1.5">
            {rows.map((c) => (
              <div key={c.id} className="flex items-center gap-2 rounded px-2 py-1.5"
                style={{ background: 'var(--s-sunken)' }}>
                <div className="min-w-0 flex-1">
                  <div className="t-body" style={{ fontWeight: 550 }}>
                    {c.kind}
                    {c.label && <span style={{ opacity: 0.7 }}> · {c.label}</span>}
                    {c.lookup_key && <span className="chip" style={{ marginLeft: 6 }}>{c.lookup_key}</span>}
                  </div>
                  <div className="t-xs" style={{ opacity: 0.65 }}>
                    {c.created_at ? `added ${c.created_at.slice(0, 10)}` : 'added —'}
                    {c.last_used_at ? ` · last used ${c.last_used_at.slice(0, 10)}` : ' · never used'}
                    {c.expires_at ? ` · expires ${c.expires_at.slice(0, 10)}` : ''}
                  </div>
                </div>
                {c.revoked_at ? (
                  <span className="v-pill v-pill-idle">revoked</span>
                ) : (
                  <Button size="compact-xs" variant="default" color="red" loading={busy}
                    onClick={() => revoke(c)}>
                    Revoke
                  </Button>
                )}
              </div>
            ))}
          </div>
        </div>

        <div style={{ borderTop: '1px solid var(--line-soft)', paddingTop: 14 }}>
          <div className="t-label mb-2">Set a password</div>
          <div className="flex items-end gap-2">
            <PasswordInput
              style={{ flex: 1 }} value={pw} placeholder="New password"
              onChange={(e) => setPw(e.currentTarget.value)}
            />
            <Button size="xs" loading={busy} disabled={pw.length < 8} onClick={resetPassword}>
              Set
            </Button>
          </div>
          <div className="t-xs mt-1" style={{ opacity: 0.7 }}>
            Checked against this population's password policy, so a rejection names the rule.
          </div>
        </div>

        <div style={{ borderTop: '1px solid var(--line-soft)', paddingTop: 14 }}>
          <div className="t-label mb-2">Invalidate issued access tokens</div>
          <div className="flex items-start gap-3">
            <div className="flex shrink-0 items-center justify-center rounded-lg"
              style={{
                width: 30, height: 30,
                background: 'color-mix(in srgb, var(--warn) 9%, transparent)',
                border: '1px solid color-mix(in srgb, var(--warn) 20%, transparent)',
              }}>
              <IconAlertTriangle size={15} style={{ color: 'var(--warn)' }} />
            </div>
            <div className="flex-1">
              {/* Naming this "sign out everywhere" would be wrong, and wrong in
                  the direction that matters: an operator handling a compromise
                  would believe the attacker was out. Signing out is three
                  operations — revoke sessions, revoke refresh tokens, bump the
                  epoch — and this is only the third. */}
              <div className="t-body" style={{ opacity: 0.85 }}>
                Refuses every access token already issued to this identity. It does
                <strong> not</strong> end sessions or revoke refresh tokens — a held refresh
                token immediately mints a new access token that is accepted.
              </div>
              <div className="t-xs mt-1" style={{ opacity: 0.7 }}>
                To cut off access entirely, disable the identity: that revokes the sessions
                and their refresh tokens as well, in one transaction.
              </div>
              <Tooltip label="Increments identities.token_epoch. The gate compares it against the epoch inside each token.">
                <Button className="mt-2" size="xs" variant="default" loading={busy} onClick={bump}>
                  Invalidate issued tokens
                </Button>
              </Tooltip>
            </div>
          </div>
        </div>

        <div className="flex justify-end">
          <Button variant="default" size="xs" onClick={onClose}>Close</Button>
        </div>
      </div>
    </Modal>
  )
}
