-- These MUST all fail. If any succeeds, the schema does not enforce what the
-- design claims and the guarantee is only as good as application code.
\set ON_ERROR_STOP off
\echo '--- 1. grant_scope pointing at ANOTHER TENANT''s node (cross-tenant leak) ---'
INSERT INTO grant_scopes (grant_id, tenant_id, axis_code, scope_node_id)
SELECT g.id, g.tenant_id, 'org', n.id
  FROM grants g, scope_nodes n
 WHERE n.tenant_id = (SELECT id FROM tenants WHERE slug='other')
   AND n.axis_code='org' AND g.tenant_id=(SELECT id FROM tenants WHERE slug='impack')
 LIMIT 1;

\echo '--- 2. grant_scope claiming axis=product but pointing at an ORG node ---'
INSERT INTO grant_scopes (grant_id, tenant_id, axis_code, scope_node_id)
SELECT g.id, g.tenant_id, 'product', n.id
  FROM grants g, scope_nodes n
 WHERE n.tenant_id=g.tenant_id AND n.axis_code='org' AND n.node_type='department'
 LIMIT 1;

\echo '--- 3. scope_node parented ACROSS AXES (product under an office) ---'
INSERT INTO scope_nodes (tenant_id, axis_code, node_type, parent_id, slug, name)
SELECT n.tenant_id, 'product', 'sku', n.id, 'illegal-sku', 'Illegal'
  FROM scope_nodes n WHERE n.node_type='office' LIMIT 1;

\echo '--- 4. node_type belonging to a different axis ---'
INSERT INTO scope_nodes (tenant_id, axis_code, node_type, parent_id, slug, name)
SELECT n.tenant_id, 'org', 'sku', n.id, 'illegal-2', 'Illegal'
  FROM scope_nodes n WHERE n.node_type='department' LIMIT 1;

\echo '--- 4b. team directly under an office (level skipped, same axis) ---'
INSERT INTO scope_nodes (tenant_id, axis_code, node_type, parent_id, slug, name)
SELECT n.tenant_id,'org','team', n.id,'illegal-team','Illegal Team'
  FROM scope_nodes n WHERE n.node_type='office' LIMIT 1;

\echo '--- 5. permission app_slug drifting from its application ---'
UPDATE permissions SET app_slug='forged' WHERE key LIKE 'app-1:%';

\echo '--- 6. two axis roots for the same (tenant, axis) ---'
INSERT INTO scope_nodes (tenant_id, axis_code, node_type, slug, name, is_axis_root)
SELECT id, 'org', 'org', '_root2', 'Second root', true FROM tenants WHERE slug='impack';

\echo '--- 6b. deleting a role people still hold ---'
DELETE FROM roles WHERE id = (SELECT role_id FROM grants WHERE revoked_at IS NULL LIMIT 1);

\echo '--- 7. cycle in the scope tree ---'
SELECT scope_move_node(
  (SELECT id FROM scope_nodes WHERE node_type='office' LIMIT 1),
  (SELECT c.descendant_id FROM scope_closure c
     WHERE c.ancestor_id=(SELECT id FROM scope_nodes WHERE node_type='office' LIMIT 1)
       AND c.depth=2 LIMIT 1));
