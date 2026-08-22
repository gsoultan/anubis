import { createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import {
  Button, ColorInput, SegmentedControl, Select, Switch, TextInput,
} from '@mantine/core'
import { IconDeviceFloppy, IconRestore } from '@tabler/icons-react'
import { useEffect, useState } from 'react'
import { Page } from '@/components/shell/Page'
import { SignInPreview } from '@/components/signin/SignInPreview'
import { api } from '@/lib/api/client'
import { qk } from '@/lib/query/keys'
import { queryClient } from '@/lib/query/client'
import { notifyCreated, notifyRejected } from '@/components/create/shell'
import type { SignInConfig } from '@/lib/api/types'

export const Route = createFileRoute('/signin-page')({
  validateSearch: (s: Record<string, unknown>) => ({
    tenant: typeof s['tenant'] === 'string' ? s['tenant'] : undefined,
  }),
  component: SignInBuilder,
})

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <div className="t-body mb-1.5" style={{ fontWeight: 550 }}>{label}</div>
      {children}
    </div>
  )
}

function SignInBuilder() {
  const { tenant: tenantParam } = Route.useSearch()
  const { data: tenants } = useQuery({ queryKey: qk.tenants(), queryFn: api.tenants })
  const [tenantId, setTenantId] = useState<string | null>(tenantParam ?? null)
  useEffect(() => {
    if (!tenantId && tenants?.[0]) setTenantId(tenants[0].id)
  }, [tenants, tenantId])

  const { data: saved } = useQuery({
    queryKey: qk.signin(tenantId ?? ''),
    queryFn: () => api.signin(tenantId!),
    enabled: !!tenantId,
  })
  const [cfg, setCfg] = useState<SignInConfig | null>(null)
  useEffect(() => { if (saved) setCfg({ ...saved }) }, [saved])

  const [busy, setBusy] = useState(false)
  const dirty = !!saved && !!cfg && JSON.stringify(saved) !== JSON.stringify(cfg)

  const save = async () => {
    if (!tenantId || !cfg) return
    setBusy(true)
    try {
      await api.saveSignin(tenantId, cfg)
      notifyCreated('Sign-in page saved', 'The next sign-in on this tenant uses the new design.')
      await queryClient.invalidateQueries({ queryKey: qk.signin(tenantId) })
    } catch (e) { notifyRejected(e) }
    setBusy(false)
  }

  const set = <K extends keyof SignInConfig>(k: K, v: SignInConfig[K]) =>
    setCfg((c) => (c ? { ...c, [k]: v } : c))

  return (
    <Page
      title="Sign-in pages"
      description="Brand each tenant's login. The preview is the real page component — what you see is exactly what signs people in. The knobs are deliberately constrained: nothing here can break the form or its accessibility."
      wide
      actions={
        <>
          <Select w={220} data={(tenants ?? []).map((t) => ({ value: t.id, label: t.name }))}
            value={tenantId} onChange={setTenantId} />
          <Button size="xs" variant="default" leftSection={<IconRestore size={13} />}
            disabled={!dirty} onClick={() => saved && setCfg({ ...saved })}>
            Discard
          </Button>
          <Button size="xs" leftSection={<IconDeviceFloppy size={13} />}
            disabled={!dirty} loading={busy} onClick={() => void save()}>
            Save
          </Button>
        </>
      }
    >
      {cfg && (
        <div className="grid gap-5" style={{ gridTemplateColumns: 'minmax(300px, 360px) minmax(0, 1fr)' }}>
          <div className="flex flex-col gap-4">
            <div className="panel flex flex-col gap-3.5 p-4">
              <div className="t-label">Brand</div>
              <Field label="Logo text">
                <TextInput value={cfg.logo_text} placeholder="Impack"
                  onChange={(e) => set('logo_text', e.currentTarget.value)} />
              </Field>
              <Field label="Brand colour">
                <ColorInput value={cfg.brand_color} format="hex" withEyeDropper={false}
                  onChange={(v) => set('brand_color', v)} />
              </Field>
              <Field label="Theme">
                <SegmentedControl fullWidth size="xs" value={cfg.theme}
                  onChange={(v) => set('theme', v as SignInConfig['theme'])}
                  data={[{ value: 'light', label: 'Light' }, { value: 'dark', label: 'Dark' }]} />
              </Field>
              <Field label="Background">
                <SegmentedControl fullWidth size="xs" value={cfg.background}
                  onChange={(v) => set('background', v as SignInConfig['background'])}
                  data={[{ value: 'solid', label: 'Solid' }, { value: 'gradient', label: 'Brand gradient' }]} />
              </Field>
            </div>

            <div className="panel flex flex-col gap-3.5 p-4">
              <div className="t-label">Layout & content</div>
              <Field label="Layout">
                <SegmentedControl fullWidth size="xs" value={cfg.layout}
                  onChange={(v) => set('layout', v as SignInConfig['layout'])}
                  data={[{ value: 'centered', label: 'Centered card' }, { value: 'split', label: 'Brand panel' }]} />
              </Field>
              <Field label="Headline">
                <TextInput value={cfg.headline}
                  onChange={(e) => set('headline', e.currentTarget.value)} />
              </Field>
              <Field label="Subheadline">
                <TextInput value={cfg.subheadline}
                  onChange={(e) => set('subheadline', e.currentTarget.value)} />
              </Field>
              <Field label="Footer help text">
                <TextInput value={cfg.footer_note}
                  onChange={(e) => set('footer_note', e.currentTarget.value)} />
              </Field>
              <Field label="Language">
                <SegmentedControl fullWidth size="xs" value={cfg.language}
                  onChange={(v) => set('language', v as SignInConfig['language'])}
                  data={[{ value: 'en', label: 'English' }, { value: 'id', label: 'Bahasa Indonesia' }]} />
              </Field>
              <div className="flex items-center justify-between">
                <div>
                  <div className="t-body" style={{ fontWeight: 550 }}>Population picker</div>
                  <div className="t-xs">Let people choose internal / partner / public</div>
                </div>
                <Switch checked={cfg.show_populations}
                  onChange={(e) => set('show_populations', e.currentTarget.checked)} />
              </div>
            </div>
          </div>

          <div>
            <div className="mb-2 flex items-baseline justify-between">
              <span className="t-label">Live preview</span>
              <span className="t-xs">{dirty ? 'unsaved changes' : 'saved'}</span>
            </div>
            <SignInPreview cfg={cfg} populations={['Internal', 'Partners', 'Public']} />
          </div>
        </div>
      )}
    </Page>
  )
}
