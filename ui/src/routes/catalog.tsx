import { createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import {
  Button, Modal, NumberInput, SegmentedControl, Select, Switch, TextInput, Tooltip,
} from '@mantine/core'
import { IconPlus, IconRefresh } from '@tabler/icons-react'
import { Page } from '@/components/shell/Page'
import { DataTable, Cell, type Column } from '@/components/ui/DataTable'
import { SourceDrawer } from '@/components/catalog/SourceDrawer'
import { RunStatus, schedule, when } from '@/components/catalog/runs'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/client'
import { notifyCreated, notifyRejected } from '@/components/create/shell'
import type { CatalogFormat, CatalogSource } from '@/lib/api/types'

export const Route = createFileRoute('/catalog')({ component: Catalog })

/* The fourth way a catalog arrives.
 *
 * The other three go through a request somebody makes: an application pushes a
 * manifest, an operator pastes one on the Applications screen, an operator
 * uploads a CSV there. This screen is for the case where the catalog is
 * maintained somewhere else entirely — an ERP export, a file a platform team
 * publishes — and Anubis should go and read it rather than wait for anyone to
 * remember.
 *
 * What the list is FOR is spotting the feed that stopped working. A source
 * that failed an hour ago and one that applied an hour ago are identical if
 * all you show is when they last ran, so the status is on the row.
 */

function Catalog() {
  const { data: sources, isFetching } = useQuery({
    queryKey: qk.catalogSources(), queryFn: api.catalogSources,
  })
  const [creating, setCreating] = useState(false)
  const [open, setOpen] = useState<CatalogSource | null>(null)

  /* The drawer edits the row it was opened on, so it has to follow the list
     rather than hold a copy: saving inside it would otherwise leave the
     drawer showing what the source used to be. */
  const selected = open ? sources?.find((s) => s.id === open.id) ?? open : null

  const columns: Column<CatalogSource>[] = [
    { key: 'name', header: 'Source', width: 300, render: (s) => (
        <Cell top={s.name} bottom={`applies to ${s.application_slug}`} />
      ) },
    { key: 'format', header: 'Document', width: 120, render: (s) => (
        <span className="chip">{s.format === 'csv' ? 'CSV' : 'JSON'}</span>
      ) },
    { key: 'schedule', header: 'Runs', width: 130,
      headerHint: 'A manual source is only ever run from this screen. The scheduler ignores it.',
      render: (s) => (
        <span className="t-body">{s.status === 'disabled' ? 'disabled' : schedule(s.interval_seconds)}</span>
      ) },
    { key: 'last', header: 'Last run', width: 170,
      /* last_run_at is when the source last RAN on its clock, and a dry run
         deliberately does not move it — so the time only appears when there
         is one. Saying "dry run / never" in the same cell was a row arguing
         with itself. */
      headerHint: 'A dry run shows here too, but does not move the schedule.',
      render: (s) =>
        s.last_status === '' ? (
          <span className="t-xs" style={{ opacity: 0.6 }}>never run</span>
        ) : (
          <div className="flex flex-col gap-0.5">
            <RunStatus status={s.last_status} />
            {s.last_run_at && (
              <span className="t-xs" style={{ opacity: 0.65 }}>{when(s.last_run_at)}</span>
            )}
          </div>
        ) },
    { key: 'next', header: 'Next', width: 130, render: (s) => {
        if (!s.next_run_at) return <span className="t-xs" style={{ opacity: 0.45 }}>—</span>
        // A due time in the past is not "1 min ago", it is waiting for the
        // next tick — the scheduler runs every minute.
        const due = new Date(s.next_run_at).getTime() <= Date.now()
        return due
          ? <span className="t-body" style={{ color: 'var(--ink-2)' }}>due now</span>
          : <span className="t-body">{when(s.next_run_at)}</span>
      } },
  ]

  const failing = (sources ?? []).filter((s) => s.last_status === 'failed').length

  const toolbar = (
    <>
      <Button size="xs" leftSection={<IconPlus size={14} />} onClick={() => setCreating(true)}>
        New source
      </Button>
      <Tooltip label="Re-read the list" withArrow>
        <Button
          variant="default" size="compact-xs" leftSection={<IconRefresh size={13} />}
          onClick={() => void queryClient.invalidateQueries({ queryKey: qk.catalogSources() })}
        >
          Refresh
        </Button>
      </Tooltip>
      {failing > 0 && (
        <span className="t-xs" style={{ color: 'var(--deny)' }}>
          {failing === 1 ? 'one source is failing' : `${failing} sources are failing`}
        </span>
      )}
      <span className="t-xs tnum ml-auto">
        {isFetching ? 'loading…' : `${sources?.length ?? 0} shown`}
      </span>
    </>
  )

  return (
    <Page
      title="Catalog sync"
      description={
        <>
          Where an application&rsquo;s permissions and roles are read from when
          nobody is pushing them. Each source is pinned to one application and
          applies under that application alone; a document it stops naming is
          retired, never deleted, so existing access keeps working.
        </>
      }
      wide
      actions={
        <Button size="xs" leftSection={<IconPlus size={14} />} onClick={() => setCreating(true)}>
          New source
        </Button>
      }
    >
      <NewSourceModal
        opened={creating}
        onClose={() => setCreating(false)}
        onCreated={async (s) => {
          setCreating(false)
          await queryClient.invalidateQueries({ queryKey: qk.catalogSources() })
          setOpen(s)
        }}
      />

      <DataTable
        columns={columns}
        rows={sources}
        rowKey={(s) => s.id}
        toolbar={toolbar}
        onRowClick={setOpen}
        empty={{
          title: 'No catalog sources',
          hint: 'Add one when an application’s permissions and roles are maintained somewhere else and should be read on a clock.',
          action: <Button size="xs" variant="light" onClick={() => setCreating(true)}>New source</Button>,
        }}
      />

      <SourceDrawer source={selected} onClose={() => setOpen(null)} />
    </Page>
  )
}

/* Creating one asks the question that cannot be changed later first: which
   application does this write to? Everything else is editable afterwards. */
function NewSourceModal({ opened, onClose, onCreated }: {
  opened: boolean
  onClose: () => void
  onCreated: (s: CatalogSource) => void | Promise<void>
}) {
  const { data: apps } = useQuery({ queryKey: ['app-choices'], queryFn: () => api.applications() })
  const [slug, setSlug] = useState<string | null>(null)
  const [name, setName] = useState('')
  const [url, setUrl] = useState('')
  const [auth, setAuth] = useState('')
  const [format, setFormat] = useState<CatalogFormat>('json')
  const [scheduled, setScheduled] = useState(true)
  const [minutes, setMinutes] = useState(60)
  const [busy, setBusy] = useState(false)

  async function create() {
    if (!slug) return
    setBusy(true)
    try {
      const s = await api.createCatalogSource({
        applicationSlug: slug,
        name: name || `${slug} catalog`,
        format,
        configJson: JSON.stringify({ url, ...(auth ? { auth_header: auth } : {}) }),
        intervalSeconds: scheduled ? minutes * 60 : 0,
      })
      if (s) {
        notifyCreated(`"${s.name}" created`,
          scheduled
            ? 'Anubis reads it on a clock. Dry-run it first from the drawer.'
            : 'It runs only when you ask it to.')
        await onCreated(s)
      }
    } catch (e) { notifyRejected(e) } finally { setBusy(false) }
  }

  return (
    <Modal opened={opened} onClose={onClose} centered title="New catalog source" size="lg">
      <div className="flex flex-col gap-3.5">
        <Select
          label="Application" size="xs" searchable value={slug} onChange={setSlug}
          description="Fixed once created: the document is applied under this application's id and slug, and that pin is what keeps a bad feed inside one application."
          data={(apps ?? []).map((a) => ({ value: a.slug, label: `${a.name} (${a.slug})` }))}
        />
        <TextInput
          label="Name" size="xs" value={name} placeholder="Billing catalog"
          description="For you, in the list."
          onChange={(e) => setName(e.currentTarget.value)}
        />
        <TextInput
          label="URL" size="xs" value={url}
          placeholder="https://platform.internal/catalog/billing.csv"
          onChange={(e) => setUrl(e.currentTarget.value)}
        />
        <TextInput
          label="Authorization header" size="xs" value={auth} placeholder="Bearer …"
          description="Optional. Sent as Authorization."
          onChange={(e) => setAuth(e.currentTarget.value)}
        />
        <div>
          <div className="t-body mb-1.5" style={{ fontWeight: 550 }}>Format</div>
          <SegmentedControl
            fullWidth size="xs" value={format}
            onChange={(v) => setFormat(v as CatalogFormat)}
            data={[{ value: 'json', label: 'JSON manifest' }, { value: 'csv', label: 'CSV sheet' }]}
          />
        </div>
        <Switch
          size="xs" label="Run on a schedule"
          description="Off creates it as manual — useful while you are still checking the URL."
          checked={scheduled}
          onChange={(e) => setScheduled(e.currentTarget.checked)}
        />
        {scheduled && (
          <NumberInput
            size="xs" label="Every" suffix=" minutes" min={5} max={10080}
            value={minutes}
            onChange={(v) => setMinutes(Math.max(5, Number(v) || 5))}
          />
        )}
        <div className="flex justify-end gap-2">
          <Button variant="default" size="xs" onClick={onClose}>Cancel</Button>
          <Button size="xs" loading={busy} disabled={!slug || !url} onClick={() => void create()}>
            Create
          </Button>
        </div>
      </div>
    </Modal>
  )
}
