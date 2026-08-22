-- THE REQUIREMENT: add a brand-new access dimension with no DDL, no migration,
-- no redeploy, and no breakage of the 150,000 grants already in flight.
\set ON_ERROR_STOP on
CREATE TEMP TABLE before_state AS
SELECT g.identity_id, g.tenant_id, p.key perm,
       jsonb_build_object('org', gso.scope_node_id) tgt,
       authorize(g.identity_id,g.tenant_id,p.key,
                 jsonb_build_object('org', gso.scope_node_id)) AS decision
  FROM grants g
  JOIN grant_scopes gso ON gso.grant_id=g.id AND gso.axis_code='org'
  JOIN role_permissions_effective rpe ON rpe.role_id=g.role_id
  JOIN permissions p ON p.id=rpe.permission_id LIMIT 2000;

\echo '=== STEP 1: register the axis (3 INSERTs, zero DDL) ==='
INSERT INTO scope_axes (code, display_name, default_effect, sort_order, resolution, ui_schema)
VALUES ('cost_center','Cost Centre','unconstrained',40,
        '{"from":"context","key":"cost_center_id"}',
        '{"picker":"tree","searchable":true,"icon":"wallet"}');
INSERT INTO scope_node_types (code, axis_code, display_name, parent_types) VALUES
  ('cc_root','cost_center','All Cost Centres','{}'),
  ('cc_division','cost_center','Division','{cc_root}'),
  ('cc_center','cost_center','Cost Centre','{cc_division}');
SELECT scope_ensure_root(id,'cost_center') IS NOT NULL AS root_created
  FROM tenants WHERE slug='impack';

\echo '=== STEP 2: bulk-sync 500 nodes from the ERP ==='
INSERT INTO scope_nodes (tenant_id, axis_code, node_type, parent_id, slug, name, external_ref)
SELECT r.tenant_id,'cost_center','cc_division', r.id, 'div-'||g, 'Division '||g, 'ERP-DIV-'||g
  FROM scope_nodes r CROSS JOIN generate_series(1,10) g
 WHERE r.axis_code='cost_center' AND r.is_axis_root;
INSERT INTO scope_closure SELECT c.ancestor_id,n.id,c.depth+1 FROM scope_nodes n
  JOIN scope_closure c ON c.descendant_id=n.parent_id WHERE n.node_type='cc_division'
UNION ALL SELECT n.id,n.id,0 FROM scope_nodes n WHERE n.node_type='cc_division';
INSERT INTO scope_nodes (tenant_id, axis_code, node_type, parent_id, slug, name, external_ref)
SELECT p.tenant_id,'cost_center','cc_center', p.id, 'cc-'||g, 'CC '||g, 'ERP-CC-'||p.slug||'-'||g
  FROM scope_nodes p CROSS JOIN generate_series(1,50) g WHERE p.node_type='cc_division';
INSERT INTO scope_closure SELECT c.ancestor_id,n.id,c.depth+1 FROM scope_nodes n
  JOIN scope_closure c ON c.descendant_id=n.parent_id WHERE n.node_type='cc_center'
UNION ALL SELECT n.id,n.id,0 FROM scope_nodes n WHERE n.node_type='cc_center';
ANALYZE scope_nodes; ANALYZE scope_closure;
SELECT count(*) AS new_nodes FROM scope_nodes WHERE axis_code='cost_center';

\echo ''
\echo '=== STEP 3: do the 150,000 existing grants still behave identically? ==='
SELECT count(*) AS total,
       count(*) FILTER (WHERE b.decision <> authorize(b.identity_id,b.tenant_id,b.perm,b.tgt))
         AS changed_decisions
  FROM before_state b;

\echo ''
\echo '=== STEP 4: constrain one grant to a cost centre; verify it binds ==='
WITH one AS (SELECT grant_id FROM grant_scopes WHERE axis_code='org' LIMIT 1),
     cc  AS (SELECT id, tenant_id FROM scope_nodes WHERE node_type='cc_division' LIMIT 1)
INSERT INTO grant_scopes (grant_id, tenant_id, axis_code, scope_node_id)
SELECT one.grant_id, cc.tenant_id, 'cost_center', cc.id FROM one, cc
ON CONFLICT DO NOTHING;

-- NOTE: the target must supply EVERY axis the grant constrains. Supplying a
-- subset is correctly DENIED (fail-closed) -- an earlier version of this test
-- supplied only org+cost_center on a grant that also constrained product and
-- customer, and read the resulting deny as a bug. It was the engine working.
WITH g AS (SELECT grant_id FROM grant_scopes WHERE axis_code='cost_center' LIMIT 1),
     ft AS (SELECT jsonb_object_agg(gs.axis_code, gs.scope_node_id) t
              FROM grant_scopes gs, g WHERE gs.grant_id=g.grant_id)
SELECT 'all axes supplied -> ALLOW' AS case,
       authorize(gr.identity_id, gr.tenant_id, p.key, ft.t) AS got, true AS want
  FROM g JOIN grants gr ON gr.id=g.grant_id
  JOIN role_permissions_effective rpe ON rpe.role_id=gr.role_id
  JOIN permissions p ON p.id=rpe.permission_id CROSS JOIN ft
UNION ALL
SELECT 'wrong cost centre -> DENY',
       authorize(gr.identity_id, gr.tenant_id, p.key,
                 ft.t || jsonb_build_object('cost_center',
                   (SELECT id FROM scope_nodes WHERE node_type='cc_division'
                     AND id <> (ft.t->>'cost_center')::uuid LIMIT 1))), false
  FROM g JOIN grants gr ON gr.id=g.grant_id
  JOIN role_permissions_effective rpe ON rpe.role_id=gr.role_id
  JOIN permissions p ON p.id=rpe.permission_id CROSS JOIN ft
UNION ALL
SELECT 'subset of axes -> DENY (fail-closed)',
       authorize(gr.identity_id, gr.tenant_id, p.key,
                 jsonb_build_object('cost_center', ft.t->'cost_center')), false
  FROM g JOIN grants gr ON gr.id=g.grant_id
  JOIN role_permissions_effective rpe ON rpe.role_id=gr.role_id
  JOIN permissions p ON p.id=rpe.permission_id CROSS JOIN ft;

\echo ''
\echo '=== STEP 5: DRY-RUN flipping the axis to strict (deny) ==='
UPDATE scope_axes SET default_effect='deny' WHERE code='cost_center';
SELECT count(*) FILTER (WHERE b.decision) AS allowed_before,
       count(*) FILTER (WHERE authorize(b.identity_id,b.tenant_id,b.perm,b.tgt)) AS allowed_after
  FROM before_state b;
UPDATE scope_axes SET default_effect='unconstrained' WHERE code='cost_center';
