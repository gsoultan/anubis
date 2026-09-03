import type { PageConfig, PageKind } from '@/lib/api/types'

/* Mirrors internal/auth/adapter/http/page_template.go field for field, for
 * both kinds.
 *
 * It used to render a shape of its own — theme/background/language against a
 * flat config — while the Go template drew brand.logo_url, copy.subheading and
 * features.* out of auth_pages. The builder therefore showed something the
 * hosted page would never produce, which is how a configured logo could be
 * invisible in both places at once: the console had no input for it and the
 * preview had nowhere to draw it.
 *
 * Sign-out has TWO states and both are configurable, so both are previewable.
 * The confirm step asks before ending the session; the signed-out step is what
 * remains afterwards. An operator editing confirm_body cannot otherwise tell
 * which of the two they are changing — the field names alone do not say.
 *
 * If the Go template gains a token, it gains one here too. A preview that is
 * merely plausible is worse than none, because it is believed.
 */

const RADIUS: Record<string, number> = { none: 0, sm: 4, md: 8, lg: 14, full: 999 }
const FONT: Record<string, string> = {
  system: 'ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, sans-serif',
  serif: 'ui-serif, Georgia, Cambria, Times New Roman, serif',
  mono: 'ui-monospace, SFMono-Regular, Menlo, monospace',
}

export function PagePreview({ cfg, realms, kind = 'signin', signedOut = false }: {
  cfg: PageConfig
  realms: string[]
  kind?: PageKind
  signedOut?: boolean
}) {
  const b = cfg.brand
  const c = cfg.copy
  const f = cfg.features ?? {}
  const beh = cfg.behavior ?? {}
  const radius = RADIUS[b.corner_radius] ?? 8
  const font = FONT[b.font] ?? FONT['system']
  /* Replay the entrance when it changes, so picking one shows what it does
     rather than only what it is called. */
  const entrance = cfg.motion?.entrance ?? 'none'

  const btn = (label: string) => (
    <button
      type="button"
      style={{
        width: '100%', padding: '10px 12px', borderRadius: Math.min(radius, 14),
        background: b.primary_color, color: '#fff', border: 'none',
        fontWeight: 600, fontSize: 13.5, fontFamily: font, cursor: 'default',
      }}
    >
      {label}
    </button>
  )

  const field = (label: string, type: string) => (
    <label style={{ display: 'block', marginBottom: 12 }}>
      <span style={{ display: 'block', fontSize: 12.5, color: b.text_color, opacity: 0.75, marginBottom: 5 }}>
        {label}
      </span>
      <input
        readOnly
        type={type}
        style={{
          width: '100%', padding: '9px 11px', borderRadius: Math.min(radius, 14),
          border: '1px solid rgb(0 0 0 / .14)', background: '#fff',
          color: '#111', fontSize: 13.5, fontFamily: font,
        }}
      />
    </label>
  )

  // The template shows ConfirmHeading only on the sign-out confirm step.
  const heading = kind === 'signout' && !signedOut ? c.confirm_heading : c.heading

  const signinBody = (
    <>
      {f.show_realm_picker && realms.length > 0 && (
        <div style={{ display: 'flex', gap: 6, marginBottom: 14, flexWrap: 'wrap' }}>
          {realms.map((r) => (
            <span
              key={r}
              style={{
                fontSize: 11.5, padding: '4px 9px', borderRadius: Math.min(radius, 999),
                border: `1px solid ${b.primary_color}55`, color: b.text_color, opacity: 0.85,
              }}
            >
              {r}
            </span>
          ))}
        </div>
      )}
      {field(c.username_label, 'text')}
      {field(c.password_label, 'password')}
      {f.remember_me && (
        <label style={{ display: 'flex', alignItems: 'center', gap: 7, fontSize: 12.5, color: b.text_color, opacity: 0.8, marginBottom: 12 }}>
          <input readOnly type="checkbox" /> Keep me signed in
        </label>
      )}
      {btn(c.submit_label)}
      {(f.show_forgot_password || f.show_registration) && (
        <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 12, fontSize: 12.5 }}>
          {f.show_forgot_password && <span style={{ color: b.primary_color }}>Forgot password?</span>}
          {f.show_registration && <span style={{ color: b.primary_color }}>Create account</span>}
        </div>
      )}
    </>
  )

  const signoutBody = (
    <>
      <div style={{ color: b.text_color, opacity: 0.7, fontSize: 13, marginBottom: 14 }}>
        {signedOut ? c.body : c.confirm_body}
      </div>
      {signedOut ? (
        <div style={{ fontSize: 13, color: b.primary_color }}>{c.return_label}</div>
      ) : (
        // The template hard-codes this label; it is not a config token.
        btn('Sign out')
      )}
      {signedOut && (beh.auto_redirect_seconds ?? 0) > 0 && (
        <div style={{ marginTop: 12, fontSize: 11.5, opacity: 0.6, color: b.text_color }}>
          Returns automatically after {beh.auto_redirect_seconds}s
        </div>
      )}
    </>
  )

  const card = (
    <div
      key={`${entrance}-${kind}-${signedOut}`}
      className={entrance === 'none' ? undefined : `pv-enter pv-${entrance}`}
      style={{
        background: '#fff', borderRadius: Math.min(radius, 24), padding: '26px 24px',
        width: 320, boxShadow: '0 10px 30px rgb(10 14 25 / .12)',
        border: '1px solid rgb(0 0 0 / .07)', fontFamily: font,
      }}
    >
      {/* Matches {{if .Cfg.Brand.LogoURL}} — an image when set, the initial
          otherwise, exactly as the template falls back. */}
      {b.logo_url ? (
        <img
          src={b.logo_url}
          alt={b.title}
          style={{ maxHeight: 40, maxWidth: 180, objectFit: 'contain', marginBottom: 16, display: 'block' }}
        />
      ) : (
        <div
          style={{
            width: 40, height: 40, borderRadius: Math.min(radius, 12), background: b.primary_color,
            color: '#fff', display: 'flex', alignItems: 'center', justifyContent: 'center',
            fontWeight: 700, fontSize: 17, marginBottom: 16,
          }}
        >
          {(b.title || '?').slice(0, 1).toUpperCase()}
        </div>
      )}

      <div style={{ color: b.text_color, fontSize: 19, fontWeight: 650, letterSpacing: '-.02em' }}>
        {heading}
      </div>
      {c.subheading && (
        <div style={{ color: b.text_color, opacity: 0.65, fontSize: 13, marginTop: 4 }}>
          {c.subheading}
        </div>
      )}
      <div style={{ height: 14 }} />

      {kind === 'signout' ? signoutBody : signinBody}

      {(cfg.links ?? []).length > 0 && (
        <div style={{ display: 'flex', gap: 12, marginTop: 14, fontSize: 12, opacity: 0.7, flexWrap: 'wrap' }}>
          {(cfg.links ?? []).map((l) => (
            <span key={l.label} style={{ color: b.text_color }}>{l.label}</span>
          ))}
        </div>
      )}
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

  // Matches the template's three layouts.
  if (cfg.layout === 'split') {
    return (
      <div style={{ display: 'flex', minHeight: 420, borderRadius: 10, overflow: 'hidden' }}>
        {motionCSS}
        <div style={{ flex: 1, background: b.primary_color, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 24 }}>
          <div style={{ color: '#fff', fontFamily: font, fontSize: 22, fontWeight: 650, textAlign: 'center' }}>
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
        padding: 24, borderRadius: 10,
      }}
    >
      {motionCSS}
      {card}
    </div>
  )
}
