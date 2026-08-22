import type { SignInConfig } from '@/lib/api/types'

/* The preview IS the page: one component renders the tenant sign-in from its
   config — the builder's live pane today, the served page when the Go tier
   exists. Builder and reality cannot drift because they are the same code. */

const I18N = {
  en: { username: 'Username', password: 'Password', continue: 'Continue',
        forgot: 'Forgot password?', pick: 'Sign in as' },
  id: { username: 'Nama pengguna', password: 'Kata sandi', continue: 'Lanjutkan',
        forgot: 'Lupa kata sandi?', pick: 'Masuk sebagai' },
}

export function SignInPreview({ cfg, populations }: {
  cfg: SignInConfig
  populations: string[]
}) {
  const t = I18N[cfg.language]
  const dark = cfg.theme === 'dark'
  const ink = dark ? '#edeff4' : '#1c2230'
  const sub = dark ? '#aab1bf' : '#5a6478'
  const cardBg = dark ? '#15181f' : '#ffffff'
  const line = dark ? '#262b34' : '#e3e6ec'
  const pageBg = cfg.background === 'gradient'
    ? `linear-gradient(135deg, ${cfg.brand_color}22, ${dark ? '#0b0d11' : '#f5f6f8'} 55%)`
    : dark ? '#0b0d11' : '#f5f6f8'

  const card = (
    <div style={{
      background: cardBg, border: `1px solid ${line}`, borderRadius: 12,
      padding: '28px 26px', width: 320, boxShadow: '0 10px 30px rgb(10 14 25 / .12)',
    }}>
      <div style={{
        width: 40, height: 40, borderRadius: 10, background: cfg.brand_color,
        color: '#fff', display: 'flex', alignItems: 'center', justifyContent: 'center',
        fontWeight: 700, fontSize: 17, marginBottom: 16,
      }}>
        {(cfg.logo_text || '?').slice(0, 1).toUpperCase()}
      </div>
      <div style={{ color: ink, fontSize: 19, fontWeight: 650, letterSpacing: '-.02em' }}>
        {cfg.headline}
      </div>
      <div style={{ color: sub, fontSize: 12.5, marginTop: 4, marginBottom: 18 }}>
        {cfg.subheadline}
      </div>

      {cfg.show_populations && populations.length > 1 && (
        <div style={{ marginBottom: 14 }}>
          <div style={{ color: sub, fontSize: 10, textTransform: 'uppercase',
            letterSpacing: '.06em', fontWeight: 600, marginBottom: 6 }}>{t.pick}</div>
          <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
            {populations.map((p, i) => (
              <span key={p} style={{
                fontSize: 11, padding: '4px 9px', borderRadius: 99,
                border: `1px solid ${i === 0 ? cfg.brand_color : line}`,
                color: i === 0 ? cfg.brand_color : sub,
                background: i === 0 ? `${cfg.brand_color}14` : 'transparent',
                fontWeight: i === 0 ? 600 : 450,
              }}>{p}</span>
            ))}
          </div>
        </div>
      )}

      {[t.username, t.password].map((label) => (
        <div key={label} style={{ marginBottom: 10 }}>
          <div style={{ color: sub, fontSize: 11, fontWeight: 550, marginBottom: 4 }}>{label}</div>
          <div style={{
            height: 34, borderRadius: 7, border: `1px solid ${line}`,
            background: dark ? '#0d1015' : '#f8f9fb',
          }} />
        </div>
      ))}

      <div style={{
        height: 36, borderRadius: 7, background: cfg.brand_color, color: '#fff',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        fontSize: 13, fontWeight: 620, marginTop: 14,
      }}>{t.continue}</div>

      <div style={{ color: cfg.brand_color, fontSize: 11.5, fontWeight: 550,
        marginTop: 12, textAlign: 'center' }}>{t.forgot}</div>

      <div style={{ borderTop: `1px solid ${line}`, marginTop: 16, paddingTop: 12,
        color: sub, fontSize: 10.5, textAlign: 'center' }}>{cfg.footer_note}</div>
    </div>
  )

  return (
    <div style={{
      background: pageBg, borderRadius: 10, overflow: 'hidden',
      display: 'flex', minHeight: 480,
      border: '1px solid var(--line)',
    }}>
      {cfg.layout === 'split' && (
        <div style={{
          flex: '0 0 42%',
          background: `linear-gradient(160deg, ${cfg.brand_color}, ${cfg.brand_color}88)`,
          display: 'flex', flexDirection: 'column', justifyContent: 'flex-end', padding: 26,
        }}>
          <div style={{ color: '#fff', fontSize: 21, fontWeight: 680, letterSpacing: '-.02em' }}>
            {cfg.logo_text || 'Your brand'}
          </div>
          <div style={{ color: '#ffffffcc', fontSize: 12, marginTop: 6 }}>
            Single sign-on for every application.
          </div>
        </div>
      )}
      <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 24 }}>
        {card}
      </div>
    </div>
  )
}
