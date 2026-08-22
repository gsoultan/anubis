-- name: LastAuditEntry :one
SELECT seq, entry_hash FROM audit_log
WHERE tenant_id = sqlc.arg(tenant_id)
ORDER BY seq DESC
LIMIT 1;

-- name: InsertAudit :one
INSERT INTO audit_log (tenant_id, actor_id, actor_kind, target_id, session_id,
                       seq, action, result, ip, detail, prev_hash, entry_hash)
VALUES (sqlc.arg(tenant_id), sqlc.narg(actor_id), sqlc.arg(actor_kind),
        sqlc.narg(target_id), sqlc.narg(session_id), sqlc.arg(seq),
        sqlc.arg(action), sqlc.arg(result), nullif(sqlc.arg(ip), '')::inet,
        sqlc.arg(detail)::jsonb, sqlc.narg(prev_hash), sqlc.arg(entry_hash))
RETURNING id, occurred_at;

-- name: AdvisoryLockAuditChain :exec
-- Serialises chain appends per tenant for the duration of the transaction.
-- hashtextextended gives a stable 64-bit key from the tenant id.
SELECT pg_advisory_xact_lock(hashtextextended('audit:' || sqlc.arg(tenant_id)::text, 0));

-- name: QueryAudit :many
SELECT id, occurred_at, seq, actor_id, actor_kind, target_id, session_id,
       action, result, COALESCE(host(ip)::text, '')::text AS ip, detail, entry_hash
FROM audit_log
WHERE tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.narg(actor_id)::uuid IS NULL OR actor_id = sqlc.narg(actor_id))
  AND (sqlc.narg(action)::text IS NULL OR action = sqlc.narg(action))
  AND (sqlc.narg(from_ts)::timestamptz IS NULL OR occurred_at >= sqlc.narg(from_ts))
  AND (sqlc.narg(to_ts)::timestamptz IS NULL OR occurred_at < sqlc.narg(to_ts))
  AND (sqlc.narg(before_seq)::bigint IS NULL OR seq < sqlc.narg(before_seq))
ORDER BY seq DESC
LIMIT sqlc.arg(page_size);

-- name: AuditChainRange :many
-- Chain verification walks seq order in batches.
SELECT seq, occurred_at, action, result, actor_kind, detail,
       actor_id, target_id, session_id, COALESCE(host(ip)::text, '')::text AS ip,
       prev_hash, entry_hash
FROM audit_log
WHERE tenant_id = sqlc.arg(tenant_id)
  AND seq > sqlc.arg(after_seq)
  AND (sqlc.narg(from_ts)::timestamptz IS NULL OR occurred_at >= sqlc.narg(from_ts))
  AND (sqlc.narg(to_ts)::timestamptz IS NULL OR occurred_at < sqlc.narg(to_ts))
ORDER BY seq
LIMIT sqlc.arg(batch_size);

-- name: EnsureAuditPartitions :exec
SELECT ensure_month_partitions('audit_log', 'occurred_at', 3);

-- name: EnsureRefreshPartitions :exec
SELECT ensure_month_partitions('refresh_tokens', 'expires_at', 3);

-- name: ListConsents :many
SELECT id, identity_id, purpose, policy_version, granted_at, withdrawn_at,
       expires_at, evidence
FROM consents
WHERE identity_id = sqlc.arg(identity_id) AND tenant_id = sqlc.arg(tenant_id)
ORDER BY granted_at DESC;

-- name: InsertConsent :one
INSERT INTO consents (tenant_id, identity_id, purpose, policy_version, evidence)
VALUES (sqlc.arg(tenant_id), sqlc.arg(identity_id), sqlc.arg(purpose),
        sqlc.arg(policy_version), sqlc.arg(evidence)::jsonb)
RETURNING id, granted_at;

-- name: WithdrawConsent :execrows
UPDATE consents SET withdrawn_at = now()
WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id)
  AND withdrawn_at IS NULL;

-- name: GetSigninPage :one
SELECT tenant_id, config, updated_at FROM signin_pages
WHERE tenant_id = sqlc.arg(tenant_id);

-- name: PutSigninPage :exec
INSERT INTO signin_pages (tenant_id, config, updated_at)
VALUES (sqlc.arg(tenant_id), sqlc.arg(config)::jsonb, now())
ON CONFLICT (tenant_id) DO UPDATE
    SET config = EXCLUDED.config, updated_at = now();
