import { createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import {
  ActionIcon, Button, ColorInput, CopyButton, Menu, Modal, NumberInput,
  SegmentedControl, Select, Switch, Tabs, TextInput, Textarea, Tooltip,
} from '@mantine/core'
import {
  IconAdjustments, IconAlertTriangle, IconCheck, IconCopy, IconDeviceDesktop, IconDeviceFloppy,
  IconDeviceMobile, IconDots, IconExternalLink, IconLayoutList, IconPalette,
  IconPlus, IconRestore, IconStar, IconTrash,
} from '@tabler/icons-react'
import { useEffect, useId, useState } from 'react'
import { Page } from '@/components/shell/Page'
import { PagePreview } from '@/components/page/PagePreview'
import { SectionOrder } from '@/components/page/SectionOrder'
import { LinksEditor } from '@/components/page/LinksEditor'
import { ContrastNote, ThemePresets, withPreset } from '@/components/page/ThemeTools'
import { api } from '@/lib/api/client'
import { defaultPageConfig } from '@/lib/api/live'
import { qk } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/client'
import { notifyCreated, notifyRejected } from '@/components/create/shell'
import type {
  AuthPage, PageConfig, PageEntrance, PageKind, PageLink, PageSection,
} from '@/lib/api/types'

export const Route = createFileRoute('/signin-page')({
  validateSearch: (s: Record<string, unknown>) => ({
    tenant: typeof s['tenant'] === 'string' ? s['tenant'] : undefined,
  }),
  component: PageBuilder,
})

/* A labelled control, or a labelled group of them.
   role="group" + aria-labelledby rather than a <label> wrapper: half of these
   hold a SegmentedControl or a list of rows, and a <label> wrapping a group of
   radios claims to name one control when it names several. A screen reader
   reads the group name either way; a wrapper would have lied about the shape. */
function Field({ label, hint, count, children }: {
  label: string
  hint?: string
  /** "used / limit" for a bounded text field, shown only as it gets close. */
  count?: { used: number; max: number }
  children: React.ReactNode
}) {
  const id = useId()
  const near = count !== undefined && count.used > count.max * 0.8
  return (
    <div role="group" aria-labelledby={id}>
      <div className="mb-1.5 flex items-baseline justify-between gap-2">
        <div id={id} className="t-body" style={{ fontWeight: 550 }}>{label}</div>
        {near && (
          <span className="t-xs tnum" style={{ color: count.used > count.max ? 'var(--deny)' : 'var(--ink-3)' }}>
            {count.used}/{count.max}
          </span>
        )}
      </div>
      {children}
      {hint && <div className="t-xs mt-1" style={{ opacity: 0.7 }}>{hint}</div>}
    </div>
  )
}

/* The server's own limits (pagecfg.Copy.validate). Enforced here so the
   refusal arrives while somebody is typing rather than when they press Save
   and lose the trip. */
const runes = (s: string) => [...s].length

/* The server names the field that is wrong (pagecfg.invalid) and its message
   arrives as `field=copy.heading, reason=too long`. Correct, and not the
   words on screen: nothing in the builder is called copy.heading. This maps
   the field back to the label the operator is looking at, and falls through
   to the raw text for anything it does not recognise — a message it cannot
   translate is still a message worth showing. */
const FIELD_LABEL: Record<string, string> = {
  'brand.title': 'Company name',
  'brand.logo_url': 'Logo URL',
  'brand.primary_color': 'Brand colour',
  'brand.background_color': 'Background colour',
  'brand.text_color': 'Text colour',
  'brand.corner_radius': 'Corners',
  'brand.font': 'Typeface',
  'layout': 'Layout',
  'sections': 'Blocks',
  'motion.entrance': 'Entrance',
  'copy.heading': 'Headline',
  'copy.subheading': 'Description',
  'copy.username_label': 'Username label',
  'copy.password_label': 'Password label',
  'copy.submit_label': 'Submit button',
  'copy.confirm_heading': 'Confirm headline',
  'copy.confirm_body': 'Confirm body',
  'copy.body': 'Signed-out body',
  'copy.return_label': 'Return link label',
  'behavior.auto_redirect_seconds': 'Return automatically after',
  'behavior.default_return_url': 'Default return URL',
}

function explain(raw: string): string {
  const field = /field=([^,)\s]+)/.exec(raw)?.[1]
  const reason = /reason=([^)]+)/.exec(raw)?.[1]?.trim()
  if (!field) return raw
  const link = /^links\[(\d+)\]/.exec(field)
  const label = link ? `Link ${Number(link[1]) + 1}` : FIELD_LABEL[field]
  if (!label) return raw
  return reason ? `${label}: ${reason}` : `${label} is not valid`
}

const LIMIT = {
  heading: 120, subheading: 240, username_label: 60, password_label: 60,
  submit_label: 60, confirm_heading: 120, confirm_body: 500, body: 500,
  return_label: 60,
} as const

/* What this page answers for, in the words the resolution order uses. */
function bindingOf(p: AuthPage): string {
  if (p.application_slug) return `application: ${p.application_slug}`
  if (p.realm_code) return `population: ${p.realm_code}`
  if (p.is_default) return 'everything else'
  return 'reachable by its link only'
}

function PageBuilder() {
  /* Both kinds live in auth_pages and share brand, layout and motion; only the
     copy and the last panel differ. Editing them in one place is what keeps a
     tenant's sign-out looking like its sign-in. */
  const [kind, setKind] = useState<PageKind>('signin')
  const { data: pages } = useQuery({
    queryKey: qk.authPages(kind),
    queryFn: () => api.authPages(kind),
  })

  /* Bindings need something to bind to. */
  const { data: realmList } = useQuery({ queryKey: qk.realms(), queryFn: () => api.realms() })
  const { data: appList } = useQuery({ queryKey: ['app-choices'], queryFn: () => api.applications() })

  const [creating, setCreating] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState<AuthPage | null>(null)
  const [pageId, setPageId] = useState<string | null>(null)
  const selected: AuthPage | undefined =
    pages?.find((p) => p.id === pageId) ?? pages?.find((p) => p.is_default) ?? pages?.[0]

  const [cfg, setCfg] = useState<PageConfig | null>(null)
  useEffect(() => {
    if (selected) {
      setPageId(selected.id)
      setCfg(structuredClone(selected.config))
    }
  }, [selected?.id])

  /* Switching page or kind reloads cfg from whatever became selected — which
     silently threw away unsaved work, with no warning and no undo. Anything
     that would reselect now goes through switchTo, and a dirty config parks
     the request here until somebody answers for it. */
  type Target = { kind: PageKind } | { id: string }
  const [pendingSwitch, setPendingSwitch] = useState<Target | null>(null)
  function applySwitch(t: Target) {
    if ('kind' in t) { setKind(t.kind); setPageId(null) } else { setPageId(t.id) }
  }
  function switchTo(t: Target) {
    if (dirty) { setPendingSwitch(t); return }
    applySwitch(t)
  }

  /* Sign-out renders two states and both are configurable, so the preview has
     to be able to show either. */
  const [signedOut, setSignedOut] = useState(false)
  /* The builder used to be one column of every control there is. Three groups
     answer three different questions — what it looks like, what it says, what
     it does — and only one of them is open at a time. */
  const [tab, setTab] = useState<string>('design')
  /* Most people who reach a sign-in page are holding a phone, so the preview
     opens on one. 390 is an iPhone 14/15 in CSS pixels. */
  const [device, setDevice] = useState<'phone' | 'desktop'>('phone')
  const [busy, setBusy] = useState(false)
  const dirty =
    !!selected && !!cfg && JSON.stringify(selected.config) !== JSON.stringify(cfg)

  /* The server has always been able to check a draft without storing it
     (PreviewAuthPage -> pagecfg.Validate) and nothing called it, so the first
     word anybody got about an invalid config was a failed Save. Same validator,
     same message, while they type — and Save is held shut until it passes.
     A transport failure is not a config problem and must not block saving. */
  const [problem, setProblem] = useState<string | null>(null)
  useEffect(() => {
    if (!cfg) { setProblem(null); return }
    let cancelled = false
    const t = setTimeout(() => {
      api.validatePageConfig(kind, cfg)
        .then((err) => { if (!cancelled) setProblem(err) })
        .catch(() => { if (!cancelled) setProblem(null) })
    }, 350)
    return () => { cancelled = true; clearTimeout(t) }
  }, [cfg, kind])

  /* The picker offers populations that accept a password and no others —
     oidc_handler.go filters on exactly that before rendering, so a preview
     that showed three invented names was showing a page nobody gets. */
  const passwordRealms = (realmList ?? []).filter((r) => r.allowed_factors.includes('password'))
  const previewRealms = passwordRealms.map((r) => r.display_name)

  /* Confirm off does NOT make the asking step unreachable, which is what it
     looks like from the switch. It decides what `/v1/logout` does — the
     RP-initiated endpoint an application links to, which then ends the session
     without asking — while this page's own URL renders the asking step either
     way (page_handler.go never signs anybody out; a GET must not). So both
     steps stay previewable, and the note under the switch says which is which. */
  const confirmOff = kind === 'signout' && cfg?.behavior?.confirm === false

  /* Section-wise setters. The config is nested because the server's is
     (internal/tenancy/domain/pagecfg) — flattening it here is what produced a
     builder whose output the hosted page could not read. */
  const brand = <K extends keyof PageConfig['brand']>(k: K, v: PageConfig['brand'][K]) =>
    setCfg((c) => (c ? { ...c, brand: { ...c.brand, [k]: v } } : c))
  const copy = <K extends keyof PageConfig['copy']>(k: K, v: PageConfig['copy'][K]) =>
    setCfg((c) => (c ? { ...c, copy: { ...c.copy, [k]: v } } : c))
  const feature = (k: keyof NonNullable<PageConfig['features']>, v: boolean) =>
    setCfg((c) => (c ? { ...c, features: { ...(c.features ?? {}), [k]: v } } : c))
  const behavior = <K extends keyof NonNullable<PageConfig['behavior']>>(
    k: K, v: NonNullable<PageConfig['behavior']>[K],
  ) => setCfg((c) => (c ? { ...c, behavior: { ...(c.behavior ?? {}), [k]: v } } : c))

  async function refresh() {
    await queryClient.invalidateQueries({ queryKey: qk.authPages(kind) })
  }

  async function promote(p: AuthPage) {
    try {
      await api.setDefaultAuthPage(p.id)
      await refresh()
      notifyCreated(`"${p.name}" is now the default`,
        'Anything without a more specific match renders it.')
    } catch (e) { notifyRejected(e) }
  }

  async function remove(p: AuthPage) {
    try {
      await api.deleteAuthPage(p.id)
      setConfirmDelete(null)
      if (pageId === p.id) setPageId(null)
      await refresh()
      notifyCreated(`"${p.name}" deleted`, 'Requests it answered now fall through.')
    } catch (e) { notifyRejected(e) }
  }

  async function save() {
    if (!selected || !cfg) return
    setBusy(true)
    try {
      await api.saveAuthPage({ ...selected, config: cfg })
      await queryClient.invalidateQueries({ queryKey: qk.authPages(kind) })
      notifyCreated(
        kind === 'signout' ? 'Sign-out page saved' : 'Sign-in page saved',
        'The hosted page now renders this configuration.',
      )
    } catch (e) {
      /* The server validates the whole config and names the first bad field
         (pagecfg.Validate), so show that rather than "invalid configuration". */
      notifyRejected(e)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Page
      title={kind === 'signout' ? 'Sign-out page' : 'Sign-in page'}
      actions={
        <>
          <Button
            variant="default" size="xs" leftSection={<IconRestore size={15} />}
            disabled={!dirty || busy}
            onClick={() => selected && setCfg(structuredClone(selected.config))}
          >
            Revert
          </Button>
          <Button
            size="xs" leftSection={<IconDeviceFloppy size={15} />}
            disabled={!dirty || busy || problem !== null} loading={busy} onClick={save}
          >
            Save
          </Button>
        </>
      }
    >
      <NewPageModal
        opened={creating} kind={kind}
        realms={realmList ?? []} apps={appList ?? []}
        onClose={() => setCreating(false)}
        onCreated={async (p) => { setCreating(false); setPageId(p.id); await refresh() }}
      />

      {/* The only thing standing between unsaved work and the reload that was
          silently eating it. */}
      <Modal
        opened={!!pendingSwitch} onClose={() => setPendingSwitch(null)}
        title="Discard unsaved changes?" centered size="sm"
      >
        <div className="t-body mb-4" style={{ opacity: 0.8 }}>
          This page has edits that have not been saved. Opening another one
          reloads it from the server, and the edits are gone.
        </div>
        <div className="flex justify-end gap-2">
          <Button variant="default" size="xs" onClick={() => setPendingSwitch(null)}>
            Keep editing
          </Button>
          <Button
            color="red" size="xs"
            onClick={() => { if (pendingSwitch) applySwitch(pendingSwitch); setPendingSwitch(null) }}
          >
            Discard
          </Button>
        </div>
      </Modal>

      <Modal
        opened={!!confirmDelete} onClose={() => setConfirmDelete(null)}
        title={`Delete "${confirmDelete?.name ?? ''}"?`} centered size="sm"
      >
        <div className="t-body mb-4" style={{ opacity: 0.8 }}>
          Anyone this page answered for falls through to the next match —
          {' '}{confirmDelete ? bindingOf(confirmDelete) : ''} becomes whatever
          the default renders. Its URL stops resolving.
        </div>
        <div className="flex justify-end gap-2">
          <Button variant="default" size="xs" onClick={() => setConfirmDelete(null)}>Cancel</Button>
          <Button color="red" size="xs" onClick={() => confirmDelete && remove(confirmDelete)}>
            Delete
          </Button>
        </div>
      </Modal>

      {cfg && (
        // Editor beside preview needs ~640px of content, which the sidebar
        // leaves only from lg up; below that the preview goes underneath.
        <div className="grid grid-cols-1 gap-5 lg:grid-cols-[minmax(300px,380px)_minmax(0,1fr)]">
          <div className="flex flex-col gap-4">
            <div className="panel flex flex-col gap-3 p-4">
              <div className="t-label">Pages</div>
              <SegmentedControl
                fullWidth size="xs" value={kind}
                onChange={(v) => switchTo({ kind: v as PageKind })}
                data={[
                  { value: 'signin', label: 'Sign-in' },
                  { value: 'signout', label: 'Sign-out' },
                ]}
              />

              {/* A tenant has as many pages of each kind as it wants. The
                  concept is invisible while only one exists, which is exactly
                  when somebody needs it explained. */}
              <div className="t-xs" style={{ opacity: 0.7 }}>
                One page is the tenant default. Add more to give an application
                or a population its own — the most specific match wins:
                {' '}<strong>link → application → population → default</strong>.
              </div>

              <div className="flex flex-col gap-1">
                {(pages ?? []).map((p) => (
                  /* A row somebody has to reach with a mouse is a row a
                     keyboard operator cannot open; it was a div with onClick. */
                  <div
                    key={p.id}
                    className="flex items-center gap-2 rounded px-2 py-1.5"
                    style={{
                      background: p.id === selected?.id ? 'rgb(120 120 160 / .12)' : undefined,
                    }}
                  >
                    <button
                      type="button"
                      aria-current={p.id === selected?.id}
                      className="min-w-0 flex-1 text-left"
                      style={{ background: 'none', border: 0, padding: 0, cursor: 'pointer' }}
                      onClick={() => switchTo({ id: p.id })}
                    >
                      <div className="t-body truncate" style={{ fontWeight: 550 }}>{p.name}</div>
                      <div className="t-xs truncate" style={{ opacity: 0.65 }}>{bindingOf(p)}</div>
                    </button>
                    {p.is_default && <span className="t-xs" style={{ opacity: 0.7 }}>default</span>}
                    <Menu position="bottom-end" withinPortal>
                      <Menu.Target>
                        <ActionIcon variant="subtle" size="sm" onClick={(e) => e.stopPropagation()}>
                          <IconDots size={14} />
                        </ActionIcon>
                      </Menu.Target>
                      <Menu.Dropdown>
                        <Menu.Item
                          leftSection={<IconStar size={14} />}
                          disabled={p.is_default}
                          onClick={() => promote(p)}
                        >
                          Make default
                        </Menu.Item>
                        <Menu.Item
                          color="red" leftSection={<IconTrash size={14} />}
                          disabled={p.is_default}
                          onClick={() => setConfirmDelete(p)}
                        >
                          Delete
                        </Menu.Item>
                      </Menu.Dropdown>
                    </Menu>
                  </div>
                ))}
              </div>

              <Button
                variant="default" size="xs" leftSection={<IconPlus size={14} />}
                onClick={() => setCreating(true)}
              >
                New {kind === 'signout' ? 'sign-out' : 'sign-in'} page
              </Button>
            </div>

            {selected && <PageURL page={selected} />}

            {selected && (selected.realm_code || selected.application_slug) && (
              <div className="panel p-4">
                <div className="t-label mb-1.5">Who sees this</div>
                <div className="t-body" style={{ opacity: 0.8 }}>
                  {selected.application_slug
                    ? `Anyone arriving through ${selected.application_slug}, whichever population they belong to.`
                    : `The ${selected.realm_code} population — unless the application they arrive through has its own page.`}
                </div>
              </div>
            )}

            <div className="panel">
              <Tabs value={tab} onChange={(v) => setTab(v ?? 'design')}>
                <Tabs.List grow>
                  <Tabs.Tab value="design" leftSection={<IconPalette size={14} />}>Design</Tabs.Tab>
                  <Tabs.Tab value="content" leftSection={<IconLayoutList size={14} />}>Content</Tabs.Tab>
                  <Tabs.Tab value="behaviour" leftSection={<IconAdjustments size={14} />}>
                    {kind === 'signout' ? 'Behaviour' : 'Features'}
                  </Tabs.Tab>
                </Tabs.List>

                <Tabs.Panel value="design">
                  <div className="flex flex-col gap-3.5 p-4">
                    <Field label="Start from" hint="Sets colours, corners, typeface and layout. Your words and links are left alone.">
                      <ThemePresets onApply={(preset) => setCfg((c) => (c ? withPreset(c, preset) : c))} />
                    </Field>

                    <Field label="Company name">
                      <TextInput value={cfg.brand.title} placeholder="Impack"
                        onChange={(e) => brand('title', e.currentTarget.value)} />
                    </Field>
                    <Field
                      label="Logo URL"
                      hint="https:// image shown above the heading. Falls back to the first letter of the company name."
                    >
                      <TextInput
                        value={cfg.brand.logo_url ?? ''} placeholder="https://cdn.example.com/logo.svg"
                        onChange={(e) => brand('logo_url', e.currentTarget.value)}
                      />
                    </Field>

                    <div className="grid gap-2.5" style={{ gridTemplateColumns: '1fr 1fr' }}>
                      <Field label="Brand">
                        <ColorInput size="xs" value={cfg.brand.primary_color} format="hex" withEyeDropper={false}
                          onChange={(v) => brand('primary_color', v)} />
                      </Field>
                      <Field label="Background">
                        <ColorInput size="xs" value={cfg.brand.background_color} format="hex" withEyeDropper={false}
                          onChange={(v) => brand('background_color', v)} />
                      </Field>
                    </div>
                    <Field
                      label="Text"
                      hint="The card, the button label and the hairlines are worked out from these three."
                    >
                      <ColorInput size="xs" value={cfg.brand.text_color} format="hex" withEyeDropper={false}
                        onChange={(v) => brand('text_color', v)} />
                    </Field>
                    <ContrastNote cfg={cfg} />

                    <Field label="Corners">
                      <SegmentedControl
                        fullWidth size="xs" value={cfg.brand.corner_radius}
                        onChange={(v) => brand('corner_radius', v as PageConfig['brand']['corner_radius'])}
                        data={[
                          { value: 'none', label: 'None' }, { value: 'sm', label: 'S' },
                          { value: 'md', label: 'M' }, { value: 'lg', label: 'L' },
                          { value: 'full', label: 'Full' },
                        ]}
                      />
                    </Field>
                    <Field label="Typeface">
                      <SegmentedControl
                        fullWidth size="xs" value={cfg.brand.font}
                        onChange={(v) => brand('font', v as PageConfig['brand']['font'])}
                        data={[
                          { value: 'system', label: 'System' },
                          { value: 'serif', label: 'Serif' },
                          { value: 'mono', label: 'Mono' },
                        ]}
                      />
                    </Field>
                    <Field label="Layout">
                      <SegmentedControl
                        fullWidth size="xs" value={cfg.layout}
                        onChange={(v) => setCfg((c) => (c ? { ...c, layout: v as PageConfig['layout'] } : c))}
                        data={[
                          { value: 'centered', label: 'Centered' },
                          { value: 'split', label: 'Split' },
                          { value: 'minimal', label: 'Minimal' },
                        ]}
                      />
                    </Field>
                    {cfg.layout === 'split' && (
                      <div className="t-xs" style={{ opacity: 0.7 }}>
                        The banner half is hidden below 820px, so on a phone this
                        renders as the centered layout. Check the phone preview.
                      </div>
                    )}
                    <Field
                      label="Entrance"
                      hint="Respects the visitor's reduced-motion setting; they see none regardless."
                    >
                      <SegmentedControl
                        fullWidth size="xs" value={cfg.motion?.entrance ?? 'none'}
                        onChange={(v) =>
                          setCfg((c) => (c ? { ...c, motion: { entrance: v as PageEntrance } } : c))}
                        data={[
                          { value: 'none', label: 'None' },
                          { value: 'fade', label: 'Fade' },
                          { value: 'rise', label: 'Rise' },
                        ]}
                      />
                    </Field>
                  </div>
                </Tabs.Panel>

                <Tabs.Panel value="content">
                  <div className="flex flex-col gap-3.5 p-4">
                    <Field
                      label="Blocks"
                      hint="Drag to reorder, or focus a handle and use the arrow keys. Hidden blocks keep their settings."
                    >
                      <SectionOrder
                        sections={cfg.sections} kind={kind}
                        onChange={(next: PageSection[]) =>
                          setCfg((c) => (c ? { ...c, sections: next } : c))}
                      />
                    </Field>

                    {kind === 'signout' ? (
                      <>
                        <Field label="Confirm headline" hint="Shown while asking whether to sign out."
                          count={{ used: runes(cfg.copy.confirm_heading ?? ''), max: LIMIT.confirm_heading }}>
                          <TextInput value={cfg.copy.confirm_heading ?? ''}
                            onChange={(e) => copy('confirm_heading', e.currentTarget.value)} />
                        </Field>
                        <Field label="Confirm body"
                          count={{ used: runes(cfg.copy.confirm_body ?? ''), max: LIMIT.confirm_body }}>
                          <Textarea autosize minRows={2} value={cfg.copy.confirm_body ?? ''}
                            onChange={(e) => copy('confirm_body', e.currentTarget.value)} />
                        </Field>
                        <Field label="Signed-out headline" hint="Shown once the session has ended."
                          count={{ used: runes(cfg.copy.heading), max: LIMIT.heading }}>
                          <TextInput value={cfg.copy.heading}
                            onChange={(e) => copy('heading', e.currentTarget.value)} />
                        </Field>
                        <Field label="Signed-out body"
                          count={{ used: runes(cfg.copy.body ?? ''), max: LIMIT.body }}>
                          <Textarea autosize minRows={2} value={cfg.copy.body ?? ''}
                            onChange={(e) => copy('body', e.currentTarget.value)} />
                        </Field>
                        <Field label="Return link label"
                          count={{ used: runes(cfg.copy.return_label ?? ''), max: LIMIT.return_label }}>
                          <TextInput value={cfg.copy.return_label ?? ''}
                            onChange={(e) => copy('return_label', e.currentTarget.value)} />
                        </Field>
                      </>
                    ) : (
                      <>
                        <Field label="Headline" count={{ used: runes(cfg.copy.heading), max: LIMIT.heading }}>
                          <TextInput value={cfg.copy.heading}
                            onChange={(e) => copy('heading', e.currentTarget.value)} />
                        </Field>
                        <Field label="Description" hint="Shown under the headline. Describe the brand or who this page is for."
                          count={{ used: runes(cfg.copy.subheading ?? ''), max: LIMIT.subheading }}>
                          <Textarea autosize minRows={2} value={cfg.copy.subheading ?? ''}
                            onChange={(e) => copy('subheading', e.currentTarget.value)} />
                        </Field>
                        <Field label="Username label" count={{ used: runes(cfg.copy.username_label), max: LIMIT.username_label }}>
                          <TextInput value={cfg.copy.username_label}
                            onChange={(e) => copy('username_label', e.currentTarget.value)} />
                        </Field>
                        <Field label="Password label" count={{ used: runes(cfg.copy.password_label), max: LIMIT.password_label }}>
                          <TextInput value={cfg.copy.password_label}
                            onChange={(e) => copy('password_label', e.currentTarget.value)} />
                        </Field>
                        <Field label="Submit button" count={{ used: runes(cfg.copy.submit_label), max: LIMIT.submit_label }}>
                          <TextInput value={cfg.copy.submit_label}
                            onChange={(e) => copy('submit_label', e.currentTarget.value)} />
                        </Field>
                      </>
                    )}

                    <Field label="Links" hint="Rendered in this order under the form. Five at most.">
                      <LinksEditor
                        links={cfg.links ?? []}
                        onChange={(next: PageLink[]) => setCfg((c) => (c ? { ...c, links: next } : c))}
                      />
                    </Field>
                  </div>
                </Tabs.Panel>

                <Tabs.Panel value="behaviour">
                  <div className="flex flex-col gap-3.5 p-4">
                    {kind === 'signout' ? (
                      <>
                        <Switch
                          size="xs" label="Ask before signing out"
                          description="Only safe where the link cannot be triggered by somebody else: a bare GET that ends sessions is reachable from any <img>."
                          checked={cfg.behavior?.confirm !== false}
                          onChange={(e) => behavior('confirm', e.currentTarget.checked)}
                        />
                        {confirmOff && (
                          <div className="t-xs" style={{ color: 'var(--warn)' }}>
                            With this off, an application-initiated sign-out
                            (<code>/v1/logout</code>) ends the session without asking
                            and lands on the signed-out step. This page&rsquo;s own URL
                            still shows the asking step — a GET must never end a
                            session — so both are worth previewing.
                          </div>
                        )}
                        <Field label="Return automatically after" hint="0 disables it. The server caps this at 30 seconds.">
                          <NumberInput
                            size="xs" min={0} max={30} suffix="s"
                            value={cfg.behavior?.auto_redirect_seconds ?? 0}
                            onChange={(v) => behavior('auto_redirect_seconds', Number(v) || 0)}
                          />
                        </Field>
                        <Field label="Default return URL" hint="Used when the application does not supply one.">
                          <TextInput
                            value={cfg.behavior?.default_return_url ?? ''} placeholder="https://app.example.com"
                            onChange={(e) => behavior('default_return_url', e.currentTarget.value)}
                          />
                        </Field>
                      </>
                    ) : (
                      <>
                        <Switch
                          size="xs" label="Population picker"
                          description="Let people choose which directory they belong to. Only populations that allow passwords are offered."
                          checked={!!cfg.features?.show_realm_picker}
                          onChange={(e) => feature('show_realm_picker', e.currentTarget.checked)}
                        />
                        {cfg.features?.show_realm_picker && passwordRealms.length === 0 && (
                          <div className="t-xs" style={{ color: 'var(--warn)' }}>
                            No population in this tenant accepts a password, so the
                            picker renders nothing. The preview shows the same.
                          </div>
                        )}
                        <Switch
                          size="xs" label="Registration link"
                          description="Shown only where the population actually allows self-registration — a link to a door that will not open is worse than no link."
                          checked={!!cfg.features?.show_registration}
                          onChange={(e) => feature('show_registration', e.currentTarget.checked)}
                        />
                        <Switch
                          size="xs" label="Remember me"
                          description="Adds the checkbox that asks for a longer session."
                          checked={!!cfg.features?.remember_me}
                          onChange={(e) => feature('remember_me', e.currentTarget.checked)}
                        />
                      </>
                    )}
                  </div>
                </Tabs.Panel>
              </Tabs>
            </div>
          </div>

          {/* Sticky, because the left rail is taller than the viewport and a
              preview you have to scroll back to is a preview nobody watches. */}
          <div style={{ position: 'sticky', top: 12, alignSelf: 'start' }}>
            {problem && (
              /* pagecfg names the first bad field, so this is the same
                 sentence the save would have failed with — only sooner. */
              <div
                className="mb-2 flex items-start gap-2 rounded-md px-2.5 py-2"
                style={{ background: 'var(--deny-bg)' }}
              >
                <IconAlertTriangle size={14} style={{ color: 'var(--deny)', marginTop: 1, flexShrink: 0 }} />
                <div className="t-xs">
                  <b style={{ color: 'var(--ink-2)' }}>The server will refuse this: </b>{explain(problem)}
                </div>
              </div>
            )}
            <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
              <span className="t-label">Live preview</span>
              <div className="flex items-center gap-3">
                <SegmentedControl
                  size="xs" value={device}
                  onChange={(v) => setDevice(v as 'phone' | 'desktop')}
                  data={[
                    { value: 'phone', label: <span className="flex items-center gap-1"><IconDeviceMobile size={13} /> Phone</span> },
                    { value: 'desktop', label: <span className="flex items-center gap-1"><IconDeviceDesktop size={13} /> Desktop</span> },
                  ]}
                />
                {kind === 'signout' && (
                  <SegmentedControl
                    size="xs" value={signedOut ? 'after' : 'confirm'}
                    onChange={(v) => setSignedOut(v === 'after')}
                    data={[
                      { value: 'confirm', label: 'Asking' },
                      { value: 'after', label: 'Signed out' },
                    ]}
                  />
                )}
                <span className="t-xs">{dirty ? 'unsaved changes' : 'saved'}</span>
              </div>
            </div>

            {/* 390 CSS pixels is a current iPhone, and it is the width where
                this page's own media queries change their mind. */}
            <div
              style={device === 'phone'
                ? { width: 390, maxWidth: '100%', margin: '0 auto', borderRadius: 20,
                    overflow: 'hidden', border: '1px solid var(--line)' }
                : undefined}
            >
              <PagePreview
                cfg={cfg} kind={kind} signedOut={signedOut}
                realms={previewRealms}
                width={device === 'phone' ? 390 : undefined}
              />
            </div>
          </div>
        </div>
      )}
    </Page>
  )
}

/* Creating a page is where the concept becomes concrete, so the form is built
   around the question that decides everything else: who is this page for?
   kind and slug are fixed at creation because the URL they form is published
   and changing it silently breaks every link pointing at it. */
function NewPageModal({ opened, kind, realms, apps, onClose, onCreated }: {
  opened: boolean
  kind: PageKind
  realms: { id: string; code: string; display_name: string }[]
  apps: { id: string; slug: string; name: string }[]
  onClose: () => void
  onCreated: (p: AuthPage) => void | Promise<void>
}) {
  const [name, setName] = useState('')
  const [slug, setSlug] = useState('')
  const [bind, setBind] = useState<'none' | 'application' | 'realm'>('none')
  const [target, setTarget] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (opened) { setName(''); setSlug(''); setBind('none'); setTarget(null) }
  }, [opened])

  async function create() {
    setBusy(true)
    try {
      const p = await api.createAuthPage({
        kind, name, slug,
        ...(bind === 'application' && target ? { applicationId: target } : {}),
        ...(bind === 'realm' && target ? { realmId: target } : {}),
        config: defaultPageConfig(kind),
      })
      if (p) {
        notifyCreated(`"${p.name}" created`, `Served at /p/{tenant}/${kind}/${p.slug}`)
        await onCreated(p)
      }
    } catch (e) {
      notifyRejected(e)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal opened={opened} onClose={onClose} centered title={
      kind === 'signout' ? 'New sign-out page' : 'New sign-in page'
    }>
      <div className="flex flex-col gap-3.5">
        <Field label="Name" hint="For you, in this list. Not shown to anyone signing in.">
          <TextInput
            value={name} placeholder="Partner portal"
            onChange={(e) => {
              setName(e.currentTarget.value)
              /* Suggest a slug until the operator edits one themselves. */
              if (!slug || slug === slugify(name)) setSlug(slugify(e.currentTarget.value))
            }}
          />
        </Field>
        <Field
          label="URL segment"
          hint={`Served at /p/{tenant}/${kind}/${slug || '…'} — at least two characters, and fixed once created because the link is published.`}
        >
          <TextInput value={slug} placeholder="partners"
            onChange={(e) => setSlug(slugify(e.currentTarget.value))} />
        </Field>
        <Field label="Who is this page for?">
          <SegmentedControl
            fullWidth size="xs" value={bind}
            onChange={(v) => { setBind(v as typeof bind); setTarget(null) }}
            data={[
              { value: 'none', label: 'Link only' },
              { value: 'application', label: 'Application' },
              { value: 'realm', label: 'Population' },
            ]}
          />
        </Field>
        {bind === 'application' && (
          <Field label="Application" hint="Anyone arriving through it sees this page.">
            <Select
              searchable value={target} onChange={setTarget}
              data={apps.map((a) => ({ value: a.id, label: `${a.name} (${a.slug})` }))}
            />
          </Field>
        )}
        {bind === 'realm' && (
          <Field label="Population" hint="Unless the application they arrive through has its own page.">
            <Select
              value={target} onChange={setTarget}
              data={realms.map((r) => ({ value: r.id, label: `${r.display_name} (${r.code})` }))}
            />
          </Field>
        )}
        {bind === 'none' && (
          <div className="t-xs" style={{ opacity: 0.7 }}>
            Reachable only at its own URL. Useful for a campaign page, or for
            drafting a design before binding it to anything.
          </div>
        )}
        <div className="mt-1 flex justify-end gap-2">
          <Button variant="default" size="xs" onClick={onClose}>Cancel</Button>
          <Button
            size="xs" loading={busy}
            disabled={!name || slug.length < 2 || (bind !== 'none' && !target)}
            onClick={create}
          >
            Create
          </Button>
        </div>
      </div>
    </Modal>
  )
}

/** Mirrors the server's slug rule: lowercase letters, digits, - and _. */
function slugify(s: string): string {
  return s.toLowerCase().replace(/[^a-z0-9_-]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 63)
}

/* The address the page is served at.

   The server has been sending this all along — AuthPage.url, whose proto
   comment reads "for the console to show and copy" — and nothing rendered it.
   So the builder could style a page in detail and still not answer the first
   question anyone asks after building one: what do I link to?

   A sign-in page is a launcher, not a form in a vacuum. ServePage hands the
   flow to /v1/authorize so PKCE, redirect validation and the existing SSO
   session behave as they do everywhere else — and that needs an application to
   sign in TO. A sign-in page bound to a population, or serving as the tenant
   default, renders "This page is not linked to an application yet" instead.
   Showing the link without saying so would hand somebody an address that looks
   live and is not, which is the failure this console keeps having. */
function PageURL({ page }: { page: AuthPage }) {
  /* pageURL() returns "" when the server has no issuer or tenant slug to build
     from — the platform console, where an operator belongs to no tenant. */
  if (!page.url) {
    return (
      <div className="panel p-4">
        <div className="t-label mb-1.5">Address</div>
        <div className="t-xs" style={{ opacity: 0.7 }}>
          Shown when the console is scoped to a tenant. The address is{' '}
          <code>{'{issuer}'}/p/{'{tenant}'}/{page.kind}/{page.slug}</code>.
        </div>
      </div>
    )
  }

  // Sign-out always renders; sign-in only launches with an application bound.
  const launches = page.kind === 'signout' || !!page.application_id

  return (
    <div className="panel p-4">
      <div className="t-label mb-1.5">Address</div>
      <div className="flex items-center gap-2">
        <code
          className="min-w-0 flex-1 truncate"
          style={{
            background: 'var(--s-sunken)', padding: '6px 9px', borderRadius: 6,
            fontSize: 12.5,
          }}
        >
          {page.url}
        </code>
        <CopyButton value={page.url}>
          {({ copied, copy }) => (
            <Tooltip label={copied ? 'Copied' : 'Copy address'}>
              <ActionIcon variant="default" size="md" onClick={copy}>
                {copied ? <IconCheck size={14} /> : <IconCopy size={14} />}
              </ActionIcon>
            </Tooltip>
          )}
        </CopyButton>
        <Tooltip label="Open in a new tab">
          <ActionIcon
            variant="default" size="md" component="a"
            href={page.url} target="_blank" rel="noreferrer"
          >
            <IconExternalLink size={14} />
          </ActionIcon>
        </Tooltip>
      </div>

      {page.kind === 'signout' ? (
        <div className="t-xs mt-2" style={{ opacity: 0.7 }}>
          Ends the session and shows the signed-out page. With confirmation on,
          it asks first.
        </div>
      ) : launches ? (
        <div className="t-xs mt-2" style={{ opacity: 0.7 }}>
          Starts a real sign-in for{' '}
          <span className="chip">{page.application_slug}</span> — the same
          authorization-code flow as everywhere else.
        </div>
      ) : (
        <div className="t-xs mt-2" style={{ color: 'var(--warn)' }}>
          No application is bound, so opening this shows “This page is not linked
          to an application yet.” It still serves as the{' '}
          {page.realm_code ? `page for ${page.realm_code}` : 'tenant default'}{' '}
          when a flow resolves to it — bind an application to make the address
          itself a working entry point.
        </div>
      )}
    </div>
  )
}
