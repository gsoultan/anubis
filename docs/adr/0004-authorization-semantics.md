# ADR-0004 — Authorization evaluation semantics

**Status:** accepted · **Date:** 2026-08-22

## The decision function

```
authorize(identity, tenant, permission, targets) → boolean
```

`targets` is a map of `axis_code → scope_node_id` describing *where* the action
is attempted.

## Evaluation rules

A grant **survives** only if all of the following hold:

1. It is live — not revoked, `valid_from <= now()`, `valid_until` null or future
2. Its role confers the permission (via `role_permissions_effective`)
3. **For every axis it constrains:** at least one constrained node is an
   ancestor-or-self of the target node on that axis
4. **For every axis with `default_effect = 'deny'`:** the grant constrains it
   explicitly

Access is granted if **any** grant survives.

| Scope | Rule |
| :--- | :--- |
| Within an axis | **OR** — multiple nodes mean any of them |
| Across axes | **AND** — every constrained axis must match |
| Axis absent, `default_effect = 'unconstrained'` | Satisfied |
| Axis absent, `default_effect = 'deny'` | **Fails** |
| Target omits an axis the grant constrains | **Fails** (see below) |
| `inherit = false` | Only `depth = 0` — the node itself |

## Fail-closed is the whole design

The natural formulation is wrong, and its wrongness is invisible in testing:

```sql
-- FAILS OPEN — do not do this
FROM grant_scopes gs
JOIN targets t ON t.axis_code = gs.axis_code
JOIN scope_closure c ON c.ancestor_id = gs.scope_node_id
                    AND c.descendant_id = t.node_id
```

If the caller supplies no target for an axis the grant constrains, the inner join
**drops the row**. The axis vanishes from evaluation and the grant passes
unchecked. An application that forgets to send `product_id` silently gets
*unrestricted product access* — and every test passes, because the happy path
always sends every axis.

The correct form uses outer joins with an explicit null check:

```sql
axis_eval AS (
    SELECT gs.grant_id, gs.axis_code,
           EXISTS (SELECT 1
                     FROM targets t
                     JOIN scope_closure c
                       ON c.descendant_id = t.node_id
                      AND c.ancestor_id   = gs.scope_node_id
                    WHERE t.axis_code = gs.axis_code
                      AND (gs.inherit OR c.depth = 0)) AS satisfied
      FROM grant_scopes gs JOIN candidates cd ON cd.id = gs.grant_id)
```

and requires that **nothing** fails:

```sql
SELECT EXISTS (
    SELECT 1 FROM candidates cd
     WHERE NOT EXISTS (SELECT 1 FROM axis_eval ae
                        WHERE ae.grant_id = cd.id AND NOT ae.satisfied)
       AND NOT EXISTS (SELECT 1 FROM scope_axes a
                        WHERE a.default_effect = 'deny' AND a.status = 'active'
                          AND NOT EXISTS (SELECT 1 FROM grant_scopes gs2
                                           WHERE gs2.grant_id = cd.id
                                             AND gs2.axis_code = a.code)));
```

Both `NOT EXISTS` clauses are deliberate: the default is deny, and a grant
survives only by having nothing fail.

**The test that matters** omits an axis from `targets` and asserts denial. It is
in [`bench/run.sql`](../../bench/run.sql) as
`FAIL-CLOSED: axis omitted -> DENY` and it is the single most important
regression test in the project.

> During development this behaviour was briefly misread as a bug: a test supplied
> `org` + `cost_center` for a grant that also constrained `product` and
> `customer`, and the resulting deny looked wrong. It was the engine working
> correctly. The note is preserved in
> [`bench/add_axis.sql`](../../bench/add_axis.sql).

## Adding an axis is non-breaking, by construction

Because an absent axis is unconstrained by default, an axis introduced later is
absent from every existing grant and changes nothing.

**Measured:** registering a `cost_center` axis, syncing 511 nodes, then replaying
2,000 previously-recorded decisions → **0 changed decisions.**

### Safe rollout to strict mode

| Phase | State | Effect |
| :--- | :--- | :--- |
| 1 | Axis added as `unconstrained` | Nothing changes. Populate nodes, attach constraints. |
| 2 | **Dry run** — replay recent decisions against the strict rule | Reports exactly what would break |
| 3 | Flip to `deny` | Every grant must now speak to this axis |

The dry run is mandatory tooling, not a nicety. Measured on the seed data,
flipping `cost_center` to strict took allows from **800 → 0**, because no grant
constrained it yet. Discovering that from a report beats discovering it from an
outage.

## Deferred: deny rules

Explicit `deny` grants ("everything in Jakarta **except** approve payments") are
frequently requested and make the model much harder to reason about. Users can no
longer predict their own access, and precedence must be documented and
understood. Google Cloud IAM shipped without deny for a decade for this reason.

**v1 is allow-only with union semantics.** If deny becomes necessary, add it with
strict precedence (deny wins, evaluated across the full ancestor chain) and an
explain endpoint. Getting this wrong produces outages that look like security
incidents.

## Permission resolution

Roles compose (`role_parents`), and roles may grant wildcards
(`role_permission_patterns`). Both are **expanded at write time** into
`role_permissions_effective`.

Wildcards are never matched at read time. Three reasons:

1. **Evaluation is pure set membership** — no runtime globbing on the hot path
2. **Auditing works** — "what can `billing.approver` do?" is a `SELECT`, not an
   interpretation
3. **New permissions cannot silently widen existing roles.** When an application
   registers `invoice:delete`, re-expansion makes the widening *visible and
   reviewable*. With runtime wildcards it just quietly applies — which is exactly
   how privilege creep happens invisibly.

`via_role_id` records provenance, so "why does Alice have this?" is a query.

Maintenance is belt-and-braces: recompute in the writer's transaction (recursive
CTE with PG's `CYCLE` clause for loop detection) **and** run a nightly
reconciliation that recomputes from scratch and alerts on any diff. A
materialised cache of authorization decisions that silently drifts is worse than
no cache, because the failure is invisible.

## Step-up authentication

Permissions declare their own authentication requirement:

```sql
risk           text     -- 'normal' | 'sensitive' | 'critical'
requires_amr   text[]   -- e.g. '{otp}'
max_auth_age   interval -- e.g. '5 minutes'
```

`/v1/authorize` enforces it centrally and returns a machine-readable answer:

```jsonc
{ "allow": false, "reason": "step_up_required",
  "required_amr": ["otp"], "max_auth_age": "2m",
  "current_amr": ["pwd"], "auth_age": "41m" }
```

One place defines what "sensitive" means. Changing security policy becomes a
catalog update rather than a coordinated deploy across twelve applications.
