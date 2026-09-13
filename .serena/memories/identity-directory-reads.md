# What the identity read path never returned (2026-09-13)

Three bugs, one shape: a column or a filter that existed everywhere except in
the query the console actually calls. Related: [[console-person-page]].

## `realm_id = ''` is not "no filter"

`realm_categories.realm_id` is `uuid`. The People screen spans populations, so
it asked for every category by sending an empty realm id — and Postgres
answered `invalid input syntax for type uuid: ""` (SQLSTATE 22P02). Every
visit to People fired two 500s, and the category label under each name had
never once rendered. `sqlc.narg(realm_id)::uuid IS NULL OR realm_id = ...` is
the repo's existing idiom for this (`ListIdentities` had it all along);
`database.OptStr` maps `""` to NULL, `&realmID` does not.

## The guard proved the tenant and the query ignored it

`ListRealmCategories` keyed on `realm_id` alone. `Require()` returned a
principal whose `TenantID` was then used only for the counts. An operator who
learned another tenant's realm id read that tenant's categories — the
directory classification of people they have no relationship with.

The query is now `WHERE tenant_id = $1 AND (realm optional)`, and
`TestRealmCategoriesRefuseAnotherTenantsRealm` plants a realm under a second
tenant and demands nothing comes back. **Any listing whose only filter is a
child id is worth re-reading**: the tenant column has to be in the WHERE, not
just in the principal. That re-reading happened — [[tenant-scoped-reads]] —
and found three more.

## A column nothing ever selected

`identities.retention_until` and its partial index have existed since
migrations/0008, and the sweeper writes it — 5,000 rows carry one in the dev
database. Neither `GetIdentity` nor `ListIdentities` selected it, so it could
not reach the proto, so `live.ts` hardcoded `retention_until: null`, so the
console's Retention column printed `—` for everyone. Column present, index
present, feature absent.

`Identity.category` was the same story from the other end: the proto HAS
carried the category code all along; `toIdentity` hardcoded `category_id:
null`, so every lookup by id found nothing. Codes are unique per REALM, so a
consumer matches code AND realm_id.

## The duplicate mapper is what let it drift

`live.identity()` hand-built the same object `toIdentity()` builds. Two places
to add a field; only one ever got it. `identity()` now calls `toIdentity()`.

## Test-harness trap

`t.Cleanup` runs AFTER the test function returns, so a `defer db.Close()`
closes the pool before a cleanup that needs it. The first run of the
cross-tenant test left its planted tenant in the dev database because of this,
silently — the delete error was discarded. Register the close with
`t.Cleanup` too (LIFO puts it last) and report the delete error.
