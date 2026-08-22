# Anubis Console

Operator console for the Anubis SSO/IAM service.

**Stack** — React 19.2 · TypeScript 7.0 · Mantine 9.5 · Tailwind 4.3 ·
TanStack Router / Query / Form · Zustand 5 · **Bun 1.3**

```bash
bun install
bun run dev          # → ../scripts/ui.sh, http://localhost:7447
bun run build        # tsr generate && tsc --noEmit && native Bun bundler
bun run build:vite   # Vite fallback — see "Which bundler" below
```

Anubis is a platform, not a single app, so **dev orchestration lives at the
repository root** in `scripts/` — `scripts/dev.sh` brings up the database, the
API and this console together, and `scripts/lib/common.sh` is the single place
ports are defined. `bun run dev` here is a thin alias to `scripts/ui.sh` for
working on the console alone.

### The port

**7447**, chosen deliberately. Avoided: 3000/3001 (Node, CRA), 4200 (Angular),
5000 (also macOS AirPlay Receiver), 5173 (Vite's default), 8000/8080/8888 — plus
everything already listening on this machine: 5432 Postgres, 5672/15672
RabbitMQ, 6379 Redis, 7000, 9000/9001 MinIO.

**7448** is the Go API and **7449** the database — one contiguous block, defined
once in `scripts/lib/common.sh`. `/v1` is proxied to the API. `strictPort` is on
so a clash fails loudly instead of silently hopping onto a neighbour's port.
`scripts/ui.sh` checks first and names the offending process:

```
✗ port 7447 is already in use:
    bun (pid 95701)
  override with: ANUBIS_UI_PORT=<port> scripts/ui.sh
```

---

## Which bundler

Builds use **Bun's native bundler**; dev runs through **Vite**. Both were
measured, not assumed.

| | Bun native | Vite |
| :--- | :--- | :--- |
| Build time | **156 ms** | 523 ms |
| JS total | 721 kB raw / **221 kB gz** | 719 kB raw / 220 kB gz |
| CSS | 440 kB / 46 kB gz | 272 kB / **40 kB gz** |
| JS chunks | **1** | **31** |
| `@layer` order statement | **preserved** | **dropped** |

Two findings worth keeping:

**Bun is ~3× faster and emits the same amount of JavaScript** — it simply does
not split it. The `tsr` CLI emits 0 dynamic imports; route-level code splitting
is a transform the *Vite plugin* performs, not something the standalone
generator does. For an internal console an operator keeps open all day, one
221 kB gz bundle is arguably better than 31 chunks with a navigation waterfall.
For a bounce-prone public site the conclusion would flip.

**Bun preserves the explicit `@layer mantine,theme,base,components,utilities;`
statement. Vite drops it.** Vite's output is still correctly ordered — but only
because the layers happen to be *emitted* in the right order, and first-appearance
order is what CSS falls back on absent a statement. That is precisely the
fragility the declaration exists to prevent. Bun's physical emission order
actually has `components` and `utilities` swapped, and it does not matter,
because the statement wins.

> Correction to an earlier claim: I previously reported the Vite build's layers
> as "correctly ordered" on the strength of all six layers being *present*. That
> was a weaker check than the wording implied — presence is not order. The table
> above is the check that actually distinguishes them.

Dev stays on Vite because React Fast Refresh and the TanStack Router plugin's
route regeneration are worth more during iteration than 300 ms of build time.
Matching that under Bun needs a second `tsr watch` process for one moving part
too many.

> The Go service does not exist yet. This console runs against
> `src/lib/api/mock.ts`, which reimplements `migrations/0009_authorize_realms.sql`
> rule for rule — including every fail-closed branch. Swapping in an HTTP client
> means rewriting `src/lib/api/client.ts` and nothing above it.

---

## The one architectural rule

The backend's defining property is that a new access dimension is an `INSERT`,
not a deployment ([ADR-0003](../docs/adr/0003-scope-model.md)). A console that
hardcoded axis names would destroy that property at the presentation layer: the
database would accept a `cost_center` axis instantly, and the UI would need a
release to show it.

> **No component switches on an axis code.** Every scope control enumerates
> `GET /v1/admin/scope-axes` and renders each axis from its own `ui_schema`.

`src/components/scope/AxisTargetPicker.tsx` is where this lives. Consequences
that fall out for free:

- a newly registered axis appears in the grant builder, the authorization
  playground and every filter on the next refetch
- an axis marked `deprecated` disappears without a code change
- `ui_schema.help` renders as the operator-facing hint, so whoever registered
  the axis controls how it is explained
- an unknown `ui_schema.icon` degrades to a neutral glyph, so a brand-new axis
  renders correctly before anyone has chosen an icon for it

Axis codes are compared only against variables — "the axis currently being
rendered" — never against literals. No branch anywhere names a specific axis.

Realm *kinds* (`employee`, `partner`, `public`) **are** hardcoded in a few
badge colours, and correctly so: unlike axes they are a closed enum fixed by a
`CHECK` constraint in migration 0008 and cannot be added at runtime. The
distinction is the point — dynamic things are data, fixed things are code.

---

## Mantine and Tailwind in one app

Both ship a reset and both style base elements. `src/styles/index.css` declares
the order **before any `@import`**:

```css
@layer theme, base, mantine, components, utilities;
```

**This order took three attempts and only a screenshot caught it.** With
`mantine` first — which reads correctly, "let utilities win" — Tailwind's
preflight lives in `base` and therefore lands *after* Mantine, resetting
`button { background: transparent }` on top of it. Every `<Button>` silently
rendered as bare text. It type-checked, it built, and all nine routes returned
200.

Mantine has to sit **after the reset it needs to survive** and **before** our
components and Tailwind utilities, so `className="p-0"` still wins without
`!important`.

Two further details:

1. Import `@mantine/core/styles.layer.css`, not `styles.css`. The plain build is
   **unlayered**, and unlayered rules beat *all* layered ones regardless of
   order.
2. In `postcss.config.cjs`, `postcss-preset-mantine` must run **before**
   `@tailwindcss/postcss`, or Mantine's at-rules reach Tailwind unresolved.

**Division of labour:** Mantine owns components (tables, modals, popovers,
selects, notifications). Tailwind owns layout and spacing. They do not overlap,
so they do not fight.

---

## Design system

`src/styles/index.css` holds the tokens; `src/styles/theme.ts` maps them onto
Mantine.

**Two schemes, one vocabulary.** Light is the default (the more legible scheme
for daily administration); dark is one click away in the header and honours the
on-call-at-3am case. The rule that makes this cheap: **no component names a raw
color** — everything reads `--s-*`, `--ink-*`, `--line-*` and the verdict
tokens, and the two `:root[data-mantine-color-scheme]` blocks are the only
places hex values exist. Tinted borders and fills are derived with
`color-mix()` from the same tokens, so they re-tint per scheme automatically.
Light mode runs deeper cuts of gold and the verdict hues because the bright
ones fail contrast on white. `?scheme=dark|light` overrides the stored
preference for a single load — used by docs and the screenshot harness.
`autoContrast` keeps filled gold buttons on dark text in both schemes.

**Type.** 13px body, not 10–12. The first version chased density and produced
text that was genuinely hard to read; density has to come from spacing and
hierarchy, not from shrinking the type. Uppercase is reserved for eyebrow labels
and table headers — never for content.

**Surface.** Four steps (`sunken → base → raised → overlay`) so panels can nest
without borders doing all the work. Structure comes from borders, not drop
shadows, which read as smudges on a dark base.

**Colour.** Anubis gold is for identity and navigation only. **Teal and red are
reserved for verdicts** and nothing decorative may use them, so green always
means allow. Verdicts additionally carry weight, an icon and a border — red and
green alone is the worst possible pairing for deuteranopia.

**Motion.** One easing curve, two durations. Everything honours
`prefers-reduced-motion`.

---

## Two vocabularies

The UI speaks **operator language**; the URLs, types and API keep the precise
**schema language**. Labels are for humans, identifiers are contracts — renaming
a button must never be a breaking change, so the mapping lives entirely in the
presentation layer:

| Schema / API (stable) | UI label |
| :--- | :--- |
| identity | **Person** |
| grant | **Access** / "Give access" |
| realm | **Population** |
| scope axis | **Structure** |
| scope node | **Structure item** |
| assurance (IAL) | **ID verification** |
| authorization playground | **Access check** |

Two supporting rules: one verb everywhere (**Add**, never "Register" or
"Provision" — Tailscale's lesson), and precise terms demoted to tooltips and
code chips (`org`, `ctx:product_id`) where experts still see them. The command
palette keeps the old terms as search keywords, so typing "grant" or "axis"
still finds the right screen. Where the schema knows more, the button says
more: selecting an office offers **"Add department under 'Jakarta Office'"**,
computed from the legal child types — the hierarchy teaches itself.

## Creating things

Every object is creatable — identity, grant, role, permission, scope node,
scope axis — through right-hand drawers with a single shared shell.

**Reachable from anywhere.** The gold **New** menu in the header, the `⌘K`
palette ("New grant…"), per-page buttons, empty-state CTAs, and row actions
("Grant a role…" on an identity preloads that identity). Create flows are also
deep-linkable: `?new=identity` on any page opens the drawer, so a runbook can
say "click here".

**The forms teach the invariants instead of hiding them:**

- The grant form filters roles by the subject's realm — blocked roles stay
  **visible but disabled with the reason**, because an invisible option reads
  as a bug while a disabled one reads as a rule.
- Self-scoped and axis constraints are mutually exclusive in the UI the same
  way `grant_scopes_self_guard` makes them in the database.
- The node form only offers node types legal under the chosen parent, so "a SKU
  under an office" is impossible to express rather than rejected after submit.
- Validity is preset durations (30/90/365 days), not a date picker — the real
  use case is "contractor, 90 days", and time-boxed access that expires on its
  own is the point.
- Guard violations from the backend surface **verbatim** — the same message the
  SQL trigger raises — under a "Rejected" notification.

**The in-memory backend now supports full write operations** behind the same
`api` seam: create/disable identity, create permission/role/grant, revoke
grant, create node, register axis (which provisions the root and an item type,
so the axis is usable immediately). A new grant is live in `authorize()` the
moment it is created — 14/14 direct mutation tests pass, including the realm
guard and the axis-under-wrong-parent rejection. The Go service itself remains
unbuilt (see ../docs/roadmap.md); this is the mock honouring the same
contract.

---

## Interaction

**Command palette** (`⌘K`) jumps to any screen or action.

**`⌘↵` evaluates** in the authorization screen. An operator iterating on a
denial changes one field and re-runs dozens of times; reaching for the mouse
each cycle is friction.

**Decisions are shareable links.** Subject, permission and every axis target
live in the URL, and a link arriving with a complete scenario **evaluates on
open**. Debugging a denial nearly always ends in "why is this denied for
Alice?" being sent to someone else, and a screenshot loses the inputs.

**Warnings are earned, not default.** Unset-axis warnings appear only after a
first evaluation. Four yellow blocks on an untouched form is noise, and noise
trains operators to ignore the colour that later has to mean something.

## State ownership

| Concern | Owner | Why |
| :--- | :--- | :--- |
| Identities, grants, axes, scope nodes | **TanStack Query** | Server-owned. Duplicating it locally is how a console shows two answers to one question. |
| Realm filter, active axis, density | **Zustand** (persisted) | Client-only preference that should survive a reload. |
| Playground draft | **Zustand** (not persisted) | A scratchpad built across screens. Query would evict it on refetch; the URL gets unwieldy past four axes. |
| Axis registration, manifests | **TanStack Form** | Field-level validation with the `code` regex mirroring the DB `CHECK`. |

Query keys are a factory (`src/lib/query/keys.ts`) rather than inline arrays, so
invalidating everything under `scope` is one call and not a grep for string
literals.

---

## Screens

| Route | Purpose |
| :--- | :--- |
| `/` | Security signals first. Refresh-token reuse — the event meaning *a token was stolen* — leads the page rather than sitting under throughput charts. |
| `/playground` | **The signature screen.** A left-to-right decision trace: each axis is a gate, and the first red one is where the evaluation stopped. Names the failing axis and shows the target's ancestor path, so a denial is a fact rather than a mystery. |
| `/scope` | The axis forest. Registers new axes and runs the strict-mode dry run. |
| `/identities` | Realm-aware directory. Shows `alice` three times — once per realm — because uniqueness is per realm. |
| `/grants` | Multi-axis constraints, per-axis `inherit`, and which axes each grant is silent about. |
| `/roles` | Roles, `allowed_realm_kinds`, permissions with risk and assurance floors. |
| `/realms` | Population policy: factors, assurance, session TTL, retention. |
| `/audit` | Hash-chain status per entry. |
| `/keys` | Rotation lifecycle, with the publish-before-activate ordering made explicit. |

### Design choices worth naming

**Density over comfort.** An operator scanning 200 grants for the wrong one
needs rows close enough to compare without scrolling. Spacing tokens are tighter
than Mantine's defaults.

**Allow/deny never relies on hue alone.** Red/green is the worst possible pairing
for deuteranopia, so verdicts carry weight, an icon and a border alongside
colour. Teal and red are reserved exclusively for verdicts — nothing else uses
them.

**Fail-closed is shown, not hidden.** Leaving an axis unset renders a warning
explaining that a constrained axis with no target is *denied, not ignored*. The
playground deliberately lets you clear `_owner` to exercise that path.

**Dark-only.** An incident console is used in dark rooms, and the verdict palette
is tuned against the dark base.

---

## Verified

```
bun install                        159 packages, 9.25 s
tsc --noEmit (TypeScript 7.0.2)    clean
bun run build (native bundler)     156 ms · 221 kB gz JS · 46 kB gz CSS
bun run build:vite                 523 ms · 220 kB gz JS · 31 chunks
9 routes on :7447                  all HTTP 200, zero router warnings
@layer order statement             present in Bun output
port-conflict guard                refuses and names the holding process
```

**Verified visually** — headless Chrome screenshots of Overview, Authorization
(empty and with a live trace), Scope, Identities and Grants, in **both
schemes**. That is what caught the layer-order bug, which `tsc`, `vite build`
and nine 200s all missed.

**Still not verified:** no interaction tests, no accessibility audit, no
responsive check below ~1280px. The console is built for desktop operators and
has not been laid out for narrow viewports.
