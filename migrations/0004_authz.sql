-- ============================================================================
-- 0004_authz.sql — permissions, role graph, grants
-- ============================================================================

-- ---------------------------------------------------------------------------
-- permissions — owned by APPLICATIONS, not by Anubis.
-- Apps register their own catalog via a manifest, so shipping a new feature
-- never requires an Anubis deploy. Namespacing prevents collisions between
-- e.g. billing:invoice:approve and procurement:invoice:approve.
-- ---------------------------------------------------------------------------
CREATE TABLE permissions (
    created_at     timestamptz NOT NULL DEFAULT now(),
    deprecated_at  timestamptz,
    -- Step-up authentication, declared BY THE PERMISSION.
    -- One central definition of "sensitive"; apps get step-up for free.
    max_auth_age   interval,
    id             uuid PRIMARY KEY DEFAULT uuidv7(),
    application_id uuid NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    tenant_id      uuid NOT NULL,
    risk           text NOT NULL DEFAULT 'normal'
                   CHECK (risk IN ('normal','sensitive','critical')),
    resource       text NOT NULL,
    action         text NOT NULL,
    -- app_slug is a COPY of applications.slug, held correct by the composite
    -- FK below with ON UPDATE CASCADE. It cannot drift: renaming an app
    -- rewrites these rows automatically, and no other value is insertable.
    app_slug       text NOT NULL,
    -- 'billing:invoice:approve' — a GENERATED column, so it is maintained by
    -- the storage engine itself. The previous BEFORE-trigger version silently
    -- produced NULLs under session_replication_role='replica' (bulk loads,
    -- logical replication apply) — a denormalisation that a trigger keeps
    -- correct is only as reliable as the trigger being enabled.
    key            text GENERATED ALWAYS AS
                   (app_slug || ':' || resource || ':' || action) STORED,
    description    text NOT NULL DEFAULT '',
    requires_amr   text[] NOT NULL DEFAULT '{}',

    FOREIGN KEY (application_id, tenant_id) REFERENCES applications(id, tenant_id) ON DELETE CASCADE,
    FOREIGN KEY (application_id, app_slug)  REFERENCES applications(id, slug) ON UPDATE CASCADE,
    UNIQUE (application_id, resource, action)
);
CREATE UNIQUE INDEX permissions_key ON permissions (tenant_id, key);
CREATE INDEX permissions_app ON permissions (application_id) WHERE deprecated_at IS NULL;


-- ---------------------------------------------------------------------------
-- roles
-- ---------------------------------------------------------------------------
CREATE TABLE roles (
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    id             uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id      uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    application_id uuid,                       -- NULL = cross-application role
    is_system      boolean NOT NULL DEFAULT false,  -- from manifest; immutable
    name           text NOT NULL,
    description    text NOT NULL DEFAULT '',
    -- which scope node types this role may be granted at
    assignable_at  text[] NOT NULL DEFAULT '{}',

    FOREIGN KEY (application_id, tenant_id) REFERENCES applications(id, tenant_id) ON DELETE CASCADE,
    UNIQUE (tenant_id, name),
    UNIQUE (id, tenant_id)
);

-- Role composition: 'finance.approver' inherits 'finance.viewer'
CREATE TABLE role_parents (
    role_id   uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    parent_id uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, parent_id),
    CHECK (role_id <> parent_id)
);
CREATE INDEX role_parents_parent ON role_parents (parent_id);

-- Concrete grants. Separate from patterns: different lifecycle -- patterns
-- get re-expanded whenever the permission catalog changes, concrete rows
-- never do. (The earlier combined table also had an illegal expression PK.)
CREATE TABLE role_permissions (
    role_id       uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id uuid NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

-- Wildcard grants. Expanded AT WRITE TIME into role_permissions_effective.
-- Never matched at read time: runtime globbing is unauditable and slow, and
-- silently widens a role when a new permission is registered.
CREATE TABLE role_permission_patterns (
    id      uuid PRIMARY KEY DEFAULT uuidv7(),
    role_id uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    pattern text NOT NULL CHECK (pattern ~ '^[a-z0-9_:*-]+$'),
    UNIQUE (role_id, pattern)
);

-- ---------------------------------------------------------------------------
-- role_permissions_effective — fully flattened closure of
--   (role graph) x (concrete grants + expanded patterns)
-- Read path is a pure index join. via_role_id carries provenance so
-- "why does Alice have this?" is a query, not an investigation.
-- ---------------------------------------------------------------------------
CREATE TABLE role_permissions_effective (
    role_id       uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id uuid NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    via_role_id   uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
) WITH (fillfactor = 95);
-- reverse: "which roles confer this permission?"
CREATE INDEX rpe_permission ON role_permissions_effective (permission_id, role_id);

-- Recompute one role's effective set. PG's CYCLE clause handles role loops.
CREATE OR REPLACE FUNCTION role_recompute_effective(p_role uuid)
RETURNS void LANGUAGE plpgsql AS $fn$
BEGIN
    DELETE FROM role_permissions_effective WHERE role_id = p_role;

    INSERT INTO role_permissions_effective (role_id, permission_id, via_role_id)
    WITH RECURSIVE ancestry AS (
        SELECT p_role AS rid
        UNION
        SELECT rp.parent_id FROM role_parents rp JOIN ancestry a ON a.rid = rp.role_id
    ) CYCLE rid SET is_cycle USING cyc_path
    SELECT DISTINCT ON (perm.id) p_role, perm.id, a.rid
      FROM ancestry a
      JOIN LATERAL (
            SELECT pm.id FROM role_permissions x
              JOIN permissions pm ON pm.id = x.permission_id
             WHERE x.role_id = a.rid AND pm.deprecated_at IS NULL
            UNION
            SELECT pm.id FROM role_permission_patterns pp
              JOIN permissions pm
                ON pm.key LIKE replace(pp.pattern, '*', '%')
             WHERE pp.role_id = a.rid AND pm.deprecated_at IS NULL
      ) perm ON true
     WHERE NOT a.is_cycle;
END;
$fn$;

-- ---------------------------------------------------------------------------
-- grants — (identity, role) + N independent axis constraints
-- ---------------------------------------------------------------------------
CREATE TABLE grants (
    created_at  timestamptz NOT NULL DEFAULT now(),
    valid_from  timestamptz NOT NULL DEFAULT now(),
    valid_until timestamptz,          -- time-boxed access (contractors, JIT)
    revoked_at  timestamptz,
    id          uuid NOT NULL DEFAULT uuidv7(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    identity_id uuid NOT NULL,
    role_id     uuid NOT NULL,
    granted_by  uuid NOT NULL,
    reason      text,

    PRIMARY KEY (id),
    UNIQUE (id, tenant_id),
    FOREIGN KEY (identity_id, tenant_id) REFERENCES identities(id, tenant_id) ON DELETE CASCADE,
    FOREIGN KEY (role_id, tenant_id)     REFERENCES roles(id, tenant_id) ON DELETE CASCADE,
    CHECK (valid_until IS NULL OR valid_until > valid_from)
) WITH (fillfactor = 90);

-- THE hot-path entry index. Partial (live rows only) and covering, so the
-- candidate-grant lookup is an index-only scan.
CREATE INDEX grants_identity_live ON grants (identity_id)
    INCLUDE (role_id, id, valid_from, valid_until)
    WHERE revoked_at IS NULL;
CREATE INDEX grants_role ON grants (role_id) WHERE revoked_at IS NULL;
-- expiry sweeper
CREATE INDEX grants_expiring ON grants (valid_until)
    WHERE revoked_at IS NULL AND valid_until IS NOT NULL;

-- ---------------------------------------------------------------------------
-- grant_scopes — one row per (grant, axis, node).
--
-- inherit lives HERE, not on grants: inheritance is per-axis.
-- "everything under Jakarta, but ONLY product line A itself, not its SKUs"
-- is a legitimate requirement a grant-level flag cannot express.
--
-- The composite FK to (id, tenant_id, axis_code) makes it impossible to
-- reference a node from another tenant, or from a different axis than the
-- row claims.
-- ---------------------------------------------------------------------------
CREATE TABLE grant_scopes (
    grant_id      uuid NOT NULL,
    tenant_id     uuid NOT NULL,
    scope_node_id uuid NOT NULL,
    inherit       boolean NOT NULL DEFAULT true,
    axis_code     text NOT NULL REFERENCES scope_axes(code) ON DELETE RESTRICT,

    PRIMARY KEY (grant_id, axis_code, scope_node_id),
    FOREIGN KEY (grant_id, tenant_id)
        REFERENCES grants (id, tenant_id) ON DELETE CASCADE,
    FOREIGN KEY (scope_node_id, tenant_id, axis_code)
        REFERENCES scope_nodes (id, tenant_id, axis_code) ON DELETE RESTRICT
) WITH (fillfactor = 95);

CREATE INDEX grant_scopes_node ON grant_scopes (scope_node_id, axis_code);
