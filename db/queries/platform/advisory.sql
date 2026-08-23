-- Advisory locks used by the maintenance scheduler. Session-scoped, so they
-- must run on a dedicated connection: the caller acquires one, takes the
-- lock, does the work, unlocks and releases.

-- name: TryAdvisoryLock :one
SELECT pg_try_advisory_lock(sqlc.arg(lock_id)) AS acquired;

-- name: AdvisoryUnlock :one
SELECT pg_advisory_unlock(sqlc.arg(lock_id)) AS released;
