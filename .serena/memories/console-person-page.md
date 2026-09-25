# The person page, and giving access from it (2026-09-13)

A record an operator can act on is a page, not a drawer. `/identities/$id`
(`ui/src/routes/identities_.$id.tsx`) replaced the 620px `PersonDetail`
drawer. Related: [[console-tables]] for the list it opens from.

## Why the drawer had to go

Everything an operator opens a person for — read the access, understand its
shape, add to it, take some away — happened in a column narrower than the
table it was covering. And the one action they came for, "Give access", threw
a SECOND drawer on top of the first, over a list they could no longer see.

Revoking was worse: it existed only on the Access table, so the one screen
showing a person's whole access could change none of it.

## The id IS in the URL now, deliberately

The drawer avoided that on purpose ("an id in the URL is an id in someone's
browser history"). That reasoning holds for the **encrypted attributes and the
credentials** — those stay behind modals on the person page, opened from
component state, never a route param. It does not hold for the identity id
itself: a ULID in the address is what makes the page linkable from a ticket.

## The trailing underscore is load-bearing

The file is `identities_.$id.tsx`, not `identities.$id.tsx`. TanStack Router
nests `identities.$id` under `identities.tsx`, which would demand an
`<Outlet/>` and turn the People list into a layout. `_` opts out of the parent
layout; the route id stays `/identities_/$id` while the path is
`/identities/$id`.

## Query keys must share a prefix or invalidation silently misses

Two bugs, one cause. React Query matches prefixes on **array elements**, not
on string prefixes:

- The People list was keyed `['identities-page', …]`. Disabling somebody
  invalidated `['identities']`, which does NOT match it — so the row kept
  saying "active" until a reload. Now `['identities', 'page', …]`, alongside
  `qk.identity` = `['identities','detail',id]`.
- A person's access was keyed `['person-access', id]` while every write
  invalidates `['grants']`. Now `qk.grants(id)` = `['grants', id]`.

Rule: a screen's key goes **under the factory prefix its writers invalidate**.
A key of its own is a list that never refreshes.

## Where the operator was standing

`usePeopleList` (`stores/session.ts`, non-persisted, beside `usePlayground`)
holds the People search and the keyset cursor trail. Every row now leads off
the screen, so returning to page 1 of an unfiltered list — having lost the
search that found the person — would be the price of the page. `realmFilter`
stays in the persisted session store as before.

## `Page` grew three slots

`back`, `lead`, `badge` (`components/shell/Page.tsx`). A detail page opening
the same way as the next detail page is the frame's job, exactly as the
heading weight is. `__root.tsx`'s `TITLES` map is keyed on an exact pathname,
so `/identities/<ulid>` rendered "Not found" in the header of a page that had
loaded fine; `PersonCrumb` reads the username out of the query cache the page
itself fills — no second request, no store to keep in sync.

## Blank on refresh until the build emitted root-absolute URLs (2026-09-25)

`/identities/$id` was the first console path with a second segment, and the
bundle loaded its assets by RELATIVE URL (`./chunk-….js`). Clicking through
from People worked — the document was already loaded — but a refresh or a
shared link resolved `./chunk.js` to `/identities/chunk.js`, the SPA
fallback answered with index.html, the browser refused HTML as a script, and
the page was blank. Shipped that way in v0.4.0.

`ui/scripts/build-bun.ts` now sets `publicPath: '/'` and refuses to finish if
the shell references any relative asset. `<base href="/">` is not an option:
the shell's CSP sets `base-uri 'none'` (`internal/api/http/console_assets.go`).

It was found because a width check "passed" on that page — a blank page has
nothing to overflow. A check that asserts something is absent must first
assert that the page rendered at all, and must load it by URL, not by click.
