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

Related: [[page-resolution]] for what a person sees when signing in.
