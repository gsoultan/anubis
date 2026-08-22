# Operations

> Anubis is the front door for every application. Its availability target is
> therefore higher than anything it protects, and its failure modes must degrade
> rather than deny.

## Contents

1. [Deployment](#deployment)
2. [Configuration](#configuration)
3. [Key management](#key-management)
4. [Scheduled jobs](#scheduled-jobs)
5. [Monitoring](#monitoring)
6. [Runbooks](#runbooks)
7. [Backup and restore](#backup-and-restore)
8. [Capacity](#capacity)

---

## Deployment

Stateless and horizontally scalable. State lives in Postgres and Redis.

| Component | Requirement |
| :--- | :--- |
| Anubis replicas | ≥ 3, spread across availability zones, with a PodDisruptionBudget |
| PostgreSQL | Primary + synchronous standby. Read replicas optional. |
| Redis | Rate-limit counters and single-use nonces. Loss degrades, does not break. |
| KMS / Vault | Master key for signing-key encryption. **Hard dependency at startup.** |

`readyz` must check: database reachable, snapshot within max age, an `active`
signing key present. A replica that cannot verify tokens must not receive
traffic.

### Zero-downtime migrations

**Expand/contract only.** Add nullable column → backfill → dual-write → switch
reads → drop old. Never a destructive change in one step.

You cannot take auth down for a schema change. A migration that takes `ACCESS
EXCLUSIVE` on `grants` or `audit_log` stops every application in the
organisation simultaneously.

---

## Configuration

| Variable | Notes |
| :--- | :--- |
| `ANUBIS_DB_URL` | Postgres DSN |
| `ANUBIS_REDIS_URL` | |
| `ANUBIS_KMS_KEY_REF` | Master key for signing-key encryption |
| `ANUBIS_ISSUER` | Must match the `iss` claim consumers expect |
| `ANUBIS_SNAPSHOT_MAX_AGE` | Beyond this the gate fails **closed**. Default 300s. |
| `ANUBIS_SNAPSHOT_POLL` | Backstop for missed `LISTEN/NOTIFY`. Default 15s. |

> Signing keys never appear in environment variables, Git, or CI variables.
> Masked CI variables are not a security boundary.

---

## Key management

### Rotation (every 30–90 days)

```
1. Generate       → status 'pending'
2. Publish        → appears in key discovery. WAIT for consumer caches
                    (≥ 2 × max cache TTL) before step 3.
3. Activate       → status 'active'; the previous key becomes 'retiring'
4. Retire         → once the longest-lived token signed by it has expired
```

Publishing before activating is the whole point: a consumer that has not yet
seen the new `kid` will reject every token signed with it.

At most one `active` key per purpose is enforced by a partial unique index —
the database will reject an attempt to activate two.

### Compromise

**This is the highest-severity incident possible.** The attacker mints valid
tokens for any user in any scope, and the audit log shows nothing wrong.

1. Generate and activate a replacement key immediately
2. Mark the compromised key `retired` — **do not wait for token expiry**
3. Remove its public key from discovery, so existing tokens fail verification
4. **Bump `token_epoch` for every identity** — invalidates all outstanding tokens
5. Revoke all sessions and refresh-token families
6. Audit every action taken during the exposure window

```sql
UPDATE identities SET token_epoch = token_epoch + 1 WHERE tenant_id = $1;
UPDATE sessions SET revoked_at = now(), revoke_reason = 'key_compromise'
 WHERE tenant_id = $1 AND revoked_at IS NULL;
```

---

## Scheduled jobs

| Job | Frequency | Consequence if it stops |
| :--- | :--- | :--- |
| `ensure_month_partitions('audit_log','occurred_at',3)` | daily | Writes land in the `DEFAULT` partition — degraded, not broken |
| `ensure_month_partitions('refresh_tokens','expires_at',3)` | daily | Same |
| Drop expired `refresh_tokens` partitions | monthly | Unbounded growth |
| Expire `grants` past `valid_until` | hourly | Access outlives its authorisation |
| Retention sweep — anonymise past `retention_until` | daily | **Statutory breach.** UU PDP 27/2022 and equivalents. |
| `role_permissions_effective` reconciliation | nightly | Silent drift in a cache of authorization decisions |
| Audit hash-chain verification | daily | Tampering goes undetected |
| Key rotation check | daily | Keys age past policy |

> The reconciliation job matters more than it looks.
> `role_permissions_effective` is a materialised cache of authorization
> decisions. Drift is **invisible** — nothing errors, permissions are just
> quietly wrong. Recompute from scratch and alert on any diff.

---

## Monitoring

### Must page a human

| Alert | Why |
| :--- | :--- |
| **`refresh_token_reuse_detected`** | **A token was stolen.** Highest-signal alert in the system. |
| Login failure rate spike per account | Credential stuffing |
| No `active` signing key | Token issuance is down |
| Snapshot age > max | Gate is failing closed — protected paths are denying |
| Audit hash chain broken | Tampering, or a bug that destroys evidentiary value |
| `authorize` p99 > 5 ms | The PDP is on someone's hot path |
| Retention sweep failed | Statutory exposure |

### Should alert but not page

Key rotation overdue · partition provisioning behind · `role_permissions_effective`
reconciliation diff · Redis unavailable (rate limiting degraded) · grant
expiry backlog.

### Dashboards

**Golden signals** — login success/failure rate, token issuance rate, refresh
rate, `authorize` p50/p99, gate check p50/p99.

**Security** — reuse detections, lockouts, step-up challenges, cross-tenant
attempts (should be **zero**; non-zero means an application is malfunctioning or
probing), assurance-gate denials.

**Capacity** — snapshot size and load time, closure row count, largest subtree
per axis, grants per identity (p99).

---

## Runbooks

### "Users cannot log in"

1. `readyz` on every replica — which check fails?
2. Active signing key present? `SELECT * FROM signing_keys WHERE status='active'`
3. Database reachable? Connection pool exhausted?
4. Rate limiter misfiring? Check per-tenant limits before per-IP.
5. Realm policy changed recently? A `required_factors` change can lock out a
   population that has not enrolled that factor.

### "User has access they should not"

```sql
-- every live grant, with provenance
SELECT g.id, r.name AS role, p.key AS permission, rpe.via_role_id,
       gs.axis_code, sn.name AS scope_node, gs.inherit, g.self_scoped
  FROM grants g
  JOIN roles r ON r.id = g.role_id
  JOIN role_permissions_effective rpe ON rpe.role_id = g.role_id
  JOIN permissions p ON p.id = rpe.permission_id
  LEFT JOIN grant_scopes gs ON gs.grant_id = g.id
  LEFT JOIN scope_nodes sn ON sn.id = gs.scope_node_id
 WHERE g.identity_id = $1 AND g.revoked_at IS NULL;
```

`via_role_id` answers *why*. Check inherited roles via `role_parents`, and
whether a wildcard pattern expanded more broadly than intended.

### "User cannot access something they should"

Almost always a **fail-closed denial from an unresolved axis.** The calling
application omitted an axis the grant constrains.

```sql
SELECT axis_code, scope_node_id, inherit FROM grant_scopes WHERE grant_id = $1;
```

Compare against the target map the application sent. Also check
`identities.assurance_level` against `permissions.min_assurance`, and whether the
grant is `self_scoped` while the caller omitted `_owner`.

### "Suspected token theft"

Reuse detection already revoked the family and the session. Then:

1. `audit_log` for that `sid` — what was done during the window?
2. Compare `ip` and `device_fp` across the family's history
3. Global logout for the identity, bump `token_epoch`
4. Force credential rotation

### Flipping a scope axis to strict

**Never without the dry run.**

```http
POST /v1/admin/scope-axes/{code}/strict-dry-run
```

Measured on seed data: flipping `cost_center` to `deny` took allows from
**800 → 0**. Discovering that from a report beats discovering it from an outage.

### Emergency access revocation

```sql
-- one identity, everywhere
UPDATE identities SET status='disabled', disabled_at=now() WHERE id=$1;
```

Sufficient and immediate — `authorize()` gates on identity state, so no grant
needs touching. Access tokens remain valid until `exp` (≤ 15 min) unless
applications introspect.

---

## Backup and restore

| Data | Method |
| :--- | :--- |
| Postgres | Continuous archiving + PITR. Test restores quarterly. |
| Signing keys | Backed up **encrypted**, restored via KMS. Losing them invalidates every token. |
| Audit log | Shipped to append-only object storage with object lock |

### The restore footgun

**Restoring `refresh_tokens` resurrects revoked tokens.**

`identities.token_epoch` is the mitigation — but only if restored consistently
**and** if applications actually validate it.

> Test this explicitly. Restore to a scratch environment, present a
> pre-revocation refresh token, and assert it fails. Nobody discovers this until
> it matters.

---

## Capacity

Measured on 4 vCPU / 4 GB ([benchmarks.md](benchmarks.md)):

| Metric | Value |
| :--- | :--- |
| Authorization decisions | ~22,400/sec, single connection |
| Per decision | 0.045 ms |
| Database at 150k grants / 57k identities | 164 MB |

**Growth drivers, in order:** `audit_log` (partition monthly, archive),
`refresh_tokens` (partition, drop old), `grant_scopes` (grows with axes ×
grants), `scope_closure` (n × depth).

**Watch:** grants per identity at p99, and the largest subtree per axis. Neither
degrades the current query formulation — `EXISTS` short-circuits, so broad grants
cost the same as narrow ones — but both drive snapshot size and load time.
