# `if _, err := guard.Require(...)` is the tell (2026-09-13)

An admin RPC that takes an id has two questions to answer and the guard only
answers one. `Require` says **may this caller read identities?** It never says
**is THIS id theirs?** That second question belongs to the query, and the query
can only ask it if the interactor passes the tenant down.

So wherever an interactor writes `if _, err := u.guard.Require(...)` it has
thrown the principal away, the tenant cannot reach the query, and the id is
taken on trust. **That discard found every case.** It is a better detector than
scanning SQL, and the audit that followed [[identity-directory-reads]] used it.

## Three confirmed, all fixed

| Call | Leaked |
| :--- | :--- |
| `ScopeAncestors` | another tenant's scope-node names — the shape of their organisation |
| `ListCredentials` | their inventory of how their people sign in (kind, label) |
| `GetRoleEffective` | their permission keys and the role names conferring them |

Each: principal discarded, id straight from `req.Msg`, query filtering on a
child id over a tenant-scoped table. `GetRoleEffective` is the sharpest — two
declarations below it in the same file, `ListRolesUsingPattern` has always
filtered `r.tenant_id`.

`test/e2e/tenant_isolation_test.go` plants a whole foreign tenant (realm,
identity + credential, application + permission + role + an
`role_permissions_effective` row, a two-node scope tree with closure) and hands
its ids to an operator of another tenant.

## Scanning SQL is the weaker detector — twice over

1. `ListCredentials` **SELECTs** `tenant_id` and filters on `identity_id`
   alone. A grep for `tenant_id` in the query body says it is fine. Check the
   **WHERE**, not the statement.
2. A missing filter is not always a leak. Safe shapes found in the same pass:
   the parent is resolved in-tenant first (`ApplicationBySlug(p.TenantID, …)`
   then `app.ID`; `IdentityRecordByID(p.TenantID, id)` before the credential
   read), the id is the caller's own (`p.IdentityID`), the table is not
   tenant-scoped at all (`scope_axes`, `scope_node_types`, `signing_keys`,
   `tenants`), the permission is installation-only (`anubis:tenant:admin` is in
   `installationPermissions`, so `TenantStats` is fine), or the key IS the
   secret (`GetSessionByCookieHash`, `GetRefreshTokenByHash`,
   `GetAPIKeyByLookup`).

## An isolation test needs a positive control

Adding the filter to `ListCredentials` first left the repository passing an
**empty** tenant id — signature changed, body not. Every isolation assertion
still passed: an empty uuid matches nothing, which from outside is
indistinguishable from "correctly refused". A security test that cannot tell a
working filter from a broken query will wave the regression through. Each test
now also proves the operator can still read their OWN tenant.

## Unrelated flake fixed in passing

`fmt.Sprintf("kindprobe%d", time.Now().UnixNano()%1e6)` — this clock has
**microsecond** granularity, so `UnixNano()` always ends in `000` and the
modulo left one thousand codes, not a million. The probe realms are never
cleaned up, so `CreateRealm` eventually hit a duplicate key. Full nanoseconds
now (`ValidCode` allows 31 chars, so it fits). Same bug was in `mfa_test.go`
twice. See [[test-timing-and-tooling]] for the other flakes in this suite.
