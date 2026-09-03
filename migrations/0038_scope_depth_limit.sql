-- ---------------------------------------------------------------------------
-- Name the depth ceiling instead of letting it surface as a type error.
--
-- scope_closure.depth is smallint, so the 32,768th level of a chain fails
-- with:
--
--     ERROR:  smallint out of range
--
-- which names neither the table, the node, nor the operation. Someone hitting
-- it during a sync from an external system has no way to tell it apart from an
-- overflow anywhere else in the transaction.
--
-- The ceiling itself is NOT changed: everything that inserted successfully
-- before still does. This only replaces the message, and adds the same guard
-- to scope_move_node, where grafting a deep subtree under a deep parent can
-- overflow on sup.depth + sub.depth + 1 even though neither side was close to
-- the limit on its own.
--
-- Worth stating plainly, because the smallint bound is not the real budget:
-- a closure is quadratic in chain length. A 2,000-deep chain is already
-- 2,003,001 closure rows and ~26 s to build; 32,767 would be ~537M rows.
-- Anything approaching these numbers is a data-modelling accident — a
-- self-referencing ERP export, usually — not a hierarchy. The guard exists to
-- say so out loud.
-- ---------------------------------------------------------------------------

CREATE OR REPLACE FUNCTION scope_max_depth() RETURNS integer
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $fn$
    SELECT 32767;   -- the smallint ceiling on scope_closure.depth
$fn$;

CREATE OR REPLACE FUNCTION scope_add_node(
    p_tenant uuid, p_axis text, p_type text, p_parent uuid,
    p_slug text, p_name text, p_external_ref text DEFAULT NULL
) RETURNS uuid LANGUAGE plpgsql AS $fn$
DECLARE
    v_id uuid;
    v_parent_depth integer;
BEGIN
    -- The parent's own distance from its axis root. One indexed probe.
    SELECT max(depth) INTO v_parent_depth
      FROM scope_closure WHERE descendant_id = p_parent;

    IF v_parent_depth IS NOT NULL AND v_parent_depth + 1 > scope_max_depth() THEN
        RAISE EXCEPTION
            'scope hierarchy too deep: attaching %.% under % would reach depth %, limit is %',
            p_axis, p_slug, p_parent, v_parent_depth + 1, scope_max_depth()
            USING ERRCODE = 'program_limit_exceeded',
                  HINT = 'A chain this long is almost always a cycle or a self-referencing external feed, not a real hierarchy.';
    END IF;

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

CREATE OR REPLACE FUNCTION scope_move_node(p_node uuid, p_new_parent uuid)
RETURNS void LANGUAGE plpgsql AS $fn$
DECLARE
    v_new_parent_depth integer;
    v_subtree_depth    integer;
BEGIN
    IF EXISTS (SELECT 1 FROM scope_closure
                WHERE ancestor_id = p_node AND descendant_id = p_new_parent) THEN
        RAISE EXCEPTION 'cycle: % is inside the subtree of %', p_new_parent, p_node;
    END IF;

    -- Deepest resulting node = new parent's depth + 1 + the subtree's own
    -- height. Both halves can be legal while the graft is not.
    SELECT max(depth) INTO v_new_parent_depth
      FROM scope_closure WHERE descendant_id = p_new_parent;
    SELECT max(depth) INTO v_subtree_depth
      FROM scope_closure WHERE ancestor_id = p_node;

    IF COALESCE(v_new_parent_depth, 0) + COALESCE(v_subtree_depth, 0) + 1 > scope_max_depth() THEN
        RAISE EXCEPTION
            'scope hierarchy too deep: moving % under % would reach depth %, limit is %',
            p_node, p_new_parent,
            COALESCE(v_new_parent_depth, 0) + COALESCE(v_subtree_depth, 0) + 1,
            scope_max_depth()
            USING ERRCODE = 'program_limit_exceeded';
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
