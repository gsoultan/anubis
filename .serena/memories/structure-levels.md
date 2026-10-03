# Structure levels: rules the database holds (0055–0056)

`scope_node_types` ("levels") says what may sit under what. 0014 holds every
ITEM to it; nothing held the rules themselves together until 0055's constraint
trigger `scope_node_types_rules` (AFTER, so multi-row inserts in any order and
whole-structure deletes still work):

- one top level (no parents) per structure — `scope_ensure_root` builds each
  tenant's top item from it and used to pick alphabetically between two;
- parents must be levels of the same structure, or the level itself
  (self-nesting is how "a company owned by a company" works at any depth);
- the top stays the top; every other level keeps at least one parent;
- a rule items rely on cannot be removed — archived items count, because
  restoring one does not re-run the placement guard;
- code and structure are fixed; a level others sit under cannot be deleted.

A new top item is named after its level, not `'All ' || axis_code`.

## Each tenant owns its structures (0056, 2026-10-03)
Structures and levels carry `tenant_id`; keys are `(tenant_id, code)` and
every reference (grant scopes, membership entries and anchors, sync sources,
items) is composite. Before 0056 they were the installation's and strictness
was one row every tenant's decisions read, so a tenant operator could deny
access in other tenants (proven against the old server, fixed and pinned by
`TestStructureEditsStayInTheirTenant`).
- The migration COPIED every structure and level into every tenant, so no
  decision changed. A tenant created later starts with none.
- Every SQL lookup of a structure or level must filter by tenant:
  `authorize()`, `authorize_explain`, `AuthorizeStrictSim`, the gate's
  `SnapshotAxes`, `scope_ensure_root`, both level triggers. A new one that
  forgets is the old defect again.
- A strict flip bumps only its own tenant's catalog version.
- The migration ANALYZEs the rewritten tables: without it authorize() p95 sat
  at 5–6 ms until autovacuum.

## Sharp edges
- **Level codes are unique per tenant**, across all of its structures. The
  console derives `<axis>_<slug>` and dedupes (`ui/src/lib/levels.ts`);
  screens show names, never codes.
- **`CreateScopeAxis.top_level` is opt-in.** With it the structure and its top
  level are one transaction. Without it nothing is created — `ensureAxis` in
  the e2e suite makes its own, and an automatic one would have collided.
- **Rename also un-archives** (that is how sync brings a node back), so the
  interactor refuses to rename an archived node; `RestoreScopeNode` is the way.
- **Archive does not remove access** — grants there keep deciding; the item
  only leaves the pickers. A MOVE does change access. The console says both.
- `ScopeNodeRepository` and `ScopeNodeAdminUsecase` are at 14 of 15 methods.
- **`bench/rebuild.sh` drops the dev database's schema** (`-d anubis`). Run its
  steps against a scratch database instead unless the dev data is disposable.

## Guard messages reach the operator
`database.MapErrSaying` attaches a RAISEd message as the `reason` detail;
`ui/src/lib/errors.ts` shows it. Opt-in per call site: read the guard's
messages first — they must name nothing outside the caller's tenant. Declared
constraints are never included (PostgreSQL's detail carries the row).

## Do not "fix" `database.OrEmptyJSON`
It also writes audit details, whose stored bytes must equal the hashed bytes.
Treating JSON `null` as `{}` there broke the audit chain for every event with
no detail. The null-targets guard for decisions lives in the authz adapter
(`decisionTargets`): a nil map marshals to `null`, and `authorize()` fails on
it only for a subject holding a place-limited grant.

## Tests and tools
`test/integration/scopelevels/`; e2e `TestCreateStructureWithItsTopLevel`,
`TestScopeNodeEditsRefuseAnotherTenantsNode`. `ui/scripts/drive-structure.ts`
drives the real screens (not CI: it writes). CI now runs
`./test/integration/...` — the subpackages ran nowhere before.
