-- Consents: lawful basis for processing an identity's personal data.
-- Append-only — a withdrawal is a new row, never an update.

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
