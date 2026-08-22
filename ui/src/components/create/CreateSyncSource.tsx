import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Select, Textarea, TextInput } from '@mantine/core'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/client'
import { useCreate } from '@/stores/create'
import { CreateShell, CancelSubmit, notifyCreated, notifyRejected } from './shell'
import type { SyncKind } from '@/lib/api/types'

const KINDS: { value: SyncKind; label: string; hint: string }[] = [
  { value: 'http', label: 'HTTP API', hint: 'Anubis calls an endpoint that returns the rows' },
  { value: 'db_query', label: 'Database query', hint: 'A SQL query against the source database' },
  { value: 'db_table', label: 'Database table', hint: 'Read a table directly, columns mapped' },
]

export function CreateSyncSource({ opened }: { opened: boolean }) {
  const { close, ctx } = useCreate()
  const { data: axes } = useQuery({ queryKey: qk.axes(), queryFn: api.axes })
  const { data: types } = useQuery({ queryKey: qk.nodeTypes(), queryFn: api.nodeTypes })

  const [axisCode, setAxisCode] = useState<string | null>(null)
  const [kind, setKind] = useState<SyncKind>('http')
  const [target, setTarget] = useState('')
  const [nodeType, setNodeType] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  useEffect(() => { if (opened && ctx.axisCode) setAxisCode(ctx.axisCode) }, [opened, ctx.axisCode])

  const kinds = (types ?? []).filter((t) => t.axis_code === axisCode && t.parent_types.length > 0)
  useEffect(() => {
    if (kinds.length === 1 && kinds[0]) setNodeType(kinds[0].code)
  }, [axisCode, kinds.length])  // eslint-disable-line react-hooks/exhaustive-deps

  const save = async () => {
    if (!axisCode || !target.trim() || !nodeType) return
    setBusy(true)
    try {
      await api.createSyncSource({ axis_code: axisCode, kind, target, default_node_type: nodeType })
      notifyCreated('Source connected',
        'Run “Preview changes” to see what a sync would do before applying it.')
      await queryClient.invalidateQueries({ queryKey: qk.syncSources() })
      setTarget(''); close()
    } catch (e) { notifyRejected(e) }
    setBusy(false)
  }

  return (
    <CreateShell
      opened={opened} onClose={close} title="Connect a sync source"
      description={<>Point a structure at the system that owns it — the ERP for products, the
        CRM for customers. Rows are matched by a stable reference, vanished rows are
        <b> archived, never deleted</b>, and items created by hand are never touched.</>}
      footer={<CancelSubmit onCancel={close}
        canSubmit={!!axisCode && !!target.trim() && !!nodeType}
        submitting={busy} label="Connect source" />}
    >
      <div className="flex flex-col gap-4">
        <Select label="Structure" required data={(axes ?? []).map((a) => ({ value: a.code, label: a.display_name }))}
          value={axisCode} onChange={setAxisCode}
          description="One source of truth per structure." />
        <div>
          <div className="t-body mb-1.5" style={{ fontWeight: 500 }}>How to reach it</div>
          <div className="flex flex-col gap-1.5">
            {KINDS.map((k) => (
              <button key={k.value} onClick={() => setKind(k.value)}
                className="panel-inset flex items-center justify-between gap-2 px-3 py-2 text-left"
                style={kind === k.value ? { borderColor: 'var(--gold-chip-line)', background: 'var(--gold-chip-bg)' } : undefined}>
                <span className="t-body" style={{ fontWeight: 530 }}>{k.label}</span>
                <span className="t-xs" style={{ textAlign: 'right' }}>{k.hint}</span>
              </button>
            ))}
          </div>
        </div>
        {kind === 'db_query' ? (
          <Textarea label="Query" autosize minRows={3}
            placeholder={'SELECT code AS ref, parent_code AS parent_ref, name\nFROM erp.product_lines'}
            description="Must return ref, parent_ref and name columns."
            value={target} onChange={(e) => setTarget(e.currentTarget.value)} />
        ) : (
          <TextInput required
            label={kind === 'http' ? 'Endpoint URL' : 'Table name'}
            placeholder={kind === 'http' ? 'https://erp.example.com/api/suppliers' : 'erp.suppliers'}
            description={kind === 'http'
              ? 'Must return rows of { ref, parent_ref, name }.'
              : 'Columns ref, parent_ref and name are read.'}
            value={target} onChange={(e) => setTarget(e.currentTarget.value)} />
        )}
        <Select label="Default item kind" required
          description="Applied to rows that do not declare one — level rules still apply."
          data={kinds.map((t) => ({ value: t.code, label: t.display_name }))}
          value={nodeType} onChange={setNodeType} />
      </div>
    </CreateShell>
  )
}
