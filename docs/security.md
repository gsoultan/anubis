# Security model

## Contents

1. [Threat model](#threat-model)
2. [Controls by threat](#controls-by-threat)
3. [Cryptography](#cryptography)
4. [Secrets handling](#secrets-handling)
5. [Schema-enforced invariants](#schema-enforced-invariants)
6. [Known gaps](#known-gaps)

---

## Threat model

**Assets, in order of value:**

1. **Signing keys** — compromise is total. The attacker mints valid tokens for
   any user in any scope, and the audit log shows nothing wrong.
2. **Password hashes and credentials** — offline cracking, credential stuffing.
3. **Refresh tokens** — long-lived session takeover.
4. **The grant graph** — silent privilege escalation.
5. **The audit log** — attacker covering tracks.

**Populations change the threat model.** Anubis authenticates employees,
external business partners and self-registered public users. Public registration
means an unauthenticated attacker can create an account and become an
*authenticated* adversary at will. Delegated administration means grant
administration is no longer performed only by trusted IT staff.

**Assumed adversaries:**

| Adversary | Capability |
| :--- | :--- |
| Unauthenticated internet attacker | Reaches public endpoints |
| Credential-stuffing operator | Large valid credential dumps |
| Authenticated low-privilege user | Valid token, seeking escalation |
| **Self-registered public user** | **Can create accounts freely; seeks any grant beyond their own record** |
| **Delegated partner administrator** | **May create identities and grants within their partner scope** |
| Compromised relying application | Holds a client secret, valid tokens for its users |
| Read-only database access | Stolen backup, SQL injection elsewhere |
| Malicious insider with DB write | Attempting silent grant or history modification |

**Explicitly out of scope:** physical access to running hosts, a compromised
KMS/HSM, and a malicious Postgres superuser.

---

## Controls by threat

### User enumeration

The login endpoint must not reveal whether an account exists.

- Identical error code, message and HTTP status for unknown user and wrong
  password
- **The KDF runs even when the user does not exist**, compared against a fixed
  dummy hash, so response timing matches
- Registration and password-reset endpoints return success regardless

> This is invisible in functional testing and visible in a timing histogram. Test
> it with a histogram, not an assertion.

### Verifying the audit log

`VerifyAuditChain` walks a tenant's entries recomputing each hash from the
one before, and reports the first sequence where the chain breaks. Editing an
entry is caught at that entry; deleting one is caught at the entry after it.
Both are asserted by tests that actually tamper with rows as an attacker with
database write access would.

Two limits worth stating plainly. An attacker who rewrites an entry **and**
every entry after it produces a self-consistent chain — the defence is that
this is no longer silent, not that it is impossible; anchoring the head hash
somewhere outside the database is what would close it. And the first entry in
a queried range anchors the walk, so narrowing the range narrows what can be
checked; verify from the beginning when it matters.

The hash covers a CANONICAL rendering of `detail`, not the bytes the writer
happened to send. `detail` is `jsonb` and Postgres re-renders what it stores,
so hashing the sent bytes produced a hash no reader could reproduce: before
this was fixed, 21,424 of 21,439 entries in a real database reported as
tampered, which is a verifier nobody would keep believing.

### Knowing what we ship, and whether it is exposed

Every release publishes an SBOM per archive, and `scripts/check/vulns.sh`
runs `govulncheck` over BOTH modules on every push — the service and the
`pkg/anubis` verifier SDK, which consuming applications compile into
themselves and where a vulnerability therefore travels further.

govulncheck is deliberately narrower than a dependency scanner: it reports a
vulnerability only when the vulnerable symbol is reachable from this code, so
a CVE in a function nothing calls does not train anyone to ignore the output.

A newly published advisory fails the gate on a push that changed nothing.
That is intended — the exposure is real whether or not the commit caused it.
Dependabot proposes the bump; the gate says whether it is urgent.

### Credential stuffing and brute force

- Rate limits on **three axes** — per IP, per account, per tenant. Per-account is
  the one people forget and the one that matters: it keys on the *target*
  account, which an attacker cannot rotate.
- Exponential backoff plus lockout with a documented unlock path
- Counters live **in the process**, sharded and bounded with eviction
  (`internal/platform/ratelimit`), so the auth database never absorbs attack
  traffic. They are therefore **per instance**: N replicas enforce N times
  the published allowance, which
  [ADR-0012](adr/0012-rate-limits-across-replicas.md) accepts on purpose and
  bounds with trigger conditions. (This document previously said the counters
  were in Redis. They never were.)
- Failed-login rate spikes alert — and across replicas that alert, not the
  counter, is what catches a distributed campaign ([alerting.md](alerting.md))

### Token theft

| Token | Defence |
| :--- | :--- |
| Access | Short TTL (5–15 min) bounds the window. `sid` enables session revocation; `token_epoch` is the global kill switch. |
| Refresh | **Rotation with reuse detection.** Single-use; presenting a consumed token revokes the whole family. Optional proof-of-possession (`bound_key`) makes a stolen token useless without the private key. |
| Internal state | AEAD-encrypted, 60-second TTL, single-use nonce with **atomic** consumption. |

Reuse detection is the highest-signal alert in the system: it means a token was
stolen. If the attacker uses it, the user's next refresh trips the alarm; if the
user refreshes first, the attacker's use trips it. Either way the compromise is
detected rather than quietly renewed forever.

### Confused deputy — cross-service token replay

**Every token carries `aud`.** Without it, a token minted for the HR application
is accepted by the payments application. Every application must reject tokens
whose `aud` does not include itself — `pkg/anubis` enforces this, which is the
main reason to ship an SDK rather than let each team write a verifier.

### Algorithm confusion

Structurally impossible: PASETO's version *is* the cipher suite, with no
negotiable `alg` field. The JWS codec pins `Ed25519` and never reads
`alg` from the token.

### Key-lookup abuse

> `kid` may only index a **pre-loaded, bounded, in-memory map**. Never a database
> query, never a filesystem path, never a network fetch. Unknown `kid` → reject
> with zero I/O.

Attacker-controlled input driving a lookup is how you get injection and denial of
service. This applies equally to PASETO's footer and JWT's header — PASETO does
not save you here.

### Privilege escalation via scope

- Cross-tenant and cross-axis grants are **impossible to insert** (composite FKs)
- **Fail-closed evaluation**: an axis the caller did not resolve is a denial, not
  an omission
- Wildcards expand at write time, so a newly registered permission cannot
  silently widen an existing role
- `via_role_id` gives provenance for every effective permission

### Privilege escalation via delegated administration

Once partners administer their own users, two escalation paths open. Both are
closed by **constraint triggers**, so a script bypassing the application layer
still cannot exploit them:

| Attack | Control |
| :--- | :--- |
| Attach an employee-only role to a public account | `roles.allowed_realm_kinds` + `grants_realm_guard` |
| Confer a role you were never authorised to confer | `role_grantable` |
| Attach axis constraints to a self-scoped grant | `grant_scopes_self_guard` |

**Verified: 2/2 rejected.** The guard also caught a genuine bug in our own seed
script.

### Grant misadministration

Delegated administration makes grant errors more likely, so `authorize()` does
not trust grants alone:

- **`permissions.min_assurance`** — a self-registered IAL1 applicant cannot
  approve a purchase order even if a grant says otherwise
- **Identity state** — `disabled` or `anonymized` identities are denied
  regardless of grants, so deprovisioning is one field rather than a grant sweep
- **`self_scoped` + `_owner`** — external users see only their own records, and a
  missing `_owner` is a denial

### Personal data of external users

Applicant data cannot be kept indefinitely (UU PDP No. 27/2022 and equivalents),
which conflicts with "never delete, for audit."

**Resolution: crypto-shredding.** PII is stored under a per-identity key
(`pii_key_id`). Deleting the key makes the data unrecoverable while rows and
referential integrity survive, keeping the audit trail and grant graph
consistent. `anonymized_at` denies authorization from that moment.

`consents` is append-only — a withdrawal is a new row, so the record of what was
consented to survives the withdrawal.

### Open redirect

`redirect_uri` is matched **exactly** against a per-application allowlist. No
wildcards. No prefix matching. No subdomain matching. Open redirect in an SSO
service is full account takeover.

### Path-based bypass

Normalise before matching; reject anything still ambiguous. The gate and the
application **must normalise identically** — the gap between two normalisers is
the bypass. See
[ADR-0006](adr/0006-path-protection.md#path-normalisation-is-the-security-critical-part).

### Audit tampering

`audit_log` is append-only and **hash-chained** — each entry embeds the previous
entry's hash. An attacker with `UPDATE` rights cannot silently rewrite history;
the chain breaks and verification detects it. Ship to append-only storage with
object lock.

### Backup restore resurrecting revoked tokens

Restoring `refresh_tokens` brings revoked tokens back to life. `token_epoch` on
`identities` is the mitigation — but only if restored consistently **and** if
applications actually validate it. **Test this explicitly**; nobody discovers it
until it matters.

---

## Cryptography

Zero third-party cryptography. See
[ADR-0002](adr/0002-dependency-policy.md).

| Purpose | Algorithm | Source |
| :--- | :--- | :--- |
| Access token signing | Ed25519 (PASETO `v4.public`) | `crypto/ed25519` |
| Internal token AEAD | AES-256-GCM (`anb.local.v1`) | `crypto/aes`, `crypto/cipher` |
| Key derivation | HKDF-SHA256 | `crypto/hkdf` |
| Password hashing | PBKDF2-HMAC-SHA256, ≥600k iterations | `crypto/pbkdf2` |
| Token / API-key hashing | SHA-256 | `crypto/sha256` |
| Randomness | **`crypto/rand` only** | `crypto/rand` |
| Comparison | Constant-time | `crypto/subtle`, `hmac.Equal` |

**Rules with no exceptions:**

1. `math/rand` is banned repository-wide. Lint it. This mistake has shipped in
   real auth systems.
2. Every secret comparison uses `subtle.ConstantTimeCompare` or `hmac.Equal`.
   Never `==`.
3. High-entropy secrets (refresh tokens, API keys) use SHA-256 — a slow KDF is
   for *low-entropy* passwords and would only add latency here.
4. Password hashes store algorithm and parameters inline so the KDF can be
   upgraded by rehashing on next successful login.

---

## Secrets handling

| Secret | At rest | Notes |
| :--- | :--- | :--- |
| Signing private keys | Encrypted with a KMS-held master key | **Never in Git. Never in a plain CI variable** — masked variables are not a security boundary. |
| Password | PBKDF2 hash | |
| Refresh token | SHA-256 hash | Plaintext is never stored |
| API key | SHA-256 hash + indexed prefix | Prefix enables lookup without storing the secret |
| TOTP shared secret | Encrypted | |
| Recovery codes | Hashed, single-use | |

Add a secret-scanning rule for `anb_live_`, `v4.public.` and `anb.local.v1.` to
the existing GitLab SAST configuration.

---

## Schema-enforced invariants

Seven security-critical invariants live in the schema. All are
[tested](../bench/negative.sql) — **7/7 rejected by the database**.

| # | Illegal write | Consequence if allowed |
| :--- | :--- | :--- |
| 1 | Grant → another tenant's scope node | **Cross-tenant privilege leak** |
| 2 | Grant claims `axis=product`, points at an org node | Unsatisfiable or wrongly satisfiable |
| 3 | Node parented across axes | Corrupt hierarchy, unpredictable inheritance |
| 4 | Node type from a different axis | Nonsense hierarchy |
| 5 | Permission `app_slug` forged | Permission key spoofing |
| 6 | Two axis roots for one (tenant, axis) | Ambiguous "unrestricted" |
| 7 | Cycle in the scope tree | Infinite inheritance |

> The schema is what is still true after someone writes a bad migration script at
> 2am. Invariants that are security-critical belong there, not in a code-review
> checklist.

---

## Known gaps

Stated plainly rather than left implicit.

| Gap | Status | Mitigation |
| :--- | :--- | :--- |
| **PBKDF2 rather than Argon2id** | Accepted | Standard library only. ≥600k iterations. Hash format supports transparent migration. |
| ~~Concurrent subtree moves unproven~~ | **Proven** | `scope_move_node` asserts `SERIALIZABLE`, and `TestConcurrentSubtreeMovesKeepClosureConsistent` runs concurrent moves against a real database and checks the closure afterwards. |
| ~~Deep role graphs unproven~~ | **Proven** | `TestDeepRoleGraphRecompute` propagates a permission 60 levels; `TestCyclicRoleGraphTerminates` shows the `CYCLE` clause ends rather than hangs. |
| **No deny rules** | Deliberate | Allow-only union semantics in v1. Adding deny needs strict precedence and an explain endpoint. |
| **No PASETO ecosystem support** | **Addressed** | `pkg/anubis/jws` behind `applications.token_format = 'jws.eddsa'`. EdDSA is compiled in and `alg` is never read to choose anything, so the algorithm-confusion family JOSE is known for is absent by construction; `crit` is refused, a five-part (JWE) token is refused, and `kid` rides in the protected header. Stdlib only. |
| **Audit chain catches a rewrite** | **Built** (2026-09-20) | The hash chain alone catches an entry edited in place or deleted; it does not catch a rewrite that recomputes every forward hash, which is self-consistent by construction. `audit_anchors` (migration 0049) signs each tenant's chain head hourly with the access key, sealed under the master — so a rewrite must also reproduce a hash somebody signed, which the database alone cannot do. `VerifyAnchors` reports the first anchored sequence that no longer matches. |
| **PII crypto-shredding** | **Built** | `migrations/0022` + `internal/identity/domain/pii`: per-identity keys sealed under the master key, erasure and the retention job destroy the key and leave a tombstone (the *fact* of erasure stays auditable). No column is encrypted yet; [ADR-0013](adr/0013-pii-encryption-scope.md) decides the scope — `identities.attributes` is sealed, identifiers deliberately are not, and the API must first write `attributes` at all. |
| **Self-registration abuse controls** | Partly built | Email verification and per-IP/per-tenant registration limits ship; bot protection (CAPTCHA/attestation) does not. |
| **Enrol-or-deny for required factors** | **Built** | `realms.factor_enrolment_deadline` is a date rather than a flag, because a flag is a lockout: before it, an un-enrolled member signs in and is warned; after it, they are refused and handed a grant token to enrol with. `Realm.EnrolmentStanceFor` decides, and both login doors call it. [enrolment-rollout.md](enrolment-rollout.md). |
| **"An enrolled factor is always demanded" was true of ONE door** | **Fixed** (2026-09-20) | It described `AuthService.Login`. The hosted page at `POST /v1/login` is a second implementation of login and had no factor check at all, so a stolen password skipped an enrolled authenticator by using the browser instead of the API. Both doors now run the same rule, and `TestBrowserLoginRefusesPasswordAloneOnceEnrolled` fails on the old code. Worth remembering when reading any row in this table: a property stated once can still be true of only one of the surfaces that implement it. |
| **The enrolment deadline was true of ONE door too** | **Fixed** (2026-09-20) | The fix above closed the *enrolled* half and left the other: a realm past its `factor_enrolment_deadline` refused an un-enrolled member at the API and signed the same member in through the hosted page. `EnrolmentStanceFor` had exactly one caller in the tree. So the policy was void for the population that uses a browser, which is most humans, and [roadmap.md](roadmap.md) listed the property as closed. `TestBrowserLoginHonoursTheEnrolmentDeadline` fails on the old code. |
| **Two login doors, one implementation** | **Fixed** (2026-09-20) | The two rows above are the same defect twice, so the third fix was structural rather than another check. `POST /v1/login` re-derived the whole of login — tenant, realm, identity, credential, KDF comparison, factor policy — beside `AuthService.Login`. Both now call one `signin.PasswordAuthenticator`, wired as a single instance in the composition root: it decides *whether* a password gets in, and each door only decides what it then issues (a token pair, or a cookie and an authorization code). Three properties were fixed by construction in the move: a failed browser sign-in is now audited (**it never was** — the page's failure branch rendered an error and returned, so wrong passwords typed into the page humans use left no trace at all), a browser sign-in now rehashes a password whose KDF parameters are out of date, and the deny events from both doors carry the surface they arrived at. |
| **Operator login revealed which operators exist** | **Fixed** (2026-09-20) | The control plane is a third implementation of password login, and the only one of three `kdf.Verify` call sites that did not default its hash to `kdf.Dummy()`. `kdf.Verify` parses the encoded hash before deriving anything, so the empty string an unknown operator produced returned instantly: **51.9 ms for an operator who exists, 0.35 ms for one who does not**. The comment above the call claimed the opposite — "*verify even when nobody was found, so a missing account and a wrong password cost the same time*" — and it did call Verify, against nothing. Platform operators are the highest-privilege accounts in the system and their usernames are what precedes a password-spray. Fixed inside `kdf.Verify`: an absent or unreadable hash now costs a full dummy derivation, so no future caller can reintroduce it. |
| **Operator passwords could never move off an old KDF cost** | **Fixed** (2026-09-20) | The same line discarded `needsRehash`. Nothing in the API changes a platform password — there is no self-service change and no admin reset — so a hash written at install time kept that day's iteration count for the life of the installation. A successful login is the only migration path there is, and it now takes it, guarded on the old hash so it cannot resurrect a password that changed underneath it. `scripts/check/kdf-rehash.sh` fails the build if any `kdf.Verify` discards the flag again. ~~Still open: there is no way to rotate an operator password at all.~~ **Built** (2026-09-21). `ChangePlatformPassword` re-presents the current password — a stolen access token must not be enough to take an account over permanently — enforces the installation's 12-character floor, and advances `token_epoch` in the same statement that writes the hash. `ResetOperatorPassword` covers somebody who has *lost* theirs: gated on `PermAssignOperators`, which is not a new privilege — `CreateAPIKey` already mints, for any operator named in the request, a credential that administers as its owner, behind that same permission. What a reset adds is that the target loses their own access, so it ends their sessions and records both names. **It refuses to reset YOU**: a self-reset would be a way around the current-password check for anybody holding a stolen token that carries the permission. The temporary password is generated rather than supplied and returned once. |
| **Disabling an operator did nothing for up to an hour** | **Fixed** (2026-09-21) | `SetPlatformUserStatus` advances `token_epoch` and its comment says why — "*token_epoch + 1 is what makes disabling take effect NOW rather than whenever the token expired*". **Nothing compared it.** The interceptor copied `claims.Epoch` onto the principal and no reader ever looked; `platformGuard.require` checked assignments and never read the operator's row, so it never saw `status = 'disabled'` either. `PlatformRefresh` does check `Active()`, which bounds the window at one access-token TTL — one hour of unchanged authority on the most privileged accounts in the installation — without closing it. Both the guard and the auth interactor's own `live()` now read the row back and reject a disabled operator or a superseded epoch. One row read per platform request; the plane is operators rather than traffic, and both paths already read assignments. Found while adding password rotation, which would have been decorative without it: a password you change while the sessions opened with the old one keep working is not a rotation. |
| **The hosted second-factor form could not be submitted** | **Fixed** (2026-09-20) | Found by writing a test that posts *what the page contains* instead of hand-built values. The MFA branch of the template carries `mfa_token` and a code and no credentials — "the password is not asked for again, and nothing on this page can replay it" — but `LoginForm` required a valid password on every submit, so a real browser got "Invalid username or password" after typing a correct code. The hosted second-factor step had never worked. The existing test passed because it posted a username and password the form does not have, which is the general lesson: **a form test that builds its own values is testing a client nobody ships.** `TestTheRenderedSecondFactorFormCanBeSubmitted` parses the rendered inputs and submits those. |
| **An enrol-or-deny refusal was a dead end in the browser** | **Fixed** (2026-09-20) | Correct, and unusable: the page refused an overdue member and had nowhere for them to enrol, so the policy was unsatisfiable by everyone who only uses SSO. The page now spends the enrol grant itself — setup key, `otpauth://` link, code, recovery codes shown once — and keeps the grant server-side behind a single-use handle (`browser_enrol`, migration 0050). A wrong code discards the key by design: `ConfirmTOTP` spends the enrolment token before checking the code, so a stolen grant buys one guess. |
| **Bot protection on public registration** | Decided against | Rate limits bound the damage; they do not stop a determined script. [ADR-0014](adr/0014-bot-protection-on-registration.md) weighs that against a third-party script on a credential page, and documents the escape hatch. |

**This document now describes running code.** The application layer is built
and its security properties are tested rather than asserted: uniform login
timing (histogram, not assertion), refresh-reuse family revocation, PKCE and
exact `redirect_uri` matching, the path-normalisation corpus, the
schema-enforced invariants, and least-privilege database roles
(`migrations/0023`). See [roadmap.md](roadmap.md#formerly-unproven-claims--now-closed)
for what each test proves.
