-- ============================================================================
-- 0014_node_type_guard.sql — enforce level rules in the schema
--
-- scope_node_types.parent_types was documented in 0003 as "validated in the
-- domain layer" — a layer that does not exist yet. Until now nothing in the
-- database stopped a team being parented directly under an office, skipping
-- the department level entirely. Hierarchy legality is an authorization-
-- relevant invariant (grants inherit down these edges), so it belongs where
-- the other eight guards live: in the schema, immune to application bugs.
--
-- Depth itself needs no work: the closure table is depth-agnostic, so adding
-- a Division level between office and department (data, in the seed) makes
-- office → division → department → team a five-level chain that inherit
-- already traverses. A division-level grant sees every department below it;
-- a department-level grant sees only its own subtree.
-- ============================================================================

CREATE OR REPLACE FUNCTION trg_scope_node_type_guard()
RETURNS trigger LANGUAGE plpgsql AS $fn$
DECLARE
    v_parent_type text;
    v_legal       text[];
BEGIN
    -- Roots are covered by the existing CHECK pair (root ⇔ no parent).
    IF NEW.parent_id IS NULL THEN RETURN NEW; END IF;

    SELECT node_type INTO v_parent_type FROM scope_nodes WHERE id = NEW.parent_id;
    SELECT parent_types INTO v_legal FROM scope_node_types
     WHERE code = NEW.node_type AND axis_code = NEW.axis_code;

    IF NOT (v_parent_type = ANY (v_legal)) THEN
        RAISE EXCEPTION
          'a "%" may not sit under a "%" (legal parents: %)',
          NEW.node_type, v_parent_type, array_to_string(v_legal, ', ')
          USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$fn$;

CREATE CONSTRAINT TRIGGER scope_nodes_type_guard
    AFTER INSERT OR UPDATE OF parent_id, node_type ON scope_nodes
    DEFERRABLE INITIALLY IMMEDIATE
    FOR EACH ROW EXECUTE FUNCTION trg_scope_node_type_guard();
