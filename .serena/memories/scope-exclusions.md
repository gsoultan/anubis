# Scope exclusions (migration 0046)

One grant can now say "everywhere under Jakarta **except** the Surabaya
branch": `grant_scopes.mode` and `membership_entry_scopes.mode` are
`include`|`exclude`, text with a CHECK.

## The rule, whole

```
satisfied(grant, axis) = (some include covers the target)
                     AND (no exclude covers the target)
```

## Why this is not the deny ADR-0004 deferred

The thing that made deny unaffordable was **cross-grant precedence** — a rule
elsewhere quietly overriding the grant you are reading. An exclusion cannot do
that: it is scoped to the axis of the grant it sits on. Grants still compose by
union, so a **second grant covering the excluded place still allows there**.
Each grant reads on its own, and there is no precedence order to learn.
Verified on the dev box both ways. If cross-grant deny is ever wanted, that is
still the deferred decision in ADR-0004; this is not a step toward it.

## An exclusion needs an include on the same axis

Otherwise "anywhere except here" is a blanket allow wearing a carve-out's
clothes, and on a strict axis it defeats the one thing strict exists to force.
Refused by `grant_scopes_exclusion_guard` / `membership_entry_scopes_exclusion_guard`,
both **DEFERRABLE INITIALLY DEFERRED** — the include and the exclude arrive as
separate INSERTs in one transaction and the writer promises no order, so the
question can only be answered at commit. **Deliberately not armed on DELETE**:
grant_scopes rows are only ever deleted by the cascade when a grant row goes,
during which "the include is missing" is true of every exclude row — a DELETE
arm would turn the cascade into an error.

With that invariant, the strict-axis check in `authorize()` needed **no change
at all**: every axis carrying rows carries at least one include.

## The hot path pays 1.1%, not 9.2%

The readable spelling is two aggregates:

```sql
bool_or(mode='include' AND covers) AND NOT bool_or(mode='exclude' AND covers)
```

Measured interleaved, best-of-15, 2,000 decisions against 270k grant_scopes:

    0013 as it stood        112.0 ms best / 113.0 median
    two bool_or             122.1        / 123.4         +9.2%
    one min() aggregate     113.4        / 114.4         +1.2%

Shipped is the single aggregate, a **severity order** where min() picks the
strongest opinion: `0` an exclude covers (veto), `1` an include covers, `2` the
row says nothing. `min = 1` is exactly "included and not excluded"; `0` and `2`
both deny, so an exclude-only axis that slipped past the guard fails CLOSED.
`authorize_explain` keeps the two-aggregate form — it is an operator tool, not
a request path, and it reports `included`/`excluded` separately anyway.

## Four implementations of one rule, not three

`authorize()`, `authorize_explain()`, `AuthorizeStrictSim` in authzrquery
(the dry run an operator reads before flipping an axis strict — left on the
includes-only aggregate it would report allows the live engine denies), and the
gate's Go evaluator. The first three are in 0046; the fourth is held to it by
`TestSnapshotAgreesOnACarveOut`.

`ScopeIndex.CoveredBy` keeps 0013's early exits behind a `hasExclude` flag, so
every grant without a carve-out costs exactly what it did. Only a set holding
an exclusion walks the full ancestor chain — and it must, because stopping at
the first include would miss an exclude further up, and **missing an exclude
fails OPEN**: the gate allowing, in memory at p99 < 1 ms, what the database
denies.

## `bool exclude` on the wire, `mode text` in the database

proto3's zero value has to BE the safe reading, and a client that never heard
of exclusions sends `false` — an include. A `string mode` leaves two ways to
lose: reject `""` and break every old client, or coerce `""` to include and
coerce a misspelt `"exlcude"` along with it, turning a carve-out into a
**widening with no error anywhere**. The bool→text mapping lives in the SQL of
`InsertGrantScope`, so exactly one place can produce the string.

## Proven, not assumed

- **0 changed decisions** across 6,000 probes replayed either side of the
  migration (exact target, inherited descendant, no targets at all).
- `bench/negative.sql` case 13b was written against the existing tables, matched
  zero rows on a database with no memberships, committed cleanly and printed
  nothing. `bench/rebuild.sh` now counts CASES WITH NO ERROR instead of ERROR
  lines (its tally said "/9" against a file of twenty) — a negative test that
  passes by doing nothing looks identical to one that passes by being enforced.
- `scope_excluded` is its own deny reason. `explainDenial`'s message was
  "no grant at or above the requested X node", which is **false** of an
  exclusion and sends whoever reads it to widen a grant that already covers the
  node.

## The console could not reach the feature until child_count existed

Found by opening the drawer, not by reading the code. `ScopeTree` gates its
expand chevron on `(node.child_count ?? 0) > 0`, and `child_count` was declared
**optional** in `ui/src/lib/api/types.ts` and set by nobody — no proto field, no
query, no mapper. So it was `undefined` for every node ever loaded, the chevron
was permanently disabled, and NO tree in the console could open. The picker
could only ever offer the axis root; `routes/scope.tsx` reported "0 children"
for a node with twenty.

Nothing failed. No error, no console warning, no red test — the tree rendered,
it just rendered one row. A carve-out needs a child node to carve, so the whole
of 0046 was unreachable through the UI until this was fixed.

`child_count` now comes from the query (`ListScopeNodes`, `GetScopeNode`,
`GetScopeNodeByRef`, `ScopeNodesByIDs`) and is **not optional** in the TS type:
an `undefined` that quietly reads as 0 is exactly how it shipped. The list
counts what THAT CALL would return (same `include_archived`), because a count
that disagrees with the listing draws a chevron expanding to nothing. Costs a
per-row index scan on `scope_nodes_sibling_slug (parent_id, slug)`: 0.154 ms →
0.393 ms for a 200-row page of the 20k-node customer axis.

`TestScopeNodeChildCountMatchesTheRows` asserts it through the repository, not
the SQL — a test written against the expression would pass with the column
dropped from the query. Verified failing on the old behaviour: "ListScopeNodes
reported child_count=0 ...; the table holds 20 children".

## Counting a carve-out as a place

Three summaries counted `scopes.length` and so read a carve-out as one more
place access reaches -- the opposite of what it is. All three now count the
exclusions apart: the "Access given" notification (`1 axis constraint, 1 place
carved out`), the membership drawer's entry list (`1 place, except 1`), and the
axis row's own chip. `GrantAccess` and the memberships list join includes with
"or" and render exclusions after an italic "except", never inside the OR -- on
those screens the sentence is what an auditor takes at face value.

Two of these were only found by opening the screen. The memberships list had
never rendered an exclusion at all, because the dev database holds zero
memberships.

## Related
[[tenant-scoped-reads]] · [[console-create-drawers]] · [[console-tables]]
