# Roadmap

## Status

| Layer | State |
| :--- | :--- |
| **Database schema** | ✅ Built, benchmarked, validated — 32 migrations |
| **Authorization engine** | ✅ `authorize()` + `authorize_explain()`, suites passing |
| **Schema-enforced invariants** | ✅ 9 guards, all verified rejecting illegal writes |
| **Go application layer** | ✅ repositories→usecases→services→endpoints→transports (ADR-0008/0009) |
| **Transports (Connect/gRPC/HTTP)** | ✅ Connect RPC (serves gRPC + gRPC-Web too) + stdlib OIDC/gate surface |
| **Client SDK** | ✅ `pkg/anubis` (zero-dep verifier + middleware) · TS client in `ui/src/lib` |
| **Admin console** | ✅ Every screen on the real API; embedded in the binary and served same-origin |
| **Control plane** | ✅ Platform users, operators, installer, machine credentials (ADR-0011) |
| **CI + observability** | ✅ Enforcement gates, integration/e2e/fuzz/load in CI; Prometheus metrics + alert rules |

Phases 0–3 are implemented and e2e-tested (login/MFA/device flows, refresh
rotation with theft detection, logout×3 with back-channel, introspect/revoke,
authorize/explain/scope-switch, admin plane, audit hash chain). Phase 4
partially (self-registration, consents, erasure request; crypto-shredding key
management still open). Phase 5 ✅ (API keys). Phase 6 ✅ (TOTP + device keys;
step-up via amr/auth_time). Phase 7 ✅ (gate + snapshot + shared normalisation
corpus + fuzz). Phase 8 partially (gRPC via Connect on day one; key prepare/
promote lifecycle; Envoy ext_authz and revocation streaming remain).

Since then, and not in the original phase plan: the platform control plane
(ADR-0011) with its own sign-in, refresh and machine credentials; the
installer; bulk import; the sign-in page builder; and the operational layer
this system had been missing — a real CI pipeline, `/metrics` with alert
rules, and a load harness that says what one instance actually sustains.

[api.md](api.md) is now **implemented**, not specification. What remains
unbuilt is listed under [Remaining gaps](#remaining-gaps) at the end of this
document.

---

## Phase 0 — Foundation

Configuration, `log/slog`, embedded migration runner with advisory locking,
error envelope, request-ID propagation, health and readiness probes.

> The migration runner comes first. Without it there is schema drift by week
> three.

## Phase 1 — Minimum viable identity

Identities, realms, password login, sessions, PASETO issuance, key discovery,
refresh rotation with reuse detection — plus **`pkg/anubis`, the client SDK**.

**This is the milestone that unblocks the first consuming application.**

Ship the SDK in the same phase. Without it, every team writes its own verifier
and one of them will skip the `aud` check. The SDK is how correctness is enforced
across the organisation, and it is the highest-leverage code in the project.

If any consumer is a browser application, this phase also carries the
authorization code flow with PKCE, the SSO session cookie, a hosted login page,
and exact-match `redirect_uri` allowlisting — see
[ADR-0006](adr/0006-path-protection.md#consequence-browser-flows-become-mandatory).

## Phase 2 — Authorization service

Scope axes, roles, permissions, grants, `/v1/authorize`, `/v1/authorize/explain`,
scope switching, manifest registration.

The database work is done; this phase exposes it.

> `/v1/authorize/explain` is not optional once more than two axes exist. Every
> IAM system lacking one generates a support ticket per week forever.

## Phase 3 — Session lifecycle

All three logout variants, `/introspect`, `/revoke`, hash-chained audit log.
Back-channel logout moves here if browser SSO shipped in Phase 1 — with browser
SSO it is required, not optional.

## Phase 4 — External populations

Partner self-service portal, delegated administration, applicant
self-registration with email verification, consent capture, retention sweeper,
PII crypto-shredding key management.

The schema is ready ([ADR-0007](adr/0007-external-identities.md)); the key
management for crypto-shredding is the one genuinely unbuilt piece.

## Phase 5 — Machine identity

API keys (`anb_live_<prefix>_<secret>`, indexed prefix, hashed secret), client
credentials for service-to-service.

## Phase 6 — Strong authentication

Device-bound biometric keys — challenge → Secure Enclave signature →
`ed25519.Verify`. Roughly 150 lines once the credential abstraction exists, which
is exactly why it lands here rather than in Phase 1: building it early means
building it twice.

Then TOTP MFA and step-up authentication via `amr`/`auth_time`.

## Phase 7 — Path protection

`/v1/gate/check` with the in-memory snapshot, nginx/Traefik/Envoy integration
configs, the shared path-normalisation corpus and fuzz target.

## Phase 8 — Scale and operations

gRPC transport, automated key rotation, admin API, the JWS codec flag
([ADR-0001](adr/0001-token-format.md)), Gate sidecar.

---

## Deliberately deferred

| Item | Reason |
| :--- | :--- |
| **Deny rules** | Allow-only union semantics in v1. Deny needs strict precedence and an explain endpoint, and getting it wrong produces outages that look like security incidents. |
| **General ABAC** | `self_scoped` covers the dominant external-user case. Full attribute predicates are unindexable and unauditable. |
| **OIDC certification** | The protocol is OIDC-*shaped* so standard clients remain possible, but conformance testing is only worth it if external third parties integrate. |
| **Social login** | Applicants may want it. `credentials.kind = 'oidc_link'` reserves the slot. |
| **Argon2id** | Requires `x/crypto`. PBKDF2 hash format supports transparent migration later. |


---

## Formerly unproven claims — now closed

Each of these was asserted by a document and verified by nothing, which is the
same as not having the property. Every one now has a test that fails if the
property regresses (`go test -tags integration ./test/integration/`).

| Claim | Closed by | Result |
| :--- | :--- | :--- |
| `scope_move_node` is safe under concurrency | `TestConcurrentSubtreeMovesKeepClosureConsistent` | 6 workers moving one subtree; the closure is recomputed from `parent_id` and compared. 14 serialisation retries observed, 0 divergence — an *extra* closure row would have been a privilege leak |
| `role_recompute_effective` handles deep role graphs | `TestDeepRoleGraphRecompute`, `TestCyclicRoleGraphTerminates` | 60-level inheritance propagates in ~3 ms; a 3-role cycle terminates instead of hanging |
| Snapshot loading is torn-read free; the gate agrees with the engine | `TestSnapshotAgreesWithAuthorizeEngine` | 404 probes replayed through both the in-memory evaluator and `authorize()`: **0 disagreements**. The loader asserts `REPEATABLE READ` at runtime |
| Path normalisation matches between gate and app | `internal/gate/routepath` corpus + `FuzzNormalizePath` | Shared corpus; it found two real bypasses (percent-encoded dot-segments, overlong UTF-8) that are now fixed |
| Uniform login timing | `TestLoginTimingDoesNotRevealUserExistence` | Existing vs unknown user: median 49.2 ms vs 48.8 ms — 0.6% apart, both paying full KDF cost |
| Backup restore cannot resurrect access | `TestBackupRestoreCannotResurrectAccess` | A restored refresh row stays useless: its session is revoked and `token_epoch` has advanced |
| The dependency policy is enforced, not just written | `scripts/check/deps.sh` | ADR-0002 promises no third-party libraries beyond infrastructure drivers and zero third-party cryptography. Nothing checked it, so it described the dependency list as of whenever somebody last read the ADR. The gate fails on a requirement outside the allowlist, on an import of third-party crypto, and on an untidy `go.mod` — each verified by introducing it. **Finding: `go.mod` was untidy**, marking `go-sql-driver/mysql` `// indirect` while `cmd/anubisd/syncengines` imported it directly, which is how a dependency enters a tree without anyone weighing it |
| The runtime cannot exceed its database privilege | `TestDatabaseRolesCannotExceedTheirPrivilege` | Every row of the privilege table in [operations.md](operations.md#database-roles), asserted as a real login role holding only the group it names: the runtime may read and append `audit_log` but not rewrite, delete or truncate it; may not CREATE, DROP or ALTER; and the reporting role may not read `credentials`, `signing_keys`, `refresh_tokens`, `one_time_tokens` or `pii_keys`. All held when first measured — the point is that they keep holding, since one widening GRANT in a later migration would undo them silently. Verified by granting `UPDATE ON audit_log` and `SELECT ON credentials` and watching it name both |
| Least privilege on the database is enforced, not described | `migrations/0037`, exercised by the whole backend suite | **Finding: the grants in 0023 decided nothing.** They gave `USAGE ON SCHEMA public` explicitly to `anubis_app` and `anubis_readonly` while leaving `PUBLIC`'s implicit grant in place, so every role in the database reached the schema regardless. Found by diffing a freshly migrated database against the development one, which had the revoke applied by hand and had been running with it. Measured after: a role holding `anubis_app` reads normally, a role holding nothing gets `relation "tenants" does not exist`. It closes the data, not the catalog — `pg_tables` still lists names, as in every Postgres |
| Every audit event the suite generates is written | `scripts/ci/backend-suite.sh` reads `anubis_audit_dropped_total` after the run | The general form of the two audit findings below. Hundreds of admin calls across every bounded context, and the suite fails if one lost its record. `authorize` is excluded and only `authorize`: it is allowed to drop rather than block, since back-pressure there would turn a slow database into a slow `authorize()` — its drops are a capacity alert instead. **Finding while wiring it: the authorize drop counter was a bare `++` on a field nothing read**, executed on request goroutines. A data race, proven under `-race`, now an atomic on the same metric |
| A realm that requires a factor can enforce it | `TestEnrolOrDenyRollout`, `TestAGrantCannotReplaceAnEnrolledFactor`, `internal/identity/domain` stance suite | The switch is a date, not a flag, because a flag is a lockout: `realms.factor_enrolment_deadline`. The e2e test walks all four states on a live server — no deadline (unchanged behaviour, which is what stops an upgrade locking out every existing realm), inside the grace period (signed in *and* warned), past it (refused, with a grant token), and after enrolling with that grant (challenged for the factor they now hold). The grant is refused if a factor is already enrolled, so a leaked one cannot replace an authenticator |
| The platform plane is audited at all | `TestFailedPlatformLoginIsRecorded` | **Finding: it was not, and not one line of it.** `audit_log.tenant_id` is `NOT NULL`, a platform user belongs to no tenant (ADR-0011), and the auditor returned early on a tenant-less event — before even counting it. So no platform login, failed login, API-key creation, logout or MFA enrolment was ever recorded, and `token.reuse_detected`, the event [alerting.md](alerting.md) calls the highest-signal in the system, could not fire for an operator. The installation now files under a reserved tenant id of its own, with its own hash chain, readable and verifiable through the ordinary audit API when no tenant is selected |
| Administrative changes are actually recorded | `TestAdminActionsLeaveAnAuditTrail` | Creates an axis and a node type, then requires both to appear in the audit log and the dropped-event counter not to move. **Finding: neither was ever recorded.** `audit_log.target_id` is a uuid and an axis is identified by a code, so the insert failed on every axis and node-type creation since the code was written — and the auditor logged the failure to a stream nobody reads. Codes moved into `detail`, and dropped events now increment `anubis_audit_dropped_total`, which [alerting.md](alerting.md) pages on |
| Field-level PII encryption protects something | `TestIdentityAttributesAreSealedInTheDatabase`, `internal/identity/domain/pii` envelope suite | `SetIdentityAttributes` seals the whole map — values *and* field names — under a key minted for that identity alone. The test writes a diagnosis code through the API, then opens Postgres the way a stolen dump would and requires that none of it is readable; that the key itself is stored sealed under the master key; that plaintext is refused by the 0035 constraint even from raw SQL; and that after a shred the data is gone. **Finding: writing to an erased identity silently resurrected it** — `pii_shred` nulls the foreign key, so an erased row looked identical to a new one. The leftover ciphertext is now what marks it erased, on both the read and write paths |
| Structure can be synced from any SQL engine, not just Postgres | `TestReadsAStructureOutOfMySQL`, `TestScopeSyncFromMySQL` | A three-level org chart read out of MySQL — different DSN grammar, backtick quoting, no `NULLS FIRST` — and landed through the real RPC with its hierarchy intact. Each fixture names a child whose primary key sorts *ahead* of its parent, so ordering is decided by the dialect rather than by luck. **Finding: the first version passed by accident**, reading a table a person had typed by hand into one container; both tests now build their own fixture, and CI runs a `mysql:8` service so they cannot skip in silence |
| The audit chain is tamper-evident | `TestAuditChainDetectsAnEditedRow`, `…DeletedRow`, `…VerifiesWhenIntact` | Editing a `deny` into an `allow` is caught at that row; deleting an entry is caught at the next. **Finding: it did not work at all.** `detail` is `jsonb`, so Postgres re-renders what it stores, and the hash was taken over the bytes the writer sent — 21,424 of 21,439 real entries read as tampered. Both sides now hash a canonical form; that same history verifies |

Performance budgets are enforced the same way — by tests, not prose. See
[operations.md](operations.md#performance-budgets).

## Remaining gaps

Stated plainly, as the earlier list was.

| Gap | Status |
| :--- | :--- |
| **Envoy `ext_authz`, revocation streaming, JWS codec flag** | Phase 8 tail. Connect already serves gRPC, so these are integrations rather than new mechanisms |
| **Redis-backed rate limits** | Decided against for now, with trigger conditions: [ADR-0012](adr/0012-rate-limits-across-replicas.md). Limits are per instance and the docs say so |
| **Bot protection on public registration** | Decided against, with the escape hatch documented: [ADR-0014](adr/0014-bot-protection-on-registration.md) |

Nothing above is an unproven claim. Two are decisions with their reasoning
written down, and the third is an integration the transport already supports.

## What is left is not code

The engineering gaps are closed. What Anubis has not had is the thing no test
can supply:

| | |
| :--- | :--- |
| **A real deployment** | Every drill — soak, restore, two replicas on one database, package upgrade — ran on a laptop against containers. That is enough to find the bugs it found; it is not the same as a host behind real TLS with real DNS and a real backup schedule. [operations.md](operations.md) is the runbook to do it with. |
| **An external security review** | The crypto is stdlib, the decisions are written down, and the properties are tested — by the people who wrote them. Somebody who did not write it should read the token paths, the guard and the gate. |
| **Somebody else's traffic** | The load figures are synthetic and the correctness assertions under them are not. Neither substitutes for a week of a real directory signing in. |
