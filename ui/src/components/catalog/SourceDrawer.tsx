import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Button, Code, Drawer, Modal, NumberInput, SegmentedControl, Switch, TextInput, Tooltip,
} from '@mantine/core'
import { IconPlayerPlay, IconTestPipe, IconTrash } from '@tabler/icons-react'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/client'
import { notifyCreated, notifyRejected } from '@/components/create/shell'
import { RunStatus, describeRun, when } from './runs'
import type { CatalogFormat, CatalogSource } from '@/lib/api/types'

/* One source: what it reads, how often, and everything it has done.
 *
 * The history is not decoration. A feed that has been failing for a week looks
 * exactly like a feed nobody has touched — the source row says "active" either
 * way — so the only thing that distinguishes them is the list of attempts and
 * the reason each one gave. That is also why a run is recorded before the
 * fetch and closed afterwards: a run stuck at `running` is a feed that hangs,
 * and it is the only way anyone finds out.
 */

type HTTPConfig = { url?: string; auth_header?: string }

function parseConfig(raw: string): HTTPConfig {
  try {
    const v = JSON.parse(raw) as unknown
    return typeof v === 'object' && v !== null ? (v as HTTPConfig) : {}
  } catch {
    return {}
  }
}

export function SourceDrawer({ source, onClose }: {
  source: CatalogSource | null
  onClose: () => void
}) {
  const [name, setName] = useState('')
  const [url, setUrl] = useState('')
  const [auth, setAuth] = useState('')
  const [format, setFormat] = useState<CatalogFormat>('json')
  const [scheduled, setScheduled] = useState(false)
  const [minutes, setMinutes] = useState(60)
  const [active, setActive] = useState(true)
  const [busy, setBusy] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState(false)

  useEffect(() => {
    if (!source) return
    const cfg = parseConfig(source.config_json)
    setName(source.name)
    setUrl(cfg.url ?? '')
    setAuth(cfg.auth_header ?? '')
    setFormat(source.format)
    setScheduled(source.interval_seconds > 0)
    setMinutes(source.interval_seconds > 0 ? Math.round(source.interval_seconds / 60) : 60)
    setActive(source.status === 'active')
  }, [source?.id])

  const { data: runs } = useQuery({
    queryKey: qk.catalogRuns(source?.id ?? ''),
    queryFn: () => api.catalogRuns(source?.id as string, 20),
    enabled: !!source,
  })

  async function refresh() {
    await queryClient.invalidateQueries({ queryKey: qk.catalogSources() })
    if (source) await queryClient.invalidateQueries({ queryKey: qk.catalogRuns(source.id) })
  }

  async function save() {
    if (!source) return
    setBusy(true)
    try {
      await api.updateCatalogSource({
        id: source.id,
        name,
        status: active ? 'active' : 'disabled',
        format,
        configJson: JSON.stringify({ url, ...(auth ? { auth_header: auth } : {}) }),
        // The server's floor is 300 seconds and it will say so; keeping the
        // input in minutes is what stops anybody typing 30 and meaning 30
        // minutes.
        intervalSeconds: scheduled ? minutes * 60 : 0,
      })
      await refresh()
      notifyCreated('Source saved', scheduled
        ? `Runs every ${minutes} minutes.`
        : 'Runs only when you ask it to.')
    } catch (e) { notifyRejected(e) } finally { setBusy(false) }
  }

  async function remove() {
    if (!source) return
    setBusy(true)
    try {
      await api.deleteCatalogSource(source.id)
      setConfirmDelete(false)
      onClose()
      await queryClient.invalidateQueries({ queryKey: qk.catalogSources() })
      notifyCreated(`"${source.name}" deleted`, 'Nothing it applied has changed.')
    } catch (e) { notifyRejected(e) } finally { setBusy(false) }
  }

  async function run(dry: boolean) {
    if (!source) return
    setBusy(true)
    try {
      const r = await api.runCatalogSource(source.id, dry)
      await refresh()
      if (r) notifyCreated(dry ? 'Dry run finished' : 'Run finished', describeRun(r))
    } catch (e) {
      // A failed run is recorded in the history whatever happens to this
      // request, which is the point of writing it before the fetch.
      await refresh()
      notifyRejected(e)
    } finally { setBusy(false) }
  }

  return (
    <Drawer
      opened={source !== null}
      onClose={onClose}
      position="right"
      size={640}
      overlayProps={{ blur: 2, backgroundOpacity: 0.45, color: 'var(--overlay-tint)' }}
      styles={{
        content: { background: 'var(--s-raised)' },
        header: { background: 'var(--s-raised)', borderBottom: '1px solid var(--line)', padding: '14px 20px' },
        title: { fontSize: 15, fontWeight: 640, letterSpacing: '-.01em' },
      }}
      title={source?.name ?? ''}
    >
      {source && (
        <div className="flex flex-col gap-5">
          <div className="panel-inset flex items-start gap-2.5 px-3.5 py-2.5">
            <div className="t-xs">
              Applied to <b style={{ color: 'var(--ink-2)' }}>{source.application_slug}</b> and
              nothing else. An application is chosen when a source is created and
              cannot be changed afterwards — the document is applied under that
              application&rsquo;s own id and slug, which is what keeps a bad feed
              inside one application.
            </div>
          </div>

          <div className="flex flex-col gap-3.5">
            <div className="t-label">Where it reads from</div>
            <TextInput label="Name" size="xs" value={name}
              onChange={(e) => setName(e.currentTarget.value)} />
            <TextInput
              label="URL" size="xs" value={url} placeholder="https://platform.internal/catalog.csv"
              onChange={(e) => setUrl(e.currentTarget.value)}
            />
            <TextInput
              label="Authorization header" size="xs" value={auth}
              placeholder="Bearer …"
              description="Sent as Authorization. Leave empty for a public URL."
              onChange={(e) => setAuth(e.currentTarget.value)}
            />
            <div>
              <div className="t-body mb-1.5" style={{ fontWeight: 550 }}>Format</div>
              <SegmentedControl
                fullWidth size="xs" value={format}
                onChange={(v) => setFormat(v as CatalogFormat)}
                data={[{ value: 'json', label: 'JSON manifest' }, { value: 'csv', label: 'CSV sheet' }]}
              />
              <div className="t-xs mt-1" style={{ opacity: 0.7 }}>
                A CSV carries one sheet — permissions or roles, decided by its
                header. Routes are JSON only.
              </div>
            </div>
          </div>

          <div className="flex flex-col gap-3">
            <div className="t-label">When it runs</div>
            <Switch
              size="xs" label="On a schedule"
              description="Off means it only runs when somebody presses Run."
              checked={scheduled}
              onChange={(e) => setScheduled(e.currentTarget.checked)}
            />
            {scheduled && (
              <NumberInput
                size="xs" label="Every" suffix=" minutes" min={5} max={10080}
                description="Five minutes is the floor — a catalog is a document a team edits, not a data feed."
                value={minutes}
                onChange={(v) => setMinutes(Math.max(5, Number(v) || 5))}
              />
            )}
            <Switch
              size="xs" label="Active"
              description="A disabled source is never run, by the scheduler or by hand."
              checked={active}
              onChange={(e) => setActive(e.currentTarget.checked)}
            />
            <div className="t-xs" style={{ opacity: 0.7 }}>
              {source.next_run_at
                ? <>Next run {when(source.next_run_at)}.</>
                : <>Not scheduled.</>}
              {source.last_run_at && <> Last ran {when(source.last_run_at)}.</>}
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <Button size="xs" loading={busy} onClick={() => void save()}>Save</Button>
            <Tooltip label="Fetches and validates without writing, and without moving the clock" withArrow>
              <Button
                variant="default" size="xs" loading={busy}
                leftSection={<IconTestPipe size={14} />}
                onClick={() => void run(true)}
              >
                Dry run
              </Button>
            </Tooltip>
            <Button
              variant="default" size="xs" loading={busy}
              leftSection={<IconPlayerPlay size={14} />}
              onClick={() => void run(false)}
            >
              Run now
            </Button>
            <Button
              variant="subtle" color="red" size="xs" className="ml-auto"
              leftSection={<IconTrash size={14} />}
              onClick={() => setConfirmDelete(true)}
            >
              Delete
            </Button>
          </div>

          <Modal
            opened={confirmDelete} onClose={() => setConfirmDelete(false)}
            title={`Delete "${source.name}"?`} centered size="sm"
          >
            <div className="t-body mb-4" style={{ opacity: 0.8 }}>
              Anubis stops reading this URL and the run history goes with it.
              Nothing it already applied changes — the permissions and roles
              stay exactly as they are, and what it did is still in the audit
              log. Disable it instead if you only want it to stop running.
            </div>
            <div className="flex justify-end gap-2">
              <Button variant="default" size="xs" onClick={() => setConfirmDelete(false)}>
                Cancel
              </Button>
              <Button color="red" size="xs" loading={busy} onClick={() => void remove()}>
                Delete
              </Button>
            </div>
          </Modal>

          <div className="flex flex-col gap-2">
            <div className="t-label">History</div>
            {(runs ?? []).length === 0 && (
              <div className="t-xs" style={{ opacity: 0.7 }}>
                Nothing yet. Every attempt lands here, including one that could
                not reach the URL.
              </div>
            )}
            {(runs ?? []).map((r) => (
              <div key={r.id} className="panel-inset flex flex-col gap-1 px-3 py-2">
                <div className="flex flex-wrap items-center gap-2">
                  <RunStatus status={r.status} />
                  <span className="t-xs">{when(r.started_at)}</span>
                  <span className="t-xs" style={{ opacity: 0.6 }}>
                    {r.actor === 'system' ? 'scheduled' : 'by hand'}
                  </span>
                  {r.document_sha && (
                    <Tooltip label="sha256 of the document that was fetched" withArrow>
                      <span className="chip tnum" style={{ cursor: 'help' }}>
                        {r.document_sha.slice(0, 8)}
                      </span>
                    </Tooltip>
                  )}
                </div>
                <div className="t-xs">{describeRun(r)}</div>
                {r.error && (
                  <div className="t-xs" style={{ color: 'var(--deny)' }}>{r.error}</div>
                )}
                {r.report_json && r.status !== 'skipped' && (
                  <Code block style={{ fontSize: 11, maxHeight: 160, overflow: 'auto' }}>
                    {r.report_json}
                  </Code>
                )}
              </div>
            ))}
          </div>
        </div>
      )}
    </Drawer>
  )
}
