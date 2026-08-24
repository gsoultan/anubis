import { createFileRoute } from '@tanstack/react-router'
import { Button, FileInput, Table } from '@mantine/core'
import { IconAlertTriangle, IconCheck, IconDownload, IconFileSpreadsheet, IconUpload } from '@tabler/icons-react'
import { useState } from 'react'
import { Page } from '@/components/shell/Page'
import { notifyCreated, notifyRejected } from '@/components/create/shell'
import { api } from '@/lib/anubis'

export const Route = createFileRoute('/import')({ component: ImportPage })

type Issue = { sheet: string; row: number; column: string; message: string }

type Report = {
  dry: boolean
  applied: boolean
  peopleCreated: number
  peopleExisting: number
  grantsCreated: number
  grantsSkipped: number
  membershipsAssigned: number
  membershipsExisting: number
  issues: Issue[]
  issuesOmitted: number
}

/* A file is identified by more than its name: an operator who fixes the
   spreadsheet and saves it keeps the name, and a clean check of the old
   contents must not unlock Apply for the new ones. */
const stamp = (f: File) => `${f.name}:${f.size}:${f.lastModified}`

function Counter({ label, value, muted }: { label: string; value: number; muted?: boolean }) {
  return (
    <div className="panel-inset px-3 py-2">
      <div className="t-h1" style={{ color: muted || value === 0 ? 'var(--ink-3)' : undefined }}>{value}</div>
      <div className="t-xs">{label}</div>
    </div>
  )
}

function ReportView({ report }: { report: Report }) {
  const clean = report.issues.length === 0 && report.issuesOmitted === 0
  return (
    <div className="panel mt-4 p-4">
      <div className="mb-3 flex items-center gap-2">
        {report.applied ? <IconCheck size={16} style={{ color: 'var(--gold)' }} />
          : clean ? <IconFileSpreadsheet size={16} /> : <IconAlertTriangle size={16} />}
        <span className="t-h1">
          {report.applied ? 'Imported'
            : clean ? 'Nothing wrong with this file'
              : 'This file has problems'}
        </span>
      </div>
      <p className="t-sm mb-3">
        {report.applied
          ? 'The workbook was applied in full.'
          : clean
            ? 'Nothing has been written yet. Apply the import to make these changes.'
            : 'Nothing has been written. An import applies in full or not at all, so fix the rows below and upload again.'}
      </p>

      <div className="grid grid-cols-2 gap-2 md:grid-cols-3">
        <Counter label={report.applied ? 'people created' : 'people to create'} value={report.peopleCreated} />
        <Counter label="people already there" value={report.peopleExisting} muted />
        <Counter label={report.applied ? 'roles granted' : 'roles to grant'} value={report.grantsCreated} />
        <Counter label="roles already held" value={report.grantsSkipped} muted />
        <Counter label={report.applied ? 'memberships added' : 'memberships to add'} value={report.membershipsAssigned} />
        <Counter label="already members" value={report.membershipsExisting} muted />
      </div>

      {report.issues.length > 0 && (
        <div className="mt-4">
          <div className="t-label mb-1.5">
            {report.issues.length} problem{report.issues.length === 1 ? '' : 's'}
            {report.issuesOmitted > 0 && ` (and ${report.issuesOmitted} more not shown)`}
          </div>
          <Table striped withTableBorder>
            <Table.Thead>
              <Table.Tr>
                <Table.Th>Sheet</Table.Th>
                <Table.Th>Row</Table.Th>
                <Table.Th>Column</Table.Th>
                <Table.Th>Problem</Table.Th>
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {report.issues.map((i, n) => (
                <Table.Tr key={`${i.sheet}-${i.row}-${i.column}-${n}`}>
                  <Table.Td>{i.sheet}</Table.Td>
                  {/* Row zero means the whole sheet is wrong, not line zero. */}
                  <Table.Td>{i.row > 0 ? i.row : '—'}</Table.Td>
                  <Table.Td>{i.column || '—'}</Table.Td>
                  <Table.Td>{i.message}</Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        </div>
      )}
    </div>
  )
}

function ImportPage() {
  const [file, setFile] = useState<File | null>(null)
  const [report, setReport] = useState<Report | null>(null)
  const [checked, setChecked] = useState<string | null>(null)
  const [busy, setBusy] = useState<'none' | 'template' | 'check' | 'apply'>('none')

  const download = async () => {
    setBusy('template')
    try {
      const resp = await api.provisioning.downloadImportTemplate({})
      const blob = new Blob([resp.workbook as BlobPart], { type: resp.contentType })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = resp.filename
      a.click()
      URL.revokeObjectURL(url)
    } catch (e) { notifyRejected(e) }
    setBusy('none')
  }

  const send = async (dry: boolean) => {
    if (!file) return
    setBusy(dry ? 'check' : 'apply')
    try {
      const workbook = new Uint8Array(await file.arrayBuffer())
      const resp = await api.provisioning.importWorkbook({ workbook, dry })
      const next: Report = {
        dry: resp.dry,
        applied: resp.applied,
        peopleCreated: resp.peopleCreated,
        peopleExisting: resp.peopleExisting,
        grantsCreated: resp.grantsCreated,
        grantsSkipped: resp.grantsSkipped,
        membershipsAssigned: resp.membershipsAssigned,
        membershipsExisting: resp.membershipsExisting,
        issues: resp.issues.map((i) => ({ sheet: i.sheet, row: i.row, column: i.column, message: i.message })),
        issuesOmitted: resp.issuesOmitted,
      }
      setReport(next)
      const clean = next.issues.length === 0 && next.issuesOmitted === 0
      /* Apply unlocks only for the exact file that checked out clean. */
      setChecked(dry && clean ? stamp(file) : null)
      if (next.applied) {
        notifyCreated('Import applied',
          `${next.peopleCreated} created, ${next.grantsCreated} granted, ${next.membershipsAssigned} added to memberships.`)
      }
    } catch (e) { notifyRejected(e) }
    setBusy('none')
  }

  const ready = file !== null && checked === stamp(file)

  return (
    <Page
      title="Import people and access"
      description="Create people and the access they hold from a spreadsheet. Download the template, fill it in, check it, then apply. Re-running the same file is safe — people who already exist are left alone."
      actions={
        <Button variant="default" leftSection={<IconDownload size={15} />}
          loading={busy === 'template'} onClick={download}>
          Download template
        </Button>
      }
    >
      <div className="panel p-4">
        <div className="t-label mb-1.5">Workbook</div>
        <FileInput
          placeholder="Choose an .xlsx file"
          accept=".xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
          leftSection={<IconFileSpreadsheet size={15} />}
          value={file}
          clearable
          onChange={(f) => { setFile(f); setReport(null); setChecked(null) }}
        />
        <div className="mt-3 flex items-center gap-2">
          <Button variant="default" leftSection={<IconUpload size={15} />}
            disabled={!file} loading={busy === 'check'} onClick={() => send(true)}>
            Check first
          </Button>
          <Button leftSection={<IconCheck size={15} />}
            disabled={!ready} loading={busy === 'apply'} onClick={() => send(false)}>
            Apply import
          </Button>
          {!ready && file && (
            <span className="t-xs">Check the file before applying it.</span>
          )}
        </div>
      </div>

      {report && <ReportView report={report} />}
    </Page>
  )
}
