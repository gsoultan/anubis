import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Popover, Select, TextInput } from '@mantine/core'
import { IconChevronDown } from '@tabler/icons-react'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/client'
import { useCreate } from '@/stores/create'
import { ScopeTree } from '@/components/scope/ScopeTree'
import { CreateShell, CancelSubmit, notifyCreated, notifyRejected } from './shell'

const slugify = (s: string) =>
  s.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 63)

/* "Add a node" is where the hierarchy rules become visible: the node-type
   options are filtered to types whose parent_types include the chosen parent's
   type, so an illegal placement (a SKU under an office) is impossible to
   express rather than rejected after the fact. */
export function CreateNode({ opened }: { opened: boolean }) {
  const { close, ctx } = useCreate()
  const { data: axes } = useQuery({ queryKey: qk.axes(), queryFn: api.axes })
  const { data: types } = useQuery({ queryKey: qk.nodeTypes(), queryFn: api.nodeTypes })

  const [axisCode, setAxisCode] = useState<string | null>(null)
  const [parentId, setParentId] = useState<string | null>(null)
  const [nodeType, setNodeType] = useState<string | null>(null)
  const [name, setName] = useState('')
  const [slug, setSlug] = useState('')
  const [slugTouched, setSlugTouched] = useState(false)
  const [externalRef, setExternalRef] = useState('')
  const [submitting, setSubmitting] = useState(false)

  // Preload from the scope page: "add a child under THIS node".
  useEffect(() => {
    if (!opened) return
    if (ctx.axisCode) setAxisCode(ctx.axisCode)
    if (ctx.parentId) setParentId(ctx.parentId)
  }, [opened, ctx.axisCode, ctx.parentId])

  // No parent chosen → default to the axis root, so the common case
  // ("first level under the root") is zero extra clicks.
  const { data: roots } = useQuery({
    queryKey: qk.scopeChildren(axisCode ?? '', null),
    queryFn: () => api.scopeChildren(axisCode!, null),
    enabled: !!axisCode,
  })
  useEffect(() => {
    if (axisCode && !parentId && roots?.[0]) setParentId(roots[0].id)
  }, [axisCode, parentId, roots])

  const { data: parent } = useQuery({
    queryKey: qk.scopeNode(parentId ?? ''),
    queryFn: () => api.scopeNode(parentId!),
    enabled: !!parentId,
  })
  const { data: parentPath } = useQuery({
    queryKey: qk.ancestorPath(parentId ?? ''),
    queryFn: () => api.ancestorPath(parentId!),
    enabled: !!parentId,
  })

  const legalTypes = useMemo(() =>
    (types ?? []).filter((t) =>
      t.axis_code === axisCode && parent && t.parent_types.includes(parent.node_type)),
    [types, axisCode, parent])

  // A single legal child type needs no decision from the operator.
  useEffect(() => {
    if (legalTypes.length === 1 && legalTypes[0]) setNodeType(legalTypes[0].code)
    else if (nodeType && !legalTypes.some((t) => t.code === nodeType)) setNodeType(null)
  }, [legalTypes, nodeType])

  const reset = () => {
    setAxisCode(null); setParentId(null); setNodeType(null)
    setName(''); setSlug(''); setSlugTouched(false); setExternalRef('')
  }

  async function submit() {
    if (!axisCode || !parentId || !nodeType || !name) return
    setSubmitting(true)
    try {
      const node = await api.createScopeNode({
        axis_code: axisCode, parent_id: parentId, node_type: nodeType,
        name, slug: slug || slugify(name), external_ref: externalRef || null,
      })
      notifyCreated('Item added', `${node.name} under ${parent?.name}. Grants can target it immediately.`)
      await queryClient.invalidateQueries({ queryKey: qk.scope() })
      await queryClient.invalidateQueries({ queryKey: qk.dashboard() })
      reset(); close()
    } catch (e) { notifyRejected(e) }
    setSubmitting(false)
  }

  return (
    <CreateShell
      opened={opened} onClose={close} title="Add a structure item"
      description={<>An office, a product line, a customer — the things access gets limited to. In production these usually arrive by ERP/CRM sync keyed on <b>external_ref</b>; this is the manual path.</>}
      footer={
        <CancelSubmit onCancel={close}
          canSubmit={!!axisCode && !!parentId && !!nodeType && !!name}
          submitting={submitting} label="Add item" />
      }
    >
      <div className="flex flex-col gap-4">
        <Select label="Structure" required placeholder="Which tree does it belong to?"
          data={(axes ?? []).map((a) => ({ value: a.code, label: a.display_name }))}
          value={axisCode}
          onChange={(v) => { setAxisCode(v); setParentId(null); setNodeType(null) }} />

        {axisCode && (
          <div>
            <div className="t-body mb-1.5" style={{ fontWeight: 500 }}>Parent</div>
            <Popover width={340} position="left-start">
              <Popover.Target>
                <button className="flex w-full items-center justify-between gap-2 rounded-md px-2.5 py-2 text-left"
                  style={{ background: 'var(--s-sunken)', border: '1px solid var(--line-soft)' }}>
                  <span className="min-w-0">
                    <span className="t-body block truncate" style={{ fontWeight: 500 }}>
                      {parent?.name ?? 'Choose a parent'}
                    </span>
                    {parentPath && parentPath.length > 1 && (
                      <span className="t-xs block truncate" style={{ fontSize: 10 }}>
                        {parentPath.map((p) => p.name).join(' › ')}
                      </span>
                    )}
                  </span>
                  <IconChevronDown size={13} style={{ color: 'var(--ink-3)', flexShrink: 0 }} />
                </button>
              </Popover.Target>
              <Popover.Dropdown p="xs">
                <ScopeTree axis={axisCode} selectedId={parentId}
                  onSelect={(n) => { setParentId(n.id); setNodeType(null) }} />
              </Popover.Dropdown>
            </Popover>
          </div>
        )}

        {parent && (
          <Select label="Node type" required
            placeholder={legalTypes.length ? 'Pick a type' : 'No type may sit under this parent'}
            description={`Filtered to types legal under a "${parent.node_type}".`}
            data={legalTypes.map((t) => ({ value: t.code, label: t.display_name }))}
            value={nodeType} onChange={setNodeType}
            disabled={legalTypes.length === 0} />
        )}

        <TextInput label="Name" placeholder="e.g. Yogyakarta Office" required
          value={name}
          onChange={(e) => {
            setName(e.currentTarget.value)
            if (!slugTouched) setSlug(slugify(e.currentTarget.value))
          }} />

        <TextInput label="Slug" placeholder="yogyakarta-office"
          description="Unique among siblings. Auto-generated from the name."
          value={slug}
          onChange={(e) => { setSlugTouched(true); setSlug(slugify(e.currentTarget.value)) }} />

        <TextInput label="External reference" placeholder="ERP-OFF-06 (optional)"
          description="Idempotency key for sync — lets the ERP adopt this node later."
          value={externalRef} onChange={(e) => setExternalRef(e.currentTarget.value)} />
      </div>
    </CreateShell>
  )
}
