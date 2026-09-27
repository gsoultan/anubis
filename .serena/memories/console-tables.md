# Console tables and the People screen

The operator console (`ui/`) renders every list through one primitive,
`src/components/ui/DataTable.tsx`. Layout decisions live there, not per screen.

## `overflow: clip`, never `overflow: hidden`, on `.panel`

`.panel` clips the table's square corners to the panel radius. It used
`overflow-hidden`, which makes the panel a **scroll container** — and a
`position: sticky` child resolves against the nearest scroll container. The
sticky column header therefore had nothing to stick to and scrolled off with
the rows, on every table in the console, silently. `overflow: clip` clips
without creating a scroll container, which hands the header back to
`<main>` (the real scrollport, `__root.tsx`).

If a sticky header ever stops sticking, check for an `overflow: hidden`
ancestor first.

## A table scrolls sideways only while it does not fit (`data-wide`)

`.panel` clips, so a table wider than its panel lost the columns past the
edge with no way to reach them — not only on phones: at 1024px Audit, Grants,
People and Applications were all cut off, and at 1280px Audit still lost
19px (its Detail chips are `nowrap`, so the widest value sets the column).

`overflow-x: auto` on `.tbl-body` fixes reach but makes it a scroll
container, which takes the sticky header away from `<main>` — per spec a
non-visible overflow on one axis turns the other into a scroller too, so no
CSS keeps both. And CSS cannot say "only when it overflows": the property
creates the container either way. So `DataTable` measures: a
`ResizeObserver` on `.tbl-body` and its `<table>` sets `data-wide` while
`scrollWidth > clientWidth`, and only `.tbl-body[data-wide]` scrolls. A table
that fits keeps its sticky header; one that does not gives it up for reach.

A phone-only `@media` rule was tried first and was wrong twice: it left every
width between 768px and ~1300px clipped, and it cost tables that DO fit on a
phone their header.

## Rails

`DataTable` takes optional `toolbar` and `footer` rails rendered inside the
panel. Both are exactly `var(--rail)` (48px) tall and `.tbl-offset thead th`
offsets its sticky `top` by the same variable, so a toolbar and the column
header stack without overlapping. Change one, change the variable.

Filters belong on the rail, not in the `Page` `actions` slot: the page header
sits a hand's width from the global ⌘K box, and two grey search fields side by
side is a screen nobody trusts. The toolbar also survives the empty state —
the control that produced "no matches" has to still be there to undo it.

## Wording

User-facing name is **Population**; `realm` is the code/schema word only.
`realms.tsx` is titled "Populations" and `PersonDetail` says "Population".

Realm `display_name` is **not unique** — this installation runs three
populations all called "Enrolment probe" — so any chooser must carry `code`
alongside the name. That is why the People filter is a `Select` and not a row
of chips: twelve near-identical chips wrapped into a band that taught nothing.
`identities.tsx` shows the code under the name only when it disambiguates
(another population shares the display name) or when it is not an echo of it
("Internal" over "internal" is the same word twice).

Kind → colour is `src/lib/realmKind.ts` (`REALM_KIND_COLOR`). Three screens
each carried a copy and two of them were missing `service`, which then came
out the same colour as `public`.

## Column discipline

A column that prints the same value on every row is noise wearing a header.
People dropped the ULID column (reachable via row click and ⋯ → Copy ID) and
turned "no statutory limit" into `—` with the explanation on a header hint
(`Column.headerHint`). Status inverts the emphasis: `active` is a quiet dot
because it is what 99 rows in 100 say; the pill is spent on the exception.

Client-side filtering of a server-paged list is forbidden — it hides rows the
server deliberately returned and makes the count lie. `grants.tsx` says the
same. That is why People has no status filter: `identitiesPage` takes realm,
query, cursor and limit, and nothing else.

The category sub-label and the Retention column carry real values now; both
were structurally incapable of it, see [[identity-directory-reads]].

The row click opens the person's page ([[console-person-page]]), not a
drawer; `.row-go` is the chevron affordance, hidden until the row is hovered
or focused so fifty rows do not each carry a permanent arrow.

Related: [[page-resolution]] for what a person sees when signing in.

## Audit at 1280px: wrapping the Detail values was measured and rejected (2026-09-26)

Audit's fixed columns total 765px and its Detail chips are `nowrap`, so the
widest value on the page — usually `subject=<uuid>`, ~300px — sets the
column, and at 1280px the table outgrows its panel, scrolls (`data-wide`) and
loses its sticky header. Letting the chips break anywhere was tried:

  1280px  table fits, header sticks — but 74 of 100 rows grew, up to 156px
  1440px  19 of 100 rows grew
  1024px  still too wide (the fixed columns alone exceed 750px), and Detail
          collapsed to a sliver: one row was 1,169px tall

Truncating is not an option either: Audit has no detail view, so the cell is
the only place those values appear. One-line rows win for a list read by
scanning (see `Cell` in DataTable.tsx), so Audit keeps `nowrap` chips and
scrolls sideways where it must. Do not retry wrapping without a detail view.

## Audit shows only the newest 100 entries, and searches only those (2026-09-26)

`live.audit()` calls `QueryAudit({ pageSize: 100 })` with no page token and
no paging controls, and audit.tsx filters the result in the browser. An
installation with 100k entries shows its newest 100, and "Search actor or
action" answers from those 100 alone — an investigation can find nothing and
be told so. This is the client-side-filter-of-a-server-page rule above,
broken. The server already takes actor_id, action, from/to and page_token;
it has no result filter. Not fixed yet.
