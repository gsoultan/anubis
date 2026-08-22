\set ON_ERROR_STOP on
\timing off

-- Probe construction must be DETERMINISTIC. An earlier version took the first
-- grant it found and supplied only the 'org' target; when the seed grew, LIMIT 1
-- returned a grant that also constrained 'product' and the correct fail-closed
-- deny looked like a regression. Build the full target map from the grant's own
-- constraints instead, then vary one axis deliberately.
CREATE TEMP TABLE probe AS
SELECT g.id AS grant_id, g.identity_id, g.tenant_id, p.key AS perm,
       gso.scope_node_id AS granted_org,
       (SELECT jsonb_object_agg(x.axis_code, x.scope_node_id)
          FROM grant_scopes x WHERE x.grant_id = g.id) AS full_target,
       (SELECT c.descendant_id FROM scope_closure c
          JOIN scope_nodes n ON n.id=c.descendant_id AND n.node_type='team'
         WHERE c.ancestor_id = gso.scope_node_id LIMIT 1) AS deep_org
  FROM grants g
  JOIN grant_scopes gso ON gso.grant_id=g.id AND gso.axis_code='org'
  JOIN role_permissions_effective rpe ON rpe.role_id=g.role_id
  JOIN permissions p ON p.id=rpe.permission_id
  JOIN identities i ON i.id=g.identity_id
 WHERE g.self_scoped = false AND i.status='active'
   AND (SELECT count(*) FROM grant_scopes x WHERE x.grant_id=g.id) >= 1
 ORDER BY g.id LIMIT 1;

CREATE TEMP TABLE foreign_node AS
SELECT n.id FROM scope_nodes n, probe pr
 WHERE n.node_type='team'
   AND NOT EXISTS (SELECT 1 FROM scope_closure c
                    WHERE c.ancestor_id=pr.granted_org AND c.descendant_id=n.id)
 ORDER BY n.id LIMIT 1;

\echo '=== CORRECTNESS ==='
SELECT 'exact scope match -> ALLOW' AS case,
       authorize(identity_id, tenant_id, perm, full_target) AS got, true AS want
  FROM probe
UNION ALL
SELECT 'inherited descendant -> ALLOW',
       authorize(identity_id, tenant_id, perm,
                 full_target || jsonb_build_object('org', deep_org)), true FROM probe
UNION ALL
SELECT 'different subtree -> DENY',
       authorize(p.identity_id, p.tenant_id, p.perm,
                 p.full_target || jsonb_build_object('org', f.id)), false
  FROM probe p, foreign_node f
UNION ALL
SELECT 'permission not held -> DENY',
       authorize(identity_id, tenant_id, 'app-9:nonexistent:action', full_target), false
  FROM probe
UNION ALL
SELECT 'FAIL-CLOSED: axis omitted -> DENY',
       authorize(identity_id, tenant_id, perm, '{}'::jsonb), false FROM probe
UNION ALL
SELECT 'cross-tenant identity -> DENY',
       authorize(identity_id, (SELECT id FROM tenants WHERE slug='other'),
                 perm, full_target), false FROM probe;

\echo ''
\echo '=== OR WITHIN AN AXIS (multi-node grants) ==='
-- A fresh identity holding ONE grant with TWO offices on the org axis.
-- "Assigned to multiple offices" is the requirement; these probes are what
-- keeps the OR semantics from regressing again.
CREATE TEMP TABLE mg AS
WITH r AS (SELECT id AS realm_id, tenant_id FROM realms WHERE code='internal' LIMIT 1),
i AS (INSERT INTO identities (tenant_id, realm_id, assurance_level, username, email)
      SELECT tenant_id, realm_id, 3, 'or_axis_probe', 'or_axis@test.local' FROM r
      RETURNING id, tenant_id),
g AS (INSERT INTO grants (tenant_id, identity_id, role_id, granted_by)
      SELECT i.tenant_id, i.id, (SELECT role_id FROM grants WHERE id=(SELECT grant_id FROM probe)), i.id
        FROM i RETURNING id, tenant_id, identity_id),
offs AS (SELECT id, row_number() OVER (ORDER BY id) rn FROM scope_nodes
          WHERE node_type='office' AND tenant_id=(SELECT tenant_id FROM g) LIMIT 3),
sc AS (INSERT INTO grant_scopes (grant_id, tenant_id, axis_code, scope_node_id)
       SELECT g.id, g.tenant_id, 'org', o.id FROM g JOIN offs o ON o.rn IN (1,2)
       RETURNING grant_id)
SELECT g.identity_id, g.tenant_id, (SELECT perm FROM probe) AS perm,
  (SELECT c.descendant_id FROM scope_closure c JOIN scope_nodes n
     ON n.id=c.descendant_id AND n.node_type='team'
    WHERE c.ancestor_id=(SELECT id FROM offs WHERE rn=1) LIMIT 1) AS under_first,
  (SELECT c.descendant_id FROM scope_closure c JOIN scope_nodes n
     ON n.id=c.descendant_id AND n.node_type='team'
    WHERE c.ancestor_id=(SELECT id FROM offs WHERE rn=2) LIMIT 1) AS under_second,
  (SELECT c.descendant_id FROM scope_closure c JOIN scope_nodes n
     ON n.id=c.descendant_id AND n.node_type='team'
    WHERE c.ancestor_id=(SELECT id FROM offs WHERE rn=3) LIMIT 1) AS under_third
FROM g;

SELECT 'under FIRST of two offices -> ALLOW' AS case,
       authorize(identity_id, tenant_id, perm, jsonb_build_object('org', under_first)) AS got,
       true AS want FROM mg
UNION ALL
SELECT 'under SECOND of two offices -> ALLOW',
       authorize(identity_id, tenant_id, perm, jsonb_build_object('org', under_second)), true FROM mg
UNION ALL
SELECT 'under an UNGRANTED third office -> DENY',
       authorize(identity_id, tenant_id, perm, jsonb_build_object('org', under_third)), false FROM mg;

DELETE FROM identities WHERE username='or_axis_probe';

\echo ''
\echo '=== THROUGHPUT: 20,000 decisions ==='
\timing on
SELECT count(*) FILTER (WHERE ok) AS allowed, count(*) FILTER (WHERE NOT ok) AS denied
FROM (
  SELECT authorize(g.identity_id, g.tenant_id, p.key,
           (SELECT jsonb_object_agg(x.axis_code, x.scope_node_id)
              FROM grant_scopes x WHERE x.grant_id=g.id)) AS ok
    FROM grants g
    JOIN role_permissions_effective rpe ON rpe.role_id=g.role_id
    JOIN permissions p ON p.id=rpe.permission_id
   LIMIT 20000
) s;
\timing off
