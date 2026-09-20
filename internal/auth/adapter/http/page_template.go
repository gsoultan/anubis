package authhttp

// pageTemplate renders both kinds. Every interpolation is escaped by
// html/template; the only values reaching CSS are validated colours and
// mapped tokens (see pagecfg.Brand.RadiusCSS / FontCSS / SurfaceCSS).
//
// This page is read on a phone more often than anywhere else — it is the one
// screen between a person and the thing they came to use — so the mobile
// rules here are not polish:
//
//   - form controls inherit the page font. A control under 16px makes iOS
//     ZOOM THE PAGE when it takes focus, which shoves half the form off
//     screen at the exact moment somebody starts typing.
//   - the username field turns off autocapitalise, autocorrect and spellcheck.
//     A phone keyboard capitalises the first letter by default, and "Alice"
//     is not the username "alice" — it is a failed sign-in nobody can explain.
//   - 100dvh, not 100vh: on mobile Safari 100vh is taller than the visible
//     viewport, so a centred card sits partly under the browser chrome.
//   - `align-content: safe center` so a tall card on a short screen (any
//     phone in landscape) scrolls from the top instead of being clipped with
//     its heading unreachable above the fold.
//   - safe-area padding, so a notch in landscape does not cover the form.
const pageTemplate = `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover">
<meta name="theme-color" content="{{.Cfg.Brand.BackgroundColor}}">
<title>{{.Cfg.Brand.Title}}</title>
{{if and (eq .Kind "signout") .SignedOut (gt .AutoRedirectSeconds 0) .ReturnURL}}
<meta http-equiv="refresh" content="{{.AutoRedirectSeconds}};url={{.ReturnURL}}">
{{end}}
<style>
:root{
  --brand:{{.Cfg.Brand.PrimaryColor}};
  --on-brand:{{.Cfg.Brand.OnPrimaryCSS}};
  --bg:{{.Cfg.Brand.BackgroundColor}};
  --fg:{{.Cfg.Brand.TextColor}};
  --muted:{{.Cfg.Brand.MutedCSS}};
  --surface:{{.Cfg.Brand.SurfaceCSS}};
  --field:{{.Cfg.Brand.FieldCSS}};
  --line:{{.Cfg.Brand.BorderCSS}};
  --radius:{{.Cfg.Brand.RadiusCSS}};
  --font:{{.Cfg.Brand.FontCSS}};
}
*{box-sizing:border-box}
html{-webkit-text-size-adjust:100%}
body{font-family:var(--font);font-size:16px;background:var(--bg);color:var(--fg);
     margin:0;min-height:100vh;min-height:100dvh;
     display:grid;justify-items:center;
     align-content:center;align-content:safe center;
     padding:clamp(1rem,4vh,2.5rem) 1rem;
     padding-left:max(1rem,env(safe-area-inset-left));
     padding-right:max(1rem,env(safe-area-inset-right));
     padding-bottom:max(clamp(1rem,4vh,2.5rem),env(safe-area-inset-bottom))}
.wrap{width:min(100%,26rem)}
.card{background:var(--surface);padding:clamp(1.25rem,5vw,2rem);
      border-radius:var(--radius);box-shadow:0 4px 24px rgba(0,0,0,.08)}
{{if .Cfg.Brand.DarkTheme}}
/* A drop shadow is invisible on a dark page, so the card needs an edge to be
   a card at all. */
.card{box-shadow:0 8px 30px rgba(0,0,0,.35);border:1px solid var(--line)}
{{end}}
.logo{max-height:44px;max-width:70%;margin-bottom:1rem}
.mark{width:44px;height:44px;margin-bottom:1rem;border-radius:calc(var(--radius)/2);
      background:var(--brand);color:var(--on-brand);display:grid;place-items:center;
      font-size:1.25rem;font-weight:700}
h1{font-size:clamp(1.2rem,4.5vw,1.35rem);margin:0 0 .35rem;line-height:1.25}
.sub{color:var(--muted);font-size:.92rem;margin:0 0 1rem;line-height:1.45}
label{display:block;font-size:.9rem;margin:.9rem 0 .3rem}
/* font:inherit is the iOS zoom fix; min-height is the 44px touch target. */
input,select,button{font:inherit}
input,select{width:100%;padding:.7rem .75rem;min-height:2.75rem;
     border:1px solid var(--line);border-radius:calc(var(--radius)/2);
     background:var(--field);color:var(--fg)}
input{-webkit-appearance:none;appearance:none}
input:focus-visible,select:focus-visible,button:focus-visible,a:focus-visible{
     outline:2px solid var(--brand);outline-offset:2px}
button{width:100%;margin-top:1.25rem;padding:.8rem 1rem;min-height:2.75rem;border:0;
       border-radius:calc(var(--radius)/2);background:var(--brand);color:var(--on-brand);
       font-weight:600;cursor:pointer;-webkit-appearance:none;
       /* kills the 300ms tap delay older mobile browsers add */
       touch-action:manipulation}
button:active{filter:brightness(.94)}
.links{margin-top:1rem;display:flex;gap:.4rem 1.1rem;flex-wrap:wrap;font-size:.9rem}
.links a{color:var(--brand);display:inline-flex;align-items:center;min-height:2.25rem}
.err{margin-top:.85rem;padding:.7rem .8rem;border-radius:calc(var(--radius)/2);
     background:#fdecec;color:#b00020;font-size:.9rem;line-height:1.4}
.check{display:flex;align-items:center;gap:.6rem;margin-top:.85rem;
       font-size:.92rem;min-height:2.75rem}
/* Overrides the field sizing above: a checkbox is not a text input. */
.check input{width:1.15rem;height:1.15rem;min-height:0;padding:0;flex:none;
       accent-color:var(--brand)}
.check label{margin:0}
{{if .Cfg.Motion.Animated}}
/* Only for people who have not asked for less motion — and only ever opacity
   and transform, so it composites rather than causing layout. The card stays
   interactive throughout: this is decoration, and it must never be the reason
   somebody cannot start typing. */
@media (prefers-reduced-motion: no-preference){
  .card{animation:enter .2s ease-out both}
  {{if eq .Cfg.Motion.Entrance "fade"}}
  @keyframes enter{from{opacity:0}to{opacity:1}}
  {{end}}
  {{if eq .Cfg.Motion.Entrance "rise"}}
  @keyframes enter{from{opacity:0;transform:translateY(8px)}to{opacity:1;transform:none}}
  {{end}}
}
{{end}}
{{if eq .Cfg.Layout "split"}}
body{padding:0;align-content:stretch;justify-items:stretch}
.wrap{width:100%;max-width:none;display:grid;grid-template-columns:1fr 1fr;
      min-height:100vh;min-height:100dvh}
.hero{background:var(--brand);display:grid;place-items:center;
      color:var(--on-brand);padding:2rem;text-align:center}
.pane{display:grid;justify-items:center;align-content:center;align-content:safe center;
      padding:clamp(1.25rem,5vw,2rem);
      padding-left:max(1.25rem,env(safe-area-inset-left));
      padding-right:max(1.25rem,env(safe-area-inset-right));
      padding-bottom:max(clamp(1.25rem,5vw,2rem),env(safe-area-inset-bottom))}
.card{width:min(100%,24rem);box-shadow:none}
/* Below this the two columns cannot both be read, and the half that has to go
   is the decorative one. */
@media (max-width:820px){.wrap{grid-template-columns:1fr}.hero{display:none}}
{{end}}
{{if eq .Cfg.Layout "minimal"}}
.card{box-shadow:none;background:transparent;border:0;padding:1rem 0}
{{end}}
</style></head><body>
<div class="wrap">
{{if eq .Cfg.Layout "split"}}<div class="hero"><strong>{{.Cfg.Brand.Title}}</strong></div><div class="pane">{{end}}
<div class="card">
{{/* Blocks render in the order the config lists them, and a block the config
     omits does not render at all. The set is closed (pagecfg.Sections) and the
     form is not removable, so the worst a wrong drag can do is move something. */}}
{{range .Cfg.Sections}}

{{if eq . "logo"}}
  {{if $.Cfg.Brand.LogoURL}}
    <img class="logo" src="{{$.Cfg.Brand.LogoURL}}" alt="{{$.Cfg.Brand.Title}}">
  {{else}}
    <div class="mark" aria-hidden="true">{{$.Cfg.Brand.Initial}}</div>
  {{end}}
{{end}}

{{if eq . "heading"}}
  {{if and (eq $.Kind "signout") (not $.SignedOut)}}
    <h1>{{$.Cfg.Copy.ConfirmHeading}}</h1>
  {{else}}
    <h1>{{$.Cfg.Copy.Heading}}</h1>
  {{end}}
{{end}}

{{if eq . "subheading"}}
  {{if $.Cfg.Copy.Subheading}}<p class="sub">{{$.Cfg.Copy.Subheading}}</p>{{end}}
{{end}}

{{if eq . "form"}}
{{if eq $.Kind "signin"}}
  <form method="post" action="/v1/login">
    <input type="hidden" name="tenant" value="{{$.Tenant}}">
    <input type="hidden" name="client_id" value="{{$.ClientID}}">
    <input type="hidden" name="redirect_uri" value="{{$.RedirectURI}}">
    <input type="hidden" name="state" value="{{$.State}}">
    <input type="hidden" name="code_challenge" value="{{$.Challenge}}">
    <input type="hidden" name="code_challenge_method" value="{{$.Method}}">
    <input type="hidden" name="nonce" value="{{$.Nonce}}">
    {{/* Proves this submission came from a page we rendered. Both branches
         below carry it: a cross-site form holds no page state, which is the
         whole check. */}}
    <input type="hidden" name="csrf" value="{{$.LoginCSRF}}">
    {{if $.MFAToken}}
    {{/* Second factor. The password already checked out, and this form
         carries a single-use token standing for that — so the password is
         not asked for again, and nothing on this page can replay it. */}}
    <input type="hidden" name="mfa_token" value="{{$.MFAToken}}">
    <input type="hidden" name="realm" value="{{$.Realm}}">
    <label for="code">Authentication code</label>
    <input id="code" name="code" inputmode="numeric" autocomplete="one-time-code"
           pattern="[0-9]*" maxlength="8" required autofocus
           autocapitalize="none" autocorrect="off" spellcheck="false" enterkeyhint="go">
    {{if $.Error}}<div class="err">{{$.Error}}</div>{{end}}
    <button type="submit">{{$.Cfg.Copy.SubmitLabel}}</button>
    {{else}}
    {{if and $.Cfg.Features.ShowRealmPicker $.Realms}}
      <label for="realm">Directory</label>
      <select id="realm" name="realm">
        {{range $.Realms}}<option value="{{.Code}}"{{if eq .Code $.Realm}} selected{{end}}>{{.Name}}</option>{{end}}
      </select>
    {{else}}
      <input type="hidden" name="realm" value="{{$.Realm}}">
    {{end}}
    <label for="u">{{$.Cfg.Copy.UsernameLabel}}</label>
    {{/* A phone capitalises and autocorrects by default, and a username is
         neither a sentence nor a word it knows. */}}
    <input id="u" name="username" autocomplete="username" required autofocus
           autocapitalize="none" autocorrect="off" spellcheck="false" enterkeyhint="next">
    <label for="p">{{$.Cfg.Copy.PasswordLabel}}</label>
    <input id="p" name="password" type="password" autocomplete="current-password"
           required enterkeyhint="go">
    {{if $.Cfg.Features.RememberMe}}
      <div class="check"><input id="r" name="remember" type="checkbox" value="1">
        <label for="r">Keep me signed in</label></div>
    {{end}}
    {{if $.Error}}<div class="err">{{$.Error}}</div>{{end}}
    <button type="submit">{{$.Cfg.Copy.SubmitLabel}}</button>
    {{end}}
  </form>
{{else}}
  {{if $.SignedOut}}
    <p class="sub">{{$.Cfg.Copy.Body}}</p>
    {{if $.ReturnURL}}<p class="links"><a href="{{$.ReturnURL}}">{{$.Cfg.Copy.ReturnLabel}}</a></p>{{end}}
  {{else}}
    <p class="sub">{{$.Cfg.Copy.ConfirmBody}}</p>
    <form method="post" action="/v1/logout">
      <input type="hidden" name="tenant" value="{{$.Tenant}}">
      <input type="hidden" name="post_logout_redirect_uri" value="{{$.ReturnURL}}">
      <input type="hidden" name="state" value="{{$.State}}">
      <input type="hidden" name="csrf" value="{{$.LogoutCSRF}}">
      {{if $.Error}}<div class="err">{{$.Error}}</div>{{end}}
      <button type="submit">Sign out</button>
    </form>
  {{end}}
{{end}}
{{end}}

{{if eq . "links"}}
  {{if or $.Cfg.Links $.RegistrationURL}}
  <div class="links">
    {{range $.Cfg.Links}}<a href="{{.URL}}" rel="noopener noreferrer">{{.Label}}</a>{{end}}
    {{if $.RegistrationURL}}<a href="{{$.RegistrationURL}}">Create an account</a>{{end}}
  </div>
  {{end}}
{{end}}

{{end}}
</div>
{{if eq .Cfg.Layout "split"}}</div>{{end}}
</div></body></html>`
