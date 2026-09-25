# Every create drawer's primary button was inert (2026-09-13)

`CancelSubmit` (`ui/src/components/create/shell.tsx`) rendered
`<Button type="submit">`. `CreateShell` renders the footer as a **sibling of
the drawer body**, so that button sat outside the `<form>` it was meant to
submit — and four of the nine drawers (grant, node, membership, sync source,
edit role) have no `<form>` at all.

Consequence: clicking "Give access", "Add item", "Create membership",
"Connect source" or "Save changes" did nothing. The tanstack-form drawers
(identity, role, permission, axis) could only be sent by pressing Enter
inside a field. It was that way from the console's first commit.

`onSubmit` is now **required** on `CancelSubmit` and the button is
`type="button"`. Required, not optional, is the point: an optional handler is
how the next drawer ships with an inert button. `type="button"` also means a
click cannot double-fire alongside a form's own `onSubmit`.

The grant form itself now lives once, in
`components/create/GrantFields.tsx` — `useGrantDraft()` owns state, validation
and the write; `<GrantFields>` renders role / validity / self-scoped / axis
constraints; the caller owns layout. Two callers: the drawer that still has to
ask WHO (`CreateGrant`), and the panel on a person's own page
([[console-person-page]]) where the subject already is the page.
`AxisConstraintRow` and `NodeSel` moved there too — `CreateMembership` imports
them from `GrantFields` now, not from `CreateGrant`.

The `ListRealmCategories` 500 found while verifying this is written up in
[[identity-directory-reads]] — it is fixed.

## Inert again, for a different reason: form state read as a snapshot (2026-09-25)

`form.state` is tanstack-form's state at the moment it is read. Typing
re-renders the field, never the component that owns the form, so anything
computed from `form.state.values` during render is frozen.

- `CreatePermission` built its key preview that way, and the submit button
  required the preview: however the form was filled, the preview stayed
  empty and the button stayed disabled. Picking a step-up factor never
  revealed "Maximum authentication age" either.
- `CreateIdentity` looked up the chosen population that way: its facts
  (required factors, session TTL, retention) never appeared and its
  categories were never fetched — nobody added from the console could be
  given a category.
- Axis, identity and role read values inside `<form.Subscribe>` whose
  selector did not include them. They worked only because a field validator
  flipped `canSubmit` after the relevant keystroke.

Rule: render reads through `useStore(form.store, selector)` or a
`<form.Subscribe>` selector; event handlers use `form.getFieldValue`.
`scripts/check/console-form-snapshots.sh` fails on any `form.state.` in
`ui/src`.

A scripted `fill` (one input event carrying the whole value) and real typing
behave differently here: the validity flip that rescued the Subscribe reads
only happens keystroke by keystroke. Verify forms by typing.
