# Sign-in and sign-out pages

The screens Anubis serves on its own origin, at
`/p/{tenant}/{kind}/{slug}` — the ones where people type their password.
Endpoints and payloads are in [api.md](api.md#sign-in-and-sign-out-pages);
this is what the configuration means, what Anubis works out for itself, and
what the pages do on a phone.

Operators edit them in the console under **Sign-in & sign-out**.

---

## The rule the whole model is built on

**Configuration, never markup.** Every field is an enum, a bounded string, a
validated `#rgb`/`#rrggbb` colour or an `http(s)` URL. There is no
`custom_html`, no `custom_css`, and there will not be one.

This is not tidiness. These pages are served from Anubis's own origin on the
screen where users type their password, so a markup field would hand every
tenant admin stored XSS against their own users, and one broken template would
take sign-in down for a tenant. Text is escaped and rendered: `<script>` in a
heading appears as the characters `<script>`.

Everything below — including the drag-and-drop layout — stays inside that
rule. The builder can reorder blocks because the order is *a list of five
names*, not a document.

---

## Which page renders

Most specific match wins:

```
?page=<slug>  →  the application's page  →  the population's page  →  the tenant default
```

Exactly one page per kind is the tenant default, enforced by a partial unique
index; it cannot be deleted or disabled without promoting another first. A
page that is missing or disabled **falls through** rather than failing the
flow — losing branding must never cost somebody the ability to sign in.

---

## The token set

### Brand

| Field | Values |
| :--- | :--- |
| `title` | ≤60 chars. Used as the document title, the split banner, and the source of the fallback initial |
| `logo_url` | `https://` image. Omit it and the page draws a brand-coloured square with the first letter of `title` |
| `primary_color` | `#rgb` or `#rrggbb` |
| `background_color` | The page behind the card |
| `text_color` | Text on the card |
| `corner_radius` | `none` · `sm` · `md` · `lg` · `full` |
| `font` | `system` · `serif` · `mono` |

### Layout and blocks

`layout` is `centered`, `split` or `minimal`. Adding one is a code change —
the layout picks a stylesheet, never a template a tenant supplied.

> **`split` collapses below 820px.** The banner half is hidden and the card
> centres, so on a phone it renders as `centered`. Check the phone preview
> before assuming a visitor sees the banner.

`sections` is the order of the blocks, and by omission which of them render:

```jsonc
"sections": ["logo", "heading", "subheading", "form", "links"]   // the default
"sections": ["heading", "subheading", "form", "links"]           // no logo
"sections": ["heading", "form", "logo"]                          // logo at the bottom
```

Rules the server enforces: every name must be one of the five, no name twice,
and **`form` cannot be removed** — a sign-in page without one is a door with
no handle, and that is one stray drag away. A config with no `sections` key
renders in the default order, so pages written before this existed are
untouched.

`subheading` and `links` render only when they have content, whatever the
order says.

### Copy

| Field | Limit | Kind |
| :--- | :--- | :--- |
| `heading` | 120 | both — on sign-out it is the *signed-out* headline |
| `subheading` | 240 | both |
| `username_label` · `password_label` · `submit_label` | 60 | sign-in |
| `confirm_heading` | 120 | sign-out, the asking step |
| `confirm_body` | 500 | sign-out, the asking step |
| `body` | 500 | sign-out, after the session ends |
| `return_label` | 60 | sign-out |

Limits count **runes**, not bytes. Control characters are refused; angle
brackets and ampersands are not, because "Smith & Co" is a company.

### Links

Up to five `{label, url}`, `http(s)` only, rendered under the form in the
order given. This is also where a tenant puts a password-reset link — see
*What is deliberately missing*.

### Features (sign-in)

| Field | Effect |
| :--- | :--- |
| `show_realm_picker` | Offers a population chooser. **Only populations that accept a password are listed** — advertising a directory that cannot answer a password is worse than not asking |
| `show_registration` | Adds "Create an account", **only where that population allows self-registration** |
| `remember_me` | Adds the longer-session checkbox |

Both visibility flags are intersected with real policy at render time. A
switch being on does not make a door exist.

### Behaviour (sign-out)

| Field | Effect |
| :--- | :--- |
| `confirm` | Ask before ending the session. Off is only safe where the link cannot be triggered by somebody else — a bare `GET` that ends sessions is reachable from any `<img>`. See the note below: it governs `/v1/logout`, not this page's own URL |
| `auto_redirect_seconds` | 0 disables; the server caps it at 30 |
| `default_return_url` | Used when the application supplies none. `post_logout_redirect_uri` is still exact-matched against the application's own allowlist |

> **What `confirm` actually switches.** With it off, an application-initiated
> sign-out — `GET /v1/logout`, the endpoint an RP links to — ends the session
> immediately and renders the signed-out step. The page's **own** URL,
> `/p/{tenant}/signout/{slug}`, still renders the asking step either way,
> because a `GET` must never end a session. `confirm` also gates CSRF
> enforcement on the `POST`. Both steps are therefore reachable in production
> and both are worth previewing.

### Motion

`entrance` is `none`, `fade` or `rise`, and only ever animates opacity and
transform. It is wrapped in `prefers-reduced-motion: no-preference`, so a
visitor who asked for less motion gets none — and the card is interactive
throughout, because decoration must never be the reason somebody cannot start
typing.

---

## What Anubis derives

Three colours are configured. The rest are computed from them, and are not
settable:

| Derived | Rule |
| :--- | :--- |
| The card | White on a light page; a lift out of the background on a dark one |
| Input interiors | White on a light page, a shade above the background on a dark one |
| Button label | Black or white, whichever contrasts with `primary_color` |
| Muted text, hairlines | `text_color` faded toward the card |

Why derive instead of asking: a dark `background_color` with the light
`text_color` it needs used to produce white text on a hardcoded white card —
an invisible form — and a light `primary_color` produced white-on-yellow
button text. Both are the first thing anyone hits after pasting in their own
palette, and neither is a matter of taste. See
`internal/tenancy/domain/pagecfg/palette`.

The one thing **not** derived is `text_color` against the card. That is the
operator's own choice, so the builder shows the WCAG contrast ratio and warns
below 4.5:1 rather than overruling it.

---

## On a phone

Most people who see these pages are holding one. The page:

- gives form controls the page font, so they render at 16px. **iOS zooms the
  page whenever a focused input is under 16px**, which shoves half the form
  off screen at the moment somebody starts typing.
- turns off autocapitalise, autocorrect and spellcheck on the username. A
  phone keyboard capitalises by default, and `Alice` is not the username
  `alice` — it is a failed sign-in nobody can explain.
- sizes to `100dvh`, not `100vh`: on mobile Safari `100vh` is taller than the
  visible viewport, so a centred card sits partly under the browser chrome.
- centres with `align-content: safe center`, so a tall card on a short screen
  (any phone in landscape) scrolls from the top instead of being clipped with
  its heading unreachable.
- pads to the safe area, so a notch in landscape does not cover the form.
- gives every control a 44px touch target.

`page_template_test.go` renders the template and asserts the ones that fail
silently.

---

## Security posture

| Header | Value |
| :--- | :--- |
| `Content-Security-Policy` | `default-src 'none'; img-src https: data:; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'` |
| `X-Frame-Options` | `DENY` — clickjacking a login form is how consent gets stolen |
| `Cache-Control` | `no-store` |
| `Referrer-Policy` | `no-referrer` |

The stylesheet is inline and generated from tokens, which is why
`style-src 'unsafe-inline'` is present; scripts are forbidden outright because
these pages have none.

Sign-out is `GET` to ask and `POST` to act, with a CSRF token that rotates on
every render.

> **A note for anyone adding a CSS token.** `html/template` refuses a quote
> inside a CSS value and silently substitutes the string `ZgotmplZ`. The font
> stacks carried quoted family names for as long as the feature existed, so
> every hosted page rendered `font-family: ZgotmplZ` — the browser default —
> and two of the three typeface choices did nothing. Nothing failed; it just
> was not the font. Keep emitted CSS values quote-free.

---

## The builder

Three tabs, one preview.

- **Design** — presets, the three colours with a live contrast warning,
  corners, typeface, layout, entrance.
- **Content** — the block list (drag, or focus a handle and use the arrow
  keys), the copy with live counters, and the links editor.
- **Features / Behaviour** — the switches for that kind.

The preview defaults to **phone**, at 390 CSS pixels, because that is where
the page's own media queries change their mind. It shows the real populations
this tenant has, not invented ones, and for sign-out it previews both steps,
which are both reachable whatever `confirm` says.

Edits are checked against the server as you type (`PreviewAuthPage` runs the
same `pagecfg.Validate` the save runs) and **Save stays shut until it
passes**. Switching page or kind with unsaved edits asks first.

---

## What is deliberately missing

**There is no password-reset flow, and therefore no reset link.** A
`show_forgot_password` flag existed and rendered nowhere: no template drew it,
no handler answered it, and the console showed a switch that did nothing next
to a preview that drew the link. It has been removed. A tenant that runs its
own reset puts it in `links`, where it is a label and a URL and renders
exactly where they expect. Configs still carrying the old key parse fine — it
is ignored.

**There is no custom CSS or HTML**, for the reason at the top of this page.

**Layouts are code.** If `centered`, `split` and `minimal` do not cover it,
the answer is a fourth layout in the template, reviewed like anything else
that renders on the password screen.
