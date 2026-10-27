import { Button, Code, FileInput, Modal, SegmentedControl, Textarea } from '@mantine/core'
import { IconFileCode } from '@tabler/icons-react'
import { useState } from 'react'
import { notifyCreated, notifyRejected } from '@/components/create/shell'
import { queryClient } from '@/lib/query/client'
import * as live from '@/lib/api/live'
import type { CatalogFormat } from '@/lib/api/types'

/* The catalog by hand: paste a manifest, or drop in the CSV the platform team
   actually maintains. The scheduled version of the same thing — a URL Anubis
   reads on a clock — is the Catalog sync screen. */
export function ManifestDialog({ slug, onDone }: { slug: string; onDone: () => void }) {
  const [open, setOpen] = useState(false)
  const [json, setJson] = useState('')
  const [format, setFormat] = useState<CatalogFormat>('json')
  const [report, setReport] = useState('')
  const [busy, setBusy] = useState(false)

  /* Read the file here rather than posting it: the document goes over the
     same RPC either way, and holding the text means it is editable and
     dry-runnable before anything is applied. */
  async function load(file: File | null) {
    if (!file) return
    try {
      const text = await file.text()
      setJson(text)
      setReport('')
      setFormat(/\.csv$/i.test(file.name) ? 'csv' : 'json')
    } catch (e) { notifyRejected(e) }
  }

  const run = async (dry: boolean) => {
    setBusy(true)
    try {
      const resp = await live.applyManifest(slug, json, dry, format)
      setReport(resp.reportJson)
      if (!dry) {
        notifyCreated('Manifest applied', `${slug} is now at version ${resp.manifestVersion}.`)
        await Promise.all(['applications', 'permissions', 'roles', 'route-rules'].map((k) =>
          queryClient.invalidateQueries({ queryKey: [k] })))
        onDone()
      }
    } catch (e) { notifyRejected(e) }
    setBusy(false)
  }

  return (
    <>
      <Button variant="default" size="compact-xs" leftSection={<IconFileCode size={13} />}
        onClick={() => setOpen(true)}>
        Manifest
      </Button>
      <Modal opened={open} onClose={() => setOpen(false)} title={`Manifest — ${slug}`} centered size="lg">
        <p className="t-sm mb-3">
          What this application declares: the permissions it defines, the roles
          that bundle them, and the routes a gateway should protect. Only the
          sections the document carries are touched, and anything it stops
          naming is retired rather than deleted — existing access keeps
          working. Check first; the report says what would change.
        </p>
        <div className="mb-3 flex flex-wrap items-end gap-3">
          <div>
            <div className="t-body mb-1.5" style={{ fontWeight: 550 }}>Format</div>
            <SegmentedControl
              size="xs" value={format}
              onChange={(v) => { setFormat(v as CatalogFormat); setReport('') }}
              data={[{ value: 'json', label: 'JSON manifest' }, { value: 'csv', label: 'CSV sheet' }]}
            />
          </div>
          <FileInput
            size="xs" w={260} clearable accept=".json,.csv,text/csv,application/json"
            label="…or load a file"
            placeholder="permissions.csv"
            onChange={(f) => void load(f)}
          />
        </div>
        {format === 'csv' && (
          <div className="t-xs mb-2" style={{ opacity: 0.75 }}>
            One sheet per file, decided by its header: <b>resource, action, …</b> for
            permissions, <b>role, permissions, …</b> for roles. A roles sheet may
            name permissions a previous document installed. Routes are JSON only.
          </div>
        )}
        <Textarea autosize minRows={8} maxRows={18} styles={{ input: { fontFamily: 'monospace' } }}
          placeholder={format === 'csv'
            ? 'resource,action,description,risk\ninvoice,read,Read invoices,normal'
            : '{"permissions":[{"resource":"invoice","action":"read","risk":"normal"}]}'}
          value={json} onChange={(e) => { setJson(e.currentTarget.value); setReport('') }} />
        <div className="mt-3 flex items-center gap-2">
          <Button variant="default" loading={busy} disabled={!json.trim()}
            onClick={() => void run(true)}>Check</Button>
          <Button loading={busy} disabled={!report}
            onClick={() => void run(false)}>Apply</Button>
          {!report && <span className="t-xs">Check before applying.</span>}
        </div>
        {report && (
          <div className="mt-3">
            <div className="t-label mb-1">What this would do</div>
            <Code block>{report}</Code>
          </div>
        )}
      </Modal>
    </>
  )
}
