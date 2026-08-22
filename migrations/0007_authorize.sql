-- ============================================================================
-- 0007_authorize.sql — the multi-axis authorization decision
--
-- THREE FORMULATIONS WERE BENCHMARKED against 150k grants / 270k grant_scopes
-- / 128k closure rows, on a workload of 3,000 decisions where grants sit at
-- BROAD scope nodes (segment: ~4k descendants, axis root: ~20k) -- i.e. the
-- ordinary admin/auditor/regional-manager case, not an edge case:
--
--   A. downward  -- expand the granted node's DESCENDANTS ....  284 ms
--   B. upward    -- expand the target's ANCESTORS via CTE join  4976 ms  (17x WORSE)
--   C. upward    -- correlated EXISTS driven by grant_scopes .. 142 ms   <-- THIS
--
--   All three returned identical results on every row (0 disagreements).
--
-- Why B loses despite touching fewer rows in isolation:
--   jsonb_each_text() is a function scan with NO statistics. Postgres assumes
--   100 rows; the real answer is 2. That 50x overestimate propagates into the
--   join above it, and the planner abandons the nested loop for a hash/
--   materialise strategy. Measured in isolation the ancestor walk really is
--   94x cheaper in buffers (8 vs 756) -- but letting an unestimatable function
--   scan DRIVE a join costs far more than the traversal ever saves.
--
-- Why C wins:
--   The closure probe is correlated to gs.scope_node_id -- a real column on a
--   real table with real statistics. Both scope_closure columns are then
--   equality-bound, giving a two-column probe on the (ancestor_id,
--   descendant_id) primary key. EXISTS short-circuits on the first hit, so
--   broad grants cost the same as narrow ones. The function scan is pinned
--   behind MATERIALIZED so its bad estimate can never drive plan choice.
--
-- LESSON WORTH KEEPING: EXPLAIN on an isolated query misled us here. Only the
-- end-to-end workload exposed the regression. Benchmark the decision, not the
-- subquery.
-- ============================================================================

CREATE OR REPLACE FUNCTION authorize(
    p_identity uuid, p_tenant uuid, p_permission text, p_targets jsonb
) RETURNS boolean LANGUAGE sql STABLE PARALLEL SAFE AS $fn$
WITH targets AS MATERIALIZED (
    SELECT key AS axis_code, value::uuid AS node_id FROM jsonb_each_text(p_targets)),
candidates AS (
    SELECT g.id FROM grants g
      JOIN role_permissions_effective rpe ON rpe.role_id=g.role_id
      JOIN permissions p ON p.id=rpe.permission_id
     WHERE g.identity_id=p_identity AND g.tenant_id=p_tenant AND g.revoked_at IS NULL
       AND g.valid_from<=now() AND (g.valid_until IS NULL OR g.valid_until>now())
       AND p.tenant_id=p_tenant AND p.key=p_permission AND p.deprecated_at IS NULL),
axis_eval AS (
    SELECT gs.grant_id, gs.axis_code,
           EXISTS (SELECT 1
                     FROM targets t
                     JOIN scope_closure c
                       ON c.descendant_id = t.node_id
                      AND c.ancestor_id   = gs.scope_node_id
                    WHERE t.axis_code = gs.axis_code
                      AND (gs.inherit OR c.depth = 0)) AS satisfied
      FROM grant_scopes gs JOIN candidates cd ON cd.id = gs.grant_id)
SELECT EXISTS (SELECT 1 FROM candidates cd
   WHERE NOT EXISTS (SELECT 1 FROM axis_eval ae
                      WHERE ae.grant_id=cd.id AND NOT ae.satisfied)
     AND NOT EXISTS (SELECT 1 FROM scope_axes a
                      WHERE a.default_effect='deny' AND a.status='active'
                        AND NOT EXISTS (SELECT 1 FROM grant_scopes gs2
                                         WHERE gs2.grant_id=cd.id AND gs2.axis_code=a.code)));
$fn$;