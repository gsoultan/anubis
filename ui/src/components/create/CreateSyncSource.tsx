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
  const [dsn, setDsn] = useState('')
  const [authHeader, setAuthHeader] = useState('')
  const [cols, setCols] = useState({ ref: '', parent_ref: '', name: '', node_type: '' })
  const [nodeType, setNodeType] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const isDB = kind === 'db_query' || kind === 'db_table'
  // db_table builds the statement, so it needs to be told which columns carry
  // the meaning. db_query returns them already named.
  const needsColumns = kind === 'db_table'
  const ready = !!axisCode && !!target.trim() && !!nodeType
    && (!isDB || !!dsn.trim())
    && (!needsColumns || (!!cols.ref.trim() && !!cols.name.trim()))

  useEffect(() => { if (opened && ctx.axisCode) setAxisCode(ctx.axisCode) }, [opened, ctx.axisCode])

  const kinds = (types ?? []).filter((t) => t.axis_code === axisCode && t.parent_types.length > 0)
  useEffect(() => {
    if (kinds.length === 1 && kinds[0]) setNodeType(kinds[0].code)
  }, [axisCode, kinds.length])  // eslint-disable-line react-hooks/exhaustive-deps

  const save = async () => {
    if (!ready || !axisCode || !nodeType) return
    setBusy(true)
    try {
      await api.createSyncSource({
        axis_code: axisCode, kind, target, default_node_type: nodeType,
        ...(isDB ? { dsn: dsn.trim() } : {}),
        ...(needsColumns ? { columns: cols } : {}),
        ...(kind === 'http' && authHeader.trim() ? { auth_header: authHeader.trim() } : {}),
      })
      notifyCreated('Source connected',
        'Run “Preview changes” to see what a sync would do before applying it.')
      await queryClient.invalidateQueries({ queryKey: qk.syncSources() })
      setTarget(''); setDsn(''); setAuthHeader('')
      setCols({ ref: '', parent_ref: '', name: '', node_type: '' })
      close()
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
        canSubmit={ready}
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
        {isDB && (
          <TextInput required label="Connection" value={dsn}
            onChange={(e) => setDsn(e.currentTarget.value)}
            placeholder="postgres://reader:secret@erp-db:5432/erp"
            description={<>The SOURCE system&rsquo;s own database, never this one. The scheme
              picks the engine — <code>postgres://</code>, <code>mysql://</code>,
              <code> mariadb://</code>. Use an account that can only read.</>} />
        )}
        {kind === 'http' && (
          <TextInput label="Authorization header" value={authHeader}
            onChange={(e) => setAuthHeader(e.currentTarget.value)}
            placeholder="Bearer eyJ…"
            description="Optional. Sent as Authorization on every fetch." />
        )}
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
        {needsColumns && (
          <div>
            <div className="t-body mb-1.5" style={{ fontWeight: 500 }}>Which columns mean what</div>
            <div className="grid gap-2" style={{ gridTemplateColumns: 'repeat(2, minmax(0,1fr))' }}>
              <TextInput required size="xs" label="Reference" value={cols.ref}
                onChange={(e) => setCols({ ...cols, ref: e.currentTarget.value })}
                placeholder="id"
                description="Stable key rows are matched on" />
              <TextInput required size="xs" label="Name" value={cols.name}
                onChange={(e) => setCols({ ...cols, name: e.currentTarget.value })}
                placeholder="display_name" />
              <TextInput size="xs" label="Parent reference" value={cols.parent_ref}
                onChange={(e) => setCols({ ...cols, parent_ref: e.currentTarget.value })}
                placeholder="parent_id"
                description="Leave empty for a flat list" />
              <TextInput size="xs" label="Item kind" value={cols.node_type}
                onChange={(e) => setCols({ ...cols, node_type: e.currentTarget.value })}
                placeholder="kind"
                description="Optional; the default below applies otherwise" />
            </div>
          </div>
        )}
        <Select label="Default item kind" required
          description="Applied to rows that do not declare one — level rules still apply."
          data={kinds.map((t) => ({ value: t.code, label: t.display_name }))}
          value={nodeType} onChange={setNodeType} />
      </div>
    </CreateShell>
  )
}
