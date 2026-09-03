import { createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import {
  Button, ColorInput, NumberInput, SegmentedControl, Select, Switch, TextInput, Textarea,
} from '@mantine/core'
import { IconDeviceFloppy, IconRestore } from '@tabler/icons-react'
import { useEffect, useState } from 'react'
import { Page } from '@/components/shell/Page'
import { PagePreview } from '@/components/page/PagePreview'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/client'
import { notifyCreated, notifyRejected } from '@/components/create/shell'
import type { AuthPage, PageConfig, PageEntrance, PageKind } from '@/lib/api/types'

export const Route = createFileRoute('/signin-page')({
  validateSearch: (s: Record<string, unknown>) => ({
    tenant: typeof s['tenant'] === 'string' ? s['tenant'] : undefined,
  }),
  component: PageBuilder,
})

function Field({ label, hint, children }: {
  label: string; hint?: string; children: React.ReactNode
}) {
  return (
    <div>
      <div className="t-body mb-1.5" style={{ fontWeight: 550 }}>{label}</div>
      {children}
      {hint && <div className="t-xs mt-1" style={{ opacity: 0.7 }}>{hint}</div>}
    </div>
  )
}

/* Which door this page is. Resolution is slug -> application -> realm ->
   tenant default, so the binding is the most useful thing to see when
   choosing which page to edit. */
function pageLabel(p: AuthPage): string {
  if (p.application_slug) return `${p.name} — app: ${p.application_slug}`
  if (p.realm_code) return `${p.name} — population: ${p.realm_code}`
  return `${p.name}${p.is_default ? ' (tenant default)' : ''}`
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
  useEffect(() => { setPageId(null) }, [kind])

  /* Sign-out renders two states and both are configurable, so the preview has
     to be able to show either. */
  const [signedOut, setSignedOut] = useState(false)
  const [busy, setBusy] = useState(false)
  const dirty =
    !!selected && !!cfg && JSON.stringify(selected.config) !== JSON.stringify(cfg)

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
            disabled={!dirty || busy} loading={busy} onClick={save}
          >
            Save
          </Button>
        </>
      }
    >
      {cfg && (
        <div className="grid gap-5" style={{ gridTemplateColumns: 'minmax(300px, 380px) minmax(0, 1fr)' }}>
          <div className="flex flex-col gap-4">
            <div className="panel flex flex-col gap-3.5 p-4">
              <div className="t-label">Page kind</div>
              <SegmentedControl
                fullWidth size="xs" value={kind}
                onChange={(v) => setKind(v as PageKind)}
                data={[
                  { value: 'signin', label: 'Sign-in' },
                  { value: 'signout', label: 'Sign-out' },
                ]}
              />
              {(pages?.length ?? 0) > 1 && (
                <Field label="Editing" hint="A page may be bound to an application or a population.">
                  <Select
                    size="xs" value={pageId} onChange={(v) => setPageId(v)}
                    data={(pages ?? []).map((p) => ({ value: p.id, label: pageLabel(p) }))}
                  />
                </Field>
              )}
            </div>

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

            <div className="panel flex flex-col gap-3.5 p-4">
              <div className="t-label">Brand</div>
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
              <Field label="Brand colour">
                <ColorInput value={cfg.brand.primary_color} format="hex" withEyeDropper={false}
                  onChange={(v) => brand('primary_color', v)} />
              </Field>
              <Field label="Background">
                <ColorInput value={cfg.brand.background_color} format="hex" withEyeDropper={false}
                  onChange={(v) => brand('background_color', v)} />
              </Field>
              <Field label="Text colour">
                <ColorInput value={cfg.brand.text_color} format="hex" withEyeDropper={false}
                  onChange={(v) => brand('text_color', v)} />
              </Field>
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

            <div className="panel flex flex-col gap-3.5 p-4">
              <div className="t-label">Layout &amp; copy</div>
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

              {kind === 'signout' ? (
                <>
                  <Field label="Confirm headline" hint="Shown while asking whether to sign out.">
                    <TextInput value={cfg.copy.confirm_heading ?? ''}
                      onChange={(e) => copy('confirm_heading', e.currentTarget.value)} />
                  </Field>
                  <Field label="Confirm body">
                    <Textarea autosize minRows={2} value={cfg.copy.confirm_body ?? ''}
                      onChange={(e) => copy('confirm_body', e.currentTarget.value)} />
                  </Field>
                  <Field label="Signed-out headline" hint="Shown once the session has ended.">
                    <TextInput value={cfg.copy.heading}
                      onChange={(e) => copy('heading', e.currentTarget.value)} />
                  </Field>
                  <Field label="Signed-out body">
                    <Textarea autosize minRows={2} value={cfg.copy.body ?? ''}
                      onChange={(e) => copy('body', e.currentTarget.value)} />
                  </Field>
                  <Field label="Return link label">
                    <TextInput value={cfg.copy.return_label ?? ''}
                      onChange={(e) => copy('return_label', e.currentTarget.value)} />
                  </Field>
                </>
              ) : (
                <>
                  <Field label="Headline">
                    <TextInput value={cfg.copy.heading}
                      onChange={(e) => copy('heading', e.currentTarget.value)} />
                  </Field>
                  <Field label="Description" hint="Shown under the headline. Describe the brand or who this page is for.">
                    <Textarea autosize minRows={2} value={cfg.copy.subheading ?? ''}
                      onChange={(e) => copy('subheading', e.currentTarget.value)} />
                  </Field>
                  <Field label="Username label">
                    <TextInput value={cfg.copy.username_label}
                      onChange={(e) => copy('username_label', e.currentTarget.value)} />
                  </Field>
                  <Field label="Password label">
                    <TextInput value={cfg.copy.password_label}
                      onChange={(e) => copy('password_label', e.currentTarget.value)} />
                  </Field>
                  <Field label="Submit button">
                    <TextInput value={cfg.copy.submit_label}
                      onChange={(e) => copy('submit_label', e.currentTarget.value)} />
                  </Field>
                </>
              )}
            </div>

            {kind === 'signout' ? (
              <div className="panel flex flex-col gap-3.5 p-4">
                <div className="t-label">Behaviour</div>
                <Switch
                  size="xs" label="Ask before signing out"
                  description="Off ends the session on arrival. Only safe where the link cannot be triggered by somebody else."
                  checked={cfg.behavior?.confirm !== false}
                  onChange={(e) => behavior('confirm', e.currentTarget.checked)}
                />
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
              </div>
            ) : (
              <div className="panel flex flex-col gap-3.5 p-4">
                <div className="t-label">Features</div>
                <Switch
                  size="xs" label="Population picker"
                  description="Let people choose internal, partner or public before signing in."
                  checked={!!cfg.features?.show_realm_picker}
                  onChange={(e) => feature('show_realm_picker', e.currentTarget.checked)}
                />
                <Switch
                  size="xs" label="Registration link"
                  checked={!!cfg.features?.show_registration}
                  onChange={(e) => feature('show_registration', e.currentTarget.checked)}
                />
                <Switch
                  size="xs" label="Forgot password link"
                  checked={!!cfg.features?.show_forgot_password}
                  onChange={(e) => feature('show_forgot_password', e.currentTarget.checked)}
                />
                <Switch
                  size="xs" label="Remember me"
                  checked={!!cfg.features?.remember_me}
                  onChange={(e) => feature('remember_me', e.currentTarget.checked)}
                />
              </div>
            )}
          </div>

          <div>
            <div className="mb-2 flex items-baseline justify-between">
              <span className="t-label">Live preview</span>
              <div className="flex items-center gap-3">
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
            <PagePreview
              cfg={cfg} kind={kind} signedOut={signedOut}
              realms={['Internal', 'Partners', 'Public']}
            />
          </div>
        </div>
      )}
    </Page>
  )
}
