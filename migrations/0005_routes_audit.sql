-- ============================================================================
-- 0005_routes_audit.sql — route policies, audit, partition management
-- ============================================================================

-- ---------------------------------------------------------------------------
-- route_policies — secure a website path without touching the app.
-- Consumed by /v1/gate/check (nginx auth_request, Traefik forwardAuth,
-- Envoy ext_authz). Explicit integer priority beats implicit "most specific
-- wins": authorization surprises are outages.
-- ---------------------------------------------------------------------------
CREATE TABLE route_policies (
    created_at     timestamptz NOT NULL DEFAULT now(),
    id             uuid PRIMARY KEY DEFAULT uuidv7(),
    application_id uuid NOT NULL,
    tenant_id      uuid NOT NULL,
    permission_id  uuid REFERENCES permissions(id) ON DELETE RESTRICT,
    priority       int  NOT NULL,
    effect         text NOT NULL
                   CHECK (effect IN ('public','require_auth','require_permission','deny')),
    path_pattern   text NOT NULL,
    host_pattern   text,
    methods        text[] NOT NULL DEFAULT '{*}',
    -- how each axis resolves for this route, e.g.
    --   {"product": "path.product_id", "org": "token"}
    scope_bindings jsonb NOT NULL DEFAULT '{}',

    FOREIGN KEY (application_id, tenant_id) REFERENCES applications(id, tenant_id) ON DELETE CASCADE,
    UNIQUE (application_id, priority),
    CHECK (effect <> 'require_permission' OR permission_id IS NOT NULL)
);
CREATE INDEX route_policies_app ON route_policies (application_id, priority);

-- ---------------------------------------------------------------------------
-- audit_log — PARTITIONED BY MONTH from day one.
--
-- Retrofitting partitioning onto a large hot table requires a full rewrite
-- under an ACCESS EXCLUSIVE lock. On an auth service that means downtime for
-- every application at once. Partition now; it is free today.
--
-- prev_hash/entry_hash form a hash chain: an attacker with UPDATE rights on
-- this table still cannot silently rewrite history.
-- ---------------------------------------------------------------------------
CREATE TABLE audit_log (
    occurred_at timestamptz NOT NULL DEFAULT now(),
    id          uuid NOT NULL DEFAULT uuidv7(),
    tenant_id   uuid NOT NULL,
    actor_id    uuid,
    target_id   uuid,
    session_id  uuid,
    seq         bigint NOT NULL,
    action      text NOT NULL,
    result      text NOT NULL CHECK (result IN ('allow','deny','error')),
    actor_kind  text NOT NULL DEFAULT 'identity',
    ip          inet,
    -- the DECISION INPUTS, snapshotted. Recording them means past decisions
    -- never need reconstructing from mutable catalog state.
    detail      jsonb NOT NULL DEFAULT '{}',
    prev_hash   bytea,
    entry_hash  bytea NOT NULL,

    PRIMARY KEY (id, occurred_at)
) PARTITION BY RANGE (occurred_at);

-- BRIN, not B-tree: audit_log is append-only and physically time-ordered, so
-- a BRIN index is ~1000x smaller than the equivalent B-tree while answering
-- range scans just as well.
CREATE INDEX audit_log_time_brin ON audit_log USING BRIN (occurred_at)
    WITH (pages_per_range = 32);
CREATE INDEX audit_log_actor ON audit_log (actor_id, occurred_at DESC)
    WHERE actor_id IS NOT NULL;
CREATE INDEX audit_log_tenant_action ON audit_log (tenant_id, action, occurred_at DESC);

-- ---------------------------------------------------------------------------
-- Partition management. Called by a scheduled job; keeps N months ahead.
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION ensure_month_partitions(
    p_table text, p_col text, p_months_ahead int DEFAULT 3
) RETURNS void LANGUAGE plpgsql AS $fn$
DECLARE
    i int; v_start date; v_end date; v_name text;
BEGIN
    FOR i IN 0..p_months_ahead LOOP
        v_start := date_trunc('month', now())::date + (i || ' month')::interval;
        v_end   := v_start + interval '1 month';
        v_name  := format('%s_%s', p_table, to_char(v_start, 'YYYYMM'));
        IF to_regclass(v_name) IS NULL THEN
            EXECUTE format(
              'CREATE TABLE %I PARTITION OF %I FOR VALUES FROM (%L) TO (%L)',
              v_name, p_table, v_start, v_end);
        END IF;
    END LOOP;
END;
$fn$;

SELECT ensure_month_partitions('audit_log', 'occurred_at', 3);
SELECT ensure_month_partitions('refresh_tokens', 'expires_at', 3);

-- Default partitions catch anything outside the provisioned range so an
-- insert can never fail because a scheduled job did not run.
CREATE TABLE audit_log_default PARTITION OF audit_log DEFAULT;
CREATE TABLE refresh_tokens_default PARTITION OF refresh_tokens DEFAULT;

-- ---------------------------------------------------------------------------
-- Catalog invalidation triggers — every table feeding the hot-path snapshot
-- ---------------------------------------------------------------------------
CREATE TRIGGER bump_scope_nodes AFTER INSERT OR UPDATE OR DELETE ON scope_nodes
    FOR EACH ROW EXECUTE FUNCTION trg_bump_catalog('tenant_id');
CREATE TRIGGER bump_grants AFTER INSERT OR UPDATE OR DELETE ON grants
    FOR EACH ROW EXECUTE FUNCTION trg_bump_catalog('tenant_id');
CREATE TRIGGER bump_grant_scopes AFTER INSERT OR UPDATE OR DELETE ON grant_scopes
    FOR EACH ROW EXECUTE FUNCTION trg_bump_catalog('tenant_id');
CREATE TRIGGER bump_roles AFTER INSERT OR UPDATE OR DELETE ON roles
    FOR EACH ROW EXECUTE FUNCTION trg_bump_catalog('tenant_id');
CREATE TRIGGER bump_permissions AFTER INSERT OR UPDATE OR DELETE ON permissions
    FOR EACH ROW EXECUTE FUNCTION trg_bump_catalog('tenant_id');
CREATE TRIGGER bump_route_policies AFTER INSERT OR UPDATE OR DELETE ON route_policies
    FOR EACH ROW EXECUTE FUNCTION trg_bump_catalog('tenant_id');
