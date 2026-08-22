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

### Credential stuffing and brute force

- Rate limits on **three axes** — per IP, per account, per tenant. Per-account is
  the one people forget and the one that matters.
- Exponential backoff plus lockout with a documented unlock path
- Counters in Redis, so the auth database never absorbs attack traffic
- Failed-login rate spikes alert

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
negotiable `alg` field. The dormant JWS codec pins `Ed25519` and never reads
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
| **Concurrent subtree moves unproven** | Open | `scope_move_node` asserts `SERIALIZABLE`; not yet stress-tested under concurrency. |
| **Deep role graphs unproven** | Open | `role_recompute_effective` has `CYCLE` detection; not stress-tested. |
| **No deny rules** | Deliberate | Allow-only union semantics in v1. Adding deny needs strict precedence and an explain endpoint. |
| **No PASETO ecosystem support** | Deliberate | Dormant JWS codec behind a per-application flag. |
| **PII crypto-shredding not implemented** | Open | `pii_key_id` is modelled; key management does not exist. Retention enforcement is `anonymized_at` only — it stops access but does not destroy data. |
| **Self-registration abuse controls** | Open | Public realms need bot protection, email verification and registration rate limits. Not built. |
| **Application layer not built** | In progress | Every claim above about tokens, rate limiting and normalisation is design, not running code. Only the database layer is validated. |

The last row is the important one: **this document describes a validated database
layer and a specified application layer.** Do not read the specified parts as
shipped.
