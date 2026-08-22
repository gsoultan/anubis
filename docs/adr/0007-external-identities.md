# ADR-0007 — External identity populations

**Status:** accepted · **Date:** 2026-08-22

## Context

Anubis serves more than employees. It must also authenticate **suppliers,
vendors and contractors** (business partners) and **job applicants and
customers** (public users).

These populations differ in ways that are **not scope questions**:

| | Employee | Partner | Public |
| :--- | :--- | :--- | :--- |
| Created by | HR sync | Contract onboarding | **Self-registration** |
| Auth factors | password + TOTP + SSO | password + TOTP | password / email OTP |
| Identity assurance | verified, ID on file (IAL3) | verified by contract (IAL2) | self-asserted email (IAL1) |
| Administered by | IT | **their own admin (delegated)** | themselves |
| Session TTL | 12 h | 8 h | 1 h |
| Retention | employment records | contract + legal period | **legally bounded — must delete** |
| Access shape | org / office / department scope | their own company's data | **only their own record** |

None of that is expressible as a scope axis. Realm is therefore a first-class
concept, not another axis.

## Rejected: one tenant per partner

The obvious model gives Supplier Acme its own tenant. **Rejected**, because an
Acme user needs access to *our* purchase-order data, which requires
**cross-tenant grants**.

Every grant and scope foreign key in this schema is deliberately composite on
`tenant_id` precisely to make cross-tenant references **impossible to insert** —
the single strongest control in the design
([ADR-0003](0003-scope-model.md#making-illegal-states-unrepresentable)).
Relaxing it to accommodate suppliers trades a real security boundary for a
modelling convenience.

## Decision: realms partition the population inside a tenant

- **Tenant** remains the isolation boundary — PT Impack Pratama.
- **Realm** partitions the population within it — `employee`, `partner`,
  `public`.
- **Partner organisations become scope nodes on a `partner` axis.**

Authorization keeps working exactly as designed. No new mechanism, no relaxed
invariant.

```sql
CREATE TABLE realms (
    kind              text,      -- employee | partner | public | service
    min_assurance     smallint,  -- NIST 800-63 IAL, 1..3
    self_registration boolean,
    allowed_factors   text[],
    required_factors  text[],
    session_ttl       interval,
    default_retention interval,  -- NULL = no statutory limit
    pii_encryption    boolean,
    ...
);
```

### Username uniqueness moves inside the realm

```sql
UNIQUE (tenant_id, realm_id, lower(username))
```

`alice` the employee and `alice` the job applicant are different people and must
not collide. **Verified:** the same username exists in all three realms
simultaneously.

### Identity linking is explicit

A supplier contact who is later hired, or an applicant who becomes an employee,
gets a **new identity in the new realm**. `identity_links` records that they are
the same human, with the method and evidence.

> Linking never merges grants. Inheriting a contractor's access on becoming an
> employee — or worse, retaining employee access after moving to a supplier — is
> exactly the accident this prevents.

## Three new authorization gates

All fail-closed, all in `authorize()`.

### 1. Identity state

```sql
AND i.status = 'active' AND i.disabled_at IS NULL AND i.anonymized_at IS NULL
```

Deprovisioning must not depend on someone remembering to revoke every grant.
Disabling the identity is sufficient and immediate. **Verified:** disabled and
anonymised identities are denied with grants left fully intact.

### 2. Assurance

```sql
AND p.min_assurance <= i.assurance_level
```

A self-registered applicant must not approve a purchase order **even if a
misconfigured grant says otherwise.** This is defence in depth against
grant-administration error — precisely the class of mistake that delegated
administration makes more likely.

**Verified:** an IAL1 applicant holding a grant conferring an IAL3 permission is
denied.

### 3. Self-scope

"You may read applications, but only your own" is the dominant external-user
shape, and it is not a scope question — **no tree node describes "the row you
own."**

Rather than open the door to general ABAC (deferred in
[ADR-0004](0004-authorization-semantics.md#deferred-deny-rules)), one narrow
concept covers most real cases:

```sql
grants.self_scoped boolean
```

The caller passes the record's owner as the reserved target key `_owner`.
Reserved keys begin with `_`, which `scope_axes.code`'s CHECK constraint forbids,
so an axis can never collide with one.

If a grant is `self_scoped` and no `_owner` is supplied, the answer is **DENY** —
the same fail-closed rule that governs unresolved axes.

**Verified:** own record allowed, another's denied, and missing `_owner` denied.

## Delegated administration without escalation

Partners administer their own users. Anubis registers **itself** as an
application, so `anubis:identity:create` and `anubis:grant:create` are ordinary
permissions scoped to a `partner_org` node. Anubis is its own relying party.

That introduces two escalation paths, both closed in the **database**, not in
admin-UI validation:

```sql
roles.allowed_realm_kinds text[]   -- which populations may hold this role
role_grantable                     -- which roles a role may confer
```

Constraint triggers enforce them, so a script bypassing the application layer
still cannot:

1. Attach an employee-only role to a public self-registered account
2. Attach axis constraints to a self-scoped grant (the two are orthogonal, and
   allowing both produces a grant nobody can reason about)

**Verified: 2/2 rejected.** The guard also caught a genuine bug in our own seed
script, which was granting employee roles to partner identities.

## Privacy and retention

Applicant data cannot be kept indefinitely — Indonesia's UU PDP No. 27/2022 and
equivalents impose statutory limits. This conflicts with "never delete, for
audit."

**Resolution: crypto-shredding.** PII is stored under a per-identity key
(`identities.pii_key_id`). Deleting the key makes the data unrecoverable while
rows and referential integrity survive, so the audit trail and the grant graph
stay consistent.

| Column | Purpose |
| :--- | :--- |
| `retention_until` | Statutory deadline, defaulted from `realms.default_retention` |
| `deletion_requested_at` | Subject request (right to erasure) |
| `anonymized_at` | Shredding completed — **authorization denies from this moment** |

`consents` is append-only: a withdrawal is a new row, never an update, so the
record of what was consented to and when survives the withdrawal.

## Consequences

**Positive** — one identity service for all populations; the tenant isolation
invariant is untouched; deprovisioning is a single field; assurance gates protect
against grant misadministration; retention is enforceable.

**Negative** — three new tables and six identity columns; realm-aware
authentication policy is application work not yet built; identity linking is a
new operational process.

**Open** — PII crypto-shredding is modelled in the schema (`pii_key_id`) but the
key-management implementation does not exist yet. Until it does, retention
enforcement is `anonymized_at` only, which stops access but does not destroy
data.
