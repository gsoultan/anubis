# Catalog sync — permissions and roles, four ways in

An application's catalog (permissions, roles, routes) arrives through ONE
apply path, `authzadmin.applyCatalog`. The four channels are shapes of the
same document, not four pipelines:

| Channel | How |
| :--- | :--- |
| HTTP API | `ApplyManifest(slug, document, format, dry)` |
| CSV file | same RPC, `format: "csv"` — parsed INTO a manifest |
| Scheduler | `catalog_sync_sources` + the `catalog_sync` job (0043) |
| Manual | the console, and `RunCatalogSource` on a configured source |

`internal/authz/app/catalog` owns what a document IS (parse, validate,
sections); `app/admin` owns what applying one DOES. CSV carries one sheet per
file, decided by its header — permissions or roles. Routes are JSON-only on
purpose: priority ordering and scope bindings do not survive being cells.

## The rules that cost something to learn

- **Absent ≠ empty.** A section the document never mentions is left alone; a
  present-but-empty one clears. Before this, a manifest omitting `routes`
  DELETED every route policy, and one omitting `permissions` deprecated the
  whole catalog — which is what a CSV of roles would have done every time.
  A present-but-empty permissions section on a live catalog is refused.
- **Digest or churn.** A run stores sha256 of the document; an identical one
  is `skipped` and writes nothing. Applying bumps `manifest_version`, and that
  is what makes every gate rebuild its snapshot ([[gate-refresh-version-gate]]).
  A 5-minute poll of an unchanged file would rebuild them 288 times a day.
  Verified live: second tick skipped, version stayed put.
- **A scheduled run has no operator.** `ApplyDocumentAsSystem` takes its own
  tenant and checks nothing, so it must never be reachable from a transport —
  `scripts/check/import-boundary.sh` fails the build if an adapter names it,
  and `CatalogAdminService` omits `RunDue` so the handler cannot reach it at
  all. Runs audit as `actor_kind='system'` with a null actor.
- **A source is pinned to one application** by composite FK and cannot be
  repointed; permissions are upserted under that app's id and slug, so a
  compromised feed makes a mess in exactly one application.
- Egress policy is shared with scope feeds: `internal/platform/egress`, moved
  out of `scope/adapter/feed` so the two cannot drift apart.

## Storm gotcha that will bite again

A `storm.SQL` declaration MUST be listed in `authzrquery.Queries()`. Omit it
and the build is clean, the unit tests pass, and it fails AT RUNTIME with
"this statement was not declared at generate time" — found by watching a
scheduler tick fail, not by any test. Two edits: the list, then
`go run ./cmd/stormgen generate ... -dsn $ANUBIS_DB_URL`.

## Removal, both halves (0044)

A permission OR a role the document stops naming is retired, not deleted and
not revoked. `authorize()` reads neither `deprecated_at` column, so every
grant that already names one decides exactly as it did; what stops is
attaching it to anything new. For roles that refusal is the `grants_role_live`
constraint trigger — the same enforcement layer as the realm-kind guard, so it
holds for the console, the API, a bulk import and whatever is written next,
not just the one code path that remembered.

Only `is_system` roles are retired automatically. A role an operator made by
hand inside the same application is theirs.

Re-declaring revives, and `UpsertSystemRole` carries the document's current
description and realm kinds through. The old path did find-or-create and
changed nothing on an existing role, so a synced description was silently
ignored — sync that can only add is not sync. `UpdateRole` cannot be reused
here: it refuses system roles on purpose (`AND NOT is_system`).

## Console

*Catalog sync* under Access (`ui/src/routes/catalog.tsx`) lists sources with
`last_status` on the row — the field exists because a source that failed an
hour ago and one that applied an hour ago are identical if all you show is
when they last ran. The drawer holds settings, dry run, run now, delete and
the history.

Two things the first cut got wrong and are worth not repeating: a dry run does
not move `last_run_at`, so pairing the status with that timestamp printed
"dry run / never" in one cell; and a `next_run_at` in the past is "due now",
not "1 min ago", because the scheduler ticks every minute.

The manual channel stayed where it already was — *Applications → Manifest* —
and now takes CSV and a file, rather than becoming a second screen.

## The trap under both deprecate-except queries

`NOT (id = ANY($2::uuid[]))` with an EMPTY keep-list is not "retire
everything" — an empty Go slice reaches Postgres as NULL, the predicate
evaluates to NULL, every row is excluded and NOTHING is retired. Silent, and
the exact opposite of what the caller asked. Both queries now wrap the
parameter in `COALESCE($2::uuid[], '{}')`.

Found by an e2e test, not by reading: the empty-section rail depended on the
deprecation actually running, so the refusal never fired. Same family as
`orDefaultKinds` / `database.EmptyIfNil` — a nil slice into a NOT NULL array
column is a constraint violation, which is why every array parameter in this
context goes through a helper.

Test discipline note: `go run ./cmd/anubisd serve` leaves the compiled child
alive when the parent is killed, so a "restarted" server can silently still be
the old binary. Build to a path and run that.
