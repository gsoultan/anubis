\set ON_ERROR_STOP on
-- Sync reconciliation on the partner structure (seed nodes carry ERP-SUP refs)
CREATE TEMP TABLE sy AS
WITH t AS (SELECT id FROM tenants WHERE slug='impack'),
s AS (INSERT INTO scope_sync_sources (tenant_id, axis_code, kind, config)
      SELECT id, 'partner', 'http',
        '{"url":"https://erp.impack.example/suppliers","default_node_type":"partner_org"}'
      FROM t RETURNING id, tenant_id)
SELECT s.id AS source_id, s.tenant_id FROM s;

-- feed: SUP-1 renamed, SUP-2..19 kept, SUP-20 dropped (archive), NEW-1 added,
-- BAD-1 carries an illegal type (rejected per row, run continues)
SELECT '  apply: '|| (
  SELECT report::text FROM (SELECT scope_sync_apply(source_id, (
    SELECT jsonb_agg(jsonb_build_object('ref', external_ref, 'parent_ref', NULL,
      'name', CASE WHEN external_ref='ERP-SUP-1' THEN 'Supplier 1 Intl' ELSE name END))
      FROM scope_nodes WHERE axis_code='partner' AND external_ref IS NOT NULL
       AND tenant_id=(SELECT tenant_id FROM sy)
       AND external_ref <> 'ERP-SUP-20' AND external_ref NOT LIKE 'ERP-NEW-%')
    || jsonb_build_array(
         jsonb_build_object('ref','ERP-NEW-1','parent_ref',NULL,'name','Wayne Polymers'),
         jsonb_build_object('ref','ERP-BAD-1','parent_ref',NULL,'name','Bad Row','node_type','sku'))
  ) AS report FROM sy) x
) AS out FROM sy LIMIT 1;

SELECT '  renamed applied: '||(EXISTS(SELECT 1 FROM scope_nodes n, sy WHERE n.tenant_id=sy.tenant_id AND n.external_ref='ERP-SUP-1' AND n.name='Supplier 1 Intl'))::text
     ||' · added: '||(EXISTS(SELECT 1 FROM scope_nodes n, sy WHERE n.tenant_id=sy.tenant_id AND n.external_ref='ERP-NEW-1'))::text
     ||' · archived: '||(SELECT (n.status='archived')::text FROM scope_nodes n, sy WHERE n.tenant_id=sy.tenant_id AND n.external_ref='ERP-SUP-20');

-- access on an archived node keeps working (archive is not revocation).
-- Two statements on purpose: a SELECT cannot see rows inserted by its own
-- data-modifying CTEs (single-snapshot rule), so the grant must be committed
-- by a prior statement before authorize() can observe it.
CREATE TEMP TABLE gp AS
WITH r AS (SELECT id realm_id, tenant_id FROM realms WHERE code='partner' LIMIT 1),
i AS (INSERT INTO identities (tenant_id, realm_id, assurance_level, username, email)
      SELECT tenant_id, realm_id, 2, 'sync_probe', 'sp@t.t' FROM r RETURNING id, tenant_id),
g AS (INSERT INTO grants (tenant_id, identity_id, role_id, granted_by)
      SELECT i.tenant_id, i.id,
        (SELECT id FROM roles WHERE name='partner.portal' LIMIT 1), i.id FROM i
      RETURNING id, tenant_id, identity_id),
gs AS (INSERT INTO grant_scopes (grant_id, tenant_id, axis_code, scope_node_id)
       SELECT g.id, g.tenant_id, 'partner',
         (SELECT id FROM scope_nodes n WHERE n.tenant_id=g.tenant_id
           AND n.external_ref='ERP-SUP-20') FROM g RETURNING grant_id)
SELECT g.identity_id, g.tenant_id,
  (SELECT p.key FROM role_permissions_effective rpe JOIN permissions p ON p.id=rpe.permission_id
    WHERE rpe.role_id=(SELECT id FROM roles WHERE name='partner.portal' LIMIT 1)
    ORDER BY p.key LIMIT 1) AS perm
FROM g;

SELECT '  grant on archived node still decides: '||authorize(identity_id, tenant_id, perm,
  jsonb_build_object('partner',(SELECT n.id FROM scope_nodes n
    WHERE n.tenant_id=gp.tenant_id AND n.external_ref='ERP-SUP-20')))::text
  ||' (want t)' FROM gp;

-- idempotent: same feed again -> everything unchanged, dry run touches nothing
SELECT '  rerun-dry: '||(scope_sync_apply(source_id, (
    SELECT jsonb_agg(jsonb_build_object('ref', external_ref, 'parent_ref', NULL, 'name', name))
      FROM scope_nodes WHERE axis_code='partner' AND external_ref IS NOT NULL AND status='active'
       AND tenant_id=(SELECT tenant_id FROM sy)
  ), true))::text FROM sy;

DELETE FROM identities WHERE username='sync_probe';
DELETE FROM scope_sync_sources WHERE id=(SELECT source_id FROM sy);
