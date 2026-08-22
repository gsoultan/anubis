CREATE TEMP TABLE mixed AS
SELECT g.identity_id, g.tenant_id, p.key AS perm,
       jsonb_build_object('org', gso.scope_node_id,
         'customer',(SELECT id FROM scope_nodes WHERE node_type='account'
                      AND tenant_id=g.tenant_id LIMIT 1)) AS targets
  FROM grants g
  JOIN grant_scopes gso ON gso.grant_id=g.id AND gso.axis_code='org'
  JOIN role_permissions_effective rpe ON rpe.role_id=g.role_id
  JOIN permissions p ON p.id=rpe.permission_id LIMIT 20000;

\timing on
\echo '=== 20,000 mixed-depth decisions ==='
SELECT count(*) FILTER (WHERE authorize(identity_id,tenant_id,perm,targets)) AS allowed FROM mixed;
\timing off

\echo ''
\echo '=== storage profile ==='
SELECT relname,
       to_char(n_live_tup,'999,999,999') AS rows,
       pg_size_pretty(pg_relation_size(c.oid))       AS heap,
       pg_size_pretty(pg_indexes_size(c.oid))        AS idx,
       pg_size_pretty(pg_total_relation_size(c.oid)) AS total
  FROM pg_stat_user_tables s JOIN pg_class c ON c.relname=s.relname
 WHERE n_live_tup > 100 ORDER BY pg_total_relation_size(c.oid) DESC LIMIT 8;

\echo ''
\echo '=== index usage: anything unused is dead weight ==='
SELECT indexrelname, idx_scan,
       pg_size_pretty(pg_relation_size(indexrelid)) sz
  FROM pg_stat_user_indexes
 WHERE relname IN ('scope_closure','grant_scopes','grants','permissions','role_permissions_effective')
 ORDER BY idx_scan DESC;
