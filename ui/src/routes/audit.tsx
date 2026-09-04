import { createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Button, SegmentedControl, TextInput, Tooltip } from '@mantine/core'
import { useState } from 'react'
import { IconLink, IconLinkOff, IconSearch, IconShieldCheck } from '@tabler/icons-react'
import { Page } from '@/components/shell/Page'
import { DataTable, Cell, type Column } from '@/components/ui/DataTable'
import { api } from '@/lib/api/client'
import { notifyCreated, notifyRejected } from '@/components/create/shell'
import { qk } from '@/lib/query/keys'
import type { AuditEntry } from '@/lib/api/types'

export const Route = createFileRoute('/audit')({ component: Audit })

function Audit() {
  const { data: rows } = useQuery({ queryKey: qk.audit(), queryFn: api.audit })
  /* The description above promises the log is tamper-evident. Evidence
     nobody can check is not evidence, so the check is on the page that makes
     the claim. */
  const [verifying, setVerifying] = useState(false)

  async function verify() {
    setVerifying(true)
    try {
      const r = await api.verifyAuditChain()
      if (r.ok) {
        notifyCreated('Chain intact',
          `${r.checked.toLocaleString()} entries recomputed; each one hashes to the next.`)
      } else {
        notifyRejected(new Error(
          `Chain broken at sequence ${r.brokenAtSeq} after ${r.checked.toLocaleString()} entries. ` +
          'An entry was altered or removed after it was written.'))
      }
    } catch (e) { notifyRejected(e) } finally { setVerifying(false) }
  }

  const [q, setQ] = useState('')
  const [result, setResult] = useState('all')
  const needle = q.trim().toLowerCase()
  const shown = (rows ?? []).filter((e) =>
    (result === 'all' || e.result === result) &&
    (!needle || e.actor_label.toLowerCase().includes(needle) ||
      e.action.toLowerCase().includes(needle)))

  const columns: Column<AuditEntry>[] = [
    { key: 'when', header: 'When', width: 165, render: (e) => (
        <Cell top={<span className="tnum">{e.occurred_at.slice(11, 19)}</span>}
          bottom={e.occurred_at.slice(0, 10)} />
      ) },
    { key: 'actor', header: 'Actor', width: 200, render: (e) => (
        <Cell top={e.actor_label} bottom={e.ip ?? undefined} />
      ) },
    { key: 'action', header: 'Action', width: 230, render: (e) => (
        <span className="font-mono" style={{ fontSize: 11.5 }}>{e.action}</span>
      ) },
    { key: 'result', header: 'Result', width: 100, render: (e) => (
        <span className={`v-pill ${e.result === 'allow' ? 'v-pill-allow'
          : e.result === 'deny' ? 'v-pill-deny' : 'v-pill-idle'}`}>
          {e.result}
        </span>
      ) },
    { key: 'detail', header: 'Detail', render: (e) => (
        <div className="flex flex-wrap gap-1">
          {Object.entries(e.detail).map(([k, v]) => (
            <span key={k} className="chip">
              <span style={{ color: 'var(--ink-4)' }}>{k}</span>
              <span style={{ margin: '0 3px', color: 'var(--ink-4)' }}>=</span>
              {String(v)}
            </span>
          ))}
        </div>
      ) },
    { key: 'chain', header: 'Chain', width: 70, render: (e) => (
        <Tooltip label={e.chain_ok
          ? 'Hash matches the previous entry.'
          : 'Chain broken — tampering, or a bug destroying evidentiary value.'}>
          <span>{e.chain_ok
            ? <IconLink size={13} style={{ color: 'var(--allow)' }} />
            : <IconLinkOff size={13} style={{ color: 'var(--deny)' }} />}</span>
        </Tooltip>
      ) },
  ]

  return (
    <Page
      title="Audit"
      description="Every decision and change, in order, tamper-evident — each entry is chained to the previous one, so history cannot be silently rewritten."
      wide
      actions={
        <>
          <TextInput w={210} placeholder="Search actor or action"
            leftSection={<IconSearch size={14} />}
            value={q} onChange={(e) => setQ(e.currentTarget.value)} />
          <SegmentedControl size="xs" value={result} onChange={setResult}
            data={[{ value: 'all', label: 'All' }, { value: 'allow', label: 'Allow' },
                   { value: 'deny', label: 'Deny' }, { value: 'error', label: 'Error' }]} />
          <Tooltip label="Recomputes the hash chain and reports the first entry where it breaks.">
            <Button variant="default" size="xs" loading={verifying}
              leftSection={<IconShieldCheck size={14} />} onClick={verify}>
              Verify chain
            </Button>
          </Tooltip>
        </>
      }
    >
      <DataTable columns={columns} rows={shown} rowKey={(e) => e.id}
        empty={{ title: needle || result !== 'all' ? 'No entries match' : 'No audit entries yet',
          ...(needle || result !== 'all' ? { hint: 'Try another search or result filter.' } : {}) }} />
    </Page>
  )
}
