import { pageColors } from '@/lib/pagePalette'
import type { PageConfig, PageKind, PageSection } from '@/lib/api/types'

/* Mirrors internal/auth/adapter/http/page_template.go block for block, for
 * both kinds.
 *
 * It used to render a shape of its own — theme/background/language against a
 * flat config — while the Go template drew brand.logo_url, copy.subheading and
 * features.* out of auth_pages. The builder therefore showed something the
 * hosted page would never produce, which is how a configured logo could be
 * invisible in both places at once.
 *
 * Two of those lies survived until the mobile pass and are gone now: the
 * preview drew the realm picker as a row of chips where the page renders a
 * <select>, and it drew a "Forgot password?" link that NOTHING renders — the
 * token exists, the hosted page has never had a password-reset flow, and a
 * preview is the worst possible place to advertise one.
 *
 * Sign-out has TWO states and both are configurable, so both are previewable.
 *
 * If the Go template gains a token, it gains one here too. A preview that is
 * merely plausible is worse than none, because it is believed.
 */

const RADIUS: Record<string, number> = { none: 0, sm: 4, md: 12, lg: 20, full: 9999 }
const FONT: Record<string, string> = {
  system: 'system-ui, -apple-system, Segoe UI, sans-serif',
  // Unquoted, exactly as pagecfg emits it — html/template refuses quotes in a
  // CSS value and the page would render in no font at all.
  serif: 'Georgia, Times New Roman, serif',
  mono: 'ui-monospace, SFMono-Regular, Menlo, monospace',
}

const DEFAULT_SECTIONS: PageSection[] = ['logo', 'heading', 'subheading', 'form', 'links']

export function PagePreview({
  cfg, realms, kind = 'signin', signedOut = false, width,
}: {
  cfg: PageConfig
  realms: string[]
  kind?: PageKind
  signedOut?: boolean
  /** Viewport width to render at. The phone width is where the hosted page's
      own media queries start behaving differently, so it has to be real.
      Undefined means "as wide as the panel", which is the desktop case. */
  width?: number | undefined
}) {
  const b = cfg.brand
  const c = cfg.copy
  const f = cfg.features ?? {}
  const beh = cfg.behavior ?? {}
  const t = pageColors(b)
  const radius = RADIUS[b.corner_radius] ?? 12
  const font = FONT[b.font] ?? FONT['system']
  const entrance = cfg.motion?.entrance ?? 'none'
  const sections = cfg.sections?.length ? cfg.sections : DEFAULT_SECTIONS
  const narrow = width !== undefined && width < 560

  // calc(var(--radius)/2) in the template — including the absurd one, because
  // an operator who picks 'Full' should see what 'Full' does.
  const ctl = radius / 2

  const btn = (label: string) => (
    <button
      type="button"
      style={{
        width: '100%', marginTop: 14, padding: '11px 14px', borderRadius: ctl,
        background: b.primary_color, color: t.onPrimary, border: 'none',
        fontWeight: 600, fontSize: 14, fontFamily: font, cursor: 'default',
      }}
    >
      {label}
    </button>
  )

  const field = (label: string, type: string) => (
    <label style={{ display: 'block', marginTop: 12 }}>
      <span style={{ display: 'block', fontSize: 12.5, color: b.text_color, marginBottom: 5 }}>
        {label}
      </span>
      <input
        readOnly
        type={type}
        style={{
          width: '100%', padding: '10px 11px', borderRadius: ctl,
          border: `1px solid ${t.border}`, background: t.field,
          color: b.text_color, fontSize: 13.5, fontFamily: font,
        }}
      />
    </label>
  )

  const heading = kind === 'signout' && !signedOut ? c.confirm_heading : c.heading

  /* The template renders a <select>, so this renders a <select>. The chips it
     used to draw were prettier and were not what anybody would see. */
  const realmPicker = f.show_realm_picker && realms.length > 0 && (
    <label style={{ display: 'block' }}>
      <span style={{ display: 'block', fontSize: 12.5, color: b.text_color, marginBottom: 5 }}>
        Directory
      </span>
      <select
        disabled
        style={{
          width: '100%', padding: '10px 11px', borderRadius: ctl,
          border: `1px solid ${t.border}`, background: t.field,
          color: b.text_color, fontSize: 13.5, fontFamily: font,
        }}
      >
        {realms.map((r) => <option key={r}>{r}</option>)}
      </select>
    </label>
  )

  const signinBody = (
    <>
      {realmPicker}
      {field(c.username_label, 'text')}
      {field(c.password_label, 'password')}
      {f.remember_me && (
        <label style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 12.5, color: b.text_color, marginTop: 12 }}>
          <input readOnly type="checkbox" style={{ accentColor: b.primary_color }} /> Keep me signed in
        </label>
      )}
      {btn(c.submit_label)}
    </>
  )

  const signoutBody = (
    <>
      <div style={{ color: t.muted, fontSize: 13, marginBottom: 4, lineHeight: 1.45 }}>
        {signedOut ? c.body : c.confirm_body}
      </div>
      {signedOut ? (
        <div style={{ fontSize: 13, color: b.primary_color, marginTop: 10 }}>{c.return_label}</div>
      ) : (
        // The template hard-codes this label; it is not a config token.
        btn('Sign out')
      )}
      {signedOut && (beh.auto_redirect_seconds ?? 0) > 0 && (
        <div style={{ marginTop: 12, fontSize: 11.5, color: t.muted }}>
          Returns automatically after {beh.auto_redirect_seconds}s
        </div>
      )}
    </>
  )

  /* One entry per section the template knows. Rendering from the same list the
     server iterates is what keeps a dragged order honest. */
  const block = (s: PageSection) => {
    switch (s) {
      case 'logo':
        return b.logo_url ? (
          <img
            key={s} src={b.logo_url} alt={b.title}
            style={{ maxHeight: 44, maxWidth: '70%', objectFit: 'contain', marginBottom: 14, display: 'block' }}
          />
        ) : (
          <div
            key={s}
            style={{
              width: 44, height: 44, borderRadius: ctl, background: b.primary_color,
              color: t.onPrimary, display: 'flex', alignItems: 'center',
              justifyContent: 'center', fontWeight: 700, fontSize: 18, marginBottom: 14,
            }}
          >
            {(b.title || '?').slice(0, 1).toUpperCase()}
          </div>
        )
      case 'heading':
        return (
          <div key={s} style={{ color: b.text_color, fontSize: 19, fontWeight: 650, letterSpacing: '-.02em', lineHeight: 1.25 }}>
            {heading}
          </div>
        )
      case 'subheading':
        return c.subheading ? (
          <div key={s} style={{ color: t.muted, fontSize: 13, marginTop: 5, lineHeight: 1.45 }}>
            {c.subheading}
          </div>
        ) : null
      case 'form':
        return <div key={s} style={{ marginTop: 12 }}>{kind === 'signout' ? signoutBody : signinBody}</div>
      case 'links': {
        const links = cfg.links ?? []
        if (links.length === 0 && !f.show_registration) return null
        return (
          <div key={s} style={{ display: 'flex', gap: '4px 14px', marginTop: 14, fontSize: 12.5, flexWrap: 'wrap' }}>
            {links.map((l, i) => (
              <span key={`${l.label}-${i}`} style={{ color: b.primary_color }}>{l.label}</span>
            ))}
            {f.show_registration && <span style={{ color: b.primary_color }}>Create an account</span>}
          </div>
        )
      }
      default:
        return null
    }
  }

  const card = (
    <div
      key={`${entrance}-${kind}-${signedOut}`}
      className={entrance === 'none' ? undefined : `pv-enter pv-${entrance}`}
      style={{
        background: t.surface,
        borderRadius: radius,
        padding: narrow ? 20 : 32,
        width: 'min(100%, 26rem)',
        boxShadow: t.dark ? '0 8px 30px rgb(0 0 0 / .35)' : '0 10px 30px rgb(10 14 25 / .12)',
        border: `1px solid ${t.dark ? t.border : 'rgb(0 0 0 / .07)'}`,
        fontFamily: font,
      }}
    >
      {sections.map(block)}
    </div>
  )

  const motionCSS = (
    <style>{`
      @media (prefers-reduced-motion: no-preference) {
        .pv-enter { animation: pv-enter .2s ease-out both }
        @keyframes pv-enter { from { opacity: 0; transform: var(--pv-shift, none) } to { opacity: 1; transform: none } }
        .pv-rise { --pv-shift: translateY(8px) }
      }
    `}</style>
  )

  /* The template collapses split to one column at 820px, so the preview does
     too — otherwise the phone view would show a layout no phone renders. */
  if (cfg.layout === 'split' && !(width !== undefined && width <= 820)) {
    return (
      <div style={{ display: 'flex', minHeight: 420, borderRadius: 10, overflow: 'hidden' }}>
        {motionCSS}
        <div style={{ flex: 1, background: b.primary_color, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 24 }}>
          <div style={{ color: t.onPrimary, fontFamily: font, fontSize: 22, fontWeight: 650, textAlign: 'center' }}>
            {b.title}
          </div>
        </div>
        <div style={{ flex: 1, background: b.background_color, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 24 }}>
          {card}
        </div>
      </div>
    )
  }

  return (
    <div
      style={{
        background: cfg.layout === 'minimal' ? 'transparent' : b.background_color,
        minHeight: 420, display: 'flex', alignItems: 'center', justifyContent: 'center',
        padding: narrow ? 16 : 24, borderRadius: 10,
      }}
    >
      {motionCSS}
      {card}
    </div>
  )
}
