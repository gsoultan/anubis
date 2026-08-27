# ADR-0013 — What field-level PII encryption actually covers

## Status

Accepted and implemented. `identities.attributes` is sealed on write
(`SetIdentityAttributes`), opened on read, and unrecoverable once the
identity's key is shredded. Migration 0035 replaced the tripwire from 0034
with a constraint that admits a sealed envelope and still refuses plaintext.

## Context

Migration 0022 and `internal/identity/domain/pii` provide per-tenant keys,
sealing bound to a field name, and crypto-shredding (destroy the key, the
ciphertext is noise forever). The retention job already shreds on schedule.

**No column is encrypted.** The machinery works and guards nothing, which is
the worst of both worlds: the roadmap has claimed field-level PII encryption
for months, and a reader reasonably assumes something is protected by it.

The reason it was never wired is not laziness. Encrypting a column takes it
out of reach of SQL: no `WHERE email = $1`, no `ORDER BY surname`, no index
that is not a blind equality probe on a deterministic ciphertext (which
leaks equality, which is how you re-identify a small population). Encrypting
the wrong column breaks sign-in.

Concretely, `identities` holds:

- `username` — the sign-in key. Uniquely indexed, looked up on every login.
- `email` — a sign-in key in some realms, a notification address in all.
- `external_ref` — the HR or CRM id; joined against on every sync.
- `attributes` (jsonb) — free-form, tenant-defined. **Nothing indexes it,
  nothing joins on it, and no flow reads it to make a decision.**

## Decision

**Encrypt `identities.attributes` and nothing else, for now.**

It is the only column whose contents are genuinely arbitrary — a tenant can
put a home address, a date of birth, a case note in there — and the only one
no query depends on. Sealing it costs nothing operationally and removes the
most dangerous unknown from a database dump.

The identifying columns stay in plaintext, deliberately:

- `username` and `email` are **lookup keys**. Encrypting them means either
  deterministic ciphertext (which leaks exactly the equality an attacker
  wants, while defeating every prefix search the console does) or breaking
  sign-in outright.
- `external_ref` is a **join key** for scope sync.
- These are, in any case, the fields a tenant's own directory already holds
  and its own emails already carry. Encrypting them at rest while they
  travel in every notification is theatre.

For those columns the protection is elsewhere and already in force:
`anubis_readonly` cannot read credentials or keys (0023), the audit log is
append-only to the runtime, and erasure anonymises rather than encrypts.

## Consequences

- The API must **write** `attributes` before this changes anything: today no
  endpoint populates it, so the encrypted column would seal an empty value.
  That is the next piece of work, not a reason to defer the decision.
- Sealing is bound to the field name as additional data, so a sealed value
  lifted into another column or another row does not open. That property is
  what makes per-field sealing worth more than encrypting the whole row.
- Shredding a tenant's key destroys `attributes` for every identity in it.
  That is the intent — it is how erasure becomes provable rather than a
  promise about a backup — but it means **a lost key is lost data**, and the
  key lives sealed under the master key, which lives in a KMS.
- The roadmap and [security.md](../security.md) must state the scope in this
  form: attributes encrypted, identifiers not, and why. A claim of
  "field-level PII encryption" with no qualifier is the thing this ADR is
  written to stop.

## Revisit when

- A tenant needs an identifying field they consider PII (a national id, say)
  that Anubis does not index or join on. It belongs in `attributes`, and the
  answer is to say so rather than to encrypt a new column.
- Searchable encryption stops being a research topic with a production
  answer that does not leak equality.
