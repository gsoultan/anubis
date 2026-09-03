package authhttp

// pageTemplate renders both kinds. Every interpolation is escaped by
// html/template; the only values reaching CSS are validated colours and
// mapped tokens (see pagecfg.Brand.RadiusCSS / FontCSS).
const pageTemplate = `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Cfg.Brand.Title}}</title>
{{if and (eq .Kind "signout") .SignedOut (gt .AutoRedirectSeconds 0) .ReturnURL}}
<meta http-equiv="refresh" content="{{.AutoRedirectSeconds}};url={{.ReturnURL}}">
{{end}}
<style>
:root{
  --brand:{{.Cfg.Brand.PrimaryColor}};
  --bg:{{.Cfg.Brand.BackgroundColor}};
  --fg:{{.Cfg.Brand.TextColor}};
  --radius:{{.Cfg.Brand.RadiusCSS}};
  --font:{{.Cfg.Brand.FontCSS}};
}
*{box-sizing:border-box}
body{font-family:var(--font);background:var(--bg);color:var(--fg);margin:0;
     min-height:100vh;display:grid;place-items:center}
.wrap{width:min(92vw,26rem)}
.card{background:#fff;padding:2rem;border-radius:var(--radius);
      box-shadow:0 4px 24px rgba(0,0,0,.08)}
.logo{max-height:44px;margin-bottom:1rem}
h1{font-size:1.3rem;margin:0 0 .35rem}
.sub{color:#555;font-size:.9rem;margin:0 0 1rem}
label{display:block;font-size:.85rem;margin:.85rem 0 .25rem}
input,select{width:100%;padding:.65rem;border:1px solid #ccc;border-radius:calc(var(--radius)/2)}
button{width:100%;margin-top:1.25rem;padding:.75rem;border:0;
       border-radius:calc(var(--radius)/2);background:var(--brand);color:#fff;
       font-weight:600;font-size:1rem;cursor:pointer}
.links{margin-top:1rem;display:flex;gap:1rem;flex-wrap:wrap;font-size:.85rem}
.links a{color:var(--brand)}
.err{margin-top:.85rem;padding:.6rem .75rem;border-radius:calc(var(--radius)/2);
     background:#fdecec;color:#b00020;font-size:.87rem}
.check{display:flex;align-items:center;gap:.5rem;margin-top:.85rem;font-size:.88rem}
.check input{width:auto}
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
body{place-items:stretch}
.wrap{width:100%;max-width:none;display:grid;grid-template-columns:1fr 1fr}
.hero{background:var(--brand);display:grid;place-items:center;color:#fff;padding:2rem}
.pane{display:grid;place-items:center;padding:2rem}
.card{width:min(90%,24rem);box-shadow:none}
@media (max-width:820px){.wrap{grid-template-columns:1fr}.hero{display:none}}
{{end}}
{{if eq .Cfg.Layout "minimal"}}
.card{box-shadow:none;background:transparent;padding:1rem}
{{end}}
</style></head><body>
<div class="wrap">
{{if eq .Cfg.Layout "split"}}<div class="hero"><strong>{{.Cfg.Brand.Title}}</strong></div><div class="pane">{{end}}
<div class="card">
  {{if .Cfg.Brand.LogoURL}}<img class="logo" src="{{.Cfg.Brand.LogoURL}}" alt="{{.Cfg.Brand.Title}}">{{end}}
  {{if and (eq .Kind "signout") (not .SignedOut)}}
    <h1>{{.Cfg.Copy.ConfirmHeading}}</h1>
  {{else}}
    <h1>{{.Cfg.Copy.Heading}}</h1>
  {{end}}
  {{if .Cfg.Copy.Subheading}}<p class="sub">{{.Cfg.Copy.Subheading}}</p>{{end}}

{{if eq .Kind "signin"}}
  <form method="post" action="/v1/login">
    <input type="hidden" name="tenant" value="{{.Tenant}}">
    <input type="hidden" name="client_id" value="{{.ClientID}}">
    <input type="hidden" name="redirect_uri" value="{{.RedirectURI}}">
    <input type="hidden" name="state" value="{{.State}}">
    <input type="hidden" name="code_challenge" value="{{.Challenge}}">
    <input type="hidden" name="code_challenge_method" value="{{.Method}}">
    <input type="hidden" name="nonce" value="{{.Nonce}}">
    {{if and .Cfg.Features.ShowRealmPicker .Realms}}
      <label for="realm">Directory</label>
      <select id="realm" name="realm">
        {{range .Realms}}<option value="{{.Code}}"{{if eq .Code $.Realm}} selected{{end}}>{{.Name}}</option>{{end}}
      </select>
    {{else}}
      <input type="hidden" name="realm" value="{{.Realm}}">
    {{end}}
    <label for="u">{{.Cfg.Copy.UsernameLabel}}</label>
    <input id="u" name="username" autocomplete="username" required autofocus>
    <label for="p">{{.Cfg.Copy.PasswordLabel}}</label>
    <input id="p" name="password" type="password" autocomplete="current-password" required>
    {{if .Cfg.Features.RememberMe}}
      <div class="check"><input id="r" name="remember" type="checkbox" value="1">
        <label for="r" style="margin:0">Keep me signed in</label></div>
    {{end}}
    {{if .Error}}<div class="err">{{.Error}}</div>{{end}}
    <button type="submit">{{.Cfg.Copy.SubmitLabel}}</button>
  </form>
{{else}}
  {{if .SignedOut}}
    <p class="sub">{{.Cfg.Copy.Body}}</p>
    {{if .ReturnURL}}<p class="links"><a href="{{.ReturnURL}}">{{.Cfg.Copy.ReturnLabel}}</a></p>{{end}}
  {{else}}
    <p class="sub">{{.Cfg.Copy.ConfirmBody}}</p>
    <form method="post" action="/v1/logout">
      <input type="hidden" name="tenant" value="{{.Tenant}}">
      <input type="hidden" name="post_logout_redirect_uri" value="{{.ReturnURL}}">
      <input type="hidden" name="state" value="{{.State}}">
      <input type="hidden" name="csrf" value="{{.LogoutCSRF}}">
      {{if .Error}}<div class="err">{{.Error}}</div>{{end}}
      <button type="submit">Sign out</button>
    </form>
  {{end}}
{{end}}

  {{if .Cfg.Links}}
  <div class="links">
    {{range .Cfg.Links}}<a href="{{.URL}}" rel="noopener noreferrer">{{.Label}}</a>{{end}}
    {{if $.RegistrationURL}}<a href="{{$.RegistrationURL}}">Create an account</a>{{end}}
  </div>
  {{else if .RegistrationURL}}
  <div class="links"><a href="{{.RegistrationURL}}">Create an account</a></div>
  {{end}}
</div>
{{if eq .Cfg.Layout "split"}}</div>{{end}}
</div></body></html>`
