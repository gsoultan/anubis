import { createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Button, SegmentedControl, TextInput, Tooltip } from '@mantine/core'
import { useState } from 'react'
import { IconLink, IconLinkOff, IconSearch, IconShieldCheck } from '@tabler/icons-react'
import { Page } from '@/components/shell/Page'
import { DataTable, Cell, type Column } from '@/components/ui/DataTable'
import { api } from '@/lib/api/client'
import { notifyCreated, notifyRejected } from '@/components/create/shell'
import type { AuditEntry } from '@/lib/api/types'

export const Route = createFileRoute('/audit')({ component: Audit })

function Audit() {
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

  /* Filter and page on the SERVER. The log holds far more than one page — an
     installation runs to hundreds of thousands of entries — so filtering a
     single fetched page in the browser showed the newest hundred and searched
     only those, and told an investigation "no matches" when the match was on
     entry a-hundred-and-one. The server matches action as a case-insensitive
     substring and result exactly, over the whole log, and hands back a cursor.
     A filter change starts paging over. */
  const [q, setQ] = useState('')
  const [result, setResult] = useState('all')
  const [trail, setTrail] = useState<string[]>([''])
  const cursor = trail[trail.length - 1] ?? ''
  const resetPaging = () => setTrail([''])

  const { data: page, isFetching } = useQuery({
    queryKey: ['audit', q.trim(), result, cursor],
    queryFn: () => api.audit({
      action: q.trim(),
      result: result === 'all' ? '' : result,
      cursor,
      pageSize: 100,
    }),
    placeholderData: (prev) => prev,
  })
  // The server applied the filters; re-doing it here would hide rows it
  // deliberately returned and make the page count lie.
  const shown = page?.rows ?? []
  const filtered = q.trim() !== '' || result !== 'all'

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

  const toolbar = (
    <>
      <TextInput size="xs" w={230} placeholder="Search action, e.g. login"
        leftSection={<IconSearch size={14} />}
        value={q} onChange={(e) => { setQ(e.currentTarget.value); resetPaging() }} />
      <SegmentedControl size="xs" value={result}
        onChange={(v) => { setResult(v); resetPaging() }}
        data={[{ value: 'all', label: 'All' }, { value: 'allow', label: 'Allow' },
               { value: 'deny', label: 'Deny' }, { value: 'error', label: 'Error' }]} />
    </>
  )

  const footer = (
    <>
      <span className="t-xs tnum">
        {shown.length} on this page
        {trail.length > 1 && ` · page ${trail.length}`}
      </span>
      <div className="ml-auto flex items-center gap-2">
        <Button variant="default" size="compact-sm" disabled={trail.length <= 1}
          onClick={() => setTrail((t) => t.slice(0, -1))}>Previous</Button>
        {/* The server returns a cursor only when a full page came back, so an
            absent one means this is the last page. */}
        <Button variant="default" size="compact-sm" disabled={!page?.next}
          onClick={() => setTrail((t) => [...t, page?.next ?? ''])}>Older</Button>
      </div>
    </>
  )

  return (
    <Page
      title="Audit"
      description="Every decision and change, in order, tamper-evident — each entry is chained to the previous one, so history cannot be silently rewritten."
      wide
      actions={
        <Tooltip label="Recomputes the hash chain and reports the first entry where it breaks.">
          <Button variant="default" size="xs" loading={verifying}
            leftSection={<IconShieldCheck size={14} />} onClick={verify}>
            Verify chain
          </Button>
        </Tooltip>
      }
    >
      <DataTable columns={columns} rows={page ? shown : undefined} rowKey={(e) => e.id}
        toolbar={toolbar} footer={footer} stale={isFetching}
        empty={{ title: filtered ? 'No entries match' : 'No audit entries yet',
          ...(filtered ? { hint: 'Try another search or result filter.' } : {}) }} />
    </Page>
  )
}
