import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ActionIcon, Button, Code, Menu, Modal, SegmentedControl, Select, TextInput } from '@mantine/core'
import { IconArrowDown, IconArrowUp, IconDots, IconPencil, IconPlus, IconRoute, IconTrash } from '@tabler/icons-react'
import { SortableList } from '@/components/page/SortableList'
import { notifyCreated, notifyRejected } from '@/components/create/shell'
import { queryClient } from '@/lib/query/client'
import { qk } from '@/lib/query/keys'
import { api } from '@/lib/api/client'
import * as live from '@/lib/api/live'
import { firstMatch, methodWords } from '@/lib/routes'
import type { Permission, RouteEffect, RouteRule } from '@/lib/api/types'

/* An app's page rules: which addresses in it need what. The gate reads them
   top to bottom and the first that matches decides, so the order is the
   policy and the list is edited as a whole, checked, then saved in one go. */

type For = 'any' | 'view' | 'change' | 'as-is'
const FOR_METHODS: Record<Exclude<For, 'as-is'>, string[]> = {
  any: [], view: ['GET', 'HEAD'], change: ['POST', 'PUT', 'PATCH', 'DELETE'],
}

function forOf(methods: string[]): For {
  const w = methodWords(methods)
  return w === 'any request' ? 'any' : w === 'viewing' ? 'view' : w === 'changes only' ? 'change' : 'as-is'
}

const EFFECTS: { value: RouteEffect; label: string }[] = [
  { value: 'public', label: 'Anyone' },
  { value: 'require_auth', label: 'Anyone signed in' },
  { value: 'require_permission', label: 'People with a permission' },
  { value: 'deny', label: 'Nobody' },
]

const EFFECT_TONE: Record<RouteEffect, { fg: string; bg: string }> = {
  public: { fg: 'var(--ink-2)', bg: 'var(--s-sunken)' },
  require_auth: { fg: 'var(--accent)', bg: 'var(--accent-bg)' },
  require_permission: { fg: 'var(--accent)', bg: 'var(--accent-bg)' },
  deny: { fg: 'var(--deny)', bg: 'var(--deny-bg)' },
}

export function permissionLabel(perms: Permission[], key: string): string {
  const p = perms.find((x) => `${x.resource}:${x.action}` === key)
  return p?.description || key
}

function effectWords(r: RouteRule, perms: Permission[]) {
  if (r.effect === 'require_permission') return `Needs: ${permissionLabel(perms, r.permission)}`
  return EFFECTS.find((e) => e.value === r.effect)?.label ?? r.effect
}

function RuleDialog({ rule, perms, onSave, onClose }: {
  rule: RouteRule | null; perms: Permission[]; onSave: (r: RouteRule) => void; onClose: () => void
}) {
  const [path, setPath] = useState('')
  const [host, setHost] = useState('')
  const [forWhat, setFor] = useState<For>('any')
  const [effect, setEffect] = useState<RouteEffect>('require_auth')
  const [permission, setPermission] = useState<string | null>(null)
  useEffect(() => {
    if (!rule) return
    setPath(rule.path_pattern); setHost(rule.host_pattern); setFor(forOf(rule.methods))
    setEffect(rule.effect); setPermission(rule.permission || null)
  }, [rule])
  if (!rule) return null

  const forData = [
    { value: 'any', label: 'Everything' }, { value: 'view', label: 'Viewing' }, { value: 'change', label: 'Changes only' },
    ...(forOf(rule.methods) === 'as-is' ? [{ value: 'as-is', label: rule.methods.join(', ') }] : []),
  ]
  const ok = path.trim().startsWith('/') && (effect !== 'require_permission' || !!permission)
  const save = () => onSave({
    ...rule, path_pattern: path.trim(), host_pattern: host.trim(), effect,
    methods: forWhat === 'as-is' ? rule.methods : FOR_METHODS[forWhat],
    permission: effect === 'require_permission' ? permission ?? '' : '',
  })

  return (
    <Modal opened onClose={onClose} centered title={rule.path_pattern ? 'Change rule' : 'New rule'}>
      <div className="flex flex-col gap-3.5">
        <TextInput label="Address" value={path} autoFocus placeholder="/invoices/*/approve"
          description="* is one part of the address, ** is everything below. {id} is one part with a name."
          onChange={(e) => setPath(e.currentTarget.value)} />
        <div>
          <div className="t-body mb-1" style={{ fontWeight: 500 }}>For</div>
          <SegmentedControl fullWidth size="xs" value={forWhat} onChange={(v) => setFor(v as For)} data={forData} />
        </div>
        <Select label="Who gets through" data={EFFECTS} value={effect} allowDeselect={false}
          onChange={(v) => setEffect((v ?? 'require_auth') as RouteEffect)} />
        {effect === 'require_permission' && (
          <Select label="Permission" searchable value={permission} onChange={setPermission}
            nothingFoundMessage="This app has no permission by that name"
            data={perms.map((p) => ({ value: `${p.resource}:${p.action}`, label: p.description ? `${p.description} (${p.resource}:${p.action})` : `${p.resource}:${p.action}` }))} />
        )}
        <TextInput label="Only on this host" value={host} placeholder="any host"
          description="Leave empty unless the same app answers on several addresses."
          onChange={(e) => setHost(e.currentTarget.value)} />
        <div className="flex justify-end gap-2">
          <Button variant="default" size="xs" onClick={onClose}>Cancel</Button>
          <Button size="xs" disabled={!ok} onClick={save}>Keep</Button>
        </div>
      </div>
    </Modal>
  )
}

function TryAddress({ rules, perms }: { rules: RouteRule[]; perms: Permission[] }) {
  const [method, setMethod] = useState('GET')
  const [path, setPath] = useState('')
  const at = path.trim() ? firstMatch(rules, method, path.trim()) : -2
  const r = at >= 0 ? rules[at] : undefined
  return (
    <div className="panel-inset flex flex-col gap-2 p-3">
      <div className="flex flex-wrap items-center gap-2">
        <Select w={110} size="xs" value={method} allowDeselect={false} onChange={(v) => setMethod(v ?? 'GET')}
          data={['GET', 'POST', 'PUT', 'PATCH', 'DELETE']} aria-label="Kind of request" />
        <TextInput size="xs" className="min-w-0 flex-1" placeholder="/invoices/2024-118/approve" value={path}
          aria-label="Address to try" onChange={(e) => setPath(e.currentTarget.value)} />
      </div>
      {at === -1 && (
        <div className="t-sm" style={{ color: 'var(--deny)' }}>
          No rule matches, so Anubis turns it away. Add a rule if this address should open.
        </div>
      )}
      {r && (
        <div className="t-sm">
          Rule {at + 1} decides: <b>{effectWords(r, perms)}</b>
          <span className="t-xs"> ({r.path_pattern}, {methodWords(r.methods)})</span>
        </div>
      )}
    </div>
  )
}

export function PageRules({ app }: { app: live.AppRecord }) {
  const { data: saved, error } = useQuery({
    queryKey: ['route-rules', app.slug], queryFn: () => live.routeRules(app.slug), retry: false,
  })
  const { data: allPerms } = useQuery({ queryKey: qk.permissions(), queryFn: api.permissions })
  const perms = useMemo(() => (allPerms ?? []).filter((p) => p.app_slug === app.slug && !p.deprecated_at), [allPerms, app.slug])
  const [rules, setRules] = useState<RouteRule[] | null>(null)
  const [editing, setEditing] = useState<{ at: number; rule: RouteRule } | null>(null)
  const [trying, setTrying] = useState(false)
  const [report, setReport] = useState('')
  const [busy, setBusy] = useState(false)
  const list = rules ?? saved ?? []
  const dirty = rules !== null

  const change = (next: RouteRule[]) => { setRules(next); setReport('') }
  const move = (from: number, to: number) => {
    const next = [...list]
    const [r] = next.splice(from, 1)
    next.splice(to, 0, r!)
    change(next)
  }

  async function run(dry: boolean) {
    setBusy(true)
    try {
      const resp = await live.saveRouteRules(app.slug, list, dry)
      if (dry) setReport(resp.reportJson)
      else {
        notifyCreated('Page rules saved', 'The gate picks them up with its next snapshot.')
        setRules(null); setReport('')
        await queryClient.invalidateQueries({ queryKey: ['route-rules', app.slug] })
      }
    } catch (e) { notifyRejected(e) } finally { setBusy(false) }
  }

  if (error) return <div className="panel p-4 t-sm">Cannot read the page rules: {(error as Error).message}</div>

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
        <p className="t-sm" style={{ maxWidth: 620 }}>
          Anubis checks each address in {app.name} against these rules, top to bottom. The first rule that
          matches decides. An address no rule matches is turned away.
        </p>
        <div className="flex shrink-0 gap-2">
          <Button size="xs" variant="default" leftSection={<IconRoute size={14} />} onClick={() => setTrying((t) => !t)}>
            Try an address
          </Button>
          <Button size="xs" leftSection={<IconPlus size={14} />} onClick={() => setEditing({
            at: list.length,
            rule: { priority: 0, path_pattern: '', host_pattern: '', methods: [], effect: 'require_auth', permission: '', scope_bindings: {} },
          })}>
            Add a rule
          </Button>
        </div>
      </div>

      {trying && <TryAddress rules={list} perms={perms} />}

      {list.length === 0 && (
        <div className="panel px-4 py-6 text-center t-sm">
          No page rules yet. Until there are, a gate in front of {app.name} lets nothing through.
        </div>
      )}
      <SortableList items={list} keyOf={(r, i) => `${i}:${r.path_pattern}:${r.methods.join()}`} onReorder={move}>
        {(r, i, handle) => {
          const tone = EFFECT_TONE[r.effect]
          return (
            <div className="flex min-w-0 flex-1 flex-wrap items-center gap-x-3 gap-y-1 py-1">
              {handle}
              <span className="t-xs tnum w-5 text-right">{i + 1}</span>
              <span className="min-w-0 basis-[calc(100%-60px)] sm:basis-auto sm:flex-1">
                <span className="t-xs">When someone opens </span>
                <code className="t-body" style={{ fontWeight: 550, wordBreak: 'break-all' }}>{r.path_pattern}</code>
                {r.host_pattern && <span className="t-xs"> on {r.host_pattern}</span>}
              </span>
              <span className="t-xs ml-auto sm:ml-0">{methodWords(r.methods)}</span>
              <span className="t-xs rounded px-2 py-0.5" style={{ color: tone.fg, background: tone.bg, fontWeight: 550 }}>{effectWords(r, perms)}</span>
              <Menu position="bottom-end" width={200}>
                <Menu.Target>
                  <ActionIcon variant="subtle" color="gray" aria-label={`Actions for rule ${i + 1}`}><IconDots size={15} /></ActionIcon>
                </Menu.Target>
                <Menu.Dropdown>
                  <Menu.Item leftSection={<IconPencil size={14} />} onClick={() => setEditing({ at: i, rule: r })}>Change</Menu.Item>
                  <Menu.Item leftSection={<IconArrowUp size={14} />} disabled={i === 0} onClick={() => move(i, i - 1)}>Move up</Menu.Item>
                  <Menu.Item leftSection={<IconArrowDown size={14} />} disabled={i === list.length - 1} onClick={() => move(i, i + 1)}>Move down</Menu.Item>
                  <Menu.Divider />
                  <Menu.Item color="red" leftSection={<IconTrash size={14} />}
                    onClick={() => change(list.filter((_, j) => j !== i))}>Remove</Menu.Item>
                </Menu.Dropdown>
              </Menu>
            </div>
          )
        }}
      </SortableList>

      {dirty && (
        <div className="panel flex flex-col gap-2 p-3" style={{ borderColor: 'var(--accent)' }}>
          <div className="flex flex-wrap items-center gap-2">
            <span className="t-sm flex-1">You have changed the rules. Nothing is live until you save.</span>
            <Button size="xs" variant="subtle" color="gray" onClick={() => { setRules(null); setReport('') }}>Undo changes</Button>
            <Button size="xs" variant="default" loading={busy} onClick={() => void run(true)}>Check</Button>
            <Button size="xs" loading={busy} disabled={!report} onClick={() => void run(false)}>Save rules</Button>
          </div>
          {report && <Code block>{report}</Code>}
        </div>
      )}

      <RuleDialog rule={editing?.rule ?? null} perms={perms} onClose={() => setEditing(null)}
        onSave={(r) => {
          if (!editing) return
          const next = [...list]
          next.splice(editing.at, editing.at < list.length ? 1 : 0, r)
          change(next); setEditing(null)
        }} />
    </div>
  )
}
