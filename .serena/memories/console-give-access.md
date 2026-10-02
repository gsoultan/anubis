# Giving access: one sheet, a person's page, no Access screen (2026-09-28)

Access is read and changed on a person's page ([[console-person-page]]).
The Access list (`/grants`) was removed at the operator's request — it listed
150k grants when the question is always about a person. `/grants` now
redirects to `/identities` (it shipped; bookmarks and runbooks hold it). The
one question only that list answered — "who holds this role?" — moved to the
role: Roles → ⋯ → **People with this role** (`components/access/RoleHolders`,
keyset-paged `searchGrants({ roleId })`, keyed `['grants','role',id,cursor]`).

## One sheet, every entry point

`components/access/GiveAccessSheet` is the only grant form. Opened from the
person page header (`?give=true`, URL-bound), the People row's hover icon
(navigates there), and globally — Add menu, ⌘K, overview — via
`openCreate('grant')`, where it asks **Who** first (or takes
`ctx.identityId`). Four numbered questions: what (role *or membership*),
where, how long (presets or a date, each to 23:59:59 local), note (stored as
the grant's `reason`). A Review block reads the answer back; the footer
`status` says why the button is off.

## Rules the form enforces, and the traps found building it

- **Where is an explicit choice** (Everywhere / Specific places / Their own
  records), never a default — the broadest grant used to be what you got by
  not touching forty-seven "Anywhere" rows.
- **Required structures** (`default_effect = 'deny'`, active): authorize()
  drops any grant with no scope row on one (0046 `NOT EXISTS … gs2`). That
  includes **every self-scoped and every "everywhere" grant** — they carry no
  scopes. The sheet disables those cards AND refuses them in
  `useGrantDraft` — disabled cards alone are not enough, a choice made
  before `axes` loaded survives the disable. `AccessList` marks existing
  grants like that "has no effect". No strict axis exists in the dev data, so
  this path is proven by `ui/test/access.test.ts`, not on screen.
- **"Specific places" with nothing chosen has the same empty scope list as
  "everywhere"**, so rendering it through `GrantWhere` printed "Everywhere —
  no limits" in the Review. Special-cased. Any new summary of a draft must
  check `reach`, not just `scopes`.
- **The sheet stays mounted and can close without `onClose`** (Back drops
  `?give`), so it resets on every *open* — or a role picked for one person is
  waiting when the sheet opens for the next.
- `?give=1` did nothing: TanStack parses search values as JSON, so it arrives
  as the NUMBER 1. `lib/searchFlag.ts`; test fails on the old comparison.

## The place picker (`components/access/PlacePicker`)

A file dialog: structures left, that structure's lazy `ScopeTree` right
(`picked` map marks include/exclude), "Chosen places" below — the only place
a choice is refined, through four plain modes (include/except × with
everything inside/this place only). Two panes by **container** query (`@xl`,
576px), a Select above the tree below that — the same picker serves the
680px sheet, the 460px membership drawer and a phone. Structure icons render
only if the installation configured any: the `AxisIcon` fallback on 48 rows
read as 48 loading spinners. Rules are said once, only those the choice
touches (AND across structures, exceptions narrow this grant only, unchosen
optional structures stay unlimited — required ones do not).

## Memberships from the person page

"A membership" in the sheet calls `assignMembership`; the person's access is
grouped *given directly* / *through <membership>*, and a membership group's
**Remove from membership** calls `unassignMembership` — a derived grant
cannot be revoked on its own (the membership writes it back). A person's
memberships are read off their grants' `via_membership_id`: the API gives
head counts, not rosters.

## Pure rules, tested

`lib/access.ts` (required axes, places→scopes, end dates, calendar-day
relative dates, role blocking) has no React/API imports so `bun test` can run
it: `ui/test/*.test.ts`, outside `src/` because the console tsconfig carries
no `bun:test` types. `bun run test` runs in both CI console jobs.

## Fixed: a revoke no longer erases why access was given (0053)

`grants.reason` held both answers: `RevokeGrant` ran `reason = CASE WHEN $3 =
'' THEN reason ELSE $3 END`, and the console revoked with the constant
`'console'` — so the note written when access was given was overwritten the
moment it was revoked, and `grant.create`'s audit event does not carry it, so
it was gone everywhere. Now `reason` is why it was given and a revoke never
writes it; `revoke_reason` (0053, nullable, catalog-only add) is why it was
taken away — the shape `sessions` already had. Exposed as `Grant.revoke_reason
= 13` (additive; `buf breaking` clean). The console's revoke confirmation
takes an optional "why" instead of sending `'console'`.

Rows revoked before 0053 were left alone: a revoke without a reason kept the
original, so no row says whether its `reason` is the grant's or the revoke's.

Pinned by `TestStormFull_RevokeKeepsWhyAccessWasGiven` (test/integration),
shown failing on the old statement first. It builds its OWN fixtures: the
older `TestStormFull_GrantFamily` starts from `SELECT … FROM grants LIMIT 1`
and skips without one — and a freshly bootstrapped database, which is what CI
runs the integration suite against, has zero grants. So that test has been
skipping in CI; it only ever ran against seeded dev data.

Related: [[console-person-page]] · [[console-create-drawers]] ·
[[console-tables]] · [[console-design-system]] · [[scope-exclusions]]
