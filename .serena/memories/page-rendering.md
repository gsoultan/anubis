# Hosted page rendering and the builder (2026-09-12)

How a configured page becomes HTML, and the traps in doing it. Resolution —
which page is chosen — is [[page-resolution]].

## The bug class this file exists for

**`html/template` silently replaces a CSS value it will not trust with the
literal string `ZgotmplZ`.** Quotes are the trigger. The font stacks carried
`'Segoe UI'` and `'Times New Roman'`, so every hosted page emitted
`font-family: ZgotmplZ` — the browser default — and two of the three typeface
tokens had never done anything. Nothing errored; the config was valid, the
template was valid, and the console preview drew its own stack so the builder
looked right. Unquoted multi-word family names are legal CSS (Fonts 3 §3.1).
`page_template_test.go` now renders every token and fails on `ZgotmplZ`.

Before that test, **nothing rendered the template at all** — it could only be
wrong in production.

## Derived colours, not more tokens

`pagecfg.Brand` configures three colours; the card, input interiors, button
label, muted text and hairlines are computed
(`internal/tenancy/domain/pagecfg/palette`, WCAG luminance, crossover 0.179).
The template used to hardcode a white card and white button text, so a dark
background gave white-on-white (an invisible form) and a light brand colour
gave white-on-yellow. Both are what happens the moment somebody pastes in
their own palette.

Colour maths is computed in Go, not CSS `color-mix()`: this is the page an old
phone has to render, and `color-mix` is Safari 16.2+.

What is NOT derived is `text_color` against the card — the operator's own
choice, so the builder warns with the ratio instead of overruling.

## `sections` — the only structure that is still tokens

An ordered list of five names (`logo heading subheading form links`); a name
omitted does not render. Closed set, no duplicates, **`form` is not
removable**. It exists so the builder has something to drag without the config
ever becoming markup, which is the line the whole model holds
(`pagecfg.go`: no HTML on the password screen, ever). Absent key = the
historical order, so old configs render unchanged.

## Mobile rules that fail silently

- `input,select,button{font:inherit}` — a control under 16px makes **iOS zoom
  the page on focus**, shoving the form off screen as somebody starts typing.
- username needs `autocapitalize="none" autocorrect="off" spellcheck="false"`
  — a phone capitalises by default and `Alice` is not the username `alice`.
- `100dvh` not `100vh` (mobile Safari chrome is inside `100vh`).
- `align-content: safe center`, or a tall card on a phone in landscape is
  clipped with its heading unreachable.

Verified by measuring the rendered page at 390×844, not by reading the CSS.

## The preview is load-bearing and kept drifting

`ui/src/components/page/PagePreview.tsx` must mirror the template block for
block. Drifts found and fixed: realm picker drawn as chips where the page
renders a `<select>`; a "Forgot password?" link nothing renders; a different
radius map (md 8 vs 12, lg 14 vs 20); no sign-out copy defaults, so the
signed-out step previewed an empty headline. A preview that is merely
plausible is worse than none, because it is believed.

## Builder facts

- `PreviewAuthPage` (→ `pagecfg.Validate`) existed, was exposed in
  `client.ts`, and **nothing called it**: the first word of an invalid config
  was a failed Save. Now debounced on every edit, and Save is held shut until
  it passes.
- Switching page or kind reloaded `cfg` from the newly selected page, which
  **silently discarded unsaved edits**. Now guarded.
- `show_forgot_password` is retired — no template drew it, no handler answered
  it. A tenant with its own reset flow puts it in `links`. Old configs
  carrying the key still parse (`Parse` ignores unknown fields), asserted by
  `TestLegacyFeatureKeyStillParses`.
- Copy limits count **runes** (`len([]rune)`), so the console counter uses
  `[...s].length`, not `.length`.

Docs: `docs/sign-in-pages.md`. See [[core]], [[page-resolution]],
[[console-tables]].
