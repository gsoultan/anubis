import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import {
  ActionIcon, Button, Menu, Popover, Select, TextInput, Tooltip, UnstyledButton,
} from '@mantine/core'
import {
  IconSearch, IconInfoCircle, IconDots, IconUserPlus, IconCirclePlus,
  IconUserOff, IconUserCheck, IconCopy, IconKey, IconLock, IconUser, IconX,
  IconChevronRight,
} from '@tabler/icons-react'
import { notifications } from '@mantine/notifications'
import { queryClient } from '@/lib/query/client'
import { useCreate } from '@/stores/create'
import { useState } from 'react'
import { Page } from '@/components/shell/Page'
import { DataTable, Cell, type Column } from '@/components/ui/DataTable'
import { AttributesModal } from '@/components/ui/AttributesModal'
import { CredentialsModal } from '@/components/ui/CredentialsModal'
import { Initial } from '@/components/ui/Initial'
import { api } from '@/lib/api/client'
import * as live from '@/lib/api/live'
import { realmKindColor } from '@/lib/realmKind'
import { qk } from '@/lib/query/keys'
import { usePeopleList, useSession } from '@/stores/session'
import type { Ial, Identity, IdentityStatus } from '@/lib/api/types'

export const Route = createFileRoute('/identities')({ component: Identities })

const IAL_HINT: Record<Ial, string> = {
  1: 'Self-asserted — email only, a self-registered applicant.',
  2: 'Remotely verified, typically through a contract or employer.',
  3: 'In-person verified with government ID on file.',
}
const IAL_COLOR: Record<Ial, string> = {
  1: 'var(--warn)', 2: 'var(--info)', 3: 'var(--allow)',
}

/* The leftmost thing in a row is what the eye lands on, so it carries two
   facts at once: who (the initial) and which population (the tint). That is
   also why the population column no longer needs a coloured dot of its own —
   the same information was being drawn twice, six columns apart.
   `Initial` itself lives in components/ui now: a person's own page draws the
   same avatar, and two copies is two colour rules. */

/* Active is what 99 rows in 100 are, so active is the quiet one. The pill is
   spent on the exception, which is the only row anyone is scanning for. The
   old screen did the reverse and painted a grey chip on every line. */
function Status({ status }: { status: IdentityStatus }) {
  if (status === 'active') {
    return (
      <span className="t-body inline-flex items-center gap-1.5" style={{ color: 'var(--ink-2)' }}>
        <span style={{ width: 6, height: 6, borderRadius: 99, background: 'var(--allow)', flexShrink: 0 }} />
        active
      </span>
    )
  }
  return (
    <Tooltip label="authorize() gates on identity state, so this is denied regardless of grants." withArrow>
      <span className="v-pill v-pill-deny" style={{ cursor: 'help' }}>{status}</span>
    </Tooltip>
  )
}

/* This used to be a banner pinned above the table on every visit — a lesson
   you need once, charging rent forever. One click away, permanently, costs no
   layout at all. */
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
          <span style={{ borderBottom: '1px dotted var(--line-strong)' }}>
            Why one name can appear several times
          </span>
        </UnstyledButton>
      </Popover.Target>
      <Popover.Dropdown p="sm">
        <div className="t-xs">
          <b style={{ color: 'var(--ink-2)' }}>alice</b> can exist once in every population.
          Those are different people: linking them is explicit, and it never merges grants.
        </div>
      </Popover.Dropdown>
    </Popover>
  )
}

function Identities() {
  const { realmFilter, setRealmFilter } = useSession()
  const { openCreate } = useCreate()
  const navigate = useNavigate()

  /** A row is a person, and a person has a page. */
  const openPerson = (i: Identity, give = false) =>
    void navigate({
      to: '/identities/$id', params: { id: i.id }, search: give ? { give: true } : {},
    })

  async function toggleStatus(id: string, current: string) {
    await api.setIdentityStatus(id, current === 'active' ? 'disabled' : 'active')
    notifications.show({
      color: current === 'active' ? 'orange' : 'teal',
      title: current === 'active' ? 'Identity disabled' : 'Identity re-enabled',
      message: current === 'active'
        ? 'authorize() gates on identity state — every grant is dead until re-enabled.'
        : 'Grants apply again immediately.',
    })
    await queryClient.invalidateQueries({ queryKey: ['identities'] })
  }
  /* Held here rather than in a route param: these are the encrypted fields,
     and an id in the URL is an id in someone's browser history. The person's
     own page carries their id and is the better place for everything else —
     but not for these two. */
  const [attrsFor, setAttrsFor] = useState<Identity | null>(null)
  const [credsFor, setCredsFor] = useState<Identity | null>(null)
  const { data: realms } = useQuery({ queryKey: qk.realms(), queryFn: api.realms })
  /* Search and paging live in a store, not in this component: every row leads
     off this screen now, and losing the search that found somebody the moment
     you open them is not paging, it is starting again. */
  const { query: q, setQuery: setQ, trail, setTrail, resetPaging } = usePeopleList()
  /* Keyset paging, and it is not optional here: a realm in this installation
     holds fifty thousand people. The screen used to ask for all of them and
     render whatever came back, which is a wrong answer dressed as a slow one.

     A stack of cursors rather than a page number, because keyset paging can
     step forward and back but cannot jump to page 40. */
  const cursor = trail[trail.length - 1] ?? ''
  const { data: page, isFetching } = useQuery({
    /* Under the 'identities' prefix on purpose: disabling somebody invalidates
       ['identities'], and this list used to be keyed 'identities-page', which
       that prefix does not match — so the row kept saying "active" until the
       operator reloaded. */
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
     all called "Enrolment probe" — so the code rides along in the option or
     the list is twelve identical rows. A dozen of these as wrapping chips was
     the single messiest band on the page. */
  const realmOptions = [
    { value: '', label: 'All populations' },
    ...(realms ?? []).map((r) => ({ value: r.id, label: `${r.display_name} · ${r.code}` })),
  ]

  /* The code exists to tell two populations apart. It earns its line when it
     does that job — another population answers to the same display name — or
     when it says something the name does not. "Internal" over "internal", or
     "Partners" over "partner", is the same word twice on every row. */
  const ambiguous = new Set(
    (realms ?? [])
      .map((r) => r.display_name)
      .filter((n, idx, all) => all.indexOf(n) !== idx),
  )
  function populationCode(code: string, name: string): string | undefined {
    if (ambiguous.has(name)) return code
    const a = code.toLowerCase().replace(/[^a-z0-9]/g, '')
    const b = name.toLowerCase().replace(/[^a-z0-9]/g, '')
    return a.startsWith(b) || b.startsWith(a) ? undefined : code
  }

  const columns: Column<Identity>[] = [
    /* An explicit width even though this column flexes: without one it is the
       only column the browser can grow, so every spare pixel on a wide display
       pooled into a single gap between the email and the next column. */
    { key: 'person', header: 'Person', width: 380, render: (i) => {
        const r = realmOf(i.realm_id)
        /* Code AND realm: a category code is unique inside a realm, not
           across the tenant, so "supplier" in Partners and "supplier" in
           Public are two different categories with two different names. */
        const c = categories?.find((x) => x.code === i.category && x.realm_id === i.realm_id)
        const sub = [i.email, c?.display_name ?? i.category].filter(Boolean).join(' · ')
        return (
          <div className="flex min-w-0 items-center gap-2.5">
            <Initial name={i.username} colour={realmKindColor(r?.kind)} />
            <Cell top={i.username}
              bottom={sub || <span style={{ opacity: 0.5 }}>no email</span>} />
          </div>
        )
      } },
    { key: 'population', header: 'Population', width: 200, render: (i) => {
        const r = realmOf(i.realm_id)
        return r
          ? <Cell top={r.display_name} bottom={populationCode(r.code, r.display_name)} />
          : <span className="t-xs">—</span>
      } },
    { key: 'ial', header: 'Assurance', width: 120, render: (i) => (
        <Tooltip label={IAL_HINT[i.assurance_level]} withArrow>
          <span className="chip" style={{ color: IAL_COLOR[i.assurance_level], cursor: 'help' }}>
            IAL{i.assurance_level}
          </span>
        </Tooltip>
      ) },
    { key: 'status', header: 'Status', width: 130, render: (i) => <Status status={i.status} /> },
    { key: 'seen', header: 'Last sign-in', width: 150,
      headerHint: 'An account nobody has ever used is the one worth asking about.',
      render: (i) =>
        i.last_login_at
          ? <span className="tnum t-body">{i.last_login_at.slice(0, 10)}</span>
          : <span className="t-xs" style={{ opacity: 0.6 }}>never</span> },
    /* "no statutory limit" is true of nearly every employee, so spelling it
       out put a sentence on every row to say nothing. The dash says the same
       and the header hint explains it once. */
    { key: 'retention', header: 'Retention', width: 140,
      headerHint: 'When the population sets a statutory retention limit, the deadline shows here. A dash means no limit.',
      render: (i) =>
        i.retention_until
          ? <span className="tnum t-body">{i.retention_until.slice(0, 10)}</span>
          : <span className="t-xs" style={{ opacity: 0.45 }}>—</span> },
    { key: 'actions', header: '', width: 74, render: (i) => (
        /* The row itself navigates now, so the menu has to keep its clicks to
           itself or every pick would also leave the page behind it. */
        <div className="flex items-center justify-end gap-0.5">
          <div onClick={(e) => e.stopPropagation()} onKeyDown={(e) => e.stopPropagation()}>
          <Menu position="bottom-end" width={230} shadow="xl">
            <Menu.Target>
              <ActionIcon variant="subtle" color="gray" aria-label={`Actions for ${i.username}`}>
                <IconDots size={15} />
              </ActionIcon>
            </Menu.Target>
            <Menu.Dropdown>
              <Menu.Item leftSection={<IconUser size={14} />}
                onClick={() => openPerson(i)}>
                Open their page
              </Menu.Item>
              <Menu.Item leftSection={<IconCirclePlus size={14} />}
                onClick={() => openPerson(i, true)}>
                Give access…
              </Menu.Item>
              <Menu.Item leftSection={<IconCopy size={14} />}
                onClick={() => { void navigator.clipboard.writeText(i.id) }}>
                Copy ID
              </Menu.Item>
              <Menu.Item leftSection={<IconKey size={14} />}
                onClick={() => setCredsFor(i)}>
                Credentials…
              </Menu.Item>
              <Menu.Item leftSection={<IconLock size={14} />}
                onClick={() => setAttrsFor(i)}>
                Encrypted attributes
              </Menu.Item>
              <Menu.Divider />
              {i.status === 'active' ? (
                <Menu.Item color="red" leftSection={<IconUserOff size={14} />}
                  onClick={() => void toggleStatus(i.id, i.status)}>
                  Disable — kills all access now
                </Menu.Item>
              ) : (
                <Menu.Item color="teal" leftSection={<IconUserCheck size={14} />}
                  onClick={() => void toggleStatus(i.id, i.status)}>
                  Re-enable
                </Menu.Item>
              )}
            </Menu.Dropdown>
          </Menu>
          </div>
          {/* The affordance for the row click. Hidden until the row is hovered
              or focused, so fifty rows do not each carry a permanent arrow. */}
          <IconChevronRight className="row-go" size={14} aria-hidden />
        </div>
      ) },
  ]

  /* Filters sit on the table they filter. They used to live in the page
     header, a hand's width from the global ⌘K box — two grey search fields
     side by side, only one of which searched this screen. */
  const toolbar = (
    <>
      <TextInput
        size="xs"
        w={260}
        placeholder="Search username or email"
        leftSection={<IconSearch size={14} />}
        value={q}
        onChange={(e) => setQ(e.currentTarget.value)}
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
        {isFetching ? 'loading…' : `${rows?.length ?? 0} shown`}
      </span>
    </>
  )

  /* Rendered whether or not there is a next page: the count used to appear
     only when paging controls did, so a list that fit on one page reported
     its size nowhere at all. */
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
          Everyone who can sign in — employees, supplier contacts, applicants. Each belongs
          to one population, and a username only has to be unique inside it.
          <br />
          <SameNameNote />
        </>
      }
      wide
      actions={
        <Button size="xs" leftSection={<IconUserPlus size={14} />}
          onClick={() => openCreate('identity')}>
          Add person
        </Button>
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
    </Page>
  )
}
