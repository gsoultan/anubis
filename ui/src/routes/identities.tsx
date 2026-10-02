import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import {
  ActionIcon, Button, Menu, Popover, Select, TextInput, Tooltip, UnstyledButton,
} from '@mantine/core'
import {
  IconSearch, IconInfoCircle, IconDots, IconUserPlus, IconCirclePlus,
  IconUserOff, IconUserCheck, IconCopy, IconKey, IconLock, IconUser, IconX,
  IconTableImport, IconTestPipe,
} from '@tabler/icons-react'
import { notifications } from '@mantine/notifications'
import { queryClient } from '@/lib/query/client'
import { useCreate } from '@/stores/create'
import { useState } from 'react'
import { Page } from '@/components/shell/Page'
import { DataTable, type Column } from '@/components/ui/DataTable'
import { AttributesModal } from '@/components/ui/AttributesModal'
import { CredentialsModal } from '@/components/ui/CredentialsModal'
import { ConfirmModal } from '@/components/ui/ConfirmModal'
import { Initial } from '@/components/ui/Initial'
import { notifyRejected } from '@/components/create/shell'
import { api } from '@/lib/api/client'
import * as live from '@/lib/api/live'
import { realmKindColor } from '@/lib/realmKind'
import { fmtDate, relDays } from '@/lib/access'
import { qk } from '@/lib/query/keys'
import { usePeopleList, useSession } from '@/stores/session'
import type { Ial, Identity, IdentityStatus } from '@/lib/api/types'

export const Route = createFileRoute('/identities')({ component: Identities })

const IAL_HINT: Record<Ial, string> = {
  1: 'IAL1 · Self-asserted — email only, a self-registered applicant.',
  2: 'IAL2 · Remotely verified, typically through a contract or employer.',
  3: 'IAL3 · In-person verified with government ID on file.',
}

/* Active is what 99 rows in 100 are, so active is the quiet one. The pill is
   spent on the exception, which is the only row anyone is scanning for. */
function Status({ status }: { status: IdentityStatus }) {
  if (status === 'active') {
    return (
      <span className="t-body inline-flex items-center gap-1.5" style={{ color: 'var(--ink-2)' }}>
        <span style={{ width: 6, height: 6, borderRadius: 99, background: 'var(--allow)', flexShrink: 0 }} />
        Active
      </span>
    )
  }
  return (
    <Tooltip label="None of their access applies while the account is not active.">
      <span className="v-pill v-pill-deny" style={{ cursor: 'help' }}>{status}</span>
    </Tooltip>
  )
}

/* A lesson you need once, one click away, instead of a banner charging rent
   above the table on every visit. */
function SameNameNote() {
  return (
    <Popover width={330} position="bottom-start" withArrow shadow="xl">
      <Popover.Target>
        <UnstyledButton
          className="t-xs mt-2 inline-flex items-center gap-1.5"
          style={{ color: 'var(--ink-3)' }}
          aria-label="Why the same name can appear more than once"
        >
          <IconInfoCircle size={13} />
          <span className="dotted">Why one name can appear several times</span>
        </UnstyledButton>
      </Popover.Target>
      <Popover.Dropdown p="sm">
        <div className="t-xs">
          <b style={{ color: 'var(--ink-2)' }}>alice</b> can exist once in every population.
          Those are different people: linking them is explicit, and it never merges access.
        </div>
      </Popover.Dropdown>
    </Popover>
  )
}

function Identities() {
  const { realmFilter, setRealmFilter } = useSession()
  const { openCreate } = useCreate()
  const navigate = useNavigate()

  /** A row is a person, and a person has a page — which is where access lives. */
  const openPerson = (i: Identity, give = false) =>
    void navigate({
      to: '/identities/$id', params: { id: i.id }, search: give ? { give: true } : {},
    })

  /* Rejects after saying why, so a confirmation dialog can stay open on
     failure; one-click callers swallow the rejection they have already seen. */
  async function setStatus(i: Identity, next: 'active' | 'disabled') {
    try {
      await api.setIdentityStatus(i.id, next)
    } catch (e) {
      notifyRejected(e)
      throw e
    }
    notifications.show({
      color: next === 'disabled' ? 'orange' : 'teal',
      title: next === 'disabled' ? `${i.username} disabled` : `${i.username} re-enabled`,
      message: next === 'disabled'
        ? 'Their sessions ended, and none of their access applies until re-enabled.'
        : 'Their access applies again immediately.',
    })
    await queryClient.invalidateQueries({ queryKey: ['identities'] })
  }
  /* Held here rather than in a route param: these are the encrypted fields,
     and an id in the URL is an id in someone's browser history. */
  const [attrsFor, setAttrsFor] = useState<Identity | null>(null)
  const [credsFor, setCredsFor] = useState<Identity | null>(null)
  const [disabling, setDisabling] = useState<Identity | null>(null)
  const { data: realms } = useQuery({ queryKey: qk.realms(), queryFn: api.realms })
  /* Search and paging live in a store: every row leads off this screen, and
     losing the search that found somebody the moment you open them is not
     paging, it is starting again. */
  const { query: q, setQuery: setQ, trail, setTrail, resetPaging } = usePeopleList()
  /* Keyset paging, and it is not optional here: a population in this
     installation holds fifty thousand people. A stack of cursors rather than
     a page number, because keyset paging can step but cannot jump. */
  const cursor = trail[trail.length - 1] ?? ''
  const { data: page, isFetching } = useQuery({
    /* Under the 'identities' prefix on purpose: disabling somebody invalidates
       ['identities'], and a key outside it kept saying "active" until a reload. */
    queryKey: ['identities', 'page', realmFilter, q, cursor],
    queryFn: () => live.identitiesPage(realmFilter ?? undefined, q || undefined, cursor, 50),
    placeholderData: (prev) => prev,
  })
  const rows = page?.rows
  const { data: categories } = useQuery({
    queryKey: qk.realmCategories(), queryFn: () => api.realmCategories(),
  })

  const realmOf = (id: string) => realms?.find((r) => r.id === id)
  const filtered = q.trim() !== '' || realmFilter !== null
  function clearFilters() {
    setQ('')
    setRealmFilter(null)
    resetPaging()
  }

  /* Display names are not unique — this installation runs three populations
     all called "Enrolment probe" — so the code rides along in the option. */
  const realmOptions = [
    { value: '', label: 'All populations' },
    ...(realms ?? []).map((r) => ({ value: r.id, label: `${r.display_name} · ${r.code}` })),
  ]

  /* The code earns its place in the column when it tells two populations
     apart, or says something the name does not. "Internal" over "internal"
     is the same word twice on every row. */
  const ambiguous = new Set(
    (realms ?? []).map((r) => r.display_name).filter((n, idx, all) => all.indexOf(n) !== idx),
  )
  function populationCode(code: string, name: string): string | undefined {
    if (ambiguous.has(name)) return code
    const a = code.toLowerCase().replace(/[^a-z0-9]/g, '')
    const b = name.toLowerCase().replace(/[^a-z0-9]/g, '')
    return a.startsWith(b) || b.startsWith(a) ? undefined : code
  }

  /* Every column has a width, so slack on a wide display is shared out
     instead of pooling in one gap. The person column is the widest because it
     is the only one that differs on every row. */
  const columns: Column<Identity>[] = [
    { key: 'person', header: 'Person', width: 360, render: (i) => {
        /* Code AND realm: a category code is unique inside a realm, not
           across the tenant. */
        const c = categories?.find((x) => x.code === i.category && x.realm_id === i.realm_id)
        const sub = [i.email, c?.display_name ?? i.category].filter(Boolean).join(' · ')
        /* A neutral avatar. Tinted by population it repeated the column
           beside it; neutral, it only gives the eye a row to land on. */
        return (
          <div className="flex min-w-0 items-center gap-2.5">
            <Initial name={i.username} colour="var(--ink-3)" size={26} />
            <div className="flex min-w-0 items-baseline gap-2">
              <span className="row-title t-body truncate" style={{ fontWeight: 560, flex: '0 1 auto' }}>
                {i.username}
              </span>
              <span className="t-xs truncate" style={{ flex: '1 1 auto' }}>
                {sub || <span style={{ opacity: 0.6 }}>no email</span>}
              </span>
            </div>
          </div>
        )
      } },
    /* The population repeats down the column — one tenant's list is mostly
       one population — so it is set quiet. */
    { key: 'population', header: 'Population', width: 190, render: (i) => {
        const r = realmOf(i.realm_id)
        if (!r) return <span className="t-xs">—</span>
        const code = populationCode(r.code, r.display_name)
        return (
          <span className="flex min-w-0 items-center gap-2">
            <span style={{ width: 6, height: 6, borderRadius: 99, flexShrink: 0,
              background: realmKindColor(r.kind) }} />
            <span className="t-body truncate" style={{ color: 'var(--ink-2)' }}>{r.display_name}</span>
            {code && <span className="t-xs truncate">{code}</span>}
          </span>
        )
      } },
    { key: 'status', header: 'Status', width: 120, render: (i) => <Status status={i.status} /> },
    /* Neutral: an assurance level is a fact about how someone was verified,
       not a verdict. Painting IAL1 amber put a warning on every applicant. */
    { key: 'ial', header: 'Assurance', width: 100, render: (i) => (
        <Tooltip label={IAL_HINT[i.assurance_level]}>
          <span className="chip" style={{ cursor: 'help' }}>IAL{i.assurance_level}</span>
        </Tooltip>
      ) },
    { key: 'seen', header: 'Last sign-in', width: 140,
      headerHint: 'An account nobody has ever used is the one worth asking about.',
      render: (i) =>
        i.last_login_at
          ? (
            <Tooltip label={fmtDate(i.last_login_at)}>
              <span className="t-body tnum" style={{ color: 'var(--ink-2)' }}>{relDays(i.last_login_at)}</span>
            </Tooltip>
          )
          : <span className="t-xs">Never</span> },
    /* A dash, explained once in the header, rather than "no statutory limit"
       spelled out on nearly every employee's row. */
    { key: 'retention', header: 'Retention', width: 120,
      headerHint: 'When the population sets a statutory retention limit, the deadline shows here. A dash means no limit.',
      render: (i) =>
        i.retention_until
          ? <span className="t-body tnum">{fmtDate(i.retention_until)}</span>
          : <span className="t-xs" style={{ opacity: 0.55 }}>—</span> },
    { key: 'actions', header: '', width: 84, render: (i) => (
        /* The row navigates, so the controls keep their clicks to themselves
           or every pick would also leave the page behind it. */
        <div className="flex items-center justify-end gap-0.5"
          onClick={(e) => e.stopPropagation()} onKeyDown={(e) => e.stopPropagation()}>
          {/* The one thing people come to this list to do, one click from
              every row — shown on hover so fifty rows do not each carry it. */}
          <Tooltip label="Give access" openDelay={200}>
            <ActionIcon className="hover-reveal" variant="subtle" color="gray"
              aria-label={`Give ${i.username} access`} onClick={() => openPerson(i, true)}>
              <IconCirclePlus size={16} />
            </ActionIcon>
          </Tooltip>
          <Menu position="bottom-end" width={230}>
            <Menu.Target>
              <ActionIcon variant="subtle" color="gray" aria-label={`Actions for ${i.username}`}>
                <IconDots size={15} />
              </ActionIcon>
            </Menu.Target>
            <Menu.Dropdown>
              <Menu.Item leftSection={<IconUser size={14} />} onClick={() => openPerson(i)}>
                Open
              </Menu.Item>
              <Menu.Item leftSection={<IconCirclePlus size={14} />} onClick={() => openPerson(i, true)}>
                Give access…
              </Menu.Item>
              <Menu.Item leftSection={<IconTestPipe size={14} />}
                onClick={() => void navigate({ to: '/playground', search: { subject: i.id } })}>
                Test access
              </Menu.Item>
              <Menu.Divider />
              <Menu.Item leftSection={<IconKey size={14} />} onClick={() => setCredsFor(i)}>
                Sign-in methods…
              </Menu.Item>
              <Menu.Item leftSection={<IconLock size={14} />} onClick={() => setAttrsFor(i)}>
                Encrypted attributes…
              </Menu.Item>
              <Menu.Item leftSection={<IconCopy size={14} />}
                onClick={() => { void navigator.clipboard.writeText(i.id) }}>
                Copy ID
              </Menu.Item>
              <Menu.Divider />
              {i.status === 'active' ? (
                <Menu.Item color="deny" leftSection={<IconUserOff size={14} />}
                  onClick={() => setDisabling(i)}>
                  Disable…
                </Menu.Item>
              ) : (
                <Menu.Item leftSection={<IconUserCheck size={14} />}
                  onClick={() => void setStatus(i, 'active').catch(() => {})}>
                  Re-enable
                </Menu.Item>
              )}
            </Menu.Dropdown>
          </Menu>
        </div>
      ) },
  ]

  /* Filters sit on the table they filter, not in the page header a hand's
     width from the global ⌘K box. */
  const toolbar = (
    <>
      <TextInput
        size="xs"
        w={260}
        placeholder="Search username or email"
        leftSection={<IconSearch size={14} />}
        value={q}
        onChange={(e) => setQ(e.currentTarget.value)}
        aria-label="Search people"
      />
      <Select
        size="xs"
        w={230}
        searchable
        allowDeselect={false}
        data={realmOptions}
        value={realmFilter ?? ''}
        onChange={(v) => { setRealmFilter(v || null); resetPaging() }}
        comboboxProps={{ width: 280, position: 'bottom-start' }}
        aria-label="Population"
        renderOption={({ option }) => {
          const r = realms?.find((x) => x.id === option.value)
          return (
            <span className="flex min-w-0 items-center gap-2">
              <span style={{
                width: 6, height: 6, borderRadius: 99, flexShrink: 0,
                background: option.value ? realmKindColor(r?.kind) : 'var(--ink-4)',
              }} />
              <span className="truncate">{option.label}</span>
            </span>
          )
        }}
      />
      {filtered && (
        <Button size="compact-xs" variant="subtle" color="gray"
          leftSection={<IconX size={12} />} onClick={clearFilters}>
          Clear
        </Button>
      )}
      <span className="t-xs tnum ml-auto">
        {isFetching ? 'Loading…' : `${rows?.length ?? 0} shown`}
      </span>
    </>
  )

  /* Rendered whether or not there is a next page, so a list that fits on one
     page still reports its size somewhere. */
  const footer = (
    <>
      <span className="t-xs tnum">
        {rows?.length ?? 0} on this page
        {trail.length > 1 && ` · page ${trail.length}`}
      </span>
      <div className="ml-auto flex items-center gap-2">
        <Button variant="default" size="compact-sm" disabled={trail.length <= 1}
          onClick={() => setTrail((t) => t.slice(0, -1))}>Previous</Button>
        <Button variant="default" size="compact-sm" disabled={!page?.next}
          onClick={() => setTrail((t) => [...t, page?.next ?? ''])}>Next</Button>
      </div>
    </>
  )

  return (
    <Page
      title="People"
      description={
        <>
          Everyone who can sign in — employees, supplier contacts, applicants. Open a person to see
          what they can do, and to give or take away access.
          <br />
          <SameNameNote />
        </>
      }
      wide
      actions={
        <>
          <Button size="xs" variant="default" leftSection={<IconTableImport size={14} />}
            onClick={() => void navigate({ to: '/import' })}>
            Import
          </Button>
          <Button size="xs" leftSection={<IconUserPlus size={14} />}
            onClick={() => openCreate('identity')}>
            Add person
          </Button>
        </>
      }
    >
      <DataTable
        columns={columns}
        rows={rows}
        rowKey={(i) => i.id}
        toolbar={toolbar}
        footer={footer}
        stale={isFetching && rows !== undefined}
        onRowClick={(i) => openPerson(i)}
        empty={{
          title: filtered ? 'No people match' : 'No people yet',
          hint: filtered
            ? 'Try a different population, or clear the search.'
            : 'Everyone who can sign in lives here, one row per population.',
          action: filtered
            ? <Button size="xs" variant="light" onClick={clearFilters}>Clear filters</Button>
            : <Button size="xs" variant="light" onClick={() => openCreate('identity')}>Add person</Button>,
        }}
      />

      <AttributesModal id={attrsFor?.id ?? null}
        label={attrsFor?.username ?? ''} onClose={() => setAttrsFor(null)} />
      <CredentialsModal id={credsFor?.id ?? null}
        label={credsFor?.username ?? ''} onClose={() => setCredsFor(null)} />
      <ConfirmModal opened={!!disabling} onClose={() => setDisabling(null)}
        title={`Disable ${disabling?.username ?? ''}?`} confirmLabel="Disable"
        onConfirm={() => (disabling ? setStatus(disabling, 'disabled') : Promise.resolve())}>
        <p>
          Every session they have ends now and the tokens already issued to them stop working. They
          cannot sign in, and none of their access applies while disabled.
        </p>
        <p className="t-xs">Their grants are kept: re-enabling gives their access back without re-granting anything.</p>
      </ConfirmModal>
    </Page>
  )
}
