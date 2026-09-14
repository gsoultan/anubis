import { IconAlertTriangle } from '@tabler/icons-react'
import { contrast, pageColors } from '@/lib/pagePalette'
import type { PageConfig, PageEntrance } from '@/lib/api/types'

/* A starting point, and a warning when the palette stops being readable.
 *
 * Presets are not themes in any deep sense — they set the same six tokens an
 * operator could set by hand. They exist because the builder opens on indigo
 * on grey and the honest first question is "can it look like us", which is
 * much easier to answer from something that is already not the default.
 *
 * The warning is the other half. Anubis derives the card, the button label and
 * the hairlines from the palette (pagecfg.Brand), so most bad combinations fix
 * themselves — but text colour against the card is the operator's own choice,
 * and overruling that would be the builder deciding it knows better. It says
 * so instead, with the number.
 */

type Preset = {
  name: string
  brand: Pick<PageConfig['brand'], 'primary_color' | 'background_color' | 'text_color' | 'corner_radius' | 'font'>
  layout: PageConfig['layout']
  entrance: PageEntrance
}

const PRESETS: Preset[] = [
  {
    name: 'Default',
    brand: { primary_color: '#4f46e5', background_color: '#f6f6f7', text_color: '#111827', corner_radius: 'md', font: 'system' },
    layout: 'centered', entrance: 'none',
  },
  {
    name: 'Midnight',
    brand: { primary_color: '#7c6cff', background_color: '#0b0b0f', text_color: '#f4f4f6', corner_radius: 'lg', font: 'system' },
    layout: 'centered', entrance: 'fade',
  },
  {
    name: 'Paper',
    brand: { primary_color: '#8a5a2b', background_color: '#f7f3ec', text_color: '#2b2119', corner_radius: 'sm', font: 'serif' },
    layout: 'centered', entrance: 'none',
  },
  {
    name: 'Banner',
    brand: { primary_color: '#0f766e', background_color: '#f4f7f6', text_color: '#10221f', corner_radius: 'md', font: 'system' },
    layout: 'split', entrance: 'rise',
  },
  {
    name: 'Sharp',
    brand: { primary_color: '#111111', background_color: '#ffffff', text_color: '#111111', corner_radius: 'none', font: 'mono' },
    layout: 'minimal', entrance: 'none',
  },
]

export function ThemePresets({ onApply }: { onApply: (p: Preset) => void }) {
  return (
    <div className="flex flex-wrap gap-1.5">
      {PRESETS.map((p) => {
        const t = pageColors(p.brand)
        return (
          <button
            key={p.name}
            type="button"
            onClick={() => onApply(p)}
            title={`Apply ${p.name} — colours, corners, typeface and layout`}
            className="flex items-center gap-2 rounded-md px-2 py-1.5"
            style={{
              border: '1px solid var(--line)', background: 'var(--s-raised)',
              cursor: 'pointer', fontSize: 12,
            }}
          >
            <span
              aria-hidden
              style={{
                width: 26, height: 18, borderRadius: 4, flexShrink: 0,
                background: p.brand.background_color,
                border: `1px solid ${t.border}`,
                display: 'grid', placeItems: 'center',
              }}
            >
              <span style={{ width: 12, height: 6, borderRadius: 2, background: p.brand.primary_color }} />
            </span>
            {p.name}
          </button>
        )
      })}
    </div>
  )
}

/** Applies a preset to a config without touching a word of the copy, the
    links or the feature switches — those are the tenant's, not the theme's. */
export function withPreset(cfg: PageConfig, p: Preset): PageConfig {
  return {
    ...cfg,
    brand: { ...cfg.brand, ...p.brand },
    layout: p.layout,
    motion: { entrance: p.entrance },
  }
}

export function ContrastNote({ cfg }: { cfg: PageConfig }) {
  const t = pageColors(cfg.brand)
  const body = contrast(cfg.brand.text_color, t.surface)
  const brand = contrast(cfg.brand.primary_color, t.surface)

  const problems: string[] = []
  if (body < 4.5) {
    problems.push(
      `Text on the card reads at ${body.toFixed(1)}:1. Body text needs 4.5:1 — ` +
      `at this level the labels are hard work on a phone in daylight.`,
    )
  }
  if (brand < 3) {
    problems.push(
      `Links and the focus ring read at ${brand.toFixed(1)}:1 against the card. ` +
      `The button is fine — its label colour is chosen for you — but a link in ` +
      `this colour is close to invisible.`,
    )
  }
  if (problems.length === 0) return null

  return (
    <div
      className="flex items-start gap-2 rounded-md px-2.5 py-2"
      style={{ background: 'color-mix(in srgb, var(--warn) 12%, transparent)' }}
    >
      <IconAlertTriangle size={14} style={{ color: 'var(--warn)', marginTop: 1, flexShrink: 0 }} />
      <div className="t-xs flex flex-col gap-1">
        {problems.map((p) => <span key={p}>{p}</span>)}
      </div>
    </div>
  )
}
