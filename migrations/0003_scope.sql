-- ============================================================================
-- 0003_scope.sql — DYNAMIC ACCESS SCOPES
--
-- Design goal: adding a new scope dimension ("by product", "by customer",
-- "by cost centre") must be an INSERT, never a migration and never a deploy.
--
-- Structure: a FOREST of axes. Each axis is an independent tree.
--   org axis:      impack > jakarta > finance > ap
--   product axis:  catalog > line-a > sku-1
--   customer axis: accounts > enterprise > acme
--
-- A grant constrains each axis independently:
--   within an axis  = OR   (line-a OR line-b)
--   across axes     = AND  (org AND product AND customer)
--   axis not present = governed by scope_axes.default_effect
--
-- Trees use a CLOSURE TABLE, not ltree/materialized path, because:
--   * no CREATE EXTENSION (blocked on many managed Postgres deployments)
--   * ancestor check is a PRIMARY KEY lookup, not a GiST range scan
--   * no label charset limits (ltree forbids '-', so 'line-a' is illegal)
--   * cycle detection is one indexed probe
--   * renaming a node never rewrites descendants
-- ============================================================================

-- ---------------------------------------------------------------------------
-- scope_axes — THE table that makes scopes runtime-extensible.
-- Adding "by product" in 2027 = one INSERT here. No DDL. No deploy.
-- ---------------------------------------------------------------------------
CREATE TABLE scope_axes (
    created_at     timestamptz NOT NULL DEFAULT now(),
    -- Evaluation order for deterministic explain output (lower = first)
    sort_order     int NOT NULL DEFAULT 100,
    code           text PRIMARY KEY CHECK (code ~ '^[a-z][a-z0-9_]{1,30}$'),
    display_name   text NOT NULL,
    -- 'unconstrained': grants silent on this axis still pass (safe to add)
    -- 'deny'         : grants must speak to this axis explicitly (strict)
    default_effect text NOT NULL DEFAULT 'unconstrained'
                   CHECK (default_effect IN ('unconstrained','deny')),
    status         text NOT NULL DEFAULT 'active'
                   CHECK (status IN ('active','deprecated')),
    -- where the TARGET value comes from at check time:
    --   {"from":"token"}                        session-pinned (e.g. active org)
    --   {"from":"context","key":"product_id"}   supplied by the calling app
    resolution     jsonb NOT NULL DEFAULT '{"from":"context"}',
    -- drives the generic admin UI: picker type, icon, searchability.
    -- The UI must render from this, never from a switch on axis code.
    ui_schema      jsonb NOT NULL DEFAULT '{}'
);

CREATE TABLE scope_node_types (
    code         text PRIMARY KEY CHECK (code ~ '^[a-z][a-z0-9_]{1,30}$'),
    axis_code    text NOT NULL REFERENCES scope_axes(code) ON DELETE RESTRICT,
    display_name text NOT NULL,
    parent_types text[] NOT NULL DEFAULT '{}',
    UNIQUE (code, axis_code)          -- composite-FK target
);

-- ---------------------------------------------------------------------------
-- scope_nodes
--
-- The composite foreign keys below are the security core of this schema.
-- They make three classes of bug PHYSICALLY UNREPRESENTABLE rather than
-- merely "covered by a test":
--   * a node parented across tenants   (cross-tenant privilege leak)
--   * a node parented across axes      (a product nested under an office)
--   * a node whose type belongs to a different axis
-- ---------------------------------------------------------------------------
CREATE TABLE scope_nodes (
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    id           uuid NOT NULL DEFAULT uuidv7(),
    tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    parent_id    uuid,
    is_axis_root boolean NOT NULL DEFAULT false,
    status       text NOT NULL DEFAULT 'active'
                 CHECK (status IN ('active','archived')),
    axis_code    text NOT NULL,
    node_type    text NOT NULL,
    slug         text NOT NULL CHECK (slug <> ''),
    name         text NOT NULL,
    external_ref text,                       -- idempotency key for ERP/CRM sync
    -- DISPLAY METADATA ONLY. Never a decision input: jsonb predicates are
    -- unindexable on the hot path and unauditable. Anything that decides
    -- access becomes its own axis.
    attributes   jsonb NOT NULL DEFAULT '{}',

    PRIMARY KEY (id),
    UNIQUE (id, tenant_id, axis_code),       -- composite-FK target

    FOREIGN KEY (node_type, axis_code)
        REFERENCES scope_node_types (code, axis_code) ON DELETE RESTRICT,
    -- parent MUST share tenant AND axis
    FOREIGN KEY (parent_id, tenant_id, axis_code)
        REFERENCES scope_nodes (id, tenant_id, axis_code) ON DELETE RESTRICT,

    CONSTRAINT root_has_no_parent CHECK (NOT is_axis_root OR parent_id IS NULL),
    CONSTRAINT nonroot_has_parent CHECK (is_axis_root OR parent_id IS NOT NULL)
) WITH (fillfactor = 90);

CREATE UNIQUE INDEX scope_nodes_sibling_slug ON scope_nodes (parent_id, slug)
    WHERE parent_id IS NOT NULL;
CREATE UNIQUE INDEX scope_nodes_one_root ON scope_nodes (tenant_id, axis_code)
    WHERE is_axis_root;
CREATE UNIQUE INDEX scope_nodes_extref ON scope_nodes (tenant_id, axis_code, external_ref)
    WHERE external_ref IS NOT NULL;
-- snapshot loader: pull one axis for one tenant
CREATE INDEX scope_nodes_axis ON scope_nodes (tenant_id, axis_code)
    INCLUDE (parent_id, node_type) WHERE status = 'active';

-- ---------------------------------------------------------------------------
-- scope_closure — transitive reachability. THE hot-path table.
--
-- DELIBERATELY NARROW. tenant_id and axis_code are omitted even though they
-- would allow composite FKs here, because:
--   * the invariant is already enforced upstream by scope_nodes.parent_id,
--     and closure rows are only ever written by the functions below --
--     never by user input;
--   * dropping them takes the row from ~90 to ~58 bytes, ~35% narrower.
--     More rows per 8KB page = higher cache hit ratio = fewer buffer reads
--     on the single most-probed table in the system.
--
-- depth is smallint: no authorization hierarchy is 32k levels deep, and
-- honest widths keep the row narrow.
-- ---------------------------------------------------------------------------
CREATE TABLE scope_closure (
    ancestor_id   uuid NOT NULL,
    descendant_id uuid NOT NULL,
    depth         smallint NOT NULL CHECK (depth >= 0),   -- 0 = self

    PRIMARY KEY (ancestor_id, descendant_id),
    FOREIGN KEY (ancestor_id)   REFERENCES scope_nodes(id) ON DELETE CASCADE,
    FOREIGN KEY (descendant_id) REFERENCES scope_nodes(id) ON DELETE CASCADE
) WITH (fillfactor = 95);   -- append-mostly; near-zero updates

-- Reverse direction: "all ancestors of target T". Covering INCLUDE(depth)
-- makes this an index-only scan -- the heap is never touched.
CREATE INDEX scope_closure_up ON scope_closure (descendant_id, ancestor_id)
    INCLUDE (depth);

-- ---------------------------------------------------------------------------
-- Closure maintenance
-- ---------------------------------------------------------------------------

-- Create an axis root for a tenant. Makes "unrestricted on this axis" an
-- EXPLICIT foreign key rather than the absence of a row -- so a later flip
-- to default_effect='deny' can distinguish deliberate breadth from legacy.
CREATE OR REPLACE FUNCTION scope_ensure_root(p_tenant uuid, p_axis text)
RETURNS uuid LANGUAGE plpgsql AS $fn$
DECLARE
    v_id uuid;
    v_type text;
BEGIN
    SELECT id INTO v_id FROM scope_nodes
     WHERE tenant_id = p_tenant AND axis_code = p_axis AND is_axis_root;
    IF FOUND THEN RETURN v_id; END IF;

    -- The root type is the one with NO legal parents. Picking alphabetically
    -- silently produced roots typed 'department' on the org axis.
    SELECT code INTO v_type FROM scope_node_types
     WHERE axis_code = p_axis AND cardinality(parent_types) = 0
     ORDER BY code LIMIT 1;
    IF v_type IS NULL THEN
        RAISE EXCEPTION 'axis % has no root node type (none with empty parent_types)', p_axis;
    END IF;

    INSERT INTO scope_nodes (tenant_id, axis_code, node_type, slug, name, is_axis_root)
         VALUES (p_tenant, p_axis, v_type, '_root', 'All ' || p_axis, true)
      RETURNING id INTO v_id;

    INSERT INTO scope_closure (ancestor_id, descendant_id, depth)
         VALUES (v_id, v_id, 0);
    RETURN v_id;
END;
$fn$;

-- Attach a node and materialise its closure edges. O(depth) inserts.
CREATE OR REPLACE FUNCTION scope_add_node(
    p_tenant uuid, p_axis text, p_type text, p_parent uuid,
    p_slug text, p_name text, p_external_ref text DEFAULT NULL
) RETURNS uuid LANGUAGE plpgsql AS $fn$
DECLARE v_id uuid;
BEGIN
    INSERT INTO scope_nodes (tenant_id, axis_code, node_type, parent_id,
                             slug, name, external_ref)
         VALUES (p_tenant, p_axis, p_type, p_parent, p_slug, p_name, p_external_ref)
      RETURNING id INTO v_id;

    INSERT INTO scope_closure (ancestor_id, descendant_id, depth)
    SELECT c.ancestor_id, v_id, c.depth + 1
      FROM scope_closure c
     WHERE c.descendant_id = p_parent
     UNION ALL
    SELECT v_id, v_id, 0;

    RETURN v_id;
END;
$fn$;

-- Move a subtree. Cycle detection is ONE indexed probe -- the property that
-- materialised paths cannot match without string comparison over descendants.
CREATE OR REPLACE FUNCTION scope_move_node(p_node uuid, p_new_parent uuid)
RETURNS void LANGUAGE plpgsql AS $fn$
BEGIN
    IF EXISTS (SELECT 1 FROM scope_closure
                WHERE ancestor_id = p_node AND descendant_id = p_new_parent) THEN
        RAISE EXCEPTION 'cycle: % is inside the subtree of %', p_new_parent, p_node;
    END IF;

    -- sever links from former ancestors into the moving subtree
    DELETE FROM scope_closure
     WHERE descendant_id IN (SELECT descendant_id FROM scope_closure WHERE ancestor_id = p_node)
       AND ancestor_id   IN (SELECT ancestor_id  FROM scope_closure
                              WHERE descendant_id = p_node AND ancestor_id <> p_node);

    -- graft onto new ancestors
    INSERT INTO scope_closure (ancestor_id, descendant_id, depth)
    SELECT sup.ancestor_id, sub.descendant_id, sup.depth + sub.depth + 1
      FROM scope_closure sup
      CROSS JOIN scope_closure sub
     WHERE sup.descendant_id = p_new_parent
       AND sub.ancestor_id   = p_node;

    UPDATE scope_nodes SET parent_id = p_new_parent, updated_at = now()
     WHERE id = p_node;
END;
$fn$;
