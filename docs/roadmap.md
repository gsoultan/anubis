# Roadmap

## Status

| Layer | State |
| :--- | :--- |
| **Database schema** | ✅ Built, benchmarked, validated — 20 migrations |
| **Authorization engine** | ✅ `authorize()` + `authorize_explain()`, suites passing |
| **Schema-enforced invariants** | ✅ 9 guards, all verified rejecting illegal writes |
| **Go application layer** | ✅ repositories→usecases→services→endpoints→transports (ADR-0008/0009) |
| **Transports (Connect/gRPC/HTTP)** | ✅ Connect RPC (serves gRPC + gRPC-Web too) + stdlib OIDC/gate surface |
| **Client SDK** | ✅ `pkg/anubis` (zero-dep verifier + middleware) · TS client in `ui/src/lib` |

Phases 0–3 are implemented and e2e-tested (login/MFA/device flows, refresh
rotation with theft detection, logout×3 with back-channel, introspect/revoke,
authorize/explain/scope-switch, admin plane, audit hash chain). Phase 4
partially (self-registration, consents, erasure request; crypto-shredding key
management still open). Phase 5 ✅ (API keys). Phase 6 ✅ (TOTP + device keys;
step-up via amr/auth_time). Phase 7 ✅ (gate + snapshot + shared normalisation
corpus + fuzz). Phase 8 partially (gRPC via Connect on day one; key prepare/
promote lifecycle; Envoy ext_authz and revocation streaming remain).

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

Performance budgets are enforced the same way — by tests, not prose. See
[operations.md](operations.md#performance-budgets).

## Remaining gaps

Stated plainly, as the earlier list was.

| Gap | Status |
| :--- | :--- |
| **Field-level PII encryption** | Key lifecycle, sealing API and shredding are built (`migrations/0022`, `internal/identity/domain/pii`). No column is encrypted yet: the schema keeps free-form PII only in `identities.attributes`, which the API does not currently write |
| **Enrol-or-deny rollout** | A realm requiring TOTP still admits a password-only login from someone not yet enrolled. An *enrolled* factor is always demanded. Closing the remaining gap locks out existing users the moment policy flips, so it is a rollout decision, not a default |
| **Envoy `ext_authz`, revocation streaming, JWS codec flag** | Phase 8 tail. Connect already serves gRPC, so these are integrations rather than new mechanisms |
| **Console still on its mock backend** | The typed client exists (`ui/src/lib/anubis.ts`) and typechecks; the screens have not been rewired to it |
| **Redis-backed rate limits** | Deliberate. In-memory counters are per-instance; sharing them across replicas needs an ADR first (ADR-0008 posture: no new infrastructure until deployment demands it) |
| **Bot protection on public registration** | Rate limits bound the damage; they do not stop a determined script |
